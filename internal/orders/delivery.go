package orders

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Ferousco-dev/sati/backend/internal/delivery"
	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

var (
	ErrRiderRequired   = apierror.New(http.StatusConflict, "RIDER_REQUIRED", "Assign a rider before sending this order out for delivery")
	ErrInvalidRider    = apierror.New(http.StatusBadRequest, "INVALID_RIDER", "Choose a user who has the RIDER role")
	ErrNotDelivery     = apierror.New(http.StatusConflict, "NOT_A_DELIVERY_ORDER", "Only delivery orders can have a rider")
	ErrNotAssignable   = apierror.New(http.StatusConflict, "ORDER_NOT_ASSIGNABLE", "A rider can be assigned after the order is confirmed and before it goes out")
	ErrDeliveryBusy    = apierror.New(http.StatusConflict, "DELIVERY_IN_PROGRESS", "This delivery can no longer be reassigned")
	ErrOrderNotReady   = apierror.New(http.StatusConflict, "ORDER_NOT_READY", "The order is not ready for pickup yet")
	ErrBadDeliveryMove = apierror.New(http.StatusConflict, "INVALID_DELIVERY_TRANSITION", "That delivery update is not allowed right now")
	ErrFailureReason   = apierror.New(http.StatusBadRequest, "REASON_REQUIRED", "Say why the delivery failed")
)

type RiderUpdate struct {
	Delivery      delivery.Delivery `json:"delivery"`
	OrderStatus   Status            `json:"order_status"`
	PaymentStatus string            `json:"payment_status"`
}

func (Repository) zoneOf(ctx context.Context, db database.DBTX, orderID string) (string, error) {
	var zone *string
	err := db.QueryRow(ctx, `SELECT delivery_address->>'zone_id' FROM orders WHERE id = $1`, orderID).Scan(&zone)
	if zone == nil {
		return "", err
	}
	return *zone, err
}

// syncDelivery keeps the delivery record in step when staff move an order by hand.
func (s *Service) syncDelivery(ctx context.Context, tx pgx.Tx, id string, cur lockedOrder, to Status, reason string, now time.Time) error {
	if cur.Fulfillment != Delivery {
		return nil
	}
	d, err := s.delivery.ByOrder(ctx, tx, id, true)
	if err != nil {
		return err
	}

	switch to {
	case StatusOutForDelivery:
		if d == nil || d.Status != delivery.StatusAssigned {
			return ErrRiderRequired
		}
		return s.delivery.SetStatus(ctx, tx, d.ID, delivery.StatusPickedUp, "", now)
	case StatusDelivered:
		if d == nil {
			return ErrRiderRequired
		}
		return s.delivery.SetStatus(ctx, tx, d.ID, delivery.StatusDelivered, "", now)
	case StatusFailed:
		if d != nil && (d.Status == delivery.StatusAssigned || d.Status == delivery.StatusPickedUp) {
			return s.delivery.SetStatus(ctx, tx, d.ID, delivery.StatusFailed, reason, now)
		}
	case StatusCancelled:
		return s.delivery.CancelForOrder(ctx, tx, id, now)
	}
	return nil
}

