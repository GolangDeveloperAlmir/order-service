package saga

import (
	"context"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"time"
)

type Executor func(ctx context.Context, sagaID string, action string, payload map[string]any) error

type Manager struct {
	store  *Store
	exec   Executor
	log    *log.Logger
	ticker *time.Ticker
}

func NewManager(store *Store, logger *log.Logger) *Manager {
	return &Manager{
		store:  store,
		exec:   defaultExec(logger),
		log:    logger,
		ticker: time.NewTicker(2 * time.Second),
	}
}

func (m *Manager) Store() *Store {
	return m.store
}

func (m *Manager) RunPoller(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-m.ticker.C:
			m.tick(ctx)
		}
	}
}

func (m *Manager) tick(ctx context.Context) {
	sagaID, stepNo, _, action, payload, err := m.store.PickNextPending(ctx)
	if err != nil {
		m.log.Error("failed to pick next pending", log.Err(err))
		return
	}
	if err := m.exec(ctx, sagaID.String(), action, payload); err != nil {
		if err := m.store.MarkStep(ctx, sagaID, stepNo, "failed", err.Error()); err != nil {
			m.log.Error("failed to mark step", log.Err(err))
		}
		return
	}
	if err := m.store.MarkStep(ctx, sagaID, stepNo, "done", ""); err != nil {
		m.log.Error("failed to mark step", log.Err(err))
	}
	if err := m.store.TryCompleteSaga(ctx, sagaID); err != nil {
		m.log.Error("failed to complete saga", log.Err(err))
	}
}

func defaultExec(logger *log.Logger) Executor {
	return func(ctx context.Context, sagaID, action string, payload map[string]any) error {
		logger.Info("saga exec", log.Str("saga_id", sagaID), log.Str("action", action))

		return nil
	}
}
