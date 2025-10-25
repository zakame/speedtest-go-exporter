package exporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRunner is a mock implementation of the Runner interface for testing
type TestRunner struct {
	MockResult *SpeedtestResult
	RunCalled  bool
}

// Run implements the Runner interface for MockRunner
func (m *TestRunner) Run() *SpeedtestResult {
	m.RunCalled = true
	return m.MockResult
}

func TestMockRunner(t *testing.T) {
	// Create a mock result
	mockResult := &SpeedtestResult{
		ServerID:      1234,
		DownloadSpeed: 100000000, // 100 Mbps
		UploadSpeed:   50000000,  // 50 Mbps
		Jitter:        5.5,       // 5.5 ms
		Ping:          20.0,      // 20 ms
	}

	// Create a mock runner
	mockRunner := &TestRunner{
		MockResult: mockResult,
	}

	// Test the Run method
	result := mockRunner.Run()

	// Verify the result
	assert.True(t, mockRunner.RunCalled)
	assert.Equal(t, mockResult.ServerID, result.ServerID)
	assert.Equal(t, mockResult.DownloadSpeed, result.DownloadSpeed)
	assert.Equal(t, mockResult.UploadSpeed, result.UploadSpeed)
	assert.Equal(t, mockResult.Jitter, result.Jitter)
	assert.Equal(t, mockResult.Ping, result.Ping)
}

func TestSpeedtestResult(t *testing.T) {
	// Create a result with specific values
	result := &SpeedtestResult{
		ServerID:      1234,
		DownloadSpeed: 100000000, // 100 Mbps
		UploadSpeed:   50000000,  // 50 Mbps
		Jitter:        5.5,       // 5.5 ms
		Ping:          20.0,      // 20 ms
	}

	// Verify field values
	assert.Equal(t, 1234, result.ServerID)
	assert.Equal(t, 100000000.0, result.DownloadSpeed)
	assert.Equal(t, 50000000.0, result.UploadSpeed)
	assert.Equal(t, 5.5, result.Jitter)
	assert.Equal(t, 20.0, result.Ping)
}

// func TestNewSpeedtestRunner(t *testing.T) {
// 	// Create a registry
// 	reg := prometheus.NewRegistry()

// 	// Create a new runner with specific server ID
// 	runner := NewSpeedtestRunner("1234", reg)

// 	// Verify the runner
// 	assert.NotNil(t, runner)
// 	assert.Equal(t, "1234", runner.Server)
// 	assert.NotNil(t, runner.client)

// 	// Create a new runner with empty server ID
// 	runner = NewSpeedtestRunner("", reg)

// 	// Verify the runner with empty server ID
// 	assert.NotNil(t, runner)
// 	assert.Equal(t, "", runner.Server)
// 	assert.NotNil(t, runner.client)
// }

// TestMultipleResults tests the behavior with multiple different results
func TestMultipleResults(t *testing.T) {
	// Test cases with different results
	testCases := []struct {
		name   string
		result *SpeedtestResult
	}{
		{
			name: "High speeds",
			result: &SpeedtestResult{
				ServerID:      1,
				DownloadSpeed: 1000000000, // 1 Gbps
				UploadSpeed:   800000000,  // 800 Mbps
				Jitter:        2.0,
				Ping:          5.0,
			},
		},
		{
			name: "Low speeds",
			result: &SpeedtestResult{
				ServerID:      2,
				DownloadSpeed: 1000000, // 1 Mbps
				UploadSpeed:   500000,  // 500 Kbps
				Jitter:        50.0,
				Ping:          200.0,
			},
		},
		{
			name: "Zero values",
			result: &SpeedtestResult{
				ServerID:      3,
				DownloadSpeed: 0,
				UploadSpeed:   0,
				Jitter:        0,
				Ping:          0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRunner := &TestRunner{
				MockResult: tc.result,
			}

			result := mockRunner.Run()

			assert.Equal(t, tc.result.ServerID, result.ServerID)
			assert.Equal(t, tc.result.DownloadSpeed, result.DownloadSpeed)
			assert.Equal(t, tc.result.UploadSpeed, result.UploadSpeed)
			assert.Equal(t, tc.result.Jitter, result.Jitter)
			assert.Equal(t, tc.result.Ping, result.Ping)
		})
	}
}
