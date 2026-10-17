package exporter

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/showwin/speedtest-go/speedtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// speedtest-go reports an unmeasurable direction as this ByteRate ("N/A").
const naByteRate = -1

// gatherGauges registers r on a pedantic registry and returns the gathered
// gauge values keyed by metric name. Absent series are absent from the map.
func gatherGauges(t *testing.T, r Runner) map[string]float64 {
	t.Helper()

	registry := prometheus.NewPedanticRegistry()
	RegisterSpeedtestCollector(r, registry, 5*time.Second)

	families, err := registry.Gather()
	require.NoError(t, err)

	values := make(map[string]float64, len(families))
	for _, mf := range families {
		require.Equal(t, dto.MetricType_GAUGE, mf.GetType(), mf.GetName())
		require.Len(t, mf.GetMetric(), 1, mf.GetName())
		values[mf.GetName()] = mf.GetMetric()[0].GetGauge().GetValue()
	}
	return values
}

// runnerFor returns a SpeedtestRunner whose mock client serves srv.
func runnerFor(srv *speedtest.Server) *SpeedtestRunner {
	mockClient := new(MockSpeedtestClient)
	mockClient.On("FetchServerByIDContext", mock.Anything, srv.ID).Return(srv, nil)
	return &SpeedtestRunner{Server: srv.ID, client: mockClient}
}

func TestSpeedtestRunner_UploadUnavailable(t *testing.T) {
	srv := createMockServer("1234", "Test ISP",
		25*time.Millisecond,
		125000000,
		naByteRate,
		5*time.Millisecond,
	)

	result, err := runnerFor(srv).Run(context.Background())

	require.NoError(t, err)
	assert.True(t, result.UploadUnavailable)
	assert.False(t, result.DownloadUnavailable)
	assert.Equal(t, 1234, result.ServerID)
	assert.Equal(t, float64(1000000000), result.DownloadSpeed)
	assert.Equal(t, 25.0, result.Ping)
	assert.Equal(t, 5.0, result.Jitter)
}

func TestSpeedtestRunner_DownloadUnavailable(t *testing.T) {
	srv := createMockServer("1234", "Test ISP",
		25*time.Millisecond,
		naByteRate,
		62500000,
		5*time.Millisecond,
	)

	result, err := runnerFor(srv).Run(context.Background())

	require.NoError(t, err)
	assert.True(t, result.DownloadUnavailable)
	assert.False(t, result.UploadUnavailable)
	assert.Equal(t, 1234, result.ServerID)
	assert.Equal(t, float64(500000000), result.UploadSpeed)
	assert.Equal(t, 25.0, result.Ping)
	assert.Equal(t, 5.0, result.Jitter)
}

// A measured zero is a real observation, not N/A.
func TestSpeedtestRunner_ZeroSpeedIsAvailable(t *testing.T) {
	srv := createMockServer("1234", "Test ISP", 25*time.Millisecond, 0, 0, 5*time.Millisecond)

	result, err := runnerFor(srv).Run(context.Background())

	require.NoError(t, err)
	assert.False(t, result.DownloadUnavailable)
	assert.False(t, result.UploadUnavailable)
	assert.Equal(t, 0.0, result.DownloadSpeed)
	assert.Equal(t, 0.0, result.UploadSpeed)
}

// speedtest-go swallows ctx errors, so Run must surface them itself.
func TestSpeedtestRunner_DoneContextReturnsError(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancel2 := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel2()

	tests := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"cancelled", cancelled, context.Canceled},
		{"deadline exceeded", expired, context.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := createMockServer("1234", "Test ISP",
				25*time.Millisecond, 125000000, 62500000, 5*time.Millisecond)

			result, err := runnerFor(srv).Run(tt.ctx)

			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.want), "got %v", err)
			assert.Nil(t, result)
		})
	}
}

