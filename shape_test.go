package gamecoin_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	gamecoin "github.com/apilow/gamecoin-go"
)

// A JSON body of an unexpected shape is INVALID_RESPONSE, whatever the call: the status is kept, no value is echoed.
func TestABodyOfTheWrongShapeIsINVALIDRESPONSEWithTheHTTPStatusAndThePath(t *testing.T) {
	ctx := context.Background()
	grant := func(r *rig) error {
		_, err := r.client.Wallet.Grant(ctx, user, gamecoin.GrantParams{Currency: "gems", Amount: 1})
		return err
	}
	spend := func(r *rig) error {
		_, err := r.client.Wallet.Spend(ctx, user, gamecoin.SpendParams{Currency: "gems", Amount: 1})
		return err
	}
	checkout := func(r *rig) error {
		_, err := r.client.Checkouts.Create(ctx, user, gamecoin.CheckoutParams{PackSKU: "starter"})
		return err
	}
	getPlayer := func(r *rig) error { _, err := r.client.Players.Get(ctx, user); return err }
	getOrder := func(r *rig) error { _, err := r.client.Orders.Get(ctx, "o1"); return err }
	getCatalog := func(r *rig) error { _, err := r.client.Catalog.Get(ctx); return err }
	entryWithout := func(old, replacement string) string {
		return `{"entry":` + strings.Replace(sampleEntry, old, replacement, 1) + `,"balance":` + sampleBal + `}`
	}

	for _, tc := range []struct {
		name string
		call func(r *rig) error
		body string
		path string
	}{
		{"player without id", getPlayer, strings.Replace(samplePlayer, `"id":"6ac40ac02e9fe50981a1e085",`, "", 1), "player.id"},
		{"player with a numeric id", getPlayer, strings.Replace(samplePlayer, `"6ac40ac02e9fe50981a1e085"`, "424242", 1), "player.id"},
		{"player without the nullable externalId", getPlayer, strings.Replace(samplePlayer, `"externalId":"probe-190",`, "", 1), "player.externalId"},
		{"player with a null kind", getPlayer, strings.Replace(samplePlayer, `"kind":"external"`, `"kind":null`, 1), "player.kind"},
		{"player with a text boolean", getPlayer, strings.Replace(samplePlayer, `"blocked":false`, `"blocked":"no"`, 1), "player.blocked"},
		{"player with a bad date", getPlayer, strings.Replace(samplePlayer, "2026-10-05T20:38:24.539Z", "yesterday", 1), "player.createdAt"},
		{"wallet without balances", func(r *rig) error { _, err := r.client.Wallet.Get(ctx, user); return err }, `{"playerId":"p"}`, "wallet.balances"},
		{"wallet with a fractional amount", func(r *rig) error { _, err := r.client.Wallet.Get(ctx, user); return err }, `{"playerId":"p","balances":[{"currency":"gems","paid":0,"bonus":1.5,"total":1}]}`, "wallet.balances[0].bonus"},
		{"token without expiry", func(r *rig) error { _, err := r.client.Players.CreateToken(ctx, user); return err }, `{"token":"gc_pt_x"}`, "response.expiresAt"},
		{"movement without balance", grant, `{"entry":` + sampleEntry + `}`, "response.balance"},
		{"movement entry without balanceAfter", grant, entryWithout(`"balanceAfter":{"paid":0,"bonus":100,"total":100},`, ""), "response.entry.balanceAfter"},
		{"entry with a null metadata", spend, entryWithout(`"metadata":{"a":"b"}`, `"metadata":null`), "response.entry.metadata"},
		{"entry with a non-string metadata value", spend, entryWithout(`"a":"b"`, `"a":7`), "response.entry.metadata.a"},
		{"entry with a null ref item", spend, entryWithout(`"ref":{}`, `"ref":{"itemSku":null}`), "response.entry.ref.itemSku"},
		{"entry without the nullable reason", spend, entryWithout(`"reason":"level_up",`, ""), "response.entry.reason"},
		{"ledger page without a cursor key", func(r *rig) error { _, err := r.client.Ledger.List(ctx, user, nil); return err }, `{"entries":[]}`, "response.nextCursor"},
		{"ledger page with a broken entry", func(r *rig) error { _, err := r.client.Ledger.List(ctx, user, nil); return err }, `{"entries":[` + sampleEntry + `,{"id":"x"}],"nextCursor":null}`, "response.entries[1].type"},
		{"inventory without items", func(r *rig) error { _, err := r.client.Inventory.List(ctx, user); return err }, `{}`, "response.items"},
		{"inventory change without item", func(r *rig) error {
			_, err := r.client.Inventory.Grant(ctx, user, gamecoin.InventoryParams{SKU: "potion"})
			return err
		}, `{"quantity":1}`, "response.item"},
		{"purchase without item", func(r *rig) error {
			_, err := r.client.Purchases.Create(ctx, user, gamecoin.PurchaseParams{SKU: "potion"})
			return err
		}, `{"entry":` + sampleEntry + `,"balance":` + sampleBal + `}`, "response.item"},
		{"checkout without url", checkout, `{"order":` + sampleOrder + `}`, "response.checkoutUrl"},
		{"checkout with a broken order", checkout, `{"order":{"id":"o"},"checkoutUrl":"u"}`, "response.order.playerId"},
		{"order without pack", getOrder, strings.Replace(sampleOrder, `"pack":{"sku":"gems-500","name":"Bag of gems"},`, "", 1), "order.pack"},
		{"order without paidAt", getOrder, strings.Replace(sampleOrder, `"paidAt":null,`, "", 1), "order.paidAt"},
		{"order with a text amount", func(r *rig) error { _, err := r.client.Orders.Refund(ctx, "o1", nil); return err }, strings.Replace(sampleOrder, `"amountCents":499`, `"amountCents":"499"`, 1), "order.amountCents"},
		{"catalog without packs", getCatalog, `{"currencies":[],"items":[]}`, "catalog.packs"},
		{"catalog item without type", getCatalog, strings.Replace(sampleCatalog, `"type":"consumable",`, "", 1), "catalog.items[0].type"},
		{"catalog pack with a bad grant", getCatalog, strings.Replace(sampleCatalog, `"amount":100,"bonus":20`, `"amount":100`, 1), "catalog.packs[0].grants[0].bonus"},
		{"catalog item with a broken price", getCatalog, strings.Replace(sampleCatalog, `"price":{"currency":"gems","amount":20}`, `"price":{"currency":"gems"}`, 1), "catalog.items[0].price.amount"},
	} {
		r := newRig(t, gamecoin.WithMaxRetries(0))
		r.f.ok(tc.body)

		err := tc.call(r)

		var gcErr *gamecoin.Error
		if !errors.As(err, &gcErr) {
			t.Errorf("%s: err = %v, want a *gamecoin.Error", tc.name, err)
			continue
		}
		if gcErr.Code != gamecoin.CodeInvalidResponse || gcErr.Status != 200 || !strings.Contains(gcErr.Message, tc.path+" must be") {
			t.Errorf("%s: error = %v, want INVALID_RESPONSE 200 mentioning %q", tc.name, gcErr, tc.path)
		}
		if n := len(r.f.requests()); n != 1 { // a malformed 200 is never retried
			t.Errorf("%s: %d requests", tc.name, n)
		}
	}
}

