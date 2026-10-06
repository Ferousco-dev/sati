package orders

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

const columns = `o.id::text, o.order_number, o.status::text, o.payment_status::text, o.payment_method::text,
	o.fulfillment_type::text, o.subtotal_kobo, o.delivery_fee_kobo, o.total_kobo, o.delivery_address,
	o.notes, o.status_reason, o.created_at, o.updated_at`

type productRow struct {
	ID        string
	Name      string
	PriceKobo int64
	Orderable bool
}

type newOrder struct {
	CustomerID      string
	Fulfillment     Fulfillment
	SubtotalKobo    int64
	DeliveryFeeKobo int64
	TotalKobo       int64
	PaymentMethod   PaymentMethod
	AddressID       *string
	AddressSnapshot []byte
	Notes           string
	IdempotencyKey  *string
}

type newItem struct {
	ProductID     string
	Name          string
	UnitPriceKobo int64
	Quantity      int32
	LineTotalKobo int64
}

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

func scan(row pgx.Row, extra ...any) (Order, error) {
	var o Order
	var number int64
	var status, fulfillment, method string
	var snapshot []byte
	dest := append([]any{&o.ID, &number, &status, &o.PaymentStatus, &method, &fulfillment,
		&o.SubtotalKobo, &o.DeliveryFeeKobo, &o.TotalKobo, &snapshot, &o.Notes, &o.StatusReason, &o.CreatedAt, &o.UpdatedAt}, extra...)
	err := row.Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}

	o.Number = FormatNumber(number)
	o.Status = Status(status)
	o.PaymentMethod = PaymentMethod(method)
	o.Fulfillment = Fulfillment(fulfillment)
	o.Currency = Currency
	o.Items = []Item{}
	if len(snapshot) > 0 {
		var a addresses.Address
		if err := json.Unmarshal(snapshot, &a); err != nil {
			return Order{}, err
		}
		o.DeliveryAddress = &a
	}
	return o, nil
}

// LockProducts reads products with a shared row lock so a price or
// availability change cannot slip in before the order commits.
func (Repository) LockProducts(ctx context.Context, db database.DBTX, ids []string) (map[string]productRow, error) {
	rows, err := db.Query(ctx,
		`SELECT p.id::text, p.name, p.price_kobo,
		        (p.active AND p.available AND p.archived_at IS NULL AND c.active)
		 FROM products p JOIN categories c ON c.id = p.category_id
		 WHERE p.id = ANY($1::uuid[]) FOR SHARE OF p`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]productRow, len(ids))
	for rows.Next() {
		var p productRow
		if err := rows.Scan(&p.ID, &p.Name, &p.PriceKobo, &p.Orderable); err != nil {
			return nil, err
		}
		out[p.ID] = p
	}
	return out, rows.Err()
}

var errDuplicateKey = errors.New("duplicate idempotency key")

func (Repository) Insert(ctx context.Context, db database.DBTX, in newOrder, items []newItem) (string, int64, error) {
	var id string
	var number int64
	err := db.QueryRow(ctx,
		`INSERT INTO orders (customer_id, fulfillment_type, subtotal_kobo, delivery_fee_kobo, total_kobo,
		                     payment_method, delivery_address_id, delivery_address, notes, idempotency_key)
		 VALUES ($1, $2::fulfillment_type, $3, $4, $5, $6::payment_method, $7, $8::jsonb, $9, $10)
		 RETURNING id::text, order_number`,
		in.CustomerID, string(in.Fulfillment), in.SubtotalKobo, in.DeliveryFeeKobo, in.TotalKobo,
		string(in.PaymentMethod), in.AddressID, in.AddressSnapshot, in.Notes, in.IdempotencyKey).Scan(&id, &number)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return "", 0, errDuplicateKey
	}
	if err != nil {
		return "", 0, err
	}

	productIDs := make([]string, len(items))
	names := make([]string, len(items))
	prices := make([]int64, len(items))
	quantities := make([]int32, len(items))
	totals := make([]int64, len(items))
	for i, it := range items {
		productIDs[i], names[i], prices[i], quantities[i], totals[i] = it.ProductID, it.Name, it.UnitPriceKobo, it.Quantity, it.LineTotalKobo
	}
	_, err = db.Exec(ctx,
		`INSERT INTO order_items (order_id, product_id, product_name_snapshot, unit_price_snapshot_kobo, quantity, line_total_kobo)
		 SELECT $1, * FROM unnest($2::uuid[], $3::text[], $4::bigint[], $5::int[], $6::bigint[])`,
		id, productIDs, names, prices, quantities, totals)
	return id, number, err
}

func (Repository) ByID(ctx context.Context, db database.DBTX, customerID, id string) (Order, error) {
	return scan(db.QueryRow(ctx, `SELECT `+columns+` FROM orders o WHERE o.id = $1 AND o.customer_id = $2`, id, customerID))
}

func (Repository) ByIdempotencyKey(ctx context.Context, db database.DBTX, customerID, key string) (Order, error) {
	return scan(db.QueryRow(ctx,
		`SELECT `+columns+` FROM orders o WHERE o.customer_id = $1 AND o.idempotency_key = $2`, customerID, key))
}

func (Repository) ListByCustomer(ctx context.Context, db database.DBTX, customerID string, limit, offset int) ([]Order, int, error) {
	var total int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE customer_id = $1`, customerID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(ctx,
		`SELECT `+columns+` FROM orders o WHERE o.customer_id = $1
		 ORDER BY o.created_at DESC, o.id LIMIT $2 OFFSET $3`, customerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Order{}
	for rows.Next() {
		o, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, o)
	}
	return out, total, rows.Err()
}

func (Repository) ItemsFor(ctx context.Context, db database.DBTX, orderIDs []string) (map[string][]Item, error) {
	out := map[string][]Item{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx,
		`SELECT order_id::text, product_id::text, product_name_snapshot, unit_price_snapshot_kobo, quantity, line_total_kobo
		 FROM order_items WHERE order_id = ANY($1::uuid[]) ORDER BY product_name_snapshot, id`, orderIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var orderID string
		var it Item
		if err := rows.Scan(&orderID, &it.ProductID, &it.Name, &it.UnitPriceKobo, &it.Quantity, &it.LineTotalKobo); err != nil {
			return nil, err
		}
		out[orderID] = append(out[orderID], it)
	}
	return out, rows.Err()
}

func (Repository) StatusOf(ctx context.Context, db database.DBTX, customerID, id string) (StatusView, error) {
	var v StatusView
	err := db.QueryRow(ctx,
		`SELECT status::text, payment_status::text, updated_at FROM orders WHERE id = $1 AND customer_id = $2`,
		id, customerID).Scan(&v.Status, &v.PaymentStatus, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StatusView{}, ErrNotFound
	}
	return v, err
}

// Cancel moves a PENDING order to CANCELLED. It reports false when the
// order exists but is no longer pending.
func (Repository) Cancel(ctx context.Context, db database.DBTX, customerID, id string, now time.Time) (bool, error) {
	tag, err := db.Exec(ctx,
		`UPDATE orders SET status = 'CANCELLED', cancelled_at = $3, updated_at = $3
		 WHERE id = $1 AND customer_id = $2 AND status = 'PENDING'`, id, customerID, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
