package gamecoin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	apiPath = "/api/v1"
	// maxRetryDelay is the longest wait the SDK accepts: a Retry-After beyond it is raised, not waited for.
	maxRetryDelay = 30 * time.Second
	// maxResponseBytes bounds what the SDK reads from a response.
	maxResponseBytes = 32 << 20
)

type retryPolicy int

const (
	// retryAll retries network errors, timeouts, 5xx and 429.
	retryAll retryPolicy = iota
	// retryRateLimitOnly retries 429 only (a call with no idempotency key: a blind retry could repeat the action).
	retryRateLimitOnly
)

// request is one API call, before any HTTP detail.
type request struct {
	method         string
	path           string // starts with "/", already URL-encoded
	query          string // "" or "a=b&c=d", already URL-encoded
	body           []byte // nil: no body
	idempotencyKey string // "": none
	retry          retryPolicy
}

type rawResponse struct {
	status     int
	body       []byte
	replayed   bool
	retryAfter time.Duration
	hasRetry   bool
}

// core is the HTTP transport shared by every service: headers, one URL encoding, retries, error mapping. It holds the
// secret key, so it is only ever reached through a pointer (printing a Client never dereferences it) and it redacts
// itself when formatted.
type core struct {
	http       *http.Client
	baseURL    string
	secretKey  string
	timeout    time.Duration
	maxRetries int
	userAgent  string
	sleep      func(ctx context.Context, d time.Duration) error
	jitter     func() float64
}

// String never shows the key.
func (c *core) String() string { return "gamecoin: transport" }

// GoString never shows the key.
func (c *core) GoString() string { return "gamecoin: transport" }

func newUserAgent() string {
	return fmt.Sprintf("gamecoin-go/%s (go/%s)", Version, strings.TrimPrefix(runtime.Version(), "go"))
}

func defaultSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// do performs a request with retries and decodes the success body into out (when out is not nil). It reports whether the
// response was an idempotent replay. A failure is a *Error, or the error of ctx when the caller's context ended.
func (c *core) do(ctx context.Context, req request, out any) (replayed bool, err error) {
	target := c.baseURL + apiPath + req.path
	if req.query != "" {
		target += "?" + req.query
	}

	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}

		raw, err := c.attempt(ctx, req, target)
		var failure *Error
		if err != nil {
			if !errors.As(err, &failure) {
				return false, err // the caller's context ended
			}
		} else if raw.status >= 200 && raw.status < 300 {
			if err := decodeSuccess(raw, out); err != nil {
				return false, err
			}
			return raw.replayed, nil
		} else {
			failure = c.errorFromResponse(raw)
		}

		if attempt >= c.maxRetries || !retryable(failure, req.retry) {
			return false, failure
		}
		wait, ok := c.retryDelay(failure, attempt)
		if !ok {
			return false, failure
		}
		if err := c.sleep(ctx, wait); err != nil {
			return false, err
		}
	}
}

func retryable(failure *Error, policy retryPolicy) bool {
	if failure.Status == http.StatusTooManyRequests {
		return true
	}
	return policy == retryAll && (failure.Status == 0 || failure.Status >= 500)
}

// retryDelay is how long to wait before the next attempt; false when the server asks for more than the cap.
func (c *core) retryDelay(failure *Error, attempt int) (time.Duration, bool) {
	if failure.hasRetryAfter {
		if failure.RetryAfter > maxRetryDelay {
			return 0, false
		}
		return failure.RetryAfter, true
	}
	seconds := 0.5 * math.Pow(2, float64(attempt)) * (0.75 + 0.5*c.jitter()) // 0.5 s x 2^attempt, +/- 25 % jitter
	delay := time.Duration(seconds * float64(time.Second))
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	return delay, true
}

// attempt sends the request once. The error is a *Error (network error, timeout) or the caller's context error.
func (c *core) attempt(ctx context.Context, req request, target string) (*rawResponse, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var body io.Reader
	if req.body != nil {
		body = bytes.NewReader(req.body)
	}
	httpReq, err := http.NewRequestWithContext(attemptCtx, req.method, target, body)
	if err != nil {
		return nil, &Error{Code: CodeNetworkError, Message: c.scrub("Invalid request: " + err.Error()), cause: err}
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.secretKey)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.userAgent)
	if req.idempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", req.idempotencyKey)
	}
	if req.body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, c.classify(ctx, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, c.classify(ctx, err)
	}

	raw := &rawResponse{status: resp.StatusCode, body: data}
	raw.replayed = strings.EqualFold(strings.TrimSpace(resp.Header.Get("Idempotent-Replayed")), "true")
	raw.retryAfter, raw.hasRetry = parseRetryAfter(resp.Header.Get("Retry-After"))
	return raw, nil
}

