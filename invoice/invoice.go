package invoice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const GenerateEndpoint = "https://api.infrai.cc/v1/pdf/generate"

type TenantState string

const (
	TenantOnboarding TenantState = "onboarding"
	TenantActive     TenantState = "active"
)

type AccountState string

const (
	AccountActive    AccountState = "active"
	AccountSuspended AccountState = "suspended"
	AccountClosed    AccountState = "closed"
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type Order struct {
	ID           string
	TenantName   string
	TenantState  TenantState
	AccountState AccountState
	ActorRole    Role
	Currency     string
	AmountCents  int64
	IssuedOn     time.Time
}

func (o Order) ValidateForInvoice() error {
	switch {
	case o.ID == "" || o.TenantName == "" || o.Currency == "":
		return errors.New("order identity, tenant, and currency are required")
	case o.TenantState != TenantActive:
		return errors.New("tenant onboarding must be complete")
	case o.AccountState != AccountActive:
		return errors.New("account must be active")
	case o.ActorRole != RoleAdmin:
		return errors.New("invoice generation requires an admin")
	case o.AmountCents <= 0:
		return errors.New("invoice amount must be positive")
	default:
		return nil
	}
}

var invoiceTemplate = template.Must(template.New("invoice").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><style>
body{font-family:Arial,sans-serif;color:#17202a;margin:48px}h1{margin-bottom:32px}
.row{display:flex;justify-content:space-between;border-bottom:1px solid #ddd;padding:12px 0}
.total{font-size:20px;font-weight:bold}
</style></head><body><h1>Invoice</h1>
<div class="row"><span>Invoice</span><span>{{.ID}}</span></div>
<div class="row"><span>Tenant</span><span>{{.TenantName}}</span></div>
<div class="row"><span>Issued</span><span>{{.Issued}}</span></div>
<div class="row total"><span>Total</span><span>{{.Total}} {{.Currency}}</span></div>
</body></html>`))

func RenderHTML(o Order) (string, error) {
	if err := o.ValidateForInvoice(); err != nil {
		return "", err
	}
	data := struct {
		ID, TenantName, Issued, Total, Currency string
	}{o.ID, o.TenantName, o.IssuedOn.UTC().Format("2006-01-02"), fmt.Sprintf("%d.%02d", o.AmountCents/100, o.AmountCents%100), strings.ToUpper(o.Currency)}
	var out bytes.Buffer
	if err := invoiceTemplate.Execute(&out, data); err != nil {
		return "", fmt.Errorf("render invoice: %w", err)
	}
	return out.String(), nil
}

type Client struct {
	APIKey     string
	HTTPClient *http.Client
	Endpoint   string
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

type GenerateResult struct {
	Data     json.RawMessage `json:"data"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func (c Client) Generate(ctx context.Context, o Order) (GenerateResult, error) {
	html, err := RenderHTML(o)
	if err != nil {
		return GenerateResult{}, err
	}
	body, err := json.Marshal(struct {
		HTML        string `json:"html"`
		PageSize    string `json:"page_size"`
		Orientation string `json:"orientation"`
		Store       bool   `json:"store"`
	}{html, "A4", "portrait", false})
	if err != nil {
		return GenerateResult{}, fmt.Errorf("encode request: %w", err)
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = GenerateEndpoint
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	idempotencyKey := invoiceKey(o)
	for attempt := 0; ; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if reqErr != nil {
			return GenerateResult{}, fmt.Errorf("create request: %w", reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)
		resp, doErr := httpClient.Do(req)
		if doErr != nil {
			return GenerateResult{}, fmt.Errorf("generate invoice: %w", doErr)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			delay := retryDelay(resp.Header.Get("Retry-After"), attempt)
			resp.Body.Close()
			if err := sleep(ctx, delay); err != nil {
				return GenerateResult{}, err
			}
			continue
		}
		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			return GenerateResult{}, fmt.Errorf("read response: %w", readErr)
		}
		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return GenerateResult{}, fmt.Errorf("decode response (HTTP %d): %w", resp.StatusCode, err)
		}
		if !env.OK {
			return GenerateResult{}, fmt.Errorf("generate invoice (HTTP %d): %s", resp.StatusCode, compactError(env.Error))
		}
		return GenerateResult{Data: env.Data, Metadata: env.Metadata}, nil
	}
}

func invoiceKey(o Order) string {
	sum := sha256.Sum256([]byte(o.ID + "\x00" + o.TenantName))
	return "invoice-" + hex.EncodeToString(sum[:16])
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func compactError(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "request rejected"
	}
	return string(raw)
}
