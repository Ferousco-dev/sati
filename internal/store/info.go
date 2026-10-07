package store

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Ferousco-dev/sati/backend/pkg/apierror"
)

func (in UpdateInput) hasInfoFields() bool {
	return in.StoreName != nil || in.StoreAddress != nil || in.StorePhone != nil || in.StoreEmail != nil ||
		in.InstagramURL != nil || in.TikTokURL != nil || in.FacebookURL != nil || in.MapEmbedURL != nil ||
		in.OpeningHours != nil || in.TemporarilyClosed != nil || in.ClosedMessage != nil
}

func hoursJSON(h *Hours) []byte {
	if h == nil {
		return nil
	}
	raw, _ := json.Marshal(h.Normalize())
	return raw
}

// onlyHosts accepts empty (to clear), or an https address on one of the allowed sites.
func onlyHosts(raw string, hosts ...string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range hosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// normalizeInfo checks the public store details. The map and social links are
// limited to the sites they belong to, because the map is shown in an iframe.
func (in *UpdateInput) normalizeInfo() *apierror.Error {
	for name, f := range map[string]*string{
		"store_name": in.StoreName, "store_address": in.StoreAddress, "store_phone": in.StorePhone,
		"closed_message": in.ClosedMessage,
	} {
		if f == nil {
			continue
		}
		*f = strings.TrimSpace(*f)
		limit := 200
		if name == "store_name" {
			limit = 100
		}
		if name == "store_phone" {
			limit = 30
		}
		if utf8.RuneCountInString(*f) > limit {
			return validation(name + " is too long")
		}
	}
	if in.StoreName != nil && *in.StoreName == "" {
		return validation("store_name cannot be empty")
	}

	if in.StoreEmail != nil {
		addr := strings.TrimSpace(*in.StoreEmail)
		if addr != "" {
			parsed, err := mail.ParseAddress(addr)
			if err != nil || parsed.Address != addr || len(addr) > 254 {
				return validation("store_email must be a valid email address")
			}
		}
		in.StoreEmail = &addr
	}

	links := []struct {
		name  string
		value *string
		hosts []string
	}{
		{"instagram_url", in.InstagramURL, []string{"instagram.com"}},
		{"tiktok_url", in.TikTokURL, []string{"tiktok.com"}},
		{"facebook_url", in.FacebookURL, []string{"facebook.com", "fb.com"}},
		{"map_embed_url", in.MapEmbedURL, []string{"google.com", "openstreetmap.org"}},
	}
	for _, l := range links {
		if l.value == nil {
			continue
		}
		*l.value = strings.TrimSpace(*l.value)
		if len(*l.value) > 2000 || !onlyHosts(*l.value, l.hosts...) {
			return validation(l.name + " must be an https link to " + strings.Join(l.hosts, " or "))
		}
	}

	if in.OpeningHours != nil {
		if err := in.OpeningHours.Validate(); err != nil {
			return validation("opening_hours: " + err.Error())
		}
	}
	return nil
}

type publicInfo struct {
	Name              string     `json:"name"`
	Address           string     `json:"address"`
	Phone             string     `json:"phone"`
	Email             string     `json:"email"`
	WhatsAppNumber    string     `json:"whatsapp_number"`
	WhatsAppURL       string     `json:"whatsapp_url"`
	InstagramURL      string     `json:"instagram_url"`
	TikTokURL         string     `json:"tiktok_url"`
	FacebookURL       string     `json:"facebook_url"`
	MapEmbedURL       string     `json:"map_embed_url"`
	OpeningHours      Hours      `json:"opening_hours"`
	Timezone          string     `json:"timezone"`
	OpenNow           bool       `json:"open_now"`
	NextChange        string     `json:"next_change,omitempty"`
	NextChangeAt      *time.Time `json:"next_change_at"`
	TemporarilyClosed bool       `json:"temporarily_closed"`
	ClosedMessage     string     `json:"closed_message"`
	Currency          string     `json:"currency"`
}

// Public builds what the website may show. Bank details, the notification
// address and the payment switches are never part of it.
func (s Settings) Public(now time.Time, loc *time.Location) publicInfo {
	status := s.OpeningHours.Status(now, loc, s.TemporarilyClosed)
	info := publicInfo{
		Name: s.StoreName, Address: s.StoreAddress, Phone: s.StorePhone, Email: s.StoreEmail,
		WhatsAppNumber: s.WhatsAppNumber, InstagramURL: s.InstagramURL, TikTokURL: s.TikTokURL,
		FacebookURL: s.FacebookURL, MapEmbedURL: s.MapEmbedURL, OpeningHours: s.OpeningHours.Normalize(),
		Timezone: loc.String(), OpenNow: status.Open, NextChange: status.NextKind, NextChangeAt: status.NextChange,
		TemporarilyClosed: s.TemporarilyClosed, ClosedMessage: s.ClosedMessage, Currency: "NGN",
	}
	if s.WhatsAppNumber != "" {
		info.WhatsAppURL = "https://wa.me/" + s.WhatsAppNumber
	}
	return info
}

func (h *Handler) public(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.Get(r.Context(), h.pool)
	if err != nil {
		h.respond(w, r, nil, err)
		return
	}
	h.respond(w, r, map[string]any{"store": s.Public(h.now(), h.loc)}, nil)
}
