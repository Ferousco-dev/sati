package server

import (
	"net/http"
	"testing"

	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/users"
)

type orderEnv struct {
	*catalogEnv
	admin    *http.Cookie
	customer *http.Cookie
	product  string
	price    int64
}

func newOrderEnv(t *testing.T) *orderEnv {
	t.Helper()
	e := newCatalogEnv(t)
	admin := e.signIn(t, "admin@example.com", users.RoleAdmin)
	customer := e.signIn(t, "cust@example.com", users.RoleCustomer)
	product := e.createProduct(t, admin, e.createCategory(t, admin, "Yoghurt"), "Plain", 250000)
	return &orderEnv{catalogEnv: e, admin: admin, customer: customer, product: product, price: 250000}
}

func (e *orderEnv) address(t *testing.T, cookie *http.Cookie) string {
	t.Helper()
	rec := e.do(t, "POST", "/api/v1/addresses", map[string]any{
		"recipient_name": "Ada", "phone": "+234 801 234 5678", "address_line": "12 Main St",
		"area": "Yaba", "city": "Lagos", "instructions": "Call at the gate",
	}, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create address: %d %s", rec.Code, rec.Body)
	}
	return decode[struct{ Address addresses.Address }](t, rec).Address.ID
}

func (e *orderEnv) order(t *testing.T, body map[string]any, cookie *http.Cookie) *httptestRecorder {
	t.Helper()
	return e.do(t, "POST", "/api/v1/orders", body, cookie)
}

func pickupBody(product string, qty int) map[string]any {
	return map[string]any{
		"items":          []map[string]any{{"product_id": product, "quantity": qty}},
		"fulfillment":    map[string]any{"type": "PICKUP"},
		"payment_method": "ONLINE",
	}
}

func TestAddressesOwnershipAndValidation(t *testing.T) {
	e := newOrderEnv(t)
	other := e.signIn(t, "other@example.com", users.RoleCustomer)

	if rec := e.do(t, "GET", "/api/v1/addresses", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}

	id := e.address(t, e.customer)
	list := decode[struct{ Addresses []addresses.Address }](t, e.do(t, "GET", "/api/v1/addresses", nil, e.customer))
	if len(list.Addresses) != 1 || list.Addresses[0].Phone != "+2348012345678" {
		t.Fatalf("list: %+v", list)
	}
	if got := decode[struct{ Addresses []addresses.Address }](t, e.do(t, "GET", "/api/v1/addresses", nil, other)); len(got.Addresses) != 0 {
		t.Fatal("another customer's addresses leaked")
	}

	if rec := e.do(t, "PATCH", "/api/v1/addresses/"+id, map[string]any{"city": "Abuja"}, other); rec.Code != http.StatusNotFound {
		t.Fatalf("other user update: %d", rec.Code)
	}
	if rec := e.do(t, "DELETE", "/api/v1/addresses/"+id, nil, other); rec.Code != http.StatusNotFound {
		t.Fatalf("other user delete: %d", rec.Code)
	}
	if rec := e.do(t, "PATCH", "/api/v1/addresses/"+id, map[string]any{"city": "Abuja"}, e.customer); rec.Code != http.StatusOK {
		t.Fatalf("own update: %d", rec.Code)
	}

	invalid := []map[string]any{
		{"recipient_name": "", "phone": "08012345678", "address_line": "x", "area": "y", "city": "z"},
		{"recipient_name": "A", "phone": "abc", "address_line": "x", "area": "y", "city": "z"},
		{"recipient_name": "A", "phone": "123", "address_line": "x", "area": "y", "city": "z"},
	}
	for _, body := range invalid {
		if rec := e.do(t, "POST", "/api/v1/addresses", body, e.customer); rec.Code != http.StatusBadRequest {
			t.Fatalf("%v: %d", body, rec.Code)
		}
	}

	for i := 0; i < addresses.MaxPerUser-1; i++ {
		e.address(t, e.customer)
	}
	if rec := e.do(t, "POST", "/api/v1/addresses", map[string]any{
		"recipient_name": "A", "phone": "08012345678", "address_line": "x", "area": "y", "city": "z",
	}, e.customer); rec.Code != http.StatusConflict {
		t.Fatalf("address limit: %d", rec.Code)
	}

	if rec := e.do(t, "DELETE", "/api/v1/addresses/"+id, nil, e.customer); rec.Code != http.StatusNoContent {
		t.Fatalf("own delete: %d", rec.Code)
	}
}

