package notifications

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Ferousco-dev/sati/backend/db"
	"github.com/Ferousco-dev/sati/backend/internal/email"
	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

type fakeSender struct {
	mu       sync.Mutex
	sent     []email.Message
	failWith []error
}

func (f *fakeSender) Send(_ context.Context, m email.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.failWith) > 0 {
		err := f.failWith[0]
		f.failWith = f.failWith[1:]
		if err != nil {
			return err
		}
	}
	f.sent = append(f.sent, m)
	return nil
}

type env struct {
	pool   *pgxpool.Pool
	sender *fakeSender
	worker *Worker
	clock  *time.Time
	userID string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	conn, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(db.Migrations)
	goose.SetLogger(goose.NopLogger())
	_ = goose.SetDialect("postgres")
	if err := goose.Up(conn, "migrations"); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), `TRUNCATE users, email_jobs CASCADE`); err != nil {
		t.Fatal(err)
	}

	var userID string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (name, email, password_hash) VALUES ('Ada', 'ada@example.com', 'x') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	sender := &fakeSender{}
	worker := NewWorker(pool, sender, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clock := time.Now()
	worker.now = func() time.Time { return clock }
	return &env{pool: pool, sender: sender, worker: worker, clock: &clock, userID: userID}
}

func (e *env) enqueue(t *testing.T, kind string, p email.Payload, dedupe string) {
	t.Helper()
	if err := NewRepository().Enqueue(context.Background(), e.pool, kind, "ada@example.com", "Ada", p, dedupe); err != nil {
		t.Fatal(err)
	}
	*e.clock = time.Now().Add(time.Second)
}

func (e *env) job(t *testing.T) (status string, attempts int, payload, lastError string) {
	t.Helper()
	err := e.pool.QueryRow(context.Background(),
		`SELECT status::text, attempts, payload::text, last_error FROM email_jobs ORDER BY created_at DESC LIMIT 1`).
		Scan(&status, &attempts, &payload, &lastError)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func (e *env) advance(d time.Duration) { *e.clock = e.clock.Add(d) }

func TestWorkerSendsAndClearsPayload(t *testing.T) {
	e := newEnv(t)
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Name: "Ada", Link: "https://shop.example/verify-email?token=secret"}, "")

	n, err := e.worker.RunOnce(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("run: %d %v", n, err)
	}
	if len(e.sender.sent) != 1 {
		t.Fatalf("sent: %d", len(e.sender.sent))
	}
	m := e.sender.sent[0]
	if m.To != "ada@example.com" || m.Subject != "Verify your email" || !strings.Contains(m.Text, "token=secret") || m.IdempotencyKey == "" {
		t.Fatalf("message: %+v", m)
	}

	status, attempts, payload, _ := e.job(t)
	if status != "SENT" || attempts != 1 || strings.Contains(payload, "secret") {
		t.Fatalf("after send: %s %d %s", status, attempts, payload)
	}
	if n, _ := e.worker.RunOnce(context.Background()); n != 0 {
		t.Fatalf("sent job picked up again: %d", n)
	}
}

func TestWorkerRetriesWithBackoffThenGivesUp(t *testing.T) {
	e := newEnv(t)
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "")
	boom := errors.New("provider down")
	e.sender.failWith = []error{boom, boom, boom, boom, boom}

	for attempt := 1; attempt <= 5; attempt++ {
		n, _ := e.worker.RunOnce(context.Background())
		if n != 1 {
			t.Fatalf("attempt %d ran %d jobs", attempt, n)
		}
		status, attempts, _, lastErr := e.job(t)
		if attempts != attempt || lastErr == "" {
			t.Fatalf("attempt %d: %s %d %q", attempt, status, attempts, lastErr)
		}
		if attempt < 5 {
			if status != "PENDING" {
				t.Fatalf("attempt %d: expected PENDING, got %s", attempt, status)
			}
			if n, _ := e.worker.RunOnce(context.Background()); n != 0 {
				t.Fatalf("attempt %d: retried before the backoff", attempt)
			}
			e.advance(backoff(attempt) + time.Second)
		} else if status != "FAILED" {
			t.Fatalf("expected FAILED after the last attempt, got %s", status)
		}
	}
	if len(e.sender.sent) != 0 {
		t.Fatal("nothing should have been sent")
	}
}

