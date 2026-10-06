package orders

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

const expirySweepEvery = time.Minute

// ExpireUnpaid cancels online orders that were never paid within the allowed time.
// A payment that still arrives afterwards is kept and flagged for refund.
func (s *Service) ExpireUnpaid(ctx context.Context, olderThan time.Time) (int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text FROM orders
		 WHERE payment_method = 'ONLINE' AND status = 'PENDING' AND payment_status IN ('UNPAID', 'PENDING')
		   AND created_at < $1 ORDER BY created_at LIMIT 100`, olderThan)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	expired := 0
	for _, id := range ids {
		done, err := s.expireOne(ctx, id, olderThan)
		if err != nil {
			s.log("expire unpaid order failed", "order_id", id, "error", err)
			continue
		}
		if done {
			expired++
		}
	}
	return expired, nil
}

func (s *Service) expireOne(ctx context.Context, id string, olderThan time.Time) (bool, error) {
	var done bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var createdBefore bool
		var status, paymentStatus string
		err := tx.QueryRow(ctx,
			`SELECT status::text, payment_status::text, created_at < $2 FROM orders WHERE id = $1 FOR UPDATE SKIP LOCKED`,
			id, olderThan).Scan(&status, &paymentStatus, &createdBefore)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil
			}
			return err
		}
		if status != string(StatusPending) || !createdBefore || (paymentStatus != "UNPAID" && paymentStatus != "PENDING") {
			return nil
		}

		now := s.now()
		if err := s.repo.setStatus(ctx, tx, id, StatusCancelled, paymentExpiryReason, now); err != nil {
			return err
		}
		if err := s.payments.FailPendingForOrder(ctx, tx, id, "Order expired", now); err != nil {
			return err
		}
		if err := s.repo.setPaymentState(ctx, tx, id, "UNPAID", "", "", now); err != nil {
			return err
		}
		s.emitStatus(ctx, tx, id, StatusCancelled, paymentExpiryReason)
		done = true
		return nil
	})
	return done, err
}

// RunExpiry sweeps for unpaid online orders until the context ends.
func (s *Service) RunExpiry(ctx context.Context, after time.Duration) {
	if after <= 0 {
		return
	}
	ticker := time.NewTicker(expirySweepEvery)
	defer ticker.Stop()

	for {
		if n, err := s.ExpireUnpaid(ctx, s.now().Add(-after)); err != nil {
			s.log("expire unpaid orders", "error", err)
		} else if n > 0 {
			s.log("expired unpaid orders", "count", n)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