func TestCreatePickupOrder(t *testing.T) {
	e := newOrderEnv(t)

	rec := e.order(t, pickupBody(e.product, 3), e.customer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	o := decode[struct{ Order orders.Order }](t, rec).Order

	if o.SubtotalKobo != 750000 || o.DeliveryFeeKobo != 0 || o.TotalKobo != 750000 {
		t.Fatalf("totals: %+v", o)
	}
	if o.Status != orders.StatusPending || o.PaymentStatus != "UNPAID" || o.Currency != "NGN" {
		t.Fatalf("states: %+v", o)
	}
	if len(o.Number) < 10 || o.Number[:5] != "SATI-" {
		t.Fatalf("order number: %q", o.Number)
	}
	if len(o.Items) != 1 || o.Items[0].Name != "Plain" || o.Items[0].UnitPriceKobo != e.price || o.Items[0].LineTotalKobo != 750000 {
		t.Fatalf("items: %+v", o.Items)
	}

	e.do(t, "PATCH", "/api/v1/admin/products/"+e.product, map[string]any{"price_kobo": 999999, "name": "Renamed"}, e.admin)
	again := decode[struct{ Order orders.Order }](t, e.do(t, "GET", "/api/v1/orders/"+o.ID, nil, e.customer)).Order
	if again.TotalKobo != 750000 || again.Items[0].Name != "Plain" || again.Items[0].UnitPriceKobo != e.price {
		t.Fatalf("history changed after product edit: %+v", again)
	}

	fresh := decode[struct{ Order orders.Order }](t, e.order(t, pickupBody(e.product, 1), e.customer)).Order
	if fresh.TotalKobo != 999999 {
		t.Fatalf("new order must use the current price, got %d", fresh.TotalKobo)
	}
}

func TestClientCannotSetPrices(t *testing.T) {
	e := newOrderEnv(t)
	body := pickupBody(e.product, 1)
	body["total_kobo"] = 1
	if rec := e.order(t, body, e.customer); rec.Code != http.StatusBadRequest {
		t.Fatalf("client total accepted: %d", rec.Code)
	}
	body = pickupBody(e.product, 1)
	body["items"] = []map[string]any{{"product_id": e.product, "quantity": 1, "price_kobo": 1}}
	if rec := e.order(t, body, e.customer); rec.Code != http.StatusBadRequest {
		t.Fatalf("client price accepted: %d", rec.Code)
	}
}

func TestMergedLinesAndLimits(t *testing.T) {
	e := newOrderEnv(t)
	body := pickupBody(e.product, 1)
	body["items"] = []map[string]any{{"product_id": e.product, "quantity": 2}, {"product_id": e.product, "quantity": 3}}
	o := decode[struct{ Order orders.Order }](t, e.order(t, body, e.customer)).Order
	if len(o.Items) != 1 || o.Items[0].Quantity != 5 || o.TotalKobo != 1250000 {
		t.Fatalf("merge: %+v", o)
	}
}

func TestDeliveryOrderUsesStoreFee(t *testing.T) {
	e := newOrderEnv(t)
	addr := e.address(t, e.customer)

	if rec := e.do(t, "PATCH", "/api/v1/admin/settings", map[string]any{"delivery_fee_kobo": 150000}, e.admin); rec.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(t, "PATCH", "/api/v1/admin/settings", map[string]any{"delivery_fee_kobo": 1}, e.customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer settings: %d", rec.Code)
	}

	body := map[string]any{
		"items":          []map[string]any{{"product_id": e.product, "quantity": 2}},
		"fulfillment":    map[string]any{"type": "DELIVERY", "address_id": addr},
		"payment_method": "ONLINE",
	}
	rec := e.order(t, body, e.customer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("delivery order: %d %s", rec.Code, rec.Body)
	}
	o := decode[struct{ Order orders.Order }](t, rec).Order
	if o.SubtotalKobo != 500000 || o.DeliveryFeeKobo != 150000 || o.TotalKobo != 650000 {
		t.Fatalf("totals: %+v", o)
	}
	if o.DeliveryAddress == nil || o.DeliveryAddress.City != "Lagos" {
		t.Fatalf("address snapshot: %+v", o.DeliveryAddress)
	}

	e.do(t, "PATCH", "/api/v1/addresses/"+addr, map[string]any{"city": "Abuja"}, e.customer)
	e.do(t, "DELETE", "/api/v1/addresses/"+addr, nil, e.customer)
	kept := decode[struct{ Order orders.Order }](t, e.do(t, "GET", "/api/v1/orders/"+o.ID, nil, e.customer)).Order
	if kept.DeliveryAddress == nil || kept.DeliveryAddress.City != "Lagos" {
		t.Fatalf("snapshot must survive address edits and deletes: %+v", kept.DeliveryAddress)
	}

	other := e.signIn(t, "other@example.com", users.RoleCustomer)
	theirs := e.address(t, other)
	body["fulfillment"] = map[string]any{"type": "DELIVERY", "address_id": theirs}
	if rec := e.order(t, body, e.customer); rec.Code != http.StatusNotFound {
		t.Fatalf("someone else's address: %d", rec.Code)
	}
}

func TestPayOnPickupRules(t *testing.T) {
	e := newOrderEnv(t)
	body := pickupBody(e.product, 1)
	body["payment_method"] = "PAY_ON_PICKUP"
	rec := e.order(t, body, e.customer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("pay on pickup: %d %s", rec.Code, rec.Body)
	}
	if o := decode[struct{ Order orders.Order }](t, rec).Order; o.PaymentMethod != orders.PayOnPickup || o.PaymentStatus != "UNPAID" {
		t.Fatalf("%+v", o)
	}

	body = map[string]any{
		"items":          []map[string]any{{"product_id": e.product, "quantity": 1}},
		"fulfillment":    map[string]any{"type": "DELIVERY", "address_id": e.address(t, e.customer)},
		"payment_method": "PAY_ON_PICKUP",
	}
	if rec := e.order(t, body, e.customer); rec.Code != http.StatusBadRequest {
		t.Fatalf("pay on pickup with delivery: %d", rec.Code)
	}
}

func TestUnavailableProductsRejected(t *testing.T) {
	e := newOrderEnv(t)

	e.do(t, "POST", "/api/v1/admin/products/"+e.product+"/availability", map[string]any{"available": false}, e.admin)
	rec := e.order(t, pickupBody(e.product, 1), e.customer)
	if rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "PRODUCT_UNAVAILABLE" {
		t.Fatalf("unavailable: %d %s", rec.Code, rec.Body)
	}

	e.do(t, "POST", "/api/v1/admin/products/"+e.product+"/availability", map[string]any{"available": true}, e.admin)
	e.do(t, "POST", "/api/v1/admin/products/"+e.product+"/archive", nil, e.admin)
	if rec := e.order(t, pickupBody(e.product, 1), e.customer); rec.Code != http.StatusConflict {
		t.Fatalf("archived: %d", rec.Code)
	}

	if rec := e.order(t, pickupBody("00000000-0000-0000-0000-000000000000", 1), e.customer); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown product: %d", rec.Code)
	}
}

