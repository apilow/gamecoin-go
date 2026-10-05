package gamecoin

import (
	"context"
	"net/http"
)

// WalletService reads wallets and moves currency (GET /players/{player}/wallet and the grant and spend actions). Get it
// from Client.Wallet.
type WalletService struct{ c *core }

// movementBody is the body of a wallet grant or spend.
type movementBody struct {
	Currency string            `json:"currency"`
	Amount   int64             `json:"amount"`
	Reason   string            `json:"reason,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Get returns the balances of a player: one line per active currency, zeros included.
func (s *WalletService) Get(ctx context.Context, player PlayerRef) (*Wallet, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	var out Wallet
	if _, err := s.c.do(ctx, request{method: http.MethodGet, path: path + "/wallet"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Grant credits a currency to a player, always to the bonus bucket. An unknown "ext:" player is created.
//
// The call is idempotent: it carries an Idempotency-Key, generated once per call unless WithIdempotencyKey gives one, so
// the automatic retries of the SDK cannot credit twice. Movement.Replayed tells whether the response was a replay.
func (s *WalletService) Grant(ctx context.Context, player PlayerRef, params GrantParams, opts ...CallOption) (*Movement, error) {
	return s.move(ctx, player, "/wallet/grant", movementBody(params), opts)
}

// Spend debits a currency from a player. It is idempotent like Grant. A wallet that is too low fails with
// CodeInsufficientFunds, and Error.Details holds "required" and "available" (see Error.DetailInt64).
func (s *WalletService) Spend(ctx context.Context, player PlayerRef, params SpendParams, opts ...CallOption) (*Movement, error) {
	return s.move(ctx, player, "/wallet/spend", movementBody(params), opts)
}

func (s *WalletService) move(ctx context.Context, player PlayerRef, action string, body movementBody, opts []CallOption) (*Movement, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	if body.Currency == "" {
		return nil, argError("the currency must not be empty")
	}
	if body.Amount < 1 {
		return nil, argError("the amount must be an integer of at least 1")
	}
	key, err := resolveIdempotencyKey(opts)
	if err != nil {
		return nil, err
	}
	encoded, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	var out Movement
	replayed, err := s.c.do(ctx, request{method: http.MethodPost, path: path + action, body: encoded, idempotencyKey: key}, &out)
	if err != nil {
		return nil, err
	}
	out.Replayed = replayed
	return &out, nil
}
