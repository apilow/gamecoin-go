# Changelog

All notable changes to this module are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to
[Semantic Versioning](https://semver.org/).

## [0.4.0] - Unreleased

Catalog management: create, change and archive currencies, items and packs.

### Added
- `client.Currencies`, `client.Items` and `client.Packs`, each with `Create`, `Get`, `Update`, `Archive` and `List`: the catalog
  management API (`/currencies`, `/items`, `/packs`). Twelve services and thirty-two calls in all.
- Results `ManagedCurrency`, `ManagedItem` and `ManagedPack` (the catalog objects plus `Status`, `CreatedAt` and `UpdatedAt`; a
  pack also has `SortOrder` and `Availability` as `ManagedPackAvailability` with `StartsAt`, `EndsAt`, `Stock`, `Founder` and
  `Sold`), and the `CatalogStatus` and `PackStatus` values.
- `Update` follows the `Players.Upsert` convention: a nil field is unchanged, and a `Clear` list erases a description, a price,
  a badge or a limit. `Grants`, `Items`, `Metadata` and `Availability` replace the old value as a whole.
- `Archive` returns the archived resource; `List` leaves archived ones out unless `&ListParams{IncludeArchived: true}`.
- `gamecoin.Ptr(v)`, a generic helper that returns a pointer to a copy of `v`, for the optional fields of the params.

### Notes
- Each environment has its own catalog: a resource created with a test key exists in test only, and a live key never sees it.
- A create has no idempotency key (the code or sku is its natural key; a taken one is `CONFLICT` with `SKU_TAKEN` or
  `CODE_TAKEN`), so it is retried on 429 only, like `Orders.Refund`. Updates, archives and reads follow the usual rules.
- `Version` and the user agent are `0.4.0`.

## [0.3.0] - Unreleased

### Added
- `client.Codes.Redeem(ctx, player, code, opts...)`: a player uses a gift code (`POST /players/{player}/codes/redeem`).
  It returns a `*Redemption` (`Grants` with the balance after the credit, `Items`, `Skipped`, `Replayed`) and is
  idempotent like the other writes. The code is sent as it is. A refused code is `VALIDATION_FAILED` with
  `Details["fieldErrors"]["code"] == ["CODE_INVALID"]`.
- `CheckoutParams.CreatorCode`: the creator code typed by the player (not sent when empty), and `Order.CreatorCode`
  (`*OrderCreatorCode` with `Code` and `CreatorName`, `nil` without a code), also in the `order.*` webhook events.
- `Pack.Availability` (`*PackAvailability` with `StartsAt`, `EndsAt`, `Remaining`, `Founder`; `nil` for an ordinary
  pack), `Catalog.Prelaunch` and `Catalog.LaunchAt`.
- The error codes `CodePackNotAvailable` (`Details` `startsAt` and `endsAt`) and `CodePackSoldOut` (`Details` `packSku`).

### Changed
- The four fields added in 0.3 are tolerated when absent (SDK-SPEC section 10.1): `Pack.Availability` reads as `nil`,
  `Catalog.Prelaunch` as `false`, `Catalog.LaunchAt` as `nil` and `Order.CreatorCode` as `nil`, so an older server or a
  webhook event queued before 0.3 stays readable. A field that is present with the wrong type or shape (an `availability`
  without `founder`, a `creatorCode` without `creatorName`, an unreadable date) is still `INVALID_RESPONSE`
  (`ReasonInvalidPayload` for a webhook event).
- The version is `0.3.0` (`Version` and the user agent).

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
