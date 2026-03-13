package exporter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	log "github.com/sirupsen/logrus"
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

// logCaptureHook records logrus log entries during a test. It is safe for
// concurrent use (Prometheus may call Collect from multiple goroutines).
type logCaptureHook struct {
	mu      sync.Mutex
	entries []*log.Entry
}

func (h *logCaptureHook) Levels() []log.Level { return log.AllLevels }

func (h *logCaptureHook) Fire(e *log.Entry) error {
	h.mu.Lock()
	h.entries = append(h.entries, e)
	h.mu.Unlock()
	return nil
}

func (h *logCaptureHook) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	msgs := make([]string, len(h.entries))
	for i, e := range h.entries {
		msgs[i] = e.Message
	}
	return msgs
}

// contextCapturingRunner records the context it was called with so tests can
// assert that Collect creates and forwards a deadline-bearing context.
type contextCapturingRunner struct {
	capturedCtx context.Context
	result      *SpeedtestResult
	err         error
}

func (r *contextCapturingRunner) Run(ctx context.Context) (*SpeedtestResult, error) {
	r.capturedCtx = ctx
	return r.result, r.err
}

// installLogHook registers hook on the global logrus logger and schedules its
// removal via t.Cleanup, so the hook does not bleed into other tests.
func installLogHook(t *testing.T, hook log.Hook) {
	t.Helper()
	log.AddHook(hook)
	t.Cleanup(func() { log.StandardLogger().ReplaceHooks(make(log.LevelHooks)) })
}

// TestSpeedtestCollector_DeadlineExceededLogsTimedOut verifies that when the
// runner returns context.DeadlineExceeded, Collect logs "Speedtest timed out"
// rather than the generic "Speedtest failed" message.  This exercises the
// errors.Is branch added in the error-handling implementation.
func TestSpeedtestCollector_DeadlineExceededLogsTimedOut(t *testing.T) {
	hook := &logCaptureHook{}
	installLogHook(t, hook)

	collector := SpeedtestCollector{
		runner:  MockRunner{result: nil, err: context.DeadlineExceeded},
		timeout: 5 * time.Second,
	}

	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	msgs := hook.messages()
	assert.Contains(t, msgs, "Speedtest timed out",
		"context.DeadlineExceeded must produce 'Speedtest timed out' log")
	assert.NotContains(t, msgs, "Speedtest failed",
		"context.DeadlineExceeded must not produce the generic 'Speedtest failed' log")
}

// TestSpeedtestCollector_NonDeadlineErrorLogsFailed verifies that non-timeout
// runner errors emit the generic "Speedtest failed" log, not "Speedtest timed out".
func TestSpeedtestCollector_NonDeadlineErrorLogsFailed(t *testing.T) {
	hook := &logCaptureHook{}
	installLogHook(t, hook)

	collector := SpeedtestCollector{
		runner:  MockRunner{result: nil, err: errors.New("network unreachable")},
		timeout: 5 * time.Second,
	}

	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	msgs := hook.messages()
	assert.Contains(t, msgs, "Speedtest failed",
		"non-deadline error must produce 'Speedtest failed' log")
	assert.NotContains(t, msgs, "Speedtest timed out",
		"non-deadline error must not produce 'Speedtest timed out' log")
}

// TestSpeedtestCollector_BothResultAndErrorTakesErrorPath verifies that when
// the runner returns (non-nil result, non-nil error), the error path is taken:
// speedtest_up=0 and all metrics are zeroed.  Without this test the error-path
// branch is only exercised with a nil result, which is the usual case.
func TestSpeedtestCollector_BothResultAndErrorTakesErrorPath(t *testing.T) {
	partialResult := &SpeedtestResult{
		ServerID:      42,
		DownloadSpeed: 1e9,
		UploadSpeed:   5e8,
		Ping:          10.0,
		Jitter:        2.0,
	}

	collector := SpeedtestCollector{
		runner:  MockRunner{result: partialResult, err: errors.New("partial failure")},
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

	assert.Equal(t, float64(0), values["speedtest_up"],
		"error path must be taken when runner returns (result, error); speedtest_up must be 0")
	assert.Equal(t, float64(0), values["speedtest_server_id"],
		"server_id must be 0 when runner returns (result, error)")
	assert.Equal(t, float64(0), values["speedtest_download_bits_per_second"],
		"download speed must be 0 when runner returns (result, error)")
	assert.Len(t, gathered, 6, "all 6 metric families must be present even with (result, error)")
}

// TestSpeedtestCollector_NilResultNilErrorDoesNotPanic documents the desired
// behaviour when a Runner violates its contract by returning (nil, nil):
// Collect must not panic and must emit speedtest_up=0 with zeroed metrics.
//
// NOTE: Collect is called directly (not via registry.Gather) so that
// assert.NotPanics can recover the panic from the same goroutine.
func TestSpeedtestCollector_NilResultNilErrorDoesNotPanic(t *testing.T) {
	hook := &logCaptureHook{}
	installLogHook(t, hook)

	collector := SpeedtestCollector{
		runner:  MockRunner{result: nil, err: nil},
		timeout: 5 * time.Second,
	}

	ch := make(chan prometheus.Metric, 10)

	assert.NotPanics(t, func() { collector.Collect(ch) },
		"Collect must not panic when runner returns (nil, nil)")
	close(ch)

	assert.Contains(t, hook.messages(), "Speedtest returned no result",
		"(nil, nil) runner result must log 'Speedtest returned no result'")

	// Collect must emit 6 zero metrics with up=0.
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 6, count,
		"Collect must emit all 6 zero metrics for (nil, nil) runner result")
}

