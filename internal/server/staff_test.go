package server

import (
	"net/http"
	"sync"
	"testing"

	"github.com/Ferousco-dev/sati/backend/internal/admin"
	"github.com/Ferousco-dev/sati/backend/internal/analytics"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/users"
)

type staffEnv struct {
	*orderEnv
	staff *http.Cookie
}

func newStaffEnv(t *testing.T) *staffEnv {
	t.Helper()
	e := newOrderEnv(t)
	return &staffEnv{orderEnv: e, staff: e.signIn(t, "staff@example.com", users.RoleStaff)}
}

func (e *staffEnv) place(t *testing.T, fulfillment, method string) orders.Order {
	t.Helper()
	f := map[string]any{"type": fulfillment}
	if fulfillment == "DELIVERY" {
		f["address_id"] = e.address(t, e.customer)
	}
	rec := e.order(t, map[string]any{
		"items":          []map[string]any{{"product_id": e.product, "quantity": 2}},
		"fulfillment":    f,
		"payment_method": method,
	}, e.customer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("place %s %s: %d %s", fulfillment, method, rec.Code, rec.Body)
	}
	return decode[struct{ Order orders.Order }](t, rec).Order
}

func (e *staffEnv) setStatus(t *testing.T, id, status, reason string) *httptestRecorder {
	t.Helper()
	body := map[string]any{"status": status}
	if reason != "" {
		body["reason"] = reason
	}
	return e.do(t, "PATCH", "/api/v1/admin/orders/"+id+"/status", body, e.staff)
}

func (e *staffEnv) mustSetStatus(t *testing.T, id, status string) orders.StaffOrder {
	t.Helper()
	rec := e.setStatus(t, id, status, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("-> %s: %d %s", status, rec.Code, rec.Body)
	}
	return decode[struct{ Order orders.StaffOrder }](t, rec).Order
}

func TestStaffOrderAccessAndSearch(t *testing.T) {
	e := newStaffEnv(t)
	rider := e.signIn(t, "rider@example.com", users.RoleRider)
	pickup := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	e.place(t, "PICKUP", "ONLINE")

	if rec := e.do(t, "GET", "/api/v1/admin/orders", nil, e.customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/orders", nil, rider); rec.Code != http.StatusForbidden {
		t.Fatalf("rider: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/orders", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}

	all := decode[orders.StaffPage](t, e.do(t, "GET", "/api/v1/admin/orders", nil, e.staff))
	if all.Total != 2 || all.Orders[0].Customer.Email != "cust@example.com" || len(all.Orders[0].Items) != 1 {
		t.Fatalf("list: %+v", all)
	}

	byNumber := decode[orders.StaffPage](t, e.do(t, "GET", "/api/v1/admin/orders?q="+pickup.Number, nil, e.staff))
	if byNumber.Total != 1 || byNumber.Orders[0].ID != pickup.ID {
		t.Fatalf("search by number: %+v", byNumber)
	}
	if got := decode[orders.StaffPage](t, e.do(t, "GET", "/api/v1/admin/orders?q=CUST@", nil, e.staff)); got.Total != 2 {
		t.Fatalf("search by email: %d", got.Total)
	}
	if got := decode[orders.StaffPage](t, e.do(t, "GET", "/api/v1/admin/orders?q=%25", nil, e.staff)); got.Total != 0 {
		t.Fatalf("wildcards must be escaped: %d", got.Total)
	}
	if got := decode[orders.StaffPage](t, e.do(t, "GET", "/api/v1/admin/orders?payment_method=online", nil, e.staff)); got.Total != 1 {
		t.Fatalf("method filter: %d", got.Total)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/orders?status=NOPE", nil, e.staff); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status filter: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/orders?from=yesterday", nil, e.staff); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad date: %d", rec.Code)
	}

	detail := decode[struct{ Order orders.StaffOrder }](t, e.do(t, "GET", "/api/v1/admin/orders/"+pickup.ID, nil, e.staff)).Order
	if detail.Customer.Name == "" || len(detail.Items) != 1 {
		t.Fatalf("detail: %+v", detail)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/orders/00000000-0000-0000-0000-000000000000", nil, e.staff); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown order: %d", rec.Code)
	}
}

