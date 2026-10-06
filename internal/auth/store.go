package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

var errNoRow = errors.New("no matching row")

type tokenTable string

const (
	verificationTokens tokenTable = "email_verification_tokens"
	resetTokens        tokenTable = "password_reset_tokens"
)

type sessionRow struct {
	ID         string
	RememberMe bool
	ExpiresAt  time.Time
	LastUsedAt time.Time
	User       users.User
}

type store struct{}

func (store) createSession(ctx context.Context, db database.DBTX, userID string, hash []byte, remember bool, expires time.Time, now time.Time) error {
	_, err := db.Exec(ctx,
		`INSERT INTO sessions (user_id, token_hash, remember_me, expires_at, last_used_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, hash, remember, expires, now)
	return err
}

func (store) sessionByHash(ctx context.Context, db database.DBTX, hash []byte, now time.Time) (sessionRow, error) {
	var s sessionRow
	err := db.QueryRow(ctx,
		`SELECT s.id::text, s.remember_me, s.expires_at, s.last_used_at,
		        u.id::text, u.name, u.email::text, u.role::text, u.email_verified, u.created_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > $2`,
		hash, now).Scan(&s.ID, &s.RememberMe, &s.ExpiresAt, &s.LastUsedAt,
		&s.User.ID, &s.User.Name, &s.User.Email, &s.User.Role, &s.User.EmailVerified, &s.User.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sessionRow{}, errNoRow
	}
	return s, err
}

func (store) touchSession(ctx context.Context, db database.DBTX, id string, now, expires time.Time) error {
	_, err := db.Exec(ctx, `UPDATE sessions SET last_used_at = $2, expires_at = $3 WHERE id = $1`, id, now, expires)
	return err
}

func (store) revokeSession(ctx context.Context, db database.DBTX, hash []byte, now time.Time) error {
	_, err := db.Exec(ctx, `UPDATE sessions SET revoked_at = $2 WHERE token_hash = $1 AND revoked_at IS NULL`, hash, now)
	return err
}

func (store) revokeAllSessions(ctx context.Context, db database.DBTX, userID string, now time.Time) error {
	_, err := db.Exec(ctx, `UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, now)
	return err
}

func (store) replaceToken(ctx context.Context, db database.DBTX, table tokenTable, userID string, hash []byte, expires, now time.Time) error {
	if _, err := db.Exec(ctx, `DELETE FROM `+string(table)+` WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	_, err := db.Exec(ctx,
		`INSERT INTO `+string(table)+` (user_id, token_hash, expires_at, created_at) VALUES ($1, $2, $3, $4)`,
		userID, hash, expires, now)
	return err
}

func (store) latestTokenCreatedAt(ctx context.Context, db database.DBTX, table tokenTable, userID string) (time.Time, bool, error) {
	var created time.Time
	err := db.QueryRow(ctx,
		`SELECT created_at FROM `+string(table)+` WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, userID).Scan(&created)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	return created, err == nil, err
}

func (store) consumeToken(ctx context.Context, db database.DBTX, table tokenTable, hash []byte, now time.Time) (string, error) {
	var userID string
	err := db.QueryRow(ctx,
		`UPDATE `+string(table)+` SET used_at = $2
		 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
		 RETURNING user_id::text`, hash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errNoRow
	}
	return userID, err
}
