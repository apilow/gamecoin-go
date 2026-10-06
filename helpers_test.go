package gamecoin_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

// testKey is a fake secret key: it only ever talks to the fake server of the tests.
const testKey = "gc_sk_test_SECRETVALUE0123456789abcdef"

// recorded is what the fake server saw of one request.
type recorded struct {
	Method     string
	RequestURI string // the raw request target, as it went on the wire
	Path       string // the escaped path
	Query      string // the raw query
	Header     http.Header
	Body       string
}

type step func(w http.ResponseWriter, r *http.Request)

// fake is a scripted HTTP server: it answers requests in order and records them. Loopback only, no network.
type fake struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	script []step
	reqs   []recorded
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.reqs = append(f.reqs, recorded{
		Method:     r.Method,
		RequestURI: r.RequestURI,
		Path:       r.URL.EscapedPath(),
		Query:      r.URL.RawQuery,
		Header:     r.Header.Clone(),
		Body:       string(body),
	})
	var next step
	if len(f.script) > 0 {
		next, f.script = f.script[0], f.script[1:]
	}
	f.mu.Unlock()

	if next == nil {
		http.Error(w, "the fake server has no scripted reply left", http.StatusTeapot)
		return
	}
	next(w, r)
}

func (f *fake) enqueue(s step) *fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.script = append(f.script, s)
	return f
}

func writeHeaders(w http.ResponseWriter, headers []string) {
	for i := 0; i+1 < len(headers); i += 2 {
		w.Header().Set(headers[i], headers[i+1])
	}
}

