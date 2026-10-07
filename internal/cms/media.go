package cms

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

const MaxUploadBytes = 5 << 20

var imageTypes = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

var (
	ErrMediaNotFound = apierror.New(http.StatusNotFound, "MEDIA_NOT_FOUND", "Picture not found")
	ErrMediaInUse    = apierror.New(http.StatusConflict, "MEDIA_IN_USE", "This picture is used by the live website or a draft. Replace it there first")
	ErrBadImage      = apierror.New(http.StatusBadRequest, "INVALID_IMAGE", "Upload a JPEG, PNG or WebP picture")
	ErrImageTooBig   = apierror.New(http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE", "The picture must be 5MB or smaller")
)

type Media struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	URL         string    `json:"url"`
	ContentType string    `json:"content_type"`
	SizeBytes   int       `json:"size_bytes"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	AltText     string    `json:"alt_text"`
	CreatedAt   time.Time `json:"created_at"`

	storageKey string
}

type mediaRepo struct{}

const mediaColumns = `id::text, filename, storage_key, content_type, size_bytes, width, height, alt_text, created_at`

func scanMedia(row pgx.Row) (Media, error) {
	var m Media
	err := row.Scan(&m.ID, &m.Filename, &m.storageKey, &m.ContentType, &m.SizeBytes, &m.Width, &m.Height, &m.AltText, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Media{}, ErrMediaNotFound
	}
	return m, err
}

func (mediaRepo) insert(ctx context.Context, db database.DBTX, m Media, uploadedBy string) (Media, error) {
	var by *string
	if uploadedBy != "" {
		by = &uploadedBy
	}
	return scanMedia(db.QueryRow(ctx,
		`INSERT INTO media (filename, storage_key, content_type, size_bytes, width, height, alt_text, uploaded_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+mediaColumns,
		m.Filename, m.storageKey, m.ContentType, m.SizeBytes, m.Width, m.Height, m.AltText, by))
}

func (mediaRepo) get(ctx context.Context, db database.DBTX, id string) (Media, error) {
	if !httpx.ValidUUID(id) {
		return Media{}, ErrMediaNotFound
	}
	return scanMedia(db.QueryRow(ctx, `SELECT `+mediaColumns+` FROM media WHERE id = $1`, id))
}

func (mediaRepo) byIDs(ctx context.Context, db database.DBTX, ids []string) (map[string]Media, error) {
	out := map[string]Media{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `SELECT `+mediaColumns+` FROM media WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		out[m.ID] = m
	}
	return out, rows.Err()
}

func (mediaRepo) list(ctx context.Context, db database.DBTX, query string, limit, offset int) ([]Media, int, error) {
	pattern := ""
	if query != "" {
		pattern = "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query) + "%"
	}
	var total int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM media WHERE ($1 = '' OR filename ILIKE $1 OR alt_text ILIKE $1)`, pattern).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.Query(ctx,
		`SELECT `+mediaColumns+` FROM media WHERE ($1 = '' OR filename ILIKE $1 OR alt_text ILIKE $1)
		 ORDER BY created_at DESC, id LIMIT $2 OFFSET $3`, pattern, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Media{}
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

func (mediaRepo) setAlt(ctx context.Context, db database.DBTX, id, alt string) (Media, error) {
	return scanMedia(db.QueryRow(ctx, `UPDATE media SET alt_text = $2 WHERE id = $1 RETURNING `+mediaColumns, id, alt))
}

// inUse reports whether a picture is part of a draft or of the version that is live.
// Older versions do not keep a picture alive, and rolling back to one that lost its picture is refused.
func (mediaRepo) inUse(ctx context.Context, db database.DBTX, id string) (bool, error) {
	var used bool
	err := db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM content_versions v JOIN content_sections c
		                   ON c.key = v.section_key AND c.published_version = v.version WHERE v.data::text LIKE $1)
		     OR EXISTS (SELECT 1 FROM content_sections WHERE draft::text LIKE $1)`, "%"+id+"%").Scan(&used)
	return used, err
}

func (mediaRepo) delete(ctx context.Context, db database.DBTX, id string) (string, error) {
	var key string
	err := db.QueryRow(ctx, `DELETE FROM media WHERE id = $1 RETURNING storage_key`, id).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMediaNotFound
	}
	return key, err
}

func randomName() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func cleanFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		return "picture"
	}
	if utf8.RuneCountInString(name) > 120 {
		runes := []rune(name)
		name = string(runes[:120])
	}
	return name
}

func (s *Service) Upload(ctx context.Context, uploadedBy, filename, alt string, data []byte) (Media, error) {
	if len(data) > MaxUploadBytes {
		return Media{}, ErrImageTooBig
	}
	contentType := http.DetectContentType(data)
	ext, ok := imageTypes[contentType]
	if !ok {
		return Media{}, ErrBadImage
	}
	alt = strings.TrimSpace(alt)
	if utf8.RuneCountInString(alt) > 200 {
		return Media{}, validation("alt text must be at most 200 characters")
	}

	m := Media{
		Filename: cleanFilename(filename), ContentType: contentType, SizeBytes: len(data), AltText: alt,
		storageKey: "media/" + randomName() + ext,
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		m.Width, m.Height = cfg.Width, cfg.Height
	}

	if err := s.storage.Put(ctx, m.storageKey, bytes.NewReader(data)); err != nil {
		return Media{}, err
	}
	saved, err := s.media.insert(ctx, s.pool, m, uploadedBy)
	if err != nil {
		if delErr := s.storage.Delete(ctx, m.storageKey); delErr != nil {
			s.log.Error("remove orphaned upload", "key", m.storageKey, "error", delErr)
		}
		return Media{}, err
	}
	return s.withURL(saved), nil
}

func (s *Service) withURL(m Media) Media {
	m.URL = s.storage.URL(m.storageKey)
	return m
}

func (s *Service) ListMedia(ctx context.Context, query string, limit, offset int) ([]Media, int, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > 100 {
		return nil, 0, validation("Search text is too long")
	}
	items, total, err := s.media.list(ctx, s.pool, query, limit, offset)
	for i := range items {
		items[i] = s.withURL(items[i])
	}
	return items, total, err
}

func (s *Service) UpdateAlt(ctx context.Context, id, alt string) (Media, error) {
	if !httpx.ValidUUID(id) {
		return Media{}, ErrMediaNotFound
	}
	alt = strings.TrimSpace(alt)
	if utf8.RuneCountInString(alt) > 200 {
		return Media{}, validation("alt text must be at most 200 characters")
	}
	m, err := s.media.setAlt(ctx, s.pool, id, alt)
	return s.withURL(m), err
}

func (s *Service) DeleteMedia(ctx context.Context, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrMediaNotFound
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.media.get(ctx, tx, id); err != nil {
			return err
		}
		if used, err := s.media.inUse(ctx, tx, id); err != nil {
			return err
		} else if used {
			return ErrMediaInUse
		}
		key, err := s.media.delete(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.storage.Delete(ctx, key); err != nil {
			s.log.Error("remove media file", "key", key, "error", err)
		}
		return nil
	})
}