func TestIdempotencyKey(t *testing.T) {
	e := newOrderEnv(t)
	send := func() *httptestRecorder {
		return e.doWithHeader(t, "POST", "/api/v1/orders", pickupBody(e.product, 1), e.customer, "Idempotency-Key", "abc-123")
	}

	first := send()
	second := send()
	if first.Code != http.StatusCreated || second.Code != http.StatusOK {
		t.Fatalf("statuses: %d %d", first.Code, second.Code)
	}
	a := decode[struct{ Order orders.Order }](t, first).Order
	b := decode[struct{ Order orders.Order }](t, second).Order
	if a.ID != b.ID || len(b.Items) != 1 {
		t.Fatalf("retry must return the same order: %s vs %s", a.ID, b.ID)
	}

	list := decode[orders.Page](t, e.do(t, "GET", "/api/v1/orders", nil, e.customer))
	if list.Total != 1 {
		t.Fatalf("retry created a duplicate order: %d", list.Total)
	}
}

func TestOrderPrivacy(t *testing.T) {
	e := newOrderEnv(t)
	other := e.signIn(t, "other@example.com", users.RoleCustomer)
	o := decode[struct{ Order orders.Order }](t, e.order(t, pickupBody(e.product, 1), e.customer)).Order

	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/orders/" + o.ID},
		{"GET", "/api/v1/orders/" + o.ID + "/status"},
		{"POST", "/api/v1/orders/" + o.ID + "/cancel"},
	} {
		if rec := e.do(t, c.method, c.path, nil, other); rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s as another customer: %d", c.method, c.path, rec.Code)
		}
	}
	if list := decode[orders.Page](t, e.do(t, "GET", "/api/v1/orders", nil, other)); list.Total != 0 {
		t.Fatal("another customer's orders listed")
	}
	if rec := e.do(t, "GET", "/api/v1/orders/"+o.ID, nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/orders/not-a-uuid", nil, e.customer); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}

	view := decode[orders.StatusView](t, e.do(t, "GET", "/api/v1/orders/"+o.ID+"/status", nil, e.customer))
	if view.Status != orders.StatusPending || view.PaymentStatus != "UNPAID" {
		t.Fatalf("status: %+v", view)
	}
}

