package idempotency

import (
	"context"
	"errors"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct {
	q   Querier
	log *log.Logger
}

func NewStore(q Querier) *Store { return &Store{q: q} }

func (s *Store) Save(ctx context.Context, key, route string, customerID uuid.UUID, orderID uuid.UUID, status int) error {
	_, err := s.q.Exec(ctx, `
		INSERT INTO idempotency_keys (key, route, customer_id, order_id, status_code)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (key, route) DO NOTHING`, key, route, customerID, orderID, status)
	if err != nil {
		s.log.Error("failed to save idempotency key: %v", log.Err(err))
		return err
	}

	return nil
}

type Result struct {
	OrderID uuid.UUID
	Status  int
	Found   bool
}

func (s *Store) Get(ctx context.Context, key, route string) (*Result, error) {
	var r Result
	err := s.q.QueryRow(ctx, `
		SELECT order_id, status_code FROM idempotency_keys
		WHERE key=$1 AND route=$2 AND ttl_at > now()`, key, route).Scan(&r.OrderID, &r.Status)
	if errors.Is(pgx.ErrNoRows, err) {
		s.log.Debug("idempotency key not found")

		return &Result{Found: false}, nil
	}
	if err != nil {
		s.log.Error("failed to get idempotency key: %v", log.Err(err))
		return nil, err
	}
	r.Found = true

	return &r, nil
}
