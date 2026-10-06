package server

import (
	"log/slog"
	"net/http"

	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

func New(cfg config.Config, log *slog.Logger, healthHandler *health.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", healthHandler.Check)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		apierror.Write(w, apierror.ErrNotFound)
	})

	return middleware.Chain(mux,
		middleware.WithRequestID(),
		middleware.WithClientIP(cfg.TrustedProxies),
		middleware.Logger(log),
		middleware.Recover(log),
		middleware.CORS(cfg.CORSAllowedOrigins),
	)
}
