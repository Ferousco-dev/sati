package delivery

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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
	Currency   = "NGN"
	maxFeeKobo = 100_000_000
)

type Zone struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	FeeKobo   int64     `json:"fee_kobo"`
	Currency  string    `json:"currency"`
	Active    bool      `json:"active"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ZoneInput struct {
	Name      string `json:"name"`
	FeeKobo   *int64 `json:"fee_kobo"`
	SortOrder int    `json:"sort_order"`
}

type ZoneUpdate struct {
	Name      *string `json:"name"`
	FeeKobo   *int64  `json:"fee_kobo"`
	Active    *bool   `json:"active"`
	SortOrder *int    `json:"sort_order"`
}

var (
	ErrZoneNotFound  = apierror.New(http.StatusNotFound, "ZONE_NOT_FOUND", "Delivery zone not found")
	ErrZoneNameTaken = apierror.New(http.StatusConflict, "ZONE_NAME_TAKEN", "A zone with this name already exists")
)

const zoneColumns = `id::text, name, fee_kobo, active, sort_order, created_at, updated_at`

type ZoneRepository struct{}

func NewZoneRepository() *ZoneRepository { return &ZoneRepository{} }

func scanZone(row pgx.Row) (Zone, error) {
	var z Zone
	err := row.Scan(&z.ID, &z.Name, &z.FeeKobo, &z.Active, &z.SortOrder, &z.CreatedAt, &z.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Zone{}, ErrZoneNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Zone{}, ErrZoneNameTaken
	}
	z.Currency = Currency
	return z, err
}

func (ZoneRepository) List(ctx context.Context, db database.DBTX, onlyActive bool) ([]Zone, error) {
	rows, err := db.Query(ctx,
		`SELECT `+zoneColumns+` FROM delivery_zones WHERE (NOT $1 OR active) ORDER BY sort_order, name`, onlyActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Zone{}
	for rows.Next() {
		z, err := scanZone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

// GetShared reads a zone with a shared lock so its fee cannot change before an order commits.
func (ZoneRepository) GetShared(ctx context.Context, db database.DBTX, id string) (Zone, error) {
	if !httpx.ValidUUID(id) {
		return Zone{}, ErrZoneNotFound
	}
	return scanZone(db.QueryRow(ctx, `SELECT `+zoneColumns+` FROM delivery_zones WHERE id = $1 FOR SHARE`, id))
}

func (ZoneRepository) CountActive(ctx context.Context, db database.DBTX) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM delivery_zones WHERE active`).Scan(&n)
	return n, err
}

func (ZoneRepository) Create(ctx context.Context, db database.DBTX, name string, fee int64, sort int) (Zone, error) {
	return scanZone(db.QueryRow(ctx,
		`INSERT INTO delivery_zones (name, fee_kobo, sort_order) VALUES ($1, $2, $3) RETURNING `+zoneColumns, name, fee, sort))
}

func (ZoneRepository) Update(ctx context.Context, db database.DBTX, id string, in ZoneUpdate, now time.Time) (Zone, error) {
	return scanZone(db.QueryRow(ctx,
		`UPDATE delivery_zones SET
		   name = COALESCE($2, name), fee_kobo = COALESCE($3, fee_kobo),
		   active = COALESCE($4, active), sort_order = COALESCE($5, sort_order), updated_at = $6
		 WHERE id = $1 RETURNING `+zoneColumns, id, in.Name, in.FeeKobo, in.Active, in.SortOrder, now))
}

type ZoneHandler struct {
	pool *pgxpool.Pool
	repo *ZoneRepository
	log  *slog.Logger
	now  func() time.Time
}

func NewZoneHandler(pool *pgxpool.Pool, log *slog.Logger) *ZoneHandler {
	return &ZoneHandler{pool: pool, repo: NewZoneRepository(), log: log, now: time.Now}
}

func (h *ZoneHandler) Mount(mux *http.ServeMux, access auth.Access) {
	cached := middleware.PublicCache(30*time.Second, time.Minute)
	mux.Handle("GET /api/v1/delivery/zones", cached(http.HandlerFunc(h.listPublic)))

	mux.Handle("GET /api/v1/admin/delivery/zones", access.Staff(http.HandlerFunc(h.listAll)))
	mux.Handle("POST /api/v1/admin/delivery/zones", access.Admin(http.HandlerFunc(h.create)))
	mux.Handle("PATCH /api/v1/admin/delivery/zones/{id}", access.Admin(http.HandlerFunc(h.update)))
}

func (h *ZoneHandler) listPublic(w http.ResponseWriter, r *http.Request) {
	list, err := h.repo.List(r.Context(), h.pool, true)
	h.respond(w, r, http.StatusOK, map[string]any{"zones": list}, err)
}

func (h *ZoneHandler) listAll(w http.ResponseWriter, r *http.Request) {
	list, err := h.repo.List(r.Context(), h.pool, false)
	h.respond(w, r, http.StatusOK, map[string]any{"zones": list}, err)
}

func (h *ZoneHandler) create(w http.ResponseWriter, r *http.Request) {
	var in ZoneInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if err := checkName(in.Name); err != nil {
		apierror.Write(w, err)
		return
	}
	if in.FeeKobo == nil {
		apierror.Write(w, validation("fee_kobo is required"))
		return
	}
	if err := checkFee(*in.FeeKobo); err != nil {
		apierror.Write(w, err)
		return
	}
	z, err := h.repo.Create(r.Context(), h.pool, in.Name, *in.FeeKobo, in.SortOrder)
	h.respond(w, r, http.StatusCreated, map[string]any{"zone": z}, err)
}

func (h *ZoneHandler) update(w http.ResponseWriter, r *http.Request) {
	var in ZoneUpdate
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	if in.Name == nil && in.FeeKobo == nil && in.Active == nil && in.SortOrder == nil {
		apierror.Write(w, validation("No fields to update"))
		return
	}
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if err := checkName(trimmed); err != nil {
			apierror.Write(w, err)
			return
		}
	}
	if in.FeeKobo != nil {
		if err := checkFee(*in.FeeKobo); err != nil {
			apierror.Write(w, err)
			return
		}
	}
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		apierror.Write(w, ErrZoneNotFound)
		return
	}
	z, err := h.repo.Update(r.Context(), h.pool, id, in, h.now())
	h.respond(w, r, http.StatusOK, map[string]any{"zone": z}, err)
}

func validation(msg string) *apierror.Error {
	return apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", msg)
}

func checkName(name string) *apierror.Error {
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		return validation("Name must be between 1 and 100 characters")
	}
	return nil
}

func checkFee(fee int64) *apierror.Error {
	if fee < 0 || fee > maxFeeKobo {
		return validation("fee_kobo must be between 0 and 100000000")
	}
	return nil
}

func (h *ZoneHandler) respond(w http.ResponseWriter, r *http.Request, status int, body any, err error) {
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
