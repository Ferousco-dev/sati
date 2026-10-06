package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

var (
	ErrInvalidCredentials = apierror.New(http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect")
	ErrInvalidToken       = apierror.New(http.StatusBadRequest, "INVALID_TOKEN", "Token is invalid or has expired")
	ErrEmailTaken         = apierror.New(http.StatusConflict, "EMAIL_TAKEN", "An account with this email already exists")
	ErrResendTooSoon      = apierror.New(http.StatusTooManyRequests, "RESEND_TOO_SOON", "Please wait before requesting another email")
	ErrAlreadyVerified    = apierror.New(http.StatusConflict, "ALREADY_VERIFIED", "Email is already verified")
)

type Mailer interface {
	SendVerification(ctx context.Context, name, email, token string) error
	SendPasswordReset(ctx context.Context, name, email, token string) error
}

type Settings struct {
	SessionTTL     time.Duration
	RememberTTL    time.Duration
	VerifyTTL      time.Duration
	ResetTTL       time.Duration
	ResendCooldown time.Duration
	TouchAfter     time.Duration
}

func DefaultSettings() Settings {
	return Settings{
		SessionTTL:     7 * 24 * time.Hour,
		RememberTTL:    30 * 24 * time.Hour,
		VerifyTTL:      15 * time.Minute,
		ResetTTL:       time.Hour,
		ResendCooldown: 60 * time.Second,
		TouchAfter:     time.Hour,
	}
}

type Service struct {
	pool   *pgxpool.Pool
	users  *users.Repository
	store  store
	hasher hasher
	mailer Mailer
	log    *slog.Logger
	set    Settings
	now    func() time.Time
}

func NewService(pool *pgxpool.Pool, mailer Mailer, log *slog.Logger, secret string, set Settings) *Service {
	return &Service{
		pool:   pool,
		users:  users.NewRepository(),
		hasher: hasher{key: []byte(secret)},
		mailer: mailer,
		log:    log,
		set:    set,
		now:    time.Now,
	}
}

type RegisterInput struct {
	Name       string
	Email      string
	Password   string
	RememberMe bool
}

type LoginInput struct {
	Email      string
	Password   string
	RememberMe bool
}

type IssuedSession struct {
	Token     string
	ExpiresAt time.Time
}

type Identity struct {
	User      users.User
	ExpiresAt time.Time
	Refreshed bool
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (users.User, IssuedSession, error) {
	name, email, err := validateRegistration(in)
	if err != nil {
		return users.User{}, IssuedSession{}, err
	}
	passwordHash, err := HashPassword(in.Password)
	if err != nil {
		return users.User{}, IssuedSession{}, err
	}
	verifyToken, err := newToken()
	if err != nil {
		return users.User{}, IssuedSession{}, err
	}

	now := s.now()
	var user users.User
	var issued IssuedSession
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if user, err = s.users.Create(ctx, tx, name, email, passwordHash); err != nil {
			return err
		}
		if issued, err = s.issueSession(ctx, tx, user.ID, in.RememberMe, now); err != nil {
			return err
		}
		return s.store.replaceToken(ctx, tx, verificationTokens, user.ID, s.hasher.sum(verifyToken), now.Add(s.set.VerifyTTL), now)
	})
	if errors.Is(err, users.ErrEmailTaken) {
		return users.User{}, IssuedSession{}, ErrEmailTaken
	}
	if err != nil {
		return users.User{}, IssuedSession{}, err
	}

	s.sendVerification(ctx, user, verifyToken)
	return user, issued, nil
}

func (s *Service) Login(ctx context.Context, in LoginInput) (users.User, IssuedSession, error) {
	user, err := s.users.ByEmail(ctx, s.pool, normalizeEmail(in.Email))
	if errors.Is(err, users.ErrNotFound) {
		burnPassword(in.Password)
		return users.User{}, IssuedSession{}, ErrInvalidCredentials
	}
	if err != nil {
		return users.User{}, IssuedSession{}, err
	}
	if !VerifyPassword(in.Password, user.PasswordHash) {
		return users.User{}, IssuedSession{}, ErrInvalidCredentials
	}

	issued, err := s.issueSession(ctx, s.pool, user.ID, in.RememberMe, s.now())
	return user, issued, err
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	return s.store.revokeSession(ctx, s.pool, s.hasher.sum(rawToken), s.now())
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (Identity, error) {
	now := s.now()
	row, err := s.store.sessionByHash(ctx, s.pool, s.hasher.sum(rawToken), now)
	if errors.Is(err, errNoRow) {
		return Identity{}, apierror.ErrUnauthorized
	}
	if err != nil {
		return Identity{}, err
	}

	id := Identity{User: row.User, ExpiresAt: row.ExpiresAt}
	if now.Sub(row.LastUsedAt) >= s.set.TouchAfter {
		id.ExpiresAt = now.Add(s.ttl(row.RememberMe))
		if err := s.store.touchSession(ctx, s.pool, row.ID, now, id.ExpiresAt); err != nil {
			return Identity{}, err
		}
		id.Refreshed = true
	}
	return id, nil
}

func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	now := s.now()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		userID, err := s.store.consumeToken(ctx, tx, verificationTokens, s.hasher.sum(token), now)
		if err != nil {
			return err
		}
		return s.users.MarkEmailVerified(ctx, tx, userID, now)
	})
	if errors.Is(err, errNoRow) {
		return ErrInvalidToken
	}
	return err
}

