package server

import (
	"log/slog"
	"net/http"

	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

type Handlers struct {
	Health *health.Handler
	Auth   *auth.Handler
}

func New(cfg config.Config, log *slog.Logger, h Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", h.Health.Check)
	h.Auth.Mount(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		apierror.Write(w, apierror.ErrNotFound)
	})

	return middleware.Chain(mux,
		middleware.WithRequestID(),
		middleware.WithClientIP(cfg.TrustedProxies),
		middleware.Logger(log),
		middleware.Recover(log),
		middleware.CORS(cfg.CORSAllowedOrigins),
		middleware.OriginCheck(cfg.CORSAllowedOrigins),
	)
}
