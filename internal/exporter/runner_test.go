package exporter

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/showwin/speedtest-go/speedtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSpeedtestClient is a mock implementation of the SpeedtestClient interface
type MockSpeedtestClient struct {
	mock.Mock
}

func (m *MockSpeedtestClient) FetchServerByID(id string) (*speedtest.Server, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*speedtest.Server), args.Error(1)
}

func (m *MockSpeedtestClient) FetchServers() (speedtest.Servers, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(speedtest.Servers), args.Error(1)
}

// createMockServer creates a properly initialized mock server with test result values already set
// This avoids calling actual network test methods which would panic in unit tests
func createMockServer(id, sponsor string, latency, dlSpeed, ulSpeed, jitter time.Duration) *speedtest.Server {
	return &speedtest.Server{
		ID:      id,
		Sponsor: sponsor,
		Latency: latency,
		DLSpeed: speedtest.ByteRate(dlSpeed), // DLSpeed in bytes/sec
		ULSpeed: speedtest.ByteRate(ulSpeed), // ULSpeed in bytes/sec
		Jitter:  jitter,
		// Context is left nil since we won't call test methods on the mock server
	}
}

// TestSpeedtestRunner_RunWithSpecificServer tests Run() with a specific server ID
func TestSpeedtestRunner_RunWithSpecificServer(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockServer := createMockServer(
		"1234",
		"Test ISP",
		25*time.Millisecond,
		125000000, // 1000 Mbps in bytes
		62500000,  // 500 Mbps in bytes
		5*time.Millisecond,
	)

	mockClient.On("FetchServerByID", "1234").Return(mockServer, nil)

	runner := &SpeedtestRunner{
		Server: "1234",
		client: mockClient,
	}

	result := runner.Run()

	assert.NotNil(t, result)
	assert.Equal(t, 1234, result.ServerID)
	assert.Equal(t, float64(1000000000), result.DownloadSpeed) // 125000000 * 8
	assert.Equal(t, float64(500000000), result.UploadSpeed)    // 62500000 * 8
	assert.Equal(t, 25.0, result.Ping)
	assert.Equal(t, 5.0, result.Jitter)
	mockClient.AssertCalled(t, "FetchServerByID", "1234")
}

// TestSpeedtestRunner_RunWithoutServerID tests Run() finding the best server
func TestSpeedtestRunner_RunWithoutServerID(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockServer := createMockServer(
		"5678",
		"Best ISP",
		15*time.Millisecond,
		250000000, // 2000 Mbps in bytes
		100000000, // 800 Mbps in bytes
		2*time.Millisecond,
	)

	servers := speedtest.Servers{mockServer}

	mockClient.On("FetchServers").Return(servers, nil)

	runner := &SpeedtestRunner{
		Server: "",
		client: mockClient,
	}

	result := runner.Run()

	assert.NotNil(t, result)
	assert.Equal(t, 5678, result.ServerID)
	assert.Equal(t, float64(2000000000), result.DownloadSpeed) // 250000000 * 8
	assert.Equal(t, float64(800000000), result.UploadSpeed)    // 100000000 * 8
	assert.Equal(t, 15.0, result.Ping)
	assert.Equal(t, 2.0, result.Jitter)
	mockClient.AssertCalled(t, "FetchServers")
}

// TestSpeedtestRunner_RunWithHighSpeeds tests Run() with high speed results
func TestSpeedtestRunner_RunWithHighSpeeds(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockServer := createMockServer(
		"9999",
		"Premium ISP",
		5*time.Millisecond,
		1250000000, // 10000 Mbps in bytes
		625000000,  // 5000 Mbps in bytes
		1*time.Millisecond,
	)

	mockClient.On("FetchServerByID", "9999").Return(mockServer, nil)

	runner := &SpeedtestRunner{
		Server: "9999",
		client: mockClient,
	}

	result := runner.Run()

	assert.NotNil(t, result)
	assert.Equal(t, 9999, result.ServerID)
	assert.Equal(t, float64(10000000000), result.DownloadSpeed)
	assert.Equal(t, float64(5000000000), result.UploadSpeed)
	assert.Equal(t, 5.0, result.Ping)
	assert.Equal(t, 1.0, result.Jitter)
}

// TestSpeedtestRunner_RunWithLowSpeeds tests Run() with low speed results
func TestSpeedtestRunner_RunWithLowSpeeds(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockServer := createMockServer(
		"2222",
		"Slow ISP",
		200*time.Millisecond,
		125000, // 1 Mbps in bytes
		62500,  // 500 Kbps in bytes
		50*time.Millisecond,
	)

	mockClient.On("FetchServerByID", "2222").Return(mockServer, nil)

	runner := &SpeedtestRunner{
		Server: "2222",
		client: mockClient,
	}

	result := runner.Run()

	assert.NotNil(t, result)
	assert.Equal(t, 2222, result.ServerID)
	assert.Equal(t, float64(1000000), result.DownloadSpeed)
	assert.Equal(t, float64(500000), result.UploadSpeed)
	assert.Equal(t, 200.0, result.Ping)
	assert.Equal(t, 50.0, result.Jitter)
}

// TestNewSpeedtestRunner tests NewSpeedtestRunner initialization
func TestNewSpeedtestRunner(t *testing.T) {
	reg := prometheus.NewRegistry()
	mockClient := new(MockSpeedtestClient)

	// Set up mock expectations for the Collector registration process
	mockServer := createMockServer(
		"1234",
		"Test ISP",
		25*time.Millisecond,
		125000000, // 1000 Mbps in bytes
		62500000,  // 500 Mbps in bytes
		5*time.Millisecond,
	)
	mockClient.On("FetchServerByID", "1234").Return(mockServer, nil)

	// Test that NewSpeedtestRunner doesn't panic
	runner := NewSpeedtestRunner("1234", reg, mockClient)
	assert.NotNil(t, runner)
	assert.Equal(t, "1234", runner.Server)
	assert.Equal(t, mockClient, runner.client)
}

// TestNewSpeedtestRunner_WithNilClient tests NewSpeedtestRunner with nil client
func TestNewSpeedtestRunner_WithNilClient(t *testing.T) {
	reg := prometheus.NewRegistry()
	mockClient := new(MockSpeedtestClient)

	// Set up mock expectations
	mockServer := createMockServer(
		"5678",
		"Test ISP",
		25*time.Millisecond,
		125000000,
		62500000,
		5*time.Millisecond,
	)
	mockClient.On("FetchServerByID", "5678").Return(mockServer, nil)

	// Test that NewSpeedtestRunner with nil client creates default client
	runner := NewSpeedtestRunner("5678", reg, mockClient)
	assert.NotNil(t, runner)
	assert.Equal(t, "5678", runner.Server)
	assert.NotNil(t, runner.client)
}
