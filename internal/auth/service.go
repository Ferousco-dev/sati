package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/files"
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
	ErrAccountDeleted     = apierror.New(http.StatusForbidden, "ACCOUNT_DELETED", "This account was deleted. To use it again, email our support team.")
	ErrWrongPassword      = apierror.New(http.StatusBadRequest, "INVALID_PASSWORD", "That password is not correct")
	ErrNotCustomer        = apierror.New(http.StatusForbidden, "NOT_A_CUSTOMER_ACCOUNT", "Staff accounts are managed by an administrator")
)

// AvatarMaxBytes caps a profile picture. Pictures are small, so this is generous.
const AvatarMaxBytes = 2 << 20

var avatarTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

type Mailer interface {
	SendVerification(ctx context.Context, name, email, token string) error
	SendPasswordReset(ctx context.Context, name, email, token string) error
	SendInvite(ctx context.Context, name, email string, role users.Role, token string) error
}

type Settings struct {
	SessionTTL     time.Duration
	RememberTTL    time.Duration
	VerifyTTL      time.Duration
	ResetTTL       time.Duration
	InviteTTL      time.Duration
	ResendCooldown time.Duration
	TouchAfter     time.Duration
}

func DefaultSettings() Settings {
	return Settings{
		SessionTTL:     7 * 24 * time.Hour,
		RememberTTL:    30 * 24 * time.Hour,
		VerifyTTL:      15 * time.Minute,
		ResetTTL:       time.Hour,
		InviteTTL:      72 * time.Hour,
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
	files  files.Storage
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

// WithStorage lets the service store and link profile pictures.
func (s *Service) WithStorage(storage files.Storage) *Service {
	s.files = storage
	return s
}

// present fills in the fields that are computed rather than stored.
func (s *Service) present(u users.User) users.User {
	if s.files != nil && u.AvatarKey != "" {
		u.AvatarURL = s.files.URL(u.AvatarKey)
	}
	return u
}

type RegisterInput struct {
	Name       string
	Email      string
	Password   string
	RememberMe bool
	Phone      string
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
	phone, err := normalizePhone(in.Phone)
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
		if phone != "" {
			if err = s.users.SetPhone(ctx, tx, user.ID, phone); err != nil {
				return err
			}
			user.Phone = phone
		}
		if issued, err = s.issueSession(ctx, tx, user.ID, in.RememberMe, now); err != nil {
			return err
		}
		return s.store.replaceToken(ctx, tx, verificationTokens, user.ID, s.hasher.sum(verifyToken), now.Add(s.set.VerifyTTL), now)
	})
	if errors.Is(err, users.ErrEmailTaken) {
		// A deleted account keeps its email for good, so say why it can't be used again.
		if old, lookup := s.users.ByEmail(ctx, s.pool, email); lookup == nil && old.DeletedAt != nil {
			return users.User{}, IssuedSession{}, ErrAccountDeleted
		}
		return users.User{}, IssuedSession{}, ErrEmailTaken
	}
	if err != nil {
		return users.User{}, IssuedSession{}, err
	}

	s.sendVerification(ctx, user, verifyToken)
	return s.present(user), issued, nil
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

	// Only someone who knows the password learns the account was deleted.
	if user.DeletedAt != nil {
		return users.User{}, IssuedSession{}, ErrAccountDeleted
	}

	issued, err := s.issueSession(ctx, s.pool, user.ID, in.RememberMe, s.now())
	return s.present(user), issued, err
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

	id := Identity{User: s.present(row.User), ExpiresAt: row.ExpiresAt}
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
	if user.DeletedAt != nil {
		return nil
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
		if err := s.users.MarkEmailVerified(ctx, tx, userID, now); err != nil {
			return err
		}
		return s.store.revokeAllSessions(ctx, tx, userID, now)
	})
	if errors.Is(err, errNoRow) {
		return ErrInvalidToken
	}
	return err
}

// Invite creates an account with an unusable password and emails a link
// to set one, using the same single-use token flow as password reset.
func (s *Service) Invite(ctx context.Context, name, email string, role users.Role) (users.User, error) {
	if role == users.RoleCustomer || !role.Valid() {
		return users.User{}, validationError("Role must be STAFF, RIDER or ADMIN")
	}
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		return users.User{}, validationError("Name must be between 1 and 100 characters")
	}
	email = normalizeEmail(email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 {
		return users.User{}, validationError("Email address is not valid")
	}

	placeholder, err := newToken()
	if err != nil {
		return users.User{}, err
	}
	passwordHash, err := HashPassword(placeholder)
	if err != nil {
		return users.User{}, err
	}
	token, err := newToken()
	if err != nil {
		return users.User{}, err
	}

	now := s.now()
	var user users.User
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if user, err = s.users.CreateWithRole(ctx, tx, name, email, passwordHash, role); err != nil {
			return err
		}
		return s.store.replaceToken(ctx, tx, resetTokens, user.ID, s.hasher.sum(token), now.Add(s.set.InviteTTL), now)
	})
	if errors.Is(err, users.ErrEmailTaken) {
		return users.User{}, ErrEmailTaken
	}
	if err != nil {
		return users.User{}, err
	}

	if err := s.mailer.SendInvite(ctx, user.Name, user.Email, user.Role, token); err != nil {
		s.log.Error("send invite email", "user_id", user.ID, "error", err)
	}
	return user, nil
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

