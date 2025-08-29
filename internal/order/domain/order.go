package domain

import (
	"errors"
	"github.com/google/uuid"
	"time"
)

type Status string

const (
	StatusCreated   Status = "created"
	StatusPaid      Status = "paid"
	StatusCancelled Status = "cancelled"
	StatusShipped   Status = "shipped"
)

type Item struct {
	SKU        string `json:"sku"`
	Quantity   int    `json:"quantity"`
	PriceMinor int64  `json:"price_minor"`
}

type Order struct {
	ID          uuid.UUID `json:"id"`
	CustomerID  uuid.UUID `json:"customer_id"`
	Status      Status    `json:"status"`
	Currency    string    `json:"currency"`
	TotalAmount int64     `json:"total_amount"`
	Items       []Item    `json:"items"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func New(customerID uuid.UUID, currency string, items []Item) (*Order, error) {
	if customerID == uuid.Nil {
		return nil, errors.New("customerID is required")
	}
	if currency == "" {
		return nil, errors.New("currency is required")
	}
	if len(items) == 0 {
		return nil, errors.New("at least one item required")
	}
	var total int64
	for _, it := range items {
		if it.SKU == "" || it.Quantity <= 0 || it.PriceMinor < 0 {
			return nil, errors.New("invalid item")
		}
		total += int64(it.Quantity) * it.PriceMinor
	}
	now := time.Now().UTC()

	return &Order{
		ID:          uuid.New(),
		CustomerID:  customerID,
		Status:      StatusCreated,
		Currency:    currency,
		TotalAmount: total,
		Items:       items,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (o *Order) MarkPaid() error {
	if o.Status != StatusCreated {
		return errors.New("only created orders can be paid")
	}
	o.Status = StatusPaid
	o.UpdatedAt = time.Now().UTC()

	return nil
}

func (o *Order) Cancel() error {
	if o.Status == StatusShipped {
		return errors.New("cannot cancel shipped order")
	}
	if o.Status == StatusCancelled {
		return nil
	}
	o.Status = StatusCancelled
	o.UpdatedAt = time.Now().UTC()

	return nil
}

func (o *Order) MarkShipped() error {
	if o.Status != StatusPaid {
		return errors.New("only paid orders can be shipped")
	}
	o.Status = StatusShipped
	o.UpdatedAt = time.Now().UTC()

	return nil
}
