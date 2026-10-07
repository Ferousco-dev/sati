// Package audit keeps a history of administrative and security actions.
// It never stores request bodies, passwords or tokens, only who did what to which record.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

type Entry struct {
	ActorID    string
	ActorRole  string
	Action     string
	EntityType string
	EntityID   string
	Status     int
	IP         string
	RequestID  string
	Metadata   map[string]any
}

type Record struct {
	ID         string         `json:"id"`
	ActorID    string         `json:"actor_id,omitempty"`
	ActorName  string         `json:"actor_name,omitempty"`
	ActorRole  string         `json:"actor_role,omitempty"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type,omitempty"`
	EntityID   string         `json:"entity_id,omitempty"`
	Status     int            `json:"status,omitempty"`
	IP         string         `json:"ip,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Filter struct {
	ActorID    string
	Action     string
	EntityType string
	EntityID   string
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func (Repository) Insert(ctx context.Context, db database.DBTX, e Entry) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil || e.Metadata == nil {
		meta = []byte("{}")
	}
	var actor *string
	if e.ActorID != "" {
		actor = &e.ActorID
	}
	_, err = db.Exec(ctx,
		`INSERT INTO audit_logs (actor_id, actor_role, action, entity_type, entity_id, status, ip, request_id, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		actor, e.ActorRole, e.Action, e.EntityType, e.EntityID, e.Status, e.IP, e.RequestID, meta)
	return err
}

func (Repository) List(ctx context.Context, db database.DBTX, f Filter) ([]Record, int, error) {
	where := `($1 = '' OR a.actor_id::text = $1) AND ($2 = '' OR a.action ILIKE $2) AND ($3 = '' OR a.entity_type = $3)
		AND ($4 = '' OR a.entity_id = $4) AND ($5::timestamptz IS NULL OR a.created_at >= $5)
		AND ($6::timestamptz IS NULL OR a.created_at < $6)`
	action := ""
	if f.Action != "" {
		action = "%" + escapeLike(f.Action) + "%"
	}
	args := []any{f.ActorID, action, f.EntityType, f.EntityID, f.From, f.To}

	var total int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM audit_logs a WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx,
		`SELECT a.id::text, COALESCE(a.actor_id::text, ''), COALESCE(u.name, ''), a.actor_role, a.action, a.entity_type,
		        a.entity_id, a.status, a.ip, a.request_id, a.metadata, a.created_at
		 FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id WHERE `+where+`
		 ORDER BY a.created_at DESC, a.id LIMIT $7 OFFSET $8`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Record{}
	for rows.Next() {
		var r Record
		var meta []byte
		if err := rows.Scan(&r.ID, &r.ActorID, &r.ActorName, &r.ActorRole, &r.Action, &r.EntityType, &r.EntityID,
			&r.Status, &r.IP, &r.RequestID, &meta, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(meta, &r.Metadata)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func escapeLike(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range s {
		if c == '\\' || c == '%' || c == '_' {
			out = append(out, '\\')
		}
		out = append(out, c)
	}
	return string(out)
}

// Recorder writes entries without ever failing the request that caused them.
type Recorder struct {
	pool *pgxpool.Pool
	repo *Repository
	log  *slog.Logger
}

func NewRecorder(pool *pgxpool.Pool, log *slog.Logger) *Recorder {
	return &Recorder{pool: pool, repo: NewRepository(), log: log}
}

func (r *Recorder) Record(ctx context.Context, e Entry) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := r.repo.Insert(ctx, r.pool, e); err != nil {
		r.log.Error("audit entry lost", "action", e.Action, "request_id", e.RequestID, "error", err)
	}
}
