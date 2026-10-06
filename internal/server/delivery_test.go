package server

import (
	"net/http"
	"testing"

	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/internal/delivery"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/users"
)

func (e *catalogEnv) userID(t *testing.T, email string) string {
	t.Helper()
	var id string
	if err := e.queryRow(`SELECT id::text FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *staffEnv) assign(t *testing.T, orderID, riderID string) orders.StaffOrder {
	t.Helper()
	rec := e.do(t, "POST", "/api/v1/admin/orders/"+orderID+"/rider", map[string]any{"rider_id": riderID}, e.staff)
	if rec.Code != http.StatusOK {
		t.Fatalf("assign: %d %s", rec.Code, rec.Body)
	}
	return decode[struct{ Order orders.StaffOrder }](t, rec).Order
}

func (e *staffEnv) zone(t *testing.T, name string, fee int64) string {
	t.Helper()
	rec := e.do(t, "POST", "/api/v1/admin/delivery/zones", map[string]any{"name": name, "fee_kobo": fee}, e.admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("zone %s: %d %s", name, rec.Code, rec.Body)
	}
	return decode[struct{ Zone delivery.Zone }](t, rec).Zone.ID
}

func (e *staffEnv) zoneAddress(t *testing.T, zoneID string) string {
	t.Helper()
	rec := e.do(t, "POST", "/api/v1/addresses", map[string]any{
		"recipient_name": "Ada", "phone": "08012345678", "address_line": "12 Main St",
		"area": "Yaba", "city": "Lagos", "zone_id": zoneID,
	}, e.customer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("zone address: %d %s", rec.Code, rec.Body)
	}
	return decode[struct{ Address addresses.Address }](t, rec).Address.ID
}

func (e *staffEnv) deliveryOrder(t *testing.T, addressID, method string) orders.Order {
	t.Helper()
	rec := e.order(t, map[string]any{
		"items":          []map[string]any{{"product_id": e.product, "quantity": 1}},
		"fulfillment":    map[string]any{"type": "DELIVERY", "address_id": addressID},
		"payment_method": method,
	}, e.customer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("delivery order: %d %s", rec.Code, rec.Body)
	}
	return decode[struct{ Order orders.Order }](t, rec).Order
}

// readyDelivery returns a cash-on-delivery order that is READY with the given rider assigned.
func (e *staffEnv) readyDelivery(t *testing.T, riderID string) orders.Order {
	t.Helper()
	o := e.place(t, "DELIVERY", "PAY_ON_DELIVERY")
	for _, st := range []string{"CONFIRMED", "PREPARING", "READY"} {
		e.mustSetStatus(t, o.ID, st)
	}
	e.assign(t, o.ID, riderID)
	return o
}

func (e *staffEnv) newRider(t *testing.T, email string) (*http.Cookie, string) {
	t.Helper()
	cookie := e.signIn(t, email, users.RoleRider)
	return cookie, e.userID(t, email)
}

func (e *staffEnv) riderJobs(t *testing.T, cookie *http.Cookie, query string) []delivery.RiderJob {
	t.Helper()
	rec := e.do(t, "GET", "/api/v1/rider/deliveries"+query, nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("rider list: %d %s", rec.Code, rec.Body)
	}
	return decode[struct{ Deliveries []delivery.RiderJob }](t, rec).Deliveries
}

func (e *staffEnv) riderMove(t *testing.T, cookie *http.Cookie, deliveryID, status, reason string) *httptestRecorder {
	t.Helper()
	body := map[string]any{"status": status}
	if reason != "" {
		body["reason"] = reason
	}
	return e.do(t, "PATCH", "/api/v1/rider/deliveries/"+deliveryID+"/status", body, cookie)
}

func TestDeliveryZonesAdmin(t *testing.T) {
	e := newStaffEnv(t)
	yaba := e.zone(t, "Yaba", 100000)
	e.zone(t, "Ikeja", 200000)

	if rec := e.do(t, "POST", "/api/v1/admin/delivery/zones", map[string]any{"name": "Yaba", "fee_kobo": 1}, e.admin); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d", rec.Code)
	}
	for _, body := range []map[string]any{{"name": "", "fee_kobo": 1}, {"name": "X"}, {"name": "X", "fee_kobo": -5}} {
		if rec := e.do(t, "POST", "/api/v1/admin/delivery/zones", body, e.admin); rec.Code != http.StatusBadRequest {
			t.Fatalf("%v: %d", body, rec.Code)
		}
	}
	if rec := e.do(t, "POST", "/api/v1/admin/delivery/zones", map[string]any{"name": "Z", "fee_kobo": 1}, e.staff); rec.Code != http.StatusForbidden {
		t.Fatalf("staff create: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/delivery/zones", nil, e.staff); rec.Code != http.StatusOK {
		t.Fatalf("staff read: %d", rec.Code)
	}

	if rec := e.do(t, "PATCH", "/api/v1/admin/delivery/zones/"+yaba, map[string]any{"active": false}, e.admin); rec.Code != http.StatusOK {
		t.Fatalf("deactivate: %d", rec.Code)
	}
	pub := e.do(t, "GET", "/api/v1/delivery/zones", nil, nil)
	list := decode[struct{ Zones []delivery.Zone }](t, pub)
	if len(list.Zones) != 1 || list.Zones[0].Name != "Ikeja" || pub.Header().Get("ETag") == "" {
		t.Fatalf("public zones: %+v", list)
	}
	if rec := e.do(t, "PATCH", "/api/v1/admin/delivery/zones/nope", map[string]any{"active": true}, e.admin); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}
}

func TestZoneSetsDeliveryFee(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{"delivery_fee_kobo": 50000})

	plain := e.address(t, e.customer)
	yaba := e.zone(t, "Yaba", 120000)
	zoned := e.zoneAddress(t, yaba)

	if rec := e.order(t, map[string]any{
		"items":          []map[string]any{{"product_id": e.product, "quantity": 1}},
		"fulfillment":    map[string]any{"type": "DELIVERY", "address_id": plain},
		"payment_method": "ONLINE",
	}, e.customer); rec.Code != http.StatusBadRequest || errorCodeOf(t, rec) != "ADDRESS_ZONE_REQUIRED" {
		t.Fatalf("address without zone while zones exist: %d %s", rec.Code, rec.Body)
	}

	o := e.deliveryOrder(t, zoned, "ONLINE")
	if o.DeliveryFeeKobo != 120000 || o.TotalKobo != e.price+120000 {
		t.Fatalf("zone fee: %+v", o)
	}
	if o.DeliveryAddress == nil || o.DeliveryAddress.ZoneName != "Yaba" {
		t.Fatalf("snapshot zone: %+v", o.DeliveryAddress)
	}

	e.do(t, "PATCH", "/api/v1/admin/delivery/zones/"+yaba, map[string]any{"fee_kobo": 300000}, e.admin)
	if again := decode[struct{ Order orders.Order }](t, e.do(t, "GET", "/api/v1/orders/"+o.ID, nil, e.customer)).Order; again.DeliveryFeeKobo != 120000 {
		t.Fatalf("existing order fee changed: %d", again.DeliveryFeeKobo)
	}
	if next := e.deliveryOrder(t, zoned, "ONLINE"); next.DeliveryFeeKobo != 300000 {
		t.Fatalf("new order must use the new fee: %d", next.DeliveryFeeKobo)
	}

	e.do(t, "PATCH", "/api/v1/admin/delivery/zones/"+yaba, map[string]any{"active": false}, e.admin)
	if rec := e.order(t, map[string]any{
		"items":          []map[string]any{{"product_id": e.product, "quantity": 1}},
		"fulfillment":    map[string]any{"type": "DELIVERY", "address_id": zoned},
		"payment_method": "ONLINE",
	}, e.customer); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "ZONE_UNAVAILABLE" {
		t.Fatalf("inactive zone: %d %s", rec.Code, rec.Body)
	}
}

func TestAddressZoneValidationAndOptions(t *testing.T) {
	e := newStaffEnv(t)
	zone := e.zone(t, "Yaba", 100000)
	body := map[string]any{"recipient_name": "A", "phone": "08012345678", "address_line": "x", "area": "y", "city": "z"}

	body["zone_id"] = "00000000-0000-0000-0000-000000000000"
	if rec := e.do(t, "POST", "/api/v1/addresses", body, e.customer); rec.Code != http.StatusBadRequest || errorCodeOf(t, rec) != "ZONE_NOT_FOUND" {
		t.Fatalf("unknown zone: %d", rec.Code)
	}

	id := e.zoneAddress(t, zone)
	if rec := e.do(t, "PATCH", "/api/v1/addresses/"+id, map[string]any{"zone_id": ""}, e.customer); rec.Code != http.StatusOK {
		t.Fatalf("clear zone: %d", rec.Code)
	} else if a := decode[struct{ Address addresses.Address }](t, rec).Address; a.ZoneID != "" {
		t.Fatalf("zone not cleared: %+v", a)
	}
	if rec := e.do(t, "PATCH", "/api/v1/addresses/"+id, map[string]any{"zone_id": zone}, e.customer); rec.Code != http.StatusOK {
		t.Fatalf("set zone: %d", rec.Code)
	}

	opts := decode[orders.CheckoutOptions](t, e.do(t, "GET", "/api/v1/checkout/options", nil, nil))
	if len(opts.DeliveryZones) != 1 || opts.DeliveryZones[0].Name != "Yaba" {
		t.Fatalf("checkout options zones: %+v", opts.DeliveryZones)
	}
}

func TestAssignRiderRules(t *testing.T) {
	e := newStaffEnv(t)
	_, riderID := e.newRider(t, "rider@example.com")
	_, otherRider := e.newRider(t, "rider2@example.com")

	pickup := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	e.mustSetStatus(t, pickup.ID, "CONFIRMED")
	post := func(order, rider string) *httptestRecorder {
		return e.do(t, "POST", "/api/v1/admin/orders/"+order+"/rider", map[string]any{"rider_id": rider}, e.staff)
	}
	if rec := post(pickup.ID, riderID); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "NOT_A_DELIVERY_ORDER" {
		t.Fatalf("pickup order: %d", rec.Code)
	}

	o := e.place(t, "DELIVERY", "PAY_ON_DELIVERY")
	if rec := post(o.ID, riderID); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "ORDER_NOT_ASSIGNABLE" {
		t.Fatalf("pending order: %d %s", rec.Code, rec.Body)
	}
	e.mustSetStatus(t, o.ID, "CONFIRMED")

	if rec := post(o.ID, e.userID(t, "cust@example.com")); rec.Code != http.StatusBadRequest || errorCodeOf(t, rec) != "INVALID_RIDER" {
		t.Fatalf("customer as rider: %d", rec.Code)
	}
	if rec := post(o.ID, "not-a-uuid"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad rider id: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/admin/orders/"+o.ID+"/rider", map[string]any{"rider_id": riderID}, e.customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer assigning: %d", rec.Code)
	}

	first := e.assign(t, o.ID, riderID)
	if first.Delivery == nil || first.Delivery.Status != "ASSIGNED" || first.Delivery.RiderName == "" {
		t.Fatalf("assignment: %+v", first.Delivery)
	}
	if again := e.assign(t, o.ID, otherRider); again.Delivery.RiderID != otherRider {
		t.Fatalf("reassign before pickup: %+v", again.Delivery)
	}

	riders := decode[struct{ Riders []struct{ ID, Name string } }](t, e.do(t, "GET", "/api/v1/admin/riders", nil, e.staff))
	if len(riders.Riders) != 2 {
		t.Fatalf("riders: %+v", riders)
	}
	list := decode[struct {
		Deliveries []struct {
			OrderNumber string `json:"order_number"`
			RiderName   string `json:"rider_name"`
		}
		Total int
	}](t, e.do(t, "GET", "/api/v1/admin/delivery?status=ASSIGNED", nil, e.staff))
	if list.Total != 1 || list.Deliveries[0].OrderNumber != o.Number {
		t.Fatalf("admin delivery list: %+v", list)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/delivery", nil, e.customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer delivery list: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/delivery?status=NOPE", nil, e.staff); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad filter: %d", rec.Code)
	}
}

func TestRiderDeliversAndCollectsCash(t *testing.T) {
	e := newStaffEnv(t)
	riderCookie, riderID := e.newRider(t, "rider@example.com")
	otherCookie, _ := e.newRider(t, "rider2@example.com")

	o := e.place(t, "DELIVERY", "PAY_ON_DELIVERY")
	e.mustSetStatus(t, o.ID, "CONFIRMED")
	e.mustSetStatus(t, o.ID, "PREPARING")
	e.assign(t, o.ID, riderID)

	jobs := e.riderJobs(t, riderCookie, "")
	if len(jobs) != 1 || jobs[0].OrderNumber != o.Number || jobs[0].CollectKobo != o.TotalKobo || jobs[0].CustomerName == "" || len(jobs[0].Address) < 5 {
		t.Fatalf("rider job: %+v", jobs)
	}
	if other := e.riderJobs(t, otherCookie, ""); len(other) != 0 {
		t.Fatalf("another rider's jobs leaked: %+v", other)
	}
	id := jobs[0].ID

	if rec := e.riderMove(t, riderCookie, id, "PICKED_UP", ""); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "ORDER_NOT_READY" {
		t.Fatalf("pickup before ready: %d %s", rec.Code, rec.Body)
	}
	e.mustSetStatus(t, o.ID, "READY")

	if rec := e.riderMove(t, otherCookie, id, "PICKED_UP", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("someone else's delivery: %d", rec.Code)
	}
	if rec := e.riderMove(t, e.customer, id, "PICKED_UP", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("customer as rider: %d", rec.Code)
	}
	if rec := e.riderMove(t, riderCookie, id, "DELIVERED", ""); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "INVALID_DELIVERY_TRANSITION" {
		t.Fatalf("deliver before pickup: %d", rec.Code)
	}
	if rec := e.riderMove(t, riderCookie, id, "FLYING", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown status: %d", rec.Code)
	}

	rec := e.riderMove(t, riderCookie, id, "PICKED_UP", "")
	picked := decode[orders.RiderUpdate](t, rec)
	if rec.Code != http.StatusOK || picked.OrderStatus != orders.StatusOutForDelivery || picked.Delivery.PickedUpAt == nil {
		t.Fatalf("pickup: %d %s", rec.Code, rec.Body)
	}

	rec = e.riderMove(t, riderCookie, id, "DELIVERED", "")
	done := decode[orders.RiderUpdate](t, rec)
	if rec.Code != http.StatusOK || done.OrderStatus != orders.StatusDelivered || done.PaymentStatus != "PAID" || done.Delivery.DeliveredAt == nil {
		t.Fatalf("delivered: %d %s", rec.Code, rec.Body)
	}

	detail := decode[struct{ Order orders.StaffOrder }](t, e.do(t, "GET", "/api/v1/admin/orders/"+o.ID, nil, e.staff)).Order
	if len(detail.Payments) != 1 || detail.Payments[0].RecordedBy != riderID || detail.Status != orders.StatusDelivered {
		t.Fatalf("cash credited to rider: %+v", detail.Payments)
	}
	if active := e.riderJobs(t, riderCookie, ""); len(active) != 0 {
		t.Fatalf("finished job still active: %+v", active)
	}
	if all := e.riderJobs(t, riderCookie, "?status=all"); len(all) != 1 {
		t.Fatalf("history: %+v", all)
	}
	if rec := e.riderMove(t, riderCookie, id, "DELIVERED", ""); rec.Code != http.StatusConflict {
		t.Fatalf("repeat delivery: %d", rec.Code)
	}
}

func TestFailedDeliveryCanBeReassigned(t *testing.T) {
	e := newStaffEnv(t)
	firstCookie, firstID := e.newRider(t, "rider@example.com")
	secondCookie, secondID := e.newRider(t, "rider2@example.com")
	o := e.readyDelivery(t, firstID)
	jobID := e.riderJobs(t, firstCookie, "")[0].ID

	if rec := e.do(t, "POST", "/api/v1/admin/orders/"+o.ID+"/rider", map[string]any{"rider_id": secondID}, e.staff); rec.Code != http.StatusOK {
		t.Fatalf("reassign before pickup: %d", rec.Code)
	}
	if rec := e.riderMove(t, firstCookie, jobID, "PICKED_UP", ""); rec.Code != http.StatusNotFound && rec.Code != http.StatusConflict {
		t.Fatalf("old rider still allowed: %d", rec.Code)
	}
	e.assign(t, o.ID, firstID)
	jobID = e.riderJobs(t, firstCookie, "")[0].ID

	e.riderMove(t, firstCookie, jobID, "PICKED_UP", "")
	if rec := e.do(t, "POST", "/api/v1/admin/orders/"+o.ID+"/rider", map[string]any{"rider_id": secondID}, e.staff); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "DELIVERY_IN_PROGRESS" {
		t.Fatalf("reassign after pickup: %d %s", rec.Code, rec.Body)
	}
	if rec := e.riderMove(t, firstCookie, jobID, "FAILED", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("failure needs a reason: %d", rec.Code)
	}
	rec := e.riderMove(t, firstCookie, jobID, "FAILED", "Customer not reachable")
	failed := decode[orders.RiderUpdate](t, rec)
	if rec.Code != http.StatusOK || failed.Delivery.Status != "FAILED" || failed.OrderStatus != orders.StatusOutForDelivery {
		t.Fatalf("failed: %d %s", rec.Code, rec.Body)
	}
	if rec := e.riderMove(t, firstCookie, jobID, "DELIVERED", ""); rec.Code != http.StatusConflict {
		t.Fatalf("update after failure: %d", rec.Code)
	}

	detail := decode[struct{ Order orders.StaffOrder }](t, e.do(t, "GET", "/api/v1/admin/orders/"+o.ID, nil, e.staff)).Order
	if detail.Delivery == nil || detail.Delivery.FailureReason != "Customer not reachable" {
		t.Fatalf("admin must see the failure: %+v", detail.Delivery)
	}

	again := e.assign(t, o.ID, secondID)
	if again.Status != orders.StatusReady || again.Delivery.Status != "ASSIGNED" || again.Delivery.RiderID != secondID || again.Delivery.FailureReason != "" {
		t.Fatalf("after reassign: status %s delivery %+v", again.Status, again.Delivery)
	}
	newJob := e.riderJobs(t, secondCookie, "")
	if len(newJob) != 1 {
		t.Fatalf("second rider jobs: %+v", newJob)
	}
	if rec := e.riderMove(t, firstCookie, jobID, "PICKED_UP", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("first rider after reassign: %d", rec.Code)
	}
	e.riderMove(t, secondCookie, newJob[0].ID, "PICKED_UP", "")
	if rec := e.riderMove(t, secondCookie, newJob[0].ID, "DELIVERED", ""); rec.Code != http.StatusOK {
		t.Fatalf("second attempt: %d", rec.Code)
	}
}

func TestFailedDeliveryThenStaffFailsOrder(t *testing.T) {
	e := newStaffEnv(t)
	cookie, id := e.newRider(t, "rider@example.com")
	o := e.readyDelivery(t, id)
	job := e.riderJobs(t, cookie, "")[0].ID
	e.riderMove(t, cookie, job, "PICKED_UP", "")
	e.riderMove(t, cookie, job, "FAILED", "Wrong address")

	rec := e.setStatus(t, o.ID, "FAILED", "Customer unreachable after retry")
	if rec.Code != http.StatusOK {
		t.Fatalf("fail order: %d %s", rec.Code, rec.Body)
	}
	if got := decode[struct{ Order orders.StaffOrder }](t, rec).Order; got.Status != orders.StatusFailed || got.Delivery.Status != "FAILED" {
		t.Fatalf("%+v", got)
	}
}

func TestStaffStatusChangesKeepDeliveryInStep(t *testing.T) {
	e := newStaffEnv(t)
	cookie, id := e.newRider(t, "rider@example.com")
	o := e.readyDelivery(t, id)

	out := e.mustSetStatus(t, o.ID, "OUT_FOR_DELIVERY")
	if out.Delivery.Status != "PICKED_UP" {
		t.Fatalf("manual pickup must update the delivery: %+v", out.Delivery)
	}
	done := e.mustSetStatus(t, o.ID, "DELIVERED")
	if done.Delivery.Status != "DELIVERED" || done.PaymentStatus != "PAID" {
		t.Fatalf("manual delivery: %+v", done)
	}
	if active := e.riderJobs(t, cookie, ""); len(active) != 0 {
		t.Fatalf("job still active: %+v", active)
	}

	cancelled := e.readyDelivery(t, id)
	rec := e.setStatus(t, cancelled.ID, "CANCELLED", "Customer changed mind")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	if got := decode[struct{ Order orders.StaffOrder }](t, rec).Order; got.Delivery.Status != "CANCELLED" {
		t.Fatalf("delivery after cancel: %+v", got.Delivery)
	}
	if active := e.riderJobs(t, cookie, ""); len(active) != 0 {
		t.Fatalf("cancelled job in rider list: %+v", active)
	}
}
