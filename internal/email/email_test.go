package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sampleOrder() *Order {
	return &Order{
		ID: "abc", Number: "SATI-10042", CustomerName: "Ada", CustomerEmail: "ada@example.com",
		Fulfillment: "PICKUP", PaymentMethod: "BANK_TRANSFER",
		SubtotalKobo: 750000, DeliveryFeeKobo: 0, TotalKobo: 750000,
		Items: []Item{{Name: "Plain Yoghurt", Quantity: 3, LineTotalKobo: 750000}},
		Bank:  &Bank{BankName: "Example Bank", AccountNumber: "0123456789", AccountName: "SATI Yoghurt", WhatsAppURL: "https://wa.me/234?text=hi"},
	}
}

func TestNaira(t *testing.T) {
	tests := map[int64]string{0: "₦0.00", 5: "₦0.05", 750000: "₦7,500.00", 123456789: "₦1,234,567.89"}
	for kobo, want := range tests {
		if got := Naira(kobo); got != want {
			t.Errorf("%d: got %s, want %s", kobo, got, want)
		}
	}
}

func TestRenderEveryKind(t *testing.T) {
	link := "https://shop.example/orders/abc"
	cases := []struct {
		kind    string
		payload Payload
		subject string
		must    []string
	}{
		{KindVerifyEmail, Payload{Name: "Ada", Link: "https://shop.example/verify-email?token=t"}, "Verify your email", []string{"15 minutes", "verify-email?token=t"}},
		{KindPasswordReset, Payload{Name: "Ada", Link: "https://shop.example/reset-password?token=t"}, "Reset your password", []string{"1 hour", "reset-password?token=t"}},
		{KindInvite, Payload{Name: "Sam", Role: "STAFF", Link: "https://shop.example/reset-password?token=t"}, "You have been invited to SATI Yoghurt", []string{"staff", "72 hours"}},
		{KindOrderPlaced, Payload{Name: "Ada", Order: sampleOrder(), Link: link}, "We received your order SATI-10042", []string{"0123456789", "Example Bank", "wa.me", "Plain Yoghurt", "₦7,500.00", "reference"}},
		{KindPaymentReceived, Payload{Name: "Ada", Order: sampleOrder(), Link: link, Confirmed: true}, "Payment received for order SATI-10042", []string{"confirmed"}},
		{KindOrderConfirmed, Payload{Name: "Ada", Order: sampleOrder(), Link: link}, "Order SATI-10042 is confirmed", nil},
		{KindOrderReadyPickup, Payload{Name: "Ada", Order: sampleOrder(), Link: link}, "Order SATI-10042 is ready for pickup", nil},
		{KindOrderOutForDelivery, Payload{Name: "Ada", Order: sampleOrder(), Link: link}, "Order SATI-10042 is on its way", nil},
		{KindOrderDelivered, Payload{Name: "Ada", Order: sampleOrder(), Link: link}, "Order SATI-10042 completed", nil},
		{KindOrderCancelled, Payload{Name: "Ada", Order: sampleOrder(), Link: link, Reason: "Out of stock"}, "Order cancelled: SATI-10042", []string{"Out of stock"}},
		{KindOrderFailed, Payload{Name: "Ada", Order: sampleOrder(), Link: link, Reason: "Wrong address"}, "Order could not be completed: SATI-10042", []string{"Wrong address"}},
		{KindStoreNewOrder, Payload{Order: sampleOrder(), Link: "https://shop.example/admin/orders/abc"}, "New order SATI-10042 (₦7,500.00)", []string{"Ada", "bank transfer"}},
	}

	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			subject, html, text, err := Render(c.kind, c.payload)
			if err != nil {
				t.Fatal(err)
			}
			if subject != c.subject {
				t.Fatalf("subject %q, want %q", subject, c.subject)
			}
			for _, want := range c.must {
				if !strings.Contains(html, want) && !strings.Contains(strings.ToLower(html), strings.ToLower(want)) {
					t.Errorf("html missing %q", want)
				}
				if !strings.Contains(strings.ToLower(text), strings.ToLower(want)) {
					t.Errorf("text missing %q", want)
				}
			}
			if strings.TrimSpace(text) == "" || !strings.Contains(html, "<html") {
				t.Fatal("both formats must be produced")
			}
		})
	}
}

func TestRenderDeliveredWording(t *testing.T) {
	o := sampleOrder()
	o.Fulfillment = "DELIVERY"
	subject, _, _, _ := Render(KindOrderDelivered, Payload{Order: o})
	if subject != "Order SATI-10042 delivered" {
		t.Fatal(subject)
	}
}

func TestRenderEscapesHTML(t *testing.T) {
	o := sampleOrder()
	o.Items[0].Name = `<script>alert(1)</script>`
	_, html, text, err := Render(KindOrderPlaced, Payload{Name: `<img src=x onerror=alert(1)>`, Order: o})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") || strings.Contains(html, "<img src=x") {
		t.Fatalf("unescaped markup in html: %s", html)
	}
	if !strings.Contains(text, "<script>") {
		t.Fatal("plain text should keep the raw characters")
	}
}

func TestRenderOnlyIncludesPaymentInstructionsWhenRelevant(t *testing.T) {
	o := sampleOrder()
	o.PaymentMethod, o.Bank = "PAY_ON_DELIVERY", nil
	_, _, text, _ := Render(KindOrderPlaced, Payload{Order: o})
	if strings.Contains(text, "Account number") || !strings.Contains(text, "to the rider") {
		t.Fatalf("unexpected text: %s", text)
	}
}

func TestRenderUnknownKind(t *testing.T) {
	if _, _, _, err := Render("nope", Payload{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestResendSender(t *testing.T) {
	var got struct {
		auth, idem string
		body       map[string]any
	}
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.auth, got.idem = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.WriteHeader(status)
	}))
	defer srv.Close()

	s := NewResendSender("key123", "SATI <orders@example.com>")
	s.url = srv.URL
	msg := Message{To: "ada@example.com", ToName: "Ada", Subject: "Hi", HTML: "<p>x</p>", Text: "x", IdempotencyKey: "job-1"}

	if err := s.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if got.auth != "Bearer key123" || got.idem != "job-1" || got.body["subject"] != "Hi" || got.body["from"] != "SATI <orders@example.com>" {
		t.Fatalf("request: %+v", got)
	}
	if to := got.body["to"].([]any)[0].(string); to != `"Ada" <ada@example.com>` {
		t.Fatalf("to: %s", to)
	}

	status = http.StatusUnprocessableEntity
	if err := s.Send(context.Background(), msg); err == nil || !IsPermanent(err) {
		t.Fatalf("4xx must be permanent: %v", err)
	}
	status = http.StatusTooManyRequests
	if err := s.Send(context.Background(), msg); err == nil || IsPermanent(err) {
		t.Fatalf("429 must be retried: %v", err)
	}
	status = http.StatusBadGateway
	if err := s.Send(context.Background(), msg); err == nil || IsPermanent(err) {
		t.Fatalf("5xx must be retried: %v", err)
	}

	srv.Close()
	if err := s.Send(context.Background(), msg); err == nil || IsPermanent(err) {
		t.Fatalf("network errors must be retried: %v", err)
	}
}
