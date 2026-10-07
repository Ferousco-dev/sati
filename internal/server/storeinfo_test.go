package server

import (
	"net/http"
	"strings"
	"testing"
)

type publicStore struct {
	Store struct {
		Name              string                       `json:"name"`
		Address           string                       `json:"address"`
		WhatsAppNumber    string                       `json:"whatsapp_number"`
		WhatsAppURL       string                       `json:"whatsapp_url"`
		MapEmbedURL       string                       `json:"map_embed_url"`
		InstagramURL      string                       `json:"instagram_url"`
		OpeningHours      map[string]map[string]string `json:"opening_hours"`
		Timezone          string                       `json:"timezone"`
		OpenNow           bool                         `json:"open_now"`
		NextChange        string                       `json:"next_change"`
		TemporarilyClosed bool                         `json:"temporarily_closed"`
		ClosedMessage     string                       `json:"closed_message"`
	}
}

func (e *staffEnv) publicStore(t *testing.T) (publicStore, *httptestRecorder) {
	t.Helper()
	rec := e.do(t, "GET", "/api/v1/store", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("store: %d %s", rec.Code, rec.Body)
	}
	return decode[publicStore](t, rec), rec
}

func allWeek(open, close string) map[string]any {
	day := map[string]any{"open": open, "close": close}
	return map[string]any{"mon": day, "tue": day, "wed": day, "thu": day, "fri": day, "sat": day, "sun": day}
}

func TestPublicStoreInfo(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{
		"store_name": "SATI-L Yoghurt", "store_address": "12 Main St, Lagos", "store_phone": "+234 801 234 5678",
		"store_email": "hello@sati.example", "instagram_url": "https://www.instagram.com/satil",
		"tiktok_url": "https://www.tiktok.com/@satil", "facebook_url": "https://facebook.com/satil",
		"map_embed_url":   "https://www.google.com/maps/embed?pb=abc",
		"whatsapp_number": "+234 801 234 5678", "closed_message": "Back tomorrow",
		"bank_name": "Secret Bank", "bank_account_number": "0123456789", "bank_account_name": "N",
		"notification_email": "private@sati.example",
	})

	info, rec := e.publicStore(t)
	if info.Store.Name != "SATI-L Yoghurt" || info.Store.Address != "12 Main St, Lagos" ||
		info.Store.MapEmbedURL != "https://www.google.com/maps/embed?pb=abc" || info.Store.InstagramURL != "https://www.instagram.com/satil" {
		t.Fatalf("store: %+v", info.Store)
	}
	if info.Store.WhatsAppNumber != "2348012345678" || info.Store.WhatsAppURL != "https://wa.me/2348012345678" || info.Store.Timezone != "Africa/Lagos" {
		t.Fatalf("whatsapp or timezone: %+v", info.Store)
	}

	body := rec.Body.String()
	for _, private := range []string{"Secret Bank", "0123456789", "private@sati.example", "delivery_fee", "online_enabled", "whatsapp_message"} {
		if strings.Contains(body, private) {
			t.Fatalf("the public endpoint leaked %q", private)
		}
	}
	if rec.Header().Get("ETag") == "" || !strings.Contains(rec.Header().Get("Cache-Control"), "public") {
		t.Fatalf("the public store info should be cacheable: %v", rec.Header())
	}
	if len(info.Store.OpeningHours) != 7 {
		t.Fatalf("all seven days are always returned: %v", info.Store.OpeningHours)
	}
}

func TestOpenNowFollowsTheScheduleAndTheSwitch(t *testing.T) {
	e := newStaffEnv(t)

	if info, _ := e.publicStore(t); info.Store.OpenNow {
		t.Fatal("a store with no hours is never open")
	}

	e.settings(t, map[string]any{"opening_hours": allWeek("00:00", "23:59")})
	if info, _ := e.publicStore(t); !info.Store.OpenNow || info.Store.NextChange != "closes" {
		t.Fatalf("open all day: %+v", info.Store)
	}

	e.settings(t, map[string]any{"temporarily_closed": true, "closed_message": "Closed for stocktake"})
	info, _ := e.publicStore(t)
	if info.Store.OpenNow || !info.Store.TemporarilyClosed || info.Store.ClosedMessage != "Closed for stocktake" || info.Store.NextChange != "opens" {
		t.Fatalf("manual closure: %+v", info.Store)
	}

	e.settings(t, map[string]any{"temporarily_closed": false, "opening_hours": map[string]any{"sun": nil}})
	if info, _ := e.publicStore(t); info.Store.OpenNow || info.Store.OpeningHours["mon"] != nil {
		t.Fatalf("replacing the schedule must clear the other days: %+v", info.Store)
	}
}

func TestStoreInfoValidation(t *testing.T) {
	e := newStaffEnv(t)
	bad := []map[string]any{
		{"map_embed_url": "http://www.google.com/maps/embed"},
		{"map_embed_url": "https://evil.example/maps/embed"},
		{"map_embed_url": "https://google.com.evil.example/embed"},
		{"map_embed_url": "javascript:alert(1)"},
		{"map_embed_url": "https://user:pw@www.google.com/maps/embed"},
		{"instagram_url": "https://facebook.com/satil"},
		{"tiktok_url": "not a url"},
		{"facebook_url": "http://facebook.com/x"},
		{"store_email": "nope"},
		{"store_name": ""},
		{"store_phone": strings.Repeat("1", 31)},
		{"opening_hours": map[string]any{"monday": map[string]any{"open": "09:00", "close": "17:00"}}},
		{"opening_hours": map[string]any{"mon": map[string]any{"open": "9am", "close": "17:00"}}},
		{"opening_hours": map[string]any{"mon": map[string]any{"open": "17:00", "close": "09:00"}}},
	}
	for _, body := range bad {
		if rec := e.do(t, "PATCH", "/api/v1/admin/settings", body, e.admin); rec.Code != http.StatusBadRequest {
			t.Errorf("%v: got %d", body, rec.Code)
		}
	}

	e.settings(t, map[string]any{"instagram_url": "https://instagram.com/satil", "map_embed_url": "https://www.openstreetmap.org/export/embed.html?bbox=1"})
	e.settings(t, map[string]any{"instagram_url": "", "map_embed_url": ""})
	if info, _ := e.publicStore(t); info.Store.InstagramURL != "" || info.Store.MapEmbedURL != "" {
		t.Fatal("links must be clearable")
	}
}

func TestAdminSettingsIncludeStoreInfo(t *testing.T) {
	e := newStaffEnv(t)
	e.settings(t, map[string]any{"store_address": "Somewhere", "opening_hours": allWeek("09:00", "17:00")})
	got := decode[struct {
		Settings struct {
			StoreAddress string                       `json:"store_address"`
			OpeningHours map[string]map[string]string `json:"opening_hours"`
			BankName     string                       `json:"bank_name"`
		}
	}](t, e.do(t, "GET", "/api/v1/admin/settings", nil, e.staff))
	if got.Settings.StoreAddress != "Somewhere" || got.Settings.OpeningHours["wed"]["close"] != "17:00" {
		t.Fatalf("%+v", got)
	}
}
