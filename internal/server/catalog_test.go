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

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Ferousco-dev/sati/backend/db"
	"github.com/Ferousco-dev/sati/backend/internal/auth"
	"github.com/Ferousco-dev/sati/backend/internal/categories"
	"github.com/Ferousco-dev/sati/backend/internal/files"
	"github.com/Ferousco-dev/sati/backend/internal/health"
	"github.com/Ferousco-dev/sati/backend/internal/products"
	"github.com/Ferousco-dev/sati/backend/internal/users"
	"github.com/Ferousco-dev/sati/backend/pkg/config"
	"github.com/Ferousco-dev/sati/backend/pkg/database"
)

type noopMailer struct{}

func (noopMailer) SendVerification(context.Context, string, string, string) error  { return nil }
func (noopMailer) SendPasswordReset(context.Context, string, string, string) error { return nil }

type catalogEnv struct {
	srv  http.Handler
	exec func(query string, args ...any)
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

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	storage, err := files.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	authSvc := auth.NewService(pool, noopMailer{}, log, "test-secret", auth.DefaultSettings())
	handlers := Handlers{
		Health:     health.New(pool),
		Auth:       auth.NewHandler(authSvc, log, false),
		Categories: categories.NewHandler(categories.NewService(categories.NewRepository(pool)), log),
		Products:   products.NewHandler(products.NewService(pool, storage, log), log),
		LocalFiles: storage.Handler(),
	}
	cfg := config.Config{CORSAllowedOrigins: []string{"http://localhost:3000"}}

	return &catalogEnv{
		srv: New(cfg, log, handlers),
		exec: func(query string, args ...any) {
			if _, err := pool.Exec(context.Background(), query, args...); err != nil {
				t.Fatal(err)
			}
		},
	}
}

func (e *catalogEnv) do(t *testing.T, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if cookie != nil {
		req.AddCookie(cookie)
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
	if rec := e.do(t, "GET", img.URL, nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted image still served: %d", rec.Code)
	}
	if rec := e.do(t, "DELETE", path+"/"+img.ID, nil, admin); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: %d", rec.Code)
	}
}
