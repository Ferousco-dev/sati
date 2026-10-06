package orders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/internal/store"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

const (
	maxLines       = 50
	maxQuantity    = 50
	maxNotes       = 500
	maxIdempotency = 128
)

type Service struct {
	pool      *pgxpool.Pool
	repo      *Repository
	addresses *addresses.Repository
	store     *store.Repository
	now       func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:      pool,
		repo:      NewRepository(),
		addresses: addresses.NewRepository(),
		store:     store.NewRepository(),
		now:       time.Now,
	}
}

type Page struct {
	Orders []Order `json:"orders"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

func validation(msg string) *apierror.Error {
	return apierror.New(http.StatusBadRequest, "VALIDATION_ERROR", msg)
}

// Create builds an order from product IDs and quantities only. Prices,
// totals and the delivery fee always come from the database.
// The bool reports whether a new order was created, false when an
// Idempotency-Key matched an earlier request.
func (s *Service) Create(ctx context.Context, customerID string, in CreateInput, idempotencyKey string) (Order, bool, error) {
	lines, err := validateCreate(&in)
	if err != nil {
		return Order{}, false, err
	}
	if len(idempotencyKey) > maxIdempotency {
		return Order{}, false, validation("Idempotency-Key must be at most 128 characters")
	}

	var key *string
	if idempotencyKey != "" {
		key = &idempotencyKey
		if existing, err := s.repo.ByIdempotencyKey(ctx, s.pool, customerID, idempotencyKey); err == nil {
			order, err := s.withItems(ctx, existing)
			return order, false, err
		} else if !errors.Is(err, ErrNotFound) {
			return Order{}, false, err
		}
	}

	var orderID string
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		orderID, err = s.place(ctx, tx, customerID, in, lines, key)
		return err
	})
	if errors.Is(err, errDuplicateKey) {
		existing, err := s.repo.ByIdempotencyKey(ctx, s.pool, customerID, idempotencyKey)
		if err != nil {
			return Order{}, false, err
		}
		order, err := s.withItems(ctx, existing)
		return order, false, err
	}
	if err != nil {
		return Order{}, false, err
	}

	order, err := s.Get(ctx, customerID, orderID)
	return order, true, err
}

func (s *Service) place(ctx context.Context, tx pgx.Tx, customerID string, in CreateInput, lines []LineInput, key *string) (string, error) {
	ids := make([]string, len(lines))
	for i, l := range lines {
		ids[i] = l.ProductID
	}
	settings, err := s.store.Get(ctx, tx)
	if err != nil {
		return "", err
	}
	if !methodEnabled(settings, in.PaymentMethod) {
		return "", ErrMethodDisabled
	}
	if in.PaymentMethod == BankTransfer && !settings.BankTransferReady() {
		return "", ErrBankNotReady
	}

	found, err := s.repo.LockProducts(ctx, tx, ids)
	if err != nil {
		return "", err
	}

	var subtotal int64
	var unavailable []string
	items := make([]newItem, 0, len(lines))
	for _, l := range lines {
		p, ok := found[l.ProductID]
		if !ok {
			return "", ErrProductNotFound
		}
		if !p.Orderable {
			unavailable = append(unavailable, p.Name)
			continue
		}
		lineTotal := p.PriceKobo * int64(l.Quantity)
		subtotal += lineTotal
		items = append(items, newItem{
			ProductID: p.ID, Name: p.Name, UnitPriceKobo: p.PriceKobo,
			Quantity: int32(l.Quantity), LineTotalKobo: lineTotal,
		})
	}
	if len(unavailable) > 0 {
		sort.Strings(unavailable)
		return "", apierror.New(http.StatusConflict, "PRODUCT_UNAVAILABLE",
			"No longer available: "+strings.Join(unavailable, ", "))
	}

	order := newOrder{
		CustomerID:     customerID,
		Fulfillment:    in.Fulfillment.Type,
		SubtotalKobo:   subtotal,
		PaymentMethod:  in.PaymentMethod,
		Notes:          in.Notes,
		IdempotencyKey: key,
	}

	if in.Fulfillment.Type == Delivery {
		address, err := s.addresses.Owned(ctx, tx, customerID, in.Fulfillment.AddressID)
		if err != nil {
			return "", err
		}
		snapshot, err := json.Marshal(address)
		if err != nil {
			return "", err
		}
		order.AddressID = &address.ID
		order.AddressSnapshot = snapshot
		order.DeliveryFeeKobo = settings.DeliveryFeeKobo
	}
	order.TotalKobo = order.SubtotalKobo + order.DeliveryFeeKobo

	return s.repo.Insert(ctx, tx, order, items)
}

func (s *Service) Get(ctx context.Context, customerID, id string) (Order, error) {
	if !httpx.ValidUUID(id) {
		return Order{}, ErrNotFound
	}
	o, err := s.repo.ByID(ctx, s.pool, customerID, id)
	if err != nil {
		return Order{}, err
	}
	return s.withItems(ctx, o)
}

func (s *Service) List(ctx context.Context, customerID string, limit, offset int) (Page, error) {
	list, total, err := s.repo.ListByCustomer(ctx, s.pool, customerID, limit, offset)
	if err != nil {
		return Page{}, err
	}

	ids := make([]string, len(list))
	for i, o := range list {
		ids[i] = o.ID
	}
	items, err := s.repo.ItemsFor(ctx, s.pool, ids)
	if err != nil {
		return Page{}, err
	}
	for i := range list {
		if found := items[list[i].ID]; found != nil {
			list[i].Items = found
		}
	}
	if err := s.attachInstructions(ctx, list); err != nil {
		return Page{}, err
	}
	return Page{Orders: list, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Status(ctx context.Context, customerID, id string) (StatusView, error) {
	if !httpx.ValidUUID(id) {
		return StatusView{}, ErrNotFound
	}
	return s.repo.StatusOf(ctx, s.pool, customerID, id)
}

func (s *Service) Cancel(ctx context.Context, customerID, id string) (Order, error) {
	if !httpx.ValidUUID(id) {
		return Order{}, ErrNotFound
	}
	cancelled, err := s.repo.Cancel(ctx, s.pool, customerID, id, s.now())
	if err != nil {
		return Order{}, err
	}
	if !cancelled {
		if _, err := s.repo.StatusOf(ctx, s.pool, customerID, id); err != nil {
			return Order{}, err
		}
		return Order{}, ErrNotCancellable
	}
	return s.Get(ctx, customerID, id)
}

func (s *Service) withItems(ctx context.Context, o Order) (Order, error) {
	items, err := s.repo.ItemsFor(ctx, s.pool, []string{o.ID})
	if err != nil {
		return Order{}, err
	}
	if found := items[o.ID]; found != nil {
		o.Items = found
	}
	list := []Order{o}
	if err := s.attachInstructions(ctx, list); err != nil {
		return Order{}, err
	}
	return list[0], nil
}

// attachInstructions adds the bank details to orders that are still waiting
// for a transfer. Settings are read once, and only when needed.
func (s *Service) attachInstructions(ctx context.Context, list []Order) error {
	var settings *store.Settings
	for i := range list {
		o := &list[i]
		if o.PaymentMethod != BankTransfer || o.Status != StatusPending || o.PaymentStatus != "UNPAID" {
			continue
		}
		if settings == nil {
			loaded, err := s.store.Get(ctx, s.pool)
			if err != nil {
				return err
			}
			settings = &loaded
		}
		if !settings.BankTransferReady() {
			continue
		}
		o.PaymentInstructions = &PaymentInstructions{
			Method:        BankTransfer,
			BankName:      settings.BankName,
			AccountNumber: settings.BankAccountNumber,
			AccountName:   settings.BankAccountName,
			Reference:     o.Number,
			AmountKobo:    o.TotalKobo,
			Currency:      Currency,
			WhatsAppURL:   whatsAppURL(settings.WhatsAppNumber, settings.WhatsAppMessage, o.Number, o.TotalKobo),
		}
	}
	return nil
}

func methodEnabled(s store.Settings, m PaymentMethod) bool {
	switch m {
	case PayOnline:
		return s.OnlineEnabled
	case BankTransfer:
		return s.BankTransferEnabled
	case PayOnPickup:
		return s.PayOnPickupEnabled
	case PayOnDelivery:
		return s.PayOnDeliveryEnabled
	}
	return false
}

func (s *Service) CheckoutOptions(ctx context.Context) (CheckoutOptions, error) {
	settings, err := s.store.Get(ctx, s.pool)
	if err != nil {
		return CheckoutOptions{}, err
	}
	out := CheckoutOptions{
		DeliveryFeeKobo: settings.DeliveryFeeKobo,
		PaymentMethods:  map[Fulfillment][]PaymentMethod{Pickup: {}, Delivery: {}},
		Currency:        Currency,
	}
	for _, f := range []Fulfillment{Pickup, Delivery} {
		for _, m := range MethodsFor(f) {
			if !methodEnabled(settings, m) || (m == BankTransfer && !settings.BankTransferReady()) {
				continue
			}
			out.PaymentMethods[f] = append(out.PaymentMethods[f], m)
		}
	}
	return out, nil
}

// validateCreate checks the request shape and merges repeated products into one line.
func validateCreate(in *CreateInput) ([]LineInput, error) {
	if len(in.Items) == 0 || len(in.Items) > maxLines {
		return nil, validation(fmt.Sprintf("An order needs between 1 and %d items", maxLines))
	}

	merged := map[string]int{}
	var order []string
	for _, l := range in.Items {
		if !httpx.ValidUUID(l.ProductID) {
			return nil, ErrProductNotFound
		}
		if l.Quantity < 1 || l.Quantity > maxQuantity {
			return nil, validation(fmt.Sprintf("Quantity must be between 1 and %d", maxQuantity))
		}
		if _, seen := merged[l.ProductID]; !seen {
			order = append(order, l.ProductID)
		}
		merged[l.ProductID] += l.Quantity
	}
	lines := make([]LineInput, len(order))
	for i, id := range order {
		if merged[id] > maxQuantity {
			return nil, validation(fmt.Sprintf("Quantity must be between 1 and %d", maxQuantity))
		}
		lines[i] = LineInput{ProductID: id, Quantity: merged[id]}
	}

	switch in.Fulfillment.Type {
	case Pickup:
		if in.Fulfillment.AddressID != "" {
			return nil, validation("address_id is only used for delivery orders")
		}
	case Delivery:
		if in.Fulfillment.AddressID == "" {
			return nil, ErrAddressRequired
		}
	default:
		return nil, validation("fulfillment.type must be PICKUP or DELIVERY")
	}

	switch in.PaymentMethod {
	case PayOnline, BankTransfer, PayOnPickup, PayOnDelivery:
		if !methodAllowed(in.Fulfillment.Type, in.PaymentMethod) {
			return nil, validation(string(in.PaymentMethod) + " is not available for " + string(in.Fulfillment.Type) + " orders")
		}
	default:
		return nil, validation("payment_method must be ONLINE, BANK_TRANSFER, PAY_ON_PICKUP or PAY_ON_DELIVERY")
	}

	in.Notes = strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(in.Notes) > maxNotes {
		return nil, validation(fmt.Sprintf("notes must be at most %d characters", maxNotes))
	}
	return lines, nil
}
