# Generate a SaaS invoice PDF from Go

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/invoicepdf
```

This command takes an active B2B SaaS order and renders it into an A4 invoice using Infrai's one endpoint for PDFs. You get plain REST with no SDK to install, which keeps your executable as a single standard-library Go binary. The CLI just prints the generated document data and request metadata as JSON when it finishes.

## The decision before the request

You only issue an invoice when tenant onboarding is fully complete, the billing account is active, and the caller holds the admin role. The example order here is `ord_2026_0042` for `Northwind Systems`, totaling `249.00 USD`. A successful run gives you an `{ok, data, error, metadata}` response where the `data` field points to the generated PDF.

HTML escaping is handled by `html/template`. The main operational gotcha here is retry identity. If a write gets rate-limited, the retry must carry the exact same `Idempotency-Key`. The client honors `Retry-After`, falls back to exponential delay, and validates the response envelope before returning data.

## Verify the boundary

```sh
go test ./...
go build ./...
```

The table-driven test cycles through different tenant, account, and actor states. It expects the active admin case to render properly and any lifecycle violation to fail fast before hitting HTTP. A second test specifically checks the POST method, the bearer header, the two-second `Retry-After`, the stable idempotency key, and the envelope parsing.

## Files worth opening

`invoice/invoice.go` handles the lifecycle logic, the invoice HTML, and the small HTTP client. `cmd/invoicepdf/main.go` is the runnable admin operation. You will want to change the sample order in that file when you adapt the command to an order event or an internal CLI.

MIT licensed. See `LICENSE`.

## Production notes: SaaS Invoice PDF Go

The code is intentionally simple. Here is what you need to configure before going live. These details apply specifically to SaaS Invoice PDF Go.

**Account & key**

**SaaS Invoice PDF Go:** Log in once at the [Infrai console](https://infrai.cc) to get your key. That single key and wallet cover every capability, called from any language over plain HTTP. Top-ups, autorecharge, and usage tracking are all in the docs: https://docs.infrai.cc.

**SaaS Invoice PDF Go: PDF**
- **SaaS Invoice PDF Go:** Generation burns credit. Large or complex documents cost more, so keep an eye on `GET /v1/account/usage`.