func (s *Service) ResendVerification(ctx context.Context, user users.User) error {
	if user.EmailVerified {
		return ErrAlreadyVerified
	}

	now := s.now()
	last, found, err := s.store.latestTokenCreatedAt(ctx, s.pool, verificationTokens, user.ID)
	if err != nil {
		return err
	}
	if found && now.Sub(last) < s.set.ResendCooldown {
		return ErrResendTooSoon
	}

	token, err := newToken()
	if err != nil {
		return err
	}
	if err := s.store.replaceToken(ctx, s.pool, verificationTokens, user.ID, s.hasher.sum(token), now.Add(s.set.VerifyTTL), now); err != nil {
		return err
	}
	s.sendVerification(ctx, user, token)
	return nil
}

func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.users.ByEmail(ctx, s.pool, normalizeEmail(email))
	if errors.Is(err, users.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	now := s.now()
	last, found, err := s.store.latestTokenCreatedAt(ctx, s.pool, resetTokens, user.ID)
	if err != nil {
		return err
	}
	if found && now.Sub(last) < s.set.ResendCooldown {
		return nil
	}

	token, err := newToken()
	if err != nil {
		return err
	}
	if err := s.store.replaceToken(ctx, s.pool, resetTokens, user.ID, s.hasher.sum(token), now.Add(s.set.ResetTTL), now); err != nil {
		return err
	}
	if err := s.mailer.SendPasswordReset(ctx, user.Name, user.Email, token); err != nil {
		s.log.Error("send password reset email", "user_id", user.ID, "error", err)
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	now := s.now()
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		userID, err := s.store.consumeToken(ctx, tx, resetTokens, s.hasher.sum(token), now)
		if err != nil {
			return err
		}
		if err := s.users.SetPassword(ctx, tx, userID, passwordHash, now); err != nil {
			return err
		}
		return s.store.revokeAllSessions(ctx, tx, userID, now)
	})
	if errors.Is(err, errNoRow) {
		return ErrInvalidToken
	}
	return err
}

func (s *Service) issueSession(ctx context.Context, db database.DBTX, userID string, remember bool, now time.Time) (IssuedSession, error) {
	token, err := newToken()
	if err != nil {
		return IssuedSession{}, err
	}
	expires := now.Add(s.ttl(remember))
	if err := s.store.createSession(ctx, db, userID, s.hasher.sum(token), remember, expires, now); err != nil {
		return IssuedSession{}, err
	}
	return IssuedSession{Token: token, ExpiresAt: expires}, nil
}

func (s *Service) ttl(remember bool) time.Duration {
	if remember {
		return s.set.RememberTTL
	}
	return s.set.SessionTTL
}

func (s *Service) sendVerification(ctx context.Context, user users.User, token string) {
	if err := s.mailer.SendVerification(ctx, user.Name, user.Email, token); err != nil {
		s.log.Error("send verification email", "user_id", user.ID, "error", err)
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validationError(message string) *apierror.Error {
	return apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", message)
}

func validatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < 8 || n > 128 {
		return validationError("Password must be between 8 and 128 characters")
	}
	return nil
}

func validateRegistration(in RegisterInput) (name, email string, err error) {
	name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		return "", "", validationError("Name must be between 1 and 100 characters")
	}

	email = normalizeEmail(in.Email)
	addr, parseErr := mail.ParseAddress(email)
	if parseErr != nil || addr.Address != email || len(email) > 254 {
		return "", "", validationError("Email address is not valid")
	}

	if err := validatePassword(in.Password); err != nil {
		return "", "", err
	}
	return name, email, nil
}
