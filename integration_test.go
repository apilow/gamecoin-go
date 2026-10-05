package gamecoin_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

// The integration tests run against the sandbox game created by `npm run sdk:sandbox` in the GameCoin application:
// currencies gems and gold; items potion (consumable, 20 gems) and fire-sword (durable, 300 gems, one at most); packs
// starter and gems-500. They are skipped unless GAMECOIN_SECRET_KEY and GAMECOIN_BASE_URL are set.

// countingTransport counts the requests that reach the ledger, to prove that iteration really pages.
type countingTransport struct{ ledger atomic.Int64 }

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.HasSuffix(r.URL.Path, "/ledger") {
		c.ledger.Add(1)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func sandbox(t *testing.T, opts ...gamecoin.Option) *gamecoin.Client {
	t.Helper()
	key, base := os.Getenv("GAMECOIN_SECRET_KEY"), os.Getenv("GAMECOIN_BASE_URL")
	if key == "" || base == "" {
		t.Skip("set GAMECOIN_SECRET_KEY and GAMECOIN_BASE_URL to run the integration tests against the local sandbox")
	}
	client, err := gamecoin.New(key, append([]gamecoin.Option{gamecoin.WithBaseURL(base)}, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func uniqueID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

func expectCode(t *testing.T, step string, err error, code string, status int) *gamecoin.Error {
	t.Helper()
	var gcErr *gamecoin.Error
	if !errors.As(err, &gcErr) {
		t.Fatalf("%s: err = %v, want a *gamecoin.Error", step, err)
	}
	if gcErr.Code != code || gcErr.Status != status {
		t.Fatalf("%s: code %s status %d, want %s %d (%s)", step, gcErr.Code, gcErr.Status, code, status, gcErr.Message)
	}
	return gcErr
}

func mustNot(t *testing.T, step string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", step, err)
	}
}

func TestIntegrationFullFlowAgainstTheSandbox(t *testing.T) {
	counting := &countingTransport{}
	client := sandbox(t, gamecoin.WithHTTPClient(&http.Client{Transport: counting}))
	ctx := context.Background()
	player := gamecoin.Ext(uniqueID("sdk-go-"))
	if client.Environment() != "test" {
		t.Fatalf("Environment() = %s", client.Environment())
	}

	// catalog
	catalog, err := client.Catalog.Get(ctx)
	mustNot(t, "catalog", err)
	var gems, gold *gamecoin.Currency
	for i := range catalog.Currencies {
		switch catalog.Currencies[i].Code {
		case "gems":
			gems = &catalog.Currencies[i]
		case "gold":
			gold = &catalog.Currencies[i]
		}
	}
	if gems == nil || !gems.Purchasable || gold == nil || gold.Purchasable {
		t.Fatalf("currencies = %+v", catalog.Currencies)
	}
	items := map[string]gamecoin.Item{}
	for _, it := range catalog.Items {
		items[it.SKU] = it
	}
	if potion := items["potion"]; potion.Type != gamecoin.ItemConsumable || potion.Price == nil || *potion.Price != (gamecoin.Price{Currency: "gems", Amount: 20}) {
		t.Errorf("potion = %+v", potion)
	}
	if sword := items["fire-sword"]; sword.Type != gamecoin.ItemDurable || sword.Price == nil || sword.Price.Amount != 300 || sword.MaxOwned == nil || *sword.MaxOwned != 1 {
		t.Errorf("fire-sword = %+v", sword)
	}
	packs := map[string]gamecoin.Pack{}
	for _, p := range catalog.Packs {
		packs[p.SKU] = p
	}
	if _, ok := packs["starter"]; !ok {
		t.Error("the starter pack is missing")
	}
	if p, ok := packs["gems-500"]; !ok || p.Currency != "EUR" {
		t.Errorf("gems-500 = %+v", p)
	}

	// upsert a fresh ext: player
	name := "SDK Go test"
	created, err := client.Players.Upsert(ctx, player, &gamecoin.PlayerProfile{DisplayName: &name})
	mustNot(t, "upsert", err)
	if created.ExternalID == nil || "ext:"+*created.ExternalID != string(player) || created.Kind != gamecoin.PlayerKindExternal || created.DisplayName == nil || *created.DisplayName != name || created.Blocked {
		t.Errorf("created = %+v", created)
	}
	fetched, err := client.Players.Get(ctx, player)
	mustNot(t, "get player", err)
	byID, err := client.Players.Get(ctx, gamecoin.PlayerID(created.ID))
	mustNot(t, "get player by id", err)
	if fetched.ID != created.ID || byID.ID != created.ID {
		t.Errorf("ids differ: %s %s %s", created.ID, fetched.ID, byID.ID)
	}

	// grant 100 gems, then replay the same key
	key := uniqueID("sdk-go-grant-")
	params := gamecoin.GrantParams{Currency: "gems", Amount: 100, Reason: "level_up", Metadata: map[string]string{"level": "7"}}
	granted, err := client.Wallet.Grant(ctx, player, params, gamecoin.WithIdempotencyKey(key))
	mustNot(t, "grant", err)
	if granted.Replayed || granted.Balance.Total != 100 || granted.Balance.Bonus != 100 || granted.Balance.Paid != 0 {
		t.Errorf("granted = %+v", granted)
	}
	if granted.Entry.Type != gamecoin.LedgerGrant || granted.Entry.Reason == nil || *granted.Entry.Reason != "level_up" || granted.Entry.Metadata["level"] != "7" || granted.Entry.BonusDelta != 100 {
		t.Errorf("grant entry = %+v", granted.Entry)
	}
	replayed, err := client.Wallet.Grant(ctx, player, params, gamecoin.WithIdempotencyKey(key))
	mustNot(t, "replayed grant", err)
	if !replayed.Replayed || replayed.Entry.ID != granted.Entry.ID || replayed.Balance.Total != 100 {
		t.Errorf("replayed = %+v", replayed)
	}

	// the same key with another request is a conflict
	changed := params
	changed.Amount = 101
	_, err = client.Wallet.Grant(ctx, player, changed, gamecoin.WithIdempotencyKey(key))
	expectCode(t, "idempotency conflict", err, gamecoin.CodeIdempotencyConflict, 409)

	// spend 30
	spent, err := client.Wallet.Spend(ctx, player, gamecoin.SpendParams{Currency: "gems", Amount: 30, Reason: "shop"})
	mustNot(t, "spend", err)
	if spent.Replayed || spent.Balance.Total != 70 || spent.Entry.Type != gamecoin.LedgerSpend || spent.Entry.PaidDelta+spent.Entry.BonusDelta != -30 {
		t.Errorf("spent = %+v", spent)
	}

	// spend too much
	_, err = client.Wallet.Spend(ctx, player, gamecoin.SpendParams{Currency: "gems", Amount: 1000})
	broke := expectCode(t, "insufficient funds", err, gamecoin.CodeInsufficientFunds, 409)
	if required, ok := broke.DetailInt64("required"); !ok || required != 1000 {
		t.Errorf("required = %d, %v", required, ok)
	}
	if available, ok := broke.DetailInt64("available"); !ok || available != 70 {
		t.Errorf("available = %d, %v", available, ok)
	}

	// wallet
	wallet, err := client.Wallet.Get(ctx, player)
	mustNot(t, "wallet", err)
	totals := map[string]int64{}
	for _, b := range wallet.Balances {
		totals[b.Currency] = b.Total
	}
	if wallet.PlayerID != created.ID || totals["gems"] != 70 || totals["gold"] != 0 || len(totals) != 2 {
		t.Errorf("wallet = %+v", wallet)
	}

	// ledger iteration, one entry per page
	before := counting.ledger.Load()
	it := client.Ledger.Iterate(ctx, player, &gamecoin.LedgerListParams{Limit: 1})
	var types, ids []string
	for it.Next() {
		types = append(types, string(it.Entry().Type))
		ids = append(ids, it.Entry().ID)
	}
	mustNot(t, "ledger iteration", it.Err())
	if strings.Join(types, ",") != "spend,grant" || ids[0] != spent.Entry.ID || ids[1] != granted.Entry.ID { // newest first
		t.Errorf("ledger = %v %v", types, ids)
	}
	if got := counting.ledger.Load() - before; got < 2 {
		t.Errorf("iteration fetched %d pages, want several", got)
	}
	page, err := client.Ledger.List(ctx, player, &gamecoin.LedgerListParams{Currency: "gems", Limit: 1})
	mustNot(t, "ledger list", err)
	if len(page.Entries) != 1 || page.NextCursor == nil || *page.NextCursor == "" {
		t.Errorf("page = %+v", page)
	}
	goldPage, err := client.Ledger.List(ctx, player, &gamecoin.LedgerListParams{Currency: "gold"})
	mustNot(t, "gold ledger", err)
	if len(goldPage.Entries) != 0 || goldPage.NextCursor != nil {
		t.Errorf("goldPage = %+v", goldPage)
	}

	// player token
	token, err := client.Players.CreateToken(ctx, player)
	mustNot(t, "token", err)
	if !strings.HasPrefix(token.Token, "gc_pt_") || !token.ExpiresAt.After(time.Now()) {
		t.Errorf("token = %v", token)
	}

	// grant and consume a potion
	potionGrant, err := client.Inventory.Grant(ctx, player, gamecoin.InventoryParams{SKU: "potion", Quantity: 2})
	mustNot(t, "inventory grant", err)
	if potionGrant.SKU != "potion" || potionGrant.Quantity != 2 || potionGrant.Replayed {
		t.Errorf("potionGrant = %+v", potionGrant)
	}
	potionConsume, err := client.Inventory.Consume(ctx, player, gamecoin.InventoryParams{SKU: "potion"})
	mustNot(t, "inventory consume", err)
	if potionConsume.Quantity != 1 {
		t.Errorf("potionConsume = %+v", potionConsume)
	}
	inventory, err := client.Inventory.List(ctx, player)
	mustNot(t, "inventory list", err)
	if len(inventory) != 1 || inventory[0].SKU != "potion" || inventory[0].Quantity != 1 {
		t.Errorf("inventory = %+v", inventory)
	}

	// purchase a potion with gems
	purchase, err := client.Purchases.Create(ctx, player, gamecoin.PurchaseParams{SKU: "potion"})
	mustNot(t, "purchase", err)
	if purchase.Balance.Total != 50 || purchase.Item.Quantity != 2 || purchase.Entry.Type != gamecoin.LedgerSpend || purchase.Replayed {
		t.Errorf("purchase = %+v", purchase)
	}
	if purchase.Entry.Ref.ItemSKU == nil || *purchase.Entry.Ref.ItemSKU != "potion" {
		t.Errorf("purchase ref = %+v", purchase.Entry.Ref)
	}
	_, err = client.Purchases.Create(ctx, player, gamecoin.PurchaseParams{SKU: "fire-sword"})
	expectCode(t, "a sword the wallet cannot afford", err, gamecoin.CodeInsufficientFunds, 409)

	// checkout of gems-500
	checkout, err := client.Checkouts.Create(ctx, player, gamecoin.CheckoutParams{PackSKU: "gems-500", Country: "BE"})
	mustNot(t, "checkout", err)
	o := checkout.Order
	if o.Status != gamecoin.OrderPending || checkout.CheckoutURL == "" || !strings.Contains(checkout.CheckoutURL, "/pay/"+o.ID) || checkout.Replayed {
		t.Errorf("checkout = %+v", checkout)
	}
	if o.Pack.SKU != "gems-500" || o.PlayerID != created.ID || o.Currency != "EUR" || o.Country != "BE" || o.AmountCents != o.NetCents+o.VatCents || o.PaidAt != nil {
		t.Errorf("order = %+v", o)
	}

	// orders.get
	order, err := client.Orders.Get(ctx, o.ID)
	mustNot(t, "order", err)
	if order.ID != o.ID || order.Status != gamecoin.OrderPending || order.AmountCents != o.AmountCents {
		t.Errorf("order = %+v", order)
	}

	// refund of the unpaid order: CONFLICT
	_, err = client.Orders.Refund(ctx, o.ID, &gamecoin.RefundParams{Reason: "test"})
	expectCode(t, "refund of an unpaid order", err, gamecoin.CodeConflict, 409)

	// unknown order, unknown player
	_, err = client.Orders.Get(ctx, "000000000000000000000000")
	expectCode(t, "unknown order", err, gamecoin.CodeNotFound, 404)
	_, err = client.Players.Get(ctx, gamecoin.Ext(uniqueID("nobody-")))
	nobody := expectCode(t, "unknown player", err, gamecoin.CodeNotFound, 404)
	if strings.Contains(nobody.Error(), os.Getenv("GAMECOIN_SECRET_KEY")) {
		t.Error("the secret key leaked in an error message")
	}
}

func TestIntegrationAwkwardExternalIDsRoundTripThroughTheRealServer(t *testing.T) {
	client := sandbox(t)
	ctx := context.Background()
	suffix := uniqueID("-")

	for _, awkward := range []string{"name with spaces ", "slash/inside", "percent%25sign", "café 日本", "plus+and?query#hash&amp"} {
		externalID := awkward + suffix
		player := gamecoin.Ext(externalID)

		created, err := client.Players.Upsert(ctx, player, nil)
		mustNot(t, "upsert "+awkward, err)
		if created.ExternalID == nil || *created.ExternalID != externalID {
			t.Errorf("%q: ExternalID = %v", externalID, created.ExternalID)
		}
		granted, err := client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gold", Amount: 1})
		mustNot(t, "grant "+awkward, err)
		again, err := client.Players.Get(ctx, player)
		mustNot(t, "get "+awkward, err)
		if granted.Balance.Total != 1 || again.ID != created.ID {
			t.Errorf("%q: granted %+v, again %+v", externalID, granted, again)
		}
	}
}

func TestIntegrationNonASCIITextAndQuotesSurviveTheJSONRoundTrip(t *testing.T) {
	client := sandbox(t)
	ctx := context.Background()
	player := gamecoin.Ext(uniqueID("sdk-go-text-"))
	reason := "récompense 日本 \"quoted\" \\ and \U0001F3AE <b>&"

	granted, err := client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gold", Amount: 5, Reason: reason, Metadata: map[string]string{"note": "café ✓"}})
	mustNot(t, "grant", err)
	if granted.Entry.Reason == nil || *granted.Entry.Reason != reason || granted.Entry.Metadata["note"] != "café ✓" {
		t.Errorf("entry = %+v", granted.Entry)
	}
	page, err := client.Ledger.List(ctx, player, nil)
	mustNot(t, "ledger", err)
	if len(page.Entries) != 1 || page.Entries[0].Reason == nil || *page.Entries[0].Reason != reason {
		t.Errorf("page = %+v", page)
	}
}

func TestIntegrationAPlayerProfileCanBeSetThenCleared(t *testing.T) {
	client := sandbox(t)
	ctx := context.Background()
	player := gamecoin.Ext(uniqueID("sdk-go-profile-"))
	ann, be := "Ann", "BE"

	set, err := client.Players.Upsert(ctx, player, &gamecoin.PlayerProfile{DisplayName: &ann, Country: &be})
	mustNot(t, "set", err)
	if set.DisplayName == nil || *set.DisplayName != "Ann" || set.Country == nil || *set.Country != "BE" {
		t.Errorf("set = %+v", set)
	}
	unchanged, err := client.Players.Upsert(ctx, player, nil)
	mustNot(t, "unchanged", err)
	if unchanged.DisplayName == nil || *unchanged.DisplayName != "Ann" { // an empty profile changes nothing
		t.Errorf("unchanged = %+v", unchanged)
	}
	cleared, err := client.Players.Upsert(ctx, player, &gamecoin.PlayerProfile{Clear: []gamecoin.ProfileField{gamecoin.FieldDisplayName}})
	mustNot(t, "cleared", err)
	if cleared.DisplayName != nil || cleared.Country == nil || *cleared.Country != "BE" {
		t.Errorf("cleared = %+v", cleared)
	}
}

func TestIntegrationAWrongKeyIsUnauthenticated(t *testing.T) {
	base := os.Getenv("GAMECOIN_BASE_URL")
	if os.Getenv("GAMECOIN_SECRET_KEY") == "" || base == "" {
		t.Skip("set GAMECOIN_SECRET_KEY and GAMECOIN_BASE_URL to run the integration tests against the local sandbox")
	}
	client, err := gamecoin.New("gc_sk_test_definitelyNotARealKey", gamecoin.WithBaseURL(base))
	mustNot(t, "New", err)

	_, err = client.Catalog.Get(context.Background())

	expectCode(t, "wrong key", err, gamecoin.CodeUnauthenticated, 401)
}

func TestIntegrationServerSideValidationErrorsCarryFieldErrors(t *testing.T) {
	client := sandbox(t)

	// 99999999999999 is above the maximum amount of the API
	_, err := client.Wallet.Grant(context.Background(), gamecoin.Ext(uniqueID("sdk-go-v-")), gamecoin.GrantParams{Currency: "gems", Amount: 99_999_999_999_999})

	gcErr := expectCode(t, "amount too large", err, gamecoin.CodeValidationFailed, 400)
	if _, ok := gcErr.Details["fieldErrors"]; !ok {
		t.Errorf("Details = %v", gcErr.Details)
	}
}
