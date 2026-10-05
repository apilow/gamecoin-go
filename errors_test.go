package gamecoin_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

func spendOnce(r *rig, amount int64) error {
	_, err := r.client.Wallet.Spend(context.Background(), user, gamecoin.SpendParams{Currency: "gems", Amount: amount})
	return err
}

func TestInsufficientFundsExposesRequiredAndAvailable(t *testing.T) {
	r := newRig(t)
	r.f.apiError(409, "INSUFFICIENT_FUNDS", "Insufficient funds", `{"required":50,"available":20}`)

	err := spendOnce(r, 50)

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodeInsufficientFunds || gcErr.Status != 409 || gcErr.Message != "Insufficient funds" {
		t.Errorf("error = %+v", gcErr)
	}
	if required, ok := gcErr.DetailInt64("required"); !ok || required != 50 {
		t.Errorf("required = %d, %v", required, ok)
	}
	if available, ok := gcErr.DetailInt64("available"); !ok || available != 20 {
		t.Errorf("available = %d, %v", available, ok)
	}
	if gcErr.RetryAfter != 0 || errors.Unwrap(gcErr) != nil {
		t.Errorf("RetryAfter = %v, cause = %v", gcErr.RetryAfter, errors.Unwrap(gcErr))
	}
}

func TestDetailInt64KeepsLargeIntegersExact(t *testing.T) {
	r := newRig(t)
	r.f.apiError(409, "INSUFFICIENT_FUNDS", "x", `{"required":9007199254740993,"available":0,"name":"text"}`)

	gcErr := asError(t, spendOnce(r, 1))

	if v, ok := gcErr.DetailInt64("required"); !ok || v != 9007199254740993 {
		t.Errorf("required = %d, %v", v, ok)
	}
	if _, ok := gcErr.DetailInt64("name"); ok {
		t.Error("a string is not an integer")
	}
	if _, ok := gcErr.DetailInt64("missing"); ok {
		t.Error("an absent key is not an integer")
	}
}

func TestValidationFailuresExposeFieldErrors(t *testing.T) {
	r := newRig(t)
	r.f.apiError(400, "VALIDATION_FAILED", "Invalid request", `{"fieldErrors":{"amount":["Too small"],"_":["Unrecognized field"]}}`)

	gcErr := asError(t, spendOnce(r, 1))

	if gcErr.Code != gamecoin.CodeValidationFailed || gcErr.Status != 400 {
		t.Errorf("error = %+v", gcErr)
	}
	fieldErrors, ok := gcErr.Details["fieldErrors"].(map[string]any)
	if !ok {
		t.Fatalf("fieldErrors = %#v", gcErr.Details["fieldErrors"])
	}
	if amount, _ := fieldErrors["amount"].([]any); len(amount) != 1 || amount[0] != "Too small" {
		t.Errorf("fieldErrors[amount] = %#v", fieldErrors["amount"])
	}
}

func TestDetailsAreEmptyWhenTheAPISendsNone(t *testing.T) {
	r := newRig(t)
	r.f.apiError(404, "NOT_FOUND", "Player not found", "")

	_, err := r.client.Players.Get(context.Background(), user)

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodeNotFound || gcErr.Status != 404 {
		t.Errorf("error = %+v", gcErr)
	}
	if gcErr.Details == nil || len(gcErr.Details) != 0 {
		t.Errorf("Details = %#v, want an empty map", gcErr.Details)
	}
}

func TestRateLimitedCarriesRetryAfter(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.apiError(429, "RATE_LIMITED", "Too many requests", "", "Retry-After", "12")

	_, err := r.client.Catalog.Get(context.Background())

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodeRateLimited || gcErr.Status != 429 || gcErr.RetryAfter != 12*time.Second {
		t.Errorf("error = %+v", gcErr)
	}
}

func TestRetryAfterIsKeptWhenTheRetriesAreExhausted(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(1))
	r.f.apiError(429, "RATE_LIMITED", "x", "", "Retry-After", "1").apiError(429, "RATE_LIMITED", "x", "", "Retry-After", "2")

	_, err := r.client.Catalog.Get(context.Background())

	if gcErr := asError(t, err); gcErr.RetryAfter != 2*time.Second {
		t.Errorf("RetryAfter = %v", gcErr.RetryAfter)
	}
}

