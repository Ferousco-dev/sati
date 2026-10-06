package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

const (
	StatusAssigned  = "ASSIGNED"
	StatusPickedUp  = "PICKED_UP"
	StatusDelivered = "DELIVERED"
	StatusFailed    = "FAILED"
	StatusCancelled = "CANCELLED"
)

type Delivery struct {
	ID            string     `json:"id"`
	OrderID       string     `json:"order_id"`
	Status        string     `json:"status"`
	RiderID       string     `json:"rider_id"`
	RiderName     string     `json:"rider_name"`
	ZoneName      string     `json:"zone_name,omitempty"`
	AssignedAt    time.Time  `json:"assigned_at"`
	PickedUpAt    *time.Time `json:"picked_up_at"`
	DeliveredAt   *time.Time `json:"delivered_at"`
	FailureReason string     `json:"failure_reason,omitempty"`
	Notes         string     `json:"notes,omitempty"`
}

type AdminRow struct {
	Delivery
	OrderNumber  string          `json:"order_number"`
	OrderStatus  string          `json:"order_status"`
	CustomerName string          `json:"customer_name"`
	Address      json.RawMessage `json:"delivery_address"`
}

type RiderJob struct {
	Delivery
	OrderNumber   string          `json:"order_number"`
	OrderStatus   string          `json:"order_status"`
	CustomerName  string          `json:"customer_name"`
	Address       json.RawMessage `json:"delivery_address"`
	TotalKobo     int64           `json:"total_kobo"`
	PaymentMethod string          `json:"payment_method"`
	PaymentStatus string          `json:"payment_status"`
	CollectKobo   int64           `json:"collect_kobo"`
	Currency      string          `json:"currency"`
}

type AdminFilter struct {
	Status  string
	RiderID string
	Limit   int
	Offset  int
}

var ErrNotFound = apierror.New(http.StatusNotFound, "DELIVERY_NOT_FOUND", "Delivery not found")