func TestWorkerSucceedsAfterTransientFailure(t *testing.T) {
	e := newEnv(t)
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "")
	e.sender.failWith = []error{errors.New("timeout")}

	e.worker.RunOnce(context.Background())
	e.advance(backoff(1) + time.Second)
	e.worker.RunOnce(context.Background())

	if status, attempts, _, _ := e.job(t); status != "SENT" || attempts != 2 || len(e.sender.sent) != 1 {
		t.Fatalf("%s %d sent=%d", status, attempts, len(e.sender.sent))
	}
}

func TestWorkerPermanentFailureIsNotRetried(t *testing.T) {
	e := newEnv(t)
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "")
	e.sender.failWith = []error{&email.PermanentError{Err: errors.New("invalid address")}}

	e.worker.RunOnce(context.Background())
	if status, attempts, payload, lastErr := e.job(t); status != "FAILED" || attempts != 1 || payload != "{}" || !strings.Contains(lastErr, "invalid address") {
		t.Fatalf("%s %d %s %q", status, attempts, payload, lastErr)
	}
}

func TestWorkerFailsUnknownKind(t *testing.T) {
	e := newEnv(t)
	e.enqueue(t, "mystery", email.Payload{}, "")
	e.worker.RunOnce(context.Background())
	if status, _, _, _ := e.job(t); status != "FAILED" {
		t.Fatalf("status %s", status)
	}
}

func TestWorkerReclaimsStuckJobs(t *testing.T) {
	e := newEnv(t)
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "")
	if _, err := NewRepository().Claim(context.Background(), e.pool, *e.clock, 10); err != nil {
		t.Fatal(err)
	}

	if n, _ := e.worker.RunOnce(context.Background()); n != 0 {
		t.Fatalf("a job being sent was claimed twice: %d", n)
	}
	e.advance(stuckAfter + time.Minute)
	if n, _ := e.worker.RunOnce(context.Background()); n != 1 || len(e.sender.sent) != 1 {
		t.Fatalf("crashed worker's job was not reclaimed: n=%d sent=%d", n, len(e.sender.sent))
	}
}

func TestEnqueueDeduplicates(t *testing.T) {
	e := newEnv(t)
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "same-key")
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "same-key")
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "")
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "")

	var n int
	e.pool.QueryRow(context.Background(), `SELECT count(*) FROM email_jobs`).Scan(&n)
	if n != 3 {
		t.Fatalf("jobs: %d", n)
	}
}

func TestConcurrentWorkersDoNotDoubleSend(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 8; i++ {
		e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "")
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := NewWorker(e.pool, e.sender, slog.New(slog.NewTextHandler(io.Discard, nil)))
			w.now = e.worker.now
			w.RunOnce(context.Background())
		}()
	}
	wg.Wait()

	if len(e.sender.sent) != 8 {
		t.Fatalf("each email must be sent exactly once, sent %d", len(e.sender.sent))
	}
}

func TestPurgeRemovesOldFinishedJobs(t *testing.T) {
	e := newEnv(t)
	e.enqueue(t, email.KindVerifyEmail, email.Payload{Link: "https://x"}, "")
	e.worker.RunOnce(context.Background())
	e.advance(retention + time.Hour)

	if err := NewRepository().Purge(context.Background(), e.pool, *e.clock); err != nil {
		t.Fatal(err)
	}
	var n int
	e.pool.QueryRow(context.Background(), `SELECT count(*) FROM email_jobs`).Scan(&n)
	if n != 0 {
		t.Fatalf("old jobs kept: %d", n)
	}
}

