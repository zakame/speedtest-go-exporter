package exporter

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

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
	r := NewSpeedtestRunner("42", fr)

	assert.NotNil(t, r)
	assert.Equal(t, "42", r.Server)
	assert.NotNil(t, r.client)

	// Ensure one collector was registered
	assert.Len(t, fr.collectors, 1)

	// Assert the registered collector is a SpeedtestCollector whose runner is r
	sc, ok := fr.collectors[0].(SpeedtestCollector)
	assert.True(t, ok, "registered collector should be SpeedtestCollector")
	assert.Equal(t, r, sc.runner)
}
