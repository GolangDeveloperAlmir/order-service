package db

import (
	"context"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TxManager struct {
	pool *pgxpool.Pool
	log  *log.Logger
}

func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{
		pool: pool,
	}
}

func (t *TxManager) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		t.log.Error("failed to begin tx", log.Err(err))
		return err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.log.Error("failed to rollback tx", log.Err(err))
		}
	}()

	if err := fn(tx); err != nil {
		t.log.Error("failed to execute tx", log.Err(err))
		return err
	}

	return tx.Commit(ctx)
}
