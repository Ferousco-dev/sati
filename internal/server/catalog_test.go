package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Ferousco-dev/sati/backend/db"
	"github.com/Ferousco-dev/sati/backend/internal/addresses"
	"github.com/Ferousco-dev/sati/backend/internal/admin"
	"github.com/Ferousco-dev/sati/backend/internal/analytics"
	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/categories"
	"github.com/Ferousco-dev/sati/backend/internal/files"
	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/internal/orders"
	"github.com/Ferousco-dev/sati/backend/internal/payments"
	"github.com/Ferousco-dev/sati/backend/internal/products"
	"github.com/Ferousco-dev/sati/backend/internal/store"
	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

type captureMailer struct{ invites map[string]string }

func (*captureMailer) SendVerification(context.Context, string, string, string) error  { return nil }
func (*captureMailer) SendPasswordReset(context.Context, string, string, string) error { return nil }

func (m *captureMailer) SendInvite(_ context.Context, _, email string, _ users.Role, token string) error {
	m.invites[email] = token
	return nil
}

type catalogEnv struct {
	srv    http.Handler
	mailer *captureMailer
	exec   func(query string, args ...any)
}

func newCatalogEnv(t *testing.T) *catalogEnv {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	conn, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(db.Migrations)
	goose.SetLogger(goose.NopLogger())
	_ = goose.SetDialect("postgres")
	if err := goose.Up(conn, "migrations"); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), `TRUNCATE users, categories CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE store_settings SET delivery_fee_kobo = 0, online_enabled = TRUE,
		bank_transfer_enabled = TRUE, pay_on_pickup_enabled = TRUE, pay_on_delivery_enabled = TRUE,
		bank_name = '', bank_account_number = '', bank_account_name = '', whatsapp_number = ''`); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	storage, err := files.NewLocalStorage(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}

	mailer := &captureMailer{invites: map[string]string{}}
	authSvc := auth.NewService(pool, mailer, log, "test-secret", auth.DefaultSettings())
	lagos, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		t.Fatal(err)
	}
	handlers := Handlers{
		Health:     health.New(pool),
		Auth:       auth.NewHandler(authSvc, log, false),
		Categories: categories.NewHandler(categories.NewService(categories.NewRepository(pool)), log),
		Products:   products.NewHandler(products.NewService(pool, storage, log), log),
		Admin:      admin.NewHandler(admin.NewService(pool, authSvc), log),
		Addresses:  addresses.NewHandler(addresses.NewService(pool), log),
		Orders:     orders.NewHandler(orders.NewService(pool), log),
		Store:      store.NewHandler(pool, log),
		Payments:   payments.NewHandler(pool, log),
		Analytics:  analytics.NewHandler(analytics.NewService(pool, lagos), log),
		LocalFiles: storage.Handler(),
	}
	cfg := config.Config{CORSAllowedOrigins: []string{"http://localhost:3000"}}

	return &catalogEnv{
		srv:    New(cfg, log, handlers),
		mailer: mailer,
		exec: func(query string, args ...any) {
			if _, err := pool.Exec(context.Background(), query, args...); err != nil {
				t.Fatal(err)
			}
		},
	}
}

type httptestRecorder = httptest.ResponseRecorder

func errorCodeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[struct{ Error struct{ Code string } }](t, rec).Error.Code
}

func (e *catalogEnv) do(t *testing.T, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return e.doWithHeader(t, method, path, body, cookie, "", "")
}

