package payments

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

const MaxAttemptsPerOrder = 10

var (
	ErrNotFound      = apierror.New(http.StatusNotFound, "PAYMENT_NOT_FOUND", "Payment not found")
	ErrNotRefundable = apierror.New(http.StatusConflict, "PAYMENT_NOT_REFUNDABLE", "Only a paid payment can be marked as refunded")
)

type Initialization struct {
	Reference        string `json:"reference"`
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code,omitempty"`
	AmountKobo       int64  `json:"amount_kobo"`
	Currency         string `json:"currency"`
}

// StatusView is what a customer sees after paying or when a payment is checked.
type StatusView struct {
	Payment            Payment `json:"payment"`
	OrderStatus        string  `json:"order_status"`
	OrderPaymentStatus string  `json:"order_payment_status"`
}

// Service is implemented by the orders module, which owns the order changes a payment causes.
type Service interface {
	InitializePayment(ctx context.Context, customerID, orderID string) (Initialization, error)
	VerifyPayment(ctx context.Context, customerID, reference string) (StatusView, error)
	HandleWebhook(ctx context.Context, signature string, body []byte) error
}

type Locked struct {
	ID          string
	OrderID     string
	AmountKobo  int64
	Status      string
	NeedsRefund bool
}

func (Repository) InsertPending(ctx context.Context, db database.DBTX, orderID, method string, amountKobo int64, provider, reference string, now time.Time) error {
	_, err := db.Exec(ctx,
		`INSERT INTO payments (order_id, method, amount_kobo, status, provider, reference, created_at, updated_at)
		 VALUES ($1, $2::payment_method, $3, 'PENDING', $4, $5, $6, $6)`,
		orderID, method, amountKobo, provider, reference, now)
	return err
}

func (Repository) CountForOrder(ctx context.Context, db database.DBTX, orderID, provider string) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM payments WHERE order_id = $1 AND provider = $2`, orderID, provider).Scan(&n)
	return n, err
}

func (Repository) HasPending(ctx context.Context, db database.DBTX, orderID string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payments WHERE order_id = $1 AND status = 'PENDING')`, orderID).Scan(&exists)
	return exists, err
}

// OrderIDByReference finds the order a gateway reference belongs to, without locking.
func (Repository) OrderIDByReference(ctx context.Context, db database.DBTX, provider, reference string) (string, error) {
	var orderID string
	err := db.QueryRow(ctx, `SELECT order_id::text FROM payments WHERE provider = $1 AND reference = $2`, provider, reference).Scan(&orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return orderID, err
}

func (Repository) LockByReference(ctx context.Context, db database.DBTX, provider, reference string) (Locked, error) {
	var l Locked
	err := db.QueryRow(ctx,
		`SELECT id::text, order_id::text, amount_kobo, status::text, needs_refund
		 FROM payments WHERE provider = $1 AND reference = $2 FOR UPDATE`, provider, reference).
		Scan(&l.ID, &l.OrderID, &l.AmountKobo, &l.Status, &l.NeedsRefund)
	if errors.Is(err, pgx.ErrNoRows) {
		return Locked{}, ErrNotFound
	}
	return l, err
}

func (Repository) SetPaid(ctx context.Context, db database.DBTX, id, transactionID, gateway string, needsRefund bool, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE payments SET status = 'PAID', paid_at = $2, provider_transaction_id = NULLIF($3, ''),
		   gateway_response = $4, needs_refund = $5, updated_at = $2 WHERE id = $1`,
		id, now, transactionID, gateway, needsRefund)
	return err
}

func (Repository) SetFailed(ctx context.Context, db database.DBTX, id, gateway string, needsRefund bool, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE payments SET status = 'FAILED', gateway_response = $2, needs_refund = $3, updated_at = $4 WHERE id = $1`,
		id, gateway, needsRefund, now)
	return err
}

// FailPendingForOrder closes attempts that can no longer succeed, such as when an order expires.
func (Repository) FailPendingForOrder(ctx context.Context, db database.DBTX, orderID, reason string, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE payments SET status = 'FAILED', gateway_response = $2, updated_at = $3
		 WHERE order_id = $1 AND status = 'PENDING'`, orderID, reason, now)
	return err
}

// FlagRefundForOrder marks money already received for an order that has just been cancelled or failed.
func (Repository) FlagRefundForOrder(ctx context.Context, db database.DBTX, orderID string, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE payments SET needs_refund = TRUE, updated_at = $2 WHERE order_id = $1 AND status = 'PAID' AND NOT needs_refund`,
		orderID, now)
	return err
}

// RecordEvent stores a webhook event once. It reports false when the event was already handled.
func (Repository) RecordEvent(ctx context.Context, db database.DBTX, provider, key, eventType, reference string, payload []byte) (bool, error) {
	tag, err := db.Exec(ctx,
		`INSERT INTO payment_events (provider, event_key, event_type, reference, payload)
		 VALUES ($1, $2, $3, $4, $5) ON CONFLICT (provider, event_key) DO NOTHING`,
		provider, key, eventType, reference, payload)
	return tag.RowsAffected() == 1, err
}

func (Repository) ForCustomerReference(ctx context.Context, db database.DBTX, customerID, reference string) (StatusView, error) {
	var v StatusView
	row := db.QueryRow(ctx,
		`SELECT `+columns+`, o.status::text, o.payment_status::text
		 FROM payments p
		 JOIN orders o ON o.id = p.order_id
		 LEFT JOIN users r ON r.id = p.recorded_by
		 WHERE p.reference = $1 AND o.customer_id = $2`, reference, customerID)
	p, err := scanPayment(row, &v.OrderStatus, &v.OrderPaymentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return StatusView{}, ErrNotFound
	}
	v.Payment = p
	return v, err
}

// MarkRefunded records that the money was returned outside this system.
func (Repository) MarkRefunded(ctx context.Context, db database.DBTX, id, note string, now time.Time) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	var orderID, reference string
	var status string
	err := db.QueryRow(ctx, `SELECT order_id::text, status::text, COALESCE(reference, '') FROM payments WHERE id = $1 FOR UPDATE`, id).
		Scan(&orderID, &status, &reference)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "PAID" {
		return ErrNotRefundable
	}

	if _, err := db.Exec(ctx,
		`UPDATE payments SET status = 'REFUNDED', needs_refund = FALSE, note = $2, updated_at = $3 WHERE id = $1`,
		id, note, now); err != nil {
		return err
	}
	_, err = db.Exec(ctx,
		`UPDATE orders SET payment_status = 'REFUNDED', updated_at = $3
		 WHERE id = $1 AND payment_status = 'PAID' AND (payment_reference = $2 OR payment_reference IS NULL OR $2 = '')`,
		orderID, reference, now)
	return err
}
