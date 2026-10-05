package gamecoin

import (
	"context"
	"net/http"
)

// OrdersService reads and refunds orders (GET /orders/{orderId}, POST /orders/{orderId}/refund). Get it from
// Client.Orders.
type OrdersService struct{ c *core }

func orderPath(orderID string) (string, error) {
	if orderID == "" {
		return "", argError("the order id must not be empty")
	}
	return "/orders/" + escapeSegment(orderID), nil
}

// Get returns an order. An unknown order is CodeNotFound.
func (s *OrdersService) Get(ctx context.Context, orderID string) (*Order, error) {
	path, err := orderPath(orderID)
	if err != nil {
		return nil, err
	}
	var out Order
	if _, err := s.c.do(ctx, request{method: http.MethodGet, path: path}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Refund refunds an order: the coins and the items it delivered are taken back. A nil params sends no reason.
// Refunding an unpaid order fails with CodeConflict.
//
// Refund has no idempotency key, so the SDK retries it on 429 only. After a network error, a timeout or a 5xx it returns
// the error, and you should check the outcome with Get before trying again.
func (s *OrdersService) Refund(ctx context.Context, orderID string, params *RefundParams) (*Order, error) {
	path, err := orderPath(orderID)
	if err != nil {
		return nil, err
	}
	body := struct {
		Reason string `json:"reason,omitempty"`
	}{}
	if params != nil {
		body.Reason = params.Reason
	}
	encoded, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	var out Order
	if _, err := s.c.do(ctx, request{method: http.MethodPost, path: path + "/refund", body: encoded, retry: retryRateLimitOnly}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
