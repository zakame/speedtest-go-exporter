package exporter

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/showwin/speedtest-go/speedtest"
	log "github.com/sirupsen/logrus"
)

// SpeedtestClient is an interface for the speedtest client behavior.
type SpeedtestClient interface {
	FetchServerByID(id string) (*speedtest.Server, error)
	FetchServers() (speedtest.Servers, error)
}

// SpeedtestResult holds the results of a speedtest run.
type SpeedtestResult struct {
	ServerID      int     `json:"server_id"`
	DownloadSpeed float64 `json:"download_speed"` // in bits per second
	UploadSpeed   float64 `json:"upload_speed"`   // in bits per second
	Jitter        float64 `json:"jitter"`         // in milliseconds
	Ping          float64 `json:"ping"`           // in milliseconds
}

// Runner is an interface for modelling a speedtest runner.
type Runner interface {
	// Run executes the speedtest and returns the selected server.
	Run() *SpeedtestResult
}

// SpeedtestRunner implements [Runner] using [speedtest.Speedtest].
type SpeedtestRunner struct {
	// Server is the ID of the speedtest server to use.
	Server string

	client SpeedtestClient
}

// Run executes the speedtest and returns the selected server.
func (r *SpeedtestRunner) Run() *SpeedtestResult {
	var s *speedtest.Server
	if r.Server != "" {
		s, _ = r.client.FetchServerByID(r.Server)
	} else {
		log.Warn("Finding the best server")
		serverList, _ := r.client.FetchServers()
		targets, _ := serverList.FindServer([]int{})
		s = targets[0]
	}
	slog := log.WithFields(log.Fields{
		"sponsor": s.Sponsor,
		"id":      s.ID,
	})
	slog.Info("Selected server")

	slog.Info("Running speedtest")
	// Only run tests if Context is set (indicates a real speedtest client, not a mock)
	if s.Context != nil {
		_ = s.PingTest(nil)
		_ = s.DownloadTest()
		_ = s.UploadTest()
	}
	slog.WithFields(log.Fields{
		"ping":           s.Latency,
		"download_speed": s.DLSpeed,
		"upload_speed":   s.ULSpeed,
		"jitter":         s.Jitter,
	}).Info("Speedtest completed")

	// Reset the context to free resources if it's set
	if s.Context != nil {
		s.Context.Reset()
	}

	id, _ := strconv.Atoi(s.ID)

	return &SpeedtestResult{
		ServerID:      id,
		DownloadSpeed: float64(s.DLSpeed * 8),                         // Convert to bits per second
		UploadSpeed:   float64(s.ULSpeed * 8),                         // Convert to bits per second
		Jitter:        float64(s.Jitter) / float64(time.Millisecond),  // Convert to milliseconds
		Ping:          float64(s.Latency) / float64(time.Millisecond), // Convert to milliseconds
	}
}

// NewSpeedtestRunner creates a new SpeedtestRunner instance and registers the
// SpeedtestCollector with the provided Prometheus registerer.
// If client is nil, it will use the default speedtest client.
func NewSpeedtestRunner(server string, reg prometheus.Registerer, client SpeedtestClient) *SpeedtestRunner {
	if client == nil {
		client = speedtest.New()
	}
	r := &SpeedtestRunner{
		Server: server,
		client: client,
	}
	// Register the runner as a collector via helper to allow test injection.
	RegisterSpeedtestCollector(r, reg)
	return r
}
