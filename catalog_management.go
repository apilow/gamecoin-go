package gamecoin

import (
	"context"
	"net/http"
	"time"
)

// Catalog management: Client.Currencies, Client.Items and Client.Packs create, change and archive the catalog, the way a
// payment platform manages its products and prices. Each environment has its own catalog: a
// resource created with a test key exists in test only.
//
// A code or a sku is the natural key of a create: a taken one (even by an archived resource) is CodeConflict with
// Details["fieldErrors"] holding "sku": ["SKU_TAKEN"] (or "code": ["CODE_TAKEN"]), so a create has no idempotency key and the
// SDK retries it on 429 only. After a network error, a timeout or a 5xx its outcome is unknown: read the resource back with Get
// before you try again. Updates, archives and reads follow the usual retry rules.

// Ptr returns a pointer to a copy of v: a convenience to fill the optional (pointer) fields of the params of the catalog
// management, e.g. gamecoin.Ptr("Elixir") or gamecoin.Ptr(int64(25)).
func Ptr[T any](v T) *T { return &v }

// CatalogStatus is the status of a currency or an item.
type CatalogStatus string

const (
	// CatalogActive is a resource that is offered.
	CatalogActive CatalogStatus = "active"
	// CatalogArchived is a resource that is no longer offered, creditable or buyable. Balances, inventories and past orders
	// that refer to it are untouched.
	CatalogArchived CatalogStatus = "archived"
)

// PackStatus is the status of a pack.
type PackStatus string

const (
	// PackDraft is a pack that is not sold (the status of a new pack).
	PackDraft PackStatus = "draft"
	// PackActive is a pack that is sold.
	PackActive PackStatus = "active"
	// PackArchived is a pack that is no longer sold; the orders already placed are unaffected.
	PackArchived PackStatus = "archived"
)

// ManagedCurrency is a currency as Currencies manages it: the catalog fields, the status and the dates.
type ManagedCurrency struct {
	// Code is the currency code.
	Code string `json:"code"`
	// Name is the display name.
	Name string `json:"name"`
	// Purchasable is true when packs can sell the currency for real money.
	Purchasable bool `json:"purchasable"`
	// Status is CatalogActive or CatalogArchived.
	Status CatalogStatus `json:"status"`
	// CreatedAt is when the currency was created.
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt is when the currency was last changed.
	UpdatedAt time.Time `json:"updatedAt"`
}

// ManagedItem is an item as Items manages it: the catalog fields, the status and the dates.
type ManagedItem struct {
	// SKU is the item sku.
	SKU string `json:"sku"`
	// Name is the display name.
	Name string `json:"name"`
	// Description is the description, if any.
	Description *string `json:"description"`
	// Type is ItemConsumable or ItemDurable.
	Type ItemType `json:"type"`
	// Price is the price in a currency, nil when the item cannot be bought with a currency.
	Price *Price `json:"price"`
	// MaxOwned is the maximum quantity a player can own, nil for no limit.
	MaxOwned *int64 `json:"maxOwned"`
	// ClientPurchasable says whether the browser SDK may buy it.
	ClientPurchasable bool `json:"clientPurchasable"`
	// Metadata is the free-form metadata of the item (empty when none).
	Metadata map[string]string `json:"metadata"`
	// Status is CatalogActive or CatalogArchived.
	Status CatalogStatus `json:"status"`
	// CreatedAt is when the item was created.
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt is when the item was last changed.
	UpdatedAt time.Time `json:"updatedAt"`
}

// ManagedPackAvailability is the sale settings of a pack as they were written, with the units delivered so far. The public
// catalog shows the remaining stock instead (PackAvailability).
type ManagedPackAvailability struct {
	// StartsAt is the start of the sale (included), nil when the pack is on sale from the beginning.
	StartsAt *time.Time `json:"startsAt"`
	// EndsAt is the end of the sale (excluded), nil when the sale has no end.
	EndsAt *time.Time `json:"endsAt"`
	// Stock is the total number of units that can be sold, nil for an unlimited stock.
	Stock *int64 `json:"stock"`
	// Founder is true for a founder pack.
	Founder bool `json:"founder"`
	// Sold is the number of units delivered so far.
	Sold int64 `json:"sold"`
}

