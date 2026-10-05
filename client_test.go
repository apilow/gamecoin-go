package gamecoin_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

func TestSendsAuthenticationAcceptAndUserAgentHeaders(t *testing.T) {
	r := newRig(t)
	r.f.ok(sampleCatalog)

	if _, err := r.client.Catalog.Get(context.Background()); err != nil {
		t.Fatal(err)
	}

	req := r.f.only()
	if req.Method != http.MethodGet || req.RequestURI != "/api/v1/catalog" {
		t.Errorf("request = %s %s", req.Method, req.RequestURI)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer "+testKey {
		t.Errorf("Authorization header is wrong (length %d)", len(got))
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
	if got := req.Header.Get("User-Agent"); !regexp.MustCompile(`^gamecoin-go/0\.1\.0 \(go/[0-9][^ )]*\)$`).MatchString(got) {
		t.Errorf("User-Agent = %q", got)
	}
	if got := req.Header.Get("Idempotency-Key"); got != "" {
		t.Errorf("a read must not carry an idempotency key, got %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "" {
		t.Errorf("a read has no body, Content-Type = %q", got)
	}
	if gamecoin.Version != "0.1.0" {
		t.Errorf("Version = %q", gamecoin.Version)
	}
}

func TestEnvironmentIsReadFromTheKeyPrefix(t *testing.T) {
	for key, want := range map[string]string{"gc_sk_test_abc123": "test", "gc_sk_live_abc123": "live"} {
		c, err := gamecoin.New(key)
		if err != nil {
			t.Fatal(err)
		}
		if c.Environment() != want {
			t.Errorf("Environment() = %q, want %q", c.Environment(), want)
		}
	}
}

func TestDefaultBaseURLIsProduction(t *testing.T) {
	c, err := gamecoin.New("gc_sk_test_abc")
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL() != "https://gamecoin.apilow.com" || gamecoin.DefaultBaseURL != "https://gamecoin.apilow.com" {
		t.Errorf("BaseURL() = %q", c.BaseURL())
	}
}

func TestRejectsAPublishableKeyAndPointsToTheBrowserSDK(t *testing.T) {
	const publishable = "gc_pk_test_PUBLISHABLE0123456789"
	_, err := gamecoin.New(publishable)
	if err == nil {
		t.Fatal("a publishable key must be rejected")
	}
	if !errors.Is(err, gamecoin.ErrInvalidArgument) {
		t.Errorf("error does not wrap ErrInvalidArgument: %v", err)
	}
	if !strings.Contains(err.Error(), "browser SDK") {
		t.Errorf("message does not point to the browser SDK: %v", err)
	}
	if strings.Contains(err.Error(), publishable) {
		t.Errorf("the key is echoed in the message: %v", err)
	}
}

func TestRejectsAnythingThatIsNotASecretKey(t *testing.T) {
	for _, key := range []string{"", "   ", "gc_pt_player_token", "sk_test_nope", "gc_sk_test_", "gc_sk_staging_abc"} {
		_, err := gamecoin.New(key)
		if err == nil {
			t.Errorf("New(%q) should fail", key)
			continue
		}
		if !errors.Is(err, gamecoin.ErrInvalidArgument) || !strings.Contains(err.Error(), "secret key") {
			t.Errorf("New(%q) error = %v", key, err)
		}
		if len(key) > len("gc_sk_test_") && strings.Contains(err.Error(), key) {
			t.Errorf("the key is echoed in the message: %v", err)
		}
	}
}

func TestBaseURLIsNormalizedAndTheAPIPathAppended(t *testing.T) {
	for _, tc := range []struct{ suffix, wantPath string }{
		{"", "/api/v1/catalog"},
		{"/", "/api/v1/catalog"},
		{"///", "/api/v1/catalog"},
		{"/gateway/gamecoin/", "/gateway/gamecoin/api/v1/catalog"},
	} {
		f := newFake(t)
		f.ok(sampleCatalog)
		c, err := gamecoin.New(testKey, gamecoin.WithBaseURL(f.srv.URL+tc.suffix))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(c.BaseURL(), "/") {
			t.Errorf("BaseURL() = %q keeps a trailing slash", c.BaseURL())
		}
		if _, err := c.Catalog.Get(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := f.only().RequestURI; got != tc.wantPath {
			t.Errorf("base URL suffix %q: request target = %q, want %q", tc.suffix, got, tc.wantPath)
		}
	}
}

func TestRejectsAnInvalidBaseURL(t *testing.T) {
	for _, base := range []string{"", "gamecoin.apilow.com", "localhost:3400", "ftp://gamecoin.apilow.com", "https://", "https://x.test/?a=b", "https://x.test/#frag"} {
		if _, err := gamecoin.New("gc_sk_test_abc", gamecoin.WithBaseURL(base)); !errors.Is(err, gamecoin.ErrInvalidArgument) {
			t.Errorf("WithBaseURL(%q): err = %v", base, err)
		}
	}
}

func TestRejectsInvalidOptions(t *testing.T) {
	if _, err := gamecoin.New("gc_sk_test_abc", gamecoin.WithMaxRetries(-1)); !errors.Is(err, gamecoin.ErrInvalidArgument) {
		t.Errorf("WithMaxRetries(-1): err = %v", err)
	}
	if _, err := gamecoin.New("gc_sk_test_abc", gamecoin.WithTimeout(0)); !errors.Is(err, gamecoin.ErrInvalidArgument) {
		t.Errorf("WithTimeout(0): err = %v", err)
	}
	if _, err := gamecoin.New("gc_sk_test_abc", nil); err != nil {
		t.Errorf("a nil option should be ignored: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAnInjectedHTTPClientIsUsedAndLeftUntouched(t *testing.T) {
	var seen []string
	custom := &http.Client{
		Timeout: 7 * time.Second,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seen = append(seen, r.Method+" "+r.URL.String())
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       http.NoBody,
			}, nil
		}),
	}
	c, err := gamecoin.New(testKey, gamecoin.WithBaseURL("https://gc.example.test"), gamecoin.WithHTTPClient(custom), gamecoin.WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Catalog.Get(context.Background()) // an empty body is not valid JSON: INVALID_RESPONSE, but the transport was used
	var gcErr *gamecoin.Error
	if !errors.As(err, &gcErr) || gcErr.Code != gamecoin.CodeInvalidResponse {
		t.Fatalf("err = %v", err)
	}
	if len(seen) != 1 || seen[0] != "GET https://gc.example.test/api/v1/catalog" {
		t.Errorf("the injected transport saw %v", seen)
	}
	if custom.Timeout != 7*time.Second || custom.CheckRedirect != nil {
		t.Error("the SDK modified the injected client")
	}
}

func TestStringFormsNeverShowTheKey(t *testing.T) {
	r := newRig(t)
	c := r.client

	forms := map[string]string{
		"%v *Client":      fmt.Sprintf("%v", c),
		"%+v *Client":     fmt.Sprintf("%+v", c),
		"%#v *Client":     fmt.Sprintf("%#v", c),
		"%s *Client":      fmt.Sprintf("%s", c),
		"%q *Client":      fmt.Sprintf("%q", c),
		"String()":        c.String(),
		"%+v Client copy": fmt.Sprintf("%+v", *c),
		"%v Client copy":  fmt.Sprintf("%v", *c),
		"%#v Client copy": fmt.Sprintf("%#v", *c),
		"%+v Wallet":      fmt.Sprintf("%+v", c.Wallet),
		"%+v Wallet copy": fmt.Sprintf("%+v", *c.Wallet),
		"%#v Orders":      fmt.Sprintf("%#v", c.Orders),
		"%+v Ledger copy": fmt.Sprintf("%+v", *c.Ledger),
		"%+v Catalog":     fmt.Sprintf("%+v", c.Catalog),
		"%+v Players":     fmt.Sprintf("%+v", c.Players),
		"%+v Inventory":   fmt.Sprintf("%+v", c.Inventory),
		"%+v Purchases":   fmt.Sprintf("%+v", c.Purchases),
		"%+v Checkouts":   fmt.Sprintf("%+v", c.Checkouts),
	}
	for what, text := range forms {
		assertNoSecret(t, what, text)
	}
	if got := c.String(); !strings.Contains(got, "test") || !strings.Contains(got, r.f.srv.URL) || !strings.Contains(got, "gc_sk_test_…") {
		t.Errorf("String() = %q", got)
	}
}

func TestEveryResultAndErrorKeepsTheKeyOutOfItsStringForm(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.ok(sampleCatalog).ok(samplePlayer).ok(`{"token":"gc_pt_x","expiresAt":"2026-10-05T21:38:32.000Z"}`).
		ok(`{"playerId":"p","balances":[]}`).ok(sampleMovement).ok(`{"entries":[],"nextCursor":null}`).
		ok(sampleChange).ok(sampleCheckout).ok(sampleOrder).apiError(401, "UNAUTHENTICATED", "Invalid API key", "")
	ctx := context.Background()

	catalog, _ := r.client.Catalog.Get(ctx)
	player, _ := r.client.Players.Get(ctx, user)
	token, _ := r.client.Players.CreateToken(ctx, user)
	wallet, _ := r.client.Wallet.Get(ctx, user)
	movement, _ := r.client.Wallet.Grant(ctx, user, gamecoin.GrantParams{Currency: "gems", Amount: 1})
	page, _ := r.client.Ledger.List(ctx, user, nil)
	item, _ := r.client.Inventory.Grant(ctx, user, gamecoin.InventoryParams{SKU: "potion"})
	checkout, _ := r.client.Checkouts.Create(ctx, user, gamecoin.CheckoutParams{PackSKU: "starter"})
	order, _ := r.client.Orders.Refund(ctx, "o1", nil)
	_, err := r.client.Catalog.Get(ctx)

	for i, v := range []any{catalog, player, token, wallet, movement, page, item, checkout, order, err} {
		assertNoSecret(t, fmt.Sprintf("result %d (%%+v)", i), fmt.Sprintf("%+v", v))
		assertNoSecret(t, fmt.Sprintf("result %d (%%#v)", i), fmt.Sprintf("%#v", v))
	}
	if strings.Contains(fmt.Sprintf("%+v %v %#v", token, token, token), "gc_pt_x") {
		t.Error("the player token value appears in its string form")
	}
}

func TestAClientIsSafeForConcurrentUse(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 30; i++ {
		r.f.ok(sampleCatalog)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.client.Catalog.Get(context.Background()); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n := len(r.f.requests()); n != 30 {
		t.Errorf("the server saw %d requests, want 30", n)
	}
}

func TestPlayerRefHelpers(t *testing.T) {
	if got := gamecoin.Ext("user-42"); got != "ext:user-42" || got.String() != "ext:user-42" {
		t.Errorf("Ext = %q", got)
	}
	if got := gamecoin.PlayerID("6ac40ac0"); got != "6ac40ac0" {
		t.Errorf("PlayerID = %q", got)
	}
}

func TestErrorCodeConstantsMatchTheAPICodes(t *testing.T) {
	want := map[string]string{
		gamecoin.CodeValidationFailed:       "VALIDATION_FAILED",
		gamecoin.CodeIdempotencyKeyRequired: "IDEMPOTENCY_KEY_REQUIRED",
		gamecoin.CodeUnauthenticated:        "UNAUTHENTICATED",
		gamecoin.CodePaymentFailed:          "PAYMENT_FAILED",
		gamecoin.CodeForbidden:              "FORBIDDEN",
		gamecoin.CodePlayerBlocked:          "PLAYER_BLOCKED",
		gamecoin.CodeEnvNotEnabled:          "ENV_NOT_ENABLED",
		gamecoin.CodeNotFound:               "NOT_FOUND",
		gamecoin.CodeInsufficientFunds:      "INSUFFICIENT_FUNDS",
		gamecoin.CodeLimitReached:           "LIMIT_REACHED",
		gamecoin.CodeIdempotencyConflict:    "IDEMPOTENCY_CONFLICT",
		gamecoin.CodeConflict:               "CONFLICT",
		gamecoin.CodeReadOnly:               "READ_ONLY",
		gamecoin.CodeRateLimited:            "RATE_LIMITED",
		gamecoin.CodeInternal:               "INTERNAL",
		gamecoin.CodeNetworkError:           "NETWORK_ERROR",
		gamecoin.CodeTimeout:                "TIMEOUT",
		gamecoin.CodeInvalidResponse:        "INVALID_RESPONSE",
	}
	if len(want) != 18 {
		t.Fatalf("the table has %d entries (two constants share a value?)", len(want))
	}
	for got, expected := range want {
		if got != expected {
			t.Errorf("constant %q != %q", got, expected)
		}
	}
}
