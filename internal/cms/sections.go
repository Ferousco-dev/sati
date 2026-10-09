package cms

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Ferousco-dev/sati/backend/pkg/httpx"
)

// Image points at a file in the media library. The API adds url, width and height when it serves a section.
type Image struct {
	MediaID string `json:"media_id"`
	Alt     string `json:"alt,omitempty"`
}

type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type check struct {
	err      string
	mediaIDs []string
}

func (c *check) fail(format string, args ...any) {
	if c.err == "" {
		c.err = fmt.Sprintf(format, args...)
	}
}

func (c *check) text(field string, s *string, min, max int) {
	*s = strings.TrimSpace(*s)
	if n := utf8.RuneCountInString(*s); n < min || n > max {
		if min == 0 {
			c.fail("%s must be at most %d characters", field, max)
		} else {
			c.fail("%s must be between %d and %d characters", field, min, max)
		}
	}
}

var safeLink = regexp.MustCompile(`^(/|#|https://|mailto:|tel:)`)

// link accepts a path on this site, an anchor, https, mail or phone. Scripts and other schemes are refused.
func (c *check) link(field string, s *string, required bool) {
	*s = strings.TrimSpace(*s)
	if *s == "" {
		if required {
			c.fail("%s is required", field)
		}
		return
	}
	if len(*s) > 500 || !safeLink.MatchString(*s) || strings.HasPrefix(*s, "//") {
		c.fail("%s must be a path, an anchor or an https, mailto or tel link", field)
	}
}

func (c *check) image(field string, img *Image, required bool) {
	img.MediaID, img.Alt = strings.TrimSpace(img.MediaID), strings.TrimSpace(img.Alt)
	if img.MediaID == "" {
		if required {
			c.fail("%s needs an image", field)
		}
		return
	}
	if !httpx.ValidUUID(img.MediaID) {
		c.fail("%s has an invalid media_id", field)
		return
	}
	if utf8.RuneCountInString(img.Alt) > 200 {
		c.fail("%s alt text is too long", field)
	}
	c.mediaIDs = append(c.mediaIDs, img.MediaID)
}

func (c *check) count(field string, n, min, max int) {
	if n < min || n > max {
		c.fail("%s needs between %d and %d entries", field, min, max)
	}
}

type section interface{ normalize(c *check) }

type Hero struct {
	Tagline     string   `json:"tagline"`
	TitleLines  []string `json:"title_lines"`
	CTALabel    string   `json:"cta_label"`
	CTAURL      string   `json:"cta_url"`
	Images      []Image  `json:"images"`
	MobileImage *Image   `json:"mobile_image,omitempty"`
}

func (s *Hero) normalize(c *check) {
	if s.Images == nil {
		s.Images = []Image{}
	}
	c.text("tagline", &s.Tagline, 0, 80)
	c.count("title_lines", len(s.TitleLines), 1, 4)
	for i := range s.TitleLines {
		c.text(fmt.Sprintf("title_lines[%d]", i), &s.TitleLines[i], 1, 60)
	}
	c.text("cta_label", &s.CTALabel, 0, 30)
	c.link("cta_url", &s.CTAURL, false)
	c.count("images", len(s.Images), 0, 8)
	for i := range s.Images {
		c.image(fmt.Sprintf("images[%d]", i), &s.Images[i], true)
	}
	if s.MobileImage != nil {
		c.image("mobile_image", s.MobileImage, true)
	}
}

type Step struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type HowItWorks struct {
	Heading string `json:"heading"`
	Steps   []Step `json:"steps"`
}

func (s *HowItWorks) normalize(c *check) {
	c.text("heading", &s.Heading, 1, 80)
	c.count("steps", len(s.Steps), 2, 6)
	for i := range s.Steps {
		c.text(fmt.Sprintf("steps[%d].title", i), &s.Steps[i].Title, 1, 40)
		c.text(fmt.Sprintf("steps[%d].text", i), &s.Steps[i].Text, 0, 200)
	}
}

