package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/internal/admin"
	"github.com/Ferousco-dev/sati/backend/internal/analytics"
	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/categories"
	"github.com/Ferousco-dev/sati/backend/internal/delivery"
	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/payments"
	"github.com/Ferousco-dev/sati/backend/internal/products"
	"github.com/Ferousco-dev/sati/backend/internal/store"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func newTestServer(pingErr error) http.Handler {
	cfg := config.Config{CORSAllowedOrigins: []string{"http://localhost:3000"}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := Handlers{
		Health:     health.New(fakePinger{err: pingErr}),
		Auth:       auth.NewHandler(nil, log, false),
		Categories: categories.NewHandler(nil, log),
		Products:   products.NewHandler(nil, log),
		Admin:      admin.NewHandler(nil, log),
		Addresses:  addresses.NewHandler(nil, log),
		Orders:     orders.NewHandler(nil, log),
		Store:      store.NewHandler(nil, log),
		Payments:   payments.NewHandler(nil, log),
		Analytics:  analytics.NewHandler(nil, log),
		Zones:      delivery.NewZoneHandler(nil, log),
		Deliveries: delivery.NewHandler(nil, log),
	}
	return New(cfg, log, handlers)
}

func TestHealth(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"database up", nil, http.StatusOK},
		{"database down", errors.New("down"), http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestServer(tt.err).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/health", nil))
			if rec.Code != tt.want {
				t.Fatalf("got %d, want %d", rec.Code, tt.want)
			}
			if rec.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing request id")
			}
		})
	}
}

func TestUnknownRouteUsesErrorFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(nil).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
	if body := rec.Body.String(); body == "" || body[0] != '{' {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestCORS(t *testing.T) {
	req := httptest.NewRequest("OPTIONS", "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	newTestServer(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatal("origin not allowed")
	}

	req = httptest.NewRequest("GET", "/api/v1/health", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec = httptest.NewRecorder()
	newTestServer(nil).ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unexpected CORS header for unknown origin")
	}
}

func TestOriginCheck(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   int
	}{
		{"allowed origin", "http://localhost:3000", http.StatusBadRequest},
		{"foreign origin", "http://evil.example", http.StatusForbidden},
		{"no origin", "", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/auth/verify-email", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			newTestServer(nil).ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("got %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
