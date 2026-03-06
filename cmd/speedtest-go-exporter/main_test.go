package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zakame/speedtest-go-exporter/internal/exporter"
)

// mockRunner is a test implementation of exporter.Runner that returns
// deterministic results so tests avoid real network calls.
type mockRunner struct{}

func (m mockRunner) Run() *exporter.SpeedtestResult {
	return &exporter.SpeedtestResult{
		ServerID:      123,
		DownloadSpeed: 1000.0,
		UploadSpeed:   500.0,
		Jitter:        1.0,
		Ping:          2.0,
	}
}

func TestRootHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Test the root handler directly
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("content-type", "text/html")
		_, _ = fmt.Fprintf(w, "See the <a href='/metrics'>metrics</a>.") // nolint:errcheck
	}).ServeHTTP(w, req)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.StatusCode)
	}
	if !strings.Contains(string(body), "See the <a href='/metrics'>metrics</a>.") {
		t.Errorf("Unexpected response body: %s", string(body))
	}
	if resp.Header.Get("content-type") != "text/html" {
		t.Errorf("Expected content-type text/html, got %s", resp.Header.Get("content-type"))
	}
}

func TestPortDefault(t *testing.T) {
	originalPort := os.Getenv("SPEEDTEST_PORT")
	if err := os.Unsetenv("SPEEDTEST_PORT"); err != nil {
		t.Fatalf("failed to unset SPEEDTEST_PORT: %v", err)
	}
	expected := "9798"
	if got := getPort(); got != expected {
		t.Errorf("default port: expected %s, got %s", expected, got)
	}
	if err := os.Setenv("SPEEDTEST_PORT", originalPort); err != nil {
		t.Errorf("failed to restore SPEEDTEST_PORT: %v", err)
	}
}