type Testimonial struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Quote  string `json:"quote"`
	Rating int    `json:"rating"`
	Avatar *Image `json:"avatar,omitempty"`
}

type Testimonials struct {
	Heading string        `json:"heading"`
	Items   []Testimonial `json:"items"`
}

func (s *Testimonials) normalize(c *check) {
	if s.Items == nil {
		s.Items = []Testimonial{}
	}
	c.text("heading", &s.Heading, 1, 80)
	c.count("items", len(s.Items), 0, 12)
	for i := range s.Items {
		t := &s.Items[i]
		c.text(fmt.Sprintf("items[%d].name", i), &t.Name, 1, 60)
		c.text(fmt.Sprintf("items[%d].role", i), &t.Role, 0, 60)
		c.text(fmt.Sprintf("items[%d].quote", i), &t.Quote, 1, 400)
		if t.Rating < 1 || t.Rating > 5 {
			c.fail("items[%d].rating must be between 1 and 5", i)
		}
		if t.Avatar != nil {
			c.image(fmt.Sprintf("items[%d].avatar", i), t.Avatar, true)
		}
	}
}

type GalleryItem struct {
	Image   Image  `json:"image"`
	Caption string `json:"caption"`
}

type Gallery struct {
	Heading      string        `json:"heading"`
	Images       []GalleryItem `json:"images"`
	InstagramURL string        `json:"instagram_url"`
}

func (s *Gallery) normalize(c *check) {
	if s.Images == nil {
		s.Images = []GalleryItem{}
	}
	c.text("heading", &s.Heading, 1, 80)
	c.count("images", len(s.Images), 0, 12)
	for i := range s.Images {
		c.image(fmt.Sprintf("images[%d].image", i), &s.Images[i].Image, true)
		c.text(fmt.Sprintf("images[%d].caption", i), &s.Images[i].Caption, 0, 80)
	}
	s.InstagramURL = strings.TrimSpace(s.InstagramURL)
	if s.InstagramURL != "" {
		u, err := url.Parse(s.InstagramURL)
		if err != nil || u.Scheme != "https" || !(u.Hostname() == "instagram.com" || strings.HasSuffix(u.Hostname(), ".instagram.com")) {
			c.fail("instagram_url must be an https link on instagram.com")
		}
	}
}

type FAQItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type FAQ struct {
	Heading string    `json:"heading"`
	Items   []FAQItem `json:"items"`
}

func (s *FAQ) normalize(c *check) {
	if s.Items == nil {
		s.Items = []FAQItem{}
	}
	c.text("heading", &s.Heading, 1, 80)
	c.count("items", len(s.Items), 0, 20)
	for i := range s.Items {
		c.text(fmt.Sprintf("items[%d].question", i), &s.Items[i].Question, 1, 150)
		c.text(fmt.Sprintf("items[%d].answer", i), &s.Items[i].Answer, 1, 800)
	}
}

type Catering struct {
	Heading     string `json:"heading"`
	Text        string `json:"text"`
	BannerImage *Image `json:"banner_image,omitempty"`
	BannerNote  string `json:"banner_note"`
	CTALabel    string `json:"cta_label"`
	CTAURL      string `json:"cta_url"`
}

func (s *Catering) normalize(c *check) {
	c.text("heading", &s.Heading, 1, 100)
	c.text("text", &s.Text, 0, 400)
	if s.BannerImage != nil {
		c.image("banner_image", s.BannerImage, true)
	}
	c.text("banner_note", &s.BannerNote, 0, 80)
	c.text("cta_label", &s.CTALabel, 0, 30)
	c.link("cta_url", &s.CTAURL, false)
}

type FooterColumn struct {
	Title string `json:"title"`
	Links []Link `json:"links"`
}

type Footer struct {
	Tagline   string         `json:"tagline"`
	CTALabel  string         `json:"cta_label"`
	CTAURL    string         `json:"cta_url"`
	Columns   []FooterColumn `json:"columns"`
	LegalText string         `json:"legal_text"`
}

