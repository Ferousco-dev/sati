package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ferousco-dev/sati/backend/internal/email"
	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

// OrderEvent describes an order for notifications. Orders fill it in without
// this package knowing anything about how orders are stored.
type OrderEvent struct {
	email.Order
	CustomerID string
	Reason     string
	StoreEmail string
}

type Service struct {
	repo        *Repository
	frontendURL string
	log         *slog.Logger
}

func NewService(frontendURL string, log *slog.Logger) *Service {
	return &Service{repo: NewRepository(), frontendURL: strings.TrimRight(frontendURL, "/"), log: log}
}

func (s *Service) link(path string) string { return s.frontendURL + path }

func (s *Service) tokenLink(path, token string) string {
	return s.link(path) + "?token=" + url.QueryEscape(token)
}

// safely runs a notification step inside a savepoint. A failure is logged and
// undone without touching the order transaction around it.
func (s *Service) safely(ctx context.Context, tx pgx.Tx, what string, fn func(db database.DBTX) error) {
	if err := pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error { return fn(sp) }); err != nil {
		s.log.Error("notification skipped", "step", what, "error", err)
	}
}

func (s *Service) orderPayload(e OrderEvent, withItems bool) email.Payload {
	order := e.Order
	if !withItems {
		order.Items, order.Bank = nil, nil
	}
	return email.Payload{Name: e.CustomerName, Order: &order, Reason: e.Reason, Link: s.link("/orders/" + e.ID)}
}

func (s *Service) OrderPlaced(ctx context.Context, tx pgx.Tx, e OrderEvent) {
	s.safely(ctx, tx, "order placed", func(db database.DBTX) error {
		body := fmt.Sprintf("We received your order. Total %s.", email.Naira(e.TotalKobo))
		if err := s.repo.AddNotification(ctx, db, e.CustomerID, "order_placed", "Order "+e.Number+" placed", body, e.ID); err != nil {
			return err
		}
		return s.repo.Enqueue(ctx, db, email.KindOrderPlaced, e.CustomerEmail, e.CustomerName, s.orderPayload(e, true), "order-placed:"+e.ID)
	})

	if e.StoreEmail == "" {
		return
	}
	s.safely(ctx, tx, "store alert", func(db database.DBTX) error {
		payload := email.Payload{Order: &e.Order, Link: s.link("/admin/orders/" + e.ID)}
		return s.repo.Enqueue(ctx, db, email.KindStoreNewOrder, e.StoreEmail, "", payload, "store-order:"+e.ID)
	})
}

func (s *Service) PaymentReceived(ctx context.Context, tx pgx.Tx, e OrderEvent, confirmed bool) {
	s.safely(ctx, tx, "payment received", func(db database.DBTX) error {
		title := "Payment received for " + e.Number
		if err := s.repo.AddNotification(ctx, db, e.CustomerID, "payment_received", title, "Thank you, we received your payment.", e.ID); err != nil {
			return err
		}
		payload := s.orderPayload(e, false)
		payload.Confirmed = confirmed
		return s.repo.Enqueue(ctx, db, email.KindPaymentReceived, e.CustomerEmail, e.CustomerName, payload, "payment:"+e.ID)
	})
}

var statusCopy = map[string][2]string{
	"CONFIRMED":        {"Order confirmed", "The store accepted your order."},
	"PREPARING":        {"Order being prepared", "Your order is being prepared."},
	"READY":            {"Order ready", "Your order is ready."},
	"OUT_FOR_DELIVERY": {"Out for delivery", "Your order is on its way."},
	"DELIVERED":        {"Order delivered", "Your order was delivered."},
	"COMPLETED":        {"Order complete", "Your order is complete."},
	"CANCELLED":        {"Order cancelled", "Your order was cancelled."},
	"FAILED":           {"Order could not be completed", "We could not complete your order."},
}

// emailKind picks the email for a status change, or "" when it only appears in the app.
func emailKind(status, fulfillment string) string {
	switch status {
	case "CONFIRMED":
		return email.KindOrderConfirmed
	case "READY":
		if fulfillment == "PICKUP" {
			return email.KindOrderReadyPickup
		}
	case "OUT_FOR_DELIVERY":
		return email.KindOrderOutForDelivery
	case "DELIVERED", "COMPLETED":
		return email.KindOrderDelivered
	case "CANCELLED":
		return email.KindOrderCancelled
	case "FAILED":
		return email.KindOrderFailed
	}
	return ""
}

func (s *Service) OrderStatus(ctx context.Context, tx pgx.Tx, e OrderEvent, status string) {
	text, ok := statusCopy[status]
	if !ok {
		return
	}
	s.safely(ctx, tx, "order status "+status, func(db database.DBTX) error {
		body := text[1]
		if e.Reason != "" {
			body += " Reason: " + e.Reason
		}
		if err := s.repo.AddNotification(ctx, db, e.CustomerID, "order_"+strings.ToLower(status), text[0]+": "+e.Number, body, e.ID); err != nil {
			return err
		}
		kind := emailKind(status, e.Fulfillment)
		if kind == "" {
			return nil
		}
		return s.repo.Enqueue(ctx, db, kind, e.CustomerEmail, e.CustomerName, s.orderPayload(e, false), "")
	})
}

// Mailer queues the account emails that the auth module sends.
type Mailer struct {
	svc  *Service
	pool *pgxpool.Pool
}

func NewMailer(svc *Service, pool *pgxpool.Pool) *Mailer { return &Mailer{svc: svc, pool: pool} }

func (m *Mailer) queue(ctx context.Context, kind, name, to string, p email.Payload) error {
	p.Name = name
	return m.svc.repo.Enqueue(ctx, m.pool, kind, to, name, p, "")
}

func (m *Mailer) SendVerification(ctx context.Context, name, to, token string) error {
	return m.queue(ctx, email.KindVerifyEmail, name, to, email.Payload{Link: m.svc.tokenLink("/verify-email", token)})
}

func (m *Mailer) SendPasswordReset(ctx context.Context, name, to, token string) error {
	return m.queue(ctx, email.KindPasswordReset, name, to, email.Payload{Link: m.svc.tokenLink("/reset-password", token)})
}

func (m *Mailer) SendInvite(ctx context.Context, name, to string, role users.Role, token string) error {
	return m.queue(ctx, email.KindInvite, name, to, email.Payload{Role: string(role), Link: m.svc.tokenLink("/reset-password", token)})
}

func decodePayload(raw json.RawMessage) (email.Payload, error) {
	var p email.Payload
	err := json.Unmarshal(raw, &p)
	return p, err
}
