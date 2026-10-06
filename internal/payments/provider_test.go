package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sign(secret, body string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestParseWebhook(t *testing.T) {
	p := NewPaystack("sk_test", "")
	body := `{"event":"charge.success","data":{"id":4099260516,"reference":"SATI-10042-AB12"}}`

	event, err := p.ParseWebhook(sign("sk_test", body), []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != EventChargeSuccess || event.Reference != "SATI-10042-AB12" || event.Key != "charge.success:4099260516" {
		t.Fatalf("event: %+v", event)
	}
	if _, err := p.ParseWebhook(strings.ToUpper(sign("sk_test", body)), []byte(body)); err != nil {
		t.Fatalf("signature case must not matter: %v", err)
	}

	for name, sig := range map[string]string{
		"wrong secret": sign("other", body),
		"empty":        "",
		"tampered":     sign("sk_test", body+" "),
		"garbage":      "zzz",
	} {
		if _, err := p.ParseWebhook(sig, []byte(body)); !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("%s: expected ErrInvalidSignature, got %v", name, err)
		}
	}
}

func TestWebhookSecretOverridesSecretKey(t *testing.T) {
	p := NewPaystack("sk_test", "whsec")
	body := `{"event":"charge.success","data":{"id":1,"reference":"r"}}`
	if _, err := p.ParseWebhook(sign("whsec", body), []byte(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ParseWebhook(sign("sk_test", body), []byte(body)); !errors.Is(err, ErrInvalidSignature) {
		t.Fatal("the webhook secret must replace the secret key when set")
	}
}

func TestWebhookWithoutIDStillHasAStableKey(t *testing.T) {
	p := NewPaystack("k", "")
	body := `{"event":"transfer.success","data":{"reference":"r"}}`
	a, _ := p.ParseWebhook(sign("k", body), []byte(body))
	b, _ := p.ParseWebhook(sign("k", body), []byte(body))
	if a.Key == "" || a.Key != b.Key {
		t.Fatalf("keys: %q %q", a.Key, b.Key)
	}
}

func TestVerifyStatusMapping(t *testing.T) {
	status := "success"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/transaction/verify/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, `{"status":true,"data":{"status":%q,"amount":250000,"currency":"NGN","id":77,"gateway_response":"ok"}}`, status)
	}))
	defer srv.Close()
	p := NewPaystack("k", "").WithBaseURL(srv.URL)

	cases := map[string]string{
		"success": VerifySuccess, "failed": VerifyFailed, "reversed": VerifyFailed,
		"abandoned": VerifyAbandoned, "ongoing": VerifyPending, "pending": VerifyPending, "queued": VerifyPending,
	}
	for in, want := range cases {
		status = in
		v, err := p.Verify(context.Background(), "ref")
		if err != nil || v.Status != want {
			t.Errorf("%s: got %q %v, want %q", in, v.Status, err, want)
		}
	}
	status = "success"
	v, _ := p.Verify(context.Background(), "ref")
	if v.AmountKobo != 250000 || v.Currency != "NGN" || v.TransactionID != "77" {
		t.Fatalf("verification: %+v", v)
	}
	if v, err := p.Verify(context.Background(), "missing"); err != nil || v.Status != VerifyPending {
		t.Fatalf("unknown reference should look pending: %+v %v", v, err)
	}
}

func TestInitializeSendsKoboAndHandlesErrors(t *testing.T) {
	var got map[string]any
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"status":false,"message":"Invalid key"}`)
			return
		}
		_ = jsonDecode(r, &got)
		fmt.Fprint(w, `{"status":true,"data":{"authorization_url":"https://pay/x","access_code":"ac","reference":"r1"}}`)
	}))
	defer srv.Close()
	p := NewPaystack("k", "").WithBaseURL(srv.URL)

	res, err := p.Initialize(context.Background(), InitRequest{Email: "a@b.co", AmountKobo: 750000, Reference: "r1", CallbackURL: "https://shop/orders/1"})
	if err != nil || res.AuthorizationURL != "https://pay/x" {
		t.Fatalf("%+v %v", res, err)
	}
	if got["amount"].(float64) != 750000 || got["currency"] != "NGN" || got["callback_url"] != "https://shop/orders/1" {
		t.Fatalf("request body: %v", got)
	}

	fail = true
	if _, err := p.Initialize(context.Background(), InitRequest{}); err == nil {
		t.Fatal("gateway errors must surface")
	}
}

func jsonDecode(r *http.Request, v any) error {
	return decodeJSON(r.Body, v)
}