func (s *Footer) normalize(c *check) {
	if s.Columns == nil {
		s.Columns = []FooterColumn{}
	}
	for i := range s.Columns {
		if s.Columns[i].Links == nil {
			s.Columns[i].Links = []Link{}
		}
	}
	c.text("tagline", &s.Tagline, 0, 80)
	c.text("cta_label", &s.CTALabel, 0, 30)
	c.link("cta_url", &s.CTAURL, false)
	c.count("columns", len(s.Columns), 0, 4)
	for i := range s.Columns {
		col := &s.Columns[i]
		c.text(fmt.Sprintf("columns[%d].title", i), &col.Title, 1, 40)
		c.count(fmt.Sprintf("columns[%d].links", i), len(col.Links), 0, 8)
		for j := range col.Links {
			c.text(fmt.Sprintf("columns[%d].links[%d].label", i, j), &col.Links[j].Label, 1, 40)
			c.link(fmt.Sprintf("columns[%d].links[%d].url", i, j), &col.Links[j].URL, true)
		}
	}
	c.text("legal_text", &s.LegalText, 0, 200)
}

type Navigation struct {
	Logo       *Image `json:"logo,omitempty"`
	LogoAlt    string `json:"logo_alt"`
	Links      []Link `json:"links"`
	OrderLabel string `json:"order_label"`
	CartLabel  string `json:"cart_label"`
}

func (s *Navigation) normalize(c *check) {
	if s.Logo != nil {
		c.image("logo", s.Logo, true)
	}
	c.text("logo_alt", &s.LogoAlt, 0, 80)
	if s.Links == nil {
		s.Links = []Link{}
	}
	c.count("links", len(s.Links), 0, 6)
	for i := range s.Links {
		c.text(fmt.Sprintf("links[%d].label", i), &s.Links[i].Label, 1, 30)
		c.link(fmt.Sprintf("links[%d].url", i), &s.Links[i].URL, true)
	}
	c.text("order_label", &s.OrderLabel, 1, 20)
	c.text("cart_label", &s.CartLabel, 1, 20)
}

type ShowcaseItem struct {
	Name      string `json:"name"`
	Image     Image  `json:"image"`
	ProductID string `json:"product_id"`
}

type Showcase struct {
	Items       []ShowcaseItem `json:"items"`
	OrderLabel  string         `json:"order_label"`
	MarqueeText string         `json:"marquee_text"`
	MarqueeURL  string         `json:"marquee_url"`
}

func (s *Showcase) normalize(c *check) {
	if s.Items == nil {
		s.Items = []ShowcaseItem{}
	}
	c.count("items", len(s.Items), 0, 8)
	for i := range s.Items {
		c.text(fmt.Sprintf("items[%d].name", i), &s.Items[i].Name, 1, 40)
		c.image(fmt.Sprintf("items[%d].image", i), &s.Items[i].Image, true)
		c.text(fmt.Sprintf("items[%d].product_id", i), &s.Items[i].ProductID, 0, 64)
	}
	c.text("order_label", &s.OrderLabel, 1, 20)
	c.text("marquee_text", &s.MarqueeText, 1, 30)
	c.link("marquee_url", &s.MarqueeURL, false)
}

type MenuSection struct {
	ScriptLines []string `json:"script_lines"`
	IntroLines  []string `json:"intro_lines"`
	IntroButton string   `json:"intro_button"`
	AllLabel    string   `json:"all_label"`
	AddLabel    string   `json:"add_label"`
	SlideImages []Image  `json:"slide_images"`
}

func (s *MenuSection) normalize(c *check) {
	if s.SlideImages == nil {
		s.SlideImages = []Image{}
	}
	c.count("script_lines", len(s.ScriptLines), 0, 3)
	for i := range s.ScriptLines {
		c.text(fmt.Sprintf("script_lines[%d]", i), &s.ScriptLines[i], 1, 40)
	}
	c.count("intro_lines", len(s.IntroLines), 0, 3)
	for i := range s.IntroLines {
		c.text(fmt.Sprintf("intro_lines[%d]", i), &s.IntroLines[i], 1, 50)
	}
	c.text("intro_button", &s.IntroButton, 1, 30)
	c.text("all_label", &s.AllLabel, 1, 20)
	c.text("add_label", &s.AddLabel, 1, 20)
	c.count("slide_images", len(s.SlideImages), 0, 10)
	for i := range s.SlideImages {
		c.image(fmt.Sprintf("slide_images[%d]", i), &s.SlideImages[i], true)
	}
}

