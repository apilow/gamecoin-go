package gamecoin_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

// Every object of the API, field for field.

func utc(y int, m time.Month, d, h, min, s, ms int) time.Time {
	return time.Date(y, m, d, h, min, s, ms*int(time.Millisecond), time.UTC)
}

func TestPlayerIgnoresUnknownFieldsAndKeepsNulls(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePlayer)

	p, err := r.client.Players.Get(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}

	if p.ID != "6ac40ac02e9fe50981a1e085" || p.Kind != gamecoin.PlayerKindExternal || p.Blocked {
		t.Errorf("player = %+v", p)
	}
	if p.ExternalID == nil || *p.ExternalID != "probe-190" {
		t.Errorf("ExternalID = %v", p.ExternalID)
	}
	if p.DisplayName == nil || *p.DisplayName != "Probe" {
		t.Errorf("DisplayName = %v", p.DisplayName)
	}
	if p.Country != nil {
		t.Errorf("Country = %v, want nil", *p.Country)
	}
	if want := utc(2026, 10, 5, 20, 38, 24, 539); !p.CreatedAt.Equal(want) || p.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want %v", p.CreatedAt, want)
	}
}

func TestHostedPlayerHasNoExternalID(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"id":"p9","externalId":null,"kind":"hosted","displayName":null,"country":"FR","blocked":true,"createdAt":"2026-01-02T03:04:05.000Z"}`)

	p, err := r.client.Players.Get(context.Background(), gamecoin.PlayerID("p9"))
	if err != nil {
		t.Fatal(err)
	}
	if p.ExternalID != nil || p.DisplayName != nil || p.Kind != gamecoin.PlayerKindHosted || !p.Blocked {
		t.Errorf("player = %+v", p)
	}
	if p.Country == nil || *p.Country != "FR" {
		t.Errorf("Country = %v", p.Country)
	}
}

func TestPlayerToken(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"token":"gc_pt_abc","expiresAt":"2026-10-05T21:38:32.000Z"}`)

	tok, err := r.client.Players.CreateToken(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Token != "gc_pt_abc" || !tok.ExpiresAt.Equal(utc(2026, 10, 5, 21, 38, 32, 0)) {
		t.Errorf("token = %+v", tok)
	}
}

func TestWalletHasOneLinePerCurrency(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"playerId":"p1","balances":[{"currency":"gems","paid":40,"bonus":60,"total":100},{"currency":"gold","paid":0,"bonus":0,"total":0}]}`)

	w, err := r.client.Wallet.Get(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	want := &gamecoin.Wallet{PlayerID: "p1", Balances: []gamecoin.Balance{
		{Currency: "gems", Paid: 40, Bonus: 60, Total: 100},
		{Currency: "gold"},
	}}
	if !reflect.DeepEqual(w, want) {
		t.Errorf("wallet = %+v, want %+v", w, want)
	}
}

func TestMovementEntryAndBalance(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleMovement)

	m, err := r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: "gems", Amount: 100})
	if err != nil {
		t.Fatal(err)
	}

	if m.Balance != (gamecoin.Balance{Currency: "gems", Paid: 0, Bonus: 100, Total: 100}) {
		t.Errorf("balance = %+v", m.Balance)
	}
	e := m.Entry
	if e.ID != "6ac40ac312fe8b4190d46ffc" || e.Type != gamecoin.LedgerGrant || e.Currency != "gems" || e.PaidDelta != 0 || e.BonusDelta != 100 {
		t.Errorf("entry = %+v", e)
	}
	if e.BalanceAfter != (gamecoin.BalanceSnapshot{Paid: 0, Bonus: 100, Total: 100}) {
		t.Errorf("BalanceAfter = %+v", e.BalanceAfter)
	}
	if e.Reason == nil || *e.Reason != "level_up" {
		t.Errorf("Reason = %v", e.Reason)
	}
	if e.Metadata["a"] != "b" || len(e.Metadata) != 1 {
		t.Errorf("Metadata = %v", e.Metadata)
	}
	if e.Ref.OrderID != nil || e.Ref.ItemSKU != nil {
		t.Errorf("Ref = %+v", e.Ref)
	}
	if !e.CreatedAt.Equal(utc(2026, 10, 5, 20, 38, 27, 227)) {
		t.Errorf("CreatedAt = %v", e.CreatedAt)
	}
	if m.Replayed {
		t.Error("Replayed should be false")
	}
}

func TestLedgerPageWithRefsAndACursor(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"entries":[
	  {"id":"e2","type":"spend","currency":"gems","paidDelta":-20,"bonusDelta":-10,"balanceAfter":{"paid":0,"bonus":70,"total":70},"reason":null,"metadata":{},"ref":{"itemSku":"potion"},"createdAt":"2026-10-05T20:39:00.000Z"},
	  {"id":"e1","type":"purchase_credit","currency":"gems","paidDelta":500,"bonusDelta":50,"balanceAfter":{"paid":500,"bonus":50,"total":550},"reason":null,"metadata":{},"ref":{"orderId":"o1"},"createdAt":"2026-10-05T20:38:00.000Z"}
	],"nextCursor":"abc"}`)

	page, err := r.client.Ledger.List(context.Background(), user, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor == nil || *page.NextCursor != "abc" || len(page.Entries) != 2 {
		t.Fatalf("page = %+v", page)
	}
	first, second := page.Entries[0], page.Entries[1]
	if first.Ref.ItemSKU == nil || *first.Ref.ItemSKU != "potion" || first.Reason != nil || len(first.Metadata) != 0 {
		t.Errorf("first = %+v", first)
	}
	if first.PaidDelta != -20 || first.BonusDelta != -10 || first.Type != gamecoin.LedgerSpend {
		t.Errorf("first deltas = %+v", first)
	}
	if second.Ref.OrderID == nil || *second.Ref.OrderID != "o1" || second.Type != gamecoin.LedgerPurchaseCredit {
		t.Errorf("second = %+v", second)
	}
}

