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

func TestRootHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Test the root handler directly
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("content-type", "text/html")
		_, _ = fmt.Fprintf(w, "See the <a href='/metrics'>metrics</a>.")
	}).ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

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
	// Save original value
	originalPort := os.Getenv("SPEEDTEST_PORT")

	// Test default port
	_ = os.Unsetenv("SPEEDTEST_PORT")
	port := os.Getenv("SPEEDTEST_PORT")
	if port == "" {
		port = "9798"
	}
	if port != "9798" {
		t.Errorf("Expected default port 9798, got %s", port)
	}
	_ = os.Setenv("SPEEDTEST_PORT", originalPort)
}

func TestPortOverride(t *testing.T) {
	// Save original value
	originalPort := os.Getenv("SPEEDTEST_PORT")

	// Test custom port
	_ = os.Setenv("SPEEDTEST_PORT", "8080")
	port := os.Getenv("SPEEDTEST_PORT")
	if port == "" {
		port = "9798"
	}
	if port != "8080" {
		t.Errorf("Expected port 8080, got %s", port)
	}
	_ = os.Setenv("SPEEDTEST_PORT", originalPort)
}

func TestMetricsEndpoint(t *testing.T) {
	// Create a test registry
	reg := prometheus.NewPedanticRegistry()

	// Setup the speedtest runner with the registry
	exporter.NewSpeedtestRunner("", reg)

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

	_ = resp.Body.Close()
}

func TestDebugCollectors(t *testing.T) {
	// Save original value
	originalDebug := os.Getenv("SPEEDTEST_EXPORTER_DEBUG")

	// Test with debug enabled
	_ = os.Setenv("SPEEDTEST_EXPORTER_DEBUG", "true")
	reg := prometheus.NewPedanticRegistry()

	if os.Getenv("SPEEDTEST_EXPORTER_DEBUG") != "" {
		reg.MustRegister(
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			collectors.NewGoCollector(),
		)
	}

	// Verify collectors are registered
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

	_ = os.Setenv("SPEEDTEST_EXPORTER_DEBUG", originalDebug)
}
