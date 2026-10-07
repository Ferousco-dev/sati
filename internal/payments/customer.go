package payments

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

// CustomerPayment is one line of a customer's payment history.
type CustomerPayment struct {
	ID            string     `json:"id"`
	Reference     string     `json:"reference,omitempty"`
	OrderID       string     `json:"order_id"`
	OrderNumber   string     `json:"order_number"`
	AmountKobo    int64      `json:"amount_kobo"`
	Currency      string     `json:"currency"`
	Method        string     `json:"method"`
	Provider      string     `json:"provider"`
	Status        string     `json:"status"`
	RefundPending bool       `json:"refund_pending"`
	PaidAt        *time.Time `json:"paid_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Summary struct {
	PaidKobo          int64 `json:"paid_kobo"`
	RefundedKobo      int64 `json:"refunded_kobo"`
	RefundPendingKobo int64 `json:"refund_pending_kobo"`
}

func (Repository) ListForCustomer(ctx context.Context, db database.DBTX, customerID, status string, limit, offset int) ([]CustomerPayment, int, error) {
	var total int
	if err := db.QueryRow(ctx,
		`SELECT count(*) FROM payments p JOIN orders o ON o.id = p.order_id
		 WHERE o.customer_id = $1 AND ($2 = '' OR p.status::text = $2)`, customerID, status).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx,
		`SELECT p.id::text, COALESCE(p.reference, ''), p.order_id::text, o.order_number, p.amount_kobo, p.method::text,
		        p.provider, p.status::text, (p.status = 'PAID' AND p.needs_refund), p.paid_at, p.created_at
		 FROM payments p JOIN orders o ON o.id = p.order_id
		 WHERE o.customer_id = $1 AND ($2 = '' OR p.status::text = $2)
		 ORDER BY COALESCE(p.paid_at, p.created_at) DESC, p.id LIMIT $3 OFFSET $4`, customerID, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []CustomerPayment{}
	for rows.Next() {
		var c CustomerPayment
		var number int64
		if err := rows.Scan(&c.ID, &c.Reference, &c.OrderID, &number, &c.AmountKobo, &c.Method,
			&c.Provider, &c.Status, &c.RefundPending, &c.PaidAt, &c.CreatedAt); err != nil {
			return nil, 0, err
		}
		c.OrderNumber, c.Currency = "SATI-"+strconv.FormatInt(number, 10), Currency
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (Repository) SummaryForCustomer(ctx context.Context, db database.DBTX, customerID string) (Summary, error) {
	var s Summary
	err := db.QueryRow(ctx,
		`SELECT COALESCE(sum(p.amount_kobo) FILTER (WHERE p.status = 'PAID' AND NOT p.needs_refund), 0),
		        COALESCE(sum(p.amount_kobo) FILTER (WHERE p.status = 'REFUNDED'), 0),
		        COALESCE(sum(p.amount_kobo) FILTER (WHERE p.status = 'PAID' AND p.needs_refund), 0)
		 FROM payments p JOIN orders o ON o.id = p.order_id WHERE o.customer_id = $1`, customerID).
		Scan(&s.PaidKobo, &s.RefundedKobo, &s.RefundPendingKobo)
	return s, err
}

// myPayments is the customer's own payment history, the "money movement" page.
func (h *Handler) myPayments(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	status := strings.ToUpper(r.URL.Query().Get("status"))
	if status != "" && !oneOf(status, "PENDING", "PAID", "FAILED", "REFUNDED", "PARTIALLY_REFUNDED") {
		apierror.Write(w, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "Unknown status filter"))
		return
	}

	id := identityID(r)
	list, total, err := h.repo.ListForCustomer(r.Context(), h.pool, id, status, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	summary, err := h.repo.SummaryForCustomer(r.Context(), h.pool, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{
		"payments": list, "total": total, "limit": limit, "offset": offset, "summary": summary,
	})
}
