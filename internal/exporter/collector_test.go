package exporter

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockRunner implements the Runner interface for testing
type MockRunner struct {
	result *SpeedtestResult
}

func (m MockRunner) Run() *SpeedtestResult {
	return m.result
}

func TestSpeedtestCollector_Describe(t *testing.T) {
	// Create a collector with a mock runner
	collector := SpeedtestCollector{
		runner: MockRunner{
			result: &SpeedtestResult{
				ServerID:      12345,
				Jitter:        5.2,
				Ping:          25.7,
				DownloadSpeed: 100000000, // 100 Mbps
				UploadSpeed:   50000000,  // 50 Mbps
			},
		},
	}

	// Create a channel to receive descriptions
	ch := make(chan *prometheus.Desc, 10)

	// Call Describe
	collector.Describe(ch)

	// Verify that the channel has received descriptions
	assert.NotEmpty(t, ch, "Expected descriptions to be sent to the channel")
}

func TestSpeedtestCollector_Collect(t *testing.T) {
	// Create test data
	mockResult := &SpeedtestResult{
		ServerID:      12345,
		Jitter:        5.2,
		Ping:          25.7,
		DownloadSpeed: 100000000, // 100 Mbps
		UploadSpeed:   50000000,  // 50 Mbps
	}

	// Create collector with mock runner
	collector := SpeedtestCollector{
		runner: MockRunner{result: mockResult},
	}

	// Create a channel to receive metrics
	ch := make(chan prometheus.Metric, 10)

	// Call Collect
	collector.Collect(ch)

	// Close the channel to iterate over it
	close(ch)

	// Count metrics to ensure all were created
	metricCount := 0
	for range ch {
		metricCount++
	}

	// We expect 6 metrics: server, jitter, ping, download, upload, and up
	require.Equal(t, 6, metricCount, "Expected 6 metrics to be collected")
}

func TestSpeedtestCollector_MetricValues(t *testing.T) {
	// Create test data
	mockResult := &SpeedtestResult{
		ServerID:      12345,
		Jitter:        5.2,
		Ping:          25.7,
		DownloadSpeed: 100000000, // 100 Mbps
		UploadSpeed:   50000000,  // 50 Mbps
	}

	// Create collector with mock runner
	collector := SpeedtestCollector{
		runner: MockRunner{result: mockResult},
	}

	// Create a registry and register our collector
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(collector)

	// Expected metrics in Prometheus exposition format
	expected := `
# HELP speedtest_download_bits_per_second Speedtest download speed in bits per second.
# TYPE speedtest_download_bits_per_second gauge
speedtest_download_bits_per_second 1e+08
# HELP speedtest_jitter_latency_milliseconds Speedtest jitter latency in milliseconds.
# TYPE speedtest_jitter_latency_milliseconds gauge
speedtest_jitter_latency_milliseconds 5.2
# HELP speedtest_ping_latency_milliseconds Speedtest ping latency in milliseconds.
# TYPE speedtest_ping_latency_milliseconds gauge
speedtest_ping_latency_milliseconds 25.7
# HELP speedtest_server_id Speedtest server ID.
# TYPE speedtest_server_id gauge
speedtest_server_id 12345
# HELP speedtest_up Speedtest up status.
# TYPE speedtest_up gauge
speedtest_up 1
# HELP speedtest_upload_bits_per_second Speedtest upload speed in bits per second.
# TYPE speedtest_upload_bits_per_second gauge
speedtest_upload_bits_per_second 5e+07
`

	err := testutil.GatherAndCompare(registry, strings.NewReader(expected))
	require.NoError(t, err)
}
