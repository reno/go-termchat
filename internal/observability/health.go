package observability

import (
	"encoding/json"
	"net/http"
)

const (
	HealthPath  = "/healthz"
	ReadyPath   = "/readyz"
	MetricsPath = "/metrics"
)

type ReadyFunc func() bool

func Health() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		write(w, "ok\n")
	})
}

func Ready(ready ReadyFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if !ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			write(w, "draining\n")
			return
		}
		w.WriteHeader(http.StatusOK)
		write(w, "ready\n")
	})
}

func MetricsHandler(metrics *Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snapshot := metrics.Snapshot()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"calls":        snapshot.Calls,
			"failures":     snapshot.Failures,
			"open_streams": snapshot.OpenStreams,
			"peak_streams": snapshot.PeakStreams,
		}); err != nil {
			return
		}
	})
}

func write(w http.ResponseWriter, body string) {
	if _, err := w.Write([]byte(body)); err != nil {
		return
	}
}

func Register(mux *http.ServeMux, ready ReadyFunc, metrics *Metrics) {
	mux.Handle(HealthPath, Health())
	mux.Handle(ReadyPath, Ready(ready))
	mux.Handle(MetricsPath, MetricsHandler(metrics))
}
