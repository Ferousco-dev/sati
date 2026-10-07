package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/internal/admin"
	"github.com/Ferousco-dev/sati/backend/internal/analytics"
	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/categories"
	"github.com/Ferousco-dev/sati/backend/internal/cms"
	"github.com/Ferousco-dev/sati/backend/internal/delivery"
	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/internal/notifications"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/payments"
	"github.com/Ferousco-dev/sati/backend/internal/products"
	"github.com/Ferousco-dev/sati/backend/internal/store"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/ratelimit"
)

type failingLimiter struct{}

func (failingLimiter) Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error) {
	return ratelimit.Result{}, errors.New("redis is down")
}

// bareApp builds the server without a database, for tests of the shared plumbing.
func bareApp(ping error, limiter ratelimit.Limiter, metricsToken string, log *slog.Logger, production bool) *App {
	cfg := config.Config{CORSAllowedOrigins: []string{"http://localhost:3000"}}
	if production {
		cfg.AppEnv = "production"
	}
	return New(cfg, log, Handlers{
		Limiter: limiter, MetricsToken: metricsToken,
		Health:        health.New(fakePinger{err: ping}),
		Auth:          auth.NewHandler(nil, log, false),
		Categories:    categories.NewHandler(nil, log),
		Products:      products.NewHandler(nil, log),
		Admin:         admin.NewHandler(nil, log),
		Addresses:     addresses.NewHandler(nil, log),
		Orders:        orders.NewHandler(nil, log),
		Store:         store.NewHandler(nil, log),
		CMS:           cms.NewHandler(nil, log),
		Payments:      payments.NewHandler(nil, log),
		Analytics:     analytics.NewHandler(nil, log),
		Notifications: notifications.NewHandler(nil, log),
		Zones:         delivery.NewZoneHandler(nil, log),
		Deliveries:    delivery.NewHandler(nil, log),
	})
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func serve(app http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func TestSecurityHeaders(t *testing.T) {
	rec := serve(bareApp(nil, nil, "", quietLog(), false), httptest.NewRequest("GET", "/api/v1/health", nil))
	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	}
	for h, v := range want {
		if got := rec.Header().Get(h); got != v {
			t.Errorf("%s = %q, want %q", h, got, v)
		}
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS must not be sent in development")
	}

	prod := serve(bareApp(nil, nil, "", quietLog(), true), httptest.NewRequest("GET", "/api/v1/health", nil))
	if !strings.Contains(prod.Header().Get("Strict-Transport-Security"), "max-age=31536000") {
		t.Fatal("production must send HSTS")
	}
}

func TestUnknownRouteHasItsOwnCode(t *testing.T) {
	rec := serve(bareApp(nil, nil, "", quietLog(), false), httptest.NewRequest("GET", "/api/v1/nothing-here", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "ROUTE_NOT_FOUND") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestReadiness(t *testing.T) {
	ok := serve(bareApp(nil, nil, "", quietLog(), false), httptest.NewRequest("GET", "/api/v1/ready", nil))
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), `"database":"ok"`) || !strings.Contains(ok.Body.String(), `"redis":"not_configured"`) {
		t.Fatalf("ready: %d %s", ok.Code, ok.Body)
	}
	down := serve(bareApp(errors.New("db down"), nil, "", quietLog(), false), httptest.NewRequest("GET", "/api/v1/ready", nil))
	if down.Code != http.StatusServiceUnavailable || !strings.Contains(down.Body.String(), `"unavailable"`) {
		t.Fatalf("db down: %d %s", down.Code, down.Body)
	}
}

func TestReadinessReportsRedisAsDegradedNotDown(t *testing.T) {
	h := health.New(fakePinger{}).WithRedis(fakePinger{err: errors.New("redis down")})
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest("GET", "/api/v1/ready", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"degraded"`) || !strings.Contains(rec.Body.String(), `"redis":"down"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	if rec := serve(bareApp(nil, nil, "", quietLog(), false), httptest.NewRequest("GET", "/metrics", nil)); rec.Code != http.StatusNotFound {
		t.Fatalf("without a token the endpoint must not exist: %d", rec.Code)
	}

	app := bareApp(nil, nil, "s3cret", quietLog(), false)
	serve(app, httptest.NewRequest("GET", "/api/v1/health", nil))

	for name, header := range map[string]string{"none": "", "wrong": "Bearer nope", "no scheme": "s3cret"} {
		req := httptest.NewRequest("GET", "/metrics", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if rec := serve(app, req); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d", name, rec.Code)
		}
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	rec := serve(app, req)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("scrape: %d %s", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`sati_http_requests_total{method="GET",route="GET /api/v1/health",status="200"}`,
		"sati_http_request_duration_seconds_bucket",
		"sati_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
	if strings.Contains(body, `route="GET /metrics"`) && strings.Contains(body, "s3cret") {
		t.Fatal("the token must never appear in metrics")
	}
}

func TestMetricsUseRoutePatternsNotRawPaths(t *testing.T) {
	app := bareApp(nil, nil, "t", quietLog(), false)
	for i := 0; i < 3; i++ {
		serve(app, httptest.NewRequest("GET", fmt.Sprintf("/api/v1/products/00000000-0000-0000-0000-00000000000%d", i), nil))
		serve(app, httptest.NewRequest("GET", fmt.Sprintf("/api/v1/random-%d", i), nil))
	}
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer t")
	body := serve(app, req).Body.String()

	if strings.Contains(body, "random-") || strings.Contains(body, "00000000-0000") {
		t.Fatal("raw paths would create unlimited metric series")
	}
	if !strings.Contains(body, `route="unmatched"`) || !strings.Contains(body, `route="GET /api/v1/products/{id}"`) {
		t.Fatalf("route labels missing:\n%s", body)
	}
}

func TestLogsNeverContainSecrets(t *testing.T) {
	var buf bytes.Buffer
	app := bareApp(nil, nil, "", slog.New(slog.NewTextHandler(&buf, nil)), false)

	req := httptest.NewRequest("POST", "/api/v1/nope?x=1", strings.NewReader(`{"password":"hunter2-password"}`))
	req.Header.Set("Cookie", "sati_session=SESSION-SECRET-VALUE")
	req.Header.Set("Authorization", "Bearer BEARER-SECRET-VALUE")
	serve(app, req)

	logs := buf.String()
	if !strings.Contains(logs, "request_id=") {
		t.Fatal("requests must be logged with an id")
	}
	for _, secret := range []string{"hunter2-password", "SESSION-SECRET-VALUE", "BEARER-SECRET-VALUE"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("log contains %q:\n%s", secret, logs)
		}
	}
}

func TestRateLimitFailsOpen(t *testing.T) {
	app := bareApp(nil, failingLimiter{}, "", quietLog(), false)
	for i := 0; i < 30; i++ {
		if rec := serve(app, httptest.NewRequest("GET", "/api/v1/nothing", nil)); rec.Code == http.StatusTooManyRequests {
			t.Fatal("a broken limiter must not block traffic")
		}
	}
}

func TestHealthChecksAreNeverRateLimited(t *testing.T) {
	app := bareApp(nil, ratelimit.NewMemory(), "", quietLog(), false)
	for i := 0; i < 700; i++ {
		if rec := serve(app, httptest.NewRequest("GET", "/api/v1/health", nil)); rec.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
}

func TestGeneralRateLimit(t *testing.T) {
	app := bareApp(nil, ratelimit.NewMemory(), "", quietLog(), false)
	var limited *httptest.ResponseRecorder
	for i := 0; i < 700; i++ {
		rec := serve(app, httptest.NewRequest("GET", "/api/v1/nothing", nil))
		if rec.Code == http.StatusTooManyRequests {
			limited = rec
			break
		}
	}
	if limited == nil {
		t.Fatal("600 requests a minute from one address must be the ceiling")
	}
	if limited.Header().Get("Retry-After") == "" || !strings.Contains(limited.Body.String(), "RATE_LIMITED") {
		t.Fatalf("429 must say when to retry: %v %s", limited.Header(), limited.Body)
	}

	req := httptest.NewRequest("GET", "/api/v1/nothing", nil)
	req.RemoteAddr = "198.51.100.9:4000"
	if rec := serve(app, req); rec.Code == http.StatusTooManyRequests {
		t.Fatal("another address must have its own allowance")
	}
}

func fromIP(method, path string, body any, ip string) *http.Request {
	b := &bytes.Buffer{}
	if body != nil {
		fmt.Fprint(b, body)
	}
	req := httptest.NewRequest(method, path, b)
	req.RemoteAddr = ip + ":5000"
	return req
}

func TestSensitiveRoutesAreLimitedPerAddress(t *testing.T) {
	e := newCatalogEnv(t)
	body := `{"email":"nobody@example.com"}`

	for i := 1; i <= 5; i++ {
		if rec := serve(e.srv, fromIP("POST", "/api/v1/auth/forgot-password", body, "203.0.113.5")); rec.Code != http.StatusAccepted {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
	rec := serve(e.srv, fromIP("POST", "/api/v1/auth/forgot-password", body, "203.0.113.5"))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("sixth request: %d %v", rec.Code, rec.Header())
	}
	if rec := serve(e.srv, fromIP("POST", "/api/v1/auth/forgot-password", body, "203.0.113.6")); rec.Code != http.StatusAccepted {
		t.Fatalf("a different address must not be affected: %d", rec.Code)
	}
}

func TestLoginIsLimitedPerAccountAcrossAddresses(t *testing.T) {
	e := newCatalogEnv(t)
	e.signIn(t, "target@example.com", "CUSTOMER")
	body := `{"email":"target@example.com","password":"wrong-password"}`

	for i := 0; i < 8; i++ {
		ip := fmt.Sprintf("203.0.113.%d", 10+i)
		if rec := serve(e.srv, fromIP("POST", "/api/v1/auth/login", body, ip)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, rec.Code)
		}
	}
	rec := serve(e.srv, fromIP("POST", "/api/v1/auth/login", body, "203.0.113.99"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("a ninth guess from a new address must be blocked: %d", rec.Code)
	}
	right := `{"email":"target@example.com","password":"correct-horse"}`
	if rec := serve(e.srv, fromIP("POST", "/api/v1/auth/login", right, "203.0.113.100")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("even the right password waits out the limit: %d", rec.Code)
	}
	if rec := serve(e.srv, fromIP("POST", "/api/v1/auth/login", `{"email":"someone-else@example.com","password":"x"}`, "203.0.113.101")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("other accounts are unaffected: %d", rec.Code)
	}
}

func TestResendVerificationIsLimitedPerSession(t *testing.T) {
	e := newCatalogEnv(t)
	a := e.signIn(t, "a@example.com", "CUSTOMER")
	b := e.signIn(t, "b@example.com", "CUSTOMER")

	// The service's own 60 second cooldown also answers 429, so compare the error codes.
	codes := func(c *http.Cookie) []string {
		var out []string
		for i := 0; i < 4; i++ {
			out = append(out, errorCodeOf(t, e.do(t, "POST", "/api/v1/auth/resend-verification", nil, c)))
		}
		return out
	}
	for name, c := range map[string]*http.Cookie{"first session": a, "second session": b} {
		got := codes(c)
		if got[0] == "RATE_LIMITED" || got[2] == "RATE_LIMITED" || got[3] != "RATE_LIMITED" {
			t.Fatalf("%s: three requests are allowed and the fourth is limited: %v", name, got)
		}
	}
}

func TestRateLimitIsCountedInMetrics(t *testing.T) {
	e := newCatalogEnv(t)
	for i := 0; i < 6; i++ {
		serve(e.srv, fromIP("POST", "/api/v1/auth/forgot-password", `{"email":"x@example.com"}`, "203.0.113.50"))
	}
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer metrics-secret")
	if body := serve(e.srv, req).Body.String(); !strings.Contains(body, `sati_rate_limited_total{policy="forgot-password"}`) {
		t.Fatal("rate limit rejections must be counted")
	}
}

func TestOriginNullIsRejected(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", "null")
	if rec := serve(bareApp(nil, nil, "", quietLog(), false), req); rec.Code != http.StatusForbidden {
		t.Fatalf("sandboxed pages send Origin: null and must be blocked: %d", rec.Code)
	}
}
