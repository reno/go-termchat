package observability

import (
	"sync"

	"connectrpc.com/connect"
)

type Metrics struct {
	mu       sync.Mutex
	calls    map[string]uint64
	failures map[string]uint64
	streams  int64
	peak     int64
}

func NewMetrics() *Metrics {
	return &Metrics{
		calls:    make(map[string]uint64),
		failures: make(map[string]uint64),
	}
}

func (m *Metrics) Observe(procedure string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls[procedure]++
	if err != nil {
		m.failures[procedure+" "+connect.CodeOf(err).String()]++
	}
}

func (m *Metrics) StreamOpened() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.streams++
	if m.streams > m.peak {
		m.peak = m.streams
	}
}

func (m *Metrics) StreamClosed() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.streams--
}

type Snapshot struct {
	Calls       map[string]uint64
	Failures    map[string]uint64
	OpenStreams int64
	PeakStreams int64
}

func (m *Metrics) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	calls := make(map[string]uint64, len(m.calls))
	for procedure, count := range m.calls {
		calls[procedure] = count
	}
	failures := make(map[string]uint64, len(m.failures))
	for label, count := range m.failures {
		failures[label] = count
	}

	return Snapshot{
		Calls:       calls,
		Failures:    failures,
		OpenStreams: m.streams,
		PeakStreams: m.peak,
	}
}
