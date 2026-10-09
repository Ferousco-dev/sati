package auth

import (
	"errors"
	"io"
	"github.com/Ferousco-dev/sati/backend/pkg/router"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ferousco-dev/sati/backend/internal/audit"
	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
	"github.com/Ferousco-dev/sati/backend/pkg/metrics"
	"github.com/Ferousco-dev/sati/backend/pkg/middleware"
	"github.com/Ferousco-dev/sati/backend/pkg/ratelimit"
)

const (
	CookieName              = "sati_session"
	loginAttemptsPerAccount = 8
	loginAttemptWindow      = 15 * time.Minute
)

type Handler struct {
	svc          *Service
	log          *slog.Logger
	secureCookie bool
	limiter      ratelimit.Limiter
	auditor      *audit.Recorder
}

// WithAuditor records administrative requests and sign-in events.
func (h *Handler) WithAuditor(a *audit.Recorder) *Handler {
	h.auditor = a
	return h
}

// WithLimiter also limits logins per account, so one password cannot be
// guessed from many addresses.
func (h *Handler) WithLimiter(l ratelimit.Limiter) *Handler {
	h.limiter = l
	return h
}

func NewHandler(svc *Service, log *slog.Logger, secureCookie bool) *Handler {
	return &Handler{svc: svc, log: log, secureCookie: secureCookie}
}

func (h *Handler) Mount(mux router.Mux) {
	authn := h.RequireAuth()

	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	mux.Handle("GET /api/v1/auth/me", authn(http.HandlerFunc(h.me)))
	mux.Handle("PATCH /api/v1/auth/me", authn(http.HandlerFunc(h.updateMe)))
	mux.Handle("DELETE /api/v1/auth/me", authn(http.HandlerFunc(h.deleteMe)))
	mux.Handle("POST /api/v1/auth/me/avatar", authn(http.HandlerFunc(h.uploadAvatar)))
	mux.Handle("DELETE /api/v1/auth/me/avatar", authn(http.HandlerFunc(h.removeAvatar)))
	mux.HandleFunc("POST /api/v1/auth/verify-email", h.verifyEmail)
	mux.Handle("POST /api/v1/auth/resend-verification", authn(http.HandlerFunc(h.resendVerification)))
	mux.HandleFunc("POST /api/v1/auth/forgot-password", h.forgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", h.resetPassword)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		Email      string `json:"email"`
		Password   string `json:"password"`
		RememberMe bool   `json:"remember_me"`
		Phone      string `json:"phone"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		apierror.Write(w, err)
		return
	}

	user, issued, err := h.svc.Register(r.Context(), RegisterInput(body))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.setCookie(w, issued.Token, issued.ExpiresAt)
	apierror.JSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		RememberMe bool   `json:"remember_me"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		apierror.Write(w, err)
		return
	}

	if h.limiter != nil {
		key := "rl:login-email:" + strings.ToLower(strings.TrimSpace(body.Email))
		if res, err := h.limiter.Allow(r.Context(), key, loginAttemptsPerAccount, loginAttemptWindow); err == nil && !res.Allowed {
			metrics.RateLimited.Inc("login-email")
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(res.RetryAfter.Seconds()))))
			apierror.Write(w, apierror.ErrRateLimited)
			return
		}
	}

	user, issued, err := h.svc.Login(r.Context(), LoginInput(body))
	if errors.Is(err, ErrInvalidCredentials) {
		h.recordLogin(r, "auth.login_failed", "", "", map[string]any{"email": strings.ToLower(strings.TrimSpace(body.Email))})
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if user.Role != users.RoleCustomer {
		h.recordLogin(r, "auth.login", user.ID, string(user.Role), nil)
	}
	h.setCookie(w, issued.Token, issued.ExpiresAt)
	apierror.JSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	apierror.JSON(w, http.StatusOK, map[string]any{"user": id.User})
}

func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	var body struct {
		Name  *string `json:"name"`
		Phone *string `json:"phone"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		apierror.Write(w, err)
		return
	}
	user, err := h.svc.UpdateProfile(r.Context(), id.User.ID, ProfileInput(body))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, AvatarMaxBytes+(1<<20))
	file, _, err := r.FormFile("image")
	if err != nil {
		apierror.Write(w, validationError("A picture is required in the \"image\" field"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, AvatarMaxBytes+1))
	if err != nil {
		apierror.Write(w, validationError("Could not read the uploaded picture"))
		return
	}
	user, err := h.svc.SetAvatar(r.Context(), id.User.ID, data)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) removeAvatar(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	user, err := h.svc.RemoveAvatar(r.Context(), id.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	apierror.JSON(w, http.StatusOK, map[string]any{"user": user})
}

// deleteMe closes the signed-in customer's account after they confirm with their password.
func (h *Handler) deleteMe(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	var body struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		apierror.Write(w, err)
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), id.User.ID, body.Password); err != nil {
		h.fail(w, r, err)
		return
	}
	h.recordLogin(r, "auth.account_deleted", id.User.ID, string(id.User.Role), map[string]any{"email": id.User.Email})
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		apierror.Write(w, err)
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), body.Token); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	if err := h.svc.ResendVerification(r.Context(), id.User); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		apierror.Write(w, err)
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), body.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		apierror.Write(w, err)
		return
	}
	if err := h.svc.ResetPassword(r.Context(), body.Token, body.Password); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		apierror.Write(w, apiErr)
		return
	}
	h.log.Error("request failed", "request_id", middleware.RequestID(r.Context()), "error", err)
	apierror.Write(w, apierror.ErrInternal)
}

func (h *Handler) setCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}