// TestSpeedtestCollector_ContextHasDeadline verifies that the context Collect
// creates and passes to the runner carries a deadline.  If this is missing, the
// timeout guard in Collect is effectively disabled.
func TestSpeedtestCollector_ContextHasDeadline(t *testing.T) {
	runner := &contextCapturingRunner{
		result: &SpeedtestResult{ServerID: 1},
	}

	collector := SpeedtestCollector{
		runner:  runner,
		timeout: 30 * time.Second,
	}

	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	require.NotNil(t, runner.capturedCtx,
		"Collect must call runner.Run with a non-nil context")
	deadline, ok := runner.capturedCtx.Deadline()
	assert.True(t, ok,
		"context passed to runner must have a deadline (timeout guard must be active)")
	assert.True(t, deadline.After(time.Now()),
		"deadline must be in the future when the timeout has not yet elapsed")
}

// TestSpeedtestCollector_WrappedDeadlineExceededLogsTimedOut verifies that a
// wrapped context.DeadlineExceeded (as produced by the runner's fmt.Errorf %w
// chain) is still identified correctly via errors.Is and logs "Speedtest timed out".
func TestSpeedtestCollector_WrappedDeadlineExceededLogsTimedOut(t *testing.T) {
	hook := &logCaptureHook{}
	installLogHook(t, hook)

	wrappedDeadline := fmt.Errorf("ping test: %w", context.DeadlineExceeded)

	collector := SpeedtestCollector{
		runner:  MockRunner{result: nil, err: wrappedDeadline},
		timeout: 5 * time.Second,
	}

	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	assert.Contains(t, hook.messages(), "Speedtest timed out",
		"wrapped context.DeadlineExceeded must still produce 'Speedtest timed out' via errors.Is")
}

// fakeRegisterer implements prometheus.Registerer and records registered
// collectors for assertions in tests.
type fakeRegisterer struct {
	collectors []prometheus.Collector
}

func (f *fakeRegisterer) Register(c prometheus.Collector) error {
	f.collectors = append(f.collectors, c)
	return nil
}

func (f *fakeRegisterer) MustRegister(cs ...prometheus.Collector) {
	for _, c := range cs {
		_ = f.Register(c)
	}
}

func (f *fakeRegisterer) Unregister(c prometheus.Collector) bool {
	for i, col := range f.collectors {
		if col == c {
			f.collectors = append(f.collectors[:i], f.collectors[i+1:]...)
			return true
		}
	}
	return false
}

func TestNewSpeedtestRunnerRegistersCollector(t *testing.T) {
	fr := &fakeRegisterer{}

	// Create a new runner which should register a SpeedtestCollector via
	// RegisterSpeedtestCollector.
	r := NewSpeedtestRunner("42", fr, nil)

	assert.NotNil(t, r)
	assert.Equal(t, "42", r.Server)
	assert.NotNil(t, r.client)

	// Ensure one collector was registered
	assert.Len(t, fr.collectors, 1)

	// Assert the registered collector is a SpeedtestCollector whose runner is r
	// and that the timeout was wired to DefaultCollectTimeout.
	sc, ok := fr.collectors[0].(SpeedtestCollector)
	assert.True(t, ok, "registered collector should be SpeedtestCollector")
	assert.Equal(t, r, sc.runner)
	assert.Equal(t, DefaultCollectTimeout, sc.timeout,
		"registered collector must use DefaultCollectTimeout")
}
