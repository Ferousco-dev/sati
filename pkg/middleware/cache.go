package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// NoStore is the default for every API response, so nothing private
// is ever kept by a browser or shared cache unless a route opts in.
func NoStore() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	}
}

type bufferedResponse struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) WriteHeader(code int) { b.status = code }

func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }

// PublicCache lets browsers, phones and CDNs reuse a successful GET response.
// It adds an ETag and answers conditional requests with 304 Not Modified.
func PublicCache(maxAge, staleWhileRevalidate time.Duration) Middleware {
	policy := fmt.Sprintf("public, max-age=%d, stale-while-revalidate=%d",
		int(maxAge/time.Second), int(staleWhileRevalidate/time.Second))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}

			buf := &bufferedResponse{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(buf, r)

			h := w.Header()
			if buf.status != http.StatusOK {
				w.WriteHeader(buf.status)
				_, _ = w.Write(buf.body.Bytes())
				return
			}

			sum := sha256.Sum256(buf.body.Bytes())
			etag := `"` + hex.EncodeToString(sum[:16]) + `"`
			h.Set("ETag", etag)
			h.Set("Cache-Control", policy)

			if etagMatches(r.Header.Get("If-None-Match"), etag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			h.Set("Content-Length", strconv.Itoa(buf.body.Len()))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buf.body.Bytes())
		})
	}
}

func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}
