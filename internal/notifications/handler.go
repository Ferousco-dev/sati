package notifications

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

type Handler struct {
	pool *pgxpool.Pool
	repo *Repository
	log  *slog.Logger
	now  func() time.Time
}

func NewHandler(pool *pgxpool.Pool, log *slog.Logger) *Handler {
	return &Handler{pool: pool, repo: NewRepository(), log: log, now: time.Now}
}

func (h *Handler) Mount(mux *http.ServeMux, access auth.Access) {
	mux.Handle("GET /api/v1/notifications", access.Customer(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/v1/notifications/unread-count", access.Customer(http.HandlerFunc(h.unreadCount)))
	mux.Handle("POST /api/v1/notifications/read-all", access.Customer(http.HandlerFunc(h.readAll)))
	mux.Handle("POST /api/v1/notifications/{id}/read", access.Customer(http.HandlerFunc(h.read)))
}

func userID(r *http.Request) string {
	id, _ := auth.IdentityFrom(r.Context())
	return id.User.ID
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, perr := httpx.Pagination(r)
	if perr != nil {
		apierror.Write(w, perr)
		return
	}
	items, total, err := h.repo.List(r.Context(), h.pool, userID(r), r.URL.Query().Get("unread") == "true", limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	unread, err := h.repo.UnreadCount(r.Context(), h.pool, userID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{
		"notifications": items, "total": total, "unread_count": unread, "limit": limit, "offset": offset,
	})
}

func (h *Handler) unreadCount(w http.ResponseWriter, r *http.Request) {
	n, err := h.repo.UnreadCount(r.Context(), h.pool, userID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]int{"unread_count": n})
}

func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		apierror.Write(w, notFound)
		return
	}
	if err := h.repo.MarkRead(r.Context(), h.pool, userID(r), id, h.now()); err != nil {
		if errors.Is(err, errNotFound) {
			apierror.Write(w, notFound)
			return
		}
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) readAll(w http.ResponseWriter, r *http.Request) {
	n, err := h.repo.MarkAllRead(r.Context(), h.pool, userID(r), h.now())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]int64{"marked_read": n})
}

var notFound = apierror.New(http.StatusNotFound, "NOTIFICATION_NOT_FOUND", "Notification not found")

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Error("request failed", "request_id", middleware.RequestID(r.Context()), "error", err)
	apierror.Write(w, apierror.ErrInternal)
}
