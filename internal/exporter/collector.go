// Package exporter provides a speedtest-go runner and a Prometheus collector
// for speedtest-go metrics.
package exporter

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// DefaultCollectTimeout is the default timeout for a single speedtest collection.
const DefaultCollectTimeout = 90 * time.Second

// SpeedtestCollector implements [prometheus.Collector].
type SpeedtestCollector struct {
	runner  Runner
	timeout time.Duration
}

// Descriptors used by SpeedtestCollector to expose metrics.
var (
	server = prometheus.NewDesc(
		"speedtest_server_id",
		"Speedtest server ID.",
		nil, nil,
	)
	jitter = prometheus.NewDesc(
		"speedtest_jitter_latency_milliseconds",
		"Speedtest jitter latency in milliseconds.",
		nil, nil,
	)
	ping = prometheus.NewDesc(
		"speedtest_ping_latency_milliseconds",
		"Speedtest ping latency in milliseconds.",
		nil, nil,
	)
	downloadSpeed = prometheus.NewDesc(
		"speedtest_download_bits_per_second",
		"Speedtest download speed in bits per second.",
		nil, nil,
	)
	uploadSpeed = prometheus.NewDesc(
		"speedtest_upload_bits_per_second",
		"Speedtest upload speed in bits per second.",
		nil, nil,
	)
	up = prometheus.NewDesc(
		"speedtest_up",
		"Speedtest up status.",
		nil, nil,
	)
)

// Describe sends all speedtest-go metric descriptors to the provided channel.
func (se SpeedtestCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- server
	ch <- jitter
	ch <- ping
	ch <- downloadSpeed
	ch <- uploadSpeed
	ch <- up
}

// Collect runs the speedtest and creates metrics to send to the provided
// channel. On failure, all metrics are set to 0 and speedtest_up is set to 0.
func (se SpeedtestCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), se.timeout)
	defer cancel()

	s, err := se.runner.Run(ctx)
	if err != nil {
		log.WithError(err).Error("Speedtest failed")
		ch <- prometheus.MustNewConstMetric(server, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(jitter, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(ping, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(downloadSpeed, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(uploadSpeed, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 0)
		return
	}

	ch <- prometheus.MustNewConstMetric(server, prometheus.GaugeValue, float64(s.ServerID))
	ch <- prometheus.MustNewConstMetric(jitter, prometheus.GaugeValue, s.Jitter)
	ch <- prometheus.MustNewConstMetric(ping, prometheus.GaugeValue, s.Ping)
	ch <- prometheus.MustNewConstMetric(downloadSpeed, prometheus.GaugeValue, s.DownloadSpeed)
	ch <- prometheus.MustNewConstMetric(uploadSpeed, prometheus.GaugeValue, s.UploadSpeed)
	ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 1.0)
}

// RegisterSpeedtestCollector registers the given Runner as a Prometheus
// collector using the provided Registerer. This allows tests to register
// mock runners without creating a real SpeedtestRunner.
func RegisterSpeedtestCollector(r Runner, reg prometheus.Registerer, timeout time.Duration) {
	se := SpeedtestCollector{runner: r, timeout: timeout}
	reg.MustRegister(se)
}
