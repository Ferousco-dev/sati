package analytics

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

const Currency = "NGN"

var allStatuses = []string{
	"PENDING", "CONFIRMED", "PREPARING", "READY", "OUT_FOR_DELIVERY",
	"DELIVERED", "COMPLETED", "CANCELLED", "FAILED",
}

var allMethods = []string{"ONLINE", "BANK_TRANSFER", "PAY_ON_PICKUP", "PAY_ON_DELIVERY"}

type Today struct {
	OrdersPlaced    int              `json:"orders_placed"`
	ByStatus        map[string]int   `json:"by_status"`
	PaidOrders      int              `json:"paid_orders"`
	RevenueKobo     int64            `json:"revenue_kobo"`
	RevenueByMethod map[string]int64 `json:"revenue_by_method_kobo"`
}

type OpenOrders struct {
	Pending          int `json:"pending"`
	Confirmed        int `json:"confirmed"`
	Preparing        int `json:"preparing"`
	Ready            int `json:"ready"`
	OutForDelivery   int `json:"out_for_delivery"`
	AwaitingTransfer int `json:"awaiting_transfer"`
}

type Dashboard struct {
	Timezone    string     `json:"timezone"`
	Date        string     `json:"date"`
	PeriodStart time.Time  `json:"period_start"`
	PeriodEnd   time.Time  `json:"period_end"`
	Currency    string     `json:"currency"`
	Today       Today      `json:"today"`
	Open        OpenOrders `json:"open_orders"`
}

type Service struct {
	pool *pgxpool.Pool
	loc  *time.Location
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool, storeLocation *time.Location) *Service {
	return &Service{pool: pool, loc: storeLocation, now: time.Now}
}

// Dashboard summarises "today" in the given timezone, falling back to the store's.
func (s *Service) Dashboard(ctx context.Context, loc *time.Location) (Dashboard, error) {
	if loc == nil {
		loc = s.loc
	}
	now := s.now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)

	d := Dashboard{
		Timezone: loc.String(), Date: start.Format("2006-01-02"),
		PeriodStart: start.UTC(), PeriodEnd: end.UTC(), Currency: Currency,
		Today: Today{ByStatus: map[string]int{}, RevenueByMethod: map[string]int64{}},
	}
	for _, st := range allStatuses {
		d.Today.ByStatus[st] = 0
	}
	for _, m := range allMethods {
		d.Today.RevenueByMethod[m] = 0
	}

	rows, err := s.pool.Query(ctx,
		`SELECT status::text, count(*) FROM orders WHERE created_at >= $1 AND created_at < $2 GROUP BY status`, start, end)
	if err != nil {
		return Dashboard{}, err
	}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return Dashboard{}, err
		}
		d.Today.ByStatus[status] = n
		d.Today.OrdersPlaced += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Dashboard{}, err
	}

	rows, err = s.pool.Query(ctx,
		`SELECT method::text, count(*), COALESCE(sum(amount_kobo), 0)
		 FROM payments WHERE status = 'PAID' AND paid_at >= $1 AND paid_at < $2 GROUP BY method`, start, end)
	if err != nil {
		return Dashboard{}, err
	}
	for rows.Next() {
		var method string
		var n int
		var sum int64
		if err := rows.Scan(&method, &n, &sum); err != nil {
			rows.Close()
			return Dashboard{}, err
		}
		d.Today.RevenueByMethod[method] = sum
		d.Today.RevenueKobo += sum
		d.Today.PaidOrders += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Dashboard{}, err
	}

	err = s.pool.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE status = 'PENDING'),
		        count(*) FILTER (WHERE status = 'CONFIRMED'),
		        count(*) FILTER (WHERE status = 'PREPARING'),
		        count(*) FILTER (WHERE status = 'READY'),
		        count(*) FILTER (WHERE status = 'OUT_FOR_DELIVERY'),
		        count(*) FILTER (WHERE status = 'PENDING' AND payment_method = 'BANK_TRANSFER' AND payment_status = 'UNPAID')
		 FROM orders WHERE status IN ('PENDING', 'CONFIRMED', 'PREPARING', 'READY', 'OUT_FOR_DELIVERY')`).
		Scan(&d.Open.Pending, &d.Open.Confirmed, &d.Open.Preparing, &d.Open.Ready, &d.Open.OutForDelivery, &d.Open.AwaitingTransfer)
	return d, err
}

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) Mount(mux *http.ServeMux, access auth.Access) {
	mux.Handle("GET /api/v1/admin/dashboard", access.Staff(http.HandlerFunc(h.dashboard)))
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	var loc *time.Location
	if name := r.URL.Query().Get("tz"); name != "" {
		l, err := time.LoadLocation(name)
		if err != nil || name == "Local" {
			apierror.Write(w, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "tz must be an IANA timezone such as Africa/Lagos"))
			return
		}
		loc = l
	}

	d, err := h.svc.Dashboard(r.Context(), loc)
	if err != nil {
		var apiErr *apierror.Error
		if errors.As(err, &apiErr) {
			apierror.Write(w, apiErr)
			return
		}
		h.log.Error("dashboard", "request_id", middleware.RequestID(r.Context()), "error", err)
		apierror.Write(w, apierror.ErrInternal)
		return
	}
	apierror.JSON(w, http.StatusOK, d)
}
