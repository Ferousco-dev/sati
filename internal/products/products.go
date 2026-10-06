package products

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

const Currency = "NGN"

type Image struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Position int    `json:"position"`
}

type Product struct {
	ID          string    `json:"id"`
	CategoryID  string    `json:"category_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PriceKobo   int64     `json:"price_kobo"`
	Currency    string    `json:"currency"`
	Active      bool      `json:"active"`
	Available   bool      `json:"available"`
	Archived    bool      `json:"archived"`
	Images      []Image   `json:"images"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateInput struct {
	CategoryID  string `json:"category_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PriceKobo   int64  `json:"price_kobo"`
}

type UpdateInput struct {
	CategoryID  *string `json:"category_id"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	PriceKobo   *int64  `json:"price_kobo"`
	Active      *bool   `json:"active"`
}

type ListFilter struct {
	CategoryID string
	PublicOnly bool
	Limit      int
	Offset     int
}

var (
	ErrNotFound         = apierror.New(http.StatusNotFound, "PRODUCT_NOT_FOUND", "Product not found")
	ErrCategoryNotFound = apierror.New(http.StatusBadRequest, "CATEGORY_NOT_FOUND", "Category does not exist")
	ErrCategoryMissing  = apierror.New(http.StatusNotFound, "CATEGORY_NOT_FOUND", "Category not found")
	ErrImageNotFound    = apierror.New(http.StatusNotFound, "IMAGE_NOT_FOUND", "Image not found")
)

const (
	columns = `p.id::text, p.category_id::text, p.name, p.description, p.price_kobo,
	           p.active, p.available, p.archived_at IS NOT NULL, p.created_at, p.updated_at`

	publicWhere = `(NOT $2 OR (p.active AND p.archived_at IS NULL AND c.active))`
)

type imageRow struct {
	ID        string
	ProductID string
	Key       string
	Position  int
}

type Repository struct{ db database.DBTX }

func NewRepository(db database.DBTX) *Repository { return &Repository{db: db} }

func scan(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Description, &p.PriceKobo,
		&p.Active, &p.Available, &p.Archived, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return Product{}, ErrCategoryNotFound
	}
	p.Currency = Currency
	return p, err
}

func (r *Repository) List(ctx context.Context, f ListFilter) ([]Product, int, error) {
	var category *string
	if f.CategoryID != "" {
		category = &f.CategoryID
	}

	var total int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM products p JOIN categories c ON c.id = p.category_id
		 WHERE ($1::uuid IS NULL OR p.category_id = $1) AND `+publicWhere,
		category, f.PublicOnly).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx,
		`SELECT `+columns+` FROM products p JOIN categories c ON c.id = p.category_id
		 WHERE ($1::uuid IS NULL OR p.category_id = $1) AND `+publicWhere+`
		 ORDER BY p.name, p.id LIMIT $3 OFFSET $4`,
		category, f.PublicOnly, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Product{}
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Product, error) {
	return scan(r.db.QueryRow(ctx, `SELECT `+columns+` FROM products p WHERE p.id = $1`, id))
}

func (r *Repository) Create(ctx context.Context, in CreateInput) (Product, error) {
	return scan(r.db.QueryRow(ctx,
		`WITH p AS (
		   INSERT INTO products (category_id, name, description, price_kobo)
		   VALUES ($1, $2, $3, $4) RETURNING *
		 ) SELECT `+columns+` FROM p`,
		in.CategoryID, in.Name, in.Description, in.PriceKobo))
}

func (r *Repository) Update(ctx context.Context, id string, in UpdateInput, now time.Time) (Product, error) {
	return scan(r.db.QueryRow(ctx,
		`WITH p AS (
		   UPDATE products SET
		     category_id = COALESCE($2::uuid, category_id),
		     name = COALESCE($3, name),
		     description = COALESCE($4, description),
		     price_kobo = COALESCE($5, price_kobo),
		     active = COALESCE($6, active),
		     updated_at = $7
		   WHERE id = $1 AND archived_at IS NULL RETURNING *
		 ) SELECT `+columns+` FROM p`,
		id, in.CategoryID, in.Name, in.Description, in.PriceKobo, in.Active, now))
}

func (r *Repository) Archive(ctx context.Context, id string, now time.Time) (Product, error) {
	return scan(r.db.QueryRow(ctx,
		`WITH p AS (
		   UPDATE products SET archived_at = COALESCE(archived_at, $2), available = FALSE, updated_at = $2
		   WHERE id = $1 RETURNING *
		 ) SELECT `+columns+` FROM p`, id, now))
}

func (r *Repository) SetAvailability(ctx context.Context, id string, available bool, now time.Time) (Product, error) {
	return scan(r.db.QueryRow(ctx,
		`WITH p AS (
		   UPDATE products SET available = $2, updated_at = $3
		   WHERE id = $1 AND archived_at IS NULL RETURNING *
		 ) SELECT `+columns+` FROM p`, id, available, now))
}

func (r *Repository) ImagesFor(ctx context.Context, productIDs []string) (map[string][]imageRow, error) {
	out := map[string][]imageRow{}
	if len(productIDs) == 0 {
		return out, nil
	}

	rows, err := r.db.Query(ctx,
		`SELECT id::text, product_id::text, storage_key, position FROM product_images
		 WHERE product_id = ANY($1::uuid[]) ORDER BY position, created_at`, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var im imageRow
		if err := rows.Scan(&im.ID, &im.ProductID, &im.Key, &im.Position); err != nil {
			return nil, err
		}
		out[im.ProductID] = append(out[im.ProductID], im)
	}
	return out, rows.Err()
}

func (r *Repository) AddImage(ctx context.Context, productID, key, contentType string, size int) (imageRow, error) {
	var im imageRow
	err := r.db.QueryRow(ctx,
		`INSERT INTO product_images (product_id, storage_key, content_type, size_bytes, position)
		 VALUES ($1, $2, $3, $4,
		   (SELECT COALESCE(MAX(position) + 1, 0) FROM product_images WHERE product_id = $1))
		 RETURNING id::text, product_id::text, storage_key, position`,
		productID, key, contentType, size).Scan(&im.ID, &im.ProductID, &im.Key, &im.Position)
	return im, err
}

func (r *Repository) DeleteImage(ctx context.Context, productID, imageID string) (string, error) {
	var key string
	err := r.db.QueryRow(ctx,
		`DELETE FROM product_images WHERE id = $1 AND product_id = $2 RETURNING storage_key`,
		imageID, productID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrImageNotFound
	}
	return key, err
}
