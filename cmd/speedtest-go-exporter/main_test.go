package main

import (
	"context"
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

func (m mockRunner) Run(_ context.Context) (*exporter.SpeedtestResult, error) {
	return &exporter.SpeedtestResult{
		ServerID:      123,
		DownloadSpeed: 1000.0,
		UploadSpeed:   500.0,
		Jitter:        1.0,
		Ping:          2.0,
	}, nil
}

func TestRootHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Test the root handler directly
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("content-type", "text/html")
		_, _ = fmt.Fprintf(w, "See the <a href='/metrics'>metrics</a>.")
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
	defer func() {
		// Use Unsetenv when the variable was not set originally so we do not
		// leave SPEEDTEST_PORT set to an empty string, which would cause
		// subsequent getPort() calls in the same process to return "" instead
		// of the "9798" default.
		if originalPort == "" {
			_ = os.Unsetenv("SPEEDTEST_PORT")
		} else {
			_ = os.Setenv("SPEEDTEST_PORT", originalPort)
		}
	}()
	expected := "9798"
	if got := getPort(); got != expected {
		t.Errorf("default port: expected %s, got %s", expected, got)
	}
}

func TestPortOverride(t *testing.T) {
	originalPort := os.Getenv("SPEEDTEST_PORT")
	defer func() {
		if originalPort == "" {
			_ = os.Unsetenv("SPEEDTEST_PORT")
		} else {
			_ = os.Setenv("SPEEDTEST_PORT", originalPort)
		}
	}()
	if err := os.Setenv("SPEEDTEST_PORT", "8080"); err != nil {
		t.Fatalf("failed to set SPEEDTEST_PORT: %v", err)
	}
	if got := getPort(); got != "8080" {
		t.Errorf("override port: expected 8080, got %s", got)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	// Create a test registry
	reg := prometheus.NewPedanticRegistry()
	// Register a mock runner so tests do not perform real network speedtests.
	exporter.RegisterSpeedtestCollector(mockRunner{}, reg, exporter.DefaultCollectTimeout)

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
		exporter.RegisterSpeedtestCollector(mockRunner{}, reg, exporter.DefaultCollectTimeout)
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
		exporter.RegisterSpeedtestCollector(mockRunner{}, reg, exporter.DefaultCollectTimeout)
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
		exporter.RegisterSpeedtestCollector(mockRunner{}, reg, exporter.DefaultCollectTimeout)
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

// TestRun_AddressPassedToListenAndServe verifies that run() assembles the
// listen address as ":<port>" and passes it verbatim to listenAndServe.
// Without this test the port-assembly logic `":"+port` is exercised only
// indirectly and a change like omitting the colon would go unnoticed.
func TestRun_AddressPassedToListenAndServe(t *testing.T) {
	var capturedAddr string

	origListen := listenAndServe
	listenAndServe = func(addr string, _ http.Handler) error {
		capturedAddr = addr
		return nil // pretend the server started and immediately exited cleanly
	}
	defer func() { listenAndServe = origListen }()

	origMux := newMux
	newMux = func(reg prometheus.Gatherer) *http.ServeMux {
		return http.NewServeMux()
	}
	defer func() { newMux = origMux }()

	origRunner := newSpeedtestRunner
	newSpeedtestRunner = func(serverID string, reg prometheus.Registerer) {
		exporter.RegisterSpeedtestCollector(mockRunner{}, reg, exporter.DefaultCollectTimeout)
	}
	defer func() { newSpeedtestRunner = origRunner }()

	if err := run("9798", "", false); err != nil {
		t.Fatalf("unexpected error from run: %v", err)
	}

	if capturedAddr != ":9798" {
		t.Errorf("listen address: expected ':9798', got '%s'", capturedAddr)
	}
}

// TestGetServerID_Unset verifies that getServerID returns an empty string when
// SPEEDTEST_SERVER is not set.  This pins the "auto-select best server" default.
func TestGetServerID_Unset(t *testing.T) {
	original := os.Getenv("SPEEDTEST_SERVER")
	defer func() {
		if original == "" {
			_ = os.Unsetenv("SPEEDTEST_SERVER")
		} else {
			_ = os.Setenv("SPEEDTEST_SERVER", original)
		}
	}()

	if err := os.Unsetenv("SPEEDTEST_SERVER"); err != nil {
		t.Fatalf("failed to unset SPEEDTEST_SERVER: %v", err)
	}

	if got := getServerID(); got != "" {
		t.Errorf("expected empty server ID when env unset, got %q", got)
	}
}

// TestMetricsEndpoint_ContainsSpeedtestMetrics verifies that the /metrics
// endpoint response body contains actual speedtest metric names, not just
// some non-empty response.  The existing TestMetricsEndpoint only asserted
// len(body)>0 which would pass even if all speedtest metrics were silently
// dropped.
func TestMetricsEndpoint_ContainsSpeedtestMetrics(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	exporter.RegisterSpeedtestCollector(mockRunner{}, reg, exporter.DefaultCollectTimeout)

	ts := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("request to metrics endpoint failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %v", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	bodyStr := string(body)
	wantMetrics := []string{
		"speedtest_up",
		"speedtest_download_bits_per_second",
		"speedtest_upload_bits_per_second",
		"speedtest_ping_latency_milliseconds",
		"speedtest_jitter_latency_milliseconds",
		"speedtest_server_id",
	}
	for _, name := range wantMetrics {
		if !strings.Contains(bodyStr, name) {
			t.Errorf("metrics endpoint body missing %q", name)
		}
	}
}

// TestNewMuxHandlers_RootBody verifies that the production root handler (the
// one registered by newMux) produces the expected HTML body and content-type.
// TestRootHandler tests an inline anonymous func, not the actual production
// handler, and would stay green even if the production implementation changed.
func TestNewMuxHandlers_RootBody(t *testing.T) {
	dummy := prometheus.NewCounter(prometheus.CounterOpts{Name: "dummy_total"})
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(dummy)

	mux := newMux(reg)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("root request failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %v", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected content-type text/html, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if !strings.Contains(string(body), "/metrics") {
		t.Errorf("root response body should contain a link to /metrics, got: %s", string(body))
	}
}
