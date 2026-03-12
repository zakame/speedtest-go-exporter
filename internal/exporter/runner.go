package exporter

import (
	"context"
	"fmt"
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
	// Run executes the speedtest and returns the result, or an error if the test failed.
	Run(ctx context.Context) (*SpeedtestResult, error)
}

// SpeedtestRunner implements [Runner] using [speedtest.Speedtest].
type SpeedtestRunner struct {
	// Server is the ID of the speedtest server to use.
	Server string

	client SpeedtestClient
}

// Run executes the speedtest and returns the result, or an error if the test failed.
func (r *SpeedtestRunner) Run(ctx context.Context) (*SpeedtestResult, error) {
	var s *speedtest.Server

	if r.Server != "" {
		var err error
		s, err = r.client.FetchServerByID(r.Server)
		if err != nil {
			return nil, fmt.Errorf("fetch server %s: %w", r.Server, err)
		}
	} else {
		log.Info("Finding the best server")
		serverList, err := r.client.FetchServers()
		if err != nil {
			return nil, fmt.Errorf("fetch servers: %w", err)
		}
		targets, err := serverList.FindServer([]int{})
		if err != nil {
			return nil, fmt.Errorf("find server: %w", err)
		}
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
		done := make(chan error, 1)
		go func() {
			if err := s.PingTest(nil); err != nil {
				done <- fmt.Errorf("ping test: %w", err)
				return
			}
			if err := s.DownloadTest(); err != nil {
				done <- fmt.Errorf("download test: %w", err)
				return
			}
			if err := s.UploadTest(); err != nil {
				done <- fmt.Errorf("upload test: %w", err)
				return
			}
			done <- nil
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-done:
			if err != nil {
				return nil, err
			}
		}
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

	id, err := strconv.Atoi(s.ID)
	if err != nil {
		log.WithField("id", s.ID).Warn("Server ID is not a valid integer, using 0")
	}

	return &SpeedtestResult{
		ServerID:      id,
		DownloadSpeed: float64(s.DLSpeed * 8),                         // Convert to bits per second
		UploadSpeed:   float64(s.ULSpeed * 8),                         // Convert to bits per second
		Jitter:        float64(s.Jitter) / float64(time.Millisecond),  // Convert to milliseconds
		Ping:          float64(s.Latency) / float64(time.Millisecond), // Convert to milliseconds
	}, nil
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
	RegisterSpeedtestCollector(r, reg, DefaultCollectTimeout)
	return r
}
