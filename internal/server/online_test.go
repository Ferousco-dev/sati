package server

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/payments"
)

func (e *staffEnv) onlineOrder(t *testing.T) orders.Order {
	t.Helper()
	return e.place(t, "PICKUP", "ONLINE")
}

func (e *staffEnv) startPayment(t *testing.T, cookie *http.Cookie, orderID string) payments.Initialization {
	t.Helper()
	rec := e.do(t, "POST", "/api/v1/payments/initialize", map[string]any{"order_id": orderID}, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize: %d %s", rec.Code, rec.Body)
	}
	return decode[struct{ Payment payments.Initialization }](t, rec).Payment
}

func (e *staffEnv) verifyPayment(t *testing.T, cookie *http.Cookie, reference string) payments.StatusView {
	t.Helper()
	rec := e.do(t, "POST", "/api/v1/payments/verify", map[string]any{"reference": reference}, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: %d %s", rec.Code, rec.Body)
	}
	return decode[payments.StatusView](t, rec)
}

func (e *staffEnv) orderState(t *testing.T, id string) (status, paymentStatus string) {
	t.Helper()
	if err := e.queryRow(`SELECT status::text, payment_status::text FROM orders WHERE id = $1`, id).Scan(&status, &paymentStatus); err != nil {
		t.Fatal(err)
	}
	return
}

func (e *staffEnv) paymentFlags(t *testing.T, reference string) (status string, needsRefund bool) {
	t.Helper()
	if err := e.queryRow(`SELECT status::text, needs_refund FROM payments WHERE reference = $1`, reference).Scan(&status, &needsRefund); err != nil {
		t.Fatal(err)
	}
	return
}

