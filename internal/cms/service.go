// Package cms holds the website content that admins edit without code:
// fixed sections with drafts, publishing and history, and a picture library.
package cms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/files"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

var (
	ErrUnknownSection = apierror.New(http.StatusNotFound, "SECTION_NOT_FOUND", "There is no such content section")
	ErrNoDraft        = apierror.New(http.StatusConflict, "NO_DRAFT", "There is no draft to publish")
	ErrNoVersion      = apierror.New(http.StatusNotFound, "VERSION_NOT_FOUND", "That version does not exist")
)

func validation(msg string) *apierror.Error {
	return apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", msg)
}

type Service struct {
	pool    *pgxpool.Pool
	storage files.Storage
	media   mediaRepo
	log     *slog.Logger
	now     func() time.Time
}

func NewService(pool *pgxpool.Pool, storage files.Storage, log *slog.Logger) *Service {
	return &Service{pool: pool, storage: storage, log: log, now: time.Now}
}

type stored struct {
	key              string
	publishedVersion int
	draft            json.RawMessage
	draftUpdatedAt   *time.Time
	draftUpdatedBy   string
	updatedAt        *time.Time
	published        json.RawMessage
}

// load reads one section: its draft and the published content (or the built-in default).
func (s *Service) load(ctx context.Context, db database.DBTX, key string, lock bool) (stored, error) {
	st := stored{key: key}
	query := `SELECT c.published_version, c.draft, c.draft_updated_at, COALESCE(u.name, ''), c.updated_at,
	                 (SELECT v.data FROM content_versions v WHERE v.section_key = c.key AND v.version = c.published_version)
	          FROM content_sections c LEFT JOIN users u ON u.id = c.draft_updated_by WHERE c.key = $1`
	if lock {
		query += ` FOR UPDATE OF c`
	}
	err := db.QueryRow(ctx, query, key).Scan(&st.publishedVersion, &st.draft, &st.draftUpdatedAt, &st.draftUpdatedBy, &st.updatedAt, &st.published)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stored{}, err
	}
	if len(st.published) == 0 {
		st.published = defaultData(key)
	}
	return st, nil
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ax, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return bytes.Equal(ax, by)
}

type SectionState struct {
	Key                  string          `json:"key"`
	Title                string          `json:"title"`
	PublishedVersion     int             `json:"published_version"`
	Published            json.RawMessage `json:"published"`
	Draft                json.RawMessage `json:"draft"`
	HasUnpublishedChange bool            `json:"has_unpublished_changes"`
	DraftUpdatedAt       *time.Time      `json:"draft_updated_at"`
	DraftUpdatedBy       string          `json:"draft_updated_by,omitempty"`
	UpdatedAt            *time.Time      `json:"updated_at"`
}

func (s *Service) state(ctx context.Context, db database.DBTX, key string, resolve bool) (SectionState, error) {
	def, ok := findDef(key)
	if !ok {
		return SectionState{}, ErrUnknownSection
	}
	st, err := s.load(ctx, db, key, false)
	if err != nil {
		return SectionState{}, err
	}

	out := SectionState{
		Key: key, Title: def.Title, PublishedVersion: st.publishedVersion, Published: st.published,
		DraftUpdatedAt: st.draftUpdatedAt, DraftUpdatedBy: st.draftUpdatedBy, UpdatedAt: st.updatedAt,
	}
	if len(st.draft) > 0 {
		out.Draft = st.draft
		out.HasUnpublishedChange = !sameJSON(st.draft, st.published)
	}
	if resolve {
		if out.Published, err = s.resolve(ctx, db, out.Published); err != nil {
			return SectionState{}, err
		}
		if len(out.Draft) > 0 {
			if out.Draft, err = s.resolve(ctx, db, out.Draft); err != nil {
				return SectionState{}, err
			}
		}
	}
	return out, nil
}

func (s *Service) Section(ctx context.Context, key string) (SectionState, error) {
	return s.state(ctx, s.pool, key, true)
}

func (s *Service) List(ctx context.Context) ([]SectionState, error) {
	out := make([]SectionState, 0, len(sectionDefs))
	for _, def := range sectionDefs {
		st, err := s.state(ctx, s.pool, def.Key, false)
		if err != nil {
			return nil, err
		}
		st.Published, st.Draft = nil, nil
		out = append(out, st)
	}
	return out, nil
}

