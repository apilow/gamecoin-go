package gamecoin_test

import (
	"context"
	"net/url"
	"testing"

	gamecoin "github.com/apilow/gamecoin-go"
)

// The player reference is URL-encoded exactly once. The fake server is a real net/http server, so recorded.RequestURI is
// what really went on the wire.

func TestExtIDsAreURLEncodedExactlyOnce(t *testing.T) {
	for _, tc := range []struct{ id, segment string }{
		{"user 42", "ext%3Auser%2042"},
		{"a/b", "ext%3Aa%2Fb"},
		{"100%", "ext%3A100%25"},
		{"100%25", "ext%3A100%2525"},
		{"café über 日本", "ext%3Acaf%C3%A9%20%C3%BCber%20%E6%97%A5%E6%9C%AC"},
		{"a+b?c#d&e=f", "ext%3Aa%2Bb%3Fc%23d%26e%3Df"},
		{"../..", "ext%3A..%2F.."},
		{"\U0001F3AE", "ext%3A%F0%9F%8E%AE"},
		{"user-42_x.y~z", "ext%3Auser-42_x.y~z"},
	} {
		r := newRig(t)
		r.f.ok(samplePlayer)

		if _, err := r.client.Players.Get(context.Background(), gamecoin.Ext(tc.id)); err != nil {
			t.Fatalf("%q: %v", tc.id, err)
		}

		req := r.f.only()
		if want := "/api/v1/players/" + tc.segment; req.RequestURI != want {
			t.Errorf("Ext(%q): request target = %s, want %s", tc.id, req.RequestURI, want)
		}
		// and the server decodes it back to exactly what the game gave
		decoded, err := url.PathUnescape(req.RequestURI[len("/api/v1/players/"):])
		if err != nil || decoded != "ext:"+tc.id {
			t.Errorf("Ext(%q) decodes to %q (%v)", tc.id, decoded, err)
		}
	}
}

func TestExtIDsAreEncodedInEveryPlayerPath(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePlayer).ok(samplePlayer).ok(`{"token":"t","expiresAt":"2026-10-05T21:38:32.000Z"}`).
		ok(`{"playerId":"p","balances":[]}`).ok(sampleMovement).ok(sampleMovement).
		ok(`{"entries":[],"nextCursor":null}`).ok(`{"items":[]}`).
		ok(sampleChange).ok(sampleChange).ok(samplePurchase).ok(sampleCheckout)
	ctx := context.Background()
	player := gamecoin.Ext("a b/c%dé")

	calls := []func() error{
		func() error { _, err := r.client.Players.Upsert(ctx, player, nil); return err },
		func() error { _, err := r.client.Players.Get(ctx, player); return err },
		func() error { _, err := r.client.Players.CreateToken(ctx, player); return err },
		func() error { _, err := r.client.Wallet.Get(ctx, player); return err },
		func() error {
			_, err := r.client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gems", Amount: 1})
			return err
		},
		func() error {
			_, err := r.client.Wallet.Spend(ctx, player, gamecoin.SpendParams{Currency: "gems", Amount: 1})
			return err
		},
		func() error { _, err := r.client.Ledger.List(ctx, player, nil); return err },
		func() error { _, err := r.client.Inventory.List(ctx, player); return err },
		func() error {
			_, err := r.client.Inventory.Grant(ctx, player, gamecoin.InventoryParams{SKU: "potion"})
			return err
		},
		func() error {
			_, err := r.client.Inventory.Consume(ctx, player, gamecoin.InventoryParams{SKU: "potion"})
			return err
		},
		func() error {
			_, err := r.client.Purchases.Create(ctx, player, gamecoin.PurchaseParams{SKU: "potion"})
			return err
		},
		func() error {
			_, err := r.client.Checkouts.Create(ctx, player, gamecoin.CheckoutParams{PackSKU: "starter"})
			return err
		},
	}
	for i, call := range calls {
		if err := call(); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}

	reqs := r.f.requests()
	if len(reqs) != len(calls) {
		t.Fatalf("the server saw %d requests, want %d", len(reqs), len(calls))
	}
	const prefix = "/api/v1/players/ext%3Aa%20b%2Fc%25d%C3%A9"
	for i, req := range reqs {
		if len(req.RequestURI) < len(prefix) || req.RequestURI[:len(prefix)] != prefix {
			t.Errorf("call %d: request target = %s", i, req.RequestURI)
		}
	}
}

func TestPlayerIDsAreEncodedToo(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePlayer)
	if _, err := r.client.Players.Get(context.Background(), gamecoin.PlayerID("6ac40ac0")); err != nil {
		t.Fatal(err)
	}
	if got := r.f.only().RequestURI; got != "/api/v1/players/6ac40ac0" {
		t.Errorf("request target = %s", got)
	}
}

func TestAnUntypedStringConstantConvertsToAPlayer(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePlayer)
	if _, err := r.client.Players.Get(context.Background(), "6ac40ac0"); err != nil {
		t.Fatal(err)
	}
	if got := r.f.only().RequestURI; got != "/api/v1/players/6ac40ac0" {
		t.Errorf("request target = %s", got)
	}
}
