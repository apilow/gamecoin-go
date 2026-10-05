package gamecoin_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

func grantOnce(r *rig) (*gamecoin.Movement, error) {
	return r.client.Wallet.Grant(context.Background(), user, gamecoin.GrantParams{Currency: "gems", Amount: 1})
}

func secs(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

func asError(t *testing.T, err error) *gamecoin.Error {
	t.Helper()
	var gcErr *gamecoin.Error
	if !errors.As(err, &gcErr) {
		t.Fatalf("err = %v (%T), want a *gamecoin.Error", err, err)
	}
	return gcErr
}

func TestRetriesAfterANetworkError(t *testing.T) {
	r := newRig(t)
	r.f.drop().ok(sampleMovement)

	m, err := grantOnce(r)
	if err != nil {
		t.Fatal(err)
	}

	if m.Balance.Total != 100 || len(r.f.requests()) != 2 {
		t.Errorf("balance %d after %d requests", m.Balance.Total, len(r.f.requests()))
	}
	if got := r.waits(); !reflect.DeepEqual(got, []time.Duration{secs(0.5)}) {
		t.Errorf("waits = %v", got)
	}
}

func TestRetriesOn5xx(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		r := newRig(t)
		r.f.apiError(status, "INTERNAL", "oops", "").ok(sampleMovement)
		if _, err := grantOnce(r); err != nil {
			t.Fatalf("status %d: %v", status, err)
		}
		if n := len(r.f.requests()); n != 2 {
			t.Errorf("status %d: %d requests, want 2", status, n)
		}
	}
}

func TestBackoffDoublesFromHalfASecond(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(4))
	r.f.apiError(500, "INTERNAL", "1", "").apiError(500, "INTERNAL", "2", "").apiError(500, "INTERNAL", "3", "").apiError(500, "INTERNAL", "4", "").ok(sampleMovement)

	if _, err := grantOnce(r); err != nil {
		t.Fatal(err)
	}

	if n := len(r.f.requests()); n != 5 {
		t.Errorf("%d requests, want 5", n)
	}
	if got, want := r.waits(), []time.Duration{secs(0.5), secs(1), secs(2), secs(4)}; !reflect.DeepEqual(got, want) {
		t.Errorf("waits = %v, want %v", got, want)
	}
}

func TestBackoffIsJitteredByAQuarter(t *testing.T) {
	for _, tc := range []struct {
		jitter float64
		want   time.Duration
	}{{0.0, secs(0.375)}, {1.0, secs(0.625)}} {
		tc := tc
		r := newRig(t, gamecoin.WithJitter(func() float64 { return tc.jitter }))
		r.f.drop().ok(sampleMovement)
		if _, err := grantOnce(r); err != nil {
			t.Fatal(err)
		}
		if got := r.waits(); len(got) != 1 || got[0] != tc.want {
			t.Errorf("jitter %v: waits = %v, want [%v]", tc.jitter, got, tc.want)
		}
	}
}

func TestGivesUpAfterMaxRetriesAndReturnsTheLastError(t *testing.T) {
	r := newRig(t) // MaxRetries = 2 -> three attempts
	r.f.apiError(503, "INTERNAL", "first", "").apiError(503, "INTERNAL", "second", "").apiError(503, "INTERNAL", "third", "").ok(sampleMovement)

	_, err := grantOnce(r)

	gcErr := asError(t, err)
	if len(r.f.requests()) != 3 || gcErr.Message != "third" || gcErr.Status != 503 || len(r.waits()) != 2 {
		t.Errorf("%d requests, error %v, waits %v", len(r.f.requests()), gcErr, r.waits())
	}
}

func TestZeroRetriesMeansOneAttempt(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.drop().ok(sampleMovement)

	_, err := grantOnce(r)

	if gcErr := asError(t, err); gcErr.Code != gamecoin.CodeNetworkError {
		t.Errorf("code = %s", gcErr.Code)
	}
	if len(r.f.requests()) != 1 || len(r.waits()) != 0 {
		t.Errorf("%d requests, waits %v", len(r.f.requests()), r.waits())
	}
}

func TestRetryAfterWinsOverTheBackoffOn429(t *testing.T) {
	r := newRig(t)
	r.f.apiError(429, "RATE_LIMITED", "Too many requests", "", "Retry-After", "7").ok(sampleMovement)

	if _, err := grantOnce(r); err != nil {
		t.Fatal(err)
	}

	if got := r.waits(); !reflect.DeepEqual(got, []time.Duration{secs(7)}) || len(r.f.requests()) != 2 {
		t.Errorf("waits = %v after %d requests", got, len(r.f.requests()))
	}
}

func TestRetryAfterWinsOn5xxToo(t *testing.T) {
	r := newRig(t)
	r.f.apiError(503, "INTERNAL", "busy", "", "Retry-After", "3").ok(sampleMovement)
	if _, err := grantOnce(r); err != nil {
		t.Fatal(err)
	}
	if got := r.waits(); !reflect.DeepEqual(got, []time.Duration{secs(3)}) {
		t.Errorf("waits = %v", got)
	}
}

func TestRetryAfterOfExactly30SecondsIsStillHonoured(t *testing.T) {
	r := newRig(t)
	r.f.apiError(429, "RATE_LIMITED", "slow down", "", "Retry-After", "30").ok(sampleMovement)
	if _, err := grantOnce(r); err != nil {
		t.Fatal(err)
	}
	if got := r.waits(); !reflect.DeepEqual(got, []time.Duration{secs(30)}) {
		t.Errorf("waits = %v", got)
	}
}

func TestRetryAfterBeyond30SecondsIsNotWaitedForAndIsReturned(t *testing.T) {
	r := newRig(t)
	r.f.apiError(429, "RATE_LIMITED", "slow down", "", "Retry-After", "45").ok(sampleMovement)

	_, err := grantOnce(r)

	gcErr := asError(t, err)
	if len(r.f.requests()) != 1 || len(r.waits()) != 0 {
		t.Errorf("%d requests, waits %v", len(r.f.requests()), r.waits())
	}
	if gcErr.Code != gamecoin.CodeRateLimited || gcErr.Status != 429 || gcErr.RetryAfter != secs(45) {
		t.Errorf("error = %+v", gcErr)
	}
}

func TestRetryAfterAsAnHTTPDateInThePastMeansRetryNow(t *testing.T) {
	r := newRig(t)
	r.f.apiError(429, "RATE_LIMITED", "slow down", "", "Retry-After", "Wed, 21 Oct 2015 07:28:00 GMT").ok(sampleMovement)
	if _, err := grantOnce(r); err != nil {
		t.Fatal(err)
	}
	if got := r.waits(); !reflect.DeepEqual(got, []time.Duration{0}) {
		t.Errorf("waits = %v", got)
	}
}

func TestRetryAfterAsAnHTTPDateInTheFutureIsConvertedToADelay(t *testing.T) {
	r := newRig(t)
	when := time.Now().UTC().Add(20 * time.Second).Format(http.TimeFormat)
	r.f.apiError(429, "RATE_LIMITED", "slow down", "", "Retry-After", when).ok(sampleMovement)
	if _, err := grantOnce(r); err != nil {
		t.Fatal(err)
	}
	got := r.waits()
	if len(got) != 1 || got[0] < 15*time.Second || got[0] > 20*time.Second {
		t.Errorf("waits = %v", got)
	}
}

func TestNeverRetriesA4xxOtherThan429(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{
		{400, "VALIDATION_FAILED"}, {400, "IDEMPOTENCY_KEY_REQUIRED"}, {401, "UNAUTHENTICATED"}, {402, "PAYMENT_FAILED"},
		{403, "FORBIDDEN"}, {403, "PLAYER_BLOCKED"}, {404, "NOT_FOUND"}, {409, "INSUFFICIENT_FUNDS"}, {409, "LIMIT_REACHED"},
		{409, "IDEMPOTENCY_CONFLICT"}, {409, "CONFLICT"}, {423, "READ_ONLY"},
	} {
		r := newRig(t)
		r.f.apiError(tc.status, tc.code, "nope", "").ok(sampleMovement)

		_, err := grantOnce(r)

		gcErr := asError(t, err)
		if len(r.f.requests()) != 1 || len(r.waits()) != 0 || gcErr.Code != tc.code || gcErr.Status != tc.status {
			t.Errorf("%d %s: %d requests, waits %v, error %+v", tc.status, tc.code, len(r.f.requests()), r.waits(), gcErr)
		}
	}
}

func TestReadsAreRetriedLikeWrites(t *testing.T) {
	r := newRig(t)
	r.f.drop().ok(sampleCatalog)
	c, err := r.client.Catalog.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Currencies) != 2 || len(r.f.requests()) != 2 {
		t.Errorf("catalog %+v after %d requests", c, len(r.f.requests()))
	}
}

func TestATimeoutIsRaisedWithStatusZeroAndRetried(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(1), gamecoin.WithTimeout(80*time.Millisecond))
	r.f.hang().ok(sampleMovement)

	m, err := grantOnce(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.Balance.Total != 100 || len(r.f.requests()) != 2 {
		t.Errorf("balance %d after %d requests", m.Balance.Total, len(r.f.requests()))
	}
}

func TestATimeoutThatPersistsIsRaisedAsTIMEOUT(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(1), gamecoin.WithTimeout(80*time.Millisecond))
	r.f.hang().hang()

	_, err := grantOnce(r)

	gcErr := asError(t, err)
	if gcErr.Code != gamecoin.CodeTimeout || gcErr.Status != 0 || len(r.f.requests()) != 2 {
		t.Errorf("error = %+v after %d requests", gcErr, len(r.f.requests()))
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("the cause of a timeout should be context.DeadlineExceeded")
	}
}

func TestACancelledContextReturnsItsOwnErrorAndIsNotRetried(t *testing.T) {
	r := newRig(t)
	r.f.hang().ok(sampleMovement)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	_, err := r.client.Wallet.Grant(ctx, user, gamecoin.GrantParams{Currency: "gems", Amount: 1})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the context's own error", err)
	}
	var gcErr *gamecoin.Error
	if errors.As(err, &gcErr) {
		t.Errorf("a caller's deadline must not look like a SDK timeout: %v", gcErr)
	}
	if len(r.f.requests()) != 1 {
		t.Errorf("%d requests, want 1", len(r.f.requests()))
	}
}

func TestAnAlreadyCancelledContextSendsNothing(t *testing.T) {
	r := newRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.client.Catalog.Get(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	if n := len(r.f.requests()); n != 0 {
		t.Errorf("%d requests were sent", n)
	}
}

func TestCancellationDuringTheBackoffStopsTheCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := newRig(t, gamecoin.WithSleep(func(c context.Context, _ time.Duration) error {
		cancel() // the caller gives up while the SDK waits to retry
		return c.Err()
	}))
	r.f.apiError(503, "INTERNAL", "down", "").ok(sampleMovement)

	_, err := r.client.Wallet.Grant(ctx, user, gamecoin.GrantParams{Currency: "gems", Amount: 1})

	if !errors.Is(err, context.Canceled) || len(r.f.requests()) != 1 {
		t.Errorf("err = %v after %d requests", err, len(r.f.requests()))
	}
}

func TestA5xxWithAnHTMLBodyIsRetriedThenReturnedAsINVALIDRESPONSE(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(1))
	r.f.raw(502, "text/html", "<html>bad gateway</html>").raw(502, "text/html", "<html>bad gateway</html>")

	_, err := grantOnce(r)

	gcErr := asError(t, err)
	if len(r.f.requests()) != 2 || gcErr.Code != gamecoin.CodeInvalidResponse || gcErr.Status != 502 {
		t.Errorf("%d requests, error %+v", len(r.f.requests()), gcErr)
	}
}

// --- refund: no idempotency key, so only 429 is retried ----------------------------------------------------------------

func TestRefundIsRetriedOn429(t *testing.T) {
	r := newRig(t)
	r.f.apiError(429, "RATE_LIMITED", "slow down", "", "Retry-After", "2").ok(sampleOrder)

	o, err := r.client.Orders.Refund(context.Background(), "o1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.ID != "o1" || len(r.f.requests()) != 2 || !reflect.DeepEqual(r.waits(), []time.Duration{secs(2)}) {
		t.Errorf("%d requests, waits %v", len(r.f.requests()), r.waits())
	}
}

func TestRefundIsNotRetriedAfterANetworkError(t *testing.T) {
	r := newRig(t)
	r.f.drop().ok(sampleOrder)

	_, err := r.client.Orders.Refund(context.Background(), "o1", nil)

	if gcErr := asError(t, err); gcErr.Code != gamecoin.CodeNetworkError || len(r.f.requests()) != 1 {
		t.Errorf("error %+v after %d requests", gcErr, len(r.f.requests()))
	}
}

func TestRefundIsNotRetriedAfterA5xx(t *testing.T) {
	for _, status := range []int{500, 502, 503} {
		r := newRig(t)
		r.f.apiError(status, "INTERNAL", "oops", "").ok(sampleOrder)

		_, err := r.client.Orders.Refund(context.Background(), "o1", nil)

		if gcErr := asError(t, err); gcErr.Status != status || len(r.f.requests()) != 1 || len(r.waits()) != 0 {
			t.Errorf("status %d: error %+v after %d requests", status, gcErr, len(r.f.requests()))
		}
	}
}

func TestRefundIsNotRetriedAfterATimeout(t *testing.T) {
	r := newRig(t, gamecoin.WithTimeout(80*time.Millisecond))
	r.f.hang().ok(sampleOrder)

	_, err := r.client.Orders.Refund(context.Background(), "o1", nil)

	if gcErr := asError(t, err); gcErr.Code != gamecoin.CodeTimeout || len(r.f.requests()) != 1 {
		t.Errorf("error %+v after %d requests", gcErr, len(r.f.requests()))
	}
}

func TestRefundConflictIsReturnedAsIs(t *testing.T) {
	r := newRig(t)
	r.f.apiError(409, "CONFLICT", "Order is not paid", "")

	_, err := r.client.Orders.Refund(context.Background(), "o1", nil)

	if gcErr := asError(t, err); gcErr.Code != gamecoin.CodeConflict || len(r.f.requests()) != 1 {
		t.Errorf("error %+v after %d requests", gcErr, len(r.f.requests()))
	}
}
