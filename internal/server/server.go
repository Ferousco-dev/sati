package server

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/internal/admin"
	"github.com/Ferousco-dev/sati/backend/internal/analytics"
	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/categories"
	"github.com/Ferousco-dev/sati/backend/internal/cms"
	"github.com/Ferousco-dev/sati/backend/internal/delivery"
	"github.com/Ferousco-dev/sati/backend/internal/files"
	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/internal/notifications"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/payments"
	"github.com/Ferousco-dev/sati/backend/internal/products"
	"github.com/Ferousco-dev/sati/backend/internal/store"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/metrics"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
	"github.com/Ferousco-dev/sati/backend/pkg/ratelimit"
	"github.com/Ferousco-dev/sati/backend/pkg/router"
)

type Handlers struct {
	Health        *health.Handler
	Auth          *auth.Handler
	Categories    *categories.Handler
	Products      *products.Handler
	Admin         *admin.Handler
	Addresses     *addresses.Handler
	Orders        *orders.Handler
	Store         *store.Handler
	CMS           *cms.Handler
	Payments      *payments.Handler
	Analytics     *analytics.Handler
	Notifications *notifications.Handler
	Zones         *delivery.ZoneHandler
	Deliveries    *delivery.Handler
	LocalFiles    http.Handler

	// Limiter counts requests for rate limiting. Nil switches rate limiting off.
	Limiter ratelimit.Limiter
	// OpenAPI is the API description served at /api/v1/openapi.yaml.
	OpenAPI []byte
	// MetricsToken protects GET /metrics. Empty leaves the endpoint out entirely.
	MetricsToken string
}

// App is the HTTP handler plus the list of routes it serves.
type App struct {
	http.Handler
	routes []string
}

func (a *App) Routes() []string { return a.routes }

func New(cfg config.Config, log *slog.Logger, h Handlers) *App {
	mux := router.New()

	mux.HandleFunc("GET /api/v1/health", h.Health.Check)
	mux.HandleFunc("GET /api/v1/ready", h.Health.Ready)
	if h.MetricsToken != "" {
		mux.HandleFunc("GET /metrics", metricsHandler(h.MetricsToken))
	}
	if len(h.OpenAPI) > 0 {
		spec := h.OpenAPI
		cached := middleware.PublicCache(5*time.Minute, time.Hour)
		mux.Handle("GET /api/v1/openapi.yaml", cached(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
			_, _ = w.Write(spec)
		})))
	}
	if h.Limiter != nil {
		h.Auth.WithLimiter(h.Limiter)
	}
	h.Auth.Mount(mux)

	access := h.Auth.Access()
	h.Categories.Mount(mux, access)
	h.Products.Mount(mux, access)
	h.Admin.Mount(mux, access)
	h.Addresses.Mount(mux, access)
	h.Orders.Mount(mux, access)
	h.Orders.MountStaff(mux, access)
	h.Payments.Mount(mux, access)
	h.Analytics.Mount(mux, access)
	h.Notifications.Mount(mux, access)
	h.Zones.Mount(mux, access)
	h.Deliveries.Mount(mux, access)
	h.Store.Mount(mux, access)
	h.CMS.Mount(mux, access)
	if h.LocalFiles != nil {
		mux.Handle("GET "+files.LocalURLPrefix, h.LocalFiles)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		apierror.Write(w, apierror.New(http.StatusNotFound, "ROUTE_NOT_FOUND", "No such route"))
	})

	handler := middleware.Chain(middleware.CaptureRoute(mux),
		middleware.WithRequestID(),
		middleware.WithClientIP(cfg.TrustedProxies),
		middleware.NoStore(),
		middleware.SecurityHeaders(cfg.IsProduction()),
		middleware.Logger(log),
		middleware.Recover(log),
		middleware.CORS(cfg.CORSAllowedOrigins),
		middleware.OriginCheck(cfg.CORSAllowedOrigins),
		middleware.RateLimit(middleware.RateLimitConfig{
			Limiter: h.Limiter, Log: log, CookieName: auth.CookieName,
			Policies: rateLimitPolicies(), Skip: skipRateLimit,
		}),
	)
	return &App{Handler: handler, routes: mux.Patterns()}
}

func metricsHandler(token string) http.HandlerFunc {
	want := []byte("Bearer " + token)
	return func(w http.ResponseWriter, r *http.Request) {
		got := []byte(strings.TrimSpace(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			apierror.Write(w, apierror.ErrUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_ = metrics.Default.WriteTo(context.Background(), w)
	}
}
