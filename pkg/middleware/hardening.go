package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/metrics"
	"github.com/Ferousco-dev/sati/backend/pkg/ratelimit"
)

// SecurityHeaders adds the headers every API response should carry.
// The API only returns JSON and images, so it can forbid everything else.
func SecurityHeaders(production bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			if production {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Policy limits how often one client may call a route.
type Policy struct {
	Name      string
	Method    string // empty matches every method
	Path      string // empty matches every path
	Limit     int
	Window    time.Duration
	BySession bool // count per signed-in session instead of per IP
}

func (p Policy) matches(r *http.Request) bool {
	return (p.Method == "" || p.Method == r.Method) && (p.Path == "" || p.Path == r.URL.Path)
}

type RateLimitConfig struct {
	Limiter    ratelimit.Limiter
	Log        *slog.Logger
	CookieName string
	Policies   []Policy
	Skip       func(r *http.Request) bool
}

// RateLimit applies every matching policy. If the limiter itself fails the
// request is allowed, because a broken counter must not take the API down.
func RateLimit(cfg RateLimitConfig) Middleware {
	return func(next http.Handler) http.Handler {
		if cfg.Limiter == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.Skip != nil && cfg.Skip(r) {
				next.ServeHTTP(w, r)
				return
			}
			for _, p := range cfg.Policies {
				if !p.matches(r) {
					continue
				}
				res, err := cfg.Limiter.Allow(r.Context(), "rl:"+p.Name+":"+subject(r, p, cfg.CookieName), p.Limit, p.Window)
				if err != nil {
					cfg.Log.Warn("rate limiter unavailable, allowing request", "policy", p.Name, "error", err)
					continue
				}
				if !res.Allowed {
					metrics.RateLimited.Inc(p.Name)
					w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(res.RetryAfter.Seconds()))))
					apierror.Write(w, apierror.ErrRateLimited)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func subject(r *http.Request, p Policy, cookieName string) string {
	if p.BySession {
		if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
			sum := sha256.Sum256([]byte(c.Value))
			return "s:" + hex.EncodeToString(sum[:8])
		}
	}
	return "ip:" + ClientIP(r.Context())
}