// SaveDraft validates and stores an unpublished version. Nothing visitors see changes yet.
func (s *Service) SaveDraft(ctx context.Context, userID, key string, raw json.RawMessage) (SectionState, error) {
	if _, ok := findDef(key); !ok {
		return SectionState{}, ErrUnknownSection
	}
	clean, mediaIDs, err := Validate(key, raw)
	if err != nil {
		return SectionState{}, validation(err.Error())
	}
	if err := s.requireMedia(ctx, mediaIDs); err != nil {
		return SectionState{}, err
	}

	now := s.now()
	var by *string
	if userID != "" {
		by = &userID
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO content_sections (key, draft, draft_updated_at, draft_updated_by, updated_at)
		 VALUES ($1, $2, $3, $4, $3)
		 ON CONFLICT (key) DO UPDATE SET draft = EXCLUDED.draft, draft_updated_at = EXCLUDED.draft_updated_at,
		   draft_updated_by = EXCLUDED.draft_updated_by`, key, clean, now, by)
	if err != nil {
		return SectionState{}, err
	}
	return s.state(ctx, s.pool, key, true)
}

func (s *Service) DiscardDraft(ctx context.Context, key string) error {
	if _, ok := findDef(key); !ok {
		return ErrUnknownSection
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE content_sections SET draft = NULL, draft_updated_at = NULL, draft_updated_by = NULL WHERE key = $1`, key)
	return err
}

func cleanNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 200 {
		return "", validation("note must be at most 200 characters")
	}
	return note, nil
}

// publish stores data as the next version and makes it the live one.
func (s *Service) publish(ctx context.Context, tx pgx.Tx, key string, data json.RawMessage, userID, note string, clearDraft bool) (int, error) {
	now := s.now()
	var by *string
	if userID != "" {
		by = &userID
	}
	var version int
	err := tx.QueryRow(ctx,
		`INSERT INTO content_versions (section_key, version, data, note, published_by, published_at)
		 VALUES ($1, (SELECT COALESCE(MAX(version), 0) + 1 FROM content_versions WHERE section_key = $1), $2, $3, $4, $5)
		 RETURNING version`, key, data, note, by, now).Scan(&version)
	if err != nil {
		return 0, err
	}
	if clearDraft {
		_, err = tx.Exec(ctx,
			`UPDATE content_sections SET published_version = $2, updated_at = $3, draft = NULL, draft_updated_at = NULL, draft_updated_by = NULL WHERE key = $1`,
			key, version, now)
	} else {
		_, err = tx.Exec(ctx, `UPDATE content_sections SET published_version = $2, updated_at = $3 WHERE key = $1`, key, version, now)
	}
	return version, err
}

// Publish makes the draft live as a new version.
func (s *Service) Publish(ctx context.Context, userID, key, note string) (SectionState, error) {
	if _, ok := findDef(key); !ok {
		return SectionState{}, ErrUnknownSection
	}
	note, err := cleanNote(note)
	if err != nil {
		return SectionState{}, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		st, err := s.load(ctx, tx, key, true)
		if err != nil {
			return err
		}
		if len(st.draft) == 0 {
			return ErrNoDraft
		}
		_, err = s.publish(ctx, tx, key, st.draft, userID, note, true)
		return err
	})
	if err != nil {
		return SectionState{}, err
	}
	return s.state(ctx, s.pool, key, true)
}

type Version struct {
	Version     int       `json:"version"`
	Note        string    `json:"note"`
	PublishedBy string    `json:"published_by,omitempty"`
	PublishedAt time.Time `json:"published_at"`
	Live        bool      `json:"live"`
}

