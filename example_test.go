package gamecoin_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

// The examples below are run by `go test`. They talk to a tiny in-memory fake of the GameCoin API (exampleClient), so
// they need no network; in your own code, build the client with gamecoin.New(os.Getenv("GAMECOIN_SECRET_KEY")).

// exampleAPI is a minimal in-memory fake of the server API: one currency (gems), one item (potion at 20 gems) and one
// pack (gems-500).
type exampleAPI struct {
	mu      sync.Mutex
	gems    int64
	potions int64
	entries []map[string]any
	seq     int
	seen    map[string]string // idempotency key -> response body
}

// record appends a ledger entry for a movement that was just applied to a.gems, and returns it.
func (a *exampleAPI) record(kind string, delta int64) map[string]any {
	a.seq++
	entry := map[string]any{
		"id": fmt.Sprintf("entry-%d", a.seq), "type": kind, "currency": "gems", "paidDelta": 0, "bonusDelta": delta,
		"balanceAfter": map[string]any{"paid": 0, "bonus": a.gems, "total": a.gems},
		"reason":       nil, "metadata": map[string]any{}, "ref": map[string]any{}, "createdAt": "2026-10-05T20:38:27.227Z",
	}
	a.entries = append(a.entries, entry)
	return entry
}

func (a *exampleAPI) balance() map[string]any {
	return map[string]any{"currency": "gems", "paid": 0, "bonus": a.gems, "total": a.gems}
}

func (a *exampleAPI) item() map[string]any {
	return map[string]any{"sku": "potion", "quantity": a.potions, "updatedAt": "2026-10-05T20:40:00.000Z"}
}

func (a *exampleAPI) order(status string) map[string]any {
	return map[string]any{
		"id": "order-1", "playerId": "player-1", "status": status, "pack": map[string]any{"sku": "gems-500", "name": "Bag of gems"},
		"amountCents": 499, "currency": "EUR", "country": "BE", "vatRateBp": 2100, "vatCents": 87, "netCents": 412,
		"createdAt": "2026-10-05T20:38:24.539Z", "paidAt": nil, "fulfilledAt": nil, "refundedAt": nil, "creatorCode": nil,
	}
}

