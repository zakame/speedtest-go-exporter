package exporter

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/showwin/speedtest-go/speedtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockSpeedtestClient is a mock implementation of the SpeedtestClient interface
type MockSpeedtestClient struct {
	mock.Mock
}

func (m *MockSpeedtestClient) FetchServerByIDContext(ctx context.Context, id string) (*speedtest.Server, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*speedtest.Server), args.Error(1)
}

func (m *MockSpeedtestClient) FetchServerListContext(ctx context.Context) (speedtest.Servers, error) {
	args := m.Called(ctx)
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

	mockClient.On("FetchServerByIDContext", mock.Anything, "1234").Return(mockServer, nil)

	runner := &SpeedtestRunner{
		Server: "1234",
		client: mockClient,
	}

	result, err := runner.Run(context.Background())

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1234, result.ServerID)
	assert.Equal(t, float64(1000000000), result.DownloadSpeed) // 125000000 * 8
	assert.Equal(t, float64(500000000), result.UploadSpeed)    // 62500000 * 8
	assert.Equal(t, 25.0, result.Ping)
	assert.Equal(t, 5.0, result.Jitter)
	mockClient.AssertCalled(t, "FetchServerByIDContext", mock.Anything, "1234")
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

	mockClient.On("FetchServerListContext", mock.Anything).Return(servers, nil)

	runner := &SpeedtestRunner{
		Server: "",
		client: mockClient,
	}

	result, err := runner.Run(context.Background())

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 5678, result.ServerID)
	assert.Equal(t, float64(2000000000), result.DownloadSpeed) // 250000000 * 8
	assert.Equal(t, float64(800000000), result.UploadSpeed)    // 100000000 * 8
	assert.Equal(t, 15.0, result.Ping)
	assert.Equal(t, 2.0, result.Jitter)
	mockClient.AssertCalled(t, "FetchServerListContext", mock.Anything)
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

	mockClient.On("FetchServerByIDContext", mock.Anything, "9999").Return(mockServer, nil)

	runner := &SpeedtestRunner{
		Server: "9999",
		client: mockClient,
	}

	result, err := runner.Run(context.Background())

	assert.NoError(t, err)
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

	mockClient.On("FetchServerByIDContext", mock.Anything, "2222").Return(mockServer, nil)

	runner := &SpeedtestRunner{
		Server: "2222",
		client: mockClient,
	}

	result, err := runner.Run(context.Background())

	assert.NoError(t, err)
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
	mockClient.On("FetchServerByIDContext", mock.Anything, "1234").Return(mockServer, nil)

	// Test that NewSpeedtestRunner doesn't panic
	runner := NewSpeedtestRunner("1234", reg, mockClient)
	assert.NotNil(t, runner)
	assert.Equal(t, "1234", runner.Server)
	assert.Equal(t, mockClient, runner.client)
}

// TestNewSpeedtestRunner_WithNilClient tests that NewSpeedtestRunner substitutes
// the default speedtest client when nil is passed.
func TestNewSpeedtestRunner_WithNilClient(t *testing.T) {
	reg := prometheus.NewRegistry()

	runner := NewSpeedtestRunner("5678", reg, nil)
	assert.NotNil(t, runner)
	assert.Equal(t, "5678", runner.Server)
	// client should have been set to the default speedtest client, not nil
	assert.NotNil(t, runner.client)
}

// TestSpeedtestRunner_FetchServerByIDError verifies that when FetchServerByID
// returns an error, Run returns (nil, non-nil error) and does not panic.
func TestSpeedtestRunner_FetchServerByIDError(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockClient.On("FetchServerByIDContext", mock.Anything, "1234").Return(nil, assert.AnError)

	runner := &SpeedtestRunner{
		Server: "1234",
		client: mockClient,
	}

	result, err := runner.Run(context.Background())

	assert.Nil(t, result)
	assert.Error(t, err)
	mockClient.AssertCalled(t, "FetchServerByIDContext", mock.Anything, "1234")
}

// TestSpeedtestRunner_FetchServersError verifies that when FetchServers returns
// an error (no explicit server ID configured), Run returns (nil, non-nil error).
func TestSpeedtestRunner_FetchServersError(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockClient.On("FetchServerListContext", mock.Anything).Return(nil, assert.AnError)

	runner := &SpeedtestRunner{
		Server: "",
		client: mockClient,
	}

	result, err := runner.Run(context.Background())

	assert.Nil(t, result)
	assert.Error(t, err)
	mockClient.AssertCalled(t, "FetchServerListContext", mock.Anything)
}

