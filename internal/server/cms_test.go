package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Ferousco-dev/sati/backend/internal/cms"
)

type siteContent struct {
	Sections  map[string]json.RawMessage `json:"sections"`
	UpdatedAt *string                    `json:"updated_at"`
}

func (e *staffEnv) site(t *testing.T) (siteContent, *httptestRecorder) {
	t.Helper()
	rec := e.do(t, "GET", "/api/v1/content", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("content: %d %s", rec.Code, rec.Body)
	}
	return decode[siteContent](t, rec), rec
}

func (e *staffEnv) draft(t *testing.T, key string, data any) *httptestRecorder {
	t.Helper()
	return e.do(t, "PUT", "/api/v1/admin/content/"+key+"/draft", map[string]any{"data": data}, e.admin)
}

func (e *staffEnv) publishSection(t *testing.T, key, note string) *httptestRecorder {
	t.Helper()
	return e.do(t, "POST", "/api/v1/admin/content/"+key+"/publish", map[string]any{"note": note}, e.admin)
}

type sectionResp struct {
	Section struct {
		Key                  string          `json:"key"`
		PublishedVersion     int             `json:"published_version"`
		Published            json.RawMessage `json:"published"`
		Draft                json.RawMessage `json:"draft"`
		HasUnpublishedChange bool            `json:"has_unpublished_changes"`
	}
}

func (e *staffEnv) section(t *testing.T, key string) sectionResp {
	t.Helper()
	rec := e.do(t, "GET", "/api/v1/admin/content/"+key, nil, e.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("section %s: %d %s", key, rec.Code, rec.Body)
	}
	return decode[sectionResp](t, rec)
}

func (e *staffEnv) uploadPicture(t *testing.T, name, alt string, w, h int) cms.Media {
	t.Helper()
	var pngData bytes.Buffer
	_ = png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, w, h)))

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", name)
	part.Write(pngData.Bytes())
	if alt != "" {
		mw.WriteField("alt", alt)
	}
	mw.Close()

	req := httptest.NewRequest("POST", "/api/v1/admin/media", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(e.admin)
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	return decode[struct{ Media cms.Media }](t, rec).Media
}

func heroWith(images ...string) map[string]any {
	var imgs []map[string]any
	for _, id := range images {
		imgs = append(imgs, map[string]any{"media_id": id})
	}
	return map[string]any{
		"tagline": "New tagline", "title_lines": []string{"Fresh", "Every day"},
		"cta_label": "Order", "cta_url": "/menu", "images": imgs,
	}
}

func TestPublicContentStartsWithDefaults(t *testing.T) {
	e := newStaffEnv(t)
	site, rec := e.site(t)

	for _, key := range []string{"hero", "how_it_works", "testimonials", "gallery", "faq", "catering", "footer", "announcement", "seo", "layout"} {
		if len(site.Sections[key]) == 0 {
			t.Errorf("section %s is missing", key)
		}
	}
	if !strings.Contains(string(site.Sections["hero"]), "Make you ask for more.") {
		t.Fatalf("hero default: %s", site.Sections["hero"])
	}
	if rec.Header().Get("ETag") == "" || !strings.Contains(rec.Header().Get("Cache-Control"), "public") {
		t.Fatalf("the public content must be cacheable: %v", rec.Header())
	}
}

func TestDraftIsInvisibleUntilPublished(t *testing.T) {
	e := newStaffEnv(t)

	rec := e.draft(t, "hero", heroWith())
	if rec.Code != http.StatusOK {
		t.Fatalf("save draft: %d %s", rec.Code, rec.Body)
	}
	if site, _ := e.site(t); strings.Contains(string(site.Sections["hero"]), "New tagline") {
		t.Fatal("a draft leaked to visitors")
	}

	s := e.section(t, "hero")
	if !s.Section.HasUnpublishedChange || s.Section.PublishedVersion != 0 || !strings.Contains(string(s.Section.Draft), "New tagline") {
		t.Fatalf("section state: %+v", s.Section)
	}

	preview := decode[siteContent](t, e.do(t, "GET", "/api/v1/admin/content/preview", nil, e.admin))
	if !strings.Contains(string(preview.Sections["hero"]), "New tagline") {
		t.Fatal("the admin preview must show the draft")
	}

	if rec := e.publishSection(t, "hero", "First edit"); rec.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	if site, _ := e.site(t); !strings.Contains(string(site.Sections["hero"]), "New tagline") {
		t.Fatal("published content must be public")
	}
	after := e.section(t, "hero")
	if after.Section.PublishedVersion != 1 || after.Section.HasUnpublishedChange || string(after.Section.Draft) != "null" {
		t.Fatalf("after publishing: %+v", after.Section)
	}
	if rec := e.publishSection(t, "hero", ""); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "NO_DRAFT" {
		t.Fatalf("publishing nothing: %d %s", rec.Code, rec.Body)
	}
}

