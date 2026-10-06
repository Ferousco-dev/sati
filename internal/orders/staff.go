package orders

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/internal/payments"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

const maxReason = 200

type CustomerRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type StaffOrder struct {
	Order
	Customer CustomerRef        `json:"customer"`
	Payments []payments.Payment `json:"payments,omitempty"`
}

type StaffFilter struct {
	Status        string
	PaymentStatus string
	PaymentMethod string
	Fulfillment   string
	Query         string
	From          *time.Time
	To            *time.Time
	Limit         int
	Offset        int
}

type StaffPage struct {
	Orders []StaffOrder `json:"orders"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

var (
	ErrReasonRequired  = apierror.New(http.StatusBadRequest, "REASON_REQUIRED", "A reason is required to cancel or fail an order")
	ErrPaymentRequired = apierror.New(http.StatusConflict, "PAYMENT_REQUIRED", "This order must be paid before it can be confirmed")
	ErrOrderClosed     = apierror.New(http.StatusConflict, "ORDER_CLOSED", "This order is cancelled or failed")
	ErrNotManual       = apierror.New(http.StatusConflict, "PAYMENT_NOT_MANUAL", "Online payments are recorded automatically and cannot be marked by hand")
)

func errTransition(from, to Status) *apierror.Error {
	return apierror.New(http.StatusConflict, "INVALID_STATUS_TRANSITION",
		fmt.Sprintf("An order cannot move from %s to %s", from, to))
}

type lockedOrder struct {
	Status        Status
	Fulfillment   Fulfillment
	Method        PaymentMethod
	PaymentStatus string
	TotalKobo     int64
}

func (Repository) lock(ctx context.Context, db database.DBTX, id string) (lockedOrder, error) {
	var l lockedOrder
	var status, fulfillment, method string
	err := db.QueryRow(ctx,
		`SELECT status::text, fulfillment_type::text, payment_method::text, payment_status::text, total_kobo
		 FROM orders WHERE id = $1 FOR UPDATE`, id).Scan(&status, &fulfillment, &method, &l.PaymentStatus, &l.TotalKobo)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedOrder{}, ErrNotFound
	}
	l.Status, l.Fulfillment, l.Method = Status(status), Fulfillment(fulfillment), PaymentMethod(method)
	return l, err
}

func (Repository) setStatus(ctx context.Context, db database.DBTX, id string, to Status, reason string, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE orders SET status = $2::order_status, status_reason = $3,
		   cancelled_at = CASE WHEN $2::text = 'CANCELLED' THEN $4 ELSE cancelled_at END, updated_at = $4
		 WHERE id = $1`, id, string(to), reason, now)
	return err
}

func (Repository) setPaid(ctx context.Context, db database.DBTX, id string, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE orders SET payment_status = 'PAID', payment_provider = $3, updated_at = $2 WHERE id = $1`,
		id, now, payments.ProviderManual)
	return err
}

const staffColumns = columns + `, u.id::text, u.name, u.email::text`

func (Repository) staffByID(ctx context.Context, db database.DBTX, id string) (StaffOrder, error) {
	var c CustomerRef
	o, err := scan(db.QueryRow(ctx,
		`SELECT `+staffColumns+` FROM orders o JOIN users u ON u.id = o.customer_id WHERE o.id = $1`, id),
		&c.ID, &c.Name, &c.Email)
	return StaffOrder{Order: o, Customer: c}, err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (Repository) staffList(ctx context.Context, db database.DBTX, f StaffFilter) ([]StaffOrder, int, error) {
	pattern := ""
	if f.Query != "" {
		pattern = "%" + escapeLike(f.Query) + "%"
	}
	where := `($1 = '' OR o.status::text = $1)
		AND ($2 = '' OR o.payment_status::text = $2)
		AND ($3 = '' OR o.payment_method::text = $3)
		AND ($4 = '' OR o.fulfillment_type::text = $4)
		AND ($5::timestamptz IS NULL OR o.created_at >= $5)
		AND ($6::timestamptz IS NULL OR o.created_at < $6)
		AND ($7 = '' OR ('SATI-' || o.order_number::text) ILIKE $7 OR u.email::text ILIKE $7 OR u.name ILIKE $7)`
	args := []any{f.Status, f.PaymentStatus, f.PaymentMethod, f.Fulfillment, f.From, f.To, pattern}

	var total int
	if err := db.QueryRow(ctx,
		`SELECT count(*) FROM orders o JOIN users u ON u.id = o.customer_id WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx,
		`SELECT `+staffColumns+` FROM orders o JOIN users u ON u.id = o.customer_id WHERE `+where+`
		 ORDER BY o.created_at DESC, o.id LIMIT $8 OFFSET $9`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []StaffOrder{}
	for rows.Next() {
		var c CustomerRef
		o, err := scan(rows, &c.ID, &c.Name, &c.Email)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, StaffOrder{Order: o, Customer: c})
	}
	return out, total, rows.Err()
}