func TestCancelOrder(t *testing.T) {
	e := newOrderEnv(t)
	o := decode[struct{ Order orders.Order }](t, e.order(t, pickupBody(e.product, 1), e.customer)).Order

	rec := e.do(t, "POST", "/api/v1/orders/"+o.ID+"/cancel", nil, e.customer)
	if rec.Code != http.StatusOK || decode[struct{ Order orders.Order }](t, rec).Order.Status != orders.StatusCancelled {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(t, "POST", "/api/v1/orders/"+o.ID+"/cancel", nil, e.customer); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "ORDER_NOT_CANCELLABLE" {
		t.Fatalf("second cancel: %d", rec.Code)
	}

	confirmed := decode[struct{ Order orders.Order }](t, e.order(t, pickupBody(e.product, 1), e.customer)).Order
	e.exec(`UPDATE orders SET status = 'CONFIRMED' WHERE id = $1`, confirmed.ID)
	if rec := e.do(t, "POST", "/api/v1/orders/"+confirmed.ID+"/cancel", nil, e.customer); rec.Code != http.StatusConflict {
		t.Fatalf("cancel confirmed order: %d", rec.Code)
	}
}

func TestListOrdersPagination(t *testing.T) {
	e := newOrderEnv(t)
	for i := 0; i < 3; i++ {
		e.order(t, pickupBody(e.product, i+1), e.customer)
	}
	page := decode[orders.Page](t, e.do(t, "GET", "/api/v1/orders?limit=2", nil, e.customer))
	if page.Total != 3 || len(page.Orders) != 2 || len(page.Orders[0].Items) != 1 {
		t.Fatalf("page: %+v", page)
	}
	if !page.Orders[0].CreatedAt.After(page.Orders[1].CreatedAt) && !page.Orders[0].CreatedAt.Equal(page.Orders[1].CreatedAt) {
		t.Fatal("orders must be newest first")
	}
}
