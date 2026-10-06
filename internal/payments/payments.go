package payments

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

const (
	ProviderManual = "MANUAL"
	Currency       = "NGN"
)

type Payment struct {
	ID             string     `json:"id"`
	OrderID        string     `json:"order_id"`
	Method         string     `json:"method"`
	AmountKobo     int64      `json:"amount_kobo"`
	Currency       string     `json:"currency"`
	Status         string     `json:"status"`
	Provider       string     `json:"provider"`
	Reference      string     `json:"reference,omitempty"`
	RecordedBy     string     `json:"recorded_by,omitempty"`
	RecordedByName string     `json:"recorded_by_name,omitempty"`
	Note           string     `json:"note,omitempty"`
	PaidAt         *time.Time `json:"paid_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type Row struct {
	Payment
	OrderNumber   string `json:"order_number"`
	CustomerName  string `json:"customer_name"`
	CustomerEmail string `json:"customer_email"`
}

type Filter struct {
	Status   string
	Method   string
	Provider string
	Limit    int
	Offset   int
}

var ErrAlreadyPaid = apierror.New(http.StatusConflict, "ALREADY_PAID", "This order has already been paid")

const columns = `p.id::text, p.order_id::text, p.method::text, p.amount_kobo, p.status::text, p.provider,
	COALESCE(p.reference, ''), COALESCE(p.recorded_by::text, ''), COALESCE(r.name, ''), p.note,
	p.paid_at, p.created_at`

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func scanPayment(row pgx.Row, extra ...any) (Payment, error) {
	var p Payment
	dest := append([]any{&p.ID, &p.OrderID, &p.Method, &p.AmountKobo, &p.Status, &p.Provider,
		&p.Reference, &p.RecordedBy, &p.RecordedByName, &p.Note, &p.PaidAt, &p.CreatedAt}, extra...)
	err := row.Scan(dest...)
	p.Currency = Currency
	return p, err
}

// InsertManualPaid records money received by hand for a whole order.
// A second PAID payment for the same order returns ErrAlreadyPaid.
func (Repository) InsertManualPaid(ctx context.Context, db database.DBTX, orderID, method string, amountKobo int64, recordedBy, note string, now time.Time) error {
	var by *string
	if recordedBy != "" {
		by = &recordedBy
	}
	_, err := db.Exec(ctx,
		`INSERT INTO payments (order_id, method, amount_kobo, status, provider, recorded_by, note, paid_at, created_at, updated_at)
		 VALUES ($1, $2::payment_method, $3, 'PAID', $4, $5, $6, $7, $7, $7)`,
		orderID, method, amountKobo, ProviderManual, by, note, now)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAlreadyPaid
	}
	return err
}

func (Repository) ForOrders(ctx context.Context, db database.DBTX, orderIDs []string) (map[string][]Payment, error) {
	out := map[string][]Payment{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx,
		`SELECT `+columns+` FROM payments p LEFT JOIN users r ON r.id = p.recorded_by
		 WHERE p.order_id = ANY($1::uuid[]) ORDER BY p.created_at`, orderIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out[p.OrderID] = append(out[p.OrderID], p)
	}
	return out, rows.Err()
}

func (Repository) List(ctx context.Context, db database.DBTX, f Filter) ([]Row, int, error) {
	where := `($1 = '' OR p.status::text = $1) AND ($2 = '' OR p.method::text = $2) AND ($3 = '' OR p.provider = $3)`

	var total int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM payments p WHERE `+where, f.Status, f.Method, f.Provider).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx,
		`SELECT `+columns+`, o.order_number, u.name, u.email::text
		 FROM payments p
		 JOIN orders o ON o.id = p.order_id
		 JOIN users u ON u.id = o.customer_id
		 LEFT JOIN users r ON r.id = p.recorded_by
		 WHERE `+where+` ORDER BY p.created_at DESC, p.id LIMIT $4 OFFSET $5`,
		f.Status, f.Method, f.Provider, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Row{}
	for rows.Next() {
		var r Row
		var number int64
		p, err := scanPayment(rows, &number, &r.CustomerName, &r.CustomerEmail)
		if err != nil {
			return nil, 0, err
		}
		r.Payment = p
		r.OrderNumber = "SATI-" + strconv.FormatInt(number, 10)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

type Handler struct {
	pool *pgxpool.Pool
	repo *Repository
	log  *slog.Logger
}

func NewHandler(pool *pgxpool.Pool, log *slog.Logger) *Handler {
	return &Handler{pool: pool, repo: NewRepository(), log: log}
}

func (h *Handler) Mount(mux *http.ServeMux, access auth.Access) {
	mux.Handle("GET /api/v1/admin/payments", access.Staff(http.HandlerFunc(h.list)))
}

type page struct {
	Payments []Row `json:"payments"`
	Total    int   `json:"total"`
	Limit    int   `json:"limit"`
	Offset   int   `json:"offset"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	q := r.URL.Query()
	f := Filter{
		Status:   strings.ToUpper(q.Get("status")),
		Method:   strings.ToUpper(q.Get("method")),
		Provider: strings.ToUpper(q.Get("provider")),
		Limit:    limit,
		Offset:   offset,
	}
	if f.Status != "" && !oneOf(f.Status, "PENDING", "PAID", "FAILED", "REFUNDED", "PARTIALLY_REFUNDED", "UNPAID") {
		apierror.Write(w, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "Unknown status filter"))
		return
	}
	if f.Method != "" && !oneOf(f.Method, "ONLINE", "BANK_TRANSFER", "PAY_ON_PICKUP", "PAY_ON_DELIVERY") {
		apierror.Write(w, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "Unknown method filter"))
		return
	}

	rows, total, err := h.repo.List(r.Context(), h.pool, f)
	if err != nil {
		h.log.Error("list payments", "request_id", middleware.RequestID(r.Context()), "error", err)
		apierror.Write(w, apierror.ErrInternal)
		return
	}
	apierror.JSON(w, http.StatusOK, page{Payments: rows, Total: total, Limit: limit, Offset: offset})
}

func oneOf(v string, options ...string) bool {
	for _, o := range options {
		if v == o {
			return true
		}
	}
	return false
}