func TestPortOverride(t *testing.T) {
	originalPort := os.Getenv("SPEEDTEST_PORT")
	if err := os.Setenv("SPEEDTEST_PORT", "8080"); err != nil {
		t.Fatalf("failed to set SPEEDTEST_PORT: %v", err)
	}
	if got := getPort(); got != "8080" {
		t.Errorf("override port: expected 8080, got %s", got)
	}
	if err := os.Setenv("SPEEDTEST_PORT", originalPort); err != nil {
		t.Errorf("failed to restore SPEEDTEST_PORT: %v", err)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	// Create a test registry
	reg := prometheus.NewPedanticRegistry()
	// Register a mock runner so tests do not perform real network speedtests.
	exporter.RegisterSpeedtestCollector(mockRunner{}, reg)

	// Create a test server with the metrics handler
	ts := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer ts.Close()

	// Make a request to the test server
	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("Error making request to test server: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err)
	}

	if len(body) == 0 {
		t.Error("Expected non-empty response from metrics endpoint")
	}

	if err := resp.Body.Close(); err != nil {
		t.Errorf("failed to close response body: %v", err)
	}
}

func TestNewRegistry_DebugEnabled(t *testing.T) {
	// Mock the runner creation to avoid real network calls
	origRunner := newSpeedtestRunner
	newSpeedtestRunner = func(serverID string, reg prometheus.Registerer) {
		exporter.RegisterSpeedtestCollector(mockRunner{}, reg)
	}
	defer func() { newSpeedtestRunner = origRunner }()

	reg := newRegistry("test-server", true)

	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	// Gather metrics and verify debug collectors are present
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	// Should have speedtest, process_, and go_ metrics when debug is true
	hasSpeedtestMetrics := false
	hasProcessMetrics := false
	hasGoMetrics := false
	for _, mf := range metrics {
		if strings.HasPrefix(mf.GetName(), "speedtest_") {
			hasSpeedtestMetrics = true
		}
		if strings.HasPrefix(mf.GetName(), "process_") {
			hasProcessMetrics = true
		}
		if strings.HasPrefix(mf.GetName(), "go_") {
			hasGoMetrics = true
		}
	}

	if !hasSpeedtestMetrics {
		t.Error("Expected speedtest metrics")
	}
	if !hasProcessMetrics {
		t.Error("Expected process_ metrics when debug enabled")
	}
	if !hasGoMetrics {
		t.Error("Expected go_ metrics when debug enabled")
	}
}

func TestNewRegistry_DebugDisabled(t *testing.T) {
	// Mock the runner creation to avoid real network calls
	origRunner := newSpeedtestRunner
	newSpeedtestRunner = func(serverID string, reg prometheus.Registerer) {
		exporter.RegisterSpeedtestCollector(mockRunner{}, reg)
	}
	defer func() { newSpeedtestRunner = origRunner }()

	reg := newRegistry("", false)

	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	// Gather metrics
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	// Should have speedtest metrics but NOT process/go metrics when debug is false
	hasSpeedtestMetrics := false
	hasProcessMetrics := false
	hasGoMetrics := false
	for _, mf := range metrics {
		if strings.HasPrefix(mf.GetName(), "speedtest_") {
			hasSpeedtestMetrics = true
		}
		if strings.HasPrefix(mf.GetName(), "process_") {
			hasProcessMetrics = true
		}
		if strings.HasPrefix(mf.GetName(), "go_") {
			hasGoMetrics = true
		}
	}

	if !hasSpeedtestMetrics {
		t.Error("Expected speedtest metrics")
	}
	if hasProcessMetrics {
		t.Error("Did not expect process_ metrics when debug disabled")
	}
	if hasGoMetrics {
		t.Error("Did not expect go_ metrics when debug disabled")
	}
}

func TestNewRegistry_WithServerID(t *testing.T) {
	// Mock the runner creation to avoid real network calls
	origRunner := newSpeedtestRunner
	var capturedServerID string
	newSpeedtestRunner = func(serverID string, reg prometheus.Registerer) {
		capturedServerID = serverID
		exporter.RegisterSpeedtestCollector(mockRunner{}, reg)
	}
	defer func() { newSpeedtestRunner = origRunner }()

	reg := newRegistry("12345", false)

	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	// Verify the server ID was passed through
	if capturedServerID != "12345" {
		t.Errorf("expected server ID '12345', got '%s'", capturedServerID)
	}
}

func TestDebugCollectors(t *testing.T) {
	originalDebug := os.Getenv("SPEEDTEST_EXPORTER_DEBUG")
	defer func() {
		if err := os.Setenv("SPEEDTEST_EXPORTER_DEBUG", originalDebug); err != nil {
			t.Errorf("failed to restore SPEEDTEST_EXPORTER_DEBUG: %v", err)
		}
	}()

	if err := os.Setenv("SPEEDTEST_EXPORTER_DEBUG", "true"); err != nil {
		t.Fatalf("failed to set SPEEDTEST_EXPORTER_DEBUG: %v", err)
	}
	reg := prometheus.NewPedanticRegistry()
	if debugEnabled() {
		reg.MustRegister(
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			collectors.NewGoCollector(),
		)
	}

	// there should be at least one process metric when debug is on
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	hasProcessMetrics := false
	for _, mf := range metrics {
		if strings.HasPrefix(mf.GetName(), "process_") {
			hasProcessMetrics = true
			break
		}
	}

	if !hasProcessMetrics {
		t.Error("No process metrics found when debug is enabled")
	}
}

func TestGetServerID(t *testing.T) {
	originalServer := os.Getenv("SPEEDTEST_SERVER")
	defer func() {
		if err := os.Setenv("SPEEDTEST_SERVER", originalServer); err != nil {
			t.Errorf("failed to restore SPEEDTEST_SERVER: %v", err)
		}
	}()

	if err := os.Setenv("SPEEDTEST_SERVER", "xyz"); err != nil {
		t.Fatalf("failed to set SPEEDTEST_SERVER: %v", err)
	}
	if got := getServerID(); got != "xyz" {
		t.Errorf("expected server id 'xyz', got '%s'", got)
	}
}

func TestDebugEnabled(t *testing.T) {
	originalDebug := os.Getenv("SPEEDTEST_EXPORTER_DEBUG")
	defer func() {
		if err := os.Setenv("SPEEDTEST_EXPORTER_DEBUG", originalDebug); err != nil {
			t.Errorf("failed to restore SPEEDTEST_EXPORTER_DEBUG: %v", err)
		}
	}()

	if err := os.Unsetenv("SPEEDTEST_EXPORTER_DEBUG"); err != nil {
		t.Fatalf("failed to unset SPEEDTEST_EXPORTER_DEBUG: %v", err)
	}
	if debugEnabled() {
		t.Error("debugEnabled should be false when env unset")
	}
	if err := os.Setenv("SPEEDTEST_EXPORTER_DEBUG", "1"); err != nil {
		t.Fatalf("failed to set SPEEDTEST_EXPORTER_DEBUG: %v", err)
	}
	if !debugEnabled() {
		t.Error("debugEnabled should be true when env is non-empty")
	}
}

func TestNewMuxHandlers(t *testing.T) {
	// build a simple registry with one dummy metric
	dummy := prometheus.NewCounter(prometheus.CounterOpts{Name: "dummy"})
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(dummy)
	mux := newMux(reg)

	// run the mux via httptest server
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("root request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("root handler status = %v", resp.StatusCode)
	}

	resp2, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("metrics request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("metrics handler status = %v", resp2.StatusCode)
	}
}

func TestRunError(t *testing.T) {
	// override the listener to avoid starting a real server or triggering
	// collection; also simulate an error so we exercise the error path.
	origListen := listenAndServe
	listenAndServe = func(addr string, handler http.Handler) error {
		return fmt.Errorf("simulated failure")
	}
	defer func() { listenAndServe = origListen }()

	// stub out newMux to avoid creating a handler that will gather and run the
	// real speedtest collector.
	origMux := newMux
	newMux = func(reg prometheus.Gatherer) *http.ServeMux {
		return http.NewServeMux()
	}
	defer func() { newMux = origMux }()

	err := run("any", "", false)
	if err == nil || !strings.Contains(err.Error(), "simulated failure") {
		t.Errorf("expected simulated failure, got %v", err)
	}
}