const (
	columns = `d.id::text, d.order_id::text, d.status::text, d.rider_id::text, r.name, COALESCE(z.name, ''),
		d.assigned_at, d.picked_up_at, d.delivered_at, d.failure_reason, d.notes`
	from = ` FROM delivery_orders d JOIN users r ON r.id = d.rider_id LEFT JOIN delivery_zones z ON z.id = d.zone_id`
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func scan(row pgx.Row, extra ...any) (Delivery, error) {
	var d Delivery
	dest := append([]any{&d.ID, &d.OrderID, &d.Status, &d.RiderID, &d.RiderName, &d.ZoneName,
		&d.AssignedAt, &d.PickedUpAt, &d.DeliveredAt, &d.FailureReason, &d.Notes}, extra...)
	err := row.Scan(dest...)
	return d, err
}

// ByOrder returns nil when no rider has been assigned. With lock set the row is held until the transaction ends.
func (Repository) ByOrder(ctx context.Context, db database.DBTX, orderID string, lock bool) (*Delivery, error) {
	query := `SELECT ` + columns + from + ` WHERE d.order_id = $1`
	if lock {
		query += ` FOR UPDATE OF d`
	}
	d, err := scan(db.QueryRow(ctx, query, orderID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// OrderIDFor finds which order a delivery belongs to, so callers can lock the order first.
func (Repository) OrderIDFor(ctx context.Context, db database.DBTX, riderID, deliveryID string) (string, error) {
	if !httpx.ValidUUID(deliveryID) {
		return "", ErrNotFound
	}
	var orderID string
	err := db.QueryRow(ctx,
		`SELECT order_id::text FROM delivery_orders WHERE id = $1 AND rider_id = $2`, deliveryID, riderID).Scan(&orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return orderID, err
}

func (Repository) Assign(ctx context.Context, db database.DBTX, orderID, riderID, zoneID, assignedBy, notes string, now time.Time) error {
	var zone, by *string
	if zoneID != "" {
		zone = &zoneID
	}
	if assignedBy != "" {
		by = &assignedBy
	}
	_, err := db.Exec(ctx,
		`INSERT INTO delivery_orders (order_id, rider_id, zone_id, status, assigned_by, assigned_at, notes, created_at, updated_at)
		 VALUES ($1, $2, $3, 'ASSIGNED', $4, $5, $6, $5, $5)
		 ON CONFLICT (order_id) DO UPDATE SET
		   rider_id = EXCLUDED.rider_id, zone_id = EXCLUDED.zone_id, status = 'ASSIGNED',
		   assigned_by = EXCLUDED.assigned_by, assigned_at = EXCLUDED.assigned_at,
		   picked_up_at = NULL, delivered_at = NULL, failure_reason = '',
		   notes = CASE WHEN EXCLUDED.notes = '' THEN delivery_orders.notes ELSE EXCLUDED.notes END,
		   updated_at = EXCLUDED.updated_at`,
		orderID, riderID, zone, by, now, notes)
	return err
}

func (Repository) SetStatus(ctx context.Context, db database.DBTX, id, status, reason string, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE delivery_orders SET status = $2::delivery_status, failure_reason = $3,
		   picked_up_at = CASE WHEN $2::text = 'PICKED_UP' THEN $4 ELSE picked_up_at END,
		   delivered_at = CASE WHEN $2::text = 'DELIVERED' THEN $4 ELSE delivered_at END,
		   updated_at = $4
		 WHERE id = $1`, id, status, reason, now)
	return err
}

// CancelForOrder closes any unfinished delivery when its order is cancelled.
func (Repository) CancelForOrder(ctx context.Context, db database.DBTX, orderID string, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE delivery_orders SET status = 'CANCELLED', updated_at = $2
		 WHERE order_id = $1 AND status IN ('ASSIGNED', 'PICKED_UP', 'FAILED')`, orderID, now)
	return err
}

func (Repository) ForOrders(ctx context.Context, db database.DBTX, orderIDs []string) (map[string]Delivery, error) {
	out := map[string]Delivery{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `SELECT `+columns+from+` WHERE d.order_id = ANY($1::uuid[])`, orderIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out[d.OrderID] = d
	}
	return out, rows.Err()
}

func (Repository) ListAdmin(ctx context.Context, db database.DBTX, f AdminFilter) ([]AdminRow, int, error) {
	where := ` WHERE ($1 = '' OR d.status::text = $1) AND ($2 = '' OR d.rider_id::text = $2)`

	var total int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM delivery_orders d`+where, f.Status, f.RiderID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx,
		`SELECT `+columns+`, o.order_number, o.status::text, c.name, o.delivery_address`+from+
			` JOIN orders o ON o.id = d.order_id JOIN users c ON c.id = o.customer_id`+where+
			` ORDER BY d.assigned_at DESC, d.id LIMIT $3 OFFSET $4`, f.Status, f.RiderID, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []AdminRow{}
	for rows.Next() {
		var row AdminRow
		var number int64
		var address []byte
		d, err := scan(rows, &number, &row.OrderStatus, &row.CustomerName, &address)
		if err != nil {
			return nil, 0, err
		}
		row.Delivery, row.OrderNumber, row.Address = d, "SATI-"+strconv.FormatInt(number, 10), address
		out = append(out, row)
	}
	return out, total, rows.Err()
}

// ListForRider returns one rider's own jobs. Active jobs are those not yet finished.
func (Repository) ListForRider(ctx context.Context, db database.DBTX, riderID string, activeOnly bool, limit, offset int) ([]RiderJob, int, error) {
	where := ` WHERE d.rider_id = $1 AND (NOT $2 OR d.status IN ('ASSIGNED', 'PICKED_UP', 'FAILED'))`

	var total int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM delivery_orders d`+where, riderID, activeOnly).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx,
		`SELECT `+columns+`, o.order_number, o.status::text, c.name, o.delivery_address, o.total_kobo,
		        o.payment_method::text, o.payment_status::text`+from+
			` JOIN orders o ON o.id = d.order_id JOIN users c ON c.id = o.customer_id`+where+
			` ORDER BY d.assigned_at DESC, d.id LIMIT $3 OFFSET $4`, riderID, activeOnly, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []RiderJob{}
	for rows.Next() {
		var job RiderJob
		var number int64
		var address []byte
		d, err := scan(rows, &number, &job.OrderStatus, &job.CustomerName, &address, &job.TotalKobo, &job.PaymentMethod, &job.PaymentStatus)
		if err != nil {
			return nil, 0, err
		}
		job.Delivery, job.OrderNumber, job.Address, job.Currency = d, "SATI-"+strconv.FormatInt(number, 10), address, Currency
		if job.PaymentMethod == "PAY_ON_DELIVERY" && job.PaymentStatus != "PAID" && job.Status != StatusCancelled {
			job.CollectKobo = job.TotalKobo
		}
		out = append(out, job)
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
	mux.Handle("GET /api/v1/admin/delivery", access.Staff(http.HandlerFunc(h.listAdmin)))
	mux.Handle("GET /api/v1/rider/deliveries", access.Rider(http.HandlerFunc(h.listRider)))
}

func (h *Handler) listAdmin(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	f := AdminFilter{Status: strings.ToUpper(r.URL.Query().Get("status")), RiderID: r.URL.Query().Get("rider_id"), Limit: limit, Offset: offset}
	if f.Status != "" && f.Status != StatusAssigned && f.Status != StatusPickedUp && f.Status != StatusDelivered &&
		f.Status != StatusFailed && f.Status != StatusCancelled {
		apierror.Write(w, validation("Unknown status filter"))
		return
	}
	if f.RiderID != "" && !httpx.ValidUUID(f.RiderID) {
		apierror.Write(w, validation("rider_id is not a valid id"))
		return
	}

	rows, total, err := h.repo.ListAdmin(r.Context(), h.pool, f)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"deliveries": rows, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) listRider(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	identity, _ := auth.IdentityFrom(r.Context())
	activeOnly := r.URL.Query().Get("status") != "all"

	jobs, total, err := h.repo.ListForRider(r.Context(), h.pool, identity.User.ID, activeOnly, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"deliveries": jobs, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Error("request failed", "request_id", middleware.RequestID(r.Context()), "error", err)
	apierror.Write(w, apierror.ErrInternal)
}