func TestPickupLifecycleRecordsCashOnCompletion(t *testing.T) {
	e := newStaffEnv(t)
	o := e.place(t, "PICKUP", "PAY_ON_PICKUP")

	if rec := e.setStatus(t, o.ID, "PREPARING", ""); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "INVALID_STATUS_TRANSITION" {
		t.Fatalf("skipping confirm: %d %s", rec.Code, rec.Body)
	}
	e.mustSetStatus(t, o.ID, "CONFIRMED")
	e.mustSetStatus(t, o.ID, "PREPARING")
	e.mustSetStatus(t, o.ID, "READY")
	if rec := e.setStatus(t, o.ID, "OUT_FOR_DELIVERY", ""); rec.Code != http.StatusConflict {
		t.Fatalf("pickup cannot go out for delivery: %d", rec.Code)
	}

	done := e.mustSetStatus(t, o.ID, "COMPLETED")
	if done.PaymentStatus != "PAID" || len(done.Payments) != 1 {
		t.Fatalf("cash must be recorded on completion: %+v", done)
	}
	p := done.Payments[0]
	if p.Provider != "MANUAL" || p.AmountKobo != done.TotalKobo || p.RecordedByName == "" || p.Method != "PAY_ON_PICKUP" {
		t.Fatalf("payment record: %+v", p)
	}
	if rec := e.setStatus(t, o.ID, "CANCELLED", "too late"); rec.Code != http.StatusConflict {
		t.Fatalf("completed order is final: %d", rec.Code)
	}
}

func TestDeliveryLifecycleRecordsCashOnDelivery(t *testing.T) {
	e := newStaffEnv(t)
	o := e.place(t, "DELIVERY", "PAY_ON_DELIVERY")

	for _, st := range []string{"CONFIRMED", "PREPARING", "READY"} {
		e.mustSetStatus(t, o.ID, st)
	}
	if rec := e.setStatus(t, o.ID, "COMPLETED", ""); rec.Code != http.StatusConflict {
		t.Fatalf("delivery cannot be COMPLETED: %d", rec.Code)
	}
	if rec := e.setStatus(t, o.ID, "OUT_FOR_DELIVERY", ""); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "RIDER_REQUIRED" {
		t.Fatalf("no rider assigned: %d %s", rec.Code, rec.Body)
	}
	rider := e.signIn(t, "rider@example.com", users.RoleRider)
	_ = rider
	e.assign(t, o.ID, e.userID(t, "rider@example.com"))
	e.mustSetStatus(t, o.ID, "OUT_FOR_DELIVERY")
	done := e.mustSetStatus(t, o.ID, "DELIVERED")
	if done.PaymentStatus != "PAID" || len(done.Payments) != 1 {
		t.Fatalf("cash on delivery: %+v", done)
	}
}

func TestCancelAndFailNeedReason(t *testing.T) {
	e := newStaffEnv(t)
	o := e.place(t, "PICKUP", "PAY_ON_PICKUP")

	if rec := e.setStatus(t, o.ID, "CANCELLED", ""); rec.Code != http.StatusBadRequest || errorCodeOf(t, rec) != "REASON_REQUIRED" {
		t.Fatalf("no reason: %d %s", rec.Code, rec.Body)
	}
	long := string(make([]byte, 201))
	if rec := e.setStatus(t, o.ID, "CANCELLED", long); rec.Code != http.StatusBadRequest {
		t.Fatalf("long reason: %d", rec.Code)
	}
	cancelled := decode[struct{ Order orders.StaffOrder }](t, e.setStatus(t, o.ID, "CANCELLED", "Out of stock")).Order
	if cancelled.Status != orders.StatusCancelled || cancelled.StatusReason != "Out of stock" {
		t.Fatalf("%+v", cancelled)
	}

	mine := decode[struct{ Order orders.Order }](t, e.do(t, "GET", "/api/v1/orders/"+o.ID, nil, e.customer)).Order
	if mine.StatusReason != "Out of stock" {
		t.Fatalf("customer must see the reason: %+v", mine)
	}
	if rec := e.setStatus(t, o.ID, "CONFIRMED", ""); rec.Code != http.StatusConflict {
		t.Fatalf("cancelled is final: %d", rec.Code)
	}
	if rec := e.setStatus(t, o.ID, "TELEPORTED", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown status: %d", rec.Code)
	}
	if rec := e.do(t, "PATCH", "/api/v1/admin/orders/"+o.ID+"/status", map[string]any{"status": "CONFIRMED"}, e.customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer changing status: %d", rec.Code)
	}
}

