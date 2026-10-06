package server

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

const paystackWebhookSecret = "whsec_test"

type verifyState struct {
	status   string
	amount   int64
	currency string
}

// fakePaystack stands in for api.paystack.co so the real client code is exercised.
type fakePaystack struct {
	srv *httptest.Server

	mu          sync.Mutex
	verify      map[string]verifyState
	failInit    bool
	failVerify  bool
	initCalls   []map[string]any
	verifyCalls int
}

func newFakePaystack() *fakePaystack {
	f := &fakePaystack{verify: map[string]verifyState{}}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /transaction/initialize", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer sk_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.initCalls = append(f.initCalls, body)
		if f.failInit {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		ref, _ := body["reference"].(string)
		fmt.Fprintf(w, `{"status":true,"message":"ok","data":{"authorization_url":"https://checkout.paystack.test/%s","access_code":"ac_%s","reference":"%s"}}`, ref, ref, ref)
	})

	mux.HandleFunc("GET /transaction/verify/{ref}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.verifyCalls++
		if f.failVerify {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		state, ok := f.verify[r.PathValue("ref")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"status":false,"message":"Transaction reference not found"}`)
			return
		}
		fmt.Fprintf(w, `{"status":true,"data":{"status":%q,"amount":%d,"currency":%q,"id":4099260516,"gateway_response":"Successful"}}`,
			state.status, state.amount, state.currency)
	})

	f.srv = httptest.NewServer(mux)
	return f
}

func (f *fakePaystack) set(reference, status string, amount int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verify[reference] = verifyState{status: status, amount: amount, currency: "NGN"}
}

func (f *fakePaystack) setCurrency(reference, status string, amount int64, currency string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verify[reference] = verifyState{status: status, amount: amount, currency: currency}
}

func (f *fakePaystack) verifyCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.verifyCalls
}

func signBody(body string) string {
	mac := hmac.New(sha512.New, []byte(paystackWebhookSecret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func webhookBody(event string, id int, reference string) string {
	return fmt.Sprintf(`{"event":%q,"data":{"id":%d,"reference":%q,"status":"success"}}`, event, id, reference)
}

func (e *catalogEnv) sendWebhook(body, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/v1/webhooks/paystack", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-Paystack-Signature", signature)
	}
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}