// ManagedPack is a pack as Packs manages it: the fields of Pack (Availability aside), the display order, the status and the
// dates.
type ManagedPack struct {
	// SKU is the pack sku.
	SKU string `json:"sku"`
	// Name is the display name.
	Name string `json:"name"`
	// Description is the description, if any.
	Description *string `json:"description"`
	// PriceCents is the price in cents (VAT included).
	PriceCents int64 `json:"priceCents"`
	// Currency is always "EUR".
	Currency string `json:"currency"`
	// Grants are the currency amounts credited.
	Grants []PackGrant `json:"grants"`
	// Items are the items delivered.
	Items []PackItem `json:"items"`
	// Badge is "popular" or "best_value", if any.
	Badge *string `json:"badge"`
	// MaxPerPlayer is the maximum number of purchases per player, nil for no limit.
	MaxPerPlayer *int64 `json:"maxPerPlayer"`
	// SortOrder is the display order in the shop, ascending.
	SortOrder int64 `json:"sortOrder"`
	// Status is PackDraft, PackActive or PackArchived.
	Status PackStatus `json:"status"`
	// Availability is the sale settings, nil for an ordinary pack (always on sale, no limit).
	Availability *ManagedPackAvailability `json:"availability"`
	// CreatedAt is when the pack was created.
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt is when the pack was last changed.
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListParams are the options of the List methods of Currencies, Items and Packs.
type ListParams struct {
	// IncludeArchived also lists the archived resources, which are left out by default.
	IncludeArchived bool
}

func listQuery(params *ListParams) string {
	if params != nil && params.IncludeArchived {
		return "includeArchived=true"
	}
	return ""
}

// ItemField names a field of an item, to say which ones UpdateItemParams.Clear erases.
type ItemField string

// The fields of an item that can be cleared.
const (
	// ItemFieldDescription is the description.
	ItemFieldDescription ItemField = "description"
	// ItemFieldPrice is the price: the item can no longer be bought with a currency.
	ItemFieldPrice ItemField = "price"
	// ItemFieldMaxOwned is the limit per player: no limit any more.
	ItemFieldMaxOwned ItemField = "maxOwned"
)

// PackField names a field of a pack, to say which ones UpdatePackParams.Clear erases.
type PackField string

// The fields of a pack that can be cleared.
const (
	// PackFieldDescription is the description.
	PackFieldDescription PackField = "description"
	// PackFieldBadge is the badge.
	PackFieldBadge PackField = "badge"
	// PackFieldMaxPerPlayer is the limit per player: no limit any more.
	PackFieldMaxPerPlayer PackField = "maxPerPlayer"
)

// PackAvailabilityParams is the whole sale setting of a pack: it replaces the previous one, it is not merged with it. A zero
// value is no limit at all: no window, an unlimited stock and not a founder pack.
type PackAvailabilityParams struct {
	// StartsAt is the first instant of sale (included); nil for on sale from the start.
	StartsAt *time.Time
	// EndsAt is the first instant when the pack is no longer sold (excluded); nil for no end.
	EndsAt *time.Time
	// Stock is the total number of units that can be sold (1 to 1,000,000); nil for an unlimited stock. It cannot go below
	// the units already sold.
	Stock *int64
	// Founder marks a founder pack.
	Founder bool
}

func (a *PackAvailabilityParams) body() (map[string]any, error) {
	if a.Stock != nil && *a.Stock < 1 {
		return nil, argError("the availability stock must be an integer of at least 1")
	}
	instant := func(t *time.Time) any {
		if t == nil {
			return nil
		}
		return t.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{"startsAt": instant(a.StartsAt), "endsAt": instant(a.EndsAt), "stock": a.Stock, "founder": a.Founder}, nil
}

func checkNonEmpty(what, value string) error {
	if value == "" {
		return argError("the " + what + " must not be empty")
	}
	return nil
}

func segmentPath(prefix, what, key string) (string, error) {
	if err := checkNonEmpty(what, key); err != nil {
		return "", err
	}
	return prefix + escapeSegment(key), nil
}

func (c *core) send(ctx context.Context, method, path, query string, body map[string]any, retry retryPolicy, out any) error {
	var encoded []byte
	if body != nil {
		var err error
		if encoded, err = marshalBody(body); err != nil {
			return err
		}
	}
	_, err := c.do(ctx, request{method: method, path: path, query: query, body: encoded, retry: retry}, out)
	return err
}

// --- Currencies ----------------------------------------------------------------------------------------------

// CurrenciesService manages the currencies of the game (/currencies). Get it from Client.Currencies.
type CurrenciesService struct{ c *core }

// CreateCurrencyParams is the request of Currencies.Create.
type CreateCurrencyParams struct {
	// Code is a lowercase letter, then lowercase letters, digits or "_" (2 to 24 characters). Unique in the game and final.
	// Required.
	Code string
	// Name is the display name. Required.
	Name string
	// Purchasable says whether packs can sell the currency for real money. Nil means true; false is a currency that is only
	// given (always credited to the bonus bucket).
	Purchasable *bool
}

// UpdateCurrencyParams is the request of Currencies.Update. A nil field is left unchanged.
type UpdateCurrencyParams struct {
	// Name is the new display name.
	Name *string
	// Purchasable cannot become false while a pack that is not archived sells the currency for money (CodeConflict,
	// Details["blockedBy"]).
	Purchasable *bool
}

// Create creates a currency. A taken code is CodeConflict ("code": ["CODE_TAKEN"] in Details["fieldErrors"]); an invalid one is
// CodeValidationFailed. A create is retried on 429 only: after a network error, a timeout or a 5xx, check with Get.
func (s *CurrenciesService) Create(ctx context.Context, params CreateCurrencyParams) (*ManagedCurrency, error) {
	if err := checkNonEmpty("currency code", params.Code); err != nil {
		return nil, err
	}
	if err := checkNonEmpty("currency name", params.Name); err != nil {
		return nil, err
	}
	body := map[string]any{"code": params.Code, "name": params.Name}
	if params.Purchasable != nil {
		body["purchasable"] = *params.Purchasable
	}
	var out ManagedCurrency
	if err := s.c.send(ctx, http.MethodPost, "/currencies", "", body, retryRateLimitOnly, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns a currency, archived ones included. An unknown code is CodeNotFound.
func (s *CurrenciesService) Get(ctx context.Context, code string) (*ManagedCurrency, error) {
	path, err := segmentPath("/currencies/", "currency code", code)
	if err != nil {
		return nil, err
	}
	var out ManagedCurrency
	if err := s.c.send(ctx, http.MethodGet, path, "", nil, retryAll, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update changes a currency; a nil field is unchanged. The code is final. An archived currency cannot be changed
// (CodeConflict). A nil params changes nothing.
func (s *CurrenciesService) Update(ctx context.Context, code string, params *UpdateCurrencyParams) (*ManagedCurrency, error) {
	path, err := segmentPath("/currencies/", "currency code", code)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if params != nil {
		if params.Name != nil {
			if err := checkNonEmpty("currency name", *params.Name); err != nil {
				return nil, err
			}
			body["name"] = *params.Name
		}
		if params.Purchasable != nil {
			body["purchasable"] = *params.Purchasable
		}
	}
	var out ManagedCurrency
	if err := s.c.send(ctx, http.MethodPatch, path, "", body, retryAll, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Archive archives a currency (nothing is deleted; balances stay) and returns it. It is CodeConflict, with
// Details["blockedBy"], while an active pack credits the currency or an active item is priced in it. Archiving an archived
// currency has no effect.
func (s *CurrenciesService) Archive(ctx context.Context, code string) (*ManagedCurrency, error) {
	path, err := segmentPath("/currencies/", "currency code", code)
	if err != nil {
		return nil, err
	}
	var out ManagedCurrency
	if err := s.c.send(ctx, http.MethodDelete, path, "", nil, retryAll, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns the currencies of the game, oldest first. A nil params lists the active ones only.
func (s *CurrenciesService) List(ctx context.Context, params *ListParams) ([]ManagedCurrency, error) {
	var out struct {
		Currencies []ManagedCurrency `json:"currencies"`
	}
	if err := s.c.send(ctx, http.MethodGet, "/currencies", listQuery(params), nil, retryAll, &out); err != nil {
		return nil, err
	}
	return out.Currencies, nil
}

// --- Items ---------------------------------------------------------------------------------------------------

// ItemsService manages the items of the game (/items). Get it from Client.Items.
type ItemsService struct{ c *core }

// CreateItemParams is the request of Items.Create.
type CreateItemParams struct {
	// SKU is unique among the items and final. Required.
	SKU string
	// Name is the display name. Required.
	Name string
	// Type is ItemConsumable or ItemDurable, and final. Required.
	Type ItemType
	// Description is an optional text (500 characters at most).
	Description string
	// Price is the price in an active currency of the game; nil for an item that cannot be bought with a currency.
	Price *Price
	// MaxOwned is the most units a player can own. Nil means the default: 1 for a durable item, no limit for a consumable.
	MaxOwned *int64
	// NoMaxOwned says that there is no limit, whatever the type (MaxOwned must then be nil).
	NoMaxOwned bool
	// ClientPurchasable lets the browser SDK sell the item; it needs a Price.
	ClientPurchasable bool
	// Metadata is free-form string metadata that your game reads.
	Metadata map[string]string
}

// UpdateItemParams is the request of Items.Update. A nil (or empty) field is left unchanged; to erase the description, the
// price or the limit, list it in Clear. A field cannot be both set and cleared.
type UpdateItemParams struct {
	// Name is the new display name.
	Name *string
	// Description is the new description.
	Description *string
	// Price is the new price.
	Price *Price
	// MaxOwned is the new limit per player.
	MaxOwned *int64
	// ClientPurchasable lets the browser SDK sell the item (it needs a price).
	ClientPurchasable *bool
	// Metadata replaces the whole metadata map; an empty non-nil map erases it.
	Metadata map[string]string
	// Clear lists the fields to erase (sent as null).
	Clear []ItemField
}

func priceBody(what string, price *Price) (map[string]any, error) {
	if err := checkNonEmpty(what+" currency", price.Currency); err != nil {
		return nil, err
	}
	if price.Amount < 1 {
		return nil, argError("the " + what + " amount must be an integer of at least 1")
	}
	return map[string]any{"currency": price.Currency, "amount": price.Amount}, nil
}

func checkMaxOwned(value *int64) error {
	if value != nil && *value < 1 {
		return argError("the item MaxOwned must be an integer of at least 1")
	}
	return nil
}

// Create creates an item. A taken sku is CodeConflict ("sku": ["SKU_TAKEN"] in Details["fieldErrors"]); an unknown price
// currency, or ClientPurchasable without a price, is CodeValidationFailed. A create is retried on 429 only: after a network
// error, a timeout or a 5xx, check with Get.
func (s *ItemsService) Create(ctx context.Context, params CreateItemParams) (*ManagedItem, error) {
	if err := checkNonEmpty("item sku", params.SKU); err != nil {
		return nil, err
	}
	if err := checkNonEmpty("item name", params.Name); err != nil {
		return nil, err
	}
	if params.Type != ItemConsumable && params.Type != ItemDurable {
		return nil, argError("the item Type must be gamecoin.ItemConsumable or gamecoin.ItemDurable")
	}
	if params.NoMaxOwned && params.MaxOwned != nil {
		return nil, argError("the item MaxOwned and NoMaxOwned are exclusive")
	}
	if err := checkMaxOwned(params.MaxOwned); err != nil {
		return nil, err
	}
	body := map[string]any{"sku": params.SKU, "name": params.Name, "type": string(params.Type)}
	if params.Description != "" {
		body["description"] = params.Description
	}
	if params.Price != nil {
		price, err := priceBody("item price", params.Price)
		if err != nil {
			return nil, err
		}
		body["price"] = price
	}
	switch {
	case params.NoMaxOwned:
		body["maxOwned"] = nil
	case params.MaxOwned != nil:
		body["maxOwned"] = *params.MaxOwned
	}
	if params.ClientPurchasable {
		body["clientPurchasable"] = true
	}
	if params.Metadata != nil {
		body["metadata"] = params.Metadata
	}
	var out ManagedItem
	if err := s.c.send(ctx, http.MethodPost, "/items", "", body, retryRateLimitOnly, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns an item, archived ones included. An unknown sku is CodeNotFound.
func (s *ItemsService) Get(ctx context.Context, sku string) (*ManagedItem, error) {
	path, err := segmentPath("/items/", "item sku", sku)
	if err != nil {
		return nil, err
	}
	var out ManagedItem
	if err := s.c.send(ctx, http.MethodGet, path, "", nil, retryAll, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update changes an item. The sku and the type are final. An archived item cannot be changed (CodeConflict). A nil params
// changes nothing.
func (s *ItemsService) Update(ctx context.Context, sku string, params *UpdateItemParams) (*ManagedItem, error) {
	path, err := segmentPath("/items/", "item sku", sku)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if params != nil {
		if params.Name != nil {
			if err := checkNonEmpty("item name", *params.Name); err != nil {
				return nil, err
			}
			body["name"] = *params.Name
		}
		if params.Description != nil {
			body["description"] = *params.Description
		}
		if params.Price != nil {
			price, err := priceBody("item price", params.Price)
			if err != nil {
				return nil, err
			}
			body["price"] = price
		}
		if err := checkMaxOwned(params.MaxOwned); err != nil {
			return nil, err
		}
		if params.MaxOwned != nil {
			body["maxOwned"] = *params.MaxOwned
		}
		if params.ClientPurchasable != nil {
			body["clientPurchasable"] = *params.ClientPurchasable
		}
		if params.Metadata != nil {
			body["metadata"] = params.Metadata
		}
		for _, field := range params.Clear {
			switch field {
			case ItemFieldDescription, ItemFieldPrice, ItemFieldMaxOwned:
			default:
				return nil, argError("Clear lists an unknown item field")
			}
			if value, alsoSet := body[string(field)]; alsoSet && value != nil {
				return nil, argError("the item " + string(field) + " cannot be both set and cleared")
			}
			body[string(field)] = nil
		}
	}
	var out ManagedItem
	if err := s.c.send(ctx, http.MethodPatch, path, "", body, retryAll, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Archive archives an item (nothing is deleted; inventories keep it) and returns it. It is CodeConflict, with
// Details["blockedBy"], while an active pack contains it. Archiving an archived item has no effect.
func (s *ItemsService) Archive(ctx context.Context, sku string) (*ManagedItem, error) {
	path, err := segmentPath("/items/", "item sku", sku)
	if err != nil {
		return nil, err
	}
	var out ManagedItem
	if err := s.c.send(ctx, http.MethodDelete, path, "", nil, retryAll, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns the items of the game, oldest first. A nil params lists the active ones only.
func (s *ItemsService) List(ctx context.Context, params *ListParams) ([]ManagedItem, error) {
	var out struct {
		Items []ManagedItem `json:"items"`
	}
	if err := s.c.send(ctx, http.MethodGet, "/items", listQuery(params), nil, retryAll, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// --- Packs ---------------------------------------------------------------------------------------------------

// PacksService manages the packs of the game (/packs). Get it from Client.Packs.
type PacksService struct{ c *core }

// CreatePackParams is the request of Packs.Create.
type CreatePackParams struct {
	// SKU is unique among the packs and final. Required.
	SKU string
	// Name is the display name. Required.
	Name string
	// PriceCents is the price in euro cents, VAT included (50 to 50,000). Required.
	PriceCents int64
	// Description is an optional text (500 characters at most).
	Description string
	// Grants are the currencies credited when the order is paid. A pack needs at least one grant or one item.
	Grants []PackGrant
	// Items are the items delivered when the order is paid.
	Items []PackItem
	// Badge is "popular" or "best_value"; empty for none.
	Badge string
	// MaxPerPlayer is the most orders per player (a starter pack: 1); 0 for no limit.
	MaxPerPlayer int64
	// SortOrder is the display order in the shop, ascending (-10,000 to 10,000); 0 by default.
	SortOrder int64
	// Availability is the sale window, the stock and the founder flag; nil for an ordinary pack.
	Availability *PackAvailabilityParams
	// Status is PackDraft (the default when empty) or PackActive, to sell the pack at once.
	Status PackStatus
}

// UpdatePackParams is the request of Packs.Update. A nil (or empty) field is left unchanged; to erase the description, the
// badge or the limit, list it in Clear. A field cannot be both set and cleared.
type UpdatePackParams struct {
	// Name is the new display name.
	Name *string
	// Description is the new description.
	Description *string
	// PriceCents is the new price in euro cents. The orders already placed keep the price they were made at.
	PriceCents *int64
	// Grants replaces the whole list of grants; nil leaves it unchanged, an empty non-nil slice empties it.
	Grants []PackGrant
	// Items replaces the whole list of items; nil leaves it unchanged, an empty non-nil slice empties it.
	Items []PackItem
	// Badge is "popular" or "best_value".
	Badge *string
	// MaxPerPlayer is the new limit per player.
	MaxPerPlayer *int64
	// SortOrder is the new display order.
	SortOrder *int64
	// Availability replaces the whole sale setting; a pointer to the zero value removes every limit.
	Availability *PackAvailabilityParams
	// Status changes the status (PackDraft, PackActive or PackArchived); empty leaves it. It is applied after the rest of the
	// change: turning a pack active checks that every currency and item it uses is still active, and when that check fails
	// the content change stays and the status does not move.
	Status PackStatus
	// Clear lists the fields to erase (sent as null).
	Clear []PackField
}

func grantLines(grants []PackGrant) ([]map[string]any, error) {
	lines := make([]map[string]any, 0, len(grants))
	for _, grant := range grants {
		if err := checkNonEmpty("pack grant currency", grant.Currency); err != nil {
			return nil, err
		}
		if grant.Amount < 0 || grant.Bonus < 0 {
			return nil, argError("a pack grant Amount and Bonus must be integers of at least 0")
		}
		lines = append(lines, map[string]any{"currency": grant.Currency, "amount": grant.Amount, "bonus": grant.Bonus})
	}
	return lines, nil
}

func itemLines(items []PackItem) ([]map[string]any, error) {
	lines := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if err := checkNonEmpty("pack item sku", item.SKU); err != nil {
			return nil, err
		}
		if item.Quantity < 1 {
			return nil, argError("a pack item Quantity must be an integer of at least 1")
		}
		lines = append(lines, map[string]any{"sku": item.SKU, "quantity": item.Quantity})
	}
	return lines, nil
}

func checkBadge(badge string) error {
	if badge != "popular" && badge != "best_value" {
		return argError(`the pack Badge must be "popular" or "best_value"`)
	}
	return nil
}

func checkSortOrder(order int64) error {
	if order < -1<<53 || order > 1<<53 {
		return argError("the pack SortOrder is out of range")
	}
	return nil
}

// Create creates a pack, as a draft unless Status is PackActive. A taken sku is CodeConflict ("sku": ["SKU_TAKEN"]); an
// invalid pack (no grant and no item, an unknown or archived currency or item, a paid amount in a free currency, a price out
// of range) is CodeValidationFailed with the faulty lines in Details["fieldErrors"]. A create is retried on 429 only: after a
// network error, a timeout or a 5xx, check with Get.
func (s *PacksService) Create(ctx context.Context, params CreatePackParams) (*ManagedPack, error) {
	if err := checkNonEmpty("pack sku", params.SKU); err != nil {
		return nil, err
	}
	if err := checkNonEmpty("pack name", params.Name); err != nil {
		return nil, err
	}
	if params.PriceCents < 0 {
		return nil, argError("the pack PriceCents must be an integer of at least 0")
	}
	body := map[string]any{"sku": params.SKU, "name": params.Name, "priceCents": params.PriceCents}
	if params.Description != "" {
		body["description"] = params.Description
	}
	if params.Grants != nil {
		lines, err := grantLines(params.Grants)
		if err != nil {
			return nil, err
		}
		body["grants"] = lines
	}
	if params.Items != nil {
		lines, err := itemLines(params.Items)
		if err != nil {
			return nil, err
		}
		body["items"] = lines
	}
	if params.Badge != "" {
		if err := checkBadge(params.Badge); err != nil {
			return nil, err
		}
		body["badge"] = params.Badge
	}
	if params.MaxPerPlayer < 0 {
		return nil, argError("the pack MaxPerPlayer must be an integer of at least 0 (0 means no limit)")
	}
	if params.MaxPerPlayer > 0 {
		body["maxPerPlayer"] = params.MaxPerPlayer
	}
	if err := checkSortOrder(params.SortOrder); err != nil {
		return nil, err
	}
	if params.SortOrder != 0 {
		body["sortOrder"] = params.SortOrder
	}
	if params.Availability != nil {
		availability, err := params.Availability.body()
		if err != nil {
			return nil, err
		}
		body["availability"] = availability
	}
	switch params.Status {
	case "":
	case PackDraft, PackActive:
		body["status"] = string(params.Status)
	default:
		return nil, argError("the pack Status of a new pack must be gamecoin.PackDraft or gamecoin.PackActive")
	}
	var out ManagedPack
	if err := s.c.send(ctx, http.MethodPost, "/packs", "", body, retryRateLimitOnly, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns a pack, archived ones included. An unknown sku is CodeNotFound.
func (s *PacksService) Get(ctx context.Context, sku string) (*ManagedPack, error) {
	path, err := segmentPath("/packs/", "pack sku", sku)
	if err != nil {
		return nil, err
	}
	var out ManagedPack
	if err := s.c.send(ctx, http.MethodGet, path, "", nil, retryAll, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update changes a pack. The sku is final. An archived pack cannot be changed (CodeConflict), except to bring it back with
// Status alone. A nil params changes nothing.
func (s *PacksService) Update(ctx context.Context, sku string, params *UpdatePackParams) (*ManagedPack, error) {
	path, err := segmentPath("/packs/", "pack sku", sku)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if params != nil {
		if params.Name != nil {
			if err := checkNonEmpty("pack name", *params.Name); err != nil {
				return nil, err
			}
			body["name"] = *params.Name
		}
		if params.Description != nil {
			body["description"] = *params.Description
		}
		if params.PriceCents != nil {
			if *params.PriceCents < 0 {
				return nil, argError("the pack PriceCents must be an integer of at least 0")
			}
			body["priceCents"] = *params.PriceCents
		}
		if params.Grants != nil {
			lines, err := grantLines(params.Grants)
			if err != nil {
				return nil, err
			}
			body["grants"] = lines
		}
		if params.Items != nil {
			lines, err := itemLines(params.Items)
			if err != nil {
				return nil, err
			}
			body["items"] = lines
		}
		if params.Badge != nil {
			if err := checkBadge(*params.Badge); err != nil {
				return nil, err
			}
			body["badge"] = *params.Badge
		}
		if params.MaxPerPlayer != nil {
			if *params.MaxPerPlayer < 1 {
				return nil, argError("the pack MaxPerPlayer must be an integer of at least 1 (to remove the limit, list it in Clear)")
			}
			body["maxPerPlayer"] = *params.MaxPerPlayer
		}
		if params.SortOrder != nil {
			if err := checkSortOrder(*params.SortOrder); err != nil {
				return nil, err
			}
			body["sortOrder"] = *params.SortOrder
		}
		if params.Availability != nil {
			availability, err := params.Availability.body()
			if err != nil {
				return nil, err
			}
			body["availability"] = availability
		}
		switch params.Status {
		case "":
		case PackDraft, PackActive, PackArchived:
			body["status"] = string(params.Status)
		default:
			return nil, argError("the pack Status must be gamecoin.PackDraft, gamecoin.PackActive or gamecoin.PackArchived")
		}
		for _, field := range params.Clear {
			switch field {
			case PackFieldDescription, PackFieldBadge, PackFieldMaxPerPlayer:
			default:
				return nil, argError("Clear lists an unknown pack field")
			}
			if value, alsoSet := body[string(field)]; alsoSet && value != nil {
				return nil, argError("the pack " + string(field) + " cannot be both set and cleared")
			}
			body[string(field)] = nil
		}
	}
	var out ManagedPack
	if err := s.c.send(ctx, http.MethodPatch, path, "", body, retryAll, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Archive archives a pack (it is no longer sold; the orders already placed are unaffected) and returns it.
func (s *PacksService) Archive(ctx context.Context, sku string) (*ManagedPack, error) {
	path, err := segmentPath("/packs/", "pack sku", sku)
	if err != nil {
		return nil, err
	}
	var out ManagedPack
	if err := s.c.send(ctx, http.MethodDelete, path, "", nil, retryAll, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns the packs of the game in display order, drafts included. A nil params leaves the archived ones out.
func (s *PacksService) List(ctx context.Context, params *ListParams) ([]ManagedPack, error) {
	var out struct {
		Packs []ManagedPack `json:"packs"`
	}
	if err := s.c.send(ctx, http.MethodGet, "/packs", listQuery(params), nil, retryAll, &out); err != nil {
		return nil, err
	}
	return out.Packs, nil
}
