package auth

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
)

type identityKey struct{}

func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

func (h *Handler) RequireAuth() middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(CookieName)
			if err != nil {
				apierror.Write(w, apierror.ErrUnauthorized)
				return
			}

			id, err := h.svc.Authenticate(r.Context(), c.Value)
			if err != nil {
				if errors.Is(err, apierror.ErrUnauthorized) {
					h.clearCookie(w)
				}
				h.fail(w, r, err)
				return
			}
			if id.Refreshed {
				h.setCookie(w, c.Value, id.ExpiresAt)
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, id)))
		})
	}
}

type Access struct {
	Customer middleware.Middleware
	Staff    middleware.Middleware
	Admin    middleware.Middleware
	Rider    middleware.Middleware
}

func (h *Handler) Access() Access {
	authn := h.RequireAuth()
	with := func(roles ...users.Role) middleware.Middleware {
		role := RequireRole(roles...)
		return func(next http.Handler) http.Handler { return authn(role(next)) }
	}
	return Access{
		Customer: authn,
		Staff:    with(users.RoleAdmin, users.RoleStaff),
		Admin:    with(users.RoleAdmin),
		Rider:    with(users.RoleRider),
	}
}

func RequireRole(roles ...users.Role) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := IdentityFrom(r.Context())
			if !ok {
				apierror.Write(w, apierror.ErrUnauthorized)
				return
			}
			if !slices.Contains(roles, id.User.Role) {
				apierror.Write(w, apierror.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
