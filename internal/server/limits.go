package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

// rateLimitPolicies is the one place that says how often each sensitive route may be called.
// Every policy matching a request is applied, so the general limit also covers the specific ones.
func rateLimitPolicies() []middleware.Policy {
	return []middleware.Policy{
		{Name: "general", Limit: 600, Window: time.Minute},

		{Name: "register", Method: http.MethodPost, Path: "/api/v1/auth/register", Limit: 10, Window: time.Hour},
		{Name: "login", Method: http.MethodPost, Path: "/api/v1/auth/login", Limit: 20, Window: 15 * time.Minute},
		{Name: "forgot-password", Method: http.MethodPost, Path: "/api/v1/auth/forgot-password", Limit: 5, Window: time.Hour},
		{Name: "reset-password", Method: http.MethodPost, Path: "/api/v1/auth/reset-password", Limit: 10, Window: 15 * time.Minute},
		{Name: "verify-email", Method: http.MethodPost, Path: "/api/v1/auth/verify-email", Limit: 10, Window: 15 * time.Minute},
		{Name: "resend-verification", Method: http.MethodPost, Path: "/api/v1/auth/resend-verification", Limit: 3, Window: 10 * time.Minute, BySession: true},

		{Name: "create-order", Method: http.MethodPost, Path: "/api/v1/orders", Limit: 30, Window: time.Hour, BySession: true},
		{Name: "payment-initialize", Method: http.MethodPost, Path: "/api/v1/payments/initialize", Limit: 20, Window: 10 * time.Minute, BySession: true},
		{Name: "payment-verify", Method: http.MethodPost, Path: "/api/v1/payments/verify", Limit: 40, Window: 10 * time.Minute, BySession: true},
		{Name: "webhook", Method: http.MethodPost, Path: "/api/v1/webhooks/paystack", Limit: 300, Window: time.Minute},
	}
}

// skipRateLimit leaves out the routes that load balancers, scrapers and payment
// providers call on a schedule, and static files that a page loads in bulk.
func skipRateLimit(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/health", "/api/v1/ready", "/metrics":
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/api/v1/files/")
}
