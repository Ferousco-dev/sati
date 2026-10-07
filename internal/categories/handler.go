package categories

import (
	"errors"
	"github.com/Ferousco-dev/sati/backend/pkg/router"
	"log/slog"
	"net/http"
	"time"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) Mount(mux router.Mux, access auth.Access) {
	cached := middleware.PublicCache(time.Minute, 5*time.Minute)

	mux.Handle("GET /api/v1/categories", cached(http.HandlerFunc(h.listPublic)))

	mux.Handle("GET /api/v1/admin/categories", access.Staff(http.HandlerFunc(h.listAll)))
	mux.Handle("POST /api/v1/admin/categories", access.Admin(http.HandlerFunc(h.create)))
	mux.Handle("PATCH /api/v1/admin/categories/{id}", access.Admin(http.HandlerFunc(h.update)))
}

func (h *Handler) listPublic(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListPublic(r.Context())
	h.respond(w, r, http.StatusOK, map[string]any{"categories": list}, err)
}

func (h *Handler) listAll(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListAll(r.Context())
	h.respond(w, r, http.StatusOK, map[string]any{"categories": list}, err)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	c, err := h.svc.Create(r.Context(), in)
	h.respond(w, r, http.StatusCreated, map[string]any{"category": c}, err)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	c, err := h.svc.Update(r.Context(), r.PathValue("id"), in)
	h.respond(w, r, http.StatusOK, map[string]any{"category": c}, err)
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