func (s *Service) StaffList(ctx context.Context, f StaffFilter) (StaffPage, error) {
	f.Status = strings.ToUpper(f.Status)
	f.PaymentStatus = strings.ToUpper(f.PaymentStatus)
	f.PaymentMethod = strings.ToUpper(f.PaymentMethod)
	f.Fulfillment = strings.ToUpper(f.Fulfillment)
	f.Query = strings.TrimSpace(f.Query)

	if f.Status != "" && !validStatus(Status(f.Status)) {
		return StaffPage{}, validation("Unknown status filter")
	}
	if f.PaymentStatus != "" && !oneOf(f.PaymentStatus, "UNPAID", "PENDING", "PAID", "FAILED", "REFUNDED", "PARTIALLY_REFUNDED") {
		return StaffPage{}, validation("Unknown payment_status filter")
	}
	if f.PaymentMethod != "" && !oneOf(f.PaymentMethod, string(PayOnline), string(BankTransfer), string(PayOnPickup), string(PayOnDelivery)) {
		return StaffPage{}, validation("Unknown payment_method filter")
	}
	if f.Fulfillment != "" && !oneOf(f.Fulfillment, string(Pickup), string(Delivery)) {
		return StaffPage{}, validation("Unknown fulfillment filter")
	}
	if utf8.RuneCountInString(f.Query) > 100 {
		return StaffPage{}, validation("Search text is too long")
	}

	list, total, err := s.repo.staffList(ctx, s.pool, f)
	if err != nil {
		return StaffPage{}, err
	}
	ids := make([]string, len(list))
	for i, o := range list {
		ids[i] = o.ID
	}
	items, err := s.repo.ItemsFor(ctx, s.pool, ids)
	if err != nil {
		return StaffPage{}, err
	}
	for i := range list {
		if found := items[list[i].ID]; found != nil {
			list[i].Items = found
		}
	}
	return StaffPage{Orders: list, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}

func (s *Service) StaffGet(ctx context.Context, id string) (StaffOrder, error) {
	return s.staffGet(ctx, s.pool, id)
}

func (s *Service) staffGet(ctx context.Context, db database.DBTX, id string) (StaffOrder, error) {
	if !httpx.ValidUUID(id) {
		return StaffOrder{}, ErrNotFound
	}
	o, err := s.repo.staffByID(ctx, db, id)
	if err != nil {
		return StaffOrder{}, err
	}
	items, err := s.repo.ItemsFor(ctx, db, []string{id})
	if err != nil {
		return StaffOrder{}, err
	}
	if found := items[id]; found != nil {
		o.Items = found
	}
	paid, err := s.payments.ForOrders(ctx, db, []string{id})
	if err != nil {
		return StaffOrder{}, err
	}
	o.Payments = paid[id]
	return o, nil
}

// ChangeStatus moves an order along the allowed transitions. Cash orders are
// recorded as paid when they are handed over.
func (s *Service) ChangeStatus(ctx context.Context, actorID, id string, to Status, reason string) (StaffOrder, error) {
	if !httpx.ValidUUID(id) {
		return StaffOrder{}, ErrNotFound
	}
	if !validStatus(to) {
		return StaffOrder{}, validation("Unknown status")
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > maxReason {
		return StaffOrder{}, validation(fmt.Sprintf("reason must be at most %d characters", maxReason))
	}
	if (to == StatusCancelled || to == StatusFailed) && reason == "" {
		return StaffOrder{}, ErrReasonRequired
	}
	if to != StatusCancelled && to != StatusFailed {
		reason = ""
	}

	now := s.now()
	var result StaffOrder
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.repo.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if !CanTransition(cur.Status, to, cur.Fulfillment) {
			return errTransition(cur.Status, to)
		}
		needsPayment := cur.Method == PayOnline || cur.Method == BankTransfer
		if to == StatusConfirmed && needsPayment && cur.PaymentStatus != "PAID" {
			return ErrPaymentRequired
		}

		if err := s.repo.setStatus(ctx, tx, id, to, reason, now); err != nil {
			return err
		}

		cash := cur.Method == PayOnPickup || cur.Method == PayOnDelivery
		if cash && cur.PaymentStatus != "PAID" && (to == StatusCompleted || to == StatusDelivered) {
			if err := s.payments.InsertManualPaid(ctx, tx, id, string(cur.Method), cur.TotalKobo, actorID, "Cash collected", now); err != nil {
				return err
			}
			if err := s.repo.setPaid(ctx, tx, id, now); err != nil {
				return err
			}
		}
		result, err = s.staffGet(ctx, tx, id)
		return err
	})
	return result, err
}

// RecordPayment marks money received by hand, such as a bank transfer
// the staff have seen proof of. A pending bank transfer order is confirmed with it.
func (s *Service) RecordPayment(ctx context.Context, actorID, id, note string) (StaffOrder, error) {
	if !httpx.ValidUUID(id) {
		return StaffOrder{}, ErrNotFound
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maxReason {
		return StaffOrder{}, validation(fmt.Sprintf("note must be at most %d characters", maxReason))
	}

	now := s.now()
	var result StaffOrder
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.repo.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur.Method == PayOnline {
			return ErrNotManual
		}
		if cur.Status == StatusCancelled || cur.Status == StatusFailed {
			return ErrOrderClosed
		}
		if cur.PaymentStatus == "PAID" {
			return payments.ErrAlreadyPaid
		}

		if err := s.payments.InsertManualPaid(ctx, tx, id, string(cur.Method), cur.TotalKobo, actorID, note, now); err != nil {
			return err
		}
		if err := s.repo.setPaid(ctx, tx, id, now); err != nil {
			return err
		}
		if cur.Method == BankTransfer && cur.Status == StatusPending {
			if err := s.repo.setStatus(ctx, tx, id, StatusConfirmed, "", now); err != nil {
				return err
			}
		}
		result, err = s.staffGet(ctx, tx, id)
		return err
	})
	return result, err
}

func validStatus(st Status) bool {
	switch st {
	case StatusPending, StatusConfirmed, StatusPreparing, StatusReady, StatusOutForDelivery,
		StatusDelivered, StatusCompleted, StatusCancelled, StatusFailed:
		return true
	}
	return false
}

func oneOf(v string, options ...string) bool {
	for _, o := range options {
		if v == o {
			return true
		}
	}
	return false
}