func TestDiscardDraft(t *testing.T) {
	e := newStaffEnv(t)
	e.draft(t, "hero", heroWith())
	if rec := e.do(t, "DELETE", "/api/v1/admin/content/hero/draft", nil, e.admin); rec.Code != http.StatusNoContent {
		t.Fatalf("discard: %d", rec.Code)
	}
	s := e.section(t, "hero")
	if string(s.Section.Draft) != "null" || s.Section.HasUnpublishedChange {
		t.Fatalf("%+v", s.Section)
	}
	if rec := e.do(t, "DELETE", "/api/v1/admin/content/nope/draft", nil, e.admin); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown section: %d", rec.Code)
	}
}

func TestDraftValidationReturnsClearMessages(t *testing.T) {
	e := newStaffEnv(t)
	bad := []any{
		map[string]any{"tagline": "x", "title_lines": []string{}},
		map[string]any{"tagline": "x", "title_lines": []string{"a"}, "cta_url": "javascript:alert(1)"},
		map[string]any{"tagline": "x", "title_lines": []string{"a"}, "surprise": true},
		heroWith("00000000-0000-0000-0000-0000000000ff"),
	}
	for _, body := range bad {
		rec := e.draft(t, "hero", body)
		if rec.Code != http.StatusBadRequest || errorCodeOf(t, rec) != "VALIDATION_ERROR" {
			t.Errorf("%v: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := e.draft(t, "nope", map[string]any{}); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown section: %d", rec.Code)
	}
	if rec := e.do(t, "PUT", "/api/v1/admin/content/hero/draft", map[string]any{}, e.admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing data: %d", rec.Code)
	}
}

func TestVersionsAndRollback(t *testing.T) {
	e := newStaffEnv(t)
	first := heroWith()
	first["tagline"] = "First"
	second := heroWith()
	second["tagline"] = "Second"

	e.draft(t, "hero", first)
	e.publishSection(t, "hero", "launch")
	e.draft(t, "hero", second)
	e.publishSection(t, "hero", "")

	versions := decode[struct {
		Versions []cms.Version
	}](t, e.do(t, "GET", "/api/v1/admin/content/hero/versions", nil, e.admin)).Versions
	if len(versions) != 2 || versions[0].Version != 2 || !versions[0].Live || versions[1].Live || versions[1].Note != "launch" || versions[1].PublishedBy == "" {
		t.Fatalf("versions: %+v", versions)
	}

	rec := e.do(t, "POST", "/api/v1/admin/content/hero/rollback", map[string]any{"version": 1}, e.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("rollback: %d %s", rec.Code, rec.Body)
	}
	if site, _ := e.site(t); !strings.Contains(string(site.Sections["hero"]), "First") {
		t.Fatal("the rolled back content must be live")
	}
	versions = decode[struct{ Versions []cms.Version }](t, e.do(t, "GET", "/api/v1/admin/content/hero/versions", nil, e.admin)).Versions
	if len(versions) != 3 || versions[0].Version != 3 || !versions[0].Live || !strings.Contains(versions[0].Note, "Rolled back to version 1") {
		t.Fatalf("history is never rewritten: %+v", versions)
	}

	if rec := e.do(t, "POST", "/api/v1/admin/content/hero/rollback", map[string]any{"version": 99}, e.admin); rec.Code != http.StatusNotFound || errorCodeOf(t, rec) != "VERSION_NOT_FOUND" {
		t.Fatalf("unknown version: %d", rec.Code)
	}
}

func TestPicturesAreResolvedAndProtected(t *testing.T) {
	e := newStaffEnv(t)
	pic := e.uploadPicture(t, "../../evil/cake photo.png", "A chocolate cake", 40, 30)
	if pic.Width != 40 || pic.Height != 30 || pic.Filename != "cake photo.png" || !strings.HasPrefix(pic.URL, "/api/v1/files/media/") || pic.AltText != "A chocolate cake" {
		t.Fatalf("upload: %+v", pic)
	}
	if served := e.do(t, "GET", pic.URL, nil, nil); served.Code != http.StatusOK {
		t.Fatalf("the uploaded picture must be served: %d", served.Code)
	}

	e.draft(t, "hero", heroWith(pic.ID))
	if rec := e.do(t, "DELETE", "/api/v1/admin/media/"+pic.ID, nil, e.admin); rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "MEDIA_IN_USE" {
		t.Fatalf("a picture in a draft must not be deleted: %d", rec.Code)
	}

	e.publishSection(t, "hero", "")
	site, _ := e.site(t)
	var hero struct {
		Images []struct {
			MediaID string `json:"media_id"`
			URL     string `json:"url"`
			Width   int    `json:"width"`
			Alt     string `json:"alt"`
		} `json:"images"`
	}
	_ = json.Unmarshal(site.Sections["hero"], &hero)
	if len(hero.Images) != 1 || hero.Images[0].URL != pic.URL || hero.Images[0].Width != 40 || hero.Images[0].Alt != "A chocolate cake" {
		t.Fatalf("the public site must get ready-to-use picture urls: %+v", hero)
	}
	if rec := e.do(t, "DELETE", "/api/v1/admin/media/"+pic.ID, nil, e.admin); rec.Code != http.StatusConflict {
		t.Fatalf("a live picture must not be deleted: %d", rec.Code)
	}

	e.draft(t, "hero", heroWith())
	e.publishSection(t, "hero", "pictures removed")
	if rec := e.do(t, "DELETE", "/api/v1/admin/media/"+pic.ID, nil, e.admin); rec.Code != http.StatusNoContent {
		t.Fatalf("an unused picture can be deleted: %d %s", rec.Code, rec.Body)
	}
	if gone := e.do(t, "GET", pic.URL, nil, nil); gone.Code != http.StatusNotFound {
		t.Fatalf("the file must be removed: %d", gone.Code)
	}

	rec := e.do(t, "POST", "/api/v1/admin/content/hero/rollback", map[string]any{"version": 1}, e.admin)
	if rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "VERSION_PICTURE_MISSING" {
		t.Fatalf("rolling back to a deleted picture: %d %s", rec.Code, rec.Body)
	}
}

func TestMediaLibrary(t *testing.T) {
	e := newStaffEnv(t)
	a := e.uploadPicture(t, "parfait.png", "Strawberry parfait", 10, 10)
	e.uploadPicture(t, "cake.png", "", 10, 10)

	list := decode[struct {
		Media []cms.Media
		Total int
	}](t, e.do(t, "GET", "/api/v1/admin/media", nil, e.admin))
	if list.Total != 2 || len(list.Media) != 2 || list.Media[0].URL == "" {
		t.Fatalf("list: %+v", list)
	}
	if found := decode[struct{ Total int }](t, e.do(t, "GET", "/api/v1/admin/media?q=parfait", nil, e.admin)); found.Total != 1 {
		t.Fatalf("search by name or alt: %d", found.Total)
	}
	if found := decode[struct{ Total int }](t, e.do(t, "GET", "/api/v1/admin/media?q=%25", nil, e.admin)); found.Total != 0 {
		t.Fatalf("wildcards must be escaped: %d", found.Total)
	}

	rec := e.do(t, "PATCH", "/api/v1/admin/media/"+a.ID, map[string]any{"alt_text": "A new description"}, e.admin)
	if rec.Code != http.StatusOK || decode[struct{ Media cms.Media }](t, rec).Media.AltText != "A new description" {
		t.Fatalf("alt text: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(t, "PATCH", "/api/v1/admin/media/"+a.ID, map[string]any{"alt_text": strings.Repeat("x", 201)}, e.admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("long alt text: %d", rec.Code)
	}
	if rec := e.do(t, "DELETE", "/api/v1/admin/media/00000000-0000-0000-0000-000000000000", nil, e.admin); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown picture: %d", rec.Code)
	}
}

func TestUploadRejectsBadFiles(t *testing.T) {
	e := newStaffEnv(t)
	post := func(content []byte) *httptestRecorder {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, _ := mw.CreateFormFile("file", "x.png")
		part.Write(content)
		mw.Close()
		req := httptest.NewRequest("POST", "/api/v1/admin/media", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.AddCookie(e.admin)
		rec := httptest.NewRecorder()
		e.srv.ServeHTTP(rec, req)
		return rec
	}
	if rec := post([]byte("<html><script>alert(1)</script></html>")); rec.Code != http.StatusBadRequest || errorCodeOf(t, rec) != "INVALID_IMAGE" {
		t.Fatalf("not a picture: %d %s", rec.Code, rec.Body)
	}
	if rec := post(make([]byte, cms.MaxUploadBytes+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: %d", rec.Code)
	}

	req := httptest.NewRequest("POST", "/api/v1/admin/media", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(e.admin)
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no file: %d", rec.Code)
	}
}

func TestOnlyAdminsEditContent(t *testing.T) {
	e := newStaffEnv(t)
	hero := heroWith()
	for _, who := range []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{{"staff", e.staff, http.StatusForbidden}, {"customer", e.customer, http.StatusForbidden}, {"anonymous", nil, http.StatusUnauthorized}} {
		if rec := e.do(t, "PUT", "/api/v1/admin/content/hero/draft", map[string]any{"data": hero}, who.cookie); rec.Code != who.want {
			t.Errorf("%s saving a draft: %d", who.name, rec.Code)
		}
		if rec := e.do(t, "GET", "/api/v1/admin/content/preview", nil, who.cookie); rec.Code != who.want {
			t.Errorf("%s previewing: %d", who.name, rec.Code)
		}
	}
}

func TestLayoutReordersAndHides(t *testing.T) {
	e := newStaffEnv(t)
	layout := map[string]any{"sections": []map[string]any{
		{"key": "faq", "visible": true}, {"key": "hero", "visible": true}, {"key": "products", "visible": true},
		{"key": "how_it_works", "visible": false}, {"key": "testimonials", "visible": true}, {"key": "gallery", "visible": false},
		{"key": "catering", "visible": true}, {"key": "hours", "visible": true},
	}}
	if rec := e.draft(t, "layout", layout); rec.Code != http.StatusOK {
		t.Fatalf("layout: %d %s", rec.Code, rec.Body)
	}
	e.publishSection(t, "layout", "faq first")

	site, _ := e.site(t)
	var got struct {
		Sections []struct {
			Key     string `json:"key"`
			Visible bool   `json:"visible"`
		} `json:"sections"`
	}
	_ = json.Unmarshal(site.Sections["layout"], &got)
	if got.Sections[0].Key != "faq" || got.Sections[3].Visible {
		t.Fatalf("layout: %+v", got)
	}
}

func TestPublishingTheSameDraftTwiceAtOnceCreatesOneVersion(t *testing.T) {
	e := newStaffEnv(t)
	e.draft(t, "hero", heroWith())

	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = e.publishSection(t, "hero", "").Code
		}(i)
	}
	wg.Wait()

	ok := 0
	for _, c := range codes {
		if c == http.StatusOK {
			ok++
		}
	}
	versions := decode[struct{ Versions []cms.Version }](t, e.do(t, "GET", "/api/v1/admin/content/hero/versions", nil, e.admin)).Versions
	if ok != 1 || len(versions) != 1 {
		t.Fatalf("one publish must win: %v, versions %d", codes, len(versions))
	}
}

func TestContentChangesAreAudited(t *testing.T) {
	e := newStaffEnv(t)
	e.draft(t, "hero", heroWith())
	e.publishSection(t, "hero", "")

	logs := e.auditLogs(t, e.admin, "?entity_type=content")
	actions := map[string]bool{}
	for _, r := range logs.AuditLogs {
		actions[r.Action] = true
	}
	if !actions["PUT /api/v1/admin/content/{key}/draft"] || !actions["POST /api/v1/admin/content/{key}/publish"] {
		t.Fatalf("content edits must appear in the audit log: %v", actions)
	}
}
