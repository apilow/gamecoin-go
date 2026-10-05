package gamecoin

import (
	"context"
	"net/http"
)

// PurchasesService buys items with their currency price (POST /players/{player}/purchases). Get it from
// Client.Purchases.
type PurchasesService struct{ c *core }

// Create buys an item for a player with its currency price: the currency is spent and the item is added to the
// inventory. An unknown "ext:" player is created.
//
// The call is idempotent: it carries an Idempotency-Key, generated once per call unless WithIdempotencyKey gives one.
// Purchase.Replayed tells whether the response was a replay. Typical failures are CodeInsufficientFunds and
// CodeLimitReached.
func (s *PurchasesService) Create(ctx context.Context, player PlayerRef, params PurchaseParams, opts ...CallOption) (*Purchase, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	body, err := newItemBody(params.SKU, params.Quantity)
	if err != nil {
		return nil, err
	}
	key, err := resolveIdempotencyKey(opts)
	if err != nil {
		return nil, err
	}
	encoded, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	var out Purchase
	replayed, err := s.c.do(ctx, request{method: http.MethodPost, path: path + "/purchases", body: encoded, idempotencyKey: key}, &out)
	if err != nil {
		return nil, err
	}
	out.Replayed = replayed
	return &out, nil
}
