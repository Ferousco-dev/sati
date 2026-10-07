package orders

import (
	"context"
	"github.com/Ferousco-dev/sati/backend/pkg/metrics"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/internal/email"
	"github.com/Ferousco-dev/sati/backend/internal/notifications"
	"github.com/Ferousco-dev/sati/backend/internal/store"
)

// announcePlaced queues the confirmation email, the in-app notice and the store alert.
// Nothing here can fail the order: each step runs in its own savepoint.
func (s *Service) announcePlaced(ctx context.Context, tx pgx.Tx, id string, number int64, o newOrder, items []newItem, settings store.Settings) {
	customer, err := s.users.ByID(ctx, tx, o.CustomerID)
	if err != nil {
		return
	}

	event := notifications.OrderEvent{
		CustomerID: customer.ID,
		StoreEmail: settings.NotificationEmail,
		Order: email.Order{
			ID: id, Number: FormatNumber(number), CustomerName: customer.Name, CustomerEmail: customer.Email,
			Fulfillment: string(o.Fulfillment), PaymentMethod: string(o.PaymentMethod),
			SubtotalKobo: o.SubtotalKobo, DeliveryFeeKobo: o.DeliveryFeeKobo, TotalKobo: o.TotalKobo,
		},
	}
	for _, it := range items {
		event.Items = append(event.Items, email.Item{Name: it.Name, Quantity: int(it.Quantity), LineTotalKobo: it.LineTotalKobo})
	}
	if o.PaymentMethod == BankTransfer && settings.BankTransferReady() {
		event.Bank = &email.Bank{
			BankName: settings.BankName, AccountNumber: settings.BankAccountNumber, AccountName: settings.BankAccountName,
			WhatsAppURL: whatsAppURL(settings.WhatsAppNumber, settings.WhatsAppMessage, event.Number, o.TotalKobo),
		}
	}
	s.notify.OrderPlaced(ctx, tx, event)
}

func (s *Service) eventFor(ctx context.Context, tx pgx.Tx, id, reason string) (notifications.OrderEvent, bool) {
	var e notifications.OrderEvent
	var number int64
	err := pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error {
		return sp.QueryRow(ctx,
			`SELECT o.order_number, o.fulfillment_type::text, o.payment_method::text, o.subtotal_kobo,
			        o.delivery_fee_kobo, o.total_kobo, u.id::text, u.name, u.email::text
			 FROM orders o JOIN users u ON u.id = o.customer_id WHERE o.id = $1`, id).
			Scan(&number, &e.Fulfillment, &e.PaymentMethod, &e.SubtotalKobo, &e.DeliveryFeeKobo, &e.TotalKobo,
				&e.CustomerID, &e.CustomerName, &e.CustomerEmail)
	})
	if err != nil {
		return notifications.OrderEvent{}, false
	}
	e.ID, e.Number, e.Reason = id, FormatNumber(number), reason
	return e, true
}

func (s *Service) emitStatus(ctx context.Context, tx pgx.Tx, id string, status Status, reason string) {
	metrics.OrderStatusChanges.Inc(string(status))
	if e, ok := s.eventFor(ctx, tx, id, reason); ok {
		s.notify.OrderStatus(ctx, tx, e, string(status))
	}
}

func (s *Service) emitPayment(ctx context.Context, tx pgx.Tx, id string, confirmed bool) {
	if e, ok := s.eventFor(ctx, tx, id, ""); ok {
		s.notify.PaymentReceived(ctx, tx, e, confirmed)
	}
}
