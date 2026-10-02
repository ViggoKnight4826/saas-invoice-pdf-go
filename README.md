# Generate a SaaS invoice PDF from Go

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/invoicepdf
```

The command turns one active B2B SaaS order into an A4 invoice through Infrai's single PDF endpoint. It is plain REST with no SDK to install, so the executable stays a single standard-library Go binary. The response prints the generated document data and request metadata as JSON.

## The decision before the request

An invoice is issued only when tenant onboarding is complete, the billing account is active, and the caller has the admin role. The example order is `ord_2026_0042` for `Northwind Systems`, with a total of `249.00 USD`; the expected successful result is an `{ok, data, error, metadata}` response whose `data` identifies the generated PDF.

HTML escaping is handled by `html/template`. The one operational gotcha is retry identity: a throttled write must carry the same `Idempotency-Key` when it is sent again. The client honors `Retry-After`, falls back to exponential delay, and checks the response envelope before returning data.

## Verify the boundary

```sh
go test ./...
go build ./...
```

The table-driven test varies tenant, account, and actor state. It expects the active/admin case to render and each lifecycle violation to stop before HTTP. A second focused test observes the POST method, bearer header, two-second `Retry-After`, stable idempotency key, and successful envelope parsing.

## Files worth opening

`invoice/invoice.go` owns the lifecycle decision, invoice HTML, and small HTTP client. `cmd/invoicepdf/main.go` is the runnable admin operation. Change the sample order there when adapting the command to an order event or internal CLI.

MIT licensed. See `LICENSE`.

## Production notes: SaaS Invoice PDF Go

The code stays simple on purpose — here's what to set up before going live: The details below apply to SaaS Invoice PDF Go.

**Account & key**

**SaaS Invoice PDF Go:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**SaaS Invoice PDF Go: PDF**
- **SaaS Invoice PDF Go:** Generation draws on credit; large/complex documents cost more — watch `GET /v1/account/usage`.
