package saga

import (
	"context"
	"encoding/json"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	log  *log.Logger
}

func NewStore(p *pgxpool.Pool) *Store {
	return &Store{
		pool: p,
	}
}

type Step struct {
	SagaID     uuid.UUID
	StepNo     int
	Name       string
	Status     string
	Action     string
	Compensate string
	Payload    map[string]any
}

func (s *Store) Create(ctx context.Context, name string, steps []Step, data map[string]any) (uuid.UUID, error) {
	id := uuid.New()
	b, err := json.Marshal(data)
	if err != nil {
		s.log.Error("failed to marshal data", log.Err(err))
		return uuid.Nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.log.Error("failed to begin tx", log.Err(err))
		return uuid.Nil, err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			s.log.Error("failed to rollback tx", log.Err(err))
		}
	}()
	if _, err := tx.Exec(ctx, `INSERT INTO sagas(id,name,state,data) VALUES ($1,$2,'pending',$3)`, id, name, b); err != nil {
		s.log.Error("failed to insert saga", log.Err(err))
		return uuid.Nil, err
	}
	for _, st := range steps {
		sb, err := json.Marshal(st.Payload)
		if err != nil {
			s.log.Error("failed to marshal payload", log.Err(err))
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO saga_steps(saga_id, step_no, name, status, action, compensate, payload)
			VALUES($1,$2,$3,'pending',$4,$5,$6)`,
			id, st.StepNo, st.Name, st.Action, st.Compensate, sb); err != nil {
			return uuid.Nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		s.log.Error("failed to commit tx", log.Err(err))
		return uuid.Nil, err
	}

	return id, nil
}

func (s *Store) PickNextPending(ctx context.Context) (uuid.UUID, int, string, string, map[string]any, error) {
	var (
		sagaID uuid.UUID
		stepNo int
		name   string
		action string
		pl     []byte
	)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.log.Error("failed to begin tx", log.Err(err))
		return uuid.Nil, 0, "", "", nil, err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			s.log.Error("failed to rollback tx", log.Err(err))
		}
	}()

	row := tx.QueryRow(ctx, `
		SELECT ss.saga_id, ss.step_no, ss.name, ss.action, ss.payload
		FROM saga_steps ss
		JOIN sagas s ON s.id = ss.saga_id
		WHERE ss.status='pending' AND s.state IN ('pending','compensating')
		ORDER BY s.created_at, ss.step_no
		LIMIT 1
		FOR UPDATE SKIP LOCKED`)
	if err := row.Scan(&sagaID, &stepNo, &name, &action, &pl); err != nil {
		s.log.Error("failed to pick next pending", log.Err(err))
		return uuid.Nil, 0, "", "", nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE saga_steps SET status='started', started_at=now() WHERE saga_id=$1 AND step_no=$2`, sagaID, stepNo); err != nil {
		s.log.Error("failed to update step", log.Err(err))
		return uuid.Nil, 0, "", "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		s.log.Error("failed to commit tx", log.Err(err))
		return uuid.Nil, 0, "", "", nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(pl, &payload); err != nil {
		s.log.Error("failed to unmarshal payload", log.Err(err))
		return uuid.Nil, 0, "", "", nil, err
	}
	return sagaID, stepNo, name, action, payload, nil
}

func (s *Store) MarkStep(ctx context.Context, sagaID uuid.UUID, stepNo int, status string, errText string) error {
	_, err := s.pool.Exec(ctx, `UPDATE saga_steps SET status=$3, error=$4, finished_at=now() WHERE saga_id=$1 AND step_no=$2`,
		sagaID, stepNo, nullIfEmpty(errText))
	if err != nil {
		s.log.Error("failed to update step", log.Err(err))
		return err
	}
	return nil
}

func (s *Store) TryCompleteSaga(ctx context.Context, sagaID uuid.UUID) error {
	var pending int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM saga_steps WHERE saga_id=$1 AND status NOT IN ('done','compensated')`, sagaID).Scan(&pending); err != nil {
		s.log.Error("failed to count pending steps", log.Err(err))
		return err
	}
	if pending == 0 {
		_, err := s.pool.Exec(ctx, `UPDATE sagas SET state='completed', updated_at=now() WHERE id=$1`, sagaID)
		if err != nil {
			s.log.Error("failed to update saga", log.Err(err))
			return err
		}
	}

	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
