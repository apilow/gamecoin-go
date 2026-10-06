package gamecoin_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

const (
	hookSecret = "whsec_gc_unit_test_secret_0123456789"
	hookTime   = int64(1760000000) // 2025-10-09T08:53:20Z
)

var hookNow = time.Unix(hookTime, 0)

const (
	orderEventBody  = `{"id":"evt_6ac419fbdbe02af6c99c3d47_ful","type":"order.fulfilled","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{"id":"6ac419fbdbe02af6c99c3d47","playerId":"6ac419fa60e877541898f38d","status":"fulfilled","pack":{"sku":"gems-500","name":"Bag of gems"},"amountCents":499,"currency":"EUR","country":"BE","vatRateBp":2100,"vatCents":87,"netCents":412,"createdAt":"2026-10-06T10:00:00.000Z","paidAt":"2026-10-06T10:00:01.000Z","fulfilledAt":"2026-10-06T10:00:01.000Z","refundedAt":null}}`
	walletEventBody = `{"id":"evt_6ac419fbdbe02af6c99c3d47_cre","type":"wallet.credited","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{"playerId":"6ac419fa60e877541898f38d","orderId":"6ac419fbdbe02af6c99c3d47","credits":[{"currency":"gems","paid":500,"bonus":50}]}}`
	testEventBody   = `{"id":"evt_test_1","type":"webhook.test","createdAt":"2026-10-06T10:00:02.000Z","env":"live","data":{"message":"Hello from GameCoin"}}`
)

// sign computes the signature the way webhooks.md describes it: the lowercase hex HMAC-SHA256 of "<t>.<body>".
func sign(secret string, t int64, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", t, body)
	return hex.EncodeToString(mac.Sum(nil))
}

func header(t int64, signatures ...string) string {
	parts := []string{fmt.Sprintf("t=%d", t)}
	for _, s := range signatures {
		parts = append(parts, "v1="+s)
	}
	return strings.Join(parts, ",")
}

func construct(body, hdr string, opts ...gamecoin.WebhookOption) (*gamecoin.WebhookEvent, error) {
	opts = append([]gamecoin.WebhookOption{gamecoin.WithClock(func() time.Time { return hookNow })}, opts...)
	return gamecoin.ConstructEvent([]byte(body), hdr, hookSecret, opts...)
}

func signed(body string) string { return header(hookTime, sign(hookSecret, hookTime, body)) }

func wantReason(t *testing.T, err error, reason gamecoin.WebhookSignatureReason) {
	t.Helper()
	var sigErr *gamecoin.WebhookSignatureError
	if !errors.As(err, &sigErr) {
		t.Fatalf("error = %v (%T), want a *WebhookSignatureError", err, err)
	}
	if sigErr.Reason != reason {
		t.Errorf("Reason = %q, want %q", sigErr.Reason, reason)
	}
	var apiErr *gamecoin.Error
	if errors.As(err, &apiErr) {
		t.Error("a WebhookSignatureError must not be a *gamecoin.Error")
	}
	if strings.Contains(err.Error(), hookSecret) || strings.Contains(err.Error(), "whsec_") {
		t.Errorf("the secret leaked in the error: %q", err.Error())
	}
}

// An independent vector, computed with `openssl dgst -sha256 -hmac`, so the test does not only mirror the code.
func TestConstructEventWithAFixedVector(t *testing.T) {
	const body = `{"id":"evt_abc_ful","type":"order.fulfilled","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{"x":1}}`
	const sig = "a9bf9562a3559581cc5511ca96cda02bf0f9811c5393b2758fb6ef6a196a1a5b"
	if got := sign("whsec_gc_unit_test_secret", hookTime, body); got != sig {
		t.Fatalf("the test helper computes %s, the vector says %s", got, sig)
	}
	// "data" is not an Order here, so the payload is refused after the signature was accepted.
	_, err := gamecoin.ConstructEvent([]byte(body), header(hookTime, sig), "whsec_gc_unit_test_secret", gamecoin.WithClock(func() time.Time { return hookNow }))
	wantReason(t, err, gamecoin.ReasonInvalidPayload)
}

