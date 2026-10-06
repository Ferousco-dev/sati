package email

import (
	"context"
	"log/slog"

	"github.com/Ferousco-dev/sati/backend/internal/users"
)

// LogMailer prints emails to the log instead of sending them.
// Tokens are only included outside production.
type LogMailer struct {
	log        *slog.Logger
	showTokens bool
}

func NewLogMailer(log *slog.Logger, showTokens bool) *LogMailer {
	return &LogMailer{log: log, showTokens: showTokens}
}

func (m *LogMailer) SendVerification(_ context.Context, name, email, token string) error {
	m.send("verification", name, email, token)
	return nil
}

func (m *LogMailer) SendPasswordReset(_ context.Context, name, email, token string) error {
	m.send("password reset", name, email, token)
	return nil
}

func (m *LogMailer) SendInvite(_ context.Context, name, email string, role users.Role, token string) error {
	m.send("invite "+string(role), name, email, token)
	return nil
}

func (m *LogMailer) send(kind, name, email, token string) {
	attrs := []any{"kind", kind, "to", email, "name", name}
	if m.showTokens {
		attrs = append(attrs, "token", token)
	}
	m.log.Info("email not sent (log mailer)", attrs...)
}
