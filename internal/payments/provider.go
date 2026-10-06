package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ProviderPaystack = "PAYSTACK"

	VerifySuccess   = "success"
	VerifyFailed    = "failed"
	VerifyAbandoned = "abandoned"
	VerifyPending   = "pending"

	EventChargeSuccess = "charge.success"
)

type InitRequest struct {
	Email       string
	AmountKobo  int64
	Reference   string
	CallbackURL string
	OrderID     string
	OrderNumber string
}

type InitResult struct {
	AuthorizationURL string
	AccessCode       string
	Reference        string
}

type Verification struct {
	Status          string
	AmountKobo      int64
	Currency        string
	GatewayResponse string
	TransactionID   string
}

type WebhookEvent struct {
	Key       string
	Type      string
	Reference string
}

var ErrInvalidSignature = errors.New("invalid webhook signature")

// Provider hides the payment gateway so another one can be added without touching orders.
type Provider interface {
	Name() string
	Initialize(ctx context.Context, req InitRequest) (InitResult, error)
	Verify(ctx context.Context, reference string) (Verification, error)
	ParseWebhook(signature string, body []byte) (WebhookEvent, error)
}

const paystackURL = "https://api.paystack.co"

type Paystack struct {
	secretKey     string
	webhookSecret string
	baseURL       string
	client        *http.Client
}

// NewPaystack signs and checks webhooks with webhookSecret, which falls back to the secret key.
func NewPaystack(secretKey, webhookSecret string) *Paystack {
	if webhookSecret == "" {
		webhookSecret = secretKey
	}
	return &Paystack{
		secretKey: secretKey, webhookSecret: webhookSecret, baseURL: paystackURL,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// WithBaseURL points the client at another address, which tests use to call a fake gateway.
func (p *Paystack) WithBaseURL(u string) *Paystack {
	p.baseURL = strings.TrimRight(u, "/")
	return p
}

func (p *Paystack) Name() string { return ProviderPaystack }

func (p *Paystack) call(ctx context.Context, method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, fmt.Errorf("paystack responded %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	return resp.StatusCode, json.Unmarshal(raw, out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (p *Paystack) Initialize(ctx context.Context, req InitRequest) (InitResult, error) {
	var out struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			AuthorizationURL string `json:"authorization_url"`
			AccessCode       string `json:"access_code"`
			Reference        string `json:"reference"`
		} `json:"data"`
	}
	_, err := p.call(ctx, http.MethodPost, "/transaction/initialize", map[string]any{
		"email": req.Email, "amount": req.AmountKobo, "currency": Currency,
		"reference": req.Reference, "callback_url": req.CallbackURL,
		"metadata": map[string]any{"order_id": req.OrderID, "order_number": req.OrderNumber},
	}, &out)
	if err != nil {
		return InitResult{}, err
	}
	if !out.Status || out.Data.AuthorizationURL == "" {
		return InitResult{}, fmt.Errorf("paystack did not start the payment: %s", out.Message)
	}
	return InitResult{AuthorizationURL: out.Data.AuthorizationURL, AccessCode: out.Data.AccessCode, Reference: out.Data.Reference}, nil
}

func (p *Paystack) Verify(ctx context.Context, reference string) (Verification, error) {
	var out struct {
		Status bool `json:"status"`
		Data   struct {
			Status          string      `json:"status"`
			Amount          int64       `json:"amount"`
			Currency        string      `json:"currency"`
			ID              json.Number `json:"id"`
			GatewayResponse string      `json:"gateway_response"`
		} `json:"data"`
	}
	code, err := p.call(ctx, http.MethodGet, "/transaction/verify/"+url.PathEscape(reference), nil, &out)
	if code == http.StatusNotFound {
		return Verification{Status: VerifyPending}, nil
	}
	if err != nil {
		return Verification{}, err
	}

	v := Verification{
		AmountKobo: out.Data.Amount, Currency: out.Data.Currency,
		GatewayResponse: out.Data.GatewayResponse, TransactionID: out.Data.ID.String(),
	}
	switch strings.ToLower(out.Data.Status) {
	case "success":
		v.Status = VerifySuccess
	case "failed", "reversed":
		v.Status = VerifyFailed
	case "abandoned":
		v.Status = VerifyAbandoned
	default:
		v.Status = VerifyPending
	}
	return v, nil
}

// ParseWebhook checks the HMAC-SHA512 signature Paystack puts on every event.
func (p *Paystack) ParseWebhook(signature string, body []byte) (WebhookEvent, error) {
	mac := hmac.New(sha512.New, []byte(p.webhookSecret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(strings.ToLower(strings.TrimSpace(signature)))) {
		return WebhookEvent{}, ErrInvalidSignature
	}

	var payload struct {
		Event string `json:"event"`
		Data  struct {
			ID        json.Number `json:"id"`
			Reference string      `json:"reference"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return WebhookEvent{}, fmt.Errorf("unreadable webhook body: %w", err)
	}

	key := payload.Event + ":" + payload.Data.ID.String()
	if payload.Data.ID.String() == "" {
		sum := sha256.Sum256(body)
		key = payload.Event + ":" + hex.EncodeToString(sum[:])
	}
	return WebhookEvent{Key: key, Type: payload.Event, Reference: payload.Data.Reference}, nil
}
