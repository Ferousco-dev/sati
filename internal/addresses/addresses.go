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
	ZoneID        string    `json:"zone_id,omitempty"`
	ZoneName      string    `json:"zone_name,omitempty"`
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
	ZoneID        string `json:"zone_id"`
}

type UpdateInput struct {
	RecipientName *string `json:"recipient_name"`
	Phone         *string `json:"phone"`
	AddressLine   *string `json:"address_line"`
	Area          *string `json:"area"`
	City          *string `json:"city"`
	Instructions  *string `json:"instructions"`
	ZoneID        *string `json:"zone_id"`
}

var (
	ErrNotFound = apierror.New(http.StatusNotFound, "ADDRESS_NOT_FOUND", "Address not found")
	ErrZone     = apierror.New(http.StatusBadRequest, "ZONE_NOT_FOUND", "That delivery zone is not available")
	ErrLimit    = apierror.New(http.StatusConflict, "ADDRESS_LIMIT_REACHED", "You can save up to 10 addresses")
)

const (
	columns = `a.id::text, a.recipient_name, a.phone, a.address_line, a.area, a.city, a.instructions,
		COALESCE(a.zone_id::text, ''), COALESCE(z.name, ''), a.created_at, a.updated_at`
	join = ` LEFT JOIN delivery_zones z ON z.id = a.zone_id`
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func scan(row pgx.Row) (Address, error) {
	var a Address
	err := row.Scan(&a.ID, &a.RecipientName, &a.Phone, &a.AddressLine, &a.Area, &a.City, &a.Instructions, &a.ZoneID, &a.ZoneName, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Address{}, ErrNotFound
	}
	return a, err
}

func (Repository) List(ctx context.Context, db database.DBTX, userID string) ([]Address, error) {
	rows, err := db.Query(ctx, `SELECT `+columns+` FROM addresses a`+join+` WHERE a.user_id = $1 ORDER BY a.created_at DESC`, userID)
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
	return scan(db.QueryRow(ctx, `SELECT `+columns+` FROM addresses a`+join+` WHERE a.id = $1 AND a.user_id = $2`, id, userID))
}

func (Repository) Count(ctx context.Context, db database.DBTX, userID string) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM addresses WHERE user_id = $1`, userID).Scan(&n)
	return n, err
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (Repository) Create(ctx context.Context, db database.DBTX, userID string, in CreateInput) (Address, error) {
	return scan(db.QueryRow(ctx,
		`WITH a AS (
		   INSERT INTO addresses (user_id, recipient_name, phone, address_line, area, city, instructions, zone_id)
		   VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *
		 ) SELECT `+columns+` FROM a`+join,
		userID, in.RecipientName, in.Phone, in.AddressLine, in.Area, in.City, in.Instructions, nullable(in.ZoneID)))
}

func (Repository) Update(ctx context.Context, db database.DBTX, userID, id string, in UpdateInput, now time.Time) (Address, error) {
	setZone := in.ZoneID != nil
	var zone *string
	if setZone {
		zone = nullable(*in.ZoneID)
	}
	return scan(db.QueryRow(ctx,
		`WITH a AS (
		   UPDATE addresses SET
		     recipient_name = COALESCE($3, recipient_name),
		     phone = COALESCE($4, phone),
		     address_line = COALESCE($5, address_line),
		     area = COALESCE($6, area),
		     city = COALESCE($7, city),
		     instructions = COALESCE($8, instructions),
		     zone_id = CASE WHEN $9 THEN $10::uuid ELSE zone_id END,
		     updated_at = $11
		   WHERE id = $1 AND user_id = $2 RETURNING *
		 ) SELECT `+columns+` FROM a`+join,
		id, userID, in.RecipientName, in.Phone, in.AddressLine, in.Area, in.City, in.Instructions, setZone, zone, now))
}

// ZoneActive reports whether a delivery zone exists and is switched on.
func (Repository) ZoneActive(ctx context.Context, db database.DBTX, zoneID string) (bool, error) {
	if !httpx.ValidUUID(zoneID) {
		return false, nil
	}
	var active bool
	err := db.QueryRow(ctx, `SELECT active FROM delivery_zones WHERE id = $1`, zoneID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return active, err
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
