# gamecoin-go

Server SDK for **Apilow GameCoin**, the bank of your game: virtual currencies, inventory and sales, behind one API. Use
it from your game backend to grant and spend coins, manage items, and sell packs with real money.

- Go 1.21 and later, **standard library only** (`net/http`, `encoding/json`, `crypto/rand`).
- `context.Context` first in every call, typed structs, `time.Time` dates, a single `*gamecoin.Error` type.
- Safe retries built in: every call that changes something carries an idempotency key.

> This is a **server** SDK. It uses your **secret key** (`gc_sk_test_…` / `gc_sk_live_…`), which must never be shipped in
> a game client or a web page. For the browser, use the GameCoin browser SDK with the publishable key.

## Install

```sh
go get github.com/apilow/gamecoin-go
```

```go
import gamecoin "github.com/apilow/gamecoin-go"
```

## Quickstart

Grant gems to a player, spend some, and start the purchase of a pack (`GAMECOIN_SECRET_KEY` holds a `gc_sk_test_…` key;
`GAMECOIN_BASE_URL` is optional and defaults to production):

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

func main() {
	ctx := context.Background()
	baseURL := os.Getenv("GAMECOIN_BASE_URL")
	if baseURL == "" {
		baseURL = gamecoin.DefaultBaseURL
	}
	client, err := gamecoin.New(os.Getenv("GAMECOIN_SECRET_KEY"), gamecoin.WithBaseURL(baseURL))
	if err != nil {
		log.Fatal(err)
	}
	player := gamecoin.Ext(fmt.Sprintf("quickstart-%d", time.Now().UnixNano())) // your own id for the player (a fresh one: every run starts at zero)

	granted, err := client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gems", Amount: 100, Reason: "level_up"})
	if err != nil {
		log.Fatal(err)
	}
	spent, err := client.Wallet.Spend(ctx, player, gamecoin.SpendParams{Currency: "gems", Amount: 30})
	if err != nil {
		log.Fatal(err)
	}
	checkout, err := client.Checkouts.Create(ctx, player, gamecoin.CheckoutParams{PackSKU: "gems-500"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gems: %d -> %d; the player pays at %s\n", granted.Balance.Total, spent.Balance.Total, checkout.CheckoutURL)
}
```

This prints `gems: 100 -> 70; the player pays at https://…/pay/<order id>?t=…`. The same program lives in
`examples/quickstart`. The examples below continue from it: they use `ctx`, `client` and `player`, and a tiny helper,
`check`, defined as `func check(err error) { if err != nil { log.Fatal(err) } }`.

## Configuration

```go
baseURL := os.Getenv("GAMECOIN_BASE_URL")
if baseURL == "" {
	baseURL = gamecoin.DefaultBaseURL
}
client, err := gamecoin.New(
	os.Getenv("GAMECOIN_SECRET_KEY"),
	gamecoin.WithBaseURL(baseURL),          // the origin of the service; default https://gamecoin.apilow.com
	gamecoin.WithTimeout(30*time.Second),   // per HTTP attempt; the default
	gamecoin.WithMaxRetries(2),             // automatic retries; the default (three attempts). 0 turns them off
	gamecoin.WithHTTPClient(&http.Client{}), // your own *http.Client: proxy, instrumentation... (never modified)
)
check(err)
fmt.Println(client.Environment(), "environment at", client.BaseURL()) // "test" or "live", read from the key

quick, cancel := context.WithTimeout(ctx, 5*time.Second) // your own deadline for a whole call, retries included
defer cancel()
catalog, err := client.Catalog.Get(quick)
check(err)
fmt.Println(len(catalog.Items), "items,", len(catalog.Packs), "packs")
```

| Option | Default | Meaning |
|---|---|---|
| `WithBaseURL` | `https://gamecoin.apilow.com` | Origin of the service; the SDK appends `/api/v1`, drops trailing `/` and keeps a path prefix. Only `http(s)` without credentials, query or fragment. |
| `WithTimeout` | 30 s | Timeout of each HTTP attempt (a retry gets a fresh one). A `time.Duration`. |
| `WithMaxRetries` | 2 | Automatic retries, 0 to 10. See [Idempotency and retries](#idempotency-and-retries). |
| `WithHTTPClient` | a client of its own | Your `*http.Client`. The SDK never modifies it; timeouts are applied per attempt through the request context. |

`gamecoin.New` rejects anything that is not a secret key (surrounding white space, such as a newline read from a file, is
trimmed); a publishable key (`gc_pk_…`) gets a message that points to the browser SDK. The key never appears in an error,
and the string form of the client (`%v`, `%+v`, `%#v`) shows its prefix only: `gc_sk_test_…`.

Create **one** client per application and keep it: it is safe for concurrent use and reuses its connections. Every
request carries `Authorization: Bearer <key>`, `Accept: application/json` and
`User-Agent: gamecoin-go/<version> (go/<go version>)`.

## Players and the `Ext` helper

A player is a GameCoin player id (`gamecoin.PlayerID(id)`), or your game's own id through `gamecoin.Ext(...)` (sent as
`ext:<your id>`):

```go
player := gamecoin.Ext("user-42") // your game's own id; any string works: spaces, slashes and accents are encoded for you
name, country := "Ann", "BE"
created, err := client.Players.Upsert(ctx, player, &gamecoin.PlayerProfile{DisplayName: &name, Country: &country})
check(err)
fmt.Println(created.ID, *created.ExternalID, *created.DisplayName)

again, err := client.Players.Get(ctx, gamecoin.PlayerID(created.ID)) // by GameCoin id
check(err)
token, err := client.Players.CreateToken(ctx, player) // short-lived, for the browser SDK
check(err)
fmt.Println(again.Kind, "- token valid until", token.ExpiresAt.Format(time.RFC3339))

_, err = client.Players.Upsert(ctx, player, &gamecoin.PlayerProfile{Clear: []gamecoin.ProfileField{gamecoin.FieldCountry}}) // erase a field
check(err)
```

- A write call (`Upsert`, grant, spend, purchase…) on an unknown `ext:` player creates it. A read (`Players.Get`,
  `Wallet.Get`…) answers `NOT_FOUND`.
- In a `PlayerProfile`, a `nil` field leaves the value unchanged; list a field in `Clear` to erase it. A `nil` profile
  sends `{}`.
- Path segments are URL-encoded **exactly once**. `PlayerRef` is a string type, so an untyped constant converts
  implicitly: `client.Players.Get(ctx, "6ac4…")`.

## Wallet and ledger

```go
wallet, err := client.Wallet.Get(ctx, player) // one line per currency, zeros included
check(err)
for _, b := range wallet.Balances {
	fmt.Printf("%s: %d (%d paid + %d bonus)\n", b.Currency, b.Total, b.Paid, b.Bonus)
}

grant, err := client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gems", Amount: 100, Reason: "level_up", Metadata: map[string]string{"level": "7"}})
check(err)
spend, err := client.Wallet.Spend(ctx, player, gamecoin.SpendParams{Currency: "gems", Amount: 30, Reason: "shop"})
check(err)
fmt.Println(grant.Entry.Type, "then", spend.Entry.Type, "- balance", spend.Balance.Total)

it := client.Ledger.Iterate(ctx, player, &gamecoin.LedgerListParams{Currency: "gems", Limit: 50})
for it.Next() {
	e := it.Entry()
	fmt.Println(e.CreatedAt.Format(time.RFC3339), e.Type, e.PaidDelta+e.BonusDelta, "->", e.BalanceAfter.Total)
}
check(it.Err())

page, err := client.Ledger.List(ctx, player, &gamecoin.LedgerListParams{Limit: 20}) // one page, newest first
check(err)
fmt.Println(len(page.Entries), "entries, more:", page.NextCursor != nil)
```

- A grant always credits the *bonus* bucket. Amounts are `int64` and must be at least 1.
- `Ledger.Iterate` returns an iterator (`Next`, `Entry`, `Err`) that works on every supported Go version: pages are
  fetched lazily, one at a time, starting with the first call to `Next`. `Limit` is the page size (1 to 100, default 20);
  `Cursor` resumes from a given page.
- Spending more than the player has returns `INSUFFICIENT_FUNDS` (see [Errors](#errors)).

## Inventory and purchases

```go
catalog, err = client.Catalog.Get(ctx) // active currencies, items and packs
check(err)
for _, item := range catalog.Items {
	fmt.Println(item.SKU, item.Type, item.Price != nil)
}

given, err := client.Inventory.Grant(ctx, player, gamecoin.InventoryParams{SKU: "potion", Quantity: 2}) // for free
check(err)
used, err := client.Inventory.Consume(ctx, player, gamecoin.InventoryParams{SKU: "potion"}) // Quantity 0 means 1
check(err)
fmt.Println(given.Quantity, "then", used.Quantity, "potion(s)")

bought, err := client.Purchases.Create(ctx, player, gamecoin.PurchaseParams{SKU: "potion"}) // pays the item's price in its currency
check(err)
fmt.Println("bought:", bought.Item.Quantity, "potion(s),", bought.Balance.Total, bought.Balance.Currency, "left")

owned, err := client.Inventory.List(ctx, player) // owned items only (quantity above 0)
check(err)
for _, item := range owned {
	fmt.Printf("%s x%d\n", item.SKU, item.Quantity)
}
```

### Pack availability

The catalog lists every active pack, including the ones that are scheduled, ended or sold out: your shop decides what to
show. `Pack.Availability` is `nil` for an ordinary pack (always on sale, no limit); otherwise it tells when the pack is on
sale and how many units are left (founder packs, limited editions):

```go
for _, pack := range catalog.Packs {
	a := pack.Availability
	if a == nil {
		continue // always on sale
	}
	fmt.Println(pack.SKU, "founder:", a.Founder)
	if a.StartsAt != nil { // *time.Time, included
		fmt.Println("  on sale from", a.StartsAt.Format(time.RFC3339))
	}
	if a.EndsAt != nil { // excluded
		fmt.Println("  until", a.EndsAt.Format(time.RFC3339))
	}
	if a.Remaining != nil { // nil is an unlimited stock; the stocks of test and live are separate
		fmt.Println("  units left:", *a.Remaining)
	}
}
if catalog.Prelaunch { // the game is not launched yet: show pre-orders with the launch date
	if catalog.LaunchAt != nil {
		fmt.Println("launch on", catalog.LaunchAt.Format(time.RFC3339))
	}
}
```

`Inventory.Consume` returns the item even when it reaches 0. A durable item with `MaxOwned` fails with `LIMIT_REACHED`
beyond the limit. `Quantity` is optional: its zero value means 1.

## Checkout, orders and refunds

A **checkout** sells a *pack* for real money: you create an order, send the player to `CheckoutURL`, and the coins and
items are delivered when the payment succeeds.

```go
checkout, err := client.Checkouts.Create(ctx, player, gamecoin.CheckoutParams{
	PackSKU:    "gems-500",
	SuccessURL: "https://game.example.com/shop/thanks", // optional: where the player lands after paying
	CancelURL:  "https://game.example.com/shop",        // optional
	Country:    "BE",                                   // optional: the VAT country (server side only)
	Locale:     "nl",                                   // optional: "fr", "en" or "nl", language of the payment page and e-mails
	CreatorCode: "LEAPLAY",                             // optional: the creator code the player typed
})
check(err)
fmt.Printf("%s: %d cents (%s), pay at %s\n", checkout.Order.Status, checkout.Order.AmountCents, checkout.Order.Currency, checkout.CheckoutURL)
if cc := checkout.Order.CreatorCode; cc != nil { // nil when the order has no creator code
	fmt.Println("creator code", cc.Code, "of", cc.CreatorName)
}

order, err := client.Orders.Get(ctx, checkout.Order.ID) // poll, or call it when the player comes back
check(err)
fmt.Println(order.ID, order.Status, "paid:", order.PaidAt != nil)

_, err = client.Orders.Refund(ctx, order.ID, &gamecoin.RefundParams{Reason: "customer asked"}) // takes the coins and items back
if gamecoin.IsCode(err, gamecoin.CodeConflict) {
	fmt.Println("cannot refund an unpaid order:", err)
}
```

In test mode the payment is simulated: open `CheckoutURL` and press "Pay (simulated)".

`Locale` (`"fr"`, `"en"` or `"nl"`) is the language of the hosted payment page, the receipt and the e-mails of the order.
The API ignores any other value without an error, and an empty `Locale` is not sent. A pending order that the API reuses
takes the language of the latest request.

A response that has no `availability`, `prelaunch`, `launchAt` or `creatorCode` field at all (a server older than 0.3, a
webhook event queued before 0.3) is accepted: they read as `nil`, `false`, `nil` and `nil`. A field that is present with the
wrong type or shape is `INVALID_RESPONSE` (`ReasonInvalidPayload` for a webhook event).

`CreatorCode` is the creator code a player typed at checkout: the buyer then gets a bonus of free coins and the creator
earns a commission, both cancelled on a refund or a chargeback. An empty `CreatorCode` is not sent. A refused code
(unknown, disabled, not valid for the pack, or the creator's own player) is `VALIDATION_FAILED` with
`Details["fieldErrors"]["creatorCode"] == ["CODE_INVALID"]`. The order keeps the code: `Order.CreatorCode` (a
`*OrderCreatorCode` with `Code` and `CreatorName`, `nil` without a code; the rates of the creator are never exposed) also
travels in the `order.*` webhooks.

A pack can be refused when it is outside its sale window (`PACK_NOT_AVAILABLE`, with `Details["startsAt"]` and
`Details["endsAt"]` as ISO 8601 text or `nil`) or when its stock is exhausted (`PACK_SOLD_OUT`); see
[Pack availability](#pack-availability).

**Refunds have no idempotency key**, so the SDK retries `Orders.Refund` on `429` only. After a network error, a timeout
or a `5xx` it returns the error, and you should check the outcome with `Orders.Get` (status `refunded` or not) before
trying again.

## Codes

A **gift code** gives a player free coins (always the bonus bucket) and items, once: you make the codes in the dashboard
(a batch of random codes, or a code for a giveaway), the player types one in your game, and your backend redeems it:

```go
redemption, err := client.Codes.Redeem(ctx, player, "pgw k7m2q xh4np") // case, spaces and dashes do not matter
switch {
case gamecoin.IsCode(err, gamecoin.CodeValidationFailed):
	fmt.Println("This code is not valid") // unknown, expired, used up, disabled or already used: one answer for all
case err != nil:
	check(err)
default:
	for _, g := range redemption.Grants {
		fmt.Println("+", g.Amount, g.Currency, "->", g.Balance.Total)
	}
	for _, item := range redemption.Items {
		fmt.Println("+", item.Quantity, item.SKU)
	}
	fmt.Println(len(redemption.Skipped), "item(s) skipped")
}
```

- `Codes.Redeem(ctx, player, code, opts...)` sends the code **as it is** (the API ignores case, spaces and dashes, so
  there is nothing to clean first) and returns a `*Redemption`: `Grants` (`Currency`, `Amount`, and the `Balance` after
  the credit), `Items` (`SKU`, `Quantity`), `Skipped` (the items the player could not receive) and `Replayed`.
- Every refusal of the code itself is the same error, `VALIDATION_FAILED` with
  `Details["fieldErrors"]["code"] == ["CODE_INVALID"]`, so that nothing tells a guess from a real code. It is never
  retried. `PLAYER_BLOCKED`, `LIMIT_REACHED`, `CONFLICT` (the player can no longer receive what the code holds; the code is
  not used up then) and `RATE_LIMITED` (attempts are limited) come through the usual error mapping.
- The call is idempotent like the other writes: it carries an `Idempotency-Key` (generated, or `WithIdempotencyKey`), and a
  replay returns the same `Redemption` with `Replayed` set, without a second credit.

A **creator code** is something else: it goes in `CheckoutParams.CreatorCode` (see [Checkout](#checkout-orders-and-refunds)).

## Idempotency and retries

Every call that changes a wallet or an inventory, redeems a code, or creates a checkout, sends an `Idempotency-Key` header. If you give
none, the SDK generates a UUID v4 **once per call** and reuses it for each automatic retry, so a retry can never credit
twice. To make *your own* retries safe (a job that may run twice), supply a stable key:

```go
key := gamecoin.WithIdempotencyKey(fmt.Sprintf("quest-completed-%d", time.Now().UnixNano())) // in real code: derive it from the event, e.g. "quest-completed-<player>-<quest>"
params := gamecoin.GrantParams{Currency: "gold", Amount: 25, Reason: "quest"}

first, err := client.Wallet.Grant(ctx, player, params, key)
check(err)
second, err := client.Wallet.Grant(ctx, player, params, key) // same key, same request: the original response
check(err)
fmt.Println(first.Replayed, second.Replayed, first.Entry.ID == second.Entry.ID) // false true true
```

`Replayed` is `true` when the API answered with the original response of an earlier request with the same key (header
`Idempotent-Replayed: true`); it exists on `Movement`, `Purchase`, `Checkout`, `Redemption` and `InventoryItem` (the result of
`Inventory.Grant` and `Inventory.Consume`). The same key with a *different* request fails with `IDEMPOTENCY_CONFLICT`. A
key is 1 to 100 printable ASCII characters and is kept for 30 days. A replayed checkout returns the order in its
**current** state.

Automatic retries (at most `WithMaxRetries`, so three attempts by default):

| Situation | Retried? |
|---|---|
| Network error, timeout, `5xx`, `429` (every read, `Players.Upsert`, `Players.CreateToken` and every call with an idempotency key) | yes |
| Any other `4xx` | never |
| `Orders.Refund` | on `429` only |
| The caller's `context` cancelled or expired | never: the context's own error is returned |

The delay is `0.5 s × 2^attempt × a random factor in [0.75, 1.25]`. A `Retry-After` header (seconds or an HTTP date) wins,
up to 30 s inclusive, on a `429` as on a `5xx`; beyond that the SDK does not wait: it returns the error with
`RetryAfter` set so that you decide. A malformed `200` (`INVALID_RESPONSE`) is never retried; an HTML `502` from a proxy
is retried like any `5xx`.

## Errors

A call that fails after a request was attempted returns a `*gamecoin.Error`:

| Field | Content |
|---|---|
| `Code` | API code (`INSUFFICIENT_FUNDS`, `NOT_FOUND`…), or `NETWORK_ERROR` / `TIMEOUT` (status 0), or `INVALID_RESPONSE` |
| `Status` | HTTP status, 0 when no response was received |
| `Message` | Message of the API, or a short description |
| `Details` | `details` of the API as a `map[string]any` (`required`, `available`, `fieldErrors`…), empty when absent; numbers are `json.Number`, see `DetailInt64` |
| `RetryAfter` | A `time.Duration` (the header is in seconds), set when the API sent `Retry-After` |

```go
_, err = client.Wallet.Spend(ctx, player, gamecoin.SpendParams{Currency: "gems", Amount: 1_000_000})
var gcErr *gamecoin.Error
switch {
case errors.As(err, &gcErr) && gcErr.Code == gamecoin.CodeInsufficientFunds:
	required, _ := gcErr.DetailInt64("required")
	available, _ := gcErr.DetailInt64("available")
	fmt.Printf("needs %d, has %d\n", required, available)
case errors.As(err, &gcErr) && gcErr.Code == gamecoin.CodeRateLimited:
	fmt.Println("slow down, retry in", gcErr.RetryAfter)
case gcErr != nil:
	fmt.Printf("%s (HTTP %d): %s\n", gcErr.Code, gcErr.Status, gcErr.Message)
case err != nil:
	fmt.Println(err) // a rejected argument (ErrInvalidArgument), or the context's own error
}
```

`gamecoin.IsCode(err, code)` is shorthand for the `errors.As` test. The codes are constants: `CodeValidationFailed`,
`CodeIdempotencyKeyRequired`, `CodeUnauthenticated`, `CodePaymentFailed`, `CodeForbidden`, `CodePlayerBlocked`,
`CodeEnvNotEnabled`, `CodeNotFound`, `CodeInsufficientFunds`, `CodeLimitReached`, `CodePackNotAvailable`, `CodePackSoldOut`,
`CodeIdempotencyConflict`, `CodeConflict`,
`CodeReadOnly`, `CodeRateLimited`, `CodeInternal`, plus the SDK's own `CodeNetworkError`, `CodeTimeout` and
`CodeInvalidResponse` (the body is not the expected JSON, or has a field of the wrong shape: the HTTP status is kept).
`Details["fieldErrors"]` is meant for developers, not for parsing. An API code this version does not know is passed through
as is, and an unknown future `Status` or `Type` value stays readable as a string.

**Arguments are checked before any request**: an empty player, currency, sku or order id, an `Amount` below 1, a negative
`Quantity`, a ledger `Limit` outside 1 to 100, an invalid idempotency key return an error that wraps
`gamecoin.ErrInvalidArgument` (it is not a `*gamecoin.Error`), and its message names the argument, never its value.

## Webhooks

GameCoin calls **your server** when an order is delivered, refunded or disputed. Every call is signed: check it with
`gamecoin.ConstructEvent`, a plain function that needs no client and makes no request. Declare the endpoint in the
dashboard, which shows its signing secret (`whsec_gc_…`) once; keep it in your server's configuration, never in the game.

```go
package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	gamecoin "github.com/apilow/gamecoin-go"
)

func main() {
	secret := os.Getenv("GAMECOIN_WEBHOOK_SECRET") // whsec_gc_…

	http.HandleFunc("/gamecoin/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body) // the RAW body: read it before any JSON parsing
		if err != nil {
			http.Error(w, "cannot read the body", http.StatusBadRequest)
			return
		}
		event, err := gamecoin.ConstructEvent(body, r.Header.Get(gamecoin.WebhookSignatureHeader), secret)
		if err != nil {
			var sigErr *gamecoin.WebhookSignatureError
			if errors.As(err, &sigErr) {
				http.Error(w, "invalid signature", http.StatusBadRequest) // sigErr.Reason says why
				return
			}
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		// GameCoin delivers at least once: process an event id only once.
		switch event.Type {
		case gamecoin.EventOrderFulfilled:
			order := event.Order()
			log.Printf("order %s delivered to player %s", order.ID, order.PlayerID)
		case gamecoin.EventWalletCredited:
			log.Printf("credits for %s: %+v", event.WalletCredited().PlayerID, event.WalletCredited().Credits)
		}
		w.WriteHeader(http.StatusOK) // a 2xx within 10 seconds is a success
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

A fuller version, with a check for repeated events, lives in `examples/webhook`.

**Pass the raw body.** The signature covers the exact bytes GameCoin sent, so read them with `io.ReadAll(r.Body)` (or
`http.MaxBytesReader` around it) *before* any JSON decoding. A body that was decoded and encoded again can differ by one
character and fails the check. `ConstructEvent` takes a `[]byte`; convert a string with `[]byte(text)`.

`ConstructEvent(rawBody []byte, signatureHeader, secret string, opts ...WebhookOption) (*WebhookEvent, error)`:

- The signature is the lowercase hex HMAC-SHA256 of `<t>.<raw body>` with the signing secret as key, carried by the
  `GameCoin-Signature` header (`gamecoin.WebhookSignatureHeader`) as `t=<unix seconds>,v1=<hex>`. Every `v1` is compared in
  constant time; unknown keys in the header are ignored.
- The timestamp must be within **5 minutes** of the clock, in either direction, which stops a replayed call.
  `gamecoin.WithTolerance(d)` changes it (`0` turns the check off); `gamecoin.WithClock(fn)` replaces the clock, for tests.
- The secret is trimmed. An empty secret or a negative tolerance returns an error that wraps `ErrInvalidArgument`.

A call that must not be trusted returns a `*gamecoin.WebhookSignatureError` (not a `*gamecoin.Error`, which is the error of
the API) whose `Reason` is one of `ReasonMissingHeader`, `ReasonMalformedHeader`, `ReasonNoMatchingSignature` (wrong secret
or altered body), `ReasonTimestampOutOfTolerance` or `ReasonInvalidPayload` (the body is not JSON or not an event).
Answer `400` to all of them. The message never contains the secret, the body or the signature.

The `Order` of the `order.*` events carries `CreatorCode` (`nil` when the order was made without a creator code).

On success you get a `*gamecoin.WebhookEvent` with `ID`, `Type`, `CreatedAt`, `Env` (`"test"` or `"live"`), `Data` and
`RawData` (the JSON of `data`). The type decides what `Data` holds; use the accessors to avoid a type assertion:

| `Type` | Constant | `Data` | Accessor |
|---|---|---|---|
| `order.fulfilled` | `EventOrderFulfilled` | `*Order` | `Order()` |
| `order.refunded` | `EventOrderRefunded` | `*Order` | `Order()` |
| `order.chargeback` | `EventOrderChargeback` | `*Order` | `Order()` |
| `wallet.credited` | `EventWalletCredited` | `*WalletCredited` (`PlayerID`, `OrderID`, `Credits` of `Currency`, `Paid`, `Bonus`) | `WalletCredited()` |
| `webhook.test` | `EventWebhookTest` | `*WebhookTest` (`Message`) | `Test()` |

An event of a type this version does not know is returned as is, with `Data` set to the decoded JSON (a `map[string]any`)
rather than failing: your endpoint keeps working when GameCoin adds an event.

## Running the tests

```sh
go test ./...        # unit tests, no network
go vet ./...
go test -race ./...
```

No Go installed? Run them in a throwaway container: `docker run --rm -v "$PWD":/work -w /work golang:1.23 go test ./...`.

The unit tests use `httptest` servers on the loopback interface. The **integration tests** are skipped unless
`GAMECOIN_SECRET_KEY` and `GAMECOIN_BASE_URL` are set; they run against the sandbox game created by `npm run sdk:sandbox`
in the GameCoin application (currencies `gems` and `gold`, items `potion` and `fire-sword`, packs `starter` and `gems-500`):

```sh
GAMECOIN_SECRET_KEY=gc_sk_test_… GAMECOIN_BASE_URL=http://localhost:3400 go test ./...
```

## Status

Pre-release (`0.3.0`). The API runs in **test mode with simulated payments**; real payments are not available yet. The
module path and the repository name may change before the first public release.

## Licence

MIT, Copyright (c) 2026 Apilow.
