package domain

import (
	"github.com/google/uuid"
	"testing"
)

func TestNewOrderComputesTotalAndValidates(t *testing.T) {
	cid := uuid.New()
	o, err := New(cid, "USD", []Item{
		{SKU: "A", Quantity: 2, PriceMinor: 150}, // 300
		{SKU: "B", Quantity: 1, PriceMinor: 125}, // 125
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := o.TotalAmount, int64(425); got != want {
		t.Fatalf("total: got %d want %d", got, want)
	}
	if o.Status != StatusCreated {
		t.Fatalf("status: got %s want created", o.Status)
	}
}

func TestStatusTransitions(t *testing.T) {
	cid := uuid.New()
	o, err := New(cid, "USD", []Item{{SKU: "A", Quantity: 1, PriceMinor: 100}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := o.MarkPaid(); err != nil {
		t.Fatalf("mark paid err: %v", err)
	}
	if err := o.MarkShipped(); err != nil {
		t.Fatalf("mark shipped err: %v", err)
	}
	if err := o.Cancel(); err == nil {
		t.Fatalf("cancel after shipped should fail")
	}
}
