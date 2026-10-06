package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

var (
	ErrOwnRole  = apierror.New(http.StatusBadRequest, "CANNOT_CHANGE_OWN_ROLE", "You cannot change your own role")
	ErrNoUser   = apierror.New(http.StatusNotFound, "USER_NOT_FOUND", "User not found")
	ErrBadRole  = apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "Role must be CUSTOMER, STAFF, ADMIN or RIDER")
	ErrBadQuery = apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "Unknown role filter")
)

type Service struct {
	pool  *pgxpool.Pool
	users *users.Repository
	auth  *auth.Service
	now   func() time.Time
}

func NewService(pool *pgxpool.Pool, authSvc *auth.Service) *Service {
	return &Service{pool: pool, users: users.NewRepository(), auth: authSvc, now: time.Now}
}

type UserPage struct {
	Users  []users.User `json:"users"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

func (s *Service) ListUsers(ctx context.Context, role users.Role, limit, offset int) (UserPage, error) {
	if role != "" && !role.Valid() {
		return UserPage{}, ErrBadQuery
	}
	list, total, err := s.users.List(ctx, s.pool, role, limit, offset)
	return UserPage{Users: list, Total: total, Limit: limit, Offset: offset}, err
}

type Rider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Service) ListRiders(ctx context.Context) ([]Rider, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, name FROM users WHERE role = 'RIDER' ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Rider{}
	for rows.Next() {
		var r Rider
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) Invite(ctx context.Context, name, email string, role users.Role) (users.User, error) {
	return s.auth.Invite(ctx, name, email, role)
}

func (s *Service) ChangeRole(ctx context.Context, actorID, targetID string, role users.Role) (users.User, error) {
	if !role.Valid() {
		return users.User{}, ErrBadRole
	}
	if !httpx.ValidUUID(targetID) {
		return users.User{}, ErrNoUser
	}
	if actorID == targetID {
		return users.User{}, ErrOwnRole
	}

	if _, err := s.users.ByID(ctx, s.pool, targetID); errors.Is(err, users.ErrNotFound) {
		return users.User{}, ErrNoUser
	} else if err != nil {
		return users.User{}, err
	}
	if err := s.users.SetRole(ctx, s.pool, targetID, role, s.now()); err != nil {
		return users.User{}, err
	}
	return s.users.ByID(ctx, s.pool, targetID)
}

type Customer struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	EmailVerified  bool      `json:"email_verified"`
	OrdersCount    int       `json:"orders_count"`
	TotalSpentKobo int64     `json:"total_spent_kobo"`
	CreatedAt      time.Time `json:"created_at"`
}

type CustomerPage struct {
	Customers []Customer `json:"customers"`
	Total     int        `json:"total"`
	Limit     int        `json:"limit"`
	Offset    int        `json:"offset"`
}

func (s *Service) ListCustomers(ctx context.Context, query string, limit, offset int) (CustomerPage, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > 100 {
		return CustomerPage{}, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "Search text is too long")
	}
	pattern := ""
	if query != "" {
		pattern = "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query) + "%"
	}
	where := `u.role = 'CUSTOMER' AND ($1 = '' OR u.name ILIKE $1 OR u.email::text ILIKE $1)`

	page := CustomerPage{Customers: []Customer{}, Limit: limit, Offset: offset}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users u WHERE `+where, pattern).Scan(&page.Total); err != nil {
		return CustomerPage{}, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT u.id::text, u.name, u.email::text, u.email_verified, u.created_at,
		        count(o.id), COALESCE(sum(o.total_kobo) FILTER (WHERE o.payment_status = 'PAID'), 0)
		 FROM users u LEFT JOIN orders o ON o.customer_id = u.id
		 WHERE `+where+` GROUP BY u.id ORDER BY u.created_at DESC, u.id LIMIT $2 OFFSET $3`, pattern, limit, offset)
	if err != nil {
		return CustomerPage{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.EmailVerified, &c.CreatedAt, &c.OrdersCount, &c.TotalSpentKobo); err != nil {
			return CustomerPage{}, err
		}
		page.Customers = append(page.Customers, c)
	}
	return page, rows.Err()
}

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) Mount(mux *http.ServeMux, access auth.Access) {
	mux.Handle("GET /api/v1/admin/riders", access.Staff(http.HandlerFunc(h.riders)))
	mux.Handle("GET /api/v1/admin/customers", access.Admin(http.HandlerFunc(h.customers)))
	mux.Handle("GET /api/v1/admin/users", access.Admin(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/admin/users", access.Admin(http.HandlerFunc(h.invite)))
	mux.Handle("PATCH /api/v1/admin/users/{id}/role", access.Admin(http.HandlerFunc(h.changeRole)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	page, err := h.svc.ListUsers(r.Context(), users.Role(r.URL.Query().Get("role")), limit, offset)
	h.respond(w, r, http.StatusOK, page, err)
}

func (h *Handler) customers(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	page, err := h.svc.ListCustomers(r.Context(), r.URL.Query().Get("q"), limit, offset)
	h.respond(w, r, http.StatusOK, page, err)
}

func (h *Handler) riders(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListRiders(r.Context())
	h.respond(w, r, http.StatusOK, map[string]any{"riders": list}, err)
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string     `json:"name"`
		Email string     `json:"email"`
		Role  users.Role `json:"role"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	u, err := h.svc.Invite(r.Context(), in.Name, in.Email, in.Role)
	h.respond(w, r, http.StatusCreated, map[string]any{"user": u}, err)
}

func (h *Handler) changeRole(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role users.Role `json:"role"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	actor, _ := auth.IdentityFrom(r.Context())
	u, err := h.svc.ChangeRole(r.Context(), actor.User.ID, r.PathValue("id"), in.Role)
	h.respond(w, r, http.StatusOK, map[string]any{"user": u}, err)
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, status int, body any, err error) {
	if err == nil {
		apierror.JSON(w, status, body)
		return
	}
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		apierror.Write(w, apiErr)
		return
	}
	h.log.Error("request failed", "request_id", middleware.RequestID(r.Context()), "error", err)
	apierror.Write(w, apierror.ErrInternal)
}
