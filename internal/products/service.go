package products

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/files"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

const (
	MaxImageBytes = 5 << 20
	maxPriceKobo  = 100_000_000_00
)

var imageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

var (
	ErrImageType = apierror.New(http.StatusBadRequest, "INVALID_IMAGE", "Image must be a JPEG, PNG or WebP file")
	ErrImageSize = apierror.New(http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE", "Image must be 5MB or smaller")
)

type Service struct {
	repo    *Repository
	storage files.Storage
	log     *slog.Logger
	now     func() time.Time
}

func NewService(pool *pgxpool.Pool, storage files.Storage, log *slog.Logger) *Service {
	return &Service{repo: NewRepository(pool), storage: storage, log: log, now: time.Now}
}

type Page struct {
	Products []Product `json:"products"`
	Total    int       `json:"total"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
}

func (s *Service) ListPublic(ctx context.Context, categoryID string, limit, offset int) (Page, error) {
	return s.list(ctx, ListFilter{CategoryID: categoryID, PublicOnly: true, Limit: limit, Offset: offset})
}

func (s *Service) ListPublicInCategory(ctx context.Context, categoryID string, limit, offset int) (Page, error) {
	if !httpx.ValidUUID(categoryID) {
		return Page{}, ErrCategoryMissing
	}
	var active bool
	err := s.repo.db.QueryRow(ctx, `SELECT active FROM categories WHERE id = $1`, categoryID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
		return Page{}, ErrCategoryMissing
	}
	if err != nil {
		return Page{}, err
	}
	return s.ListPublic(ctx, categoryID, limit, offset)
}

func (s *Service) ListAdmin(ctx context.Context, categoryID string, limit, offset int) (Page, error) {
	return s.list(ctx, ListFilter{CategoryID: categoryID, Limit: limit, Offset: offset})
}

func (s *Service) list(ctx context.Context, f ListFilter) (Page, error) {
	if f.CategoryID != "" && !httpx.ValidUUID(f.CategoryID) {
		return Page{}, validation("category_id is not a valid id")
	}
	items, total, err := s.repo.List(ctx, f)
	if err != nil {
		return Page{}, err
	}
	if err := s.attachImages(ctx, items); err != nil {
		return Page{}, err
	}
	return Page{Products: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}

func (s *Service) GetPublic(ctx context.Context, id string) (Product, error) {
	p, err := s.get(ctx, id)
	if err != nil {
		return Product{}, err
	}
	visible, err := s.visibleToPublic(ctx, p)
	if err != nil {
		return Product{}, err
	}
	if !visible {
		return Product{}, ErrNotFound
	}
	return p, nil
}

func (s *Service) visibleToPublic(ctx context.Context, p Product) (bool, error) {
	if !p.Active || p.Archived {
		return false, nil
	}
	var active bool
	err := s.repo.db.QueryRow(ctx, `SELECT active FROM categories WHERE id = $1`, p.CategoryID).Scan(&active)
	return active, err
}

func (s *Service) GetAdmin(ctx context.Context, id string) (Product, error) {
	return s.get(ctx, id)
}

func (s *Service) get(ctx context.Context, id string) (Product, error) {
	if !httpx.ValidUUID(id) {
		return Product{}, ErrNotFound
	}
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return Product{}, err
	}
	return s.withImages(ctx, p)
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Product, error) {
	in.Name = strings.TrimSpace(in.Name)
	if !httpx.ValidUUID(in.CategoryID) {
		return Product{}, ErrCategoryNotFound
	}
	if err := validateFields(&in.Name, &in.Description, &in.PriceKobo); err != nil {
		return Product{}, err
	}
	p, err := s.repo.Create(ctx, in)
	if err != nil {
		return Product{}, err
	}
	return s.withImages(ctx, p)
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (Product, error) {
	if !httpx.ValidUUID(id) {
		return Product{}, ErrNotFound
	}
	if in.CategoryID == nil && in.Name == nil && in.Description == nil && in.PriceKobo == nil && in.Active == nil {
		return Product{}, validation("No fields to update")
	}
	if in.CategoryID != nil && !httpx.ValidUUID(*in.CategoryID) {
		return Product{}, ErrCategoryNotFound
	}
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
	}
	if err := validateFields(in.Name, in.Description, in.PriceKobo); err != nil {
		return Product{}, err
	}

	p, err := s.repo.Update(ctx, id, in, s.now())
	if err != nil {
		return Product{}, err
	}
	return s.withImages(ctx, p)
}

func (s *Service) Archive(ctx context.Context, id string) (Product, error) {
	if !httpx.ValidUUID(id) {
		return Product{}, ErrNotFound
	}
	p, err := s.repo.Archive(ctx, id, s.now())
	if err != nil {
		return Product{}, err
	}
	return s.withImages(ctx, p)
}

func (s *Service) SetAvailability(ctx context.Context, id string, available bool) (Product, error) {
	if !httpx.ValidUUID(id) {
		return Product{}, ErrNotFound
	}
	p, err := s.repo.SetAvailability(ctx, id, available, s.now())
	if err != nil {
		return Product{}, err
	}
	return s.withImages(ctx, p)
}

func (s *Service) AddImage(ctx context.Context, productID string, data []byte) (Image, error) {
	if len(data) > MaxImageBytes {
		return Image{}, ErrImageSize
	}
	contentType := http.DetectContentType(data)
	ext, ok := imageTypes[contentType]
	if !ok {
		return Image{}, ErrImageType
	}

	if _, err := s.get(ctx, productID); err != nil {
		return Image{}, err
	}

	key := "products/" + productID + "/" + newKeyID() + ext
	if err := s.storage.Put(ctx, key, bytes.NewReader(data)); err != nil {
		return Image{}, err
	}

	row, err := s.repo.AddImage(ctx, productID, key, contentType, len(data))
	if err != nil {
		if delErr := s.storage.Delete(ctx, key); delErr != nil {
			s.log.Error("remove orphaned image", "key", key, "error", delErr)
		}
		return Image{}, err
	}
	return s.toImage(row), nil
}

func (s *Service) DeleteImage(ctx context.Context, productID, imageID string) error {
	if !httpx.ValidUUID(productID) || !httpx.ValidUUID(imageID) {
		return ErrImageNotFound
	}
	key, err := s.repo.DeleteImage(ctx, productID, imageID)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, key); err != nil {
		s.log.Error("remove image file", "key", key, "error", err)
	}
	return nil
}

func (s *Service) withImages(ctx context.Context, p Product) (Product, error) {
	items := []Product{p}
	if err := s.attachImages(ctx, items); err != nil {
		return Product{}, err
	}
	return items[0], nil
}

func (s *Service) attachImages(ctx context.Context, items []Product) error {
	ids := make([]string, len(items))
	for i, p := range items {
		ids[i] = p.ID
	}
	byProduct, err := s.repo.ImagesFor(ctx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		items[i].Images = []Image{}
		for _, row := range byProduct[items[i].ID] {
			items[i].Images = append(items[i].Images, s.toImage(row))
		}
	}
	return nil
}

func (s *Service) toImage(row imageRow) Image {
	return Image{ID: row.ID, URL: s.storage.URL(row.Key), Position: row.Position}
}

func validation(msg string) *apierror.Error {
	return apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", msg)
}

func validateFields(name, description *string, price *int64) error {
	if name != nil {
		if n := utf8.RuneCountInString(*name); n < 1 || n > 150 {
			return validation("Name must be between 1 and 150 characters")
		}
	}
	if description != nil && utf8.RuneCountInString(*description) > 2000 {
		return validation("Description must be at most 2000 characters")
	}
	if price != nil && (*price < 0 || *price > maxPriceKobo) {
		return validation("price_kobo must be between 0 and 10000000000")
	}
	return nil
}
