package orders

import (
	"errors"
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

func (h *Handler) Mount(mux *http.ServeMux, access auth.Access) {
	mux.Handle("POST /api/v1/orders", access.Customer(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/v1/orders", access.Customer(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/v1/orders/{id}", access.Customer(http.HandlerFunc(h.get)))
	mux.Handle("POST /api/v1/orders/{id}/cancel", access.Customer(http.HandlerFunc(h.cancel)))
	mux.Handle("GET /api/v1/orders/{id}/status", access.Customer(http.HandlerFunc(h.status)))
}

func customerID(r *http.Request) string {
	id, _ := auth.IdentityFrom(r.Context())
	return id.User.ID
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	order, created, err := h.svc.Create(r.Context(), customerID(r), in, r.Header.Get("Idempotency-Key"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	apierror.JSON(w, status, map[string]any{"order": order})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	page, err := h.svc.List(r.Context(), customerID(r), limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, page)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.Get(r.Context(), customerID(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"order": order})
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.Cancel(r.Context(), customerID(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"order": order})
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.Status(r.Context(), customerID(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, view)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		apierror.Write(w, apiErr)
		return
	}
	h.log.Error("request failed", "request_id", middleware.RequestID(r.Context()), "error", err)
	apierror.Write(w, apierror.ErrInternal)
}
