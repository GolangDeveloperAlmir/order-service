CREATE TABLE IF NOT EXISTS outbox (
  id BIGSERIAL PRIMARY KEY,
  aggregate_id UUID NOT NULL,
  aggregate_type TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  fail_count INT NOT NULL DEFAULT 0,
  last_error TEXT
);

CREATE INDEX IF NOT EXISTS idx_outbox_available ON outbox (available_at, id) WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS inbox (
  message_id TEXT PRIMARY KEY,
  consumed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
  key TEXT NOT NULL,
  route TEXT NOT NULL,
  customer_id UUID,
  order_id UUID,
  status_code INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ttl_at TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '24 hours'),
  PRIMARY KEY (key, route)
);

CREATE INDEX IF NOT EXISTS idx_idemp_ttl ON idempotency_keys(ttl_at);