func TestConstructEventOrderFulfilled(t *testing.T) {
	event, err := construct(orderEventBody, signed(orderEventBody))
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != "evt_6ac419fbdbe02af6c99c3d47_ful" || event.Type != gamecoin.EventOrderFulfilled || event.Env != "test" {
		t.Errorf("envelope = %+v", event)
	}
	if want := time.Date(2026, 10, 6, 10, 0, 2, 0, time.UTC); !event.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v", event.CreatedAt)
	}
	order := event.Order()
	if order == nil || order.ID != "6ac419fbdbe02af6c99c3d47" || order.Status != gamecoin.OrderFulfilled || order.Pack.SKU != "gems-500" ||
		order.AmountCents != 499 || order.VatRateBp != 2100 || order.PaidAt == nil || order.RefundedAt != nil {
		t.Errorf("order = %+v", order)
	}
	if event.WalletCredited() != nil || event.Test() != nil {
		t.Error("the accessors of the other types must return nil")
	}
	if string(event.RawData) == "" || !json.Valid(event.RawData) {
		t.Errorf("RawData = %q", event.RawData)
	}
}

func TestConstructEventEveryOrderType(t *testing.T) {
	for _, kind := range []gamecoin.WebhookEventType{gamecoin.EventOrderFulfilled, gamecoin.EventOrderRefunded, gamecoin.EventOrderChargeback} {
		body := strings.Replace(orderEventBody, `"type":"order.fulfilled"`, `"type":"`+string(kind)+`"`, 1)
		event, err := construct(body, signed(body))
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if event.Type != kind || event.Order() == nil {
			t.Errorf("%s: event = %+v", kind, event)
		}
	}
}

func TestConstructEventWalletCredited(t *testing.T) {
	event, err := construct(walletEventBody, signed(walletEventBody))
	if err != nil {
		t.Fatal(err)
	}
	credited := event.WalletCredited()
	if event.Type != gamecoin.EventWalletCredited || credited == nil || credited.PlayerID != "6ac419fa60e877541898f38d" ||
		credited.OrderID != "6ac419fbdbe02af6c99c3d47" || len(credited.Credits) != 1 ||
		credited.Credits[0] != (gamecoin.WalletCredit{Currency: "gems", Paid: 500, Bonus: 50}) {
		t.Errorf("event = %+v, credited = %+v", event, credited)
	}
}

func TestConstructEventWebhookTest(t *testing.T) {
	event, err := construct(testEventBody, signed(testEventBody))
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != gamecoin.EventWebhookTest || event.Env != "live" || event.Test() == nil || event.Test().Message != "Hello from GameCoin" {
		t.Errorf("event = %+v", event)
	}
}

func TestConstructEventConstants(t *testing.T) {
	for got, want := range map[gamecoin.WebhookEventType]string{
		gamecoin.EventOrderFulfilled: "order.fulfilled", gamecoin.EventOrderRefunded: "order.refunded",
		gamecoin.EventOrderChargeback: "order.chargeback", gamecoin.EventWalletCredited: "wallet.credited",
		gamecoin.EventWebhookTest: "webhook.test",
	} {
		if string(got) != want {
			t.Errorf("%q != %q", got, want)
		}
	}
	for got, want := range map[gamecoin.WebhookSignatureReason]string{
		gamecoin.ReasonMissingHeader: "missing_header", gamecoin.ReasonMalformedHeader: "malformed_header",
		gamecoin.ReasonNoMatchingSignature: "no_matching_signature", gamecoin.ReasonTimestampOutOfTolerance: "timestamp_out_of_tolerance",
		gamecoin.ReasonInvalidPayload: "invalid_payload",
	} {
		if string(got) != want {
			t.Errorf("%q != %q", got, want)
		}
	}
	if gamecoin.WebhookSignatureHeader != "GameCoin-Signature" {
		t.Errorf("WebhookSignatureHeader = %q", gamecoin.WebhookSignatureHeader)
	}
}