func TestSpeedtestCollector_UnavailableSpeedIsOmitted(t *testing.T) {
	tests := []struct {
		name    string
		dl, ul  speedtest.ByteRate
		absent  []string
		present []string
	}{
		{
			name:    "upload",
			dl:      125000000,
			ul:      naByteRate,
			absent:  []string{"speedtest_upload_bits_per_second"},
			present: []string{"speedtest_download_bits_per_second"},
		},
		{
			name:    "download",
			dl:      naByteRate,
			ul:      62500000,
			absent:  []string{"speedtest_download_bits_per_second"},
			present: []string{"speedtest_upload_bits_per_second"},
		},
		{
			name:   "both",
			dl:     naByteRate,
			ul:     naByteRate,
			absent: []string{"speedtest_download_bits_per_second", "speedtest_upload_bits_per_second"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := createMockServer("1234", "Test ISP",
				25*time.Millisecond, time.Duration(tt.dl), time.Duration(tt.ul), 5*time.Millisecond)

			values := gatherGauges(t, runnerFor(srv))

			for _, name := range tt.absent {
				assert.NotContains(t, values, name)
			}
			for _, name := range tt.present {
				assert.Contains(t, values, name)
			}
			assert.Equal(t, 1.0, values["speedtest_up"])
			assert.Equal(t, 1234.0, values["speedtest_server_id"])
			assert.Equal(t, 25.0, values["speedtest_ping_latency_milliseconds"])
			assert.Equal(t, 5.0, values["speedtest_jitter_latency_milliseconds"])
		})
	}
}

// The flag alone decides omission, regardless of the speed value carried.
func TestSpeedtestCollector_UnavailableFlagOmitsSeries(t *testing.T) {
	values := gatherGauges(t, MockRunner{result: &SpeedtestResult{
		ServerID:          1,
		DownloadSpeed:     8,
		UploadSpeed:       -8,
		UploadUnavailable: true,
	}})

	assert.NotContains(t, values, "speedtest_upload_bits_per_second")
	assert.Equal(t, 8.0, values["speedtest_download_bits_per_second"])
	assert.Equal(t, 1.0, values["speedtest_up"])
}

// A collector timeout must still go through the error path (up=0).
func TestSpeedtestCollector_ExpiredTimeoutReportsDown(t *testing.T) {
	srv := createMockServer("1234", "Test ISP",
		25*time.Millisecond, 125000000, 62500000, 5*time.Millisecond)
	registry := prometheus.NewPedanticRegistry()
	RegisterSpeedtestCollector(runnerFor(srv), registry, time.Nanosecond)

	families, err := registry.Gather()
	require.NoError(t, err)

	for _, mf := range families {
		if mf.GetName() == "speedtest_up" {
			assert.Equal(t, 0.0, mf.GetMetric()[0].GetGauge().GetValue())
			return
		}
	}
	t.Fatal("speedtest_up not gathered")
}

// Whatever speedtest-go hands back, no negative or NaN value may be exported.
func TestSpeedtestCollector_NeverExportsNegativeOrNaN(t *testing.T) {
	tests := []struct {
		name   string
		dl, ul speedtest.ByteRate
	}{
		{"zero", 0, 0},
		{"na upload", 125000000, naByteRate},
		{"na download", naByteRate, 62500000},
		{"na both", naByteRate, naByteRate},
		{"other negative", -2, -0.5},
		{"nan", speedtest.ByteRate(math.NaN()), speedtest.ByteRate(math.NaN())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &speedtest.Server{
				ID: "1234", Sponsor: "Test ISP",
				Latency: 25 * time.Millisecond, Jitter: 5 * time.Millisecond,
				DLSpeed: tt.dl, ULSpeed: tt.ul,
			}

			values := gatherGauges(t, runnerFor(srv))

			for name, v := range values {
				if math.IsNaN(v) {
					t.Errorf("%s is NaN", name)
					continue
				}
				assert.GreaterOrEqual(t, v, 0.0, name)
			}
		})
	}
}

func TestSpeedtestCollector_ZeroSpeedIsExported(t *testing.T) {
	srv := createMockServer("1234", "Test ISP", 25*time.Millisecond, 0, 0, 5*time.Millisecond)

	values := gatherGauges(t, runnerFor(srv))

	require.Contains(t, values, "speedtest_download_bits_per_second")
	require.Contains(t, values, "speedtest_upload_bits_per_second")
	assert.Equal(t, 0.0, values["speedtest_download_bits_per_second"])
	assert.Equal(t, 0.0, values["speedtest_upload_bits_per_second"])
	assert.Equal(t, 1.0, values["speedtest_up"])
}
