package gamecoin_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	gamecoin "github.com/apilow/gamecoin-go"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// idempotentCall is one of the calls that carry an Idempotency-Key.
type idempotentCall struct {
	name   string
	okBody string
	call   func(r *rig, opts ...gamecoin.CallOption) error
}

func idempotentCalls() []idempotentCall {
	ctx := context.Background()
	return []idempotentCall{
		{"wallet.grant", sampleMovement, func(r *rig, opts ...gamecoin.CallOption) error {
			_, err := r.client.Wallet.Grant(ctx, user, gamecoin.GrantParams{Currency: "gems", Amount: 1}, opts...)
			return err
		}},
		{"wallet.spend", sampleMovement, func(r *rig, opts ...gamecoin.CallOption) error {
			_, err := r.client.Wallet.Spend(ctx, user, gamecoin.SpendParams{Currency: "gems", Amount: 1}, opts...)
			return err
		}},
		{"inventory.grant", sampleChange, func(r *rig, opts ...gamecoin.CallOption) error {
			_, err := r.client.Inventory.Grant(ctx, user, gamecoin.InventoryParams{SKU: "potion"}, opts...)
			return err
		}},
		{"inventory.consume", sampleChange, func(r *rig, opts ...gamecoin.CallOption) error {
			_, err := r.client.Inventory.Consume(ctx, user, gamecoin.InventoryParams{SKU: "potion"}, opts...)
			return err
		}},
		{"purchases.create", samplePurchase, func(r *rig, opts ...gamecoin.CallOption) error {
			_, err := r.client.Purchases.Create(ctx, user, gamecoin.PurchaseParams{SKU: "potion"}, opts...)
			return err
		}},
		{"checkouts.create", sampleCheckout, func(r *rig, opts ...gamecoin.CallOption) error {
			_, err := r.client.Checkouts.Create(ctx, user, gamecoin.CheckoutParams{PackSKU: "starter"}, opts...)
			return err
		}},
		{"codes.redeem", sampleRedemption, func(r *rig, opts ...gamecoin.CallOption) error {
			_, err := r.client.Codes.Redeem(ctx, user, "PGW-K7M2Q", opts...)
			return err
		}},
	}
}

func TestEveryIdempotentCallSendsAGeneratedUUIDv4Key(t *testing.T) {
	for _, tc := range idempotentCalls() {
		r := newRig(t)
		r.f.ok(tc.okBody)
		if err := tc.call(r); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if key := r.f.only().Header.Get("Idempotency-Key"); !uuidV4.MatchString(key) {
			t.Errorf("%s: Idempotency-Key = %q, want a UUID v4", tc.name, key)
		}
	}
}

func TestTheGeneratedKeyIsKeptAcrossRetriesAndNewForEachCall(t *testing.T) {
	for _, tc := range idempotentCalls() {
		r := newRig(t)
		r.f.apiError(503, "INTERNAL", "down", "").drop().ok(tc.okBody).ok(tc.okBody)

		if err := tc.call(r); err != nil { // three attempts
			t.Fatalf("%s: %v", tc.name, err)
		}
		if err := tc.call(r); err != nil { // a second, distinct call
			t.Fatalf("%s: %v", tc.name, err)
		}

		reqs := r.f.requests()
		if len(reqs) != 4 {
			t.Fatalf("%s: %d requests, want 4", tc.name, len(reqs))
		}
		first := reqs[0].Header.Get("Idempotency-Key")
		if !uuidV4.MatchString(first) {
			t.Errorf("%s: key = %q", tc.name, first)
		}
		for i := 1; i <= 2; i++ {
			if got := reqs[i].Header.Get("Idempotency-Key"); got != first {
				t.Errorf("%s: retry %d changed the key: %q != %q", tc.name, i, got, first)
			}
		}
		if second := reqs[3].Header.Get("Idempotency-Key"); second == first || !uuidV4.MatchString(second) {
			t.Errorf("%s: the second call reused the key (%q)", tc.name, second)
		}
	}
}

func TestTheCallerCanSupplyTheKeyAndItSurvivesRetries(t *testing.T) {
	for _, tc := range idempotentCalls() {
		r := newRig(t)
		r.f.apiError(502, "INTERNAL", "down", "").ok(tc.okBody)

		if err := tc.call(r, gamecoin.WithIdempotencyKey("my-own-key-001")); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		reqs := r.f.requests()
		if len(reqs) != 2 {
			t.Fatalf("%s: %d requests, want 2", tc.name, len(reqs))
		}
		for i, req := range reqs {
			if got := req.Header.Get("Idempotency-Key"); got != "my-own-key-001" {
				t.Errorf("%s: request %d key = %q", tc.name, i, got)
			}
		}
	}
}

