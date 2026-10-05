package gamecoin

import (
	"context"
	"net/http"
)

// InventoryService reads inventories and grants or consumes items (GET /players/{player}/inventory and the grant and
// consume actions). Get it from Client.Inventory.
type InventoryService struct{ c *core }

// itemBody is the body of the calls that name an item and a quantity.
type itemBody struct {
	SKU      string `json:"sku"`
	Quantity int64  `json:"quantity"`
}

func newItemBody(sku string, quantity int64) (itemBody, error) {
	if sku == "" {
		return itemBody{}, argError("the sku must not be empty")
	}
	if quantity < 0 {
		return itemBody{}, argError("the quantity must be an integer of at least 1 (0 means 1)")
	}
	if quantity == 0 {
		quantity = 1
	}
	return itemBody{SKU: sku, Quantity: quantity}, nil
}

// List returns the items a player owns (quantity above 0).
func (s *InventoryService) List(ctx context.Context, player PlayerRef) ([]InventoryItem, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []InventoryItem `json:"items"`
	}
	if _, err := s.c.do(ctx, request{method: http.MethodGet, path: path + "/inventory"}, &out); err != nil {
		return nil, err
	}
	if out.Items == nil {
		out.Items = []InventoryItem{}
	}
	return out.Items, nil
}

// Grant puts an item in the inventory of a player, for free. An unknown "ext:" player is created.
//
// The call is idempotent: it carries an Idempotency-Key, generated once per call unless WithIdempotencyKey gives one.
// InventoryItem.Replayed tells whether the response was a replay. Going above the item's MaxOwned fails with
// CodeLimitReached.
func (s *InventoryService) Grant(ctx context.Context, player PlayerRef, params InventoryParams, opts ...CallOption) (*InventoryItem, error) {
	return s.change(ctx, player, "/inventory/grant", params, opts)
}

// Consume uses up an item of the inventory of a player. It is idempotent like Grant. The item is returned even when its
// quantity reaches 0; consuming more than the player owns fails with CodeInsufficientFunds.
func (s *InventoryService) Consume(ctx context.Context, player PlayerRef, params InventoryParams, opts ...CallOption) (*InventoryItem, error) {
	return s.change(ctx, player, "/inventory/consume", params, opts)
}

func (s *InventoryService) change(ctx context.Context, player PlayerRef, action string, params InventoryParams, opts []CallOption) (*InventoryItem, error) {
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
	var out struct {
		Item InventoryItem `json:"item"`
	}
	replayed, err := s.c.do(ctx, request{method: http.MethodPost, path: path + action, body: encoded, idempotencyKey: key}, &out)
	if err != nil {
		return nil, err
	}
	out.Item.Replayed = replayed
	return &out.Item, nil
}
