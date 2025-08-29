package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/GolangDeveloperAlmir/order-service/internal/order/domain"
	ordersvc "github.com/GolangDeveloperAlmir/order-service/internal/order/service"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/idempotency"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	"github.com/GolangDeveloperAlmir/order-service/internal/platform/saga"
	"github.com/GolangDeveloperAlmir/order-service/pkg/respond"
	"github.com/google/uuid"
)

const maxBodyBytes = 1 << 20

var currencyRe = regexp.MustCompile("^[A-Z]{3}$")

type Service interface {
	Create(ctx context.Context, customerID uuid.UUID, currency string, items []domain.Item) (*domain.Order, error)
	Get(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	List(ctx context.Context, limit int, cursor string) (*ordersvc.Page, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.Status) error
}

type Handler struct {
	svc  Service
	log  *log.Logger
	idem *idempotency.Store
	sg   *saga.Manager
}

func NewHandler(svc Service, logger *log.Logger, idem *idempotency.Store, sg *saga.Manager) *Handler {
	return &Handler{svc: svc, log: logger, idem: idem, sg: sg}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(struct{})); err != io.EOF {
		return errors.New("body must contain only one JSON object")
	}
	return nil
}

type createReq struct {
	CustomerID string        `json:"customer_id"`
	Currency   string        `json:"currency"`
	Items      []domain.Item `json:"items"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := decodeJSON(w, r, &req); err != nil {
		h.log.Error("failed to decode json", log.Err(err))
		respond.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !currencyRe.MatchString(req.Currency) {
		respond.Error(w, http.StatusBadRequest, "invalid currency")
		return
	}
	cid, err := uuid.Parse(req.CustomerID)
	if err != nil {
		h.log.Error("failed to parse customer_id: %v", log.Err(err))
		respond.Error(w, http.StatusBadRequest, "invalid customer_id")
		return
	}
	const route = "POST:/api/v1/orders"
	key := r.Header.Get("Idempotency-Key")
	if key != "" {
		if res, err := h.idem.Get(r.Context(), key, route); err == nil && res.Found {
			if o, err := h.svc.Get(r.Context(), res.OrderID); err == nil && o != nil {
				respond.JSON(w, res.Status, o)
				return
			}
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	o, err := h.svc.Create(ctx, cid, req.Currency, req.Items)
	if err != nil {
		h.log.Error("failed to create order: %v", log.Err(err))
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if key != "" {
		if err := h.idem.Save(r.Context(), key, route, cid, o.ID, http.StatusCreated); err != nil {
			h.log.Error("failed to save idempotency key: %v", log.Err(err))
		}
	}

	// Start a tiny saga (demo) to reserve inventory & authorize payment
	if h.sg != nil {
		steps := []saga.Step{
			{StepNo: 1, Name: "reserve-inventory", Action: "reserve_inventory", Compensate: "release_inventory",
				Payload: map[string]any{"order_id": o.ID.String()}},
			{StepNo: 2, Name: "authorize-payment", Action: "authorize_payment", Compensate: "void_payment",
				Payload: map[string]any{"order_id": o.ID.String(), "amount_minor": o.TotalAmount}},
		}
		_, err = h.sg.Store().Create(r.Context(), "order-fulfillment", steps, map[string]any{"order_id": o.ID.String()})
		if err != nil {
			h.log.Error("failed to create saga: %v", log.Err(err))
		}
	}

	respond.JSON(w, http.StatusCreated, o)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chiURLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.log.Error("failed to parse id: %v", log.Err(err))
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	o, err := h.svc.Get(ctx, id)
	if err != nil {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	respond.JSON(w, http.StatusOK, o)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("cursor")
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	page, err := h.svc.List(ctx, limit, cursor)
	if err != nil {
		h.log.Error("failed to list orders: %v", log.Err(err))
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, page)
}

type patchStatusReq struct {
	Status string `json:"status"`
}

func (h *Handler) PatchStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chiURLParam(r, "id"))
	if err != nil {
		h.log.Error("failed to parse id: %v", log.Err(err))
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req patchStatusReq
	if err := decodeJSON(w, r, &req); err != nil || req.Status == "" {
		h.log.Error("failed to decode body", log.Err(err))
		respond.Error(w, http.StatusBadRequest, "invalid body")
		return
	}

	var target domain.Status
	switch req.Status {
	case string(domain.StatusPaid):
		target = domain.StatusPaid
	case string(domain.StatusCancelled):
		target = domain.StatusCancelled
	case string(domain.StatusShipped):
		target = domain.StatusShipped
	default:
		respond.Error(w, http.StatusBadRequest, "unsupported status")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.svc.UpdateStatus(ctx, id, target); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": string(target)})
}

// --- tiny shims to decouple router from handler for tests ---

type ctxKey string

func WithURLParam(r *http.Request, key, val string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxKey(key), val)

	return r.WithContext(ctx)
}

func chiURLParam(r *http.Request, key string) string {
	if v := r.Context().Value(ctxKey(key)); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