func TestOnlineAndTransferNeedPaymentToConfirm(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{"bank_name": "B", "bank_account_number": "1234567890", "bank_account_name": "N"})

	online := e.place(t, "PICKUP", "ONLINE")
	if rec := e.setStatus(t, online.ID, "CONFIRMED", ""); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "PAYMENT_REQUIRED" {
		t.Fatalf("online without payment: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(t, "POST", "/api/v1/admin/orders/"+online.ID+"/payment", map[string]any{}, e.staff); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "PAYMENT_NOT_MANUAL" {
		t.Fatalf("manual payment on online order: %d", rec.Code)
	}

	transfer := e.place(t, "PICKUP", "BANK_TRANSFER")
	if rec := e.setStatus(t, transfer.ID, "CONFIRMED", ""); rec.Code != http.StatusConflict {
		t.Fatalf("transfer without payment: %d", rec.Code)
	}
}

func TestRecordBankTransferPayment(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{"bank_name": "B", "bank_account_number": "1234567890", "bank_account_name": "N"})
	o := e.place(t, "PICKUP", "BANK_TRANSFER")

	rec := e.do(t, "POST", "/api/v1/admin/orders/"+o.ID+"/payment", map[string]any{"note": "Proof seen on WhatsApp"}, e.staff)
	if rec.Code != http.StatusOK {
		t.Fatalf("record: %d %s", rec.Code, rec.Body)
	}
	paid := decode[struct{ Order orders.StaffOrder }](t, rec).Order
	if paid.PaymentStatus != "PAID" || paid.Status != orders.StatusConfirmed || len(paid.Payments) != 1 {
		t.Fatalf("after payment: %+v", paid)
	}
	if paid.Payments[0].Note != "Proof seen on WhatsApp" || paid.Payments[0].RecordedByName == "" {
		t.Fatalf("payment record: %+v", paid.Payments[0])
	}

	if rec := e.do(t, "POST", "/api/v1/admin/orders/"+o.ID+"/payment", map[string]any{}, e.staff); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "ALREADY_PAID" {
		t.Fatalf("second record: %d %s", rec.Code, rec.Body)
	}

	mine := decode[struct{ Order orders.Order }](t, e.do(t, "GET", "/api/v1/orders/"+o.ID, nil, e.customer)).Order
	if mine.PaymentInstructions != nil || mine.PaymentStatus != "PAID" {
		t.Fatalf("customer view after payment: %+v", mine)
	}
	if rec := e.do(t, "POST", "/api/v1/admin/orders/"+o.ID+"/payment", map[string]any{}, e.customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer recording payment: %d", rec.Code)
	}

	cancelled := e.place(t, "PICKUP", "BANK_TRANSFER")
	e.setStatus(t, cancelled.ID, "CANCELLED", "duplicate")
	if rec := e.do(t, "POST", "/api/v1/admin/orders/"+cancelled.ID+"/payment", map[string]any{}, e.staff); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "ORDER_CLOSED" {
		t.Fatalf("payment on cancelled order: %d", rec.Code)
	}
}

func TestConcurrentPaymentRecordedOnce(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{"bank_name": "B", "bank_account_number": "1234567890", "bank_account_name": "N"})
	o := e.place(t, "PICKUP", "BANK_TRANSFER")

	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = e.do(t, "POST", "/api/v1/admin/orders/"+o.ID+"/payment", map[string]any{}, e.staff).Code
		}(i)
	}
	wg.Wait()

	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		}
	}
	if ok != 1 || conflict != 3 {
		t.Fatalf("expected exactly one success, got %v", codes)
	}
	detail := decode[struct{ Order orders.StaffOrder }](t, e.do(t, "GET", "/api/v1/admin/orders/"+o.ID, nil, e.staff)).Order
	if len(detail.Payments) != 1 {
		t.Fatalf("payments recorded: %d", len(detail.Payments))
	}
}

func TestPaymentsList(t *testing.T) {
	e := newStaffEnv(t)
	o := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	for _, st := range []string{"CONFIRMED", "PREPARING", "READY", "COMPLETED"} {
		e.mustSetStatus(t, o.ID, st)
	}

	rec := e.do(t, "GET", "/api/v1/admin/payments", nil, e.staff)
	list := decode[struct {
		Payments []struct {
			OrderNumber   string `json:"order_number"`
			CustomerEmail string `json:"customer_email"`
			AmountKobo    int64  `json:"amount_kobo"`
			Provider      string
		}
		Total int
	}](t, rec)
	if rec.Code != http.StatusOK || list.Total != 1 || list.Payments[0].OrderNumber != o.Number ||
		list.Payments[0].CustomerEmail != "cust@example.com" || list.Payments[0].AmountKobo != 500000 {
		t.Fatalf("payments: %d %s", rec.Code, rec.Body)
	}
	if got := decode[struct{ Total int }](t, e.do(t, "GET", "/api/v1/admin/payments?method=ONLINE", nil, e.staff)); got.Total != 0 {
		t.Fatalf("method filter: %d", got.Total)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/payments?status=NOPE", nil, e.staff); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad filter: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/payments", nil, e.customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer: %d", rec.Code)
	}
}

