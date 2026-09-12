package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/example/saas-invoice-pdf/invoice"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "INFRAI_API_KEY is required")
		os.Exit(2)
	}
	order := invoice.Order{
		ID: "ord_2026_0042", TenantName: "Northwind Systems",
		TenantState: invoice.TenantActive, AccountState: invoice.AccountActive,
		ActorRole: invoice.RoleAdmin, Currency: "USD", AmountCents: 24900,
		IssuedOn: time.Now(),
	}
	result, err := (invoice.Client{APIKey: key, MaxRetries: 3}).Generate(context.Background(), order)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
