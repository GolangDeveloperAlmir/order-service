package outbox

import (
	"context"
	"encoding/json"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

type Publisher interface {
	Publish(ctx context.Context, key string, value []byte) error
}

type Relay struct {
	pool    *pgxpool.Pool
	pub     Publisher
	ticker  *time.Ticker
	batch   int
	logger  *log.Logger
	metrics *relayMetrics
}

type relayMetrics struct {
	total  *prometheus.CounterVec
	errors prometheus.Counter
	lag    prometheus.Gauge
}

func newMetrics() *relayMetrics {
	m := &relayMetrics{
		total: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbox_events_total", Help: "published outbox events",
		}, []string{"event"}),
		errors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "outbox_publish_errors_total", Help: "outbox publish errors",
		}),
		lag: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_oldest_age_seconds", Help: "oldest unpublished event age",
		}),
	}
	prometheus.MustRegister(m.total, m.errors, m.lag)

	return m
}

func New(pool *pgxpool.Pool, pub Publisher, interval time.Duration, batch int, logger *log.Logger) *Relay {
	return &Relay{
		pool:    pool,
		pub:     pub,
		ticker:  time.NewTicker(interval),
		batch:   batch,
		logger:  logger,
		metrics: newMetrics(),
	}
}

func (r *Relay) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-r.ticker.C:
			if err := r.drain(ctx); err != nil {
				r.logger.Error("outbox drain error", log.Err(err))
			}
		}
	}
}

func (r *Relay) drain(ctx context.Context) error {
	var oldest time.Time
	_ = r.pool.QueryRow(ctx, `SELECT COALESCE(MIN(created_at), now()) FROM outbox WHERE published_at IS NULL`).Scan(&oldest)
	r.metrics.lag.Set(time.Since(oldest).Seconds())

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		r.logger.Error("failed to begin tx", log.Err(err))
		return err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			r.logger.Error("failed to rollback tx", log.Err(err))
		}
	}()

	rows, err := tx.Query(ctx, `
		SELECT id, event_type, aggregate_type, aggregate_id, payload, created_at
		FROM outbox
		WHERE published_at IS NULL AND available_at <= now()
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, r.batch)
	if err != nil {
		r.logger.Error("failed to list outbox", log.Err(err))
		return err
	}
	defer rows.Close()

	type picked struct {
		id  int64
		key string
		val []byte
		typ string
	}
	var batch []picked

	for rows.Next() {
		var (
			id           int64
			etype        string
			aggType      string
			aggID        string
			payloadBytes []byte
			createdAt    time.Time
		)
		if err := rows.Scan(&id, &etype, &aggType, &aggID, &payloadBytes, &createdAt); err != nil {
			return err
		}
		env, _ := json.Marshal(map[string]any{
			"type":           etype,
			"aggregate_type": aggType,
			"aggregate_id":   aggID,
			"payload":        json.RawMessage(payloadBytes),
			"created_at":     createdAt,
		})
		batch = append(batch, picked{id: id, key: aggID, val: env, typ: etype})
	}
	if err := rows.Err(); err != nil {
		r.logger.Error("failed to list outbox", log.Err(err))
		return err
	}
	if len(batch) == 0 {
		return tx.Commit(ctx)
	}

	for _, m := range batch {
		if err := r.pub.Publish(ctx, m.key, m.val); err != nil {
			r.metrics.errors.Inc()
			_, _ = tx.Exec(ctx, `UPDATE outbox
				SET fail_count = fail_count + 1,
				    last_error = $2,
				    available_at = now() + make_interval(secs => LEAST(60, POW(2, fail_count)))
				WHERE id = $1`, m.id, err.Error())
			continue
		}
		r.metrics.total.WithLabelValues(m.typ).Inc()
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id=$1`, m.id); err != nil {
			r.logger.Error("failed to update outbox", log.Err(err))
			return err
		}
	}

	return tx.Commit(ctx)
}