// AssignRider sets or changes the rider for a delivery order. Reassigning after
// a failed attempt sends the order back to READY for another try.
func (s *Service) AssignRider(ctx context.Context, actorID, id, riderID, notes string) (StaffOrder, error) {
	if !httpx.ValidUUID(id) {
		return StaffOrder{}, ErrNotFound
	}
	notes = strings.TrimSpace(notes)
	if utf8.RuneCountInString(notes) > maxReason {
		return StaffOrder{}, validation("notes must be at most 200 characters")
	}
	if !httpx.ValidUUID(riderID) {
		return StaffOrder{}, ErrInvalidRider
	}
	rider, err := s.users.ByID(ctx, s.pool, riderID)
	if err == users.ErrNotFound || (err == nil && rider.Role != users.RoleRider) {
		return StaffOrder{}, ErrInvalidRider
	}
	if err != nil {
		return StaffOrder{}, err
	}

	now := s.now()
	var result StaffOrder
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.repo.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur.Fulfillment != Delivery {
			return ErrNotDelivery
		}
		d, err := s.delivery.ByOrder(ctx, tx, id, true)
		if err != nil {
			return err
		}

		assignable := cur.Status == StatusConfirmed || cur.Status == StatusPreparing || cur.Status == StatusReady
		switch {
		case d == nil || d.Status == delivery.StatusAssigned:
			if !assignable {
				return ErrNotAssignable
			}
		case d.Status == delivery.StatusFailed:
			if cur.Status != StatusOutForDelivery {
				return ErrNotAssignable
			}
			if err := s.repo.setStatus(ctx, tx, id, StatusReady, "", now); err != nil {
				return err
			}
		default:
			return ErrDeliveryBusy
		}

		zone, err := s.repo.zoneOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.delivery.Assign(ctx, tx, id, riderID, zone, actorID, notes, now); err != nil {
			return err
		}
		result, err = s.staffGet(ctx, tx, id)
		return err
	})
	return result, err
}

// RiderUpdateStatus lets a rider move their own delivery along. Picking up and
// delivering also move the order, and cash on delivery is recorded as collected by the rider.
func (s *Service) RiderUpdateStatus(ctx context.Context, riderID, deliveryID, status, reason string) (RiderUpdate, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != delivery.StatusPickedUp && status != delivery.StatusDelivered && status != delivery.StatusFailed {
		return RiderUpdate{}, validation("status must be PICKED_UP, DELIVERED or FAILED")
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > maxReason {
		return RiderUpdate{}, validation("reason must be at most 200 characters")
	}
	if status == delivery.StatusFailed && reason == "" {
		return RiderUpdate{}, ErrFailureReason
	}
	if status != delivery.StatusFailed {
		reason = ""
	}

	orderID, err := s.delivery.OrderIDFor(ctx, s.pool, riderID, deliveryID)
	if err != nil {
		return RiderUpdate{}, err
	}

	now := s.now()
	var out RiderUpdate
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := s.repo.lock(ctx, tx, orderID)
		if err != nil {
			return err
		}
		d, err := s.delivery.ByOrder(ctx, tx, orderID, true)
		if err != nil {
			return err
		}
		if d == nil || d.ID != deliveryID || d.RiderID != riderID {
			return delivery.ErrNotFound
		}

		orderStatus := cur.Status
		switch status {
		case delivery.StatusPickedUp:
			if d.Status != delivery.StatusAssigned {
				return ErrBadDeliveryMove
			}
			if cur.Status != StatusReady {
				return ErrOrderNotReady
			}
			orderStatus = StatusOutForDelivery
		case delivery.StatusDelivered:
			if d.Status != delivery.StatusPickedUp || cur.Status != StatusOutForDelivery {
				return ErrBadDeliveryMove
			}
			orderStatus = StatusDelivered
		case delivery.StatusFailed:
			if d.Status != delivery.StatusPickedUp {
				return ErrBadDeliveryMove
			}
		}

		if err := s.delivery.SetStatus(ctx, tx, d.ID, status, reason, now); err != nil {
			return err
		}
		if orderStatus != cur.Status {
			if err := s.repo.setStatus(ctx, tx, orderID, orderStatus, "", now); err != nil {
				return err
			}
		}
		paymentStatus := cur.PaymentStatus
		if orderStatus == StatusDelivered {
			if err := s.collectCash(ctx, tx, orderID, cur, StatusDelivered, riderID, now); err != nil {
				return err
			}
			if cur.Method == PayOnDelivery {
				paymentStatus = "PAID"
			}
		}

		updated, err := s.delivery.ByOrder(ctx, tx, orderID, false)
		if err != nil {
			return err
		}
		out = RiderUpdate{Delivery: *updated, OrderStatus: orderStatus, PaymentStatus: paymentStatus}
		return nil
	})
	return out, err
}
