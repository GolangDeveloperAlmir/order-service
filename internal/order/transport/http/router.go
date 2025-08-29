package http

import (
	"context"
	"database/sql"
	stdhttp "net/http"
	"os"
	"time"

	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/observability"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/time/rate"
)

type RouterOpt func(*routerConfig)

type routerConfig struct {
	AuthMW func(stdhttp.Handler) stdhttp.Handler
}

func WithAuth(mw func(stdhttp.Handler) stdhttp.Handler) RouterOpt {
	return func(c *routerConfig) { c.AuthMW = mw }
}

func NewRouter(h *Handler, logger *log.Logger, opts ...RouterOpt) stdhttp.Handler {
	cfg := &routerConfig{}
	for _, o := range opts {
		o(cfg)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(15 * time.Second))
	r.Use(mwZap(logger))
	r.Use(rateLimit(10, 20)) // global default; per-endpoint is added below if needed

	// Health & metrics
	r.Get("/healthz", func(w stdhttp.ResponseWriter, r *stdhttp.Request) { w.WriteHeader(200) })
	r.Get("/readyz", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if err := dbPing(r.Context()); err != nil {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	})
	r.Handle("/metrics", observability.Handler())

	// Spec
	r.Get("/openapi.yaml", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		stdhttp.ServeFile(w, r, "openapi.yaml")
	})

	// Public reads
	r.Route("/api/v1/orders", func(r chi.Router) {
		r.Get("/", h.List)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(bindIDParam("id"))
			r.Get("/", h.Get)
		})
	})

	// Protected writes (if Auth middleware provided)
	if cfg.AuthMW != nil {
		r.Group(func(r chi.Router) {
			r.Use(cfg.AuthMW)
			r.Route("/api/v1/orders", func(r chi.Router) {
				r.Post("/", h.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.Use(bindIDParam("id"))
					r.Patch("/", h.PatchStatus)
				})
			})
		})
	} else {
		r.Post("/api/v1/orders", h.Create)
		r.Route("/api/v1/orders/{id}", func(r chi.Router) {
			r.Use(bindIDParam("id"))
			r.Patch("/", h.PatchStatus)
		})
	}

	return r
}

// --- helpers ---

func bindIDParam(name string) func(next stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			id := chi.URLParam(r, name)
			next.ServeHTTP(w, WithURLParam(r, name, id))
		})
	}
}

func rateLimit(rps float64, burst int) func(stdhttp.Handler) stdhttp.Handler {
	lim := rate.NewLimiter(rate.Limit(rps), burst)
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			if !lim.Allow() {
				w.WriteHeader(429)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func dbPing(ctx context.Context) error {
	dsn := os.Getenv("DATABASE_URL")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			return
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	return db.PingContext(ctx)
}