func TestLastLedgerPageHasANilCursor(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"entries":[],"nextCursor":null}`)

	page, err := r.client.Ledger.List(context.Background(), user, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != nil || len(page.Entries) != 0 {
		t.Errorf("page = %+v", page)
	}
}

func TestInventoryListAndChanges(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"items":[` + sampleItem + `,{"sku":"fire-sword","quantity":1,"updatedAt":"2026-10-05T20:41:00.000Z"}]}`).
		ok(sampleChange).
		ok(`{"item":{"sku":"potion","quantity":0,"updatedAt":"2026-10-05T20:42:00.000Z"}}`).
		ok(`{"items":[]}`)
	ctx := context.Background()

	items, err := r.client.Inventory.List(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	granted, err := r.client.Inventory.Grant(ctx, user, gamecoin.InventoryParams{SKU: "potion"})
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := r.client.Inventory.Consume(ctx, user, gamecoin.InventoryParams{SKU: "potion", Quantity: 3})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := r.client.Inventory.List(ctx, user)
	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 2 || items[0].SKU != "potion" || items[0].Quantity != 3 || items[1].SKU != "fire-sword" {
		t.Errorf("items = %+v", items)
	}
	if !items[0].UpdatedAt.Equal(utc(2026, 10, 5, 20, 40, 0, 0)) {
		t.Errorf("UpdatedAt = %v", items[0].UpdatedAt)
	}
	if granted.Quantity != 3 || consumed.Quantity != 0 { // returned even at 0
		t.Errorf("granted = %+v, consumed = %+v", granted, consumed)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("an empty inventory is an empty (non-nil) slice, got %#v", empty)
	}
}