func TestTheMessageNeverEchoesTheOffendingValue(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.ok(strings.Replace(samplePlayer, `"6ac40ac02e9fe50981a1e085"`, "7777777777", 1))

	_, err := r.client.Players.Get(context.Background(), user)

	if gcErr := asError(t, err); strings.Contains(gcErr.Message, "7777777777") {
		t.Errorf("message = %q", gcErr.Message)
	}
}

func TestUnknownFieldsAndUnknownEnumerationValuesAreFine(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.ok(strings.Replace(strings.Replace(sampleOrder, `"status":"pending"`, `"status":"disputed_v2"`, 1), `{"id":"o1"`, `{"brandNew":[1,2],"id":"o1"`, 1))

	order, err := r.client.Orders.Get(context.Background(), "o1")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != "disputed_v2" {
		t.Errorf("Status = %q", order.Status)
	}
}

func TestAnOptionalLedgerRefFieldMayBeAbsent(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"entries":[` + strings.Replace(sampleEntry, `"ref":{}`, `"ref":{"orderId":"o1"}`, 1) + `],"nextCursor":null}`)

	page, err := r.client.Ledger.List(context.Background(), user, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ref := page.Entries[0].Ref; ref.OrderID == nil || *ref.OrderID != "o1" || ref.ItemSKU != nil {
		t.Errorf("ref = %+v", ref)
	}
}

func TestABodyThatIsNotJSONOrHasTrailingGarbageIsINVALIDRESPONSE(t *testing.T) {
	for _, body := range []string{`{"playerId":"p","balances":[]} trailing`, `{"playerId":"p","balances":[]}{}`, "not json"} {
		r := newRig(t, gamecoin.WithMaxRetries(0))
		r.f.ok(body)

		_, err := r.client.Wallet.Get(context.Background(), user)

		if gcErr := asError(t, err); gcErr.Code != gamecoin.CodeInvalidResponse || gcErr.Status != 200 {
			t.Errorf("body %q: error = %v", body, gcErr)
		}
	}
}

func TestA5xxHTMLPageIsRetriedButAMalformed200IsNot(t *testing.T) {
	r := newRig(t)
	r.f.raw(502, "text/html", "<html>gateway</html>").ok(`{"playerId":"p"}`)

	_, err := r.client.Wallet.Get(context.Background(), user)

	if gcErr := asError(t, err); len(r.f.requests()) != 2 || gcErr.Code != gamecoin.CodeInvalidResponse || gcErr.Status != 200 {
		t.Errorf("%d requests, error %v", len(r.f.requests()), gcErr)
	}
}

// --- configuration alignment -------------------------------------------------------------------------------------------

func TestTheSecretKeyIsTrimmed(t *testing.T) {
	f := newFake(t)
	f.ok(sampleCatalog)
	client, err := gamecoin.New("  gc_sk_test_abc123\r\n", gamecoin.WithBaseURL(f.srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Catalog.Get(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := f.only().Header.Get("Authorization"); got != "Bearer gc_sk_test_abc123" || client.Environment() != "test" {
		t.Errorf("Authorization length %d, environment %s", len(got), client.Environment())
	}
}

func TestTheClientShowsTheKeyPrefixOnly(t *testing.T) {
	client, err := gamecoin.New("gc_sk_live_abcdefghijkl")
	if err != nil {
		t.Fatal(err)
	}
	for _, form := range []string{client.String(), fmt.Sprintf("%v", client), fmt.Sprintf("%+v", client), fmt.Sprintf("%#v", client)} {
		if !strings.Contains(form, "gc_sk_live_…") || strings.Contains(form, "abcdefghijkl") {
			t.Errorf("form = %q", form)
		}
	}
}

func TestMaxRetriesIsBoundedFrom0To10(t *testing.T) {
	for n, accepted := range map[int]bool{0: true, 10: true, 11: false, -1: false} {
		_, err := gamecoin.New("gc_sk_test_abc", gamecoin.WithMaxRetries(n))
		if (err == nil) != accepted {
			t.Errorf("WithMaxRetries(%d): err = %v", n, err)
		}
	}
}

func TestTheBaseURLHasNoCredentialsQueryOrFragment(t *testing.T) {
	for _, base := range []string{"https://gc.example.test/?a=b", "https://gc.example.test/#frag", "https://user:pass@gc.example.test/", "https://user@gc.example.test/", "https://gc.example.test/?"} {
		_, err := gamecoin.New("gc_sk_test_abc", gamecoin.WithBaseURL(base))
		if !errors.Is(err, gamecoin.ErrInvalidArgument) {
			t.Errorf("WithBaseURL(%q): err = %v", base, err)
		} else if strings.Contains(err.Error(), "pass") {
			t.Errorf("the error echoes the credentials: %v", err)
		}
	}
}

func TestTheBaseURLIsReducedToTheOriginPlusAnOptionalPathPrefix(t *testing.T) {
	for in, want := range map[string]string{
		"https://gc.example.test/":         "https://gc.example.test",
		"http://localhost:3400/gateway//":  "http://localhost:3400/gateway",
		"  https://gc.example.test/a/b/  ": "https://gc.example.test/a/b",
		"https://gc.example.test///":       "https://gc.example.test",
	} {
		client, err := gamecoin.New("gc_sk_test_abc", gamecoin.WithBaseURL(in))
		if err != nil {
			t.Fatalf("WithBaseURL(%q): %v", in, err)
		}
		if client.BaseURL() != want {
			t.Errorf("WithBaseURL(%q): BaseURL() = %q, want %q", in, client.BaseURL(), want)
		}
	}
}

func TestMessagesNameTheArgumentNeverItsValue(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()

	_, amountErr := r.client.Wallet.Grant(ctx, user, gamecoin.GrantParams{Currency: "gems", Amount: -123456})
	_, limitErr := r.client.Ledger.List(ctx, user, &gamecoin.LedgerListParams{Limit: 987654})
	_, keyErr := r.client.Wallet.Grant(ctx, user, gamecoin.GrantParams{Currency: "gems", Amount: 1}, gamecoin.WithIdempotencyKey("tab\there-SECRETISH"))
	_, quantityErr := r.client.Purchases.Create(ctx, user, gamecoin.PurchaseParams{SKU: "potion", Quantity: -424242})
	_, fieldErr := r.client.Players.Upsert(ctx, user, &gamecoin.PlayerProfile{Clear: []gamecoin.ProfileField{"SECRETFIELD"}})

	for err, value := range map[error]string{amountErr: "123456", limitErr: "987654", keyErr: "SECRETISH", quantityErr: "424242", fieldErr: "SECRETFIELD"} {
		if !isArgError(err) || strings.Contains(err.Error(), value) {
			t.Errorf("err = %v (must be an argument error without %q)", err, value)
		}
	}
	if !strings.Contains(amountErr.Error(), "amount") || !strings.Contains(limitErr.Error(), "1 to 100") {
		t.Errorf("messages = %v / %v", amountErr, limitErr)
	}
}

func TestIteratingChecksItsArgumentsRightAwayAndSendsNothing(t *testing.T) {
	r := newRig(t)

	bad := r.client.Ledger.Iterate(context.Background(), user, &gamecoin.LedgerListParams{Limit: 0, Currency: "gems", Cursor: ""})
	if bad.Err() != nil {
		t.Errorf("valid arguments: Err = %v", bad.Err())
	}
	tooBig := r.client.Ledger.Iterate(context.Background(), user, &gamecoin.LedgerListParams{Limit: 101})
	noPlayer := r.client.Ledger.Iterate(context.Background(), "", nil)

	if !isArgError(tooBig.Err()) || !isArgError(noPlayer.Err()) {
		t.Errorf("Err = %v / %v, want argument errors before any Next", tooBig.Err(), noPlayer.Err())
	}
	if tooBig.Next() || noPlayer.Next() || len(r.f.requests()) != 0 {
		t.Errorf("%d requests were sent", len(r.f.requests()))
	}
}

func TestWhitespaceOnlyIDsAreTheServersBusinessOnlyEmptyOnesAreRejectedLocally(t *testing.T) {
	r := newRig(t)
	r.f.ok(samplePlayer)

	if _, err := r.client.Players.Get(context.Background(), gamecoin.Ext(" ")); err != nil {
		t.Fatal(err)
	}

	if got := r.f.only().RequestURI; got != "/api/v1/players/ext%3A%20" {
		t.Errorf("request target = %s", got)
	}
}