func TestAMalformedRetryAfterIsIgnored(t *testing.T) {
	for _, value := range []string{"-5", "soon"} {
		r := newRig(t, gamecoin.WithMaxRetries(0))
		r.f.apiError(429, "RATE_LIMITED", "x", "", "Retry-After", value)

		_, err := r.client.Catalog.Get(context.Background())

		if gcErr := asError(t, err); gcErr.RetryAfter != 0 {
			t.Errorf("Retry-After %q: RetryAfter = %v", value, gcErr.RetryAfter)
		}
	}
}

func TestANetworkFailureHasStatusZeroAndKeepsTheCause(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.drop()

	_, err := r.client.Catalog.Get(context.Background())

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodeNetworkError || gcErr.Status != 0 {
		t.Errorf("error = %+v", gcErr)
	}
	if errors.Unwrap(gcErr) == nil || !strings.Contains(gcErr.Message, "Network error") || len(gcErr.Details) != 0 {
		t.Errorf("cause = %v, message = %q", errors.Unwrap(gcErr), gcErr.Message)
	}
	assertNoSecret(t, "network error message", gcErr.Error())
}

func TestATimeoutHasStatusZero(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0), gamecoin.WithTimeout(50*time.Millisecond))
	r.f.hang()

	_, err := r.client.Catalog.Get(context.Background())

	gcErr := asError(t, err)
	if gcErr.Code != "TIMEOUT" || gcErr.Status != 0 || !strings.Contains(gcErr.Message, "No response within") {
		t.Errorf("error = %+v", gcErr)
	}
}

func TestASuccessBodyThatIsNotTheExpectedJSONIsINVALIDRESPONSE(t *testing.T) {
	for _, body := range []string{
		"<html>Welcome to a captive portal</html>", "not json at all", "", "[1,2,3]", "null", `{"entry":"this should be an object","balance":1}`,
	} {
		r := newRig(t, gamecoin.WithMaxRetries(0))
		r.f.ok(body)

		_, err := r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: "gems", Amount: 1})

		gcErr := asError(t, err)
		if gcErr.Code != gamecoin.CodeInvalidResponse || gcErr.Status != 200 {
			t.Errorf("body %q: error = %+v", body, gcErr)
		}
		if n := len(r.f.requests()); n != 1 { // a 2xx is never retried
			t.Errorf("body %q: %d requests", body, n)
		}
	}
}

func TestAnErrorStatusWithAnUnexpectedBodyIsINVALIDRESPONSEWithTheHTTPStatus(t *testing.T) {
	for _, body := range []string{"<html>oops</html>", `{"message":"not our format"}`, `{"error":"a string"}`, `{"error":{"message":"no code"}}`, ""} {
		r := newRig(t, gamecoin.WithMaxRetries(0))
		r.f.raw(400, "application/json", body)

		_, err := r.client.Catalog.Get(context.Background())

		if gcErr := asError(t, err); gcErr.Code != gamecoin.CodeInvalidResponse || gcErr.Status != 400 {
			t.Errorf("body %q: error = %+v", body, gcErr)
		}
	}
}

func TestAnErrorWithoutAMessageIsNotAGameCoinErrorBody(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.raw(409, "application/json", `{"error":{"code":"CONFLICT"}}`)

	_, err := r.client.Catalog.Get(context.Background())

	if gcErr := asError(t, err); gcErr.Code != gamecoin.CodeInvalidResponse || gcErr.Status != 409 {
		t.Errorf("error = %+v", gcErr)
	}
}

func TestAnUnknownFutureErrorCodeIsPassedThrough(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.apiError(409, "SOMETHING_NEW", "A new rule", "")

	_, err := r.client.Catalog.Get(context.Background())

	if gcErr := asError(t, err); gcErr.Code != "SOMETHING_NEW" || gcErr.Message != "A new rule" {
		t.Errorf("error = %+v", gcErr)
	}
}

