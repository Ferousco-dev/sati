package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/netip"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/clientip"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	clientIPKey
)

type Middleware func(http.Handler) http.Handler

func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func RequestID(from context.Context) string {
	id, _ := from.Value(requestIDKey).(string)
	return id
}

func ClientIP(from context.Context) string {
	ip, _ := from.Value(clientIPKey).(string)
	return ip
}

func WithRequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := newID()
			w.Header().Set("X-Request-ID", id)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
		})
	}
}

func WithClientIP(trusted []netip.Prefix) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientip.Resolve(r, trusted)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey, ip)))
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func Logger(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.Info("request",
				"request_id", RequestID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"client_ip", ClientIP(r.Context()),
			)
		})
	}
}

func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic",
						"request_id", RequestID(r.Context()),
						"panic", rec,
						"stack", string(debug.Stack()),
					)
					apierror.Write(w, apierror.ErrInternal)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func CORS(allowed []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !slices.Contains(allowed, origin) {
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Add("Vary", "Origin")

			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", strings.Join([]string{
					http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions,
				}, ", "))
				h.Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// OriginCheck blocks cross-site state-changing requests from browsers.
// Requests without an Origin header carry no ambient cookies and pass.
func OriginCheck(allowed []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			if origin != "" && !slices.Contains(allowed, origin) {
				apierror.Write(w, apierror.New(http.StatusForbidden, "CSRF_REJECTED", "Request origin is not allowed"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
