package exporter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockRunner implements the Runner interface for testing
type MockRunner struct {
	result *SpeedtestResult
	err    error
}

func (m MockRunner) Run(_ context.Context) (*SpeedtestResult, error) {
	return m.result, m.err
}

// blockingRunner is a Runner that blocks until its context is cancelled,
// simulating a hung speedtest for timeout-path tests.
type blockingRunner struct{}

func (b blockingRunner) Run(ctx context.Context) (*SpeedtestResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
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
		timeout: 5 * time.Second,
	}

	// Create a channel to receive descriptions
	ch := make(chan *prometheus.Desc, 10)

	// Call Describe
	collector.Describe(ch)

	// Verify that the channel has received descriptions
	assert.NotEmpty(t, ch, "Expected descriptions to be sent to the channel")
}

// TestSpeedtestCollector_Describe_AllSixDescriptors verifies that Describe
// sends exactly 6 descriptors and that each expected metric name is present.
// The weak predecessor test only checked the channel was non-empty; this test
// would catch any accidentally dropped descriptor.
func TestSpeedtestCollector_Describe_AllSixDescriptors(t *testing.T) {
	collector := SpeedtestCollector{
		runner:  MockRunner{result: &SpeedtestResult{}},
		timeout: 5 * time.Second,
	}

	ch := make(chan *prometheus.Desc, 10)
	collector.Describe(ch)
	close(ch)

	var names []string
	for desc := range ch {
		names = append(names, desc.String())
	}

	require.Len(t, names, 6, "Describe must send exactly 6 descriptors")

	wantSubstrings := []string{
		"speedtest_server_id",
		"speedtest_jitter_latency_milliseconds",
		"speedtest_ping_latency_milliseconds",
		"speedtest_download_bits_per_second",
		"speedtest_upload_bits_per_second",
		"speedtest_up",
	}
	for _, want := range wantSubstrings {
		found := false
		for _, name := range names {
			if strings.Contains(name, want) {
				found = true
				break
			}
		}
		assert.True(t, found, "descriptor for %q must be present in Describe output", want)
	}
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
		runner:  MockRunner{result: mockResult},
		timeout: 5 * time.Second,
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
		runner:  MockRunner{result: mockResult},
		timeout: 5 * time.Second,
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

// TestSpeedtestCollector_CollectOnRunnerError verifies that when the Runner
// returns an error, Collect emits all-zeros for every metric and sets
// speedtest_up to 0.  The pedantic registry is used so any descriptor
// mismatch would also surface as an error.
func TestSpeedtestCollector_CollectOnRunnerError(t *testing.T) {
	collector := SpeedtestCollector{
		runner:  MockRunner{result: nil, err: errors.New("test failure")},
		timeout: 5 * time.Second,
	}

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(collector)

	gathered, err := registry.Gather()
	require.NoError(t, err, "Gather must not return an error even when the runner fails")

	// Build a map of metric-family name -> value for easy assertion.
	values := make(map[string]float64, len(gathered))
	for _, mf := range gathered {
		if len(mf.GetMetric()) > 0 {
			values[mf.GetName()] = mf.GetMetric()[0].GetGauge().GetValue()
		}
	}

	assert.Equal(t, float64(0), values["speedtest_up"], "speedtest_up must be 0 on error")
	assert.Equal(t, float64(0), values["speedtest_server_id"], "speedtest_server_id must be 0 on error")
	assert.Equal(t, float64(0), values["speedtest_download_bits_per_second"], "download speed must be 0 on error")
	assert.Equal(t, float64(0), values["speedtest_upload_bits_per_second"], "upload speed must be 0 on error")
	assert.Equal(t, float64(0), values["speedtest_ping_latency_milliseconds"], "ping must be 0 on error")
	assert.Equal(t, float64(0), values["speedtest_jitter_latency_milliseconds"], "jitter must be 0 on error")

	// Confirm all 6 metric families are present (no silently dropped metrics).
	assert.Len(t, gathered, 6, "all 6 metric families must be emitted even on runner error")
}

// TestSpeedtestCollector_TimeoutProducesZeroMetrics verifies that when the
// Runner blocks past the collector timeout, Collect still emits all 6 metrics
// with zero values and speedtest_up=0.  This validates that the 90-second
// DefaultCollectTimeout guard actually cancels and falls back correctly.
func TestSpeedtestCollector_TimeoutProducesZeroMetrics(t *testing.T) {
	// Use a very short timeout so the test completes quickly.
	collector := SpeedtestCollector{
		runner:  blockingRunner{},
		timeout: 50 * time.Millisecond,
	}

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(collector)

	gathered, err := registry.Gather()
	require.NoError(t, err, "Gather must not return an error even when the runner times out")

	values := make(map[string]float64, len(gathered))
	for _, mf := range gathered {
		if len(mf.GetMetric()) > 0 {
			values[mf.GetName()] = mf.GetMetric()[0].GetGauge().GetValue()
		}
	}

	assert.Len(t, gathered, 6, "all 6 metric families must be emitted on timeout")
	assert.Equal(t, float64(0), values["speedtest_up"], "speedtest_up must be 0 on timeout")
	assert.Equal(t, float64(0), values["speedtest_server_id"], "server_id must be 0 on timeout")
	assert.Equal(t, float64(0), values["speedtest_download_bits_per_second"], "download must be 0 on timeout")
	assert.Equal(t, float64(0), values["speedtest_upload_bits_per_second"], "upload must be 0 on timeout")
	assert.Equal(t, float64(0), values["speedtest_ping_latency_milliseconds"], "ping must be 0 on timeout")
	assert.Equal(t, float64(0), values["speedtest_jitter_latency_milliseconds"], "jitter must be 0 on timeout")
}

// TestSpeedtestCollector_ZeroValueSuccessResult verifies that a legitimately
// all-zero result (e.g. a server that reports 0 speed, 0 ping, 0 jitter) is
// still treated as success: speedtest_up must be 1, not 0.  Without this test,
// the zero-success path and the error path are indistinguishable from a
// coverage standpoint.
func TestSpeedtestCollector_ZeroValueSuccessResult(t *testing.T) {
	zeroResult := &SpeedtestResult{
		ServerID:      0,
		DownloadSpeed: 0,
		UploadSpeed:   0,
		Ping:          0,
		Jitter:        0,
	}

	collector := SpeedtestCollector{
		runner:  MockRunner{result: zeroResult},
		timeout: 5 * time.Second,
	}

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(collector)

	gathered, err := registry.Gather()
	require.NoError(t, err)

	values := make(map[string]float64, len(gathered))
	for _, mf := range gathered {
		if len(mf.GetMetric()) > 0 {
			values[mf.GetName()] = mf.GetMetric()[0].GetGauge().GetValue()
		}
	}

	// The critical assertion: success with zero-value data must set up=1.
	assert.Equal(t, float64(1), values["speedtest_up"],
		"speedtest_up must be 1 even when all other result fields are 0 (success is success)")
	assert.Len(t, gathered, 6, "all 6 metric families must be present for zero-value success")
}