type ProfileInput struct {
	Name  *string
	Phone *string
}

// UpdateProfile changes the fields the customer sent and leaves the rest alone.
func (s *Service) UpdateProfile(ctx context.Context, userID string, in ProfileInput) (users.User, error) {
	current, err := s.users.ByID(ctx, s.pool, userID)
	if err != nil {
		return users.User{}, err
	}
	name, phone := current.Name, current.Phone
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
			return users.User{}, validationError("Name must be between 1 and 100 characters")
		}
	}
	if in.Phone != nil {
		if phone, err = normalizePhone(*in.Phone); err != nil {
			return users.User{}, err
		}
	}
	if err := s.users.UpdateProfile(ctx, s.pool, userID, name, phone, s.now()); err != nil {
		return users.User{}, err
	}
	return s.reload(ctx, userID)
}

// SetAvatar stores a new profile picture and removes the one it replaces.
func (s *Service) SetAvatar(ctx context.Context, userID string, data []byte) (users.User, error) {
	if s.files == nil {
		return users.User{}, apierror.ErrInternal
	}
	if len(data) > AvatarMaxBytes {
		return users.User{}, validationError("The picture must be 2 MB or smaller")
	}
	ext, ok := avatarTypes[http.DetectContentType(data)]
	if !ok {
		return users.User{}, validationError("Please upload a JPG, PNG or WebP picture")
	}

	current, err := s.users.ByID(ctx, s.pool, userID)
	if err != nil {
		return users.User{}, err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return users.User{}, err
	}
	key := "avatars/" + userID + "/" + hex.EncodeToString(raw) + ext
	if err := s.files.Put(ctx, key, bytes.NewReader(data)); err != nil {
		return users.User{}, err
	}
	if err := s.users.SetAvatarKey(ctx, s.pool, userID, key, s.now()); err != nil {
		_ = s.files.Delete(ctx, key)
		return users.User{}, err
	}
	s.discardAvatar(ctx, current.AvatarKey)
	return s.reload(ctx, userID)
}

func (s *Service) RemoveAvatar(ctx context.Context, userID string) (users.User, error) {
	current, err := s.users.ByID(ctx, s.pool, userID)
	if err != nil {
		return users.User{}, err
	}
	if err := s.users.SetAvatarKey(ctx, s.pool, userID, "", s.now()); err != nil {
		return users.User{}, err
	}
	s.discardAvatar(ctx, current.AvatarKey)
	return s.reload(ctx, userID)
}

func (s *Service) discardAvatar(ctx context.Context, key string) {
	if key == "" || s.files == nil {
		return
	}
	if err := s.files.Delete(ctx, key); err != nil {
		s.log.Error("remove old avatar", "key", key, "error", err)
	}
}

// DeleteAccount closes a customer's own account after they confirm with their password.
// Nothing is erased: the row stays, so the email can never be registered again, and
// bringing the account back needs an administrator. Every session ends immediately.
func (s *Service) DeleteAccount(ctx context.Context, userID, password string) error {
	user, err := s.users.ByID(ctx, s.pool, userID)
	if err != nil {
		return err
	}
	if user.Role != users.RoleCustomer {
		return ErrNotCustomer
	}
	if !VerifyPassword(password, user.PasswordHash) {
		return ErrWrongPassword
	}

	now := s.now()
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.users.MarkDeleted(ctx, tx, userID, now); err != nil {
			return err
		}
		if err := s.store.revokeAllSessions(ctx, tx, userID, now); err != nil {
			return err
		}
		return s.store.clearTokens(ctx, tx, userID)
	})
}

func (s *Service) reload(ctx context.Context, userID string) (users.User, error) {
	u, err := s.users.ByID(ctx, s.pool, userID)
	return s.present(u), err
}

// normalizePhone keeps digits and a leading plus, and accepts the usual separators.
// A phone number is optional, so an empty value is fine and clears it.
func normalizePhone(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var b strings.Builder
	digits := 0
	for i, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			digits++
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return "", validationError("Phone number is not valid")
		}
	}
	if digits < 7 || digits > 15 {
		return "", validationError("Phone number is not valid")
	}
	return b.String(), nil
}
