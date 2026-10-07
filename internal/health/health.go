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
	db    Pinger
	redis Pinger
}

func New(db Pinger) *Handler {
	return &Handler{db: db}
}

// WithRedis adds Redis to the readiness report. Redis is optional, so it never fails readiness.
func (h *Handler) WithRedis(redis Pinger) *Handler {
	h.redis = redis
	return h
}

// Ready tells a load balancer whether to send traffic. The database must be
// reachable. A Redis outage is reported as degraded but the API keeps serving.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{"database": "ok", "redis": "not_configured"}
	status, code := "ready", http.StatusOK

	if err := h.db.Ping(ctx); err != nil {
		checks["database"], status, code = "down", "unavailable", http.StatusServiceUnavailable
	}
	if h.redis != nil {
		if err := h.redis.Ping(ctx); err != nil {
			checks["redis"] = "down"
			if status == "ready" {
				status = "degraded"
			}
		} else {
			checks["redis"] = "ok"
		}
	}
	apierror.JSON(w, code, map[string]any{"status": status, "checks": checks})
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
