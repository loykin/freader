package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server wraps an HTTP server exposing Prometheus metrics.
// It is safe to use as a library: nothing starts automatically.
// Call Start to run the server and Stop to gracefully shut it down.
type Server struct {
	server *http.Server
}

// Start creates and starts a metrics HTTP server on the given address.
// The default Prometheus registry is exposed at /metrics.
// It returns a Server and a nil error on success.
func Start(addr string) (*Server, error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start in a goroutine; caller controls lifetime via Stop.
	go func() {
		_ = srv.ListenAndServe()
	}()

	return &Server{server: srv}, nil
}

// Stop gracefully shuts down the metrics server with a timeout.
func (s *Server) Stop() error {
	if s == nil || s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}
