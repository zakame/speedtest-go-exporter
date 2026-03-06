package main

import (
	"fmt"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zakame/speedtest-go-exporter/internal/exporter"
)

// helpers separated from main() to improve testability.

func init() {
	log.SetOutput(os.Stdout)
}

// getPort returns the port the server should listen on, defaulting to 9798.
func getPort() string {
	port := os.Getenv("SPEEDTEST_PORT")
	if port == "" {
		return "9798"
	}
	return port
}

// getServerID returns the configured Speedtest server ID (may be empty).
func getServerID() string {
	return os.Getenv("SPEEDTEST_SERVER")
}

// debugEnabled checks whether DEBUG env variable is set.
func debugEnabled() bool {
	return os.Getenv("SPEEDTEST_EXPORTER_DEBUG") != ""
}

// newRegistry builds a Prometheus registry, registers a speedtest runner,
// and optionally adds the debug collectors.
func newRegistry(serverID string, debug bool) *prometheus.Registry {
	reg := prometheus.NewPedanticRegistry()
	exporter.NewSpeedtestRunner(serverID, reg, nil)
	if debug {
		reg.MustRegister(
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			collectors.NewGoCollector(),
		)
	}
	return reg
}

// newMux creates an HTTP mux wired to serve the basic root and /metrics routes
// using the provided registry. It is a variable so tests can override it.
var newMux = func(reg prometheus.Gatherer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("content-type", "text/html")
		_, _ = fmt.Fprintf(w, "See the <a href='/metrics'>metrics</a>.") // nolint:errcheck
	})
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	return mux
}

// run spins up the HTTP server. It returns the error returned from
// ListenAndServe, so callers (including tests) can handle it.
// listenAndServe is a variable so it can be swapped in tests.
var listenAndServe = http.ListenAndServe

func run(port, serverID string, debug bool) error {
	log.WithFields(log.Fields{
		"port":     port,
		"serverID": serverID,
		"debug":    debug,
	}).Info("Starting speedtest-go-exporter")

	reg := newRegistry(serverID, debug)
	mux := newMux(reg)
	return listenAndServe(":"+port, mux)
}

func main() {
	port := getPort()
	serverID := getServerID()
	dbg := debugEnabled()

	if err := run(port, serverID, dbg); err != nil {
		log.Fatal(err)
	}
}
