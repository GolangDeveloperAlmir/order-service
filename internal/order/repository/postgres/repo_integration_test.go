//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	pgrepo "github.com/GolangDeveloperAlmir/order-service/internal/order/repository/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func withDB(t *testing.T, fn func(ctx context.Context, pool *pgxpool.Pool)) {
	t.Helper()
	ctx := context.Background()

	pg, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16"),
		postgres.WithDatabase("orders"),
		postgres.WithUsername("app"),
		postgres.WithPassword("app"),
	)
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}

	// apply migrations
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	defer db.Close()
	migs := []string{
		"../../../../migrations/001_init.sql",
		"../../../../migrations/002_outbox_inbox_idempotency.sql",
		"../../../../migrations/003_saga.sql",
	}
	for _, p := range migs {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if _, err := db.Exec(string(b)); err != nil {
			t.Fatalf("apply %s: %v", p, err)
		}
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()

	fn(ctx, pool)
}

func TestRepo_Create_Get_List_Update_Outbox(t *testing.T) {
	withDB(t, func(ctx context.Context, pool *pgxpool.Pool) {
		r := pgrepo.New(pool)

		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)

		cid := uuid.New()
		o, err := domain.New(cid, "USD", []domain.Item{{SKU: "X", Quantity: 2, PriceMinor: 200}})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.CreateInTx(ctx, tx, o); err != nil {
			t.Fatal(err)
		}
		if err := r.AddOutboxInTx(ctx, tx, o.ID, "order.created", o); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		got, err := r.Get(ctx, o.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.TotalAmount != o.TotalAmount {
			t.Fatalf("amount mismatch")
		}

		page, err := r.List(ctx, 10, "")
		if err != nil || len(page.Orders) == 0 {
			t.Fatalf("list err: %v len=%d", err, len(page.Orders))
		}

		tx2, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx2.Rollback(ctx)

		if err := r.UpdateStatusInTx(ctx, tx2, o.ID, domain.StatusPaid); err != nil {
			t.Fatal(err)
		}
		if err := r.AddOutboxInTx(ctx, tx2, o.ID, "order.paid", map[string]any{"id": o.ID}); err != nil {
			t.Fatal(err)
		}
		if err := tx2.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		var cnt int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox WHERE aggregate_id=$1`, o.ID).Scan(&cnt); err != nil {
			t.Fatal(err)
		}
		if cnt != 2 {
			t.Fatalf("want 2 outbox rows, got %d", cnt)
		}
	})
}
