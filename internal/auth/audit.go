package auth

import (
	"net/http"
	"strings"

	"github.com/Ferousco-dev/sati/backend/internal/audit"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// auditChanges records every successful request that is not a read. The entry names
// the route pattern and the ids in the path, never the request body.
func (h *Handler) auditChanges(next http.Handler) http.Handler {
	if h.auditor == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if sw.status >= 400 {
			return
		}

		id, _ := IdentityFrom(r.Context())
		entityType, entityID := entityOf(r)
		h.auditor.Record(r.Context(), audit.Entry{
			ActorID: id.User.ID, ActorRole: string(id.User.Role),
			Action: r.Pattern, EntityType: entityType, EntityID: entityID, Status: sw.status,
			IP: middleware.ClientIP(r.Context()), RequestID: middleware.RequestID(r.Context()),
			Metadata: pathValues(r),
		})
	})
}

func (h *Handler) recordLogin(r *http.Request, action, actorID, role string, metadata map[string]any) {
	if h.auditor == nil {
		return
	}
	h.auditor.Record(r.Context(), audit.Entry{
		ActorID: actorID, ActorRole: role, Action: action,
		IP: middleware.ClientIP(r.Context()), RequestID: middleware.RequestID(r.Context()),
		Metadata: metadata,
	})
}

var pathNames = []string{"id", "imageId", "reference"}

func pathValues(r *http.Request) map[string]any {
	out := map[string]any{}
	for _, name := range pathNames {
		if v := r.PathValue(name); v != "" {
			out[name] = v
		}
	}
	return out
}

// entityOf turns "PATCH /api/v1/admin/orders/{id}/status" into ("orders", the id).
func entityOf(r *http.Request) (string, string) {
	_, path, _ := strings.Cut(r.Pattern, " ")
	path = strings.TrimPrefix(path, "/api/v1/")
	for _, prefix := range []string{"admin/", "rider/"} {
		path = strings.TrimPrefix(path, prefix)
	}
	entity, _, _ := strings.Cut(path, "/")
	return entity, r.PathValue("id")
}
