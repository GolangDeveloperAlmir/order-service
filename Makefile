APP=order-service

.PHONY: run build tidy test test-int migrate-up

run:
	go run ./cmd/order-service

build:
	go build -o bin/$(APP) ./cmd/order-service

tidy:
	go mod tidy

migrate-up:
	@psql "$$DATABASE_URL" -f migrations/001_init.sql
	@psql "$$DATABASE_URL" -f migrations/002_outbox_inbox_idempotency.sql
	@psql "$$DATABASE_URL" -f migrations/003_saga.sql

test:
	go test ./... -cover

test-int:
	go test -tags=integration ./internal/order/repository/postgres -run TestRepo -v