package orders

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/internal/payments"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

const (
	paymentExpiryReason = "Payment not completed in time"
	mismatchNote        = "Amount or currency did not match the order"
)

var (
	ErrPaymentsOff    = apierror.New(http.StatusServiceUnavailable, "PAYMENTS_NOT_CONFIGURED", "Online payment is not available right now")
	ErrNotOnline      = apierror.New(http.StatusConflict, "NOT_AN_ONLINE_ORDER", "This order is not paid online")
	ErrNotPayable     = apierror.New(http.StatusConflict, "ORDER_NOT_PAYABLE", "This order can no longer be paid")
	ErrAttempts       = apierror.New(http.StatusConflict, "PAYMENT_ATTEMPTS_EXCEEDED", "Too many payment attempts for this order")
	ErrProviderFailed = apierror.New(http.StatusBadGateway, "PAYMENT_PROVIDER_ERROR", "The payment provider could not be reached, please try again")
)

// EnableOnline switches on ONLINE payments with the given gateway. Until then
// ONLINE is not offered, so no order can be created that nobody can pay.
func (s *Service) EnableOnline(provider payments.Provider, frontendURL string) {
	s.provider = provider
	s.frontendURL = strings.TrimRight(frontendURL, "/")
}

func (s *Service) onlineOpen() bool { return s.provider != nil }

type onlineOrder struct {
	lockedOrder
	CustomerID string
	Number     int64
}

func (Repository) lockOnline(ctx context.Context, db database.DBTX, id string) (onlineOrder, error) {
	var o onlineOrder
	var status, fulfillment, method string
	err := db.QueryRow(ctx,
		`SELECT status::text, fulfillment_type::text, payment_method::text, payment_status::text, total_kobo,
		        customer_id::text, order_number
		 FROM orders WHERE id = $1 FOR UPDATE`, id).
		Scan(&status, &fulfillment, &method, &o.PaymentStatus, &o.TotalKobo, &o.CustomerID, &o.Number)
	if errors.Is(err, pgx.ErrNoRows) {
		return onlineOrder{}, ErrNotFound
	}
	o.Status, o.Fulfillment, o.Method = Status(status), Fulfillment(fulfillment), PaymentMethod(method)
	return o, err
}

