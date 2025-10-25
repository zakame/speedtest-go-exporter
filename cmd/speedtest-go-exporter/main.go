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

func init() {
	log.SetOutput(os.Stdout)
}

func main() {
	port := os.Getenv("SPEEDTEST_PORT")
	if port == "" {
		port = "9798"
	}
	log.WithFields(log.Fields{
		"port": port,
	}).Info("Starting speedtest-go-exporter")

	reg := prometheus.NewPedanticRegistry()

	server_id := os.Getenv("SPEEDTEST_SERVER")
	exporter.NewSpeedtestRunner(server_id, reg)

	if os.Getenv("SPEEDTEST_EXPORTER_DEBUG") != "" {
		reg.MustRegister(
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			collectors.NewGoCollector(),
		)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("content-type", "text/html")
		_, _ = fmt.Fprintf(w, "See the <a href='/metrics'>metrics</a>.")
	})
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
