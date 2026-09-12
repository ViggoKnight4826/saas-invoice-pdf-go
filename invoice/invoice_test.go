package invoice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func validOrder() Order {
	return Order{ID: "ord_42", TenantName: "A & B", TenantState: TenantActive, AccountState: AccountActive, ActorRole: RoleAdmin, Currency: "usd", AmountCents: 24900, IssuedOn: time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)}
}

func TestInvoiceEligibility(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Order)
		want string
	}{
		{"active tenant admin", func(*Order) {}, ""},
		{"onboarding tenant", func(o *Order) { o.TenantState = TenantOnboarding }, "onboarding"},
		{"suspended account", func(o *Order) { o.AccountState = AccountSuspended }, "account must be active"},
		{"member actor", func(o *Order) { o.ActorRole = RoleMember }, "requires an admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := validOrder()
			tt.edit(&o)
			_, err := RenderHTML(o)
			if tt.want == "" && err != nil {
				t.Fatalf("RenderHTML() error = %v", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("RenderHTML() error = %v, want text %q", err, tt.want)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGenerateRetriesRateLimitWithStableRequest(t *testing.T) {
	var calls int
	var firstKey string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		var requestBody struct {
			Store bool `json:"store"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if requestBody.Store {
			t.Fatal("store = true, want false")
		}
		key := r.Header.Get("Idempotency-Key")
		if calls == 1 {
			firstKey = key
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"2"}}, Body: io.NopCloser(strings.NewReader(`{"ok":false,"error":{"message":"rate limited"}}`))}, nil
		}
		if key == "" || key != firstKey {
			t.Fatalf("idempotency key changed: %q then %q", firstKey, key)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"data":{"job_id":"job_42"},"metadata":{}}`))}, nil
	})
	var slept time.Duration
	c := Client{APIKey: "test-key", HTTPClient: &http.Client{Transport: transport}, MaxRetries: 1, Sleep: func(_ context.Context, d time.Duration) error { slept = d; return nil }}
	result, err := c.Generate(context.Background(), validOrder())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || slept != 2*time.Second || !strings.Contains(string(result.Data), "job_42") {
		t.Fatalf("calls=%d slept=%s data=%s", calls, slept, result.Data)
	}
}