func TestCustomersList(t *testing.T) {
	e := newStaffEnv(t)
	o := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	e.place(t, "PICKUP", "ONLINE")
	for _, st := range []string{"CONFIRMED", "PREPARING", "READY", "COMPLETED"} {
		e.mustSetStatus(t, o.ID, st)
	}

	if rec := e.do(t, "GET", "/api/v1/admin/customers", nil, e.staff); rec.Code != http.StatusForbidden {
		t.Fatalf("staff: %d", rec.Code)
	}
	page := decode[admin.CustomerPage](t, e.do(t, "GET", "/api/v1/admin/customers", nil, e.admin))
	if page.Total != 1 || page.Customers[0].Email != "cust@example.com" {
		t.Fatalf("customers must exclude staff and admins: %+v", page)
	}
	c := page.Customers[0]
	if c.OrdersCount != 2 || c.TotalSpentKobo != 500000 {
		t.Fatalf("aggregates: %+v", c)
	}
	if got := decode[admin.CustomerPage](t, e.do(t, "GET", "/api/v1/admin/customers?q=nobody", nil, e.admin)); got.Total != 0 {
		t.Fatalf("search: %d", got.Total)
	}
	if got := decode[admin.CustomerPage](t, e.do(t, "GET", "/api/v1/admin/customers?q=CUST", nil, e.admin)); got.Total != 1 {
		t.Fatalf("search match: %d", got.Total)
	}
}

func TestDashboard(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{"bank_name": "B", "bank_account_number": "1234567890", "bank_account_name": "N"})

	cash := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	transfer := e.place(t, "PICKUP", "BANK_TRANSFER")
	e.place(t, "PICKUP", "ONLINE")
	e.do(t, "POST", "/api/v1/admin/orders/"+transfer.ID+"/payment", map[string]any{}, e.staff)
	e.mustSetStatus(t, cash.ID, "CONFIRMED")
	e.mustSetStatus(t, cash.ID, "PREPARING")

	if rec := e.do(t, "GET", "/api/v1/admin/dashboard", nil, e.customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer: %d", rec.Code)
	}
	rec := e.do(t, "GET", "/api/v1/admin/dashboard", nil, e.staff)
	d := decode[analytics.Dashboard](t, rec)
	if rec.Code != http.StatusOK || d.Timezone != "Africa/Lagos" {
		t.Fatalf("dashboard: %d %s", rec.Code, rec.Body)
	}
	if d.Today.OrdersPlaced != 3 || d.Today.ByStatus["PENDING"] != 1 || d.Today.ByStatus["CONFIRMED"] != 1 || d.Today.ByStatus["PREPARING"] != 1 {
		t.Fatalf("today: %+v", d.Today)
	}
	if d.Today.PaidOrders != 1 || d.Today.RevenueKobo != 500000 || d.Today.RevenueByMethod["BANK_TRANSFER"] != 500000 {
		t.Fatalf("revenue: %+v", d.Today)
	}
	if d.Open.Pending != 1 || d.Open.Confirmed != 1 || d.Open.Preparing != 1 || d.Open.AwaitingTransfer != 0 {
		t.Fatalf("open orders: %+v", d.Open)
	}
	if !d.PeriodEnd.After(d.PeriodStart) || d.PeriodEnd.Sub(d.PeriodStart).Hours() != 24 {
		t.Fatalf("period: %v - %v", d.PeriodStart, d.PeriodEnd)
	}

	other := decode[analytics.Dashboard](t, e.do(t, "GET", "/api/v1/admin/dashboard?tz=Pacific/Kiritimati", nil, e.staff))
	if other.Timezone != "Pacific/Kiritimati" {
		t.Fatalf("custom timezone: %+v", other)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/dashboard?tz=Mars/Base", nil, e.staff); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad timezone: %d", rec.Code)
	}
}
