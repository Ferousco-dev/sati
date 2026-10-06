package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Ferousco-dev/sati/backend/db"
	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

type sentMail struct{ kind, email, token string }

type captureMailer struct{ sent []sentMail }

func (m *captureMailer) SendVerification(_ context.Context, _, email, token string) error {
	m.sent = append(m.sent, sentMail{"verification", email, token})
	return nil
}

func (m *captureMailer) SendPasswordReset(_ context.Context, _, email, token string) error {
	m.sent = append(m.sent, sentMail{"reset", email, token})
	return nil
}

func (m *captureMailer) SendInvite(_ context.Context, _, email string, _ users.Role, token string) error {
	m.sent = append(m.sent, sentMail{"invite", email, token})
	return nil
}

func (m *captureMailer) last(kind string) string {
	for i := len(m.sent) - 1; i >= 0; i-- {
		if m.sent[i].kind == kind {
			return m.sent[i].token
		}
	}
	return ""
}

type env struct {
	svc    *Service
	mailer *captureMailer
	srv    http.Handler
	clock  *time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	migrateTestDB(t, url)

	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), `TRUNCATE users CASCADE`); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mailer := &captureMailer{}
	svc := NewService(pool, mailer, log, "test-secret", DefaultSettings())

	clock := time.Now()
	svc.now = func() time.Time { return clock }

	mux := http.NewServeMux()
	NewHandler(svc, log, false).Mount(mux)
	return &env{svc: svc, mailer: mailer, srv: mux, clock: &clock}
}

func migrateTestDB(t *testing.T, url string) {
	t.Helper()
	conn, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	goose.SetBaseFS(db.Migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(conn, "migrations"); err != nil {
		t.Fatal(err)
	}
}

func (e *env) call(t *testing.T, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	return nil
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error.Code
}

func (e *env) register(t *testing.T, email string, remember bool) *http.Cookie {
	t.Helper()
	rec := e.call(t, "POST", "/api/v1/auth/register", map[string]any{
		"name": "Ada", "email": email, "password": "correct-horse", "remember_me": remember,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: got %d %s", rec.Code, rec.Body)
	}
	c := sessionCookie(rec)
	if c == nil {
		t.Fatal("no session cookie")
	}
	return c
}

func TestRegisterLoginLogout(t *testing.T) {
	e := newEnv(t)
	cookie := e.register(t, "Ada@Example.com", false)

	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("cookie must be HttpOnly and SameSite=Lax")
	}

	me := e.call(t, "GET", "/api/v1/auth/me", nil, cookie)
	if me.Code != http.StatusOK {
		t.Fatalf("me: got %d", me.Code)
	}

	dup := e.call(t, "POST", "/api/v1/auth/register", map[string]any{
		"name": "Ada", "email": "ada@example.com", "password": "correct-horse",
	}, nil)
	if dup.Code != http.StatusConflict || errorCode(t, dup) != "EMAIL_TAKEN" {
		t.Fatalf("duplicate email: got %d", dup.Code)
	}

	bad := e.call(t, "POST", "/api/v1/auth/login", map[string]any{"email": "ada@example.com", "password": "wrong-password"}, nil)
	if bad.Code != http.StatusUnauthorized || errorCode(t, bad) != "INVALID_CREDENTIALS" {
		t.Fatalf("wrong password: got %d", bad.Code)
	}
	unknown := e.call(t, "POST", "/api/v1/auth/login", map[string]any{"email": "nobody@example.com", "password": "whatever-123"}, nil)
	if unknown.Code != http.StatusUnauthorized || errorCode(t, unknown) != "INVALID_CREDENTIALS" {
		t.Fatalf("unknown email: got %d", unknown.Code)
	}

	login := e.call(t, "POST", "/api/v1/auth/login", map[string]any{"email": "ada@example.com", "password": "correct-horse"}, nil)
	second := sessionCookie(login)
	if login.Code != http.StatusOK || second == nil {
		t.Fatalf("login: got %d", login.Code)
	}

	if out := e.call(t, "POST", "/api/v1/auth/logout", nil, cookie); out.Code != http.StatusNoContent {
		t.Fatalf("logout: got %d", out.Code)
	}
	if after := e.call(t, "GET", "/api/v1/auth/me", nil, cookie); after.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session still works: %d", after.Code)
	}
	if still := e.call(t, "GET", "/api/v1/auth/me", nil, second); still.Code != http.StatusOK {
		t.Fatalf("other session should stay valid: %d", still.Code)
	}
}

func TestValidation(t *testing.T) {
	e := newEnv(t)
	cases := []map[string]any{
		{"name": "", "email": "a@example.com", "password": "correct-horse"},
		{"name": "Ada", "email": "not-an-email", "password": "correct-horse"},
		{"name": "Ada", "email": "a@example.com", "password": "short"},
	}
	for _, c := range cases {
		rec := e.call(t, "POST", "/api/v1/auth/register", c, nil)
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "VALIDATION_ERROR" {
			t.Fatalf("%v: got %d", c, rec.Code)
		}
	}
}