// TestSpeedtestRunner_NoServersFound verifies that when FetchServerListContext returns an
// empty list and FindServer returns no targets, Run returns an error containing
// "no speedtest servers found".
func TestSpeedtestRunner_NoServersFound(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	// Return an empty (non-nil) server list so the code proceeds to FindServer.
	mockClient.On("FetchServerListContext", mock.Anything).Return(speedtest.Servers{}, nil)

	runner := &SpeedtestRunner{
		Server: "",
		client: mockClient,
	}

	result, err := runner.Run(context.Background())

	assert.Nil(t, result)
	assert.Error(t, err)
	mockClient.AssertCalled(t, "FetchServerListContext", mock.Anything)
}

// TestSpeedtestRunner_NonIntegerServerID verifies that Run returns an error
// when the speedtest server reports a non-integer ID, since server IDs are
// always integers and a non-numeric value indicates an unexpected API response.
func TestSpeedtestRunner_NonIntegerServerID(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockServer := createMockServer(
		"not-an-int",
		"Weird ISP",
		10*time.Millisecond,
		125000000, // 1000 Mbps in bytes
		62500000,  // 500 Mbps in bytes
		3*time.Millisecond,
	)

	mockClient.On("FetchServerByIDContext", mock.Anything, "not-an-int").Return(mockServer, nil)

	runner := &SpeedtestRunner{
		Server: "not-an-int",
		client: mockClient,
	}

	result, err := runner.Run(context.Background())

	assert.Error(t, err, "non-integer server ID must return an error")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not-an-int")
}

// TestSpeedtestRunner_FetchServerByIDError_WrapsError verifies that the error
// message returned when FetchServerByID fails includes the server ID so
// operators can identify which server caused the problem.
func TestSpeedtestRunner_FetchServerByIDError_WrapsError(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockClient.On("FetchServerByIDContext", mock.Anything, "9001").Return(nil, assert.AnError)

	runner := &SpeedtestRunner{
		Server: "9001",
		client: mockClient,
	}

	_, err := runner.Run(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "9001", "error message must include the server ID")
}

// TestSpeedtestRunner_FetchServersError_WrapsError verifies that the error
// returned when FetchServers fails contains identifying context.
func TestSpeedtestRunner_FetchServersError_WrapsError(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockClient.On("FetchServerListContext", mock.Anything).Return(nil, assert.AnError)

	runner := &SpeedtestRunner{
		Server: "",
		client: mockClient,
	}

	_, err := runner.Run(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fetch servers", "error must mention fetch servers")
}

// TestSpeedtestRunner_NoServersFound_ErrorMessage verifies that the error
// returned when FindServer finds nothing contains "find server" context.
// Note: speedtest.Servers{}.FindServer returns its own library error before
// our len(targets)==0 guard is reached, so the wrapped message is "find server: ...".
func TestSpeedtestRunner_NoServersFound_ErrorMessage(t *testing.T) {
	mockClient := new(MockSpeedtestClient)
	mockClient.On("FetchServerListContext", mock.Anything).Return(speedtest.Servers{}, nil)

	runner := &SpeedtestRunner{
		Server: "",
		client: mockClient,
	}

	_, err := runner.Run(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "find server",
		"error must contain 'find server' context")
}

// networkAvailable returns true if a TCP connection to the speedtest API can
// be established within 3 seconds, indicating a live internet connection.
func networkAvailable() bool {
	conn, err := net.DialTimeout("tcp", "www.speedtest.net:443", 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// TestSpeedtestRunner_ContextCancellation exercises the context-cancellation
// path inside Run. The context-aware library methods (PingTestContext,
// DownloadTestContext, UploadTestContext) abort in-flight network I/O when the
// context is cancelled, so no background goroutine is left running.
// The test is skipped automatically when no network is detected so it is safe
// to run in CI without internet access.
func TestSpeedtestRunner_ContextCancellation(t *testing.T) {
	if !networkAvailable() {
		t.Skip("no network available; skipping live speedtest cancellation test")
	}

	client := speedtest.New()
	serverList, err := client.FetchServers()
	if err != nil {
		t.Skipf("cannot fetch servers: %v", err)
	}
	targets, err := serverList.FindServer([]int{})
	if err != nil || len(targets) == 0 {
		t.Skip("no speedtest servers found")
	}

	runner := &SpeedtestRunner{
		Server: targets[0].ID,
		client: client,
	}

	// Cancel before calling Run so the goroutine select fires immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := runner.Run(ctx)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}

// contextBlockingClient is a SpeedtestClient whose methods block until the
// supplied context is cancelled.  This lets tests verify that a cancelled
// context causes Run to return immediately with a context error.
type contextBlockingClient struct{}

func (contextBlockingClient) FetchServerByIDContext(ctx context.Context, _ string) (*speedtest.Server, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (contextBlockingClient) FetchServerListContext(ctx context.Context) (speedtest.Servers, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// contextCapturingClient records the context passed to it so tests can assert
// it is the same context forwarded from Run — not a fresh background context.
type contextCapturingClient struct {
	capturedCtx context.Context
	server      *speedtest.Server
}

func (c *contextCapturingClient) FetchServerByIDContext(ctx context.Context, _ string) (*speedtest.Server, error) {
	c.capturedCtx = ctx
	return c.server, nil
}

func (c *contextCapturingClient) FetchServerListContext(ctx context.Context) (speedtest.Servers, error) {
	c.capturedCtx = ctx
	if c.server == nil {
		return speedtest.Servers{}, nil
	}
	return speedtest.Servers{c.server}, nil
}

// TestSpeedtestRunner_ContextCancelledBeforeFetchServerByID verifies that
// cancelling the context before (or during) FetchServerByIDContext causes
// Run to return (nil, context.Canceled) with the "fetch server" wrapping.
func TestSpeedtestRunner_ContextCancelledBeforeFetchServerByID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so the blocking client fires immediately

	runner := &SpeedtestRunner{
		Server: "1234",
		client: contextBlockingClient{},
	}

	result, err := runner.Run(ctx)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled,
		"Run must propagate context.Canceled from FetchServerByIDContext")
	assert.Contains(t, err.Error(), "fetch server",
		"error must carry 'fetch server' context for operator diagnostics")
}

// TestSpeedtestRunner_ContextDeadlineBeforeFetchServerByID mirrors the above
// for context.DeadlineExceeded, which is the error produced by the collector's
// WithTimeout when the full speedtest takes too long.
func TestSpeedtestRunner_ContextDeadlineBeforeFetchServerByID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond) // let the deadline expire

	runner := &SpeedtestRunner{
		Server: "1234",
		client: contextBlockingClient{},
	}

	result, err := runner.Run(ctx)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.DeadlineExceeded,
		"Run must propagate context.DeadlineExceeded from FetchServerByIDContext")
	assert.Contains(t, err.Error(), "fetch server")
}

// TestSpeedtestRunner_ContextCancelledBeforeFetchServers tests the no-server-ID
// path where FetchServerListContext is called.  Cancelling before the call must
// surface as context.Canceled wrapped with "fetch servers".
func TestSpeedtestRunner_ContextCancelledBeforeFetchServers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := &SpeedtestRunner{
		Server: "",
		client: contextBlockingClient{},
	}

	result, err := runner.Run(ctx)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled,
		"Run must propagate context.Canceled from FetchServerListContext")
	assert.Contains(t, err.Error(), "fetch servers",
		"error must carry 'fetch servers' context for operator diagnostics")
}

