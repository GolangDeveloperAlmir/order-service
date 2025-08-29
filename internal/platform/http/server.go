package server

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"time"

	"github.com/GolangDeveloperAlmir/order-service/internal/config"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
)

type Server struct {
	http     *http.Server
	log      *log.Logger
	certFile string
	keyFile  string
}

func New(handler http.Handler, cfg *config.Config, logger *log.Logger) *Server {
	return &Server{
		http: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
		},
		log:      logger,
		certFile: cfg.TLSCertFile,
		keyFile:  cfg.TLSKeyFile,
	}
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("https server started", log.Str("addr", s.http.Addr))
		if err := s.http.ListenAndServeTLS(s.certFile, s.keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
