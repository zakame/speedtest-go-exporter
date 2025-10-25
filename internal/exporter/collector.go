// Package exporter provides a speedtest-go runner and a Prometheus collector
// for speedtest-go metrics.
package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SpeedtestCollector implements [prometheus.Collector].
type SpeedtestCollector struct {
	runner Runner
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
	download_speed = prometheus.NewDesc(
		"speedtest_download_bits_per_second",
		"Speedtest download speed in bits per second.",
		nil, nil,
	)
	upload_speed = prometheus.NewDesc(
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

// Describe sends speedtest-go metrics to the provided channel.
// See also [prometheus.DescribeByCollect].
func (se SpeedtestCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(se, ch)
}

// Collect runs the speedtest and creates metrics to send to the provided
// channel.
func (se SpeedtestCollector) Collect(ch chan<- prometheus.Metric) {
	s := se.runner.Run()

	ch <- prometheus.MustNewConstMetric(
		server,
		prometheus.GaugeValue,
		float64(s.ServerID),
	)
	ch <- prometheus.MustNewConstMetric(
		jitter,
		prometheus.GaugeValue,
		s.Jitter,
	)
	ch <- prometheus.MustNewConstMetric(
		ping,
		prometheus.GaugeValue,
		s.Ping,
	)
	ch <- prometheus.MustNewConstMetric(
		download_speed,
		prometheus.GaugeValue,
		s.DownloadSpeed,
	)
	ch <- prometheus.MustNewConstMetric(
		upload_speed,
		prometheus.GaugeValue,
		s.UploadSpeed,
	)
	ch <- prometheus.MustNewConstMetric(
		up,
		prometheus.GaugeValue,
		1.0, // Indicating the speedtest was successful
	)
}