// reply answers with a JSON body. headers are name/value pairs.
func (f *fake) reply(status int, body string, headers ...string) *fake {
	return f.enqueue(func(w http.ResponseWriter, _ *http.Request) {
		writeHeaders(w, headers)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

func (f *fake) ok(body string, headers ...string) *fake {
	return f.reply(http.StatusOK, body, headers...)
}

// apiError answers with the error envelope of the API.
func (f *fake) apiError(status int, code, message, detailsJSON string, headers ...string) *fake {
	details := ""
	if detailsJSON != "" {
		details = `,"details":` + detailsJSON
	}
	return f.reply(status, fmt.Sprintf(`{"error":{"code":%q,"message":%q%s}}`, code, message, details), headers...)
}

// raw answers with an arbitrary body and content type.
func (f *fake) raw(status int, contentType, body string, headers ...string) *fake {
	return f.enqueue(func(w http.ResponseWriter, _ *http.Request) {
		writeHeaders(w, headers)
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

// drop closes the connection without answering: the client sees a network error.
func (f *fake) drop() *fake {
	return f.enqueue(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
}

// hang never answers: the client times out (or is cancelled).
func (f *fake) hang() *fake {
	return f.enqueue(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	})
}

func (f *fake) requests() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.reqs...)
}

// only returns the single request the server saw.
func (f *fake) only() recorded {
	f.t.Helper()
	reqs := f.requests()
	if len(reqs) != 1 {
		f.t.Fatalf("the server saw %d requests, want exactly 1", len(reqs))
	}
	return reqs[0]
}

// rig is a client wired to a fake server, whose waits are recorded instead of slept.
type rig struct {
	t      *testing.T
	f      *fake
	client *gamecoin.Client
	mu     sync.Mutex
	sleeps []time.Duration
}

// newRig builds a client against a fake server. Connections are not reused, so that the transport of net/http never
// retries on its own behind the SDK's back; later options override the defaults.
func newRig(t *testing.T, opts ...gamecoin.Option) *rig {
	t.Helper()
	r := &rig{t: t, f: newFake(t)}
	all := []gamecoin.Option{
		gamecoin.WithBaseURL(r.f.srv.URL),
		gamecoin.WithHTTPClient(&http.Client{
			Transport:     &http.Transport{DisableKeepAlives: true},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}),
		gamecoin.WithSleep(func(ctx context.Context, d time.Duration) error {
			r.mu.Lock()
			r.sleeps = append(r.sleeps, d)
			r.mu.Unlock()
			return ctx.Err()
		}),
		gamecoin.WithJitter(func() float64 { return 0.5 }), // neutral: delays are exactly 0.5 s x 2^attempt
	}
	all = append(all, opts...)
	client, err := gamecoin.New(testKey, all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.client = client
	return r
}

func (r *rig) waits() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.sleeps...)
}

const (
	samplePlayer = `{"id":"6ac40ac02e9fe50981a1e085","externalId":"probe-190","kind":"external","displayName":"Probe","country":null,"blocked":false,"createdAt":"2026-10-05T20:38:24.539Z","somethingNew":{"a":1}}`
	sampleEntry  = `{"id":"6ac40ac312fe8b4190d46ffc","type":"grant","currency":"gems","paidDelta":0,"bonusDelta":100,"balanceAfter":{"paid":0,"bonus":100,"total":100},"reason":"level_up","metadata":{"a":"b"},"ref":{},"createdAt":"2026-10-05T20:38:27.227Z"}`
	sampleBal    = `{"currency":"gems","paid":0,"bonus":100,"total":100}`
	sampleItem   = `{"sku":"potion","quantity":3,"updatedAt":"2026-10-05T20:40:00.000Z"}`
	sampleOrder  = `{"id":"o1","playerId":"p1","status":"pending","pack":{"sku":"gems-500","name":"Bag of gems"},"amountCents":499,"currency":"EUR","country":"BE","vatRateBp":2100,"vatCents":87,"netCents":412,"createdAt":"2026-10-05T20:38:24.539Z","paidAt":null,"fulfilledAt":null,"refundedAt":null,"creatorCode":null}`

	sampleMovement   = `{"entry":` + sampleEntry + `,"balance":` + sampleBal + `}`
	samplePurchase   = `{"entry":` + sampleEntry + `,"balance":` + sampleBal + `,"item":` + sampleItem + `}`
	sampleCheckout   = `{"order":` + sampleOrder + `,"checkoutUrl":"https://gc.example.test/pay/o1?t=x"}`
	sampleRedemption = `{"grants":[{"currency":"gems","amount":100,"balance":{"currency":"gems","paid":0,"bonus":100,"total":100}}],"items":[{"sku":"potion","quantity":2}],"skipped":[{"sku":"fire-sword","quantity":1}]}`
	sampleChange     = `{"item":` + sampleItem + `}`
	sampleCatalog    = `{"currencies":[{"code":"gems","name":"Gems","purchasable":true},{"code":"gold","name":"Gold","purchasable":false}],"items":[{"sku":"potion","name":"Potion","description":null,"type":"consumable","price":{"currency":"gems","amount":20},"maxOwned":null,"clientPurchasable":true,"metadata":{}},{"sku":"fire-sword","name":"Fire sword","description":"Hot","type":"durable","price":{"currency":"gems","amount":300},"maxOwned":1,"clientPurchasable":true,"metadata":{"tier":"3"}}],"packs":[{"sku":"starter","name":"Starter pack","description":null,"priceCents":99,"currency":"EUR","grants":[{"currency":"gems","amount":100,"bonus":20}],"items":[{"sku":"potion","quantity":1}],"badge":"new","maxPerPlayer":1,"availability":{"startsAt":"2026-12-01T09:00:00.000Z","endsAt":null,"remaining":42,"founder":true}}],"prelaunch":true,"launchAt":"2026-12-01T09:00:00.000Z"}`
)

var user = gamecoin.Ext("user-42")

func assertNoSecret(t *testing.T, what, text string) {
	t.Helper()
	if strings.Contains(text, testKey) || strings.Contains(text, "SECRETVALUE") {
		t.Errorf("the secret key leaked in %s: %q", what, text)
	}
}
