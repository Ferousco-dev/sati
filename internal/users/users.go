package users

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

type Role string

const (
	RoleCustomer Role = "CUSTOMER"
	RoleStaff    Role = "STAFF"
	RoleAdmin    Role = "ADMIN"
	RoleRider    Role = "RIDER"
)

func (r Role) Valid() bool {
	switch r {
	case RoleCustomer, RoleStaff, RoleAdmin, RoleRider:
		return true
	}
	return false
}

type User struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	Role          Role      `json:"role"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	PasswordHash  string    `json:"-"`
}

var (
	ErrNotFound   = errors.New("user not found")
	ErrEmailTaken = errors.New("email already registered")
)

const columns = `id::text, name, email::text, role::text, email_verified, created_at, password_hash`

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func scan(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.EmailVerified, &u.CreatedAt, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (r *Repository) Create(ctx context.Context, db database.DBTX, name, email, passwordHash string) (User, error) {
	return r.CreateWithRole(ctx, db, name, email, passwordHash, RoleCustomer)
}

func (r *Repository) CreateWithRole(ctx context.Context, db database.DBTX, name, email, passwordHash string, role Role) (User, error) {
	u, err := scan(db.QueryRow(ctx,
		`INSERT INTO users (name, email, password_hash, role) VALUES ($1, $2, $3, $4::user_role) RETURNING `+columns,
		name, email, passwordHash, string(role)))

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return User{}, ErrEmailTaken
	}
	return u, err
}

func (r *Repository) ByEmail(ctx context.Context, db database.DBTX, email string) (User, error) {
	return scan(db.QueryRow(ctx, `SELECT `+columns+` FROM users WHERE email = $1`, email))
}

func (r *Repository) ByID(ctx context.Context, db database.DBTX, id string) (User, error) {
	return scan(db.QueryRow(ctx, `SELECT `+columns+` FROM users WHERE id = $1`, id))
}

func (r *Repository) List(ctx context.Context, db database.DBTX, role Role, limit, offset int) ([]User, int, error) {
	var total int
	if err := db.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE ($1 = '' OR role::text = $1)`, string(role)).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx,
		`SELECT `+columns+` FROM users WHERE ($1 = '' OR role::text = $1)
		 ORDER BY created_at DESC, id LIMIT $2 OFFSET $3`, string(role), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []User{}
	for rows.Next() {
		u, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

func (r *Repository) MarkEmailVerified(ctx context.Context, db database.DBTX, id string, at time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE users SET email_verified = TRUE, email_verified_at = $2, updated_at = $2 WHERE id = $1`, id, at)
	return err
}

func (r *Repository) SetPassword(ctx context.Context, db database.DBTX, id, passwordHash string, at time.Time) error {
	_, err := db.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = $3 WHERE id = $1`, id, passwordHash, at)
	return err
}

func (r *Repository) SetRole(ctx context.Context, db database.DBTX, id string, role Role, at time.Time) error {
	_, err := db.Exec(ctx, `UPDATE users SET role = $2::user_role, updated_at = $3 WHERE id = $1`, id, string(role), at)
	return err
}
