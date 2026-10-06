package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Ferousco-dev/sati/backend/internal/notifications"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
)

func (e *catalogEnv) jobCount(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := e.queryRow(`SELECT count(*) FROM email_jobs WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *catalogEnv) jobPayload(t *testing.T, kind string) string {
	t.Helper()
	var p string
	if err := e.queryRow(`SELECT payload::text FROM email_jobs WHERE kind = $1 ORDER BY created_at DESC LIMIT 1`, kind).Scan(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *catalogEnv) jobRecipient(t *testing.T, kind string) string {
	t.Helper()
	var to string
	if err := e.queryRow(`SELECT to_email FROM email_jobs WHERE kind = $1 ORDER BY created_at DESC LIMIT 1`, kind).Scan(&to); err != nil {
		t.Fatal(err)
	}
	return to
}

func (e *staffEnv) notifications(t *testing.T, cookie *http.Cookie) (list struct {
	Notifications []notifications.Notification
	UnreadCount   int `json:"unread_count"`
	Total         int
}) {
	t.Helper()
	rec := e.do(t, "GET", "/api/v1/notifications", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("notifications: %d %s", rec.Code, rec.Body)
	}
	return decode[struct {
		Notifications []notifications.Notification
		UnreadCount   int `json:"unread_count"`
		Total         int
	}](t, rec)
}

func TestOrderPlacedQueuesEmailAndNotification(t *testing.T) {
	e := newStaffEnv(t)
	e.place(t, "PICKUP", "PAY_ON_PICKUP")

	if n := e.jobCount(t, "order_placed"); n != 1 {
		t.Fatalf("order_placed jobs: %d", n)
	}
	if to := e.jobRecipient(t, "order_placed"); to != "cust@example.com" {
		t.Fatalf("recipient: %s", to)
	}
	if n := e.jobCount(t, "store_new_order"); n != 0 {
		t.Fatalf("store alert without an address: %d", n)
	}
	list := e.notifications(t, e.customer)
	if list.Total != 1 || list.UnreadCount != 1 || !strings.HasPrefix(list.Notifications[0].Title, "Order SATI-") || list.Notifications[0].OrderID == "" {
		t.Fatalf("in-app: %+v", list)
	}
	if staffList := e.notifications(t, e.staff); staffList.Total != 0 {
		t.Fatal("another user's notification leaked")
	}
}

func TestStoreAlertAndBankDetailsInEmail(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{
		"notification_email": "orders@sati.example",
		"bank_name":          "Example Bank", "bank_account_number": "0123456789", "bank_account_name": "SATI Yoghurt",
		"whatsapp_number": "2348012345678",
	})
	e.place(t, "PICKUP", "BANK_TRANSFER")

	if n := e.jobCount(t, "store_new_order"); n != 1 || e.jobRecipient(t, "store_new_order") != "orders@sati.example" {
		t.Fatalf("store alert: %d", n)
	}
	payload := e.jobPayload(t, "order_placed")
	for _, want := range []string{"0123456789", "Example Bank", "wa.me/2348012345678", "Plain", "SATI-"} {
		if !strings.Contains(payload, want) {
			t.Errorf("payload missing %q: %s", want, payload)
		}
	}
	if !strings.Contains(e.jobPayload(t, "store_new_order"), "/admin/orders/") {
		t.Fatal("store alert should link to the admin order")
	}
}

func TestSettingsNotificationEmail(t *testing.T) {
	e := newStaffEnv(t)
	for _, bad := range []string{"not-an-email", "a b@example.com", "Name <a@example.com>"} {
		if rec := e.do(t, "PATCH", "/api/v1/admin/settings", map[string]any{"notification_email": bad}, e.admin); rec.Code != http.StatusBadRequest {
			t.Fatalf("%q accepted: %d", bad, rec.Code)
		}
	}
	e.settings(t, map[string]any{"notification_email": "orders@sati.example"})
	e.settings(t, map[string]any{"notification_email": ""})
	got := decode[struct {
		Settings struct {
			NotificationEmail string `json:"notification_email"`
		}
	}](t, e.do(t, "GET", "/api/v1/admin/settings", nil, e.staff))
	if got.Settings.NotificationEmail != "" {
		t.Fatal("clearing the address must work")
	}
}

func TestPickupStatusEmails(t *testing.T) {
	e := newStaffEnv(t)
	o := e.place(t, "PICKUP", "PAY_ON_PICKUP")

	steps := []struct {
		status string
		kind   string
		count  int
	}{
		{"CONFIRMED", "order_confirmed", 1},
		{"PREPARING", "", 0},
		{"READY", "order_ready_pickup", 1},
		{"COMPLETED", "order_delivered", 1},
	}
	for _, s := range steps {
		before := e.totalJobs(t)
		e.mustSetStatus(t, o.ID, s.status)
		added := e.totalJobs(t) - before
		if s.kind == "" && added != 0 {
			t.Fatalf("%s must not send an email, sent %d", s.status, added)
		}
		if s.kind != "" && e.jobCount(t, s.kind) != s.count {
			t.Fatalf("%s: %s jobs = %d", s.status, s.kind, e.jobCount(t, s.kind))
		}
	}

	list := e.notifications(t, e.customer)
	if list.Total != 5 {
		t.Fatalf("every status change appears in the app (placed + 4 changes), got %d", list.Total)
	}
}

func (e *catalogEnv) totalJobs(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.queryRow(`SELECT count(*) FROM email_jobs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeliveryStatusEmails(t *testing.T) {
	e := newStaffEnv(t)
	cookie, riderID := e.newRider(t, "rider@example.com")
	o := e.readyDelivery(t, riderID)

	if n := e.jobCount(t, "order_ready_pickup"); n != 0 {
		t.Fatal("delivery orders do not get a pickup email")
	}
	job := e.riderJobs(t, cookie, "")[0].ID
	e.riderMove(t, cookie, job, "PICKED_UP", "")
	if n := e.jobCount(t, "order_out_for_delivery"); n != 1 {
		t.Fatalf("out for delivery: %d", n)
	}
	e.riderMove(t, cookie, job, "DELIVERED", "")
	if n := e.jobCount(t, "order_delivered"); n != 1 {
		t.Fatalf("delivered: %d", n)
	}
	if !strings.Contains(e.jobPayload(t, "order_delivered"), o.Number) {
		t.Fatal("delivered email should name the order")
	}
}

func TestCancelEmailsCarryTheReason(t *testing.T) {
	e := newStaffEnv(t)
	o := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	e.setStatus(t, o.ID, "CANCELLED", "Out of stock")
	if p := e.jobPayload(t, "order_cancelled"); !strings.Contains(p, "Out of stock") {
		t.Fatalf("payload: %s", p)
	}
	n := e.notifications(t, e.customer).Notifications[0]
	if !strings.Contains(n.Body, "Out of stock") {
		t.Fatalf("in-app reason: %+v", n)
	}

	mine := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	e.do(t, "POST", "/api/v1/orders/"+mine.ID+"/cancel", nil, e.customer)
	if e.jobCount(t, "order_cancelled") != 2 || !strings.Contains(e.jobPayload(t, "order_cancelled"), "Cancelled at your request") {
		t.Fatal("customer cancellation should also notify")
	}

	failed := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	e.setStatus(t, failed.ID, "FAILED", "Payment expired")
	if e.jobCount(t, "order_failed") != 1 {
		t.Fatal("failed email missing")
	}
}

func TestPaymentReceivedEmail(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{"bank_name": "B", "bank_account_number": "1234567890", "bank_account_name": "N"})
	o := e.place(t, "PICKUP", "BANK_TRANSFER")

	e.do(t, "POST", "/api/v1/admin/orders/"+o.ID+"/payment", map[string]any{}, e.staff)
	if e.jobCount(t, "payment_received") != 1 {
		t.Fatal("payment email missing")
	}
	if !strings.Contains(e.jobPayload(t, "payment_received"), `"confirmed": true`) {
		t.Fatalf("payload: %s", e.jobPayload(t, "payment_received"))
	}
	if e.jobCount(t, "order_confirmed") != 0 {
		t.Fatal("payment and confirmation should be one email")
	}

	e.do(t, "POST", "/api/v1/admin/orders/"+o.ID+"/payment", map[string]any{}, e.staff)
	if e.jobCount(t, "payment_received") != 1 {
		t.Fatal("a rejected second payment must not email again")
	}
}

func TestRetriedOrderDoesNotEmailTwice(t *testing.T) {
	e := newStaffEnv(t)
	send := func() {
		e.doWithHeader(t, "POST", "/api/v1/orders", pickupBody(e.product, 1), e.customer, "Idempotency-Key", "k1")
	}
	send()
	send()
	if n := e.jobCount(t, "order_placed"); n != 1 {
		t.Fatalf("order_placed jobs: %d", n)
	}
}

func TestOrdersSurviveNotificationFailure(t *testing.T) {
	e := newStaffEnv(t)
	e.exec(`ALTER TABLE email_jobs RENAME TO email_jobs_offline`)
	defer e.exec(`ALTER TABLE email_jobs_offline RENAME TO email_jobs`)

	body := pickupBody(e.product, 1)
	body["payment_method"] = "PAY_ON_PICKUP"
	rec := e.order(t, body, e.customer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("a broken email queue must not block orders: %d %s", rec.Code, rec.Body)
	}
	o := decode[struct{ Order orders.Order }](t, rec).Order
	if rec := e.setStatus(t, o.ID, "CONFIRMED", ""); rec.Code != http.StatusOK {
		t.Fatalf("status change with broken queue: %d %s", rec.Code, rec.Body)
	}
}

func TestNotificationsAPI(t *testing.T) {
	e := newStaffEnv(t)
	o := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	e.mustSetStatus(t, o.ID, "CONFIRMED")
	other := e.signIn(t, "other@example.com", "CUSTOMER")

	if rec := e.do(t, "GET", "/api/v1/notifications", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}

	count := decode[struct {
		UnreadCount int `json:"unread_count"`
	}](t, e.do(t, "GET", "/api/v1/notifications/unread-count", nil, e.customer))
	if count.UnreadCount != 2 {
		t.Fatalf("unread: %d", count.UnreadCount)
	}
	if got := e.do(t, "GET", "/api/v1/notifications/unread-count", nil, e.customer).Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("notifications must never be cached: %q", got)
	}

	first := e.notifications(t, e.customer).Notifications[0]
	if rec := e.do(t, "POST", "/api/v1/notifications/"+first.ID+"/read", nil, other); rec.Code != http.StatusNotFound {
		t.Fatalf("reading someone else's notification: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/notifications/"+first.ID+"/read", nil, e.customer); rec.Code != http.StatusNoContent {
		t.Fatalf("mark read: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/notifications/not-a-uuid/read", nil, e.customer); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}

	unread := decode[struct {
		Notifications []notifications.Notification
		Total         int
	}](t, e.do(t, "GET", "/api/v1/notifications?unread=true", nil, e.customer))
	if unread.Total != 1 || unread.Notifications[0].ID == first.ID {
		t.Fatalf("unread filter: %+v", unread)
	}

	rec := e.do(t, "POST", "/api/v1/notifications/read-all", nil, e.customer)
	if marked := decode[struct {
		MarkedRead int `json:"marked_read"`
	}](t, rec); rec.Code != http.StatusOK || marked.MarkedRead != 1 {
		t.Fatalf("read all: %d %s", rec.Code, rec.Body)
	}
	if e.notifications(t, e.customer).UnreadCount != 0 {
		t.Fatal("everything should be read")
	}
}
