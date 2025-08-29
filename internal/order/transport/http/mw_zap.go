package http

import (
	"context"
	"net/http"
	"time"

	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/go-chi/chi/middleware"
	"go.uber.org/zap"
)

func mwZap(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &respWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(ww, r)
			logger.Info("http",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.status),
				zap.Duration("dur", time.Since(start)),
				zap.String("req_id", reqIDFromCtx(r.Context())),
			)
		})
	}
}

type respWriter struct {
	http.ResponseWriter
	status int
}

func (w *respWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func reqIDFromCtx(ctx context.Context) string {
	if v := ctx.Value(middleware.RequestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