type Announcement struct {
	Enabled   bool   `json:"enabled"`
	Text      string `json:"text"`
	LinkLabel string `json:"link_label"`
	LinkURL   string `json:"link_url"`
}

func (s *Announcement) normalize(c *check) {
	c.text("text", &s.Text, 0, 160)
	if s.Enabled && s.Text == "" {
		c.fail("text is required when the announcement is switched on")
	}
	c.text("link_label", &s.LinkLabel, 0, 30)
	c.link("link_url", &s.LinkURL, false)
}

type SEO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ShareImage  *Image `json:"share_image,omitempty"`
}

func (s *SEO) normalize(c *check) {
	c.text("title", &s.Title, 1, 70)
	c.text("description", &s.Description, 0, 170)
	if s.ShareImage != nil {
		c.image("share_image", s.ShareImage, true)
	}
}

// layoutKeys are the blocks of the landing page that can be reordered or hidden.
var layoutKeys = []string{"hero", "products", "how_it_works", "testimonials", "gallery", "catering", "faq", "hours"}

type LayoutItem struct {
	Key     string `json:"key"`
	Visible bool   `json:"visible"`
}

type Layout struct {
	Sections []LayoutItem `json:"sections"`
}

func (s *Layout) normalize(c *check) {
	allowed := map[string]bool{}
	for _, k := range layoutKeys {
		allowed[k] = true
	}
	seen := map[string]bool{}
	for i, item := range s.Sections {
		switch {
		case !allowed[item.Key]:
			c.fail("sections[%d] has an unknown key %q", i, item.Key)
		case seen[item.Key]:
			c.fail("sections[%d] repeats %q", i, item.Key)
		}
		seen[item.Key] = true
	}
	if len(s.Sections) != len(layoutKeys) {
		c.fail("layout must list all %d sections once each", len(layoutKeys))
	}
}

type sectionDef struct {
	Key      string
	Title    string
	New      func() section
	Defaults func() section
}

func decode[T section](raw []byte, v T) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the content")
	}
	return nil
}

