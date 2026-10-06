package store

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

const maxDeliveryFeeKobo = 100_000_000

type Settings struct {
	DeliveryFeeKobo int64     `json:"delivery_fee_kobo"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (Repository) Get(ctx context.Context, db database.DBTX) (Settings, error) {
	var s Settings
	err := db.QueryRow(ctx, `SELECT delivery_fee_kobo, updated_at FROM store_settings`).Scan(&s.DeliveryFeeKobo, &s.UpdatedAt)
	return s, err
}

func (Repository) Update(ctx context.Context, db database.DBTX, deliveryFee *int64, now time.Time) (Settings, error) {
	var s Settings
	err := db.QueryRow(ctx,
		`UPDATE store_settings SET delivery_fee_kobo = COALESCE($1, delivery_fee_kobo), updated_at = $2
		 RETURNING delivery_fee_kobo, updated_at`, deliveryFee, now).Scan(&s.DeliveryFeeKobo, &s.UpdatedAt)
	return s, err
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
	mux.Handle("GET /api/v1/admin/settings", access.Staff(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /api/v1/admin/settings", access.Admin(http.HandlerFunc(h.update)))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.Get(r.Context(), h.pool)
	h.respond(w, r, map[string]any{"settings": s}, err)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeliveryFeeKobo *int64 `json:"delivery_fee_kobo"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	if in.DeliveryFeeKobo == nil {
		apierror.Write(w, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "No fields to update"))
		return
	}
	if *in.DeliveryFeeKobo < 0 || *in.DeliveryFeeKobo > maxDeliveryFeeKobo {
		apierror.Write(w, apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", "delivery_fee_kobo must be between 0 and 100000000"))
		return
	}
	s, err := h.repo.Update(r.Context(), h.pool, in.DeliveryFeeKobo, time.Now())
	h.respond(w, r, map[string]any{"settings": s}, err)
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, body any, err error) {
	if err == nil {
		apierror.JSON(w, http.StatusOK, body)
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