func TestInitializePayment(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	other := e.signIn(t, "other@example.com", "CUSTOMER")

	pay := e.startPayment(t, e.customer, o.ID)
	if !strings.HasPrefix(pay.Reference, "SATI-") || pay.AmountKobo != o.TotalKobo || pay.Currency != "NGN" ||
		pay.AuthorizationURL != "https://checkout.paystack.test/"+pay.Reference {
		t.Fatalf("initialization: %+v", pay)
	}

	call := e.paystack.initCalls[0]
	if call["amount"].(float64) != float64(o.TotalKobo) || call["email"] != "cust@example.com" ||
		call["callback_url"] != "http://localhost:3000/orders/"+o.ID {
		t.Fatalf("what Paystack received: %v", call)
	}
	if _, ps := e.orderState(t, o.ID); ps != "PENDING" {
		t.Fatalf("order payment status: %s", ps)
	}
	if st, _ := e.paymentFlags(t, pay.Reference); st != "PENDING" {
		t.Fatalf("payment status: %s", st)
	}

	if rec := e.do(t, "POST", "/api/v1/payments/initialize", map[string]any{"order_id": o.ID}, other); rec.Code != http.StatusNotFound {
		t.Fatalf("someone else's order: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/payments/initialize", map[string]any{"order_id": o.ID}, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/payments/initialize", map[string]any{"order_id": "nope"}, e.customer); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}
	cash := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	if rec := e.do(t, "POST", "/api/v1/payments/initialize", map[string]any{"order_id": cash.ID}, e.customer); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "NOT_AN_ONLINE_ORDER" {
		t.Fatalf("cash order: %d %s", rec.Code, rec.Body)
	}
}

func TestVerifyConfirmsOrder(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	pay := e.startPayment(t, e.customer, o.ID)

	if v := e.verifyPayment(t, e.customer, pay.Reference); v.Payment.Status != "PENDING" || v.OrderStatus != "PENDING" {
		t.Fatalf("not paid yet: %+v", v)
	}

	e.paystack.set(pay.Reference, "success", o.TotalKobo)
	v := e.verifyPayment(t, e.customer, pay.Reference)
	if v.Payment.Status != "PAID" || v.OrderStatus != "CONFIRMED" || v.OrderPaymentStatus != "PAID" || v.Payment.PaidAt == nil {
		t.Fatalf("after payment: %+v", v)
	}
	if e.jobCount(t, "payment_received") != 1 || !strings.Contains(e.jobPayload(t, "payment_received"), `"confirmed": true`) {
		t.Fatal("payment email missing")
	}

	calls := e.paystack.verifyCount()
	again := e.verifyPayment(t, e.customer, pay.Reference)
	if again.Payment.Status != "PAID" || e.paystack.verifyCount() != calls || e.jobCount(t, "payment_received") != 1 {
		t.Fatal("verifying a settled payment must change nothing and not call Paystack again")
	}

	detail := decode[struct{ Order orders.StaffOrder }](t, e.do(t, "GET", "/api/v1/admin/orders/"+o.ID, nil, e.staff)).Order
	if len(detail.Payments) != 1 || detail.Payments[0].Provider != "PAYSTACK" || detail.Payments[0].GatewayResponse != "Successful" {
		t.Fatalf("admin view: %+v", detail.Payments)
	}

	got := e.do(t, "GET", "/api/v1/payments/"+pay.Reference, nil, e.customer)
	if got.Code != http.StatusOK {
		t.Fatalf("get payment: %d", got.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/payments/"+pay.Reference, nil, e.signIn(t, "other@example.com", "CUSTOMER")); rec.Code != http.StatusNotFound {
		t.Fatalf("someone else's payment: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/payments/verify", map[string]any{"reference": pay.Reference}, e.signIn(t, "third@example.com", "CUSTOMER")); rec.Code != http.StatusNotFound {
		t.Fatalf("verifying someone else's payment: %d", rec.Code)
	}
}

func TestWrongAmountIsNeverAccepted(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	pay := e.startPayment(t, e.customer, o.ID)

	e.paystack.set(pay.Reference, "success", o.TotalKobo-1)
	v := e.verifyPayment(t, e.customer, pay.Reference)
	if v.Payment.Status != "FAILED" || v.OrderStatus != "PENDING" || v.OrderPaymentStatus != "UNPAID" {
		t.Fatalf("short payment: %+v", v)
	}
	if _, flagged := e.paymentFlags(t, pay.Reference); !flagged {
		t.Fatal("money received for the wrong amount must be flagged")
	}

	second := e.startPayment(t, e.customer, o.ID)
	e.paystack.setCurrency(second.Reference, "success", o.TotalKobo, "USD")
	if v := e.verifyPayment(t, e.customer, second.Reference); v.OrderStatus != "PENDING" {
		t.Fatalf("wrong currency confirmed the order: %+v", v)
	}
}

func TestFailedPaymentCanBeRetried(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	first := e.startPayment(t, e.customer, o.ID)

	e.paystack.set(first.Reference, "abandoned", o.TotalKobo)
	v := e.verifyPayment(t, e.customer, first.Reference)
	if v.Payment.Status != "FAILED" || v.OrderPaymentStatus != "UNPAID" || v.OrderStatus != "PENDING" {
		t.Fatalf("after abandoned payment: %+v", v)
	}

	second := e.startPayment(t, e.customer, o.ID)
	if second.Reference == first.Reference {
		t.Fatal("every attempt needs its own reference")
	}
	e.paystack.set(second.Reference, "success", o.TotalKobo)
	if v := e.verifyPayment(t, e.customer, second.Reference); v.OrderStatus != "CONFIRMED" {
		t.Fatalf("retry: %+v", v)
	}

	if rec := e.do(t, "POST", "/api/v1/payments/initialize", map[string]any{"order_id": o.ID}, e.customer); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "ALREADY_PAID" {
		t.Fatalf("paying a paid order: %d %s", rec.Code, rec.Body)
	}
}

func TestProviderOutageAllowsRetry(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)

	e.paystack.mu.Lock()
	e.paystack.failInit = true
	e.paystack.mu.Unlock()
	rec := e.do(t, "POST", "/api/v1/payments/initialize", map[string]any{"order_id": o.ID}, e.customer)
	if rec.Code != http.StatusBadGateway || errorCodeOf(t, rec) != "PAYMENT_PROVIDER_ERROR" {
		t.Fatalf("outage: %d %s", rec.Code, rec.Body)
	}
	if _, ps := e.orderState(t, o.ID); ps != "UNPAID" {
		t.Fatalf("a failed start must leave the order payable: %s", ps)
	}

	e.paystack.mu.Lock()
	e.paystack.failInit = false
	e.paystack.mu.Unlock()
	e.startPayment(t, e.customer, o.ID)

	e.paystack.mu.Lock()
	e.paystack.failVerify = true
	e.paystack.mu.Unlock()
	pay := e.startPayment(t, e.customer, o.ID)
	if rec := e.do(t, "POST", "/api/v1/payments/verify", map[string]any{"reference": pay.Reference}, e.customer); rec.Code != http.StatusBadGateway {
		t.Fatalf("verify during outage: %d", rec.Code)
	}
}

func TestAttemptLimit(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	for i := 0; i < payments.MaxAttemptsPerOrder; i++ {
		e.startPayment(t, e.customer, o.ID)
	}
	rec := e.do(t, "POST", "/api/v1/payments/initialize", map[string]any{"order_id": o.ID}, e.customer)
	if rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "PAYMENT_ATTEMPTS_EXCEEDED" {
		t.Fatalf("attempt limit: %d %s", rec.Code, rec.Body)
	}
}

func TestWebhookSettlesOrderOnce(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	pay := e.startPayment(t, e.customer, o.ID)
	e.paystack.set(pay.Reference, "success", o.TotalKobo)

	body := webhookBody("charge.success", 4099260516, pay.Reference)
	if rec := e.sendWebhook(body, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned: %d", rec.Code)
	}
	if rec := e.sendWebhook(body, signBody(body+"x")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: %d", rec.Code)
	}
	if st, _ := e.orderState(t, o.ID); st != "PENDING" {
		t.Fatal("an unsigned webhook changed an order")
	}

	if rec := e.sendWebhook(body, signBody(body)); rec.Code != http.StatusOK {
		t.Fatalf("webhook: %d %s", rec.Code, rec.Body)
	}
	if st, ps := e.orderState(t, o.ID); st != "CONFIRMED" || ps != "PAID" {
		t.Fatalf("after webhook: %s %s", st, ps)
	}

	for i := 0; i < 3; i++ {
		if rec := e.sendWebhook(body, signBody(body)); rec.Code != http.StatusOK {
			t.Fatalf("repeat %d: %d", i, rec.Code)
		}
	}
	var events, paid int
	e.queryRow(`SELECT count(*) FROM payment_events`).Scan(&events)
	e.queryRow(`SELECT count(*) FROM payments WHERE status = 'PAID'`).Scan(&paid)
	if events != 1 || paid != 1 || e.jobCount(t, "payment_received") != 1 {
		t.Fatalf("duplicates must change nothing: events=%d paid=%d emails=%d", events, paid, e.jobCount(t, "payment_received"))
	}
}

func TestWebhookDoesNotTrustItsOwnBody(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	pay := e.startPayment(t, e.customer, o.ID)

	body := webhookBody("charge.success", 1, pay.Reference)
	if rec := e.sendWebhook(body, signBody(body)); rec.Code != http.StatusOK {
		t.Fatalf("webhook: %d", rec.Code)
	}
	if st, _ := e.orderState(t, o.ID); st != "PENDING" {
		t.Fatal("the gateway says nothing was paid, so the order must stay pending")
	}
}

func TestWebhookIgnoredEventsAndUnknownReferences(t *testing.T) {
	e := newStaffEnv(t)
	for _, body := range []string{
		webhookBody("charge.success", 11, "NOT-OURS"),
		webhookBody("transfer.success", 12, "whatever"),
		`{"event":"charge.success","data":{}}`,
	} {
		if rec := e.sendWebhook(body, signBody(body)); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", body, rec.Code)
		}
	}
	if rec := e.sendWebhook("not json", signBody("not json")); rec.Code == http.StatusOK {
		t.Fatal("an unreadable body must not be acknowledged")
	}
}

func TestWebhookFailureIsRetried(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	pay := e.startPayment(t, e.customer, o.ID)
	e.paystack.set(pay.Reference, "success", o.TotalKobo)

	e.paystack.mu.Lock()
	e.paystack.failVerify = true
	e.paystack.mu.Unlock()
	body := webhookBody("charge.success", 5, pay.Reference)
	if rec := e.sendWebhook(body, signBody(body)); rec.Code != http.StatusInternalServerError {
		t.Fatalf("a failure must not be acknowledged: %d", rec.Code)
	}

	e.paystack.mu.Lock()
	e.paystack.failVerify = false
	e.paystack.mu.Unlock()
	if rec := e.sendWebhook(body, signBody(body)); rec.Code != http.StatusOK {
		t.Fatalf("retry: %d", rec.Code)
	}
	if st, _ := e.orderState(t, o.ID); st != "CONFIRMED" {
		t.Fatal("the retried webhook must settle the order")
	}
}

func TestWebhookAndVerifyRaceSettlesOnce(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	pay := e.startPayment(t, e.customer, o.ID)
	e.paystack.set(pay.Reference, "success", o.TotalKobo)
	body := webhookBody("charge.success", 9, pay.Reference)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); e.sendWebhook(body, signBody(body)) }()
		go func() {
			defer wg.Done()
			e.do(t, "POST", "/api/v1/payments/verify", map[string]any{"reference": pay.Reference}, e.customer)
		}()
	}
	wg.Wait()

	var paid, flagged int
	e.queryRow(`SELECT count(*) FILTER (WHERE status = 'PAID'), count(*) FILTER (WHERE needs_refund) FROM payments`).Scan(&paid, &flagged)
	if paid != 1 || flagged != 0 || e.jobCount(t, "payment_received") != 1 {
		t.Fatalf("paid=%d flagged=%d emails=%d", paid, flagged, e.jobCount(t, "payment_received"))
	}
}