var sectionDefs = []sectionDef{
	{"hero", "Hero", func() section { return &Hero{} }, func() section {
		return &Hero{
			Tagline: "Make you ask for more.", TitleLines: []string{"Parfait, Cakes,", "Small chops, yoghurt & more."},
			CTALabel: "View menu", CTAURL: "/menu", Images: []Image{},
		}
	}},
	{"how_it_works", "How it works", func() section { return &HowItWorks{} }, func() section {
		return &HowItWorks{Heading: "How it works", Steps: []Step{
			{"Choose", "Pick your favourites from the menu."},
			{"Pay", "Pay online, by bank transfer, or when it arrives."},
			{"We prepare", "Everything is made fresh to order."},
			{"Delivered", "Collect it, or we bring it to you."},
		}}
	}},
	{"testimonials", "Testimonials", func() section { return &Testimonials{} }, func() section {
		return &Testimonials{Heading: "What people say", Items: []Testimonial{}}
	}},
	{"gallery", "Gallery", func() section { return &Gallery{} }, func() section {
		return &Gallery{Heading: "From our kitchen", Images: []GalleryItem{}}
	}},
	{"faq", "FAQ", func() section { return &FAQ{} }, func() section {
		return &FAQ{Heading: "Questions", Items: []FAQItem{}}
	}},
	{"catering", "Catering", func() section { return &Catering{} }, func() section {
		return &Catering{
			Heading:    "We've got your catering covered",
			Text:       "From weddings and birthdays to cocktail evenings and outdoor events, our small chops, parfait, cakes, zobo and cocktails are made to feed any crowd.",
			BannerNote: "Make them ask for more", CTALabel: "Learn more", CTAURL: "#catering",
		}
	}},
	{"footer", "Footer", func() section { return &Footer{} }, func() section {
		return &Footer{
			Tagline: "Make you ask for more", CTALabel: "Order Now", CTAURL: "/menu",
			Columns: []FooterColumn{
				{"Menu", []Link{{"Parfait", "/menu#parfait"}, {"Desserts & Cakes", "/menu#cakes"}, {"Small Chops", "/menu#small-chops"}, {"Yoghurt", "/menu#yoghurt"}, {"Healthy Drinks", "/menu#drinks"}}},
				{"SATI-L", []Link{{"Our Story", "/brand"}, {"Find a Spot", "/stores"}, {"Catering", "/catering"}, {"Careers", "/careers"}}},
				{"Visit", []Link{{"Visit our store", "/stores"}, {"Open 9am – 11pm daily", "/stores"}, {"hello@orentegrills.com", "mailto:hello@orentegrills.com"}}},
			},
		}
	}},
	{"navigation", "Navigation bar", func() section { return &Navigation{} }, func() section {
		return &Navigation{
			LogoAlt: "SATI-L Yoghurt & Confectionery",
			Links: []Link{
				{"Menu", "/menu"}, {"Gallery", "/#gallery"}, {"Find Us", "/#stores"},
			},
			OrderLabel: "Order Now", CartLabel: "Cart",
		}
	}},
	{"showcase", "Product showcase", func() section { return &Showcase{} }, func() section {
		return &Showcase{Items: []ShowcaseItem{}, OrderLabel: "Order Now", MarqueeText: "Order Now", MarqueeURL: "/menu"}
	}},
	{"menu_section", "Menu section", func() section { return &MenuSection{} }, func() section {
		return &MenuSection{
			ScriptLines: []string{"Sweet treats", "& healthy sips"},
			IntroLines:  []string{"Fresh & sweet", "for every occasion"},
			IntroButton: "See the full menu", AllLabel: "All", AddLabel: "Add to bag",
			SlideImages: []Image{},
		}
	}},
	{"announcement", "Announcement bar", func() section { return &Announcement{} }, func() section {
		return &Announcement{}
	}},
	{"seo", "Search and sharing", func() section { return &SEO{} }, func() section {
		return &SEO{
			Title:       "SATI-L Yoghurt & Confectionery — Parfait, Cakes, Small Chops & More",
			Description: "Parfait, desserts, cakes, small chops, yoghurt and healthy drinks, with outdoor catering and events. Make you ask for more.",
		}
	}},
	{"layout", "Landing page layout", func() section { return &Layout{} }, func() section {
		items := make([]LayoutItem, len(layoutKeys))
		for i, k := range layoutKeys {
			items[i] = LayoutItem{Key: k, Visible: true}
		}
		return &Layout{Sections: items}
	}},
}

func findDef(key string) (sectionDef, bool) {
	for _, d := range sectionDefs {
		if d.Key == key {
			return d, true
		}
	}
	return sectionDef{}, false
}

// Validate checks a section's content against its fixed fields and returns it
// cleaned up, along with the media files it uses.
func Validate(key string, raw json.RawMessage) (json.RawMessage, []string, error) {
	def, ok := findDef(key)
	if !ok {
		return nil, nil, fmt.Errorf("unknown section %q", key)
	}
	s := def.New()
	if err := decode(raw, s); err != nil {
		return nil, nil, fmt.Errorf("the content does not match the %s section: %w", key, err)
	}
	c := &check{}
	s.normalize(c)
	if c.err != "" {
		return nil, nil, errors.New(c.err)
	}
	out, err := json.Marshal(s)
	return out, dedupe(c.mediaIDs), err
}

func dedupe(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func defaultData(key string) json.RawMessage {
	def, ok := findDef(key)
	if !ok {
		return nil
	}
	raw, _ := json.Marshal(def.Defaults())
	return raw
}
