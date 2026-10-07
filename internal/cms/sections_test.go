package cms

import (
	"encoding/json"
	"strings"
	"testing"
)

const mediaA = "00000000-0000-0000-0000-00000000000a"

func TestEveryDefaultIsValid(t *testing.T) {
	for _, def := range sectionDefs {
		raw, _ := json.Marshal(def.Defaults())
		if _, _, err := Validate(def.Key, raw); err != nil {
			t.Errorf("the built-in default for %s fails its own rules: %v", def.Key, err)
		}
	}
}

func TestEverySectionHasAKeyAndTitle(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range sectionDefs {
		if d.Key == "" || d.Title == "" || seen[d.Key] {
			t.Fatalf("bad definition: %+v", d)
		}
		seen[d.Key] = true
	}
}

func TestValidateCleansAndCollectsMedia(t *testing.T) {
	raw := `{"tagline":"  Make you ask for more  ","title_lines":[" Parfait "],"cta_label":"Menu","cta_url":"/menu",
		"images":[{"media_id":"` + mediaA + `","alt":" a cake "},{"media_id":"` + mediaA + `"}]}`
	out, ids, err := Validate("hero", json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	var h Hero
	_ = json.Unmarshal(out, &h)
	if h.Tagline != "Make you ask for more" || h.TitleLines[0] != "Parfait" || h.Images[0].Alt != "a cake" {
		t.Fatalf("not trimmed: %+v", h)
	}
	if len(ids) != 1 || ids[0] != mediaA {
		t.Fatalf("media ids: %v", ids)
	}
}

func TestValidateRejects(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	cases := map[string]struct{ key, body string }{
		"unknown section":           {"nope", `{}`},
		"unknown field":             {"hero", `{"tagline":"a","title_lines":["b"],"evil":1}`},
		"wrong type":                {"hero", `{"tagline":5,"title_lines":["b"]}`},
		"trailing data":             {"hero", `{"tagline":"a","title_lines":["b"]} {"x":1}`},
		"not json":                  {"hero", `nope`},
		"no title lines":            {"hero", `{"tagline":"a","title_lines":[]}`},
		"too many title lines":      {"hero", `{"tagline":"a","title_lines":["1","2","3","4","5"]}`},
		"tagline too long":          {"hero", `{"tagline":"` + long(81) + `","title_lines":["b"]}`},
		"script link":               {"hero", `{"tagline":"a","title_lines":["b"],"cta_url":"javascript:alert(1)"}`},
		"protocol relative link":    {"hero", `{"tagline":"a","title_lines":["b"],"cta_url":"//evil.example"}`},
		"http link":                 {"hero", `{"tagline":"a","title_lines":["b"],"cta_url":"http://x.example"}`},
		"data link":                 {"hero", `{"tagline":"a","title_lines":["b"],"cta_url":"data:text/html,x"}`},
		"bad media id":              {"hero", `{"tagline":"a","title_lines":["b"],"images":[{"media_id":"abc"}]}`},
		"empty image":               {"hero", `{"tagline":"a","title_lines":["b"],"images":[{}]}`},
		"too many images":           {"hero", `{"tagline":"a","title_lines":["b"],"images":[` + strings.Repeat(`{"media_id":"`+mediaA+`"},`, 8) + `{"media_id":"` + mediaA + `"}]}`},
		"one step":                  {"how_it_works", `{"heading":"h","steps":[{"title":"a","text":"b"}]}`},
		"empty heading":             {"how_it_works", `{"heading":"","steps":[{"title":"a"},{"title":"b"}]}`},
		"rating zero":               {"testimonials", `{"heading":"h","items":[{"name":"n","quote":"q","rating":0}]}`},
		"rating six":                {"testimonials", `{"heading":"h","items":[{"name":"n","quote":"q","rating":6}]}`},
		"empty quote":               {"testimonials", `{"heading":"h","items":[{"name":"n","quote":" ","rating":5}]}`},
		"faq answer too long":       {"faq", `{"heading":"h","items":[{"question":"q","answer":"` + long(801) + `"}]}`},
		"instagram wrong site":      {"gallery", `{"heading":"h","images":[],"instagram_url":"https://facebook.com/x"}`},
		"instagram lookalike":       {"gallery", `{"heading":"h","images":[],"instagram_url":"https://instagram.com.evil.example/x"}`},
		"footer link without url":   {"footer", `{"tagline":"t","columns":[{"title":"c","links":[{"label":"l","url":""}]}]}`},
		"too many footer columns":   {"footer", `{"tagline":"t","columns":[{"title":"1"},{"title":"2"},{"title":"3"},{"title":"4"},{"title":"5"}]}`},
		"announcement without text": {"announcement", `{"enabled":true,"text":""}`},
		"seo title too long":        {"seo", `{"title":"` + long(71) + `"}`},
		"layout missing sections":   {"layout", `{"sections":[{"key":"hero","visible":true}]}`},
		"layout unknown key":        {"layout", `{"sections":[{"key":"evil","visible":true},{"key":"products","visible":true},{"key":"how_it_works","visible":true},{"key":"testimonials","visible":true},{"key":"gallery","visible":true},{"key":"catering","visible":true},{"key":"faq","visible":true},{"key":"hours","visible":true}]}`},
		"layout repeat":             {"layout", `{"sections":[{"key":"hero","visible":true},{"key":"hero","visible":true},{"key":"how_it_works","visible":true},{"key":"testimonials","visible":true},{"key":"gallery","visible":true},{"key":"catering","visible":true},{"key":"faq","visible":true},{"key":"hours","visible":true}]}`},
	}
	for name, c := range cases {
		if _, _, err := Validate(c.key, json.RawMessage(c.body)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestSafeLinksAreAccepted(t *testing.T) {
	for _, u := range []string{"/menu", "#catering", "https://example.com/x", "mailto:hi@example.com", "tel:+2348012345678", ""} {
		body := `{"tagline":"a","title_lines":["b"],"cta_url":"` + u + `"}`
		if _, _, err := Validate("hero", json.RawMessage(body)); err != nil {
			t.Errorf("%q: %v", u, err)
		}
	}
}

func TestLayoutAcceptsAReorderedList(t *testing.T) {
	order := []string{"faq", "hero", "products", "how_it_works", "testimonials", "gallery", "catering", "hours"}
	var items []string
	for i, k := range order {
		items = append(items, `{"key":"`+k+`","visible":`+map[bool]string{true: "true", false: "false"}[i%2 == 0]+`}`)
	}
	if _, _, err := Validate("layout", json.RawMessage(`{"sections":[`+strings.Join(items, ",")+`]}`)); err != nil {
		t.Fatal(err)
	}
}
