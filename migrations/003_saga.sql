CREATE TABLE IF NOT EXISTS sagas (
  id           UUID PRIMARY KEY,
  name         TEXT NOT NULL,
  state        TEXT NOT NULL,        -- pending | completed | failed | compensating
  data         JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS saga_steps (
  id           BIGSERIAL PRIMARY KEY,
  saga_id      UUID NOT NULL REFERENCES sagas(id) ON DELETE CASCADE,
  step_no      INT NOT NULL,
  name         TEXT NOT NULL,
  status       TEXT NOT NULL,        -- pending | started | done | failed | compensating | compensated
  action       TEXT NOT NULL,
  compensate   TEXT,
  payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
  error        TEXT,
  started_at   TIMESTAMPTZ,
  finished_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_saga_pending ON sagas(state) WHERE state IN ('pending','compensating');
