package notifications

import (
	"context"
	"github.com/Ferousco-dev/sati/backend/pkg/metrics"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/email"
)

const (
	pollInterval  = 2 * time.Second
	purgeInterval = time.Hour
	batchSize     = 10
	baseBackoff   = 30 * time.Second
	maxBackoff    = 6 * time.Hour
)

type Worker struct {
	pool   *pgxpool.Pool
	repo   *Repository
	sender email.Sender
	log    *slog.Logger
	now    func() time.Time
}

func NewWorker(pool *pgxpool.Pool, sender email.Sender, log *slog.Logger) *Worker {
	return &Worker{pool: pool, repo: NewRepository(), sender: sender, log: log, now: time.Now}
}

// Run sends queued emails until the context ends. Several instances can run at once.
func (w *Worker) Run(ctx context.Context) {
	poll := time.NewTicker(pollInterval)
	purge := time.NewTicker(purgeInterval)
	defer poll.Stop()
	defer purge.Stop()

	for {
		for {
			n, err := w.RunOnce(ctx)
			if err != nil {
				w.log.Error("email worker", "error", err)
			}
			if n < batchSize || err != nil {
				break
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-purge.C:
			if err := w.repo.Purge(ctx, w.pool, w.now()); err != nil {
				w.log.Error("purge email jobs", "error", err)
			}
		case <-poll.C:
		}
	}
}

// RunOnce sends one batch of due emails and reports how many it handled.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	jobs, err := w.repo.Claim(ctx, w.pool, w.now(), batchSize)
	if err != nil {
		return 0, err
	}
	for _, job := range jobs {
		w.process(ctx, job)
	}
	return len(jobs), nil
}

func (w *Worker) process(ctx context.Context, job Job) {
	err := w.deliver(ctx, job)
	now := w.now()

	switch {
	case err == nil:
		metrics.Emails.Inc("sent")
		if err := w.repo.MarkSent(ctx, w.pool, job.ID, now); err != nil {
			w.log.Error("mark email sent", "job_id", job.ID, "error", err)
		}
	case email.IsPermanent(err) || job.Attempts >= job.MaxAttempts:
		metrics.Emails.Inc("failed")
		w.log.Error("email failed for good", "job_id", job.ID, "kind", job.Kind, "attempts", job.Attempts, "error", err)
		if err := w.repo.MarkFailed(ctx, w.pool, job.ID, err.Error(), now); err != nil {
			w.log.Error("mark email failed", "job_id", job.ID, "error", err)
		}
	default:
		metrics.Emails.Inc("retry")
		w.log.Warn("email will be retried", "job_id", job.ID, "kind", job.Kind, "attempts", job.Attempts, "error", err)
		if err := w.repo.Retry(ctx, w.pool, job.ID, err.Error(), now.Add(backoff(job.Attempts))); err != nil {
			w.log.Error("schedule email retry", "job_id", job.ID, "error", err)
		}
	}
}

func (w *Worker) deliver(ctx context.Context, job Job) error {
	payload, err := decodePayload(job.Payload)
	if err != nil {
		return &email.PermanentError{Err: err}
	}
	subject, html, text, err := email.Render(job.Kind, payload)
	if err != nil {
		return &email.PermanentError{Err: err}
	}
	return w.sender.Send(ctx, email.Message{
		To: job.ToEmail, ToName: job.ToName, Subject: subject, HTML: html, Text: text, IdempotencyKey: job.ID,
	})
}

func backoff(attempts int) time.Duration {
	d := baseBackoff
	for i := 1; i < attempts; i++ {
		d *= 4
		if d >= maxBackoff {
			return maxBackoff
		}
	}
	return d
}
