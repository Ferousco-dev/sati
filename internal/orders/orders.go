package orders

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
)

const Currency = "NGN"

type Status string

const (
	StatusPending        Status = "PENDING"
	StatusConfirmed      Status = "CONFIRMED"
	StatusPreparing      Status = "PREPARING"
	StatusReady          Status = "READY"
	StatusOutForDelivery Status = "OUT_FOR_DELIVERY"
	StatusDelivered      Status = "DELIVERED"
	StatusCompleted      Status = "COMPLETED"
	StatusCancelled      Status = "CANCELLED"
	StatusFailed         Status = "FAILED"
)

type Fulfillment string

const (
	Pickup   Fulfillment = "PICKUP"
	Delivery Fulfillment = "DELIVERY"
)

type PaymentMethod string

const (
	PayOnline   PaymentMethod = "ONLINE"
	PayOnPickup PaymentMethod = "PAY_ON_PICKUP"
)

type Item struct {
	ProductID     string `json:"product_id"`
	Name          string `json:"name"`
	UnitPriceKobo int64  `json:"unit_price_kobo"`
	Quantity      int    `json:"quantity"`
	LineTotalKobo int64  `json:"line_total_kobo"`
}

type Order struct {
	ID              string             `json:"id"`
	Number          string             `json:"order_number"`
	Status          Status             `json:"status"`
	PaymentStatus   string             `json:"payment_status"`
	PaymentMethod   PaymentMethod      `json:"payment_method"`
	Fulfillment     Fulfillment        `json:"fulfillment_type"`
	SubtotalKobo    int64              `json:"subtotal_kobo"`
	DeliveryFeeKobo int64              `json:"delivery_fee_kobo"`
	TotalKobo       int64              `json:"total_kobo"`
	Currency        string             `json:"currency"`
	DeliveryAddress *addresses.Address `json:"delivery_address"`
	Notes           string             `json:"notes"`
	Items           []Item             `json:"items"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type StatusView struct {
	Status        Status    `json:"status"`
	PaymentStatus string    `json:"payment_status"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type LineInput struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type FulfillmentInput struct {
	Type      Fulfillment `json:"type"`
	AddressID string      `json:"address_id"`
}

type CreateInput struct {
	Items         []LineInput      `json:"items"`
	Fulfillment   FulfillmentInput `json:"fulfillment"`
	PaymentMethod PaymentMethod    `json:"payment_method"`
	Notes         string           `json:"notes"`
}

var (
	ErrNotFound        = apierror.New(http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found")
	ErrNotCancellable  = apierror.New(http.StatusConflict, "ORDER_NOT_CANCELLABLE", "This order can no longer be cancelled")
	ErrProductNotFound = apierror.New(http.StatusBadRequest, "PRODUCT_NOT_FOUND", "One or more products do not exist")
	ErrAddressRequired = apierror.New(http.StatusBadRequest, "ADDRESS_REQUIRED", "A delivery address is required for delivery orders")
)

func FormatNumber(n int64) string { return fmt.Sprintf("SATI-%d", n) }

var transitions = map[Status][]Status{
	StatusPending:        {StatusConfirmed, StatusCancelled, StatusFailed},
	StatusConfirmed:      {StatusPreparing, StatusCancelled},
	StatusPreparing:      {StatusReady, StatusCancelled},
	StatusReady:          {StatusCompleted, StatusOutForDelivery, StatusCancelled},
	StatusOutForDelivery: {StatusDelivered, StatusFailed},
}

// CanTransition reports whether an order may move from one status to another.
// Pickup orders finish with COMPLETED, delivery orders go out and finish with DELIVERED.
func CanTransition(from, to Status, f Fulfillment) bool {
	if from == StatusReady {
		if to == StatusCompleted && f != Pickup {
			return false
		}
		if to == StatusOutForDelivery && f != Delivery {
			return false
		}
	}
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}
