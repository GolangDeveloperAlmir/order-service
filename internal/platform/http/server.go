package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/GolangDeveloperAlmir/order-service/internal/config"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
)

type Server struct {
	http *http.Server
	log  *log.Logger
}

func New(handler http.Handler, cfg *config.Config, logger *log.Logger) *Server {
	return &Server{
		http: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
		log: logger,
	}
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("http server started", log.Str("addr", s.http.Addr))
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		grace, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		s.log.Info("shutting down http server")
		if err := s.http.Shutdown(grace); err != nil {
			s.log.Error("http shutdown error", log.Err(err))

			return err
		}
	case err := <-errCh:
		s.log.Error("http server error", log.Err(err))
		return err
	}

	return nil
}
