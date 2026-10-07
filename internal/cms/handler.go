package cms

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
	"github.com/Ferousco-dev/sati/backend/pkg/router"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

func (h *Handler) Mount(mux router.Mux, access auth.Access) {
	cached := middleware.PublicCache(time.Minute, 5*time.Minute)
	mux.Handle("GET /api/v1/content", cached(http.HandlerFunc(h.site)))

	mux.Handle("GET /api/v1/admin/content", access.Admin(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/v1/admin/content/preview", access.Admin(http.HandlerFunc(h.preview)))
	mux.Handle("GET /api/v1/admin/content/{key}", access.Admin(http.HandlerFunc(h.get)))
	mux.Handle("PUT /api/v1/admin/content/{key}/draft", access.Admin(http.HandlerFunc(h.saveDraft)))
	mux.Handle("DELETE /api/v1/admin/content/{key}/draft", access.Admin(http.HandlerFunc(h.discardDraft)))
	mux.Handle("POST /api/v1/admin/content/{key}/publish", access.Admin(http.HandlerFunc(h.publish)))
	mux.Handle("GET /api/v1/admin/content/{key}/versions", access.Admin(http.HandlerFunc(h.versions)))
	mux.Handle("POST /api/v1/admin/content/{key}/rollback", access.Admin(http.HandlerFunc(h.rollback)))

	mux.Handle("GET /api/v1/admin/media", access.Admin(http.HandlerFunc(h.listMedia)))
	mux.Handle("POST /api/v1/admin/media", access.Admin(http.HandlerFunc(h.upload)))
	mux.Handle("PATCH /api/v1/admin/media/{id}", access.Admin(http.HandlerFunc(h.updateMedia)))
	mux.Handle("DELETE /api/v1/admin/media/{id}", access.Admin(http.HandlerFunc(h.deleteMedia)))
}

func actorID(r *http.Request) string {
	id, _ := auth.IdentityFrom(r.Context())
	return id.User.ID
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

func (h *Handler) site(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Site(r.Context(), false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, c)
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Site(r.Context(), true)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, c)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"sections": list})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Section(r.Context(), r.PathValue("key"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"section": st})
}

func (h *Handler) saveDraft(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Data json.RawMessage `json:"data"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	if len(in.Data) == 0 {
		apierror.Write(w, validation("data is required"))
		return
	}
	st, err := h.svc.SaveDraft(r.Context(), actorID(r), r.PathValue("key"), in.Data)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"section": st})
}

func (h *Handler) discardDraft(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DiscardDraft(r.Context(), r.PathValue("key")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Note string `json:"note"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	st, err := h.svc.Publish(r.Context(), actorID(r), r.PathValue("key"), in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"section": st})
}

func (h *Handler) versions(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Versions(r.Context(), r.PathValue("key"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"versions": list})
}

func (h *Handler) rollback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version int    `json:"version"`
		Note    string `json:"note"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	st, err := h.svc.Rollback(r.Context(), actorID(r), r.PathValue("key"), in.Version, in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"section": st})
}

func (h *Handler) listMedia(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	items, total, err := h.svc.ListMedia(r.Context(), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"media": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		apierror.Write(w, validation(`A picture is required in the "file" field`))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxUploadBytes+1))
	if err != nil {
		apierror.Write(w, validation("Could not read the uploaded picture"))
		return
	}
	m, err := h.svc.Upload(r.Context(), actorID(r), header.Filename, r.FormValue("alt"), data)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusCreated, map[string]any{"media": m})
}

func (h *Handler) updateMedia(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AltText string `json:"alt_text"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		apierror.Write(w, err)
		return
	}
	m, err := h.svc.UpdateAlt(r.Context(), r.PathValue("id"), in.AltText)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"media": m})
}

func (h *Handler) deleteMedia(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteMedia(r.Context(), r.PathValue("id")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
