package categories

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

type Category struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SortOrder   int       `json:"sort_order"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

type UpdateInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	SortOrder   *int    `json:"sort_order"`
	Active      *bool   `json:"active"`
}

var (
	ErrNotFound  = apierror.New(http.StatusNotFound, "CATEGORY_NOT_FOUND", "Category not found")
	ErrNameTaken = apierror.New(http.StatusConflict, "CATEGORY_NAME_TAKEN", "A category with this name already exists")
)

const columns = `id::text, name, description, sort_order, active, created_at, updated_at`

type Repository struct{ db database.DBTX }

func NewRepository(db database.DBTX) *Repository { return &Repository{db: db} }

func scan(row pgx.Row) (Category, error) {
	var c Category
	err := row.Scan(&c.ID, &c.Name, &c.Description, &c.SortOrder, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Category{}, ErrNameTaken
	}
	return c, err
}

func (r *Repository) List(ctx context.Context, onlyActive bool) ([]Category, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+columns+` FROM categories WHERE (NOT $1 OR active) ORDER BY sort_order, name`, onlyActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Category{}
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Category, error) {
	return scan(r.db.QueryRow(ctx, `SELECT `+columns+` FROM categories WHERE id = $1`, id))
}

func (r *Repository) Create(ctx context.Context, in CreateInput) (Category, error) {
	return scan(r.db.QueryRow(ctx,
		`INSERT INTO categories (name, description, sort_order) VALUES ($1, $2, $3) RETURNING `+columns,
		in.Name, in.Description, in.SortOrder))
}

func (r *Repository) Update(ctx context.Context, id string, in UpdateInput, now time.Time) (Category, error) {
	return scan(r.db.QueryRow(ctx,
		`UPDATE categories SET
		   name = COALESCE($2, name),
		   description = COALESCE($3, description),
		   sort_order = COALESCE($4, sort_order),
		   active = COALESCE($5, active),
		   updated_at = $6
		 WHERE id = $1 RETURNING `+columns,
		id, in.Name, in.Description, in.SortOrder, in.Active, now))
}

type Service struct {
	repo *Repository
	now  func() time.Time
}

func NewService(repo *Repository) *Service { return &Service{repo: repo, now: time.Now} }

func (s *Service) Repository() *Repository { return s.repo }

func (s *Service) ListPublic(ctx context.Context) ([]Category, error) {
	return s.repo.List(ctx, true)
}

func (s *Service) ListAll(ctx context.Context) ([]Category, error) {
	return s.repo.List(ctx, false)
}

func (s *Service) GetPublic(ctx context.Context, id string) (Category, error) {
	if !httpx.ValidUUID(id) {
		return Category{}, ErrNotFound
	}
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return Category{}, err
	}
	if !c.Active {
		return Category{}, ErrNotFound
	}
	return c, nil
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Category, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := validateName(in.Name); err != nil {
		return Category{}, err
	}
	if err := validateDescription(in.Description); err != nil {
		return Category{}, err
	}
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (Category, error) {
	if !httpx.ValidUUID(id) {
		return Category{}, ErrNotFound
	}
	if in.Name == nil && in.Description == nil && in.SortOrder == nil && in.Active == nil {
		return Category{}, validation("No fields to update")
	}
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if err := validateName(trimmed); err != nil {
			return Category{}, err
		}
	}
	if in.Description != nil {
		if err := validateDescription(*in.Description); err != nil {
			return Category{}, err
		}
	}
	return s.repo.Update(ctx, id, in, s.now())
}

func validation(msg string) *apierror.Error {
	return apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", msg)
}

func validateName(name string) error {
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		return validation("Name must be between 1 and 100 characters")
	}
	return nil
}

func validateDescription(d string) error {
	if utf8.RuneCountInString(d) > 1000 {
		return validation("Description must be at most 1000 characters")
	}
	return nil
}