func TestARedirectIsReportedWithAHintNotFollowed(t *testing.T) {
	// with the default HTTP client of the SDK: no WithHTTPClient here
	f := newFake(t)
	f.raw(301, "text/plain", "", "Location", "https://elsewhere.invalid/")
	c, err := gamecoin.New(testKey, gamecoin.WithBaseURL(f.srv.URL), gamecoin.WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Catalog.Get(context.Background())

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodeInvalidResponse || gcErr.Status != 301 || !strings.Contains(gcErr.Message, "base URL") {
		t.Errorf("error = %+v", gcErr)
	}
	if n := len(f.requests()); n != 1 {
		t.Errorf("%d requests: the redirect must not be followed", n)
	}
}

func TestInternalErrorsAreReturned(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.apiError(500, "INTERNAL", "Internal error", "")

	_, err := r.client.Catalog.Get(context.Background())

	if gcErr := asError(t, err); gcErr.Code != gamecoin.CodeInternal || gcErr.Status != 500 {
		t.Errorf("error = %+v", gcErr)
	}
}

func TestErrorStringAndHelpers(t *testing.T) {
	api := &gamecoin.Error{Code: gamecoin.CodeNotFound, Status: 404, Message: "Player not found"}
	if got := api.Error(); got != "gamecoin: NOT_FOUND (HTTP 404): Player not found" {
		t.Errorf("Error() = %q", got)
	}
	network := &gamecoin.Error{Code: gamecoin.CodeNetworkError, Message: "Network error: boom"}
	if got := network.Error(); got != "gamecoin: NETWORK_ERROR: Network error: boom" {
		t.Errorf("Error() = %q", got)
	}

	wrapped := fmt.Errorf("granting the daily bonus: %w", api)
	if !gamecoin.IsCode(wrapped, gamecoin.CodeNotFound) || gamecoin.IsCode(wrapped, gamecoin.CodeConflict) || gamecoin.IsCode(errors.New("plain"), gamecoin.CodeNotFound) {
		t.Error("IsCode misbehaves")
	}
	var target *gamecoin.Error
	if !errors.As(wrapped, &target) || target != api {
		t.Error("errors.As should find the *Error through a wrap")
	}
}

func TestErrorsReadNaturallyWithErrorsAs(t *testing.T) {
	r := newRig(t)
	r.f.apiError(409, "INSUFFICIENT_FUNDS", "Insufficient funds", `{"required":50,"available":20}`)

	err := spendOnce(r, 50)

	var gcErr *gamecoin.Error
	if !errors.As(err, &gcErr) || gcErr.Code != gamecoin.CodeInsufficientFunds {
		t.Fatalf("err = %v", err)
	}
	required, _ := gcErr.DetailInt64("required")
	available, _ := gcErr.DetailInt64("available")
	if required-available != 30 {
		t.Errorf("missing %d", required-available)
	}
}

func TestTheKeyIsAbsentFromEveryKindOfError(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	fast := newRig(t, gamecoin.WithMaxRetries(0), gamecoin.WithTimeout(50*time.Millisecond))
	r.f.apiError(401, "UNAUTHENTICATED", "Invalid API key", "").
		apiError(409, "INSUFFICIENT_FUNDS", "Insufficient funds", `{"required":1,"available":0}`).
		drop().raw(502, "text/html", "<html>gateway</html>").ok("not json")
	fast.f.hang()
	ctx := context.Background()

	_, e1 := r.client.Catalog.Get(ctx)
	e2 := spendOnce(r, 1)
	_, e3 := r.client.Catalog.Get(ctx)
	_, e4 := r.client.Catalog.Get(ctx)
	_, e5 := r.client.Wallet.Grant(ctx, user, gamecoin.GrantParams{Currency: "gems", Amount: 1})
	_, e6 := fast.client.Catalog.Get(ctx)

	for i, err := range []error{e1, e2, e3, e4, e5, e6} {
		gcErr := asError(t, err)
		details, _ := json.Marshal(gcErr.Details)
		assertNoSecret(t, fmt.Sprintf("error %d Error()", i), gcErr.Error())
		assertNoSecret(t, fmt.Sprintf("error %d %%+v", i), fmt.Sprintf("%+v", gcErr))
		assertNoSecret(t, fmt.Sprintf("error %d %%#v", i), fmt.Sprintf("%#v", gcErr))
		assertNoSecret(t, fmt.Sprintf("error %d details", i), string(details))
	}
}

func TestAServerThatEchoesTheKeyInItsMessageDoesNotLeakIt(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.apiError(401, "UNAUTHENTICATED", "Unknown key "+testKey, "")

	_, err := r.client.Catalog.Get(context.Background())

	gcErr := asError(t, err)
	assertNoSecret(t, "message", gcErr.Message)
	if !strings.Contains(gcErr.Message, "[redacted]") {
		t.Errorf("message = %q", gcErr.Message)
	}
}