func (e *catalogEnv) doWithHeader(t *testing.T, method, path string, body any, cookie *http.Cookie, header, value string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if header != "" {
		req.Header.Set(header, value)
	}
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

func (e *catalogEnv) signIn(t *testing.T, email string, role users.Role) *http.Cookie {
	t.Helper()
	rec := e.do(t, "POST", "/api/v1/auth/register", map[string]any{"name": "T", "email": email, "password": "correct-horse"}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	if role != users.RoleCustomer {
		e.exec(`UPDATE users SET role = $2::user_role WHERE email = $1`, email, string(role))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	t.Fatal("no cookie")
	return nil
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
	return v
}

func (e *catalogEnv) createCategory(t *testing.T, admin *http.Cookie, name string) string {
	t.Helper()
	rec := e.do(t, "POST", "/api/v1/admin/categories", map[string]any{"name": name}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category: %d %s", rec.Code, rec.Body)
	}
	return decode[struct{ Category struct{ ID string } }](t, rec).Category.ID
}

func (e *catalogEnv) createProduct(t *testing.T, admin *http.Cookie, categoryID, name string, price int64) string {
	t.Helper()
	rec := e.do(t, "POST", "/api/v1/admin/products", map[string]any{
		"category_id": categoryID, "name": name, "price_kobo": price,
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product: %d %s", rec.Code, rec.Body)
	}
	return decode[struct{ Product struct{ ID string } }](t, rec).Product.ID
}

func TestAdminRoutesRequireAdmin(t *testing.T) {
	e := newCatalogEnv(t)
	customer := e.signIn(t, "c@example.com", users.RoleCustomer)

	if rec := e.do(t, "GET", "/api/v1/admin/products", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: got %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/products", nil, customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer: got %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/admin/categories", map[string]any{"name": "X"}, customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer create category: got %d", rec.Code)
	}
}

func TestCatalogLifecycle(t *testing.T) {
	e := newCatalogEnv(t)
	admin := e.signIn(t, "a@example.com", users.RoleAdmin)

	cat := e.createCategory(t, admin, "Yoghurt")
	if rec := e.do(t, "POST", "/api/v1/admin/categories", map[string]any{"name": "Yoghurt"}, admin); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate category: got %d", rec.Code)
	}

	product := e.createProduct(t, admin, cat, "Plain", 250000)

	bad := []map[string]any{
		{"category_id": cat, "name": "", "price_kobo": 100},
		{"category_id": cat, "name": "Neg", "price_kobo": -1},
		{"category_id": "00000000-0000-0000-0000-000000000000", "name": "Ghost", "price_kobo": 100},
	}
	for _, body := range bad {
		if rec := e.do(t, "POST", "/api/v1/admin/products", body, admin); rec.Code != http.StatusBadRequest {
			t.Fatalf("%v: got %d", body, rec.Code)
		}
	}

	list := decode[products.Page](t, e.do(t, "GET", "/api/v1/products", nil, nil))
	if list.Total != 1 || list.Products[0].PriceKobo != 250000 || list.Products[0].Currency != "NGN" {
		t.Fatalf("public list: %+v", list)
	}

	if rec := e.do(t, "POST", "/api/v1/admin/products/"+product+"/availability", map[string]any{"available": false}, admin); rec.Code != http.StatusOK {
		t.Fatalf("availability: %d", rec.Code)
	}
	one := decode[struct{ Product products.Product }](t, e.do(t, "GET", "/api/v1/products/"+product, nil, nil))
	if one.Product.Available {
		t.Fatal("product should be unavailable")
	}

	rec := e.do(t, "PATCH", "/api/v1/admin/products/"+product, map[string]any{"price_kobo": 275000, "name": "Plain 500ml"}, admin)
	updated := decode[struct{ Product products.Product }](t, rec)
	if rec.Code != http.StatusOK || updated.Product.PriceKobo != 275000 || updated.Product.Name != "Plain 500ml" {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}

	byCategory := decode[products.Page](t, e.do(t, "GET", "/api/v1/categories/"+cat+"/products", nil, nil))
	if byCategory.Total != 1 {
		t.Fatalf("by category: %+v", byCategory)
	}

	if rec := e.do(t, "POST", "/api/v1/admin/products/"+product+"/archive", nil, admin); rec.Code != http.StatusOK {
		t.Fatalf("archive: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/products/"+product, nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("archived product public get: got %d", rec.Code)
	}
	if after := decode[products.Page](t, e.do(t, "GET", "/api/v1/products", nil, nil)); after.Total != 0 {
		t.Fatalf("archived product still listed: %+v", after)
	}
	adminList := decode[products.Page](t, e.do(t, "GET", "/api/v1/admin/products", nil, admin))
	if adminList.Total != 1 || !adminList.Products[0].Archived {
		t.Fatalf("admin should still see archived product: %+v", adminList)
	}
	if rec := e.do(t, "PATCH", "/api/v1/admin/products/"+product, map[string]any{"name": "Revived"}, admin); rec.Code != http.StatusNotFound {
		t.Fatalf("archived product update: got %d", rec.Code)
	}
}

func TestInactiveCategoryHidesProducts(t *testing.T) {
	e := newCatalogEnv(t)
	admin := e.signIn(t, "a@example.com", users.RoleAdmin)
	cat := e.createCategory(t, admin, "Seasonal")
	product := e.createProduct(t, admin, cat, "Mango", 100)

	rec := e.do(t, "PATCH", "/api/v1/admin/categories/"+cat, map[string]any{"active": false}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("deactivate: %d", rec.Code)
	}

	if cats := decode[struct{ Categories []categories.Category }](t, e.do(t, "GET", "/api/v1/categories", nil, nil)); len(cats.Categories) != 0 {
		t.Fatal("inactive category listed")
	}
	if rec := e.do(t, "GET", "/api/v1/categories/"+cat+"/products", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("inactive category products: got %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/products/"+product, nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("product in inactive category: got %d", rec.Code)
	}
}

func TestUnknownAndMalformedIDs(t *testing.T) {
	e := newCatalogEnv(t)
	for _, path := range []string{"/api/v1/products/not-a-uuid", "/api/v1/products/00000000-0000-0000-0000-000000000000", "/api/v1/categories/xyz/products"} {
		if rec := e.do(t, "GET", path, nil, nil); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: got %d", path, rec.Code)
		}
	}
	if rec := e.do(t, "GET", "/api/v1/products?limit=0", nil, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad limit: got %d", rec.Code)
	}
}

func upload(t *testing.T, e *catalogEnv, path string, cookie *http.Cookie, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("image", "photo.png")
	part.Write(data)
	w.Close()

	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

func TestProductImages(t *testing.T) {
	e := newCatalogEnv(t)
	admin := e.signIn(t, "a@example.com", users.RoleAdmin)
	product := e.createProduct(t, admin, e.createCategory(t, admin, "Yoghurt"), "Plain", 100)
	path := "/api/v1/admin/products/" + product + "/images"

	var pngData bytes.Buffer
	_ = png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 4, 4)))

	if rec := upload(t, e, path, admin, []byte("<html>not an image</html>")); rec.Code != http.StatusBadRequest {
		t.Fatalf("non-image: got %d %s", rec.Code, rec.Body)
	}
	if rec := upload(t, e, path, admin, make([]byte, products.MaxImageBytes+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized: got %d", rec.Code)
	}

	rec := upload(t, e, path, admin, pngData.Bytes())
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	img := decode[struct{ Image products.Image }](t, rec).Image

	served := e.do(t, "GET", img.URL, nil, nil)
	if served.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("image cache header: %q", served.Header().Get("Cache-Control"))
	}
	if served.Code != http.StatusOK || !bytes.Equal(served.Body.Bytes(), pngData.Bytes()) {
		t.Fatalf("serve image: %d", served.Code)
	}

	got := decode[struct{ Product products.Product }](t, e.do(t, "GET", "/api/v1/products/"+product, nil, nil))
	if len(got.Product.Images) != 1 || got.Product.Images[0].URL != img.URL {
		t.Fatalf("product images: %+v", got.Product.Images)
	}

	if rec := e.do(t, "DELETE", path+"/"+img.ID, nil, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("delete image: %d", rec.Code)
	}
	if rec := e.do(t, "GET", img.URL, nil, nil); rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") == "public, max-age=31536000, immutable" {
		t.Fatalf("deleted image still served or its 404 cached: %d", rec.Code)
	}
	if rec := e.do(t, "DELETE", path+"/"+img.ID, nil, admin); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: %d", rec.Code)
	}
}

