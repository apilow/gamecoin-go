package gamecoin_test

import (
	"context"
	"errors"
	"testing"

	gamecoin "github.com/apilow/gamecoin-go"
)

func isArgError(err error) bool { return errors.Is(err, gamecoin.ErrInvalidArgument) }

// Bad arguments never cost a request.
func TestInvalidArgumentsAreRejectedBeforeAnyRequest(t *testing.T) {
	ctx := context.Background()
	none := gamecoin.PlayerRef("")
	cases := map[string]func(c *gamecoin.Client) error{
		"empty player on wallet.get":  func(c *gamecoin.Client) error { _, err := c.Wallet.Get(ctx, none); return err },
		"empty player on players.get": func(c *gamecoin.Client) error { _, err := c.Players.Get(ctx, none); return err },
		"empty player on upsert":      func(c *gamecoin.Client) error { _, err := c.Players.Upsert(ctx, none, nil); return err },
		"empty player on token":       func(c *gamecoin.Client) error { _, err := c.Players.CreateToken(ctx, none); return err },
		"empty player on ledger":      func(c *gamecoin.Client) error { _, err := c.Ledger.List(ctx, none, nil); return err },
		"empty player on inventory":   func(c *gamecoin.Client) error { _, err := c.Inventory.List(ctx, none); return err },
		"Ext of nothing":              func(c *gamecoin.Client) error { _, err := c.Wallet.Get(ctx, gamecoin.Ext("")); return err },
		"grant: amount 0":             func(c *gamecoin.Client) error { return grant(c, "gems", 0) },
		"grant: negative amount":      func(c *gamecoin.Client) error { return grant(c, "gems", -5) },
		"grant: empty currency":       func(c *gamecoin.Client) error { return grant(c, "", 5) },
		"grant: empty player": func(c *gamecoin.Client) error {
			_, err := c.Wallet.Grant(ctx, none, gamecoin.GrantParams{Currency: "gems", Amount: 1})
			return err
		},
		"spend: negative amount": func(c *gamecoin.Client) error {
			_, err := c.Wallet.Spend(ctx, user, gamecoin.SpendParams{Currency: "gems", Amount: -1})
			return err
		},
		"spend: empty currency": func(c *gamecoin.Client) error {
			_, err := c.Wallet.Spend(ctx, user, gamecoin.SpendParams{Amount: 1})
			return err
		},
		"inventory.grant: negative qty": func(c *gamecoin.Client) error {
			_, err := c.Inventory.Grant(ctx, user, gamecoin.InventoryParams{SKU: "potion", Quantity: -2})
			return err
		},
		"inventory.grant: empty sku": func(c *gamecoin.Client) error {
			_, err := c.Inventory.Grant(ctx, user, gamecoin.InventoryParams{})
			return err
		},
		"inventory.consume: negative qty": func(c *gamecoin.Client) error {
			_, err := c.Inventory.Consume(ctx, user, gamecoin.InventoryParams{SKU: "potion", Quantity: -1})
			return err
		},
		"inventory.consume: empty sku": func(c *gamecoin.Client) error {
			_, err := c.Inventory.Consume(ctx, user, gamecoin.InventoryParams{})
			return err
		},
		"purchase: negative quantity": func(c *gamecoin.Client) error {
			_, err := c.Purchases.Create(ctx, user, gamecoin.PurchaseParams{SKU: "potion", Quantity: -3})
			return err
		},
		"purchase: empty sku": func(c *gamecoin.Client) error {
			_, err := c.Purchases.Create(ctx, user, gamecoin.PurchaseParams{})
			return err
		},
		"checkout: empty pack": func(c *gamecoin.Client) error {
			_, err := c.Checkouts.Create(ctx, user, gamecoin.CheckoutParams{})
			return err
		},
		"checkout: empty player": func(c *gamecoin.Client) error {
			_, err := c.Checkouts.Create(ctx, none, gamecoin.CheckoutParams{PackSKU: "starter"})
			return err
		},
		"ledger: negative limit": func(c *gamecoin.Client) error {
			_, err := c.Ledger.List(ctx, user, &gamecoin.LedgerListParams{Limit: -3})
			return err
		},
		"codes.redeem: empty code":   func(c *gamecoin.Client) error { _, err := c.Codes.Redeem(ctx, user, ""); return err },
		"codes.redeem: empty player": func(c *gamecoin.Client) error { _, err := c.Codes.Redeem(ctx, none, "PGW-K7M2Q"); return err },
		"codes.redeem: bad idempotency key": func(c *gamecoin.Client) error {
			_, err := c.Codes.Redeem(ctx, user, "PGW-K7M2Q", gamecoin.WithIdempotencyKey("bad key\n"))
			return err
		},
		"orders.get: empty id": func(c *gamecoin.Client) error { _, err := c.Orders.Get(ctx, ""); return err },
		"ledger: limit 101": func(c *gamecoin.Client) error {
			_, err := c.Ledger.List(ctx, user, &gamecoin.LedgerListParams{Limit: 101})
			return err
		},
		"upsert: empty display name": func(c *gamecoin.Client) error {
			_, err := c.Players.Upsert(ctx, user, &gamecoin.PlayerProfile{DisplayName: ptr("")})
			return err
		},
		"upsert: empty email": func(c *gamecoin.Client) error {
			_, err := c.Players.Upsert(ctx, user, &gamecoin.PlayerProfile{Email: ptr("")})
			return err
		},
		"upsert: empty country": func(c *gamecoin.Client) error {
			_, err := c.Players.Upsert(ctx, user, &gamecoin.PlayerProfile{Country: ptr("")})
			return err
		},
		"upsert: birth year 0": func(c *gamecoin.Client) error {
			_, err := c.Players.Upsert(ctx, user, &gamecoin.PlayerProfile{BirthYear: ptr(0)})
			return err
		},
		"orders.refund: empty id": func(c *gamecoin.Client) error { _, err := c.Orders.Refund(ctx, "", nil); return err },
	}

	for name, call := range cases {
		r := newRig(t)
		err := call(r.client)
		if !isArgError(err) {
			t.Errorf("%s: err = %v, want ErrInvalidArgument", name, err)
		}
		var gcErr *gamecoin.Error
		if errors.As(err, &gcErr) {
			t.Errorf("%s: a local rejection must not be a *gamecoin.Error", name)
		}
		if n := len(r.f.requests()); n != 0 {
			t.Errorf("%s: %d requests were sent", name, n)
		}
	}
}

func grant(c *gamecoin.Client, currency string, amount int64) error {
	_, err := c.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: currency, Amount: amount})
	return err
}

func TestAZeroQuantityMeansOne(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePurchase)
	if _, err := r.client.Purchases.Create(context.Background(), user, gamecoin.PurchaseParams{SKU: "potion"}); err != nil {
		t.Fatal(err)
	}
	if body := r.f.only().Body; body != `{"sku":"potion","quantity":1}` {
		t.Errorf("body = %s", body)
	}
}