func TestReadsNeverCarryAnIdempotencyKey(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleCatalog).ok(sampleOrder)
	ctx := context.Background()
	if _, err := r.client.Catalog.Get(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.client.Orders.Get(ctx, "o1"); err != nil {
		t.Fatal(err)
	}
	for _, req := range r.f.requests() {
		if req.Header.Get("Idempotency-Key") != "" {
			t.Errorf("%s %s carries an idempotency key", req.Method, req.RequestURI)
		}
	}
}

func TestAnInvalidCustomKeyIsRejectedBeforeAnyRequest(t *testing.T) {
	for _, key := range []string{"", "café", "tab\there", "line\nbreak", strings.Repeat("k", 101)} {
		r := newRig(t)
		_, err := r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: "gems", Amount: 1}, gamecoin.WithIdempotencyKey(key))
		if !isArgError(err) {
			t.Errorf("key %q: err = %v", key, err)
		}
		if n := len(r.f.requests()); n != 0 {
			t.Errorf("key %q: %d requests were sent", key, n)
		}
	}
}

func TestAKeyOf100CharactersIsAccepted(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleMovement)
	key := "with space ~!" + strings.Repeat("k", 87)
	if _, err := r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: "gems", Amount: 1}, gamecoin.WithIdempotencyKey(key)); err != nil {
		t.Fatal(err)
	}
	if got := r.f.only().Header.Get("Idempotency-Key"); got != key || len(got) != 100 {
		t.Errorf("key = %q", got)
	}
}

func TestANilCallOptionIsIgnored(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleMovement)
	if _, err := r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: "gems", Amount: 1}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestReplayedIsTrueOnlyWhenTheAPIsaysSo(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleMovement).ok(sampleMovement, "Idempotent-Replayed", "true").ok(sampleMovement, "Idempotent-Replayed", "false")
	ctx := context.Background()
	params := gamecoin.GrantParams{Currency: "gems", Amount: 1}

	fresh, err := r.client.Wallet.Grant(ctx, user, params)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := r.client.Wallet.Grant(ctx, user, params)
	if err != nil {
		t.Fatal(err)
	}
	other, err := r.client.Wallet.Grant(ctx, user, params)
	if err != nil {
		t.Fatal(err)
	}

	if fresh.Replayed || !replay.Replayed || other.Replayed {
		t.Errorf("Replayed = %v, %v, %v; want false, true, false", fresh.Replayed, replay.Replayed, other.Replayed)
	}
	if fresh.Entry.ID != replay.Entry.ID {
		t.Error("same body expected")
	}
}

func TestReplayedIsExposedByEachIdempotentResultType(t *testing.T) {
	r := newRig(t)
	replayed := []string{"Idempotent-Replayed", "true"}
	r.f.ok(sampleChange, replayed...).ok(sampleChange, replayed...).ok(samplePurchase, replayed...).ok(sampleCheckout, replayed...).ok(sampleRedemption, replayed...).ok(`{"items":[]}`)
	ctx := context.Background()

	granted, err := r.client.Inventory.Grant(ctx, user, gamecoin.InventoryParams{SKU: "potion"})
	if err != nil || !granted.Replayed {
		t.Errorf("Inventory.Grant: %+v, %v", granted, err)
	}
	consumed, err := r.client.Inventory.Consume(ctx, user, gamecoin.InventoryParams{SKU: "potion"})
	if err != nil || !consumed.Replayed {
		t.Errorf("Inventory.Consume: %+v, %v", consumed, err)
	}
	purchase, err := r.client.Purchases.Create(ctx, user, gamecoin.PurchaseParams{SKU: "potion"})
	if err != nil || !purchase.Replayed {
		t.Errorf("Purchases.Create: %+v, %v", purchase, err)
	}
	checkout, err := r.client.Checkouts.Create(ctx, user, gamecoin.CheckoutParams{PackSKU: "starter"})
	if err != nil || !checkout.Replayed {
		t.Errorf("Checkouts.Create: %+v, %v", checkout, err)
	}
	redemption, err := r.client.Codes.Redeem(ctx, user, "PGW-K7M2Q")
	if err != nil || !redemption.Replayed {
		t.Errorf("Codes.Redeem: %+v, %v", redemption, err)
	}
	if _, err := r.client.Inventory.List(ctx, user); err != nil {
		t.Fatal(err)
	}
}