func TestLatePaymentOnCancelledOrderIsFlagged(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	pay := e.startPayment(t, e.customer, o.ID)
	e.do(t, "POST", "/api/v1/orders/"+o.ID+"/cancel", nil, e.customer)

	e.paystack.set(pay.Reference, "success", o.TotalKobo)
	v := e.verifyPayment(t, e.customer, pay.Reference)
	if v.OrderStatus != "CANCELLED" || v.Payment.Status != "PAID" || !v.Payment.NeedsRefund || v.OrderPaymentStatus != "PAID" {
		t.Fatalf("late payment: %+v", v)
	}
	if e.jobCount(t, "payment_received") != 0 {
		t.Fatal("the customer must not be told the order is confirmed")
	}

	list := decode[struct {
		Payments []struct {
			ID          string
			NeedsRefund bool `json:"needs_refund"`
		}
		Total int
	}](t, e.do(t, "GET", "/api/v1/admin/payments?needs_refund=true", nil, e.staff))
	if list.Total != 1 || !list.Payments[0].NeedsRefund {
		t.Fatalf("refund queue: %+v", list)
	}

	id := list.Payments[0].ID
	if rec := e.do(t, "POST", "/api/v1/admin/payments/"+id+"/refunded", map[string]any{"note": "Refunded in Paystack"}, e.staff); rec.Code != http.StatusForbidden {
		t.Fatalf("staff marking refunded: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/admin/payments/"+id+"/refunded", map[string]any{"note": "Refunded in Paystack"}, e.admin); rec.Code != http.StatusNoContent {
		t.Fatalf("mark refunded: %d", rec.Code)
	}
	if st, flagged := e.paymentFlags(t, pay.Reference); st != "REFUNDED" || flagged {
		t.Fatalf("after refund: %s %v", st, flagged)
	}
	if _, ps := e.orderState(t, o.ID); ps != "REFUNDED" {
		t.Fatalf("order payment status: %s", ps)
	}
	if rec := e.do(t, "POST", "/api/v1/admin/payments/"+id+"/refunded", map[string]any{}, e.admin); rec.Code != http.StatusConflict {
		t.Fatalf("refunding twice: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/admin/payments/nope/refunded", map[string]any{}, e.admin); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown payment: %d", rec.Code)
	}
}

func TestSecondSuccessfulAttemptIsFlagged(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	first := e.startPayment(t, e.customer, o.ID)
	second := e.startPayment(t, e.customer, o.ID)

	e.paystack.set(first.Reference, "success", o.TotalKobo)
	e.paystack.set(second.Reference, "success", o.TotalKobo)
	e.verifyPayment(t, e.customer, first.Reference)
	e.verifyPayment(t, e.customer, second.Reference)

	if st, flagged := e.paymentFlags(t, first.Reference); st != "PAID" || flagged {
		t.Fatalf("first: %s %v", st, flagged)
	}
	if st, flagged := e.paymentFlags(t, second.Reference); st != "PAID" || !flagged {
		t.Fatalf("a second payment for the same order must be flagged: %s %v", st, flagged)
	}
}

func TestStaffCancellingPaidOrderFlagsRefund(t *testing.T) {
	e := newStaffEnv(t)
	o := e.onlineOrder(t)
	pay := e.startPayment(t, e.customer, o.ID)
	e.paystack.set(pay.Reference, "success", o.TotalKobo)
	e.verifyPayment(t, e.customer, pay.Reference)

	if rec := e.setStatus(t, o.ID, "CANCELLED", "Kitchen closed"); rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	if _, flagged := e.paymentFlags(t, pay.Reference); !flagged {
		t.Fatal("cancelling a paid order must put the payment on the refund list")
	}
}

func TestUnpaidOnlineOrdersExpire(t *testing.T) {
	e := newStaffEnv(t)
	stale := e.onlineOrder(t)
	startedPay := e.startPayment(t, e.customer, stale.ID)
	fresh := e.onlineOrder(t)
	paid := e.onlineOrder(t)
	paidPay := e.startPayment(t, e.customer, paid.ID)
	e.paystack.set(paidPay.Reference, "success", paid.TotalKobo)
	e.verifyPayment(t, e.customer, paidPay.Reference)
	cash := e.place(t, "PICKUP", "PAY_ON_PICKUP")

	for _, id := range []string{stale.ID, paid.ID, cash.ID} {
		e.exec(`UPDATE orders SET created_at = now() - interval '45 minutes' WHERE id = $1`, id)
	}

	n, err := e.orderSvc.ExpireUnpaid(t.Context(), time.Now().Add(-30*time.Minute))
	if err != nil || n != 1 {
		t.Fatalf("expired %d, %v", n, err)
	}

	if st, _ := e.orderState(t, stale.ID); st != "CANCELLED" {
		t.Fatalf("stale order: %s", st)
	}
	if st, _ := e.paymentFlags(t, startedPay.Reference); st != "FAILED" {
		t.Fatalf("its open attempt must be closed: %s", st)
	}
	for name, id := range map[string]string{"fresh": fresh.ID, "paid": paid.ID, "cash": cash.ID} {
		if st, _ := e.orderState(t, id); st == "CANCELLED" {
			t.Fatalf("%s order was expired", name)
		}
	}

	mine := decode[struct{ Order orders.Order }](t, e.do(t, "GET", "/api/v1/orders/"+stale.ID, nil, e.customer)).Order
	if mine.StatusReason != "Payment not completed in time" {
		t.Fatalf("customer must see why: %+v", mine)
	}
	if !strings.Contains(e.jobPayload(t, "order_cancelled"), "Payment not completed in time") {
		t.Fatal("the customer must be emailed")
	}

	e.paystack.set(startedPay.Reference, "success", stale.TotalKobo)
	e.sendWebhook(webhookBody("charge.success", 77, startedPay.Reference), signBody(webhookBody("charge.success", 77, startedPay.Reference)))
	if st, flagged := e.paymentFlags(t, startedPay.Reference); st != "PAID" || !flagged {
		t.Fatalf("a payment after expiry must be flagged for refund: %s %v", st, flagged)
	}
	if again, _ := e.orderSvc.ExpireUnpaid(t.Context(), time.Now().Add(-30*time.Minute)); again != 0 {
		t.Fatalf("second sweep expired %d", again)
	}
}

func TestOnlineHiddenUntilGatewayConfigured(t *testing.T) {
	e := newStaffEnv(t)
	opts := decode[orders.CheckoutOptions](t, e.do(t, "GET", "/api/v1/checkout/options", nil, nil))
	hasOnline := func(o orders.CheckoutOptions) bool {
		for _, m := range o.PaymentMethods[orders.Pickup] {
			if m == orders.PayOnline {
				return true
			}
		}
		return false
	}
	if !hasOnline(opts) {
		t.Fatal("online should be offered once a gateway is configured")
	}

	bare := orders.NewService(e.pool, nil, nil)
	got, err := bare.CheckoutOptions(t.Context())
	if err != nil || hasOnline(got) {
		t.Fatalf("online must be hidden without a gateway: %v %v", got.PaymentMethods, err)
	}
	if _, _, err := bare.Create(t.Context(), e.userID(t, "cust@example.com"), orders.CreateInput{
		Items:         []orders.LineInput{{ProductID: e.product, Quantity: 1}},
		Fulfillment:   orders.FulfillmentInput{Type: orders.Pickup},
		PaymentMethod: orders.PayOnline,
	}, ""); err == nil {
		t.Fatal("an online order was accepted without a gateway")
	}
}