func TestMailerBuildsFrontendLinks(t *testing.T) {
	e := newEnv(t)
	svc := NewService("https://shop.example/", slog.New(slog.NewTextHandler(io.Discard, nil)))
	m := NewMailer(svc, e.pool)
	ctx := context.Background()

	if err := m.SendVerification(ctx, "Ada", "ada@example.com", "tok+/=1"); err != nil {
		t.Fatal(err)
	}
	if err := m.SendPasswordReset(ctx, "Ada", "ada@example.com", "tok2"); err != nil {
		t.Fatal(err)
	}
	if err := m.SendInvite(ctx, "Sam", "sam@example.com", users.RoleStaff, "tok3"); err != nil {
		t.Fatal(err)
	}
	*e.clock = time.Now().Add(time.Second)
	for i := 0; i < 3; i++ {
		e.worker.RunOnce(ctx)
	}

	if len(e.sender.sent) != 3 {
		t.Fatalf("sent %d", len(e.sender.sent))
	}
	want := map[string]string{
		"Verify your email":   "https://shop.example/verify-email?token=tok%2B%2F%3D1",
		"Reset your password": "https://shop.example/reset-password?token=tok2",
	}
	for _, m := range e.sender.sent {
		if link, ok := want[m.Subject]; ok && !strings.Contains(m.Text, link) {
			t.Errorf("%s: missing link %s in %s", m.Subject, link, m.Text)
		}
		if strings.HasPrefix(m.Subject, "You have been invited") && !strings.Contains(m.Text, "https://shop.example/reset-password?token=tok3") {
			t.Errorf("invite link: %s", m.Text)
		}
	}
}

func TestInAppNotifications(t *testing.T) {
	e := newEnv(t)
	repo := NewRepository()
	ctx := context.Background()
	for _, title := range []string{"one", "two", "three"} {
		if err := repo.AddNotification(ctx, e.pool, e.userID, "order_placed", title, "body", ""); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := repo.List(ctx, e.pool, e.userID, false, 2, 0)
	if err != nil || total != 3 || len(items) != 2 || items[0].Title != "three" {
		t.Fatalf("list: %+v %d %v", items, total, err)
	}
	if n, _ := repo.UnreadCount(ctx, e.pool, e.userID); n != 3 {
		t.Fatalf("unread: %d", n)
	}
	if err := repo.MarkRead(ctx, e.pool, e.userID, items[0].ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.UnreadCount(ctx, e.pool, e.userID); n != 2 {
		t.Fatalf("unread after read: %d", n)
	}
	if err := repo.MarkRead(ctx, e.pool, "00000000-0000-0000-0000-000000000000", items[0].ID, time.Now()); err == nil {
		t.Fatal("another user marked a notification read")
	}
	if n, err := repo.MarkAllRead(ctx, e.pool, e.userID, time.Now()); err != nil || n != 2 {
		t.Fatalf("read all: %d %v", n, err)
	}
	if unread, _, _ := repo.List(ctx, e.pool, e.userID, true, 10, 0); len(unread) != 0 {
		t.Fatalf("unread filter: %+v", unread)
	}
}

func TestEmailKind(t *testing.T) {
	tests := []struct{ status, fulfillment, want string }{
		{"CONFIRMED", "PICKUP", email.KindOrderConfirmed},
		{"PREPARING", "PICKUP", ""},
		{"READY", "PICKUP", email.KindOrderReadyPickup},
		{"READY", "DELIVERY", ""},
		{"OUT_FOR_DELIVERY", "DELIVERY", email.KindOrderOutForDelivery},
		{"DELIVERED", "DELIVERY", email.KindOrderDelivered},
		{"COMPLETED", "PICKUP", email.KindOrderDelivered},
		{"CANCELLED", "PICKUP", email.KindOrderCancelled},
		{"FAILED", "DELIVERY", email.KindOrderFailed},
	}
	for _, tt := range tests {
		if got := emailKind(tt.status, tt.fulfillment); got != tt.want {
			t.Errorf("%s/%s: got %q, want %q", tt.status, tt.fulfillment, got, tt.want)
		}
	}
}
