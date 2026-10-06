package gamecoin

import (
	"context"
	"net/http"
)

// CheckoutsService sells packs for real money (POST /players/{player}/checkouts). Get it from Client.Checkouts.
type CheckoutsService struct{ c *core }

// checkoutBody is the body of a checkout.
type checkoutBody struct {
	PackSKU    string `json:"packSku"`
	SuccessURL string `json:"successUrl,omitempty"`
	CancelURL  string `json:"cancelUrl,omitempty"`
	Country    string `json:"country,omitempty"`
	Locale     string `json:"locale,omitempty"`
}

// Create creates an order for a pack and the hosted payment page to send the player to (Checkout.CheckoutURL). An
// unknown "ext:" player is created.
//
// The call is idempotent: it carries an Idempotency-Key, generated once per call unless WithIdempotencyKey gives one. A
// replay returns the order in its current state; Checkout.Replayed tells whether it was one.
func (s *CheckoutsService) Create(ctx context.Context, player PlayerRef, params CheckoutParams, opts ...CallOption) (*Checkout, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	if params.PackSKU == "" {
		return nil, argError("the pack sku must not be empty")
	}
	key, err := resolveIdempotencyKey(opts)
	if err != nil {
		return nil, err
	}
	encoded, err := marshalBody(checkoutBody(params))
	if err != nil {
		return nil, err
	}
	var out Checkout
	replayed, err := s.c.do(ctx, request{method: http.MethodPost, path: path + "/checkouts", body: encoded, idempotencyKey: key}, &out)
	if err != nil {
		return nil, err
	}
	out.Replayed = replayed
	return &out, nil
}
