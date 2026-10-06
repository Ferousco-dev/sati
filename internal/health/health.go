package health

import (
	"context"
	"net/http"
	"time"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	db Pinger
}

func New(db Pinger) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		apierror.Write(w, apierror.ErrUnavailable)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