func (Repository) setPaymentState(ctx context.Context, db database.DBTX, id, status, provider, reference string, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE orders SET payment_status = $2::payment_status,
		   payment_provider = COALESCE(NULLIF($3, ''), payment_provider),
		   payment_reference = COALESCE(NULLIF($4, ''), payment_reference),
		   updated_at = $5 WHERE id = $1`, id, status, provider, reference, now)
	return err
}

func newReference(number int64) (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("SATI-%d-%s", number, strings.ToUpper(hex.EncodeToString(b))), nil
}

// InitializePayment starts a new payment attempt for one of the customer's own orders.
// Every attempt gets its own reference and only one can ever settle the order.
func (s *Service) InitializePayment(ctx context.Context, customerID, orderID string) (payments.Initialization, error) {
	if s.provider == nil {
		return payments.Initialization{}, ErrPaymentsOff
	}
	if !httpx.ValidUUID(orderID) {
		return payments.Initialization{}, ErrNotFound
	}
	customer, err := s.users.ByID(ctx, s.pool, customerID)
	if err != nil {
		return payments.Initialization{}, err
	}

	now := s.now()
	var reference string
	var amount int64
	var number int64
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		o, err := s.repo.lockOnline(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if o.CustomerID != customerID {
			return ErrNotFound
		}
		if o.Method != PayOnline {
			return ErrNotOnline
		}
		if o.PaymentStatus == "PAID" {
			return payments.ErrAlreadyPaid
		}
		if o.Status != StatusPending {
			return ErrNotPayable
		}
		attempts, err := s.payments.CountForOrder(ctx, tx, orderID, s.provider.Name())
		if err != nil {
			return err
		}
		if attempts >= payments.MaxAttemptsPerOrder {
			return ErrAttempts
		}

		if reference, err = newReference(o.Number); err != nil {
			return err
		}
		amount, number = o.TotalKobo, o.Number
		if err := s.payments.InsertPending(ctx, tx, orderID, string(PayOnline), amount, s.provider.Name(), reference, now); err != nil {
			return err
		}
		return s.repo.setPaymentState(ctx, tx, orderID, "PENDING", s.provider.Name(), reference, now)
	})
	if err != nil {
		return payments.Initialization{}, err
	}

	res, err := s.provider.Initialize(ctx, payments.InitRequest{
		Email: customer.Email, AmountKobo: amount, Reference: reference,
		CallbackURL: s.frontendURL + "/orders/" + orderID, OrderID: orderID, OrderNumber: FormatNumber(number),
	})
	if err != nil {
		s.abandonAttempt(ctx, orderID, reference)
		s.log("payment provider initialize failed", "order_id", orderID, "reference", reference, "error", err)
		return payments.Initialization{}, ErrProviderFailed
	}
	return payments.Initialization{
		Reference: reference, AuthorizationURL: res.AuthorizationURL, AccessCode: res.AccessCode,
		AmountKobo: amount, Currency: payments.Currency,
	}, nil
}

// abandonAttempt closes an attempt the gateway never accepted, so the customer can try again.
func (s *Service) abandonAttempt(ctx context.Context, orderID, reference string) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		now := s.now()
		p, err := s.payments.LockByReference(ctx, tx, s.provider.Name(), reference)
		if err != nil {
			return err
		}
		if err := s.payments.SetFailed(ctx, tx, p.ID, "Could not start the payment", false, now); err != nil {
			return err
		}
		return s.refreshPaymentState(ctx, tx, orderID, now)
	})
	if err != nil {
		s.log("could not close failed payment attempt", "reference", reference, "error", err)
	}
}

// refreshPaymentState sets an unpaid order to PENDING while an attempt is open and back to UNPAID when none is.
func (s *Service) refreshPaymentState(ctx context.Context, tx pgx.Tx, orderID string, now time.Time) error {
	var current string
	if err := tx.QueryRow(ctx, `SELECT payment_status::text FROM orders WHERE id = $1`, orderID).Scan(&current); err != nil {
		return err
	}
	if current != "UNPAID" && current != "PENDING" {
		return nil
	}
	open, err := s.payments.HasPending(ctx, tx, orderID)
	if err != nil {
		return err
	}
	status := "UNPAID"
	if open {
		status = "PENDING"
	}
	return s.repo.setPaymentState(ctx, tx, orderID, status, "", "", now)
}

// VerifyPayment asks the gateway what happened to a payment and applies the answer.
// The browser is never trusted to say a payment succeeded.
func (s *Service) VerifyPayment(ctx context.Context, customerID, reference string) (payments.StatusView, error) {
	if s.provider == nil {
		return payments.StatusView{}, ErrPaymentsOff
	}
	view, err := s.payments.ForCustomerReference(ctx, s.pool, customerID, reference)
	if err != nil {
		return payments.StatusView{}, err
	}
	// A failed attempt is asked about again: the customer may have paid after it was closed.
	if view.Payment.Status != "PENDING" && view.Payment.Status != "FAILED" {
		return view, nil
	}

	verification, err := s.provider.Verify(ctx, reference)
	if err != nil {
		s.log("payment provider verify failed", "reference", reference, "error", err)
		return payments.StatusView{}, ErrProviderFailed
	}
	if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return s.applyVerification(ctx, tx, reference, verification)
	}); err != nil {
		return payments.StatusView{}, err
	}
	return s.payments.ForCustomerReference(ctx, s.pool, customerID, reference)
}

// HandleWebhook checks the signature, confirms the transaction with the gateway
// and settles it. Each event is processed once, however many times it is sent.
func (s *Service) HandleWebhook(ctx context.Context, signature string, body []byte) error {
	if s.provider == nil {
		return ErrPaymentsOff
	}
	event, err := s.provider.ParseWebhook(signature, body)
	if err != nil {
		return err
	}

	name := s.provider.Name()
	record := func(db database.DBTX) (bool, error) {
		return s.payments.RecordEvent(ctx, db, name, event.Key, event.Type, event.Reference, body)
	}

	if event.Type != payments.EventChargeSuccess || event.Reference == "" {
		_, err := record(s.pool)
		return err
	}
	if _, err := s.payments.OrderIDByReference(ctx, s.pool, name, event.Reference); errors.Is(err, payments.ErrNotFound) {
		_, err := record(s.pool)
		return err
	} else if err != nil {
		return err
	}

	verification, err := s.provider.Verify(ctx, event.Reference)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		fresh, err := record(tx)
		if err != nil || !fresh {
			return err
		}
		return s.applyVerification(ctx, tx, event.Reference, verification)
	})
}

// applyVerification is the single place a payment result changes an order. It is
// safe to run more than once for the same payment and from several requests at once.
func (s *Service) applyVerification(ctx context.Context, tx pgx.Tx, reference string, v payments.Verification) error {
	name := s.provider.Name()
	orderID, err := s.payments.OrderIDByReference(ctx, tx, name, reference)
	if errors.Is(err, payments.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	order, err := s.repo.lockOnline(ctx, tx, orderID)
	if err != nil {
		return err
	}
	p, err := s.payments.LockByReference(ctx, tx, name, reference)
	if err != nil {
		return err
	}
	now := s.now()

	switch v.Status {
	case payments.VerifySuccess:
		if p.Status == "PAID" {
			return nil
		}
		if v.AmountKobo != p.AmountKobo || v.Currency != payments.Currency {
			s.log("payment amount mismatch", "reference", reference, "expected", p.AmountKobo, "got", v.AmountKobo, "currency", v.Currency)
			if err := s.payments.SetFailed(ctx, tx, p.ID, mismatchNote, true, now); err != nil {
				return err
			}
			return s.refreshPaymentState(ctx, tx, orderID, now)
		}

		if order.Status == StatusPending && order.PaymentStatus != "PAID" {
			if err := s.payments.SetPaid(ctx, tx, p.ID, v.TransactionID, v.GatewayResponse, false, now); err != nil {
				return err
			}
			if err := s.repo.setPaymentState(ctx, tx, orderID, "PAID", name, reference, now); err != nil {
				return err
			}
			if err := s.repo.setStatus(ctx, tx, orderID, StatusConfirmed, "", now); err != nil {
				return err
			}
			s.emitPayment(ctx, tx, orderID, true)
			return nil
		}

		// The money arrived but the order cannot use it: it was cancelled or expired,
		// or another attempt already paid. Keep the record and flag it for a refund.
		if err := s.payments.SetPaid(ctx, tx, p.ID, v.TransactionID, v.GatewayResponse, true, now); err != nil {
			return err
		}
		if order.PaymentStatus != "PAID" {
			return s.repo.setPaymentState(ctx, tx, orderID, "PAID", name, reference, now)
		}
		return nil

	case payments.VerifyFailed, payments.VerifyAbandoned:
		if p.Status != "PENDING" {
			return nil
		}
		if err := s.payments.SetFailed(ctx, tx, p.ID, v.GatewayResponse, false, now); err != nil {
			return err
		}
		return s.refreshPaymentState(ctx, tx, orderID, now)
	}
	return nil
}