func (a *exampleAPI) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var body struct {
		Currency string `json:"currency"`
		Amount   int64  `json:"amount"`
		SKU      string `json:"sku"`
		Quantity int64  `json:"quantity"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	send := func(status int, v any, replayed bool) {
		w.Header().Set("Content-Type", "application/json")
		if replayed {
			w.Header().Set("Idempotent-Replayed", "true")
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	fail := func(status int, code, message string, details map[string]any) {
		e := map[string]any{"code": code, "message": message}
		if details != nil {
			e["details"] = details
		}
		send(status, map[string]any{"error": e}, false)
	}

	// idempotent replay of a mutation
	if key := r.Header.Get("Idempotency-Key"); key != "" && r.Method == http.MethodPost {
		if previous, ok := a.seen[key]; ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotent-Replayed", "true")
			_, _ = w.Write([]byte(previous))
			return
		}
	}
	remember := func(v any) {
		encoded, _ := json.Marshal(v)
		if key := r.Header.Get("Idempotency-Key"); key != "" {
			a.seen[key] = string(encoded)
		}
		send(http.StatusOK, v, false)
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	switch {
	case r.Method == http.MethodGet && path == "/catalog":
		send(200, map[string]any{
			"currencies": []any{map[string]any{"code": "gems", "name": "Gems", "purchasable": true}},
			"items": []any{map[string]any{"sku": "potion", "name": "Potion", "description": nil, "type": "consumable",
				"price": map[string]any{"currency": "gems", "amount": 20}, "maxOwned": nil, "clientPurchasable": true, "metadata": map[string]any{}}},
			"packs": []any{map[string]any{"sku": "gems-500", "name": "Bag of gems", "description": nil, "priceCents": 499, "currency": "EUR",
				"grants": []any{map[string]any{"currency": "gems", "amount": 500, "bonus": 50}}, "items": []any{}, "badge": nil, "maxPerPlayer": nil, "availability": nil}},
			"prelaunch": false, "launchAt": nil,
		}, false)
	case r.Method == http.MethodPut && strings.HasPrefix(path, "/players/"):
		send(200, map[string]any{"id": "player-1", "externalId": "user-42", "kind": "external", "displayName": "Ann",
			"country": "BE", "blocked": false, "createdAt": "2026-10-05T20:38:24.539Z"}, false)
	case strings.HasSuffix(path, "/wallet") && r.Method == http.MethodGet:
		send(200, map[string]any{"playerId": "player-1", "balances": []any{a.balance()}}, false)
	case strings.HasSuffix(path, "/wallet/grant"):
		a.gems += body.Amount
		remember(map[string]any{"entry": a.record("grant", body.Amount), "balance": a.balance()})
	case strings.HasSuffix(path, "/wallet/spend"):
		if body.Amount > a.gems {
			fail(409, "INSUFFICIENT_FUNDS", "Insufficient funds", map[string]any{"required": body.Amount, "available": a.gems})
			return
		}
		a.gems -= body.Amount
		remember(map[string]any{"entry": a.record("spend", -body.Amount), "balance": a.balance()})
	case strings.HasSuffix(path, "/ledger"):
		newestFirst := make([]map[string]any, 0, len(a.entries))
		for i := len(a.entries) - 1; i >= 0; i-- {
			newestFirst = append(newestFirst, a.entries[i])
		}
		send(200, map[string]any{"entries": newestFirst, "nextCursor": nil}, false)
	case strings.HasSuffix(path, "/inventory/consume"):
		a.potions -= body.Quantity
		remember(map[string]any{"item": a.item()})
	case strings.HasSuffix(path, "/purchases"):
		a.gems -= 20 * body.Quantity
		a.potions += body.Quantity
		remember(map[string]any{"entry": a.record("spend", -20*body.Quantity), "balance": a.balance(), "item": a.item()})
	case strings.HasSuffix(path, "/checkouts"):
		remember(map[string]any{"order": a.order("pending"), "checkoutUrl": "https://gamecoin.example/pay/order-1?t=signed"})
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/orders/"):
		send(200, a.order("pending"), false)
	case strings.HasSuffix(path, "/refund"):
		fail(409, "CONFLICT", "Order cannot go from pending to refunded", nil)
	default:
		fail(404, "NOT_FOUND", "Not found", nil)
	}
}

// exampleClient returns a client wired to a fresh in-memory fake of the API.
func exampleClient() *gamecoin.Client {
	api := &exampleAPI{seen: map[string]string{}}
	server := httptest.NewServer(http.HandlerFunc(api.serve))
	client, err := gamecoin.New("gc_sk_test_example", gamecoin.WithBaseURL(server.URL))
	if err != nil {
		panic(err)
	}
	return client
}

func Example() {
	// client, err := gamecoin.New(os.Getenv("GAMECOIN_SECRET_KEY"))
	client := exampleClient()
	ctx := context.Background()
	player := gamecoin.Ext("user-42") // your own id for the player

	granted, err := client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gems", Amount: 100, Reason: "level_up"})
	if err != nil {
		panic(err)
	}
	spent, err := client.Wallet.Spend(ctx, player, gamecoin.SpendParams{Currency: "gems", Amount: 30})
	if err != nil {
		panic(err)
	}
	checkout, err := client.Checkouts.Create(ctx, player, gamecoin.CheckoutParams{PackSKU: "gems-500"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("gems: %d -> %d; %s order, pay at %s\n", granted.Balance.Total, spent.Balance.Total, checkout.Order.Status, checkout.CheckoutURL)
	// Output: gems: 100 -> 70; pending order, pay at https://gamecoin.example/pay/order-1?t=signed
}

func ExampleNew() {
	client, err := gamecoin.New(
		"gc_sk_test_example",
		gamecoin.WithBaseURL("http://localhost:3400"), // default: https://gamecoin.apilow.com
		gamecoin.WithTimeout(10*time.Second),          // per HTTP attempt; default 30 s
		gamecoin.WithMaxRetries(3),                    // automatic retries; default 2, 0 turns them off
		gamecoin.WithHTTPClient(&http.Client{}),       // your own *http.Client (proxy, instrumentation...)
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(client.Environment(), client.BaseURL())

	// A publishable key belongs in the browser SDK: the server SDK refuses it.
	_, err = gamecoin.New("gc_pk_test_example")
	fmt.Println(errors.Is(err, gamecoin.ErrInvalidArgument))
	// Output:
	// test http://localhost:3400
	// true
}

func ExamplePlayersService_Upsert() {
	client := exampleClient()
	ctx := context.Background()
	name, country := "Ann", "BE"

	player, err := client.Players.Upsert(ctx, gamecoin.Ext("user-42"), &gamecoin.PlayerProfile{
		DisplayName: &name,
		Country:     &country,
		Clear:       []gamecoin.ProfileField{gamecoin.FieldEmail}, // erase a field
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(*player.ExternalID, *player.DisplayName)
	// Output: user-42 Ann
}

func ExampleWalletService_Grant() {
	client := exampleClient()
	ctx := context.Background()
	player := gamecoin.Ext("user-42")
	params := gamecoin.GrantParams{Currency: "gems", Amount: 100, Reason: "quest", Metadata: map[string]string{"quest": "dragon"}}

	// Your own key makes your own retries safe: the same key with the same request returns the original response.
	key := gamecoin.WithIdempotencyKey("quest-dragon-user-42")
	first, _ := client.Wallet.Grant(ctx, player, params, key)
	second, _ := client.Wallet.Grant(ctx, player, params, key)

	fmt.Println(first.Replayed, second.Replayed, second.Balance.Total)
	// Output: false true 100
}

func ExampleWalletService_Spend() {
	client := exampleClient()
	ctx := context.Background()

	_, err := client.Wallet.Spend(ctx, gamecoin.Ext("user-42"), gamecoin.SpendParams{Currency: "gems", Amount: 50})

	var gcErr *gamecoin.Error
	if errors.As(err, &gcErr) && gcErr.Code == gamecoin.CodeInsufficientFunds {
		required, _ := gcErr.DetailInt64("required")
		available, _ := gcErr.DetailInt64("available")
		fmt.Printf("needs %d, has %d (HTTP %d)\n", required, available, gcErr.Status)
	}
	// Output: needs 50, has 0 (HTTP 409)
}

func ExampleLedgerService_Iterate() {
	client := exampleClient()
	ctx := context.Background()
	player := gamecoin.Ext("user-42")
	_, _ = client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gems", Amount: 100})
	_, _ = client.Wallet.Spend(ctx, player, gamecoin.SpendParams{Currency: "gems", Amount: 30})

	it := client.Ledger.Iterate(ctx, player, &gamecoin.LedgerListParams{Currency: "gems", Limit: 50})
	for it.Next() {
		entry := it.Entry()
		fmt.Println(entry.Type, entry.BonusDelta+entry.PaidDelta)
	}
	if err := it.Err(); err != nil {
		panic(err)
	}
	// Output:
	// spend -30
	// grant 100
}

func ExamplePurchasesService_Create() {
	client := exampleClient()
	ctx := context.Background()
	player := gamecoin.Ext("user-42")
	_, _ = client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gems", Amount: 100})

	purchase, err := client.Purchases.Create(ctx, player, gamecoin.PurchaseParams{SKU: "potion", Quantity: 2})
	if err != nil {
		panic(err)
	}
	fmt.Println(purchase.Item.Quantity, "potion(s),", purchase.Balance.Total, purchase.Balance.Currency, "left")
	// Output: 2 potion(s), 60 gems left
}

func ExampleOrdersService_Refund() {
	client := exampleClient()
	ctx := context.Background()

	_, err := client.Orders.Refund(ctx, "order-1", &gamecoin.RefundParams{Reason: "customer asked"})

	if gamecoin.IsCode(err, gamecoin.CodeConflict) {
		fmt.Println("cannot refund an unpaid order")
	}
	// Output: cannot refund an unpaid order
}
