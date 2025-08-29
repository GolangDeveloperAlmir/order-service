package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GolangDeveloperAlmir/order-service/internal/order/domain"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	pool *pgxpool.Pool
	log  *log.Logger
}

func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) CreateInTx(ctx context.Context, tx pgx.Tx, o *domain.Order) error {
	items, err := json.Marshal(o.Items)
	if err != nil {
		r.log.Error("failed to marshal items: %v", log.Err(err))
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO orders (id, customer_id, status, currency, total_amount, items, created_at, updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		o.ID, o.CustomerID, o.Status, o.Currency, o.TotalAmount, items, o.CreatedAt, o.UpdatedAt)
	if err != nil {
		r.log.Error("failed to init transaction: %v", log.Err(err))
	}

	return nil
}

func (r *Repo) AddOutboxInTx(ctx context.Context, tx pgx.Tx, aggregateID uuid.UUID, eventType string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		r.log.Error("failed to marshal payload: %v", log.Err(err))
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (aggregate_id, aggregate_type, event_type, payload)
		VALUES ($1,'order',$2,$3)`,
		aggregateID, eventType, b)
	if err != nil {
		r.log.Error("failed to insert outbox: %v", log.Err(err))
		return err
	}

	return nil
}

func (r *Repo) UpdateStatusInTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, status domain.Status) error {
	ct, err := tx.Exec(ctx, `UPDATE orders SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	if err != nil {
		r.log.Error("failed to update status: %v", log.Err(err))
		return nil
	}
	if ct.RowsAffected() == 0 {
		r.log.Error("failed to update status: not found")
		return errors.New("not found")
	}

	return nil
}

func (r *Repo) Get(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, customer_id, status, currency, total_amount, items, created_at, updated_at
         FROM orders WHERE id=$1`, id)

	var o domain.Order
	var items []byte
	if err := row.Scan(&o.ID, &o.CustomerID, &o.Status, &o.Currency, &o.TotalAmount, &items, &o.CreatedAt, &o.UpdatedAt); err != nil {
		r.log.Error("failed to get order: %v", log.Err(err))
		return nil, err
	}
	if err := json.Unmarshal(items, &o.Items); err != nil {
		r.log.Error("failed to unmarshal items: %v", log.Err(err))
		return nil, err
	}

	return &o, nil
}

type Page struct {
	Orders []*domain.Order `json:"orders"`
	Next   string          `json:"next"`
}

func (r *Repo) List(ctx context.Context, limit int, cursor string) (*Page, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = r.pool.Query(ctx, `
			SELECT id, customer_id, status, currency, total_amount, items, created_at, updated_at
			FROM orders
			ORDER BY created_at, id
			LIMIT $1`, limit+1)
	} else {
		var ts time.Time
		var id uuid.UUID
		if _, err := fmt.Sscanf(cursor, "%s|%s", &ts, &id); err != nil {
			return nil, errors.New("invalid cursor")
		}
		rows, err = r.pool.Query(ctx, `
			SELECT id, customer_id, status, currency, total_amount, items, created_at, updated_at
			FROM orders
			WHERE (created_at, id) > ($1, $2)
			ORDER BY created_at, id
			LIMIT $3`, ts, id, limit+1)
	}
	if err != nil {
		r.log.Error("failed to list orders: %v", log.Err(err))
		return nil, err
	}
	defer rows.Close()

	var page Page
	for rows.Next() {
		var o domain.Order
		var items []byte
		if err := rows.Scan(&o.ID, &o.CustomerID, &o.Status, &o.Currency, &o.TotalAmount, &items, &o.CreatedAt, &o.UpdatedAt); err != nil {
			r.log.Error("failed to scan row: %v", log.Err(err))
			return nil, err
		}
		if err := json.Unmarshal(items, &o.Items); err != nil {
			r.log.Error("failed to unmarshal items: %v", log.Err(err))
			return nil, err
		}
		page.Orders = append(page.Orders, &o)
	}
	if len(page.Orders) > limit {
		last := page.Orders[limit-1]
		page.Orders = page.Orders[:limit]
		page.Next = fmt.Sprintf("%s|%s", last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID)
	}
	if rows.Err() != nil {
		r.log.Error("failed to list orders: %v", log.Err(err))
		return nil, rows.Err()
	}

	return &page, rows.Err()
}
