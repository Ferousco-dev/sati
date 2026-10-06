package server

import (
	"log/slog"
	"net/http"

	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/internal/admin"
	"github.com/Ferousco-dev/sati/backend/internal/analytics"
	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/categories"
	"github.com/Ferousco-dev/sati/backend/internal/delivery"
	"github.com/Ferousco-dev/sati/backend/internal/files"
	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/payments"
	"github.com/Ferousco-dev/sati/backend/internal/products"
	"github.com/Ferousco-dev/sati/backend/internal/store"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

type Handlers struct {
	Health     *health.Handler
	Auth       *auth.Handler
	Categories *categories.Handler
	Products   *products.Handler
	Admin      *admin.Handler
	Addresses  *addresses.Handler
	Orders     *orders.Handler
	Store      *store.Handler
	Payments   *payments.Handler
	Analytics  *analytics.Handler
	Zones      *delivery.ZoneHandler
	Deliveries *delivery.Handler
	LocalFiles http.Handler
}

func New(cfg config.Config, log *slog.Logger, h Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", h.Health.Check)
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
	h.Zones.Mount(mux, access)
	h.Deliveries.Mount(mux, access)
	h.Store.Mount(mux, access)
	if h.LocalFiles != nil {
		mux.Handle("GET "+files.LocalURLPrefix, h.LocalFiles)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		apierror.Write(w, apierror.ErrNotFound)
	})

	return middleware.Chain(mux,
		middleware.WithRequestID(),
		middleware.WithClientIP(cfg.TrustedProxies),
		middleware.NoStore(),
		middleware.Logger(log),
		middleware.Recover(log),
		middleware.CORS(cfg.CORSAllowedOrigins),
		middleware.OriginCheck(cfg.CORSAllowedOrigins),
	)
}
