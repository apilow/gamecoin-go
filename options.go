package gamecoin

import (
	"context"
	"net/http"
	"time"
)

// DefaultBaseURL is the production base URL.
const DefaultBaseURL = "https://gamecoin.apilow.com"

const (
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 2
)

// config is what the Option functions fill in.
type config struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
	maxRetries int

	// Unexported hooks, reachable from the tests of this package only.
	sleep  func(ctx context.Context, d time.Duration) error
	jitter func() float64
}

// Option configures a Client. See WithBaseURL, WithHTTPClient, WithTimeout and WithMaxRetries.
type Option func(*config)

// WithBaseURL sets the base URL of GameCoin: the origin of the service, to which the SDK appends "/api/v1". Trailing
// slashes are dropped and a path prefix is kept (for a gateway); anything but an http(s) URL without credentials, query or
// fragment is refused. The default is DefaultBaseURL.
func WithBaseURL(baseURL string) Option {
	return func(c *config) { c.baseURL = baseURL }
}

// WithHTTPClient makes the SDK use your own *http.Client (a custom transport, a proxy, instrumentation...). The SDK never
// modifies it. Timeouts are applied per attempt through the request context (see WithTimeout), whatever the client's own
// Timeout says; redirects follow your client's policy.
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) { c.httpClient = client }
}

// WithTimeout sets the timeout of each HTTP attempt (a retry gets a fresh one). The default is 30 seconds.
func WithTimeout(timeout time.Duration) Option {
	return func(c *config) { c.timeout = timeout }
}

// WithMaxRetries sets the number of automatic retries after a network error, a timeout, a 5xx or a 429; the default is 2,
// so three attempts at most. 0 turns retries off; the maximum is 10.
func WithMaxRetries(n int) Option {
	return func(c *config) { c.maxRetries = n }
}

// CallOption customises one call of a method that changes something (Wallet.Grant, Wallet.Spend, Inventory.Grant,
// Inventory.Consume, Purchases.Create, Checkouts.Create).
type CallOption func(*callConfig)

type callConfig struct {
	idempotencyKey *string
}

// WithIdempotencyKey sets the Idempotency-Key of the call (1 to 100 printable ASCII characters), to make a retry of your
// own safe: the same key with the same request returns the original response (Replayed is then true); the same key with
// another request fails with CodeIdempotencyConflict. Without this option the SDK generates a UUID v4 once per call and
// reuses it for every automatic retry of that call.
func WithIdempotencyKey(key string) CallOption {
	return func(c *callConfig) { c.idempotencyKey = &key }
}
