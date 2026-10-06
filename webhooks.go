package gamecoin

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

// WebhookSignatureHeader is the name of the HTTP header that carries the signature of a webhook call.
const WebhookSignatureHeader = "GameCoin-Signature"

// DefaultWebhookTolerance is how far the timestamp of a webhook call may be from the clock (in either direction) before
// ConstructEvent refuses it.
const DefaultWebhookTolerance = 5 * time.Minute

// WebhookEventType is the type of a webhook event.
type WebhookEventType string

// The types of webhook events GameCoin sends. A type this version does not know is passed through unchanged.
const (
	// EventOrderFulfilled is an order that is paid and delivered. WebhookEvent.Data is an *Order.
	EventOrderFulfilled WebhookEventType = "order.fulfilled"
	// EventOrderRefunded is an order that was refunded. WebhookEvent.Data is an *Order.
	EventOrderRefunded WebhookEventType = "order.refunded"
	// EventOrderChargeback is an order whose payment the payer disputes. WebhookEvent.Data is an *Order.
	EventOrderChargeback WebhookEventType = "order.chargeback"
	// EventWalletCredited is a delivery that added currency to a wallet. WebhookEvent.Data is a *WalletCredited.
	EventWalletCredited WebhookEventType = "wallet.credited"
	// EventWebhookTest is the event sent from the dashboard to try an endpoint. WebhookEvent.Data is a *WebhookTest.
	EventWebhookTest WebhookEventType = "webhook.test"
)

// WalletCredit is one currency credited by a delivery.
type WalletCredit struct {
	// Currency is the currency code ("gems").
	Currency string `json:"currency"`
	// Paid is the amount credited to the paid bucket.
	Paid int64 `json:"paid"`
	// Bonus is the amount credited to the bonus bucket.
	Bonus int64 `json:"bonus"`
}

// WalletCredited is the data of a EventWalletCredited event.
type WalletCredited struct {
	// PlayerID is the GameCoin player id.
	PlayerID string `json:"playerId"`
	// OrderID is the order that was delivered.
	OrderID string `json:"orderId"`
	// Credits lists what was added to the wallet.
	Credits []WalletCredit `json:"credits"`
}

// WebhookTest is the data of a EventWebhookTest event.
type WebhookTest struct {
	// Message is a short text.
	Message string `json:"message"`
}

// WebhookEvent is a verified webhook event, returned by ConstructEvent.
type WebhookEvent struct {
	// ID is the id of the event. The same event has the same ID every time it is sent: use it to process an event once.
	ID string
	// Type is the type of the event (EventOrderFulfilled, ...); a type this version does not know is kept as is.
	Type WebhookEventType
	// CreatedAt is when the event was created.
	CreatedAt time.Time
	// Env is the environment of the event: "test" or "live".
	Env string
	// Data is the payload: an *Order for the EventOrder… types, a *WalletCredited for EventWalletCredited and a
	// *WebhookTest for EventWebhookTest. For a type this version does not know it is the generic decoded JSON value
	// (a map[string]any, numbers as json.Number); RawData always holds the JSON itself.
	Data any
	// RawData is the "data" member of the event, as JSON.
	RawData json.RawMessage
}

// Order returns the order of an order.* event, nil for any other event.
func (e *WebhookEvent) Order() *Order {
	order, _ := e.Data.(*Order)
	return order
}

// WalletCredited returns the data of a wallet.credited event, nil for any other event.
func (e *WebhookEvent) WalletCredited() *WalletCredited {
	credited, _ := e.Data.(*WalletCredited)
	return credited
}

// Test returns the data of a webhook.test event, nil for any other event.
func (e *WebhookEvent) Test() *WebhookTest {
	test, _ := e.Data.(*WebhookTest)
	return test
}

// WebhookSignatureReason says why ConstructEvent refused a call.
type WebhookSignatureReason string

// The values of WebhookSignatureError.Reason.
const (
	// ReasonMissingHeader means the GameCoin-Signature header is absent or empty.
	ReasonMissingHeader WebhookSignatureReason = "missing_header"
	// ReasonMalformedHeader means the header is not "t=<unix seconds>,v1=<hex>".
	ReasonMalformedHeader WebhookSignatureReason = "malformed_header"
	// ReasonNoMatchingSignature means no signature of the header matches: wrong secret, or a body that was changed
	// (read it raw, before any JSON parsing).
	ReasonNoMatchingSignature WebhookSignatureReason = "no_matching_signature"
	// ReasonTimestampOutOfTolerance means the signature is right but the timestamp is too far from the clock (a replay).
	ReasonTimestampOutOfTolerance WebhookSignatureReason = "timestamp_out_of_tolerance"
	// ReasonInvalidPayload means the signature is right but the body is not a webhook event.
	ReasonInvalidPayload WebhookSignatureReason = "invalid_payload"
)

// WebhookSignatureError is returned by ConstructEvent when a call must not be trusted. It is not a *Error, which is the
// error of the API. Its message never contains the secret, the body or the signature.
//
//	event, err := gamecoin.ConstructEvent(body, r.Header.Get(gamecoin.WebhookSignatureHeader), secret)
//	var sigErr *gamecoin.WebhookSignatureError
//	if errors.As(err, &sigErr) { /* answer 400; sigErr.Reason says why */ }
type WebhookSignatureError struct {
	// Reason is why the call was refused.
	Reason WebhookSignatureReason

	message string
}

// Error implements the error interface.
func (e *WebhookSignatureError) Error() string {
	return "gamecoin: webhook refused (" + string(e.Reason) + "): " + e.message
}

func signatureError(reason WebhookSignatureReason, message string) error {
	return &WebhookSignatureError{Reason: reason, message: message}
}

// WebhookOption configures ConstructEvent. See WithTolerance and WithClock.
type WebhookOption func(*webhookConfig)

type webhookConfig struct {
	tolerance time.Duration
	now       func() time.Time
}

// WithTolerance sets how far the timestamp of the call may be from the clock, in either direction. The default is
// DefaultWebhookTolerance (5 minutes); 0 turns the timestamp check off. A negative value is an invalid argument.
func WithTolerance(d time.Duration) WebhookOption {
	return func(c *webhookConfig) { c.tolerance = d }
}

// WithClock replaces the clock used for the timestamp check (tests). The default is time.Now.
func WithClock(now func() time.Time) WebhookOption {
	return func(c *webhookConfig) { c.now = now }
}

// ConstructEvent checks the signature of a webhook call and returns the event it carries. It is a pure function: it
// needs no Client and makes no request.
//
//   - rawBody is the body exactly as received (io.ReadAll(r.Body)), never re-serialised JSON: one changed character
//     breaks the signature.
//   - signatureHeader is the value of the GameCoin-Signature header (WebhookSignatureHeader).
//   - secret is the signing secret of the endpoint ("whsec_gc_…"). Surrounding white space is trimmed.
//
// Every "v1" signature of the header is compared in constant time; keys the SDK does not know are ignored. A call that
// must not be trusted returns a *WebhookSignatureError (use errors.As); an empty secret or a negative tolerance wraps
// ErrInvalidArgument instead.
func ConstructEvent(rawBody []byte, signatureHeader, secret string, opts ...WebhookOption) (*WebhookEvent, error) {
	cfg := webhookConfig{tolerance: DefaultWebhookTolerance, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, argError("the webhook signing secret must not be empty")
	}
	if cfg.tolerance < 0 {
		return nil, argError("the webhook tolerance must not be negative")
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}

	timestamp, signatures, err := parseSignatureHeader(signatureHeader)
	if err != nil {
		return nil, err
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp.text))
	mac.Write([]byte{'.'})
	mac.Write(rawBody)
	expected := mac.Sum(nil)
	matched := false
	for _, candidate := range signatures {
		given, decodeErr := hex.DecodeString(candidate)
		if decodeErr == nil && hmac.Equal(given, expected) { // no early exit: every signature is compared
			matched = true
		}
	}
	if !matched {
		return nil, signatureError(ReasonNoMatchingSignature, "none of the signatures matches the body: check the signing secret, and that the body is the raw one")
	}

	if cfg.tolerance > 0 {
		age := cfg.now().Sub(time.Unix(timestamp.seconds, 0))
		if age < 0 {
			age = -age
		}
		if age > cfg.tolerance {
			return nil, signatureError(ReasonTimestampOutOfTolerance, "the timestamp is too far from the clock")
		}
	}
	return parseEvent(rawBody)
}

type signatureTimestamp struct {
	text    string // as written in the header: it is what was signed
	seconds int64
}

func parseSignatureHeader(header string) (signatureTimestamp, []string, error) {
	var zero signatureTimestamp
	if strings.TrimSpace(header) == "" {
		return zero, nil, signatureError(ReasonMissingHeader, "the "+WebhookSignatureHeader+" header is missing or empty")
	}
	var timestamp signatureTimestamp
	var signatures []string
	seenTimestamp := false
	for _, part := range strings.Split(header, ",") {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "t":
			value = strings.TrimSpace(value)
			seconds, err := parseDigits(value)
			if err != nil || seenTimestamp {
				return zero, nil, signatureError(ReasonMalformedHeader, "the timestamp of the header is not a single unix time in seconds")
			}
			timestamp, seenTimestamp = signatureTimestamp{text: value, seconds: seconds}, true
		case "v1":
			if value = strings.TrimSpace(value); value != "" {
				signatures = append(signatures, value)
			}
		}
	}
	if !seenTimestamp || len(signatures) == 0 {
		return zero, nil, signatureError(ReasonMalformedHeader, "the header must look like t=<unix seconds>,v1=<hex signature>")
	}
	return timestamp, signatures, nil
}

// parseDigits reads a non-negative integer made of ASCII digits only (no sign, no space).
func parseDigits(text string) (int64, error) {
	if text == "" {
		return 0, errors.New("empty")
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return 0, errors.New("not a digit")
		}
	}
	return strconv.ParseInt(text, 10, 64)
}

// envelope is the common shape of every event.
type envelope struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"createdAt"`
	Env       string          `json:"env"`
	Data      json.RawMessage `json:"data"`
}

func invalidPayload(why string) error {
	return signatureError(ReasonInvalidPayload, "the body is not a GameCoin event: "+why)
}

func parseEvent(rawBody []byte) (*WebhookEvent, error) {
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	var document any
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, invalidPayload("it is not JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, invalidPayload("it is not JSON")
	}
	object, ok := document.(map[string]any)
	if !ok {
		return nil, invalidPayload("it is not a JSON object")
	}
	for _, name := range []string{"id", "type", "createdAt", "env"} {
		if text, isString := object[name].(string); !isString || text == "" {
			return nil, invalidPayload(name + " must be a non-empty string")
		}
	}
	data, isObject := object["data"].(map[string]any)
	if !isObject {
		return nil, invalidPayload("data must be an object")
	}

	var env envelope
	if err := json.Unmarshal(rawBody, &env); err != nil {
		return nil, invalidPayload("createdAt must be an ISO 8601 date-time")
	}
	event := &WebhookEvent{
		ID:        env.ID,
		Type:      WebhookEventType(env.Type),
		CreatedAt: env.CreatedAt,
		Env:       env.Env,
		RawData:   env.Data,
	}

	var typed any
	switch event.Type {
	case EventOrderFulfilled, EventOrderRefunded, EventOrderChargeback:
		typed = new(Order)
	case EventWalletCredited:
		typed = new(WalletCredited)
	case EventWebhookTest:
		typed = new(WebhookTest)
	default:
		event.Data = data // an unknown type is passed through
		return event, nil
	}
	if reason, _ := parseResponse(env.Data, typed); reason != "" {
		return nil, invalidPayload("its data is not the expected shape: " + reason)
	}
	event.Data = typed
	return event, nil
}
