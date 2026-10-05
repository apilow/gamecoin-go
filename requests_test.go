package gamecoin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	gamecoin "github.com/apilow/gamecoin-go"
)

// What each method sends: verb, path, query, body, headers.

func ptr[T any](v T) *T { return &v }

func checkRequest(t *testing.T, got recorded, method, target, body string) {
	t.Helper()
	if got.Method != method {
		t.Errorf("method = %s, want %s", got.Method, method)
	}
	if got.RequestURI != target {
		t.Errorf("request target = %s, want %s", got.RequestURI, target)
	}
	if got.Body != body {
		t.Errorf("body = %s, want %s", got.Body, body)
	}
}

func TestCatalogGet(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleCatalog)
	if _, err := r.client.Catalog.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, r.f.only(), http.MethodGet, "/api/v1/catalog", "")
}

func TestPlayersUpsertWithoutAProfileSendsAnEmptyObject(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePlayer)
	if _, err := r.client.Players.Upsert(context.Background(), user, nil); err != nil {
		t.Fatal(err)
	}
	req := r.f.only()
	checkRequest(t, req, http.MethodPut, "/api/v1/players/ext%3Auser-42", "{}")
	if ct := req.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if req.Header.Get("Idempotency-Key") != "" {
		t.Error("an upsert carries no idempotency key")
	}
}

func TestPlayersUpsertSendsOnlyTheFieldsThatAreSet(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePlayer)
	_, err := r.client.Players.Upsert(context.Background(), user, &gamecoin.PlayerProfile{
		DisplayName: ptr("Ann"), Country: ptr("BE"), BirthYear: ptr(1990),
	})
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, r.f.only(), http.MethodPut, "/api/v1/players/ext%3Auser-42", `{"birthYear":1990,"country":"BE","displayName":"Ann"}`)
}

func TestPlayersUpsertSendsNullForTheFieldsToClear(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePlayer)
	_, err := r.client.Players.Upsert(context.Background(), user, &gamecoin.PlayerProfile{
		DisplayName: ptr("Ann"),
		Clear:       []gamecoin.ProfileField{gamecoin.FieldEmail, gamecoin.FieldBirthYear},
	})
	if err != nil {
		t.Fatal(err)
	}
	checkRequest(t, r.f.only(), http.MethodPut, "/api/v1/players/ext%3Auser-42", `{"birthYear":null,"displayName":"Ann","email":null}`)
}

func TestPlayersUpsertRejectsAFieldBothSetAndCleared(t *testing.T) {
	r := newRig(t)
	_, err := r.client.Players.Upsert(context.Background(), user, &gamecoin.PlayerProfile{
		Email: ptr("a@b.test"), Clear: []gamecoin.ProfileField{gamecoin.FieldEmail},
	})
	if !isArgError(err) {
		t.Errorf("err = %v", err)
	}
	_, err = r.client.Players.Upsert(context.Background(), user, &gamecoin.PlayerProfile{Clear: []gamecoin.ProfileField{"nickname"}})
	if !isArgError(err) {
		t.Errorf("an unknown field in Clear: err = %v", err)
	}
	if n := len(r.f.requests()); n != 0 {
		t.Errorf("%d requests were sent", n)
	}
}

func TestPlayersGetAndCreateToken(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePlayer).ok(`{"token":"gc_pt_abc","expiresAt":"2026-10-05T21:38:32.000Z"}`)
	ctx := context.Background()
	if _, err := r.client.Players.Get(ctx, user); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.Players.CreateToken(ctx, user); err != nil {
		t.Fatal(err)
	}
	reqs := r.f.requests()
	checkRequest(t, reqs[0], http.MethodGet, "/api/v1/players/ext%3Auser-42", "")
	checkRequest(t, reqs[1], http.MethodPost, "/api/v1/players/ext%3Auser-42/tokens", "")
	if reqs[1].Header.Get("Idempotency-Key") != "" {
		t.Error("creating a token carries no idempotency key")
	}
}