// TestSpeedtestRunner_ContextDeadlineBeforeFetchServers mirrors the above for
// context.DeadlineExceeded on the server-list path.
func TestSpeedtestRunner_ContextDeadlineBeforeFetchServers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	runner := &SpeedtestRunner{
		Server: "",
		client: contextBlockingClient{},
	}

	result, err := runner.Run(ctx)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.DeadlineExceeded,
		"Run must propagate context.DeadlineExceeded from FetchServerListContext")
	assert.Contains(t, err.Error(), "fetch servers")
}

// TestSpeedtestRunner_ContextForwardedToFetchServerByID verifies that the
// context passed to Run is the exact same context forwarded to
// FetchServerByIDContext — not a detached background context.
func TestSpeedtestRunner_ContextForwardedToFetchServerByID(t *testing.T) {
	mockServer := createMockServer("42", "Forward ISP", 10*time.Millisecond, 100000, 50000, 2*time.Millisecond)
	client := &contextCapturingClient{server: mockServer}

	ctx := context.WithValue(context.Background(), struct{ key string }{"trace"}, "test-trace-id")

	runner := &SpeedtestRunner{Server: "42", client: client}
	_, err := runner.Run(ctx)

	require.NoError(t, err)
	assert.Equal(t, ctx, client.capturedCtx,
		"FetchServerByIDContext must receive the exact context passed to Run, not a new one")
}

// TestSpeedtestRunner_ContextForwardedToFetchServers verifies context identity
// on the server-list path.
func TestSpeedtestRunner_ContextForwardedToFetchServers(t *testing.T) {
	mockServer := createMockServer("99", "Forward ISP", 10*time.Millisecond, 100000, 50000, 2*time.Millisecond)
	client := &contextCapturingClient{server: mockServer}

	ctx := context.WithValue(context.Background(), struct{ key string }{"trace"}, "test-trace-id")

	runner := &SpeedtestRunner{Server: "", client: client}
	_, err := runner.Run(ctx)

	require.NoError(t, err)
	assert.Equal(t, ctx, client.capturedCtx,
		"FetchServerListContext must receive the exact context passed to Run, not a new one")
}
