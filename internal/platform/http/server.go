package http

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type Server struct{ server *http.Server }

func NewServer(address string, handler http.Handler) *Server {
	return &Server{server: &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second}}
}

func (s *Server) ListenAndServe() error {
	err := s.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }
