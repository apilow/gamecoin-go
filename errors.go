package gamecoin

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// The values of Error.Code: the error codes of the GameCoin API, plus three codes raised by the SDK itself
// (CodeNetworkError, CodeTimeout and CodeInvalidResponse).
const (
	// CodeValidationFailed is a 400: body or query invalid (see Error.Details["fieldErrors"]).
	CodeValidationFailed = "VALIDATION_FAILED"
	// CodeIdempotencyKeyRequired is a 400: missing or empty Idempotency-Key on a mutation. The SDK always sends one.
	CodeIdempotencyKeyRequired = "IDEMPOTENCY_KEY_REQUIRED"
	// CodeUnauthenticated is a 401: missing, unknown or revoked key; invalid or expired player token.
	CodeUnauthenticated = "UNAUTHENTICATED"
	// CodePaymentFailed is a 402: payment refused, or no payment provider for this environment.
	CodePaymentFailed = "PAYMENT_FAILED"
	// CodeForbidden is a 403: key kind not allowed here, anonymous players disabled, or item not client-purchasable.
	CodeForbidden = "FORBIDDEN"
	// CodePlayerBlocked is a 403: the player is blocked.
	CodePlayerBlocked = "PLAYER_BLOCKED"
	// CodeEnvNotEnabled is a 403: the live environment was used by a studio that is not verified.
	CodeEnvNotEnabled = "ENV_NOT_ENABLED"
	// CodeNotFound is a 404: unknown player, currency, item, pack or order in this game and environment.
	CodeNotFound = "NOT_FOUND"
	// CodeInsufficientFunds is a 409: the wallet (or the item quantity) is too low (Details "required" and "available").
	CodeInsufficientFunds = "INSUFFICIENT_FUNDS"
	// CodeLimitReached is a 409: maxOwned or maxPerPlayer exceeded.
	CodeLimitReached = "LIMIT_REACHED"
	// CodePackNotAvailable is a 409: the pack is outside its sale window (Details "startsAt" and "endsAt": ISO 8601 text, or
	// nil when there is no such bound).
	CodePackNotAvailable = "PACK_NOT_AVAILABLE"
	// CodePackSoldOut is a 409: the total stock of the pack is exhausted (Details "packSku").
	CodePackSoldOut = "PACK_SOLD_OUT"
	// CodeIdempotencyConflict is a 409: the idempotency key was already used with another request.
	CodeIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
	// CodeConflict is a 409: invalid state transition (for example the refund of an unpaid order).
	CodeConflict = "CONFLICT"
	// CodeReadOnly is a 423: the studio is suspended and its space is read-only.
	CodeReadOnly = "READ_ONLY"
	// CodeRateLimited is a 429: too many requests (see Error.RetryAfter).
	CodeRateLimited = "RATE_LIMITED"
	// CodeInternal is a 500: unexpected error on the GameCoin side.
	CodeInternal = "INTERNAL"

	// CodeNetworkError is raised by the SDK when no response was received (DNS, connection refused, reset...).
	// Error.Status is 0.
	CodeNetworkError = "NETWORK_ERROR"
	// CodeTimeout is raised by the SDK when no response arrived within the timeout (WithTimeout). Error.Status is 0.
	CodeTimeout = "TIMEOUT"
	// CodeInvalidResponse is raised by the SDK when the body of a response is not the expected JSON.
	CodeInvalidResponse = "INVALID_RESPONSE"
)

// ErrInvalidArgument is wrapped by every error returned for an argument that is rejected before any request (an empty
// player, a negative amount...). Test for it with errors.Is.
var ErrInvalidArgument = errors.New("gamecoin: invalid argument")

// Error is the error returned by every call that fails after a request was attempted. The secret key never appears in
// it.
//
// Use errors.As to get it:
//
//	var gcErr *gamecoin.Error
//	if errors.As(err, &gcErr) && gcErr.Code == gamecoin.CodeInsufficientFunds { ... }
type Error struct {
	// Code is the API code (CodeInsufficientFunds, CodeNotFound...), or CodeNetworkError, CodeTimeout or
	// CodeInvalidResponse raised by the SDK.
	Code string
	// Status is the HTTP status, 0 when no response was received.
	Status int
	// Message is the message of the API, or a short description.
	Message string
	// Details is the "details" object of the API ("required" and "available" for CodeInsufficientFunds, "fieldErrors"
	// for CodeValidationFailed...); empty when absent. Numbers are json.Number: see DetailInt64. Meant for developers.
	Details map[string]any
	// RetryAfter is the Retry-After header of the response, when the API sent one (zero otherwise). It is a
	// time.Duration; the header itself is in seconds.
	RetryAfter time.Duration

	cause         error
	hasRetryAfter bool // the header was present (a Retry-After of 0 means "retry now")
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("gamecoin: %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("gamecoin: %s (HTTP %d): %s", e.Code, e.Status, e.Message)
}

// Unwrap returns the underlying cause of a CodeNetworkError or CodeTimeout (for instance context.DeadlineExceeded),
// nil otherwise.
func (e *Error) Unwrap() error { return e.cause }

// DetailInt64 reads an integer from Details, such as "required" or "available" of a CodeInsufficientFunds error. The
// boolean is false when the key is absent or is not an integer.
func (e *Error) DetailInt64(key string) (int64, bool) {
	switch v := e.Details[key].(type) {
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case float64:
		return int64(v), float64(int64(v)) == v
	case int64:
		return v, true
	case int:
		return int64(v), true
	}
	return 0, false
}

// IsCode reports whether err is (or wraps) an *Error with the given code. It is shorthand for errors.As followed by a
// comparison.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}
