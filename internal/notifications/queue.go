package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/internal/email"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

const (
	stuckAfter = 5 * time.Minute
	retention  = 14 * 24 * time.Hour
	failedKeep = 30 * 24 * time.Hour
)

type Job struct {
	ID          string
	Kind        string
	ToEmail     string
	ToName      string
	Payload     json.RawMessage
	Attempts    int
	MaxAttempts int
}

type Notification struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	OrderID   string     `json:"order_id,omitempty"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

// Enqueue stores an email to be sent later. A repeated dedupeKey is ignored.
func (Repository) Enqueue(ctx context.Context, db database.DBTX, kind, toEmail, toName string, payload email.Payload, dedupeKey string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var key *string
	if dedupeKey != "" {
		key = &dedupeKey
	}
	_, err = db.Exec(ctx,
		`INSERT INTO email_jobs (kind, to_email, to_name, payload, dedupe_key)
		 VALUES ($1, $2, $3, $4, $5) ON CONFLICT (dedupe_key) DO NOTHING`,
		kind, toEmail, toName, raw, key)
	return err
}

func (Repository) Claim(ctx context.Context, db database.DBTX, now time.Time, limit int) ([]Job, error) {
	rows, err := db.Query(ctx,
		`UPDATE email_jobs SET status = 'SENDING', locked_at = $1, attempts = attempts + 1
		 WHERE id IN (
		   SELECT id FROM email_jobs
		   WHERE (status = 'PENDING' AND next_attempt_at <= $1) OR (status = 'SENDING' AND locked_at < $2)
		   ORDER BY next_attempt_at, created_at LIMIT $3 FOR UPDATE SKIP LOCKED)
		 RETURNING id::text, kind, to_email, to_name, payload, attempts, max_attempts`,
		now, now.Add(-stuckAfter), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Kind, &j.ToEmail, &j.ToName, &j.Payload, &j.Attempts, &j.MaxAttempts); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// MarkSent also blanks the payload, which holds sign-in links, once it is no longer needed.
func (Repository) MarkSent(ctx context.Context, db database.DBTX, id string, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE email_jobs SET status = 'SENT', sent_at = $2, finished_at = $2, payload = '{}', last_error = '', locked_at = NULL
		 WHERE id = $1`, id, now)
	return err
}

func (Repository) MarkFailed(ctx context.Context, db database.DBTX, id, reason string, now time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE email_jobs SET status = 'FAILED', finished_at = $2, payload = '{}', last_error = $3, locked_at = NULL
		 WHERE id = $1`, id, now, truncate(reason, 500))
	return err
}

func (Repository) Retry(ctx context.Context, db database.DBTX, id, reason string, next time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE email_jobs SET status = 'PENDING', next_attempt_at = $2, last_error = $3, locked_at = NULL
		 WHERE id = $1`, id, next, truncate(reason, 500))
	return err
}

func (Repository) Purge(ctx context.Context, db database.DBTX, now time.Time) error {
	_, err := db.Exec(ctx,
		`DELETE FROM email_jobs WHERE (status = 'SENT' AND finished_at < $1) OR (status = 'FAILED' AND finished_at < $2)`,
		now.Add(-retention), now.Add(-failedKeep))
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (Repository) AddNotification(ctx context.Context, db database.DBTX, userID, kind, title, body, orderID string) error {
	var order *string
	if orderID != "" {
		order = &orderID
	}
	_, err := db.Exec(ctx,
		`INSERT INTO notifications (user_id, kind, title, body, order_id) VALUES ($1, $2, $3, $4, $5)`,
		userID, kind, title, body, order)
	return err
}

func (Repository) List(ctx context.Context, db database.DBTX, userID string, unreadOnly bool, limit, offset int) ([]Notification, int, error) {
	var total int
	if err := db.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND (NOT $2 OR read_at IS NULL)`, userID, unreadOnly).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx,
		`SELECT id::text, kind, title, body, COALESCE(order_id::text, ''), read_at, created_at
		 FROM notifications WHERE user_id = $1 AND (NOT $2 OR read_at IS NULL)
		 ORDER BY created_at DESC, id LIMIT $3 OFFSET $4`, userID, unreadOnly, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Kind, &n.Title, &n.Body, &n.OrderID, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, n)
	}
	return out, total, rows.Err()
}

func (Repository) UnreadCount(ctx context.Context, db database.DBTX, userID string) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

var errNotFound = errors.New("notification not found")

func (Repository) MarkRead(ctx context.Context, db database.DBTX, userID, id string, now time.Time) error {
	var found string
	err := db.QueryRow(ctx,
		`UPDATE notifications SET read_at = COALESCE(read_at, $3) WHERE id = $1 AND user_id = $2 RETURNING id::text`,
		id, userID, now).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	return err
}

func (Repository) MarkAllRead(ctx context.Context, db database.DBTX, userID string, now time.Time) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE notifications SET read_at = $2 WHERE user_id = $1 AND read_at IS NULL`, userID, now)
	return tag.RowsAffected(), err
}
