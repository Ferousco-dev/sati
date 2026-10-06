package addresses

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

const MaxPerUser = 10

type Address struct {
	ID            string    `json:"id"`
	RecipientName string    `json:"recipient_name"`
	Phone         string    `json:"phone"`
	AddressLine   string    `json:"address_line"`
	Area          string    `json:"area"`
	City          string    `json:"city"`
	Instructions  string    `json:"instructions"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateInput struct {
	RecipientName string `json:"recipient_name"`
	Phone         string `json:"phone"`
	AddressLine   string `json:"address_line"`
	Area          string `json:"area"`
	City          string `json:"city"`
	Instructions  string `json:"instructions"`
}

type UpdateInput struct {
	RecipientName *string `json:"recipient_name"`
	Phone         *string `json:"phone"`
	AddressLine   *string `json:"address_line"`
	Area          *string `json:"area"`
	City          *string `json:"city"`
	Instructions  *string `json:"instructions"`
}

var (
	ErrNotFound = apierror.New(http.StatusNotFound, "ADDRESS_NOT_FOUND", "Address not found")
	ErrLimit    = apierror.New(http.StatusConflict, "ADDRESS_LIMIT_REACHED", "You can save up to 10 addresses")
)

const columns = `id::text, recipient_name, phone, address_line, area, city, instructions, created_at, updated_at`

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func scan(row pgx.Row) (Address, error) {
	var a Address
	err := row.Scan(&a.ID, &a.RecipientName, &a.Phone, &a.AddressLine, &a.Area, &a.City, &a.Instructions, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Address{}, ErrNotFound
	}
	return a, err
}

func (Repository) List(ctx context.Context, db database.DBTX, userID string) ([]Address, error) {
	rows, err := db.Query(ctx, `SELECT `+columns+` FROM addresses WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Address{}
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (Repository) Owned(ctx context.Context, db database.DBTX, userID, id string) (Address, error) {
	if !httpx.ValidUUID(id) {
		return Address{}, ErrNotFound
	}
	return scan(db.QueryRow(ctx, `SELECT `+columns+` FROM addresses WHERE id = $1 AND user_id = $2`, id, userID))
}

func (Repository) Count(ctx context.Context, db database.DBTX, userID string) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM addresses WHERE user_id = $1`, userID).Scan(&n)
	return n, err
}

func (Repository) Create(ctx context.Context, db database.DBTX, userID string, in CreateInput) (Address, error) {
	return scan(db.QueryRow(ctx,
		`INSERT INTO addresses (user_id, recipient_name, phone, address_line, area, city, instructions)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+columns,
		userID, in.RecipientName, in.Phone, in.AddressLine, in.Area, in.City, in.Instructions))
}

func (Repository) Update(ctx context.Context, db database.DBTX, userID, id string, in UpdateInput, now time.Time) (Address, error) {
	return scan(db.QueryRow(ctx,
		`UPDATE addresses SET
		   recipient_name = COALESCE($3, recipient_name),
		   phone = COALESCE($4, phone),
		   address_line = COALESCE($5, address_line),
		   area = COALESCE($6, area),
		   city = COALESCE($7, city),
		   instructions = COALESCE($8, instructions),
		   updated_at = $9
		 WHERE id = $1 AND user_id = $2 RETURNING `+columns,
		id, userID, in.RecipientName, in.Phone, in.AddressLine, in.Area, in.City, in.Instructions, now))
}

func (Repository) Delete(ctx context.Context, db database.DBTX, userID, id string) error {
	tag, err := db.Exec(ctx, `DELETE FROM addresses WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func validation(msg string) *apierror.Error {
	return apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", msg)
}

func cleanPhone(raw string) (string, bool) {
	var b strings.Builder
	for i, c := range strings.TrimSpace(raw) {
		switch {
		case c >= '0' && c <= '9':
			b.WriteRune(c)
		case c == '+' && i == 0:
			b.WriteRune(c)
		case c == ' ' || c == '-' || c == '(' || c == ')':
		default:
			return "", false
		}
	}
	digits := strings.TrimPrefix(b.String(), "+")
	return b.String(), len(digits) >= 7 && len(digits) <= 15
}

func checkText(field string, value string, min, max int) error {
	if n := utf8.RuneCountInString(value); n < min || n > max {
		return validation(field + " must be between " + strconv.Itoa(min) + " and " + strconv.Itoa(max) + " characters")
	}
	return nil
}