func TestConstructEventUnknownTypeIsPassedThrough(t *testing.T) {
	body := `{"id":"evt_9","type":"order.something_new","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{"a":1,"nested":{"b":["c"]}}}`
	event, err := construct(body, signed(body))
	if err != nil {
		t.Fatal(err)
	}
	data, ok := event.Data.(map[string]any)
	if event.Type != "order.something_new" || !ok || data["a"] != json.Number("1") {
		t.Errorf("event = %+v", event)
	}
	if event.Order() != nil || event.WalletCredited() != nil || event.Test() != nil {
		t.Error("an unknown type has no typed data")
	}
	if string(event.RawData) != `{"a":1,"nested":{"b":["c"]}}` {
		t.Errorf("RawData = %s", event.RawData)
	}
}

func TestConstructEventSeveralSignatures(t *testing.T) {
	good := sign(hookSecret, hookTime, orderEventBody)
	wrong := sign("whsec_gc_other", hookTime, orderEventBody)
	for name, hdr := range map[string]string{
		"wrong then right": header(hookTime, wrong, good),
		"right then wrong": header(hookTime, good, wrong),
		"right then junk":  header(hookTime, good, "not-hex"),
	} {
		if _, err := construct(orderEventBody, hdr); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	_, err := construct(orderEventBody, header(hookTime, wrong, strings.Repeat("0", 64)))
	wantReason(t, err, gamecoin.ReasonNoMatchingSignature)
	_, err = construct(orderEventBody, header(hookTime, "zz-not-hex"))
	wantReason(t, err, gamecoin.ReasonNoMatchingSignature)
}

func TestConstructEventHeaderDetails(t *testing.T) {
	good := sign(hookSecret, hookTime, orderEventBody)
	for name, hdr := range map[string]string{
		"unknown keys are ignored":  fmt.Sprintf("v0=abc, t=%d , v1=%s, future=1", hookTime, good),
		"order does not matter":     fmt.Sprintf("v1=%s,t=%d", good, hookTime),
		"uppercase hex is the same": fmt.Sprintf("t=%d,v1=%s", hookTime, strings.ToUpper(good)),
	} {
		if _, err := construct(orderEventBody, hdr); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestConstructEventWrongSecret(t *testing.T) {
	_, err := gamecoin.ConstructEvent([]byte(orderEventBody), signed(orderEventBody), "whsec_gc_another", gamecoin.WithClock(func() time.Time { return hookNow }))
	wantReason(t, err, gamecoin.ReasonNoMatchingSignature)
}

func TestConstructEventTrimsTheSecret(t *testing.T) {
	_, err := gamecoin.ConstructEvent([]byte(orderEventBody), signed(orderEventBody), "  "+hookSecret+"\n", gamecoin.WithClock(func() time.Time { return hookNow }))
	if err != nil {
		t.Fatal(err)
	}
}

func TestConstructEventAlteredBody(t *testing.T) {
	hdr := signed(orderEventBody)
	altered := strings.Replace(orderEventBody, `"amountCents":499`, `"amountCents":498`, 1)
	_, err := construct(altered, hdr)
	wantReason(t, err, gamecoin.ReasonNoMatchingSignature)
	// A re-serialised body is not the raw body: the same JSON with another layout fails.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(orderEventBody), &parsed); err != nil {
		t.Fatal(err)
	}
	pretty, _ := json.MarshalIndent(parsed, "", "  ")
	_, err = construct(string(pretty), hdr)
	wantReason(t, err, gamecoin.ReasonNoMatchingSignature)
	_, err = construct(orderEventBody+"\n", hdr)
	wantReason(t, err, gamecoin.ReasonNoMatchingSignature)
}

func TestConstructEventTimestampTolerance(t *testing.T) {
	cases := map[string]struct {
		offset time.Duration // clock minus timestamp
		opts   []gamecoin.WebhookOption
		ok     bool
	}{
		"in the same second":                {0, nil, true},
		"4 minutes old":                     {4 * time.Minute, nil, true},
		"exactly 5 minutes old":             {5 * time.Minute, nil, true},
		"expired":                           {5*time.Minute + time.Second, nil, false},
		"a day old":                         {24 * time.Hour, nil, false},
		"4 minutes in the future":           {-4 * time.Minute, nil, true},
		"future":                            {-(5*time.Minute + time.Second), nil, false},
		"a day in the future":               {-24 * time.Hour, nil, false},
		"tolerance 0 accepts an old call":   {24 * 365 * time.Hour, []gamecoin.WebhookOption{gamecoin.WithTolerance(0)}, true},
		"tolerance 0 accepts a future call": {-24 * 365 * time.Hour, []gamecoin.WebhookOption{gamecoin.WithTolerance(0)}, true},
		"custom tolerance, inside":          {50 * time.Second, []gamecoin.WebhookOption{gamecoin.WithTolerance(time.Minute)}, true},
		"custom tolerance, outside":         {70 * time.Second, []gamecoin.WebhookOption{gamecoin.WithTolerance(time.Minute)}, false},
	}
	for name, c := range cases {
		opts := append([]gamecoin.WebhookOption{gamecoin.WithClock(func() time.Time { return hookNow.Add(c.offset) })}, c.opts...)
		_, err := gamecoin.ConstructEvent([]byte(orderEventBody), signed(orderEventBody), hookSecret, opts...)
		if c.ok && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if !c.ok {
			wantReason(t, err, gamecoin.ReasonTimestampOutOfTolerance)
		}
	}
}

func TestConstructEventUsesTheRealClockByDefault(t *testing.T) {
	now := time.Now().Unix()
	if _, err := gamecoin.ConstructEvent([]byte(orderEventBody), header(now, sign(hookSecret, now, orderEventBody)), hookSecret); err != nil {
		t.Fatal(err)
	}
	old := now - 3600
	_, err := gamecoin.ConstructEvent([]byte(orderEventBody), header(old, sign(hookSecret, old, orderEventBody)), hookSecret)
	wantReason(t, err, gamecoin.ReasonTimestampOutOfTolerance)
}

// The signature is checked before the timestamp: a forged call never learns whether its timestamp was acceptable.
func TestConstructEventChecksTheSignatureBeforeTheTimestamp(t *testing.T) {
	old := hookTime - 7200
	_, err := construct(orderEventBody, header(old, sign("whsec_gc_other", old, orderEventBody)))
	wantReason(t, err, gamecoin.ReasonNoMatchingSignature)
}

func TestConstructEventBytesAndTextBodies(t *testing.T) {
	const body = `{"id":"evt_é","type":"webhook.test","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{"message":"Café ☕ 日本語"}}`
	hdr := signed(body)
	fromText, err := construct(body, hdr)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(body)
	fromBytes, err := gamecoin.ConstructEvent(raw, hdr, hookSecret, gamecoin.WithClock(func() time.Time { return hookNow }))
	if err != nil {
		t.Fatal(err)
	}
	if fromText.Test().Message != "Café ☕ 日本語" || fromBytes.Test().Message != fromText.Test().Message || fromBytes.ID != "evt_é" {
		t.Errorf("text %+v, bytes %+v", fromText, fromBytes)
	}
	if string(raw) != body {
		t.Error("ConstructEvent must not modify the body")
	}
}

func TestConstructEventHeaderErrors(t *testing.T) {
	good := sign(hookSecret, hookTime, orderEventBody)
	missing := []string{"", "   "}
	for _, hdr := range missing {
		_, err := construct(orderEventBody, hdr)
		wantReason(t, err, gamecoin.ReasonMissingHeader)
	}
	malformed := map[string]string{
		"garbage":              "hello",
		"no timestamp":         "v1=" + good,
		"no signature":         fmt.Sprintf("t=%d", hookTime),
		"empty signature":      fmt.Sprintf("t=%d,v1=", hookTime),
		"timestamp not a int":  "t=abc,v1=" + good,
		"timestamp negative":   "t=-5,v1=" + good,
		"timestamp fractional": "t=1.5,v1=" + good,
		"timestamp empty":      "t=,v1=" + good,
		"timestamp overflow":   "t=99999999999999999999999,v1=" + good,
		"two timestamps":       fmt.Sprintf("t=%d,t=%d,v1=%s", hookTime, hookTime, good),
		"only unknown keys":    "v0=abc,foo=bar",
		"commas and nothing":   ",,,",
	}
	for name, hdr := range malformed {
		_, err := construct(orderEventBody, hdr)
		if err == nil {
			t.Errorf("%s: want an error", name)
			continue
		}
		wantReason(t, err, gamecoin.ReasonMalformedHeader)
	}
}

func TestConstructEventInvalidPayloads(t *testing.T) {
	for name, body := range map[string]string{
		"not JSON":                "this is not json",
		"empty body":              "",
		"truncated JSON":          `{"id":"evt_1","type":"webhook.test"`,
		"two documents":           testEventBody + testEventBody,
		"an array":                `[1,2,3]`,
		"a string":                `"hello"`,
		"null":                    `null`,
		"no id":                   `{"type":"webhook.test","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{"message":"x"}}`,
		"empty id":                `{"id":"","type":"webhook.test","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{"message":"x"}}`,
		"no type":                 `{"id":"e","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{"message":"x"}}`,
		"no createdAt":            `{"id":"e","type":"webhook.test","env":"test","data":{"message":"x"}}`,
		"bad createdAt":           `{"id":"e","type":"webhook.test","createdAt":"yesterday","env":"test","data":{"message":"x"}}`,
		"no env":                  `{"id":"e","type":"webhook.test","createdAt":"2026-10-06T10:00:02.000Z","data":{"message":"x"}}`,
		"no data":                 `{"id":"e","type":"webhook.test","createdAt":"2026-10-06T10:00:02.000Z","env":"test"}`,
		"data not an object":      `{"id":"e","type":"webhook.test","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":"x"}`,
		"order without a pack":    strings.Replace(orderEventBody, `"pack":{"sku":"gems-500","name":"Bag of gems"},`, "", 1),
		"order with a wrong type": strings.Replace(orderEventBody, `"amountCents":499`, `"amountCents":"499"`, 1),
		"wallet without credits":  `{"id":"e","type":"wallet.credited","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{"playerId":"p","orderId":"o"}}`,
		"test without message":    `{"id":"e","type":"webhook.test","createdAt":"2026-10-06T10:00:02.000Z","env":"test","data":{}}`,
	} {
		_, err := construct(body, signed(body))
		if err == nil {
			t.Errorf("%s: want an error", name)
			continue
		}
		wantReason(t, err, gamecoin.ReasonInvalidPayload)
		if body != "" && len(body) > 5 && strings.Contains(err.Error(), body) {
			t.Errorf("%s: the body leaked in the error", name)
		}
	}
}

func TestConstructEventArgumentErrors(t *testing.T) {
	_, err := gamecoin.ConstructEvent([]byte(orderEventBody), signed(orderEventBody), "")
	if !errors.Is(err, gamecoin.ErrInvalidArgument) {
		t.Errorf("empty secret: %v", err)
	}
	_, err = gamecoin.ConstructEvent([]byte(orderEventBody), signed(orderEventBody), " \t\n")
	if !errors.Is(err, gamecoin.ErrInvalidArgument) {
		t.Errorf("blank secret: %v", err)
	}
	_, err = gamecoin.ConstructEvent([]byte(orderEventBody), signed(orderEventBody), hookSecret, gamecoin.WithTolerance(-time.Second))
	if !errors.Is(err, gamecoin.ErrInvalidArgument) {
		t.Errorf("negative tolerance: %v", err)
	}
	var sigErr *gamecoin.WebhookSignatureError
	if errors.As(err, &sigErr) {
		t.Error("an argument error is not a signature error")
	}
}

func TestWebhookErrorsNeverContainTheSecretTheBodyOrTheSignature(t *testing.T) {
	good := sign(hookSecret, hookTime, orderEventBody)
	cases := []struct{ body, hdr string }{
		{orderEventBody, ""},
		{orderEventBody, "garbage"},
		{orderEventBody, header(hookTime, "00"+good[2:])},
		{"{ not json", signed("{ not json")},
		{orderEventBody, header(hookTime-86400, sign(hookSecret, hookTime-86400, orderEventBody))},
	}
	for _, c := range cases {
		_, err := construct(c.body, c.hdr)
		if err == nil {
			t.Fatal("want an error")
		}
		text := fmt.Sprintf("%v|%+v|%#v|%s", err, err, err, err.Error())
		for _, forbidden := range []string{hookSecret, "whsec_", good, "evt_6ac419fbdbe02af6c99c3d47", "Bag of gems"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%q leaked in %q", forbidden, text)
			}
		}
	}
}
