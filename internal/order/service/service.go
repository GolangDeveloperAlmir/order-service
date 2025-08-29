package service

import (
	"context"
	"encoding/json"

	"github.com/GolangDeveloperAlmir/order-service/internal/order/domain"
	"github.com/GolangDeveloperAlmir/order-service/internal/order/repository/postgres"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/db"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Repo interface {
	CreateInTx(ctx context.Context, tx pgx.Tx, o *domain.Order) error
	UpdateStatusInTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, status domain.Status) error
	AddOutboxInTx(ctx context.Context, tx pgx.Tx, aggregateID uuid.UUID, eventType string, payload any) error

	Get(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	List(ctx context.Context, limit int, cursor string) (*Page, error)
}

type Page = postgres.Page

type Service struct {
	repo Repo
	tx   *db.TxManager
	log  *log.Logger
}

func New(repo Repo, tx *db.TxManager, logger *log.Logger) *Service {
	return &Service{repo: repo, tx: tx, log: logger}
}

var (
	ordersCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "orders_created_total",
		Help: "total number of orders created",
	})
	statusUpdated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "order_status_updates_total",
		Help: "number of order status updates",
	}, []string{"status"})
)

func (s *Service) Create(ctx context.Context, customerID uuid.UUID, currency string, items []domain.Item) (*domain.Order, error) {
	ctx, span := observability.Tracer("order.service").Start(ctx, "Create")
	defer span.End()

	o, err := domain.New(customerID, currency, items)
	if err != nil {
		s.log.Error("failed to create order", log.Err(err))
		return nil, err
	}
	if err := s.tx.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CreateInTx(ctx, tx, o); err != nil {
			s.log.Error("failed to create order", log.Err(err))
			return err
		}
		b, err := json.Marshal(o)
		if err != nil {
			s.log.Error("failed to marshal order", log.Err(err))
			return err
		}

		return s.repo.AddOutboxInTx(ctx, tx, o.ID, "order.created", b)
	}); err != nil {
		s.log.Error("failed to create order", log.Err(err))
		return nil, err
	}

	ordersCreated.Inc()
	return o, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	ctx, span := observability.Tracer("order.service").Start(ctx, "Get")
	defer span.End()
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, limit int, cursor string) (*Page, error) {
	ctx, span := observability.Tracer("order.service").Start(ctx, "List")
	defer span.End()
	return s.repo.List(ctx, limit, cursor)
}

func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.Status) error {
	ctx, span := observability.Tracer("order.service").Start(ctx, "UpdateStatus")
	defer span.End()

	err := s.tx.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.UpdateStatusInTx(ctx, tx, id, status); err != nil {
			s.log.Error("failed to update order status", log.Err(err))
			return err
		}
		payload := map[string]any{"id": id, "status": status}

		return s.repo.AddOutboxInTx(ctx, tx, id, eventForStatus(status), payload)
	})
	if err == nil {
		statusUpdated.WithLabelValues(string(status)).Inc()
	}
	return err
}

func eventForStatus(s domain.Status) string {
	switch s {
	case domain.StatusPaid:
		return "order.paid"
	case domain.StatusCancelled:
		return "order.cancelled"
	case domain.StatusShipped:
		return "order.shipped"
	default:
		return "order.updated"
	}
}