func TestWalletGet(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"playerId":"p1","balances":[]}`)
	if _, err := r.client.Wallet.Get(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, r.f.only(), http.MethodGet, "/api/v1/players/ext%3Auser-42/wallet", "")
}

func TestWalletGrantBodyWithReasonAndMetadata(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleMovement)
	_, err := r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{
		Currency: "gems", Amount: 100, Reason: "level_up", Metadata: map[string]string{"level": "7", "quote": `a"b<c>&`},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := r.f.only()
	checkRequest(t, req, http.MethodPost, "/api/v1/players/ext%3Auser-42/wallet/grant",
		`{"currency":"gems","amount":100,"reason":"level_up","metadata":{"level":"7","quote":"a\"b<c>&"}}`)
	if req.Header.Get("Idempotency-Key") == "" {
		t.Error("a grant carries an idempotency key")
	}
	if ct := req.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestWalletGrantMinimalBodyOmitsOptionalFields(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleMovement)
	if _, err := r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: "gems", Amount: 5}); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, r.f.only(), http.MethodPost, "/api/v1/players/ext%3Auser-42/wallet/grant", `{"currency":"gems","amount":5}`)
}

func TestWalletSpendBody(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleMovement)
	_, err := r.client.Wallet.Spend(context.Background(), user, gamecoin.SpendParams{Currency: "gold", Amount: 30, Reason: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	req := r.f.only()
	checkRequest(t, req, http.MethodPost, "/api/v1/players/ext%3Auser-42/wallet/spend", `{"currency":"gold","amount":30,"reason":"shop"}`)
	if req.Header.Get("Idempotency-Key") == "" {
		t.Error("a spend carries an idempotency key")
	}
}

func TestLargeAmountsAreSentAsExactIntegers(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleMovement)
	if _, err := r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: "gems", Amount: 9_007_199_254_740_993}); err != nil {
		t.Fatal(err)
	}
	if body := r.f.only().Body; !strings.Contains(body, `"amount":9007199254740993`) {
		t.Errorf("body = %s", body)
	}
}

