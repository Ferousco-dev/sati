package addresses

import (
	"errors"
	"github.com/Ferousco-dev/sati/backend/pkg/router"
	"log/slog"
	"net/http"

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
	mux.Handle("GET /api/v1/addresses", access.Customer(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/addresses", access.Customer(http.HandlerFunc(h.create)))
	mux.Handle("PATCH /api/v1/addresses/{id}", access.Customer(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /api/v1/addresses/{id}", access.Customer(http.HandlerFunc(h.delete)))
}

func userID(r *http.Request) string {
	id, _ := auth.IdentityFrom(r.Context())
	return id.User.ID
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context(), userID(r))
	h.respond(w, r, http.StatusOK, map[string]any{"addresses": list}, err)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	a, err := h.svc.Create(r.Context(), userID(r), in)
	h.respond(w, r, http.StatusCreated, map[string]any{"address": a}, err)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	a, err := h.svc.Update(r.Context(), userID(r), r.PathValue("id"), in)
	h.respond(w, r, http.StatusOK, map[string]any{"address": a}, err)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), userID(r), r.PathValue("id")); err != nil {
		h.respond(w, r, 0, nil, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
