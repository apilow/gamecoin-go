// Package gamecoin is the server SDK of Apilow GameCoin, the bank of your game: virtual currencies, inventory and
// sales behind one API.
//
// Use it from your game backend to grant and spend coins, manage items, and sell packs with real money. It
// authenticates with a secret key ("gc_sk_test_…" or "gc_sk_live_…") that must never be shipped in a game client
// or a web page; the browser has its own SDK, which uses the publishable key.
//
// The package depends on the standard library only and supports Go 1.21 and later.
//
// # Quickstart
//
//	client, err := gamecoin.New(os.Getenv("GAMECOIN_SECRET_KEY"))
//	if err != nil {
//		log.Fatal(err)
//	}
//	player := gamecoin.Ext("user-42") // your own id for the player
//
//	movement, err := client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gems", Amount: 100, Reason: "level_up"})
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(movement.Balance.Total, movement.Replayed)
//
// # Errors
//
// A call that fails returns a *Error. Use errors.As and the Code constants to react to a particular failure:
//
//	var gcErr *gamecoin.Error
//	if errors.As(err, &gcErr) && gcErr.Code == gamecoin.CodeInsufficientFunds {
//		// gcErr.Details["required"], gcErr.Details["available"]
//	}
//
// Arguments rejected before any request wrap ErrInvalidArgument. A cancelled or expired context returns the context's
// own error.
//
// # Idempotency and retries
//
// Every call that changes a wallet or an inventory, redeems a gift code, or creates a checkout, carries an Idempotency-Key. When you give
// none (see WithIdempotencyKey) the SDK generates a UUID v4 once per call and reuses it for each automatic retry, so a
// retry can never credit twice. Network errors, timeouts, 5xx and 429 responses are retried (WithMaxRetries, default 2)
// with exponential backoff; a Retry-After header is honoured up to 30 seconds. Orders.Refund has no idempotency key and
// is retried on 429 only.
//
// # Webhooks
//
// ConstructEvent verifies the signature of a call that GameCoin makes to your server and returns the typed event. Give
// it the raw body (io.ReadAll(r.Body)), never a re-serialised one:
//
//	event, err := gamecoin.ConstructEvent(body, r.Header.Get(gamecoin.WebhookSignatureHeader), secret)
//	var sigErr *gamecoin.WebhookSignatureError
//	if errors.As(err, &sigErr) { /* answer 400 */ }
package gamecoin