func TestLedgerListWithoutParamsHasNoQueryString(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"entries":[],"nextCursor":null}`)
	if _, err := r.client.Ledger.List(context.Background(), user, nil); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, r.f.only(), http.MethodGet, "/api/v1/players/ext%3Auser-42/ledger", "")
}

func TestLedgerListSerializesCurrencyLimitAndCursor(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"entries":[],"nextCursor":null}`).ok(`{"entries":[],"nextCursor":null}`).ok(`{"entries":[],"nextCursor":null}`)
	ctx := context.Background()
	if _, err := r.client.Ledger.List(ctx, user, &gamecoin.LedgerListParams{Currency: "gems", Limit: 50, Cursor: "a+b/c=d e"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.Ledger.List(ctx, user, &gamecoin.LedgerListParams{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.Ledger.List(ctx, user, &gamecoin.LedgerListParams{}); err != nil {
		t.Fatal(err)
	}
	reqs := r.f.requests()
	checkRequest(t, reqs[0], http.MethodGet, "/api/v1/players/ext%3Auser-42/ledger?currency=gems&limit=50&cursor=a%2Bb%2Fc%3Dd%20e", "")
	checkRequest(t, reqs[1], http.MethodGet, "/api/v1/players/ext%3Auser-42/ledger?limit=1", "")
	checkRequest(t, reqs[2], http.MethodGet, "/api/v1/players/ext%3Auser-42/ledger", "")
	// and the server decodes the cursor back to exactly what it gave
	values, err := url.ParseQuery(reqs[0].Query)
	if err != nil || values.Get("cursor") != "a+b/c=d e" {
		t.Errorf("cursor decodes to %q (%v)", values.Get("cursor"), err)
	}
}

func TestInventoryListGrantConsume(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"items":[]}`).ok(sampleChange).ok(sampleChange)
	ctx := context.Background()
	if _, err := r.client.Inventory.List(ctx, user); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.Inventory.Grant(ctx, user, gamecoin.InventoryParams{SKU: "potion", Quantity: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.Inventory.Consume(ctx, user, gamecoin.InventoryParams{SKU: "potion"}); err != nil {
		t.Fatal(err)
	}
	reqs := r.f.requests()
	checkRequest(t, reqs[0], http.MethodGet, "/api/v1/players/ext%3Auser-42/inventory", "")
	if reqs[0].Header.Get("Idempotency-Key") != "" {
		t.Error("a list carries no idempotency key")
	}
	checkRequest(t, reqs[1], http.MethodPost, "/api/v1/players/ext%3Auser-42/inventory/grant", `{"sku":"potion","quantity":3}`)
	checkRequest(t, reqs[2], http.MethodPost, "/api/v1/players/ext%3Auser-42/inventory/consume", `{"sku":"potion","quantity":1}`) // 0 means 1
	if reqs[1].Header.Get("Idempotency-Key") == "" || reqs[2].Header.Get("Idempotency-Key") == "" {
		t.Error("grant and consume carry an idempotency key")
	}
}

func TestPurchasesCreate(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePurchase)
	if _, err := r.client.Purchases.Create(context.Background(), user, gamecoin.PurchaseParams{SKU: "potion", Quantity: 2}); err != nil {
		t.Fatal(err)
	}
	req := r.f.only()
	checkRequest(t, req, http.MethodPost, "/api/v1/players/ext%3Auser-42/purchases", `{"sku":"potion","quantity":2}`)
	if req.Header.Get("Idempotency-Key") == "" {
		t.Error("a purchase carries an idempotency key")
	}
}

func TestCheckoutsCreateWithEveryField(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleCheckout)
	_, err := r.client.Checkouts.Create(context.Background(), user, gamecoin.CheckoutParams{
		PackSKU: "gems-500", SuccessURL: "https://game.test/ok", CancelURL: "https://game.test/ko", Country: "BE",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := r.f.only()
	checkRequest(t, req, http.MethodPost, "/api/v1/players/ext%3Auser-42/checkouts",
		`{"packSku":"gems-500","successUrl":"https://game.test/ok","cancelUrl":"https://game.test/ko","country":"BE"}`)
	if req.Header.Get("Idempotency-Key") == "" {
		t.Error("a checkout carries an idempotency key")
	}
}

func TestCheckoutsCreateWithOnlyThePack(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleCheckout)
	if _, err := r.client.Checkouts.Create(context.Background(), user, gamecoin.CheckoutParams{PackSKU: "starter"}); err != nil {
		t.Fatal(err)
	}
	checkRequest(t, r.f.only(), http.MethodPost, "/api/v1/players/ext%3Auser-42/checkouts", `{"packSku":"starter"}`)
}

func TestOrdersGetAndRefund(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleOrder).ok(sampleOrder).ok(sampleOrder)
	ctx := context.Background()
	if _, err := r.client.Orders.Get(ctx, "o1"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.Orders.Refund(ctx, "o1", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.Orders.Refund(ctx, "o 1/2", &gamecoin.RefundParams{Reason: "chargeback"}); err != nil {
		t.Fatal(err)
	}
	reqs := r.f.requests()
	checkRequest(t, reqs[0], http.MethodGet, "/api/v1/orders/o1", "")
	checkRequest(t, reqs[1], http.MethodPost, "/api/v1/orders/o1/refund", "{}")
	if reqs[1].Header.Get("Idempotency-Key") != "" {
		t.Error("a refund has no idempotency key")
	}
	checkRequest(t, reqs[2], http.MethodPost, "/api/v1/orders/o%201%2F2/refund", `{"reason":"chargeback"}`)
}

func TestRequestBodiesAreValidJSON(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleMovement)
	_, err := r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: "gems", Amount: 1, Reason: "café 日本 \U0001F3AE \\ \""})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(r.f.only().Body), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["reason"] != "café 日本 \U0001F3AE \\ \"" {
		t.Errorf("reason = %q", decoded["reason"])
	}
}