func TestPurchaseHasEntryBalanceAndItem(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePurchase)

	p, err := r.client.Purchases.Create(context.Background(), user, gamecoin.PurchaseParams{SKU: "potion"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Entry.Type != gamecoin.LedgerGrant || p.Balance.Total != 100 || p.Item.SKU != "potion" || p.Item.Quantity != 3 || p.Replayed {
		t.Errorf("purchase = %+v", p)
	}
}

func TestPendingOrderAndCheckoutURL(t *testing.T) {
	r := newRig(t)
	r.f.reply(201, sampleCheckout)

	c, err := r.client.Checkouts.Create(context.Background(), user, gamecoin.CheckoutParams{PackSKU: "gems-500"})
	if err != nil {
		t.Fatal(err)
	}
	if c.CheckoutURL != "https://gc.example.test/pay/o1?t=x" {
		t.Errorf("CheckoutURL = %q", c.CheckoutURL)
	}
	o := c.Order
	if o.ID != "o1" || o.PlayerID != "p1" || o.Status != gamecoin.OrderPending || o.Pack != (gamecoin.OrderPack{SKU: "gems-500", Name: "Bag of gems"}) {
		t.Errorf("order = %+v", o)
	}
	if o.AmountCents != 499 || o.Currency != "EUR" || o.Country != "BE" || o.VatRateBp != 2100 || o.VatCents != 87 || o.NetCents != 412 {
		t.Errorf("order amounts = %+v", o)
	}
	if !o.CreatedAt.Equal(utc(2026, 10, 5, 20, 38, 24, 539)) || o.PaidAt != nil || o.FulfilledAt != nil || o.RefundedAt != nil {
		t.Errorf("order dates = %+v", o)
	}
}

func TestOrderDatesAreHonouredWithOffsets(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"id":"o1","playerId":"p1","status":"refunded","pack":{"sku":"s","name":"n"},"amountCents":1,"currency":"EUR","country":"BE","vatRateBp":0,"vatCents":0,"netCents":1,
	"createdAt":"2026-10-05T20:38:24.539Z","paidAt":"2026-10-05T20:40:00.000Z","fulfilledAt":"2026-10-05T20:40:01.000Z","refundedAt":"2026-10-05T20:50:00.000+02:00"}`)

	o, err := r.client.Orders.Get(context.Background(), "o1")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != gamecoin.OrderRefunded || o.PaidAt == nil || o.FulfilledAt == nil || o.RefundedAt == nil {
		t.Fatalf("order = %+v", o)
	}
	if !o.PaidAt.Equal(utc(2026, 10, 5, 20, 40, 0, 0)) || !o.FulfilledAt.Equal(utc(2026, 10, 5, 20, 40, 1, 0)) {
		t.Errorf("PaidAt = %v, FulfilledAt = %v", o.PaidAt, o.FulfilledAt)
	}
	if !o.RefundedAt.Equal(utc(2026, 10, 5, 18, 50, 0, 0)) {
		t.Errorf("RefundedAt = %v (offsets must be honoured)", o.RefundedAt)
	}
}

func TestCatalogCurrenciesItemsAndPacks(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleCatalog)

	c, err := r.client.Catalog.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(c.Currencies) != 2 || c.Currencies[0] != (gamecoin.Currency{Code: "gems", Name: "Gems", Purchasable: true}) || c.Currencies[1].Purchasable {
		t.Errorf("currencies = %+v", c.Currencies)
	}
	if len(c.Items) != 2 {
		t.Fatalf("items = %+v", c.Items)
	}
	potion, sword := c.Items[0], c.Items[1]
	if potion.SKU != "potion" || potion.Description != nil || potion.Type != gamecoin.ItemConsumable || !potion.ClientPurchasable || len(potion.Metadata) != 0 {
		t.Errorf("potion = %+v", potion)
	}
	if potion.Price == nil || *potion.Price != (gamecoin.Price{Currency: "gems", Amount: 20}) || potion.MaxOwned != nil {
		t.Errorf("potion price = %+v, max = %v", potion.Price, potion.MaxOwned)
	}
	if sword.Description == nil || *sword.Description != "Hot" || sword.Type != gamecoin.ItemDurable || sword.MaxOwned == nil || *sword.MaxOwned != 1 || sword.Metadata["tier"] != "3" {
		t.Errorf("sword = %+v", sword)
	}
	if len(c.Packs) != 1 {
		t.Fatalf("packs = %+v", c.Packs)
	}
	p := c.Packs[0]
	if p.SKU != "starter" || p.PriceCents != 99 || p.Currency != "EUR" || p.Badge == nil || *p.Badge != "new" || p.MaxPerPlayer == nil || *p.MaxPerPlayer != 1 {
		t.Errorf("pack = %+v", p)
	}
	if !reflect.DeepEqual(p.Grants, []gamecoin.PackGrant{{Currency: "gems", Amount: 100, Bonus: 20}}) || !reflect.DeepEqual(p.Items, []gamecoin.PackItem{{SKU: "potion", Quantity: 1}}) {
		t.Errorf("pack grants/items = %+v / %+v", p.Grants, p.Items)
	}
}

func TestItemWithoutAPriceAndPackWithoutALimit(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"currencies":[],"items":[{"sku":"gift","name":"Gift","description":null,"type":"durable","price":null,"maxOwned":null,"clientPurchasable":false,"metadata":{}}],"packs":[{"sku":"p","name":"P","description":null,"priceCents":1,"currency":"EUR","grants":[],"items":[],"badge":null,"maxPerPlayer":null}]}`)

	c, err := r.client.Catalog.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.Items[0].Price != nil || c.Items[0].ClientPurchasable || c.Packs[0].MaxPerPlayer != nil || c.Packs[0].Badge != nil || len(c.Packs[0].Grants) != 0 {
		t.Errorf("catalog = %+v", c)
	}
}

func TestUnknownEnumerationValuesDoNotBreakDecoding(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"id":"o1","playerId":"p1","status":"disputed_v2","pack":{"sku":"s","name":"n"},"amountCents":1,"currency":"EUR","country":"BE","vatRateBp":0,"vatCents":0,"netCents":1,"createdAt":"2026-10-05T20:38:24.539Z","paidAt":null,"fulfilledAt":null,"refundedAt":null}`)

	o, err := r.client.Orders.Get(context.Background(), "o1")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != "disputed_v2" {
		t.Errorf("Status = %q", o.Status)
	}
}

func TestFieldNamesAreMatchedExactly(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.ok(`{"PlayerID":"p1","BALANCES":[]}`)

	_, err := r.client.Wallet.Get(context.Background(), user)

	if gcErr := asError(t, err); gcErr.Code != gamecoin.CodeInvalidResponse {
		t.Errorf("error = %+v", gcErr)
	}
}
