# Changelog

All notable changes to this module are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to
[Semantic Versioning](https://semver.org/).

## [0.2.0] - Unreleased

### Added
- `CheckoutParams.Locale` (`"fr"`, `"en"` or `"nl"`): the language of the hosted payment page, the receipt and the e-mails
  of the order. It is not sent when empty.
- Webhook signature verification: `gamecoin.ConstructEvent(rawBody, signatureHeader, secret, opts...)`, a pure function
  that checks the `GameCoin-Signature` header (HMAC-SHA256, constant-time comparison of every `v1`, 5-minute timestamp
  tolerance by default, `WithTolerance` and `WithClock`) and returns a typed `*WebhookEvent`.
- `*WebhookSignatureError` with a `Reason` (`ReasonMissingHeader`, `ReasonMalformedHeader`, `ReasonNoMatchingSignature`,
  `ReasonTimestampOutOfTolerance`, `ReasonInvalidPayload`); it is not a `*gamecoin.Error`.
- Typed events (`Order` for `order.*`, `WalletCredited`, `WebhookTest`; an unknown type is passed through), the
  `EventOrderFulfilled`… constants and `WebhookSignatureHeader`.
- A Webhooks section in the README and `examples/webhook`.

## [0.1.0] - Unreleased

First pre-release.

### Added
- `gamecoin.New` and a `Client` with `Catalog`, `Players`, `Wallet`, `Ledger`, `Inventory`, `Purchases`, `Checkouts` and
  `Orders` services covering the whole GameCoin server API v1.
- `Ext` and `PlayerID` helpers; path segments are URL-encoded exactly once.
- Idempotency keys on every mutation (UUID v4 generated once per call and kept across retries, or your own through
  `WithIdempotencyKey`), and a `Replayed` flag on the results.
- Automatic retries on network errors, timeouts, `5xx` and `429` (exponential backoff with jitter, `Retry-After` honoured
  up to 30 s). Refunds are retried on `429` only. A cancelled context is never retried.
- `*gamecoin.Error` with `Code`, `Status`, `Message`, `Details` and `RetryAfter`, the `Code…` constants, `IsCode` and
  `ErrInvalidArgument` for arguments rejected before any request.
- `Ledger.Iterate`, a lazy iterator (`Next`, `Entry`, `Err`) that needs no Go newer than 1.21.
- Strict checking of response shapes (`INVALID_RESPONSE`), standard library only.
