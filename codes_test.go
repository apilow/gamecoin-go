package gamecoin_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

// Version 0.3: gift codes, creator code on checkouts, pack availability, launch date.

func TestCodesRedeemSendsTheCodeAsItIs(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleRedemption)

	// Case, spaces and dashes are the API's business: the SDK does not normalise the code.
	if _, err := r.client.Codes.Redeem(context.Background(), user, " pgw k7m2q-XH4NP "); err != nil {
		t.Fatal(err)
	}

	req := r.f.only()
	checkRequest(t, req, http.MethodPost, "/api/v1/players/ext%3Auser-42/codes/redeem", `{"code":" pgw k7m2q-XH4NP "}`)
	if req.Header.Get("Idempotency-Key") == "" || req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("headers = %v", req.Header)
	}
}

func TestCodesRedeemParsesTheRedemption(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleRedemption)

	got, err := r.client.Codes.Redeem(context.Background(), user, "PGW-K7M2Q")
	if err != nil {
		t.Fatal(err)
	}

	want := &gamecoin.Redemption{
		Grants:  []gamecoin.RedemptionGrant{{Currency: "gems", Amount: 100, Balance: gamecoin.Balance{Currency: "gems", Paid: 0, Bonus: 100, Total: 100}}},
		Items:   []gamecoin.RedemptionItem{{SKU: "potion", Quantity: 2}},
		Skipped: []gamecoin.RedemptionItem{{SKU: "fire-sword", Quantity: 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("redemption = %+v, want %+v", got, want)
	}
}

func TestCodesRedeemOfACodeThatGivesOnlyItemsHasEmptyLists(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"grants":[],"items":[{"sku":"potion","quantity":1}],"skipped":[]}`)

	got, err := r.client.Codes.Redeem(context.Background(), gamecoin.PlayerID("p1"), "X")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Grants) != 0 || len(got.Items) != 1 || len(got.Skipped) != 0 || got.Replayed {
		t.Errorf("redemption = %+v", got)
	}
	checkRequest(t, r.f.only(), http.MethodPost, "/api/v1/players/p1/codes/redeem", `{"code":"X"}`)
}

func TestCodesRedeemCustomIdempotencyKeyAndReplay(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleRedemption, "Idempotent-Replayed", "true")

	got, err := r.client.Codes.Redeem(context.Background(), user, "PGW-K7M2Q", gamecoin.WithIdempotencyKey("redeem-user-42-pgw"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Replayed {
		t.Error("Replayed should be true")
	}
	if key := r.f.only().Header.Get("Idempotency-Key"); key != "redeem-user-42-pgw" {
		t.Errorf("Idempotency-Key = %q", key)
	}
}

func TestAnInvalidCodeIsVALIDATIONFAILEDWithCODEINVALIDAndIsNeverRetried(t *testing.T) {
	r := newRig(t)
	r.f.apiError(400, "VALIDATION_FAILED", "Invalid request", `{"fieldErrors":{"code":["CODE_INVALID"]}}`).
		apiError(400, "VALIDATION_FAILED", "Invalid request", `{"fieldErrors":{"code":["CODE_INVALID"]}}`)

	_, err := r.client.Codes.Redeem(context.Background(), user, "NOPE")

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodeValidationFailed || gcErr.Status != 400 {
		t.Fatalf("error = %+v", gcErr)
	}
	fieldErrors, _ := gcErr.Details["fieldErrors"].(map[string]any)
	if codes, _ := fieldErrors["code"].([]any); len(codes) != 1 || codes[0] != "CODE_INVALID" {
		t.Errorf("fieldErrors = %#v", gcErr.Details["fieldErrors"])
	}
	if n := len(r.f.requests()); n != 1 {
		t.Errorf("%d requests, a 400 is never retried", n)
	}
}

func TestCodesRedeemErrorsComeThroughTheUsualMapping(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   string
	}{
		{403, "PLAYER_BLOCKED", gamecoin.CodePlayerBlocked},
		{409, "LIMIT_REACHED", gamecoin.CodeLimitReached},
		{409, "CONFLICT", gamecoin.CodeConflict},
	} {
		r := newRig(t)
		r.f.apiError(tc.status, tc.code, "refused", "")

		_, err := r.client.Codes.Redeem(context.Background(), user, "PGW-K7M2Q")

		gcErr := asError(t, err)
		if gcErr.Code != tc.want || gcErr.Status != tc.status {
			t.Errorf("%s: error = %+v", tc.code, gcErr)
		}
		if n := len(r.f.requests()); n != 1 {
			t.Errorf("%s: %d requests", tc.code, n)
		}
	}
}

func TestCodesRedeemRateLimitedIsRetriedAndCarriesRetryAfter(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(1))
	r.f.apiError(429, "RATE_LIMITED", "Too many", "", "Retry-After", "7").
		apiError(429, "RATE_LIMITED", "Too many", "", "Retry-After", "40")

	_, err := r.client.Codes.Redeem(context.Background(), user, "PGW-K7M2Q")

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodeRateLimited || gcErr.RetryAfter != 40*time.Second {
		t.Errorf("error = %+v", gcErr)
	}
	if w := r.waits(); len(w) != 1 || w[0] != 7*time.Second {
		t.Errorf("waits = %v", w)
	}
	reqs := r.f.requests()
	if len(reqs) != 2 || reqs[0].Header.Get("Idempotency-Key") != reqs[1].Header.Get("Idempotency-Key") {
		t.Errorf("the key must be kept across the retry: %+v", reqs)
	}
}

func TestCheckoutsCreateSendsTheCreatorCode(t *testing.T) {
	r := newRig(t)
	r.f.ok(strings.Replace(sampleCheckout, `"creatorCode":null`, `"creatorCode":{"code":"LEAPLAY","creatorName":"Lea Play"}`, 1))

	c, err := r.client.Checkouts.Create(context.Background(), user, gamecoin.CheckoutParams{PackSKU: "gems-500", Locale: "fr", CreatorCode: "leaplay"})
	if err != nil {
		t.Fatal(err)
	}

	checkRequest(t, r.f.only(), http.MethodPost, "/api/v1/players/ext%3Auser-42/checkouts", `{"packSku":"gems-500","locale":"fr","creatorCode":"leaplay"}`)
	if want := (&gamecoin.OrderCreatorCode{Code: "LEAPLAY", CreatorName: "Lea Play"}); !reflect.DeepEqual(c.Order.CreatorCode, want) {
		t.Errorf("CreatorCode = %+v, want %+v", c.Order.CreatorCode, want)
	}
}

func TestCheckoutsCreateOmitsAnEmptyCreatorCode(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleCheckout)

	c, err := r.client.Checkouts.Create(context.Background(), user, gamecoin.CheckoutParams{PackSKU: "starter"})
	if err != nil {
		t.Fatal(err)
	}

	checkRequest(t, r.f.only(), http.MethodPost, "/api/v1/players/ext%3Auser-42/checkouts", `{"packSku":"starter"}`)
	if c.Order.CreatorCode != nil {
		t.Errorf("CreatorCode = %+v, want nil", c.Order.CreatorCode)
	}
}

func TestARefusedCreatorCodeIsVALIDATIONFAILEDWithCODEINVALID(t *testing.T) {
	r := newRig(t)
	r.f.apiError(400, "VALIDATION_FAILED", "Invalid request", `{"fieldErrors":{"creatorCode":["CODE_INVALID"]}}`)

	_, err := r.client.Checkouts.Create(context.Background(), user, gamecoin.CheckoutParams{PackSKU: "starter", CreatorCode: "NOPE"})

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodeValidationFailed || gcErr.Status != 400 {
		t.Fatalf("error = %+v", gcErr)
	}
	fieldErrors, _ := gcErr.Details["fieldErrors"].(map[string]any)
	if codes, _ := fieldErrors["creatorCode"].([]any); len(codes) != 1 || codes[0] != "CODE_INVALID" {
		t.Errorf("fieldErrors = %#v", gcErr.Details["fieldErrors"])
	}
	if n := len(r.f.requests()); n != 1 {
		t.Errorf("%d requests, a 400 is never retried", n)
	}
}

func TestPackNotAvailableExposesTheSaleWindow(t *testing.T) {
	r := newRig(t)
	r.f.apiError(409, "PACK_NOT_AVAILABLE", "Pack not available", `{"startsAt":"2026-12-01T09:00:00.000Z","endsAt":null}`)

	_, err := r.client.Checkouts.Create(context.Background(), user, gamecoin.CheckoutParams{PackSKU: "founder"})

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodePackNotAvailable || gcErr.Status != 409 || !gamecoin.IsCode(err, gamecoin.CodePackNotAvailable) {
		t.Fatalf("error = %+v", gcErr)
	}
	if startsAt, ok := gcErr.Details["startsAt"].(string); !ok || startsAt != "2026-12-01T09:00:00.000Z" {
		t.Errorf("startsAt = %#v", gcErr.Details["startsAt"])
	}
	if endsAt, present := gcErr.Details["endsAt"]; !present || endsAt != nil {
		t.Errorf("endsAt = %#v (present: %v), want a nil value", endsAt, present)
	}
	if n := len(r.f.requests()); n != 1 {
		t.Errorf("%d requests, a 409 is never retried", n)
	}
}

func TestPackSoldOut(t *testing.T) {
	r := newRig(t)
	r.f.apiError(409, "PACK_SOLD_OUT", "Pack sold out", `{"packSku":"founder"}`)

	_, err := r.client.Checkouts.Create(context.Background(), user, gamecoin.CheckoutParams{PackSKU: "founder"})

	var gcErr *gamecoin.Error
	if !errors.As(err, &gcErr) || gcErr.Code != gamecoin.CodePackSoldOut || gcErr.Status != 409 || gcErr.Details["packSku"] != "founder" {
		t.Errorf("error = %+v", gcErr)
	}
}

func TestCatalogExposesAvailabilityAndLaunchDate(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleCatalog)

	c, err := r.client.Catalog.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !c.Prelaunch || c.LaunchAt == nil || !c.LaunchAt.Equal(utc(2026, 12, 1, 9, 0, 0, 0)) || c.LaunchAt.Location() != time.UTC {
		t.Errorf("Prelaunch = %v, LaunchAt = %v", c.Prelaunch, c.LaunchAt)
	}
	a := c.Packs[0].Availability
	if a == nil || !a.Founder || a.StartsAt == nil || !a.StartsAt.Equal(utc(2026, 12, 1, 9, 0, 0, 0)) || a.EndsAt != nil || a.Remaining == nil || *a.Remaining != 42 {
		t.Errorf("Availability = %+v", a)
	}
}

func TestAnAvailabilityWithAnEndAndAnUnlimitedStock(t *testing.T) {
	r := newRig(t)
	body := strings.Replace(sampleCatalog,
		`"availability":{"startsAt":"2026-12-01T09:00:00.000Z","endsAt":null,"remaining":42,"founder":true}`,
		`"availability":{"startsAt":null,"endsAt":"2027-01-01T00:00:00.000Z","remaining":null,"founder":false}`, 1)
	r.f.ok(body)

	c, err := r.client.Catalog.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	a := c.Packs[0].Availability
	if a == nil || a.Founder || a.StartsAt != nil || a.Remaining != nil || a.EndsAt == nil || !a.EndsAt.Equal(utc(2027, 1, 1, 0, 0, 0, 0)) {
		t.Errorf("Availability = %+v", a)
	}
}

func TestAnOrdinaryPackHasNoAvailabilityAndTheGameIsNotInPrelaunch(t *testing.T) {
	r := newRig(t)
	body := strings.Replace(sampleCatalog,
		`,"availability":{"startsAt":"2026-12-01T09:00:00.000Z","endsAt":null,"remaining":42,"founder":true}`, `,"availability":null`, 1)
	body = strings.Replace(strings.Replace(body, `"prelaunch":true`, `"prelaunch":false`, 1), `"launchAt":"2026-12-01T09:00:00.000Z"`, `"launchAt":null`, 1)
	r.f.ok(body)

	c, err := r.client.Catalog.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if c.Packs[0].Availability != nil || c.Prelaunch || c.LaunchAt != nil {
		t.Errorf("Availability = %+v, Prelaunch = %v, LaunchAt = %v", c.Packs[0].Availability, c.Prelaunch, c.LaunchAt)
	}
}

func TestOrdersGetParsesTheCreatorCode(t *testing.T) {
	r := newRig(t)
	r.f.ok(strings.Replace(sampleOrder, `"creatorCode":null`, `"creatorCode":{"code":"LEAPLAY","creatorName":"Lea Play","commissionBp":1000}`, 1))

	o, err := r.client.Orders.Get(context.Background(), "o1")
	if err != nil {
		t.Fatal(err)
	}
	if o.CreatorCode == nil || o.CreatorCode.Code != "LEAPLAY" || o.CreatorCode.CreatorName != "Lea Play" { // an unknown field is ignored
		t.Errorf("CreatorCode = %+v", o.CreatorCode)
	}
}

func TestWebhookOrderEventsCarryTheCreatorCodeOrNull(t *testing.T) {
	with := strings.Replace(orderEventBody, `"creatorCode":null`, `"creatorCode":{"code":"LEAPLAY","creatorName":"Lea Play"}`, 1)
	if with == orderEventBody {
		t.Fatal("the fixture has no creatorCode")
	}

	event, err := construct(with, signed(with))
	if err != nil {
		t.Fatal(err)
	}
	if o := event.Order(); o == nil || o.CreatorCode == nil || o.CreatorCode.Code != "LEAPLAY" || o.CreatorCode.CreatorName != "Lea Play" {
		t.Errorf("order = %+v", o)
	}

	event, err = construct(orderEventBody, signed(orderEventBody))
	if err != nil {
		t.Fatal(err)
	}
	if o := event.Order(); o == nil || o.CreatorCode != nil {
		t.Errorf("order = %+v, want a nil CreatorCode", o)
	}
}