func TestSessionLifetimeAndExpiry(t *testing.T) {
	e := newEnv(t)
	normal := e.register(t, "a@example.com", false)
	remember := e.register(t, "b@example.com", true)

	set := DefaultSettings()
	if got := normal.Expires.Sub(*e.clock); got < set.SessionTTL-time.Minute || got > set.SessionTTL+time.Minute {
		t.Fatalf("normal session lifetime %v", got)
	}
	if got := remember.Expires.Sub(*e.clock); got < set.RememberTTL-time.Minute || got > set.RememberTTL+time.Minute {
		t.Fatalf("remember me lifetime %v", got)
	}

	*e.clock = e.clock.Add(set.SessionTTL + time.Hour)
	if rec := e.call(t, "GET", "/api/v1/auth/me", nil, normal); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session: got %d", rec.Code)
	}
	if rec := e.call(t, "GET", "/api/v1/auth/me", nil, remember); rec.Code != http.StatusOK {
		t.Fatalf("remember me session: got %d", rec.Code)
	}
}

func TestSessionExtendsOnUse(t *testing.T) {
	e := newEnv(t)
	cookie := e.register(t, "a@example.com", false)

	*e.clock = e.clock.Add(2 * time.Hour)
	rec := e.call(t, "GET", "/api/v1/auth/me", nil, cookie)
	refreshed := sessionCookie(rec)
	if refreshed == nil || !refreshed.Expires.After(cookie.Expires) {
		t.Fatal("expected a refreshed cookie with a later expiry")
	}
}

func TestEmailVerification(t *testing.T) {
	e := newEnv(t)
	cookie := e.register(t, "a@example.com", false)
	token := e.mailer.last("verification")
	if token == "" {
		t.Fatal("no verification email sent")
	}

	if rec := e.call(t, "POST", "/api/v1/auth/resend-verification", nil, cookie); rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != "RESEND_TOO_SOON" {
		t.Fatalf("resend inside cooldown: got %d", rec.Code)
	}

	*e.clock = e.clock.Add(61 * time.Second)
	if rec := e.call(t, "POST", "/api/v1/auth/resend-verification", nil, cookie); rec.Code != http.StatusAccepted {
		t.Fatalf("resend after cooldown: got %d", rec.Code)
	}
	fresh := e.mailer.last("verification")

	if rec := e.call(t, "POST", "/api/v1/auth/verify-email", map[string]any{"token": token}, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("replaced token should be invalid: got %d", rec.Code)
	}
	if rec := e.call(t, "POST", "/api/v1/auth/verify-email", map[string]any{"token": fresh}, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("verify: got %d", rec.Code)
	}
	if rec := e.call(t, "POST", "/api/v1/auth/verify-email", map[string]any{"token": fresh}, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("token reuse: got %d", rec.Code)
	}
	if rec := e.call(t, "POST", "/api/v1/auth/resend-verification", nil, cookie); rec.Code != http.StatusConflict {
		t.Fatalf("already verified: got %d", rec.Code)
	}
}

func TestVerificationTokenExpires(t *testing.T) {
	e := newEnv(t)
	e.register(t, "a@example.com", false)
	token := e.mailer.last("verification")

	*e.clock = e.clock.Add(DefaultSettings().VerifyTTL + time.Minute)
	if rec := e.call(t, "POST", "/api/v1/auth/verify-email", map[string]any{"token": token}, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("expired token: got %d", rec.Code)
	}
}

func TestPasswordReset(t *testing.T) {
	e := newEnv(t)
	cookie := e.register(t, "a@example.com", false)

	if rec := e.call(t, "POST", "/api/v1/auth/forgot-password", map[string]any{"email": "nobody@example.com"}, nil); rec.Code != http.StatusAccepted {
		t.Fatalf("unknown email must look identical: got %d", rec.Code)
	}
	if e.mailer.last("reset") != "" {
		t.Fatal("reset email sent for unknown address")
	}

	if rec := e.call(t, "POST", "/api/v1/auth/forgot-password", map[string]any{"email": "a@example.com"}, nil); rec.Code != http.StatusAccepted {
		t.Fatalf("forgot: got %d", rec.Code)
	}
	token := e.mailer.last("reset")
	if token == "" {
		t.Fatal("no reset email sent")
	}

	if rec := e.call(t, "POST", "/api/v1/auth/reset-password", map[string]any{"token": token, "password": "short"}, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("weak password: got %d", rec.Code)
	}
	if rec := e.call(t, "POST", "/api/v1/auth/reset-password", map[string]any{"token": token, "password": "brand-new-pass"}, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("reset: got %d", rec.Code)
	}
	if rec := e.call(t, "POST", "/api/v1/auth/reset-password", map[string]any{"token": token, "password": "another-pass-1"}, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("token reuse: got %d", rec.Code)
	}

	if rec := e.call(t, "GET", "/api/v1/auth/me", nil, cookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("reset must revoke sessions: got %d", rec.Code)
	}
	if rec := e.call(t, "POST", "/api/v1/auth/login", map[string]any{"email": "a@example.com", "password": "correct-horse"}, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old password still works: got %d", rec.Code)
	}
	if rec := e.call(t, "POST", "/api/v1/auth/login", map[string]any{"email": "a@example.com", "password": "brand-new-pass"}, nil); rec.Code != http.StatusOK {
		t.Fatalf("new password: got %d", rec.Code)
	}
}

func TestMeRequiresSession(t *testing.T) {
	e := newEnv(t)
	if rec := e.call(t, "GET", "/api/v1/auth/me", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	bogus := &http.Cookie{Name: CookieName, Value: "not-a-real-token"}
	if rec := e.call(t, "GET", "/api/v1/auth/me", nil, bogus); rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}