// classify turns a transport failure into the caller's context error, a timeout or a network error.
func (c *core) classify(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return &Error{
			Code:    CodeTimeout,
			Message: fmt.Sprintf("No response within %s.", c.timeout),
			cause:   err,
		}
	}
	return &Error{Code: CodeNetworkError, Message: c.scrub("Network error: " + err.Error()), cause: err}
}

// parseRetryAfter reads seconds or an HTTP date; false when the header is absent or malformed.
func parseRetryAfter(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		if math.IsNaN(seconds) || seconds < 0 {
			return 0, false
		}
		if seconds > 31536000 {
			seconds = 31536000
		}
		return time.Duration(seconds * float64(time.Second)), true
	}
	if when, err := http.ParseTime(value); err == nil {
		if wait := time.Until(when); wait > 0 {
			return wait, true
		}
		return 0, true
	}
	return 0, false
}

// errorFromResponse maps a non-2xx response to an *Error.
func (c *core) errorFromResponse(raw *rawResponse) *Error {
	var envelope struct {
		Error *struct {
			Code    string         `json:"code"`
			Message *string        `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw.body))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err == nil && envelope.Error != nil && envelope.Error.Code != "" && envelope.Error.Message != nil {
		message := *envelope.Error.Message
		details := envelope.Error.Details
		if details == nil {
			details = map[string]any{}
		}
		return &Error{
			Code:          envelope.Error.Code,
			Status:        raw.status,
			Message:       c.scrub(message),
			Details:       details,
			RetryAfter:    raw.retryAfter,
			hasRetryAfter: raw.hasRetry,
		}
	}

	message := fmt.Sprintf("Unexpected response (HTTP %d): the body is not a GameCoin error.", raw.status)
	if raw.status >= 300 && raw.status < 400 {
		message = fmt.Sprintf("Unexpected redirect (HTTP %d): check the base URL (scheme and host).", raw.status)
	}
	return &Error{
		Code:          CodeInvalidResponse,
		Status:        raw.status,
		Message:       message,
		Details:       map[string]any{},
		RetryAfter:    raw.retryAfter,
		hasRetryAfter: raw.hasRetry,
	}
}

func decodeSuccess(raw *rawResponse, out any) error {
	reason, cause := parseResponse(raw.body, out)
	if reason == "" {
		return nil
	}
	return &Error{
		Code:    CodeInvalidResponse,
		Status:  raw.status,
		Message: fmt.Sprintf("The response of the API (HTTP %d) is not the expected JSON: %s.", raw.status, reason),
		Details: map[string]any{},
		cause:   cause,
	}
}

// scrub is defence in depth: whatever the server or the network stack says, the secret key never leaves in a message.
func (c *core) scrub(text string) string {
	if c.secretKey == "" {
		return text
	}
	return strings.ReplaceAll(text, c.secretKey, "[redacted]")
}

// escapeSegment URL-encodes a path segment or a query value exactly once, the way encodeURIComponent does (everything
// but the RFC 3986 unreserved characters is percent-encoded, byte by byte, so non-ASCII text goes out as UTF-8).
func escapeSegment(s string) string {
	const upperHex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0x0F])
		}
	}
	return b.String()
}

// newUUIDv4 returns a random (version 4) UUID.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("gamecoin: cannot generate an idempotency key: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:], b[10:])
	return string(out[:]), nil
}

// resolveIdempotencyKey returns the caller's key (validated) or a fresh UUID v4 that every retry of the call will reuse.
func resolveIdempotencyKey(opts []CallOption) (string, error) {
	var cc callConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cc)
		}
	}
	if cc.idempotencyKey == nil {
		return newUUIDv4()
	}
	key := *cc.idempotencyKey
	if len(key) < 1 || len(key) > 100 {
		return "", argError("the idempotency key must be 1 to 100 characters long")
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x20 || key[i] > 0x7E {
			return "", argError("the idempotency key must only contain printable ASCII characters")
		}
	}
	return key, nil
}

// argError builds a local rejection. Messages name the argument, never its value (a value could be a secret).
func argError(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidArgument, message)
}

func marshalBody(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, fmt.Errorf("gamecoin: cannot encode the request: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
