package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Ferousco-dev/sati/backend/internal/audit"
	"github.com/Ferousco-dev/sati/backend/internal/users"
)

type auditPage struct {
	AuditLogs []audit.Record `json:"audit_logs"`
	Total     int
}

func (e *catalogEnv) auditLogs(t *testing.T, admin *http.Cookie, query string) auditPage {
	t.Helper()
	rec := e.do(t, "GET", "/api/v1/admin/audit-logs"+query, nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit logs: %d %s", rec.Code, rec.Body)
	}
	return decode[auditPage](t, rec)
}

func TestAdminChangesAreAudited(t *testing.T) {
	e := newStaffEnv(t)
	cat := e.createCategory(t, e.admin, "Parfait")
	product := e.createProduct(t, e.admin, cat, "Plain", 100)
	e.settings(t, map[string]any{"bank_account_number": "0123456789", "bank_name": "Secret Bank"})
	e.do(t, "POST", "/api/v1/admin/products/"+product+"/availability", map[string]any{"available": false}, e.staff)

	logs := e.auditLogs(t, e.admin, "")
	byAction := map[string]audit.Record{}
	for _, r := range logs.AuditLogs {
		byAction[r.Action] = r
	}

	create := byAction["POST /api/v1/admin/products"]
	if create.ActorRole != "ADMIN" || create.ActorName == "" || create.EntityType != "products" || create.Status != http.StatusCreated || create.IP == "" || create.RequestID == "" {
		t.Fatalf("product create entry: %+v", create)
	}
	avail := byAction["POST /api/v1/admin/products/{id}/availability"]
	if avail.ActorRole != "STAFF" || avail.EntityID != product || avail.Metadata["id"] != product {
		t.Fatalf("staff availability entry: %+v", avail)
	}
	if _, ok := byAction["PATCH /api/v1/admin/settings"]; !ok {
		t.Fatal("settings changes must be audited")
	}

	for _, r := range logs.AuditLogs {
		if strings.HasPrefix(r.Action, "GET ") {
			t.Fatalf("reads must not be audited: %s", r.Action)
		}
	}
}

func TestAuditNeverStoresRequestContents(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{"bank_account_number": "0123456789", "bank_name": "Secret Bank", "whatsapp_number": "2348012345678"})
	e.do(t, "POST", "/api/v1/admin/users", map[string]any{"name": "Sam", "email": "sam@example.com", "role": "STAFF"}, e.admin)

	var dump string
	if err := e.queryRow(`SELECT COALESCE(string_agg(metadata::text || action || entity_id, ' '), '') FROM audit_logs`).Scan(&dump); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"0123456789", "Secret Bank", "2348012345678", "sam@example.com"} {
		if strings.Contains(dump, secret) {
			t.Fatalf("audit log contains request data %q", secret)
		}
	}
}

func TestFailedAndCustomerRequestsAreNotAudited(t *testing.T) {
	e := newStaffEnv(t)
	before := e.auditLogs(t, e.admin, "").Total

	e.do(t, "POST", "/api/v1/admin/products", map[string]any{"name": ""}, e.admin)
	e.do(t, "POST", "/api/v1/admin/products", map[string]any{"name": "x"}, e.customer)
	e.place(t, "PICKUP", "PAY_ON_PICKUP")

	if after := e.auditLogs(t, e.admin, "").Total; after != before {
		t.Fatalf("rejected requests and customer actions must not be audited: %d -> %d", before, after)
	}
}

func TestOrderWorkIsAuditedWithTheOrder(t *testing.T) {
	e := newStaffEnv(t)
	o := e.place(t, "PICKUP", "PAY_ON_PICKUP")
	e.mustSetStatus(t, o.ID, "CONFIRMED")

	logs := e.auditLogs(t, e.admin, "?entity_type=orders&entity_id="+o.ID)
	if logs.Total != 1 || logs.AuditLogs[0].Action != "PATCH /api/v1/admin/orders/{id}/status" || logs.AuditLogs[0].ActorRole != "STAFF" {
		t.Fatalf("%+v", logs)
	}
}

func TestSignInEventsAreAudited(t *testing.T) {
	e := newStaffEnv(t)
	e.signIn(t, "boss@example.com", users.RoleAdmin)

	e.do(t, "POST", "/api/v1/auth/login", map[string]any{"email": "Boss@Example.com", "password": "correct-horse"}, nil)
	e.do(t, "POST", "/api/v1/auth/login", map[string]any{"email": "cust@example.com", "password": "correct-horse"}, nil)
	e.do(t, "POST", "/api/v1/auth/login", map[string]any{"email": "boss@example.com", "password": "not-the-password"}, nil)

	logins := e.auditLogs(t, e.admin, "?action=auth.login")
	var success, failed int
	for _, r := range logins.AuditLogs {
		switch r.Action {
		case "auth.login":
			success++
			if r.ActorRole != "ADMIN" {
				t.Fatalf("successful logins are recorded for staff only: %+v", r)
			}
		case "auth.login_failed":
			failed++
			if r.Metadata["email"] != "boss@example.com" || strings.Contains(strings.ToLower(toJSON(r.Metadata)), "password") {
				t.Fatalf("failed login entry: %+v", r)
			}
		}
	}
	if success != 1 || failed != 1 {
		t.Fatalf("customer sign-ins must not be logged, a failed attempt must: success=%d failed=%d", success, failed)
	}
}

func toJSON(m map[string]any) string {
	var b strings.Builder
	for k, v := range m {
		b.WriteString(k)
		b.WriteString(":")
		b.WriteString(strings.ToLower(strings.TrimSpace(strings.Join([]string{asString(v)}, ""))))
	}
	return b.String()
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestAuditLogEndpoint(t *testing.T) {
	e := newStaffEnv(t)
	e.createCategory(t, e.admin, "Drinks")
	e.createCategory(t, e.admin, "Parfait")

	if rec := e.do(t, "GET", "/api/v1/admin/audit-logs", nil, e.staff); rec.Code != http.StatusForbidden {
		t.Fatalf("staff: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/audit-logs", nil, e.customer); rec.Code != http.StatusForbidden {
		t.Fatalf("customer: %d", rec.Code)
	}
	if rec := e.do(t, "GET", "/api/v1/admin/audit-logs", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}

	page := e.auditLogs(t, e.admin, "?limit=1&action=categories")
	if page.Total != 3 || len(page.AuditLogs) != 1 {
		t.Fatalf("pagination and search: %+v", page)
	}
	if got := e.auditLogs(t, e.admin, "?action=%25"); got.Total != 0 {
		t.Fatalf("wildcards must be escaped: %d", got.Total)
	}
	adminID := e.userID(t, "admin@example.com")
	if got := e.auditLogs(t, e.admin, "?actor_id="+adminID); got.Total < 2 {
		t.Fatalf("actor filter: %d", got.Total)
	}
	if got := e.auditLogs(t, e.admin, "?from=2999-01-01T00:00:00Z"); got.Total != 0 {
		t.Fatalf("date filter: %d", got.Total)
	}
	for _, bad := range []string{"?actor_id=nope", "?from=yesterday", "?limit=0"} {
		if rec := e.do(t, "GET", "/api/v1/admin/audit-logs"+bad, nil, e.admin); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", bad, rec.Code)
		}
	}
}
