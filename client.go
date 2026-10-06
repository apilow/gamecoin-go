package gamecoin

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
)

// Client is the client of the GameCoin server API. Create one per application with New and keep it: it is safe for
// concurrent use and reuses its connections.
//
// The secret key is held out of reach: it never appears in an error, and the string form of a Client (String, %v, %+v,
// %#v) shows its prefix only ("gc_sk_test_…").
type Client struct {
	// Catalog gives access to the catalog of the game.
	Catalog *CatalogService
	// Players gives access to players.
	Players *PlayersService
	// Wallet gives access to wallets: balances, grants and spends.
	Wallet *WalletService
	// Ledger gives access to the ledger of a player.
	Ledger *LedgerService
	// Inventory gives access to inventories.
	Inventory *InventoryService
	// Purchases buys items with a currency.
	Purchases *PurchasesService
	// Checkouts sells packs for real money.
	Checkouts *CheckoutsService
	// Codes redeems gift codes.
	Codes *CodesService
	// Orders reads and refunds orders.
	Orders *OrdersService

	core        *core
	environment string
	maskedKey   string
}

// New creates a client from a secret key ("gc_sk_test_…" or "gc_sk_live_…").
//
// Leading and trailing white space (a newline from a file, say) is trimmed from the key. It returns an error wrapping
// ErrInvalidArgument when the key is not a secret key (a publishable key "gc_pk_…" belongs in the browser SDK) or when an
// option is invalid. The key is never echoed in the message.
func New(secretKey string, opts ...Option) (*Client, error) {
	secretKey = strings.TrimSpace(secretKey) // a key read from a file often carries a trailing newline
	environment, maskedKey, err := parseSecretKey(secretKey)
	if err != nil {
		return nil, err
	}

	cfg := config{baseURL: DefaultBaseURL, timeout: defaultTimeout, maxRetries: defaultMaxRetries}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.maxRetries < 0 || cfg.maxRetries > 10 {
		return nil, argError("WithMaxRetries must be an integer from 0 to 10")
	}
	if cfg.timeout <= 0 {
		return nil, argError("WithTimeout must be positive")
	}
	baseURL, err := normalizeBaseURL(cfg.baseURL)
	if err != nil {
		return nil, err
	}

	httpClient := cfg.httpClient
	if httpClient == nil {
		httpClient = &http.Client{
			// A redirect would silently turn a POST into a GET: surface it as an error instead.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	sleep := cfg.sleep
	if sleep == nil {
		sleep = defaultSleep
	}
	jitter := cfg.jitter
	if jitter == nil {
		jitter = rand.Float64
	}

	c := &core{
		http:       httpClient,
		baseURL:    baseURL,
		secretKey:  secretKey,
		timeout:    cfg.timeout,
		maxRetries: cfg.maxRetries,
		userAgent:  newUserAgent(),
		sleep:      sleep,
		jitter:     jitter,
	}
	return &Client{
		Catalog:     &CatalogService{c},
		Players:     &PlayersService{c},
		Wallet:      &WalletService{c},
		Ledger:      &LedgerService{c},
		Inventory:   &InventoryService{c},
		Purchases:   &PurchasesService{c},
		Checkouts:   &CheckoutsService{c},
		Codes:       &CodesService{c},
		Orders:      &OrdersService{c},
		core:        c,
		environment: environment,
		maskedKey:   maskedKey,
	}, nil
}

// Environment is "test" or "live", read from the prefix of the secret key.
func (c *Client) Environment() string { return c.environment }

// BaseURL is the base URL in use, without a trailing slash.
func (c *Client) BaseURL() string { return c.core.baseURL }

// String describes the client; the secret key shows as its prefix only ("gc_sk_test_…").
func (c *Client) String() string {
	return fmt.Sprintf("gamecoin.Client{Environment: %s, BaseURL: %s, SecretKey: %s}", c.environment, c.core.baseURL, c.maskedKey)
}

// GoString describes the client like String (the %#v verb).
func (c *Client) GoString() string { return c.String() }

// parseSecretKey returns the environment and the masked form ("gc_sk_test_…") of a secret key; the error never echoes the
// key.
func parseSecretKey(key string) (environment, masked string, err error) {
	switch {
	case strings.HasPrefix(key, "gc_sk_test_") && len(key) > len("gc_sk_test_"):
		return "test", "gc_sk_test_…", nil
	case strings.HasPrefix(key, "gc_sk_live_") && len(key) > len("gc_sk_live_"):
		return "live", "gc_sk_live_…", nil
	case strings.HasPrefix(key, "gc_pk_"):
		return "", "", argError("this is a publishable key (gc_pk_...), which belongs in the browser SDK; the server SDK needs a secret key (gc_sk_test_... or gc_sk_live_...)")
	}
	return "", "", argError("the server SDK needs a secret key (gc_sk_test_... or gc_sk_live_...)")
}

// normalizeBaseURL reduces the URL to the origin of the service plus an optional path prefix, without trailing slashes.
func normalizeBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", argError("WithBaseURL must be an absolute http(s) URL without credentials, query or fragment, such as https://gamecoin.apilow.com")
	}
	return u.Scheme + "://" + u.Host + strings.TrimRight(u.EscapedPath(), "/"), nil
}