func (s *Service) Versions(ctx context.Context, key string) ([]Version, error) {
	if _, ok := findDef(key); !ok {
		return nil, ErrUnknownSection
	}
	rows, err := s.pool.Query(ctx,
		`SELECT v.version, v.note, COALESCE(u.name, ''), v.published_at, v.version = c.published_version
		 FROM content_versions v JOIN content_sections c ON c.key = v.section_key
		 LEFT JOIN users u ON u.id = v.published_by WHERE v.section_key = $1 ORDER BY v.version DESC`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Version{}
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.Version, &v.Note, &v.PublishedBy, &v.PublishedAt, &v.Live); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Rollback publishes an old version again as a new version, so history is never rewritten.
func (s *Service) Rollback(ctx context.Context, userID, key string, version int, note string) (SectionState, error) {
	if _, ok := findDef(key); !ok {
		return SectionState{}, ErrUnknownSection
	}
	note, err := cleanNote(note)
	if err != nil {
		return SectionState{}, err
	}
	if note == "" {
		note = fmt.Sprintf("Rolled back to version %d", version)
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.load(ctx, tx, key, true); err != nil {
			return err
		}
		var data json.RawMessage
		err := tx.QueryRow(ctx, `SELECT data FROM content_versions WHERE section_key = $1 AND version = $2`, key, version).Scan(&data)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoVersion
		}
		if err != nil {
			return err
		}
		var ids []string
		var parsed any
		if json.Unmarshal(data, &parsed) == nil {
			collect(parsed, &ids)
		}
		found, err := s.media.byIDs(ctx, tx, dedupe(ids))
		if err != nil {
			return err
		}
		if len(found) != len(dedupe(ids)) {
			return apierror.New(http.StatusConflict, "VERSION_PICTURE_MISSING", "That version uses a picture that was deleted from the library")
		}
		_, err = s.publish(ctx, tx, key, data, userID, note, false)
		return err
	})
	if err != nil {
		return SectionState{}, err
	}
	return s.state(ctx, s.pool, key, true)
}

func (s *Service) requireMedia(ctx context.Context, ids []string) error {
	found, err := s.media.byIDs(ctx, s.pool, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, ok := found[id]; !ok {
			return validation("A picture used in this section is not in the library: " + id)
		}
	}
	return nil
}

// resolve adds url, width and height to every picture reference in a section.
func (s *Service) resolve(ctx context.Context, db database.DBTX, raw json.RawMessage) (json.RawMessage, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	var ids []string
	collect(v, &ids)
	found, err := s.media.byIDs(ctx, db, dedupe(ids))
	if err != nil {
		return nil, err
	}
	fill(v, found, s.storage)
	return json.Marshal(v)
}

func collect(v any, ids *[]string) {
	switch t := v.(type) {
	case map[string]any:
		if id, ok := t["media_id"].(string); ok {
			*ids = append(*ids, id)
		}
		for _, child := range t {
			collect(child, ids)
		}
	case []any:
		for _, child := range t {
			collect(child, ids)
		}
	}
}

func fill(v any, found map[string]Media, storage files.Storage) {
	switch t := v.(type) {
	case map[string]any:
		if id, ok := t["media_id"].(string); ok {
			if m, ok := found[id]; ok {
				t["url"], t["width"], t["height"] = storage.URL(m.storageKey), m.Width, m.Height
				if alt, _ := t["alt"].(string); alt == "" {
					t["alt"] = m.AltText
				}
			} else {
				t["url"] = ""
			}
		}
		for _, child := range t {
			fill(child, found, storage)
		}
	case []any:
		for _, child := range t {
			fill(child, found, storage)
		}
	}
}

type Content struct {
	Sections  map[string]json.RawMessage `json:"sections"`
	UpdatedAt *time.Time                 `json:"updated_at"`
}

// Site returns every section for the public website. With preview set, drafts are used
// where they exist so an admin can see changes before publishing.
func (s *Service) Site(ctx context.Context, preview bool) (Content, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT c.key, c.draft, c.updated_at,
		        (SELECT v.data FROM content_versions v WHERE v.section_key = c.key AND v.version = c.published_version)
		 FROM content_sections c`)
	if err != nil {
		return Content{}, err
	}
	data := map[string]json.RawMessage{}
	var latest *time.Time
	for rows.Next() {
		var key string
		var draft, published json.RawMessage
		var updated time.Time
		if err := rows.Scan(&key, &draft, &updated, &published); err != nil {
			rows.Close()
			return Content{}, err
		}
		if latest == nil || updated.After(*latest) {
			u := updated
			latest = &u
		}
		switch {
		case preview && len(draft) > 0:
			data[key] = draft
		case len(published) > 0:
			data[key] = published
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Content{}, err
	}

	out := Content{Sections: map[string]json.RawMessage{}, UpdatedAt: latest}
	for _, def := range sectionDefs {
		raw, ok := data[def.Key]
		if !ok {
			raw = defaultData(def.Key)
		}
		if out.Sections[def.Key], err = s.resolve(ctx, s.pool, raw); err != nil {
			return Content{}, err
		}
	}
	return out, nil
}
