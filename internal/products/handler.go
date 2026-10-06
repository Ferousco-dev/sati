package products

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) Mount(mux *http.ServeMux, admin middleware.Middleware) {
	mux.HandleFunc("GET /api/v1/products", h.listPublic)
	mux.HandleFunc("GET /api/v1/products/{id}", h.getPublic)
	mux.HandleFunc("GET /api/v1/categories/{id}/products", h.listByCategory)

	mux.Handle("GET /api/v1/admin/products", admin(http.HandlerFunc(h.listAdmin)))
	mux.Handle("POST /api/v1/admin/products", admin(http.HandlerFunc(h.create)))
	mux.Handle("PATCH /api/v1/admin/products/{id}", admin(http.HandlerFunc(h.update)))
	mux.Handle("POST /api/v1/admin/products/{id}/archive", admin(http.HandlerFunc(h.archive)))
	mux.Handle("POST /api/v1/admin/products/{id}/availability", admin(http.HandlerFunc(h.setAvailability)))
	mux.Handle("POST /api/v1/admin/products/{id}/images", admin(http.HandlerFunc(h.addImage)))
	mux.Handle("DELETE /api/v1/admin/products/{id}/images/{imageId}", admin(http.HandlerFunc(h.deleteImage)))
}

func (h *Handler) listPublic(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, r.URL.Query().Get("category_id"), true)
}

func (h *Handler) listAdmin(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, r.URL.Query().Get("category_id"), false)
}

func (h *Handler) listByCategory(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	page, err := h.svc.ListPublicInCategory(r.Context(), r.PathValue("id"), limit, offset)
	h.respond(w, r, http.StatusOK, page, err)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, categoryID string, public bool) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}

	var page Page
	var err error
	if public {
		page, err = h.svc.ListPublic(r.Context(), categoryID, limit, offset)
	} else {
		page, err = h.svc.ListAdmin(r.Context(), categoryID, limit, offset)
	}
	h.respond(w, r, http.StatusOK, page, err)
}

func (h *Handler) getPublic(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.GetPublic(r.Context(), r.PathValue("id"))
	h.respond(w, r, http.StatusOK, map[string]any{"product": p}, err)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	p, err := h.svc.Create(r.Context(), in)
	h.respond(w, r, http.StatusCreated, map[string]any{"product": p}, err)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	p, err := h.svc.Update(r.Context(), r.PathValue("id"), in)
	h.respond(w, r, http.StatusOK, map[string]any{"product": p}, err)
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Archive(r.Context(), r.PathValue("id"))
	h.respond(w, r, http.StatusOK, map[string]any{"product": p}, err)
}

func (h *Handler) setAvailability(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Available *bool `json:"available"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	if in.Available == nil {
		apierror.Write(w, validation("available is required"))
		return
	}
	p, err := h.svc.SetAvailability(r.Context(), r.PathValue("id"), *in.Available)
	h.respond(w, r, http.StatusOK, map[string]any{"product": p}, err)
}

func (h *Handler) addImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxImageBytes+(1<<20))
	file, _, err := r.FormFile("image")
	if err != nil {
		apierror.Write(w, validation("An image file is required in the \"image\" field"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxImageBytes+1))
	if err != nil {
		apierror.Write(w, validation("Could not read the uploaded image"))
		return
	}

	img, err := h.svc.AddImage(r.Context(), r.PathValue("id"), data)
	h.respond(w, r, http.StatusCreated, map[string]any{"image": img}, err)
}

func (h *Handler) deleteImage(w http.ResponseWriter, r *http.Request) {
	err := h.svc.DeleteImage(r.Context(), r.PathValue("id"), r.PathValue("imageId"))
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.respond(w, r, 0, nil, err)
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

func newKeyID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