func TestStaffAndRiderAccess(t *testing.T) {
	e := newCatalogEnv(t)
	admin := e.signIn(t, "a@example.com", users.RoleAdmin)
	staff := e.signIn(t, "s@example.com", users.RoleStaff)
	rider := e.signIn(t, "r@example.com", users.RoleRider)

	cat := e.createCategory(t, admin, "Yoghurt")
	product := e.createProduct(t, admin, cat, "Plain", 100)

	if rec := e.do(t, "GET", "/api/v1/admin/products", nil, staff); rec.Code != http.StatusOK {
		t.Fatalf("staff list: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/categories", nil, staff); rec.Code != http.StatusOK {
		t.Fatalf("staff categories: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/admin/products/"+product+"/availability", map[string]any{"available": false}, staff); rec.Code != http.StatusOK {
		t.Fatalf("staff availability: %d", rec.Code)
	}

	forbiddenForStaff := []struct{ method, path string }{
		{"POST", "/api/v1/admin/products"},
		{"PATCH", "/api/v1/admin/products/" + product},
		{"POST", "/api/v1/admin/products/" + product + "/archive"},
		{"POST", "/api/v1/admin/categories"},
		{"GET", "/api/v1/admin/users"},
		{"POST", "/api/v1/admin/users"},
	}
	for _, c := range forbiddenForStaff {
		if rec := e.do(t, c.method, c.path, map[string]any{}, staff); rec.Code != http.StatusForbidden {
			t.Fatalf("staff %s %s: got %d", c.method, c.path, rec.Code)
		}
	}

	for _, path := range []string{"/api/v1/admin/products", "/api/v1/admin/categories", "/api/v1/admin/users"} {
		if rec := e.do(t, "GET", path, nil, rider); rec.Code != http.StatusForbidden {
			t.Fatalf("rider %s: got %d", path, rec.Code)
		}
	}
}

func TestInviteAndRoleManagement(t *testing.T) {
	e := newCatalogEnv(t)
	admin := e.signIn(t, "a@example.com", users.RoleAdmin)

	rec := e.do(t, "POST", "/api/v1/admin/users", map[string]any{"name": "Sam", "email": "Sam@Example.com", "role": "STAFF"}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite: %d %s", rec.Code, rec.Body)
	}
	invited := decode[struct{ User users.User }](t, rec).User
	if invited.Role != users.RoleStaff {
		t.Fatalf("role: %s", invited.Role)
	}

	token := e.mailer.invites["sam@example.com"]
	if token == "" {
		t.Fatal("no invite email")
	}
	if rec := e.do(t, "POST", "/api/v1/admin/users", map[string]any{"name": "Sam", "email": "sam@example.com", "role": "STAFF"}, admin); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate invite: %d", rec.Code)
	}
	if rec := e.do(t, "POST", "/api/v1/admin/users", map[string]any{"name": "Cust", "email": "c@example.com", "role": "CUSTOMER"}, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("invite as customer: %d", rec.Code)
	}

	if rec := e.do(t, "POST", "/api/v1/auth/reset-password", map[string]any{"token": token, "password": "staff-password-1"}, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("set password: %d %s", rec.Code, rec.Body)
	}
	login := e.do(t, "POST", "/api/v1/auth/login", map[string]any{"email": "sam@example.com", "password": "staff-password-1"}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("invited login: %d", login.Code)
	}
	var staffCookie *http.Cookie
	for _, c := range login.Result().Cookies() {
		staffCookie = c
	}
	if rec := e.do(t, "GET", "/api/v1/admin/products", nil, staffCookie); rec.Code != http.StatusOK {
		t.Fatalf("new staff access: %d", rec.Code)
	}

	promote := e.do(t, "PATCH", "/api/v1/admin/users/"+invited.ID+"/role", map[string]any{"role": "RIDER"}, admin)
	if promote.Code != http.StatusOK || decode[struct{ User users.User }](t, promote).User.Role != users.RoleRider {
		t.Fatalf("change role: %d %s", promote.Code, promote.Body)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/products", nil, staffCookie); rec.Code != http.StatusForbidden {
		t.Fatalf("role change must apply immediately: %d", rec.Code)
	}

	self := decode[struct{ User users.User }](t, e.do(t, "GET", "/api/v1/auth/me", nil, admin)).User
	if rec := e.do(t, "PATCH", "/api/v1/admin/users/"+self.ID+"/role", map[string]any{"role": "CUSTOMER"}, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("self role change: %d", rec.Code)
	}
	if rec := e.do(t, "PATCH", "/api/v1/admin/users/"+invited.ID+"/role", map[string]any{"role": "BOSS"}, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad role: %d", rec.Code)
	}

	list := decode[struct {
		Users []users.User
		Total int
	}](t, e.do(t, "GET", "/api/v1/admin/users?role=RIDER", nil, admin))
	if list.Total != 1 || list.Users[0].Email != "sam@example.com" {
		t.Fatalf("rider filter: %+v", list)
	}
}

func TestCaching(t *testing.T) {
	e := newCatalogEnv(t)
	admin := e.signIn(t, "a@example.com", users.RoleAdmin)
	e.createProduct(t, admin, e.createCategory(t, admin, "Yoghurt"), "Plain", 100)

	first := e.do(t, "GET", "/api/v1/products", nil, nil)
	etag := first.Header().Get("ETag")
	if etag == "" || first.Header().Get("Cache-Control") != "public, max-age=60, stale-while-revalidate=300" {
		t.Fatalf("headers: %v", first.Header())
	}
	if first.Header().Get("Vary") != "Origin" {
		t.Fatalf("shared caches need Vary: Origin, got %q", first.Header().Get("Vary"))
	}

	req := httptest.NewRequest("GET", "/api/v1/products", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("conditional request: %d", rec.Code)
	}

	e.createProduct(t, admin, e.createCategory(t, admin, "Parfait"), "Fruit", 200)
	req = httptest.NewRequest("GET", "/api/v1/products", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("changed data must not return 304: %d", rec.Code)
	}

	for _, path := range []string{"/api/v1/auth/me", "/api/v1/admin/products", "/api/v1/health"} {
		if got := e.do(t, "GET", path, nil, admin).Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s must not be cached, got %q", path, got)
		}
	}
	if got := e.do(t, "GET", "/api/v1/products/not-a-uuid", nil, nil).Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("errors must not be cached, got %q", got)
	}
}
