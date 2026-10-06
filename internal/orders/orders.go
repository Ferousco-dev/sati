package orders

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	PayOnline     PaymentMethod = "ONLINE"
	PayOnPickup   PaymentMethod = "PAY_ON_PICKUP"
	PayOnDelivery PaymentMethod = "PAY_ON_DELIVERY"
	BankTransfer  PaymentMethod = "BANK_TRANSFER"
)

type Item struct {
	ProductID     string `json:"product_id"`
	Name          string `json:"name"`
	UnitPriceKobo int64  `json:"unit_price_kobo"`
	Quantity      int    `json:"quantity"`
	LineTotalKobo int64  `json:"line_total_kobo"`
}

type PaymentInstructions struct {
	Method        PaymentMethod `json:"method"`
	BankName      string        `json:"bank_name"`
	AccountNumber string        `json:"account_number"`
	AccountName   string        `json:"account_name"`
	Reference     string        `json:"reference"`
	AmountKobo    int64         `json:"amount_kobo"`
	Currency      string        `json:"currency"`
	WhatsAppURL   string        `json:"whatsapp_url,omitempty"`
}

type CheckoutOptions struct {
	DeliveryFeeKobo int64                           `json:"delivery_fee_kobo"`
	PaymentMethods  map[Fulfillment][]PaymentMethod `json:"payment_methods"`
	Currency        string                          `json:"currency"`
}

type Order struct {
	ID                  string               `json:"id"`
	Number              string               `json:"order_number"`
	Status              Status               `json:"status"`
	PaymentStatus       string               `json:"payment_status"`
	PaymentMethod       PaymentMethod        `json:"payment_method"`
	Fulfillment         Fulfillment          `json:"fulfillment_type"`
	SubtotalKobo        int64                `json:"subtotal_kobo"`
	DeliveryFeeKobo     int64                `json:"delivery_fee_kobo"`
	TotalKobo           int64                `json:"total_kobo"`
	Currency            string               `json:"currency"`
	DeliveryAddress     *addresses.Address   `json:"delivery_address"`
	Notes               string               `json:"notes"`
	PaymentInstructions *PaymentInstructions `json:"payment_instructions"`
	Items               []Item               `json:"items"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
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
	ErrMethodDisabled  = apierror.New(http.StatusBadRequest, "PAYMENT_METHOD_DISABLED", "This payment method is not available right now")
	ErrBankNotReady    = apierror.New(http.StatusBadRequest, "BANK_TRANSFER_NOT_CONFIGURED", "Bank transfer is not available right now")
	ErrAddressRequired = apierror.New(http.StatusBadRequest, "ADDRESS_REQUIRED", "A delivery address is required for delivery orders")
)

// MethodsFor lists the payment methods allowed for a fulfillment type.
// Cash is only paid where the order is handed over.
func MethodsFor(f Fulfillment) []PaymentMethod {
	switch f {
	case Pickup:
		return []PaymentMethod{PayOnline, BankTransfer, PayOnPickup}
	case Delivery:
		return []PaymentMethod{PayOnline, BankTransfer, PayOnDelivery}
	}
	return nil
}

func methodAllowed(f Fulfillment, m PaymentMethod) bool {
	for _, allowed := range MethodsFor(f) {
		if allowed == m {
			return true
		}
	}
	return false
}

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

func formatNaira(kobo int64) string {
	naira := fmt.Sprintf("%d", kobo/100)
	var grouped []string
	for len(naira) > 3 {
		grouped = append([]string{naira[len(naira)-3:]}, grouped...)
		naira = naira[:len(naira)-3]
	}
	grouped = append([]string{naira}, grouped...)
	return fmt.Sprintf("\u20a6%s.%02d", strings.Join(grouped, ","), kobo%100)
}

func whatsAppURL(number, template, orderNumber string, totalKobo int64) string {
	if number == "" {
		return ""
	}
	message := strings.NewReplacer(
		"{order_number}", orderNumber,
		"{total}", formatNaira(totalKobo),
	).Replace(template)
	return "https://wa.me/" + number + "?text=" + url.QueryEscape(message)
}
