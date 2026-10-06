package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type Message struct {
	To             string
	ToName         string
	Subject        string
	HTML           string
	Text           string
	IdempotencyKey string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

// PermanentError marks a failure that retrying cannot fix, such as a rejected address.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

const resendURL = "https://api.resend.com/emails"

type ResendSender struct {
	apiKey string
	from   string
	url    string
	client *http.Client
}

func NewResendSender(apiKey, from string) *ResendSender {
	return &ResendSender{apiKey: apiKey, from: from, url: resendURL, client: &http.Client{Timeout: 10 * time.Second}}
}

func (r *ResendSender) Send(ctx context.Context, m Message) error {
	to := m.To
	if m.ToName != "" {
		to = fmt.Sprintf("%q <%s>", m.ToName, m.To)
	}
	body, err := json.Marshal(map[string]any{
		"from": r.from, "to": []string{to}, "subject": m.Subject, "html": m.HTML, "text": m.Text,
	})
	if err != nil {
		return &PermanentError{Err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return &PermanentError{Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if m.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", m.IdempotencyKey)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		return nil
	}

	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	err = fmt.Errorf("resend responded %d: %s", resp.StatusCode, detail)
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return err
	}
	return &PermanentError{Err: err}
}

// LogSender prints emails instead of sending them, for development without Resend credentials.
// Message bodies are only included outside production, because they contain sign-in links.
type LogSender struct {
	log        *slog.Logger
	showBodies bool
}

func NewLogSender(log *slog.Logger, showBodies bool) *LogSender {
	return &LogSender{log: log, showBodies: showBodies}
}

func (l *LogSender) Send(_ context.Context, m Message) error {
	attrs := []any{"to", m.To, "subject", m.Subject}
	if l.showBodies {
		attrs = append(attrs, "text", m.Text)
	}
	l.log.Info("email not sent (log sender)", attrs...)
	return nil
}
