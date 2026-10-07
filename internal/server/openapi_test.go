package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Ferousco-dev/sati/backend/api"
	"github.com/Ferousco-dev/sati/backend/internal/users"
)

type operation struct {
	Summary   string         `yaml:"summary"`
	Access    string         `yaml:"x-access"`
	Responses map[string]any `yaml:"responses"`
}

type spec struct {
	OpenAPI string                          `yaml:"openapi"`
	Paths   map[string]map[string]yaml.Node `yaml:"paths"`
}

type docRoute struct {
	method, path string
	op           operation
}

func loadSpec(t *testing.T) []docRoute {
	t.Helper()
	var s spec
	if err := yaml.Unmarshal(api.Spec, &s); err != nil {
		t.Fatalf("openapi.yaml is not valid YAML: %v", err)
	}
	if !strings.HasPrefix(s.OpenAPI, "3.") {
		t.Fatalf("openapi version %q", s.OpenAPI)
	}

	var out []docRoute
	for path, item := range s.Paths {
		for key, node := range item {
			switch key {
			case "get", "post", "patch", "put", "delete":
				var op operation
				if err := node.Decode(&op); err != nil {
					t.Fatalf("%s %s: %v", key, path, err)
				}
				out = append(out, docRoute{strings.ToUpper(key), path, op})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path+out[i].method < out[j].path+out[j].method })
	return out
}

// The file route is registered as a prefix, which the document writes as a path parameter.
func registeredAsDocumented(pattern string) string {
	if pattern == "GET /api/v1/files/" {
		return "GET /api/v1/files/{path}"
	}
	return pattern
}

func TestOpenAPIMatchesTheRealRoutes(t *testing.T) {
	e := newCatalogEnv(t)

	documented := map[string]bool{}
	for _, r := range loadSpec(t) {
		documented[r.method+" "+r.path] = true
		if r.op.Summary == "" || len(r.op.Responses) == 0 {
			t.Errorf("%s %s needs a summary and responses", r.method, r.path)
		}
		switch r.op.Access {
		case "public", "customer", "staff", "admin", "rider", "metrics":
		default:
			t.Errorf("%s %s has x-access %q", r.method, r.path, r.op.Access)
		}
	}

	registered := map[string]bool{}
	for _, p := range e.srv.Routes() {
		if p == "/" {
			continue
		}
		registered[registeredAsDocumented(p)] = true
	}

	for r := range registered {
		if !documented[r] {
			t.Errorf("route %q is not in api/openapi.yaml", r)
		}
	}
	for d := range documented {
		if !registered[d] {
			t.Errorf("api/openapi.yaml documents %q, which does not exist", d)
		}
	}
}

func TestOpenAPIIsServed(t *testing.T) {
	e := newCatalogEnv(t)
	rec := e.do(t, "GET", "/api/v1/openapi.yaml", nil, nil)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/yaml") || !strings.HasPrefix(rec.Body.String(), "openapi: 3.") {
		t.Fatalf("%d %s", rec.Code, rec.Header())
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("the document should be cacheable")
	}
}

// TestEveryRouteEnforcesItsDocumentedAccess calls every documented route as each kind
// of user, so a new route that forgets its access check fails here.
func TestEveryRouteEnforcesItsDocumentedAccess(t *testing.T) {
	e := newCatalogEnv(t)
	cookies := map[string]*http.Cookie{
		"customer": e.signIn(t, "sweep-customer@example.com", users.RoleCustomer),
		"staff":    e.signIn(t, "sweep-staff@example.com", users.RoleStaff),
		"admin":    e.signIn(t, "sweep-admin@example.com", users.RoleAdmin),
		"rider":    e.signIn(t, "sweep-rider@example.com", users.RoleRider),
	}

	allowed := map[string][]string{
		"customer": {"customer", "staff", "admin", "rider"},
		"staff":    {"staff", "admin"},
		"admin":    {"admin"},
		"rider":    {"rider"},
	}

	call := func(r docRoute, who string) string {
		path := strings.NewReplacer("{id}", "00000000-0000-0000-0000-000000000000", "{imageId}", "00000000-0000-0000-0000-000000000001",
			"{reference}", "SATI-0-NONE", "{path}", "none.png").Replace(r.path)
		var body any
		if r.method == "POST" || r.method == "PATCH" || r.method == "PUT" {
			body = map[string]any{}
		}
		rec := e.do(t, r.method, path, body, cookies[who])
		var failure struct {
			Error struct{ Code string } `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &failure)
		if failure.Error.Code == "RATE_LIMITED" {
			t.Fatalf("%s %s as %s hit a rate limit during the sweep", r.method, r.path, who)
		}
		return failure.Error.Code
	}

	checked := 0
	for _, r := range loadSpec(t) {
		if r.op.Access == "metrics" {
			continue
		}
		checked++
		name := r.method + " " + r.path

		if r.op.Access == "public" {
			if code := call(r, ""); code == "UNAUTHORIZED" || code == "FORBIDDEN" {
				t.Errorf("%s is documented as public but demands a session", name)
			}
			continue
		}

		if code := call(r, ""); code != "UNAUTHORIZED" {
			t.Errorf("%s as anonymous: got %q, want UNAUTHORIZED", name, code)
		}
		for _, who := range []string{"customer", "staff", "admin", "rider"} {
			code := call(r, who)
			isAllowed := false
			for _, a := range allowed[r.op.Access] {
				isAllowed = isAllowed || a == who
			}
			switch {
			case isAllowed && (code == "UNAUTHORIZED" || code == "FORBIDDEN"):
				t.Errorf("%s as %s should be allowed (%s access) but got %s", name, who, r.op.Access, code)
			case !isAllowed && code != "FORBIDDEN":
				t.Errorf("%s as %s must be forbidden (%s access) but got %q", name, who, r.op.Access, code)
			}
		}
	}
	if checked < 50 {
		t.Fatalf("the sweep only covered %d routes", checked)
	}
}
