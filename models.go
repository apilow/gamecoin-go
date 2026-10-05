package gamecoin

import (
	"fmt"
	"time"
)

// The objects of the GameCoin API, field for field. Optional and nullable fields are pointers, timestamps are
// time.Time (UTC), amounts are int64. Unknown fields in a response are ignored. Enumerations are string types so that
// a value added by the API later never breaks decoding: the constants name the known ones.

// PlayerKind is the kind of a player.
type PlayerKind string

// Known values of Player.Kind.
const (
	// PlayerKindExternal is a player created by your backend (an "ext:" reference).
	PlayerKindExternal PlayerKind = "external"
	// PlayerKindHosted is an anonymous player created by the browser SDK.
	PlayerKindHosted PlayerKind = "hosted"
)

// ItemType is the type of a catalog item.
type ItemType string

// Known values of Item.Type.
const (
	// ItemConsumable is used up by Inventory.Consume.
	ItemConsumable ItemType = "consumable"
	// ItemDurable is kept; usually capped by Item.MaxOwned.
	ItemDurable ItemType = "durable"
)

// LedgerEntryType is the kind of movement a ledger entry records.
type LedgerEntryType string

// Known values of LedgerEntry.Type.
const (
	// LedgerPurchaseCredit is currency credited by a paid order.
	LedgerPurchaseCredit LedgerEntryType = "purchase_credit"
	// LedgerGrant is currency granted by your backend (it credits the bonus bucket).
	LedgerGrant LedgerEntryType = "grant"
	// LedgerSpend is currency spent.
	LedgerSpend LedgerEntryType = "spend"
	// LedgerRefundClawback is currency taken back after a refund.
	LedgerRefundClawback LedgerEntryType = "refund_clawback"
	// LedgerChargebackClawback is currency taken back after a chargeback.
	LedgerChargebackClawback LedgerEntryType = "chargeback_clawback"
	// LedgerAdjustment is a manual correction.
	LedgerAdjustment LedgerEntryType = "adjustment"
)

// OrderStatus is the state of an order.
type OrderStatus string

// Known values of Order.Status.
const (
	// OrderPending is created, waiting for the payment.
	OrderPending OrderStatus = "pending"
	// OrderPaid is paid, not delivered yet.
	OrderPaid OrderStatus = "paid"
	// OrderFulfilled is paid and delivered (currencies credited, items granted).
	OrderFulfilled OrderStatus = "fulfilled"
	// OrderRefunded is refunded; the coins and items were taken back.
	OrderRefunded OrderStatus = "refunded"
	// OrderChargeback is disputed by the cardholder.
	OrderChargeback OrderStatus = "chargeback"
	// OrderFailed is a payment that failed.
	OrderFailed OrderStatus = "failed"
	// OrderCanceled is canceled by the player.
	OrderCanceled OrderStatus = "canceled"
	// OrderExpired is not paid in time.
	OrderExpired OrderStatus = "expired"
)

// Player is a player.
type Player struct {
	// ID is the GameCoin player id.
	ID string `json:"id"`
	// ExternalID is your own id for the player ("ext:" players), nil otherwise.
	ExternalID *string `json:"externalId"`
	// Kind is PlayerKindExternal or PlayerKindHosted.
	Kind PlayerKind `json:"kind"`
	// DisplayName is the display name, if any.
	DisplayName *string `json:"displayName"`
	// Country is the ISO 3166-1 alpha-2 country, if known.
	Country *string `json:"country"`
	// Blocked is true when the player is blocked.
	Blocked bool `json:"blocked"`
	// CreatedAt is the creation time.
	CreatedAt time.Time `json:"createdAt"`
}

// PlayerToken is a short-lived player token, for the browser SDK.
type PlayerToken struct {
	// Token is the token ("gc_pt_…"). Give it to the player's browser; do not log it.
	Token string `json:"token"`
	// ExpiresAt is the expiry time.
	ExpiresAt time.Time `json:"expiresAt"`
}

// String describes the token without its value.
func (t PlayerToken) String() string {
	return fmt.Sprintf("PlayerToken{Token: ***, ExpiresAt: %s}", t.ExpiresAt.Format(time.RFC3339))
}

// GoString describes the token without its value (the %#v verb).
func (t PlayerToken) GoString() string { return t.String() }

// Balance is the balance of one currency.
type Balance struct {
	// Currency is the currency code ("gems").
	Currency string `json:"currency"`
	// Paid is the amount bought with real money.
	Paid int64 `json:"paid"`
	// Bonus is the amount granted for free (bonuses, rewards).
	Bonus int64 `json:"bonus"`
	// Total is Paid plus Bonus.
	Total int64 `json:"total"`
}

// Wallet holds the balances of a player: one line per active currency, zeros included.
type Wallet struct {
	// PlayerID is the GameCoin player id.
	PlayerID string `json:"playerId"`
	// Balances has one balance per active currency.
	Balances []Balance `json:"balances"`
}

// BalanceSnapshot is the balance after a ledger entry.
type BalanceSnapshot struct {
	// Paid is the paid bucket after the entry.
	Paid int64 `json:"paid"`
	// Bonus is the bonus bucket after the entry.
	Bonus int64 `json:"bonus"`
	// Total is the total after the entry.
	Total int64 `json:"total"`
}

// LedgerRef says what a ledger entry refers to.
type LedgerRef struct {
	// OrderID is the order, for entries caused by an order.
	OrderID *string `json:"orderId,omitempty"`
	// ItemSKU is the item sku, for entries caused by the purchase of an item.
	ItemSKU *string `json:"itemSku,omitempty"`
}

// LedgerEntry is an immutable line of the ledger.
type LedgerEntry struct {
	// ID is the entry id.
	ID string `json:"id"`
	// Type is the kind of movement.
	Type LedgerEntryType `json:"type"`
	// Currency is the currency code.
	Currency string `json:"currency"`
	// PaidDelta is the change of the paid bucket (negative when spent).
	PaidDelta int64 `json:"paidDelta"`
	// BonusDelta is the change of the bonus bucket (negative when spent).
	BonusDelta int64 `json:"bonusDelta"`
	// BalanceAfter is the balance right after this entry.
	BalanceAfter BalanceSnapshot `json:"balanceAfter"`
	// Reason is the free-form reason you gave, if any.
	Reason *string `json:"reason"`
	// Metadata is the free-form metadata you gave (empty when none).
	Metadata map[string]string `json:"metadata"`
	// Ref is the order or item the entry refers to.
	Ref LedgerRef `json:"ref"`
	// CreatedAt is the creation time.
	CreatedAt time.Time `json:"createdAt"`
}

// LedgerPage is a page of the ledger, newest entries first.
type LedgerPage struct {
	// Entries are the entries of the page.
	Entries []LedgerEntry `json:"entries"`
	// NextCursor is the cursor of the next page, nil on the last page.
	NextCursor *string `json:"nextCursor"`
}

// InventoryItem is a quantity of an item owned by a player.
type InventoryItem struct {
	// SKU is the item sku.
	SKU string `json:"sku"`
	// Quantity is the quantity owned (0 is possible right after a consume).
	Quantity int64 `json:"quantity"`
	// UpdatedAt is the time of the last change.
	UpdatedAt time.Time `json:"updatedAt"`
	// Replayed is true when the response to Inventory.Grant or Inventory.Consume was the replay of an earlier request
	// with the same idempotency key. It is always false elsewhere.
	Replayed bool `json:"-"`
}

// Movement is the result of a wallet grant or spend.
type Movement struct {
	// Entry is the ledger entry written.
	Entry LedgerEntry `json:"entry"`
	// Balance is the balance of the currency after the movement.
	Balance Balance `json:"balance"`
	// Replayed is true when the response was the replay of an earlier request with the same idempotency key.
	Replayed bool `json:"-"`
}

// Purchase is the result of the purchase of an item with its currency price.
type Purchase struct {
	// Entry is the ledger entry written.
	Entry LedgerEntry `json:"entry"`
	// Balance is the balance of the currency after the purchase.
	Balance Balance `json:"balance"`
	// Item is the item with its new quantity.
	Item InventoryItem `json:"item"`
	// Replayed is true when the response was the replay of an earlier request with the same idempotency key.
	Replayed bool `json:"-"`
}

// Currency is a currency of the catalog.
type Currency struct {
	// Code is the currency code.
	Code string `json:"code"`
	// Name is the display name.
	Name string `json:"name"`
	// Purchasable is true when the currency can be bought with real money (it then has a paid bucket).
	Purchasable bool `json:"purchasable"`
}

// Price is the price of an item in a currency.
type Price struct {
	// Currency is the currency code.
	Currency string `json:"currency"`
	// Amount is the amount.
	Amount int64 `json:"amount"`
}

// Item is an item of the catalog.
type Item struct {
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
}

// PackGrant is a currency amount credited by a pack.
type PackGrant struct {
	// Currency is the currency code.
	Currency string `json:"currency"`
	// Amount is credited to the paid bucket.
	Amount int64 `json:"amount"`
	// Bonus is the extra amount credited to the bonus bucket.
	Bonus int64 `json:"bonus"`
}

// PackItem is an item delivered by a pack.
type PackItem struct {
	// SKU is the item sku.
	SKU string `json:"sku"`
	// Quantity is the quantity delivered.
	Quantity int64 `json:"quantity"`
}

// Pack is a pack of the catalog: what a player buys with real money.
type Pack struct {
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
	// Badge is a badge to display ("best value"...), if any.
	Badge *string `json:"badge"`
	// MaxPerPlayer is the maximum number of purchases per player, nil for no limit.
	MaxPerPlayer *int64 `json:"maxPerPlayer"`
}

// Catalog lists the active currencies, items and packs of the game.
type Catalog struct {
	// Currencies are the currencies.
	Currencies []Currency `json:"currencies"`
	// Items are the items.
	Items []Item `json:"items"`
	// Packs are the packs.
	Packs []Pack `json:"packs"`
}

// OrderPack is the pack an order was made for, as it was at the time of the purchase.
type OrderPack struct {
	// SKU is the pack sku.
	SKU string `json:"sku"`
	// Name is the pack name.
	Name string `json:"name"`
}

// Order is the purchase of a pack with real money.
type Order struct {
	// ID is the order id.
	ID string `json:"id"`
	// PlayerID is the GameCoin player id.
	PlayerID string `json:"playerId"`
	// Status is the state of the order.
	Status OrderStatus `json:"status"`
	// Pack is the pack.
	Pack OrderPack `json:"pack"`
	// AmountCents is the amount charged in cents, VAT included.
	AmountCents int64 `json:"amountCents"`
	// Currency is always "EUR".
	Currency string `json:"currency"`
	// Country is the country used for the VAT (ISO 3166-1 alpha-2).
	Country string `json:"country"`
	// VatRateBp is the VAT rate in basis points (2100 = 21 %).
	VatRateBp int64 `json:"vatRateBp"`
	// VatCents is the VAT in cents.
	VatCents int64 `json:"vatCents"`
	// NetCents is the amount without VAT in cents.
	NetCents int64 `json:"netCents"`
	// CreatedAt is the creation time.
	CreatedAt time.Time `json:"createdAt"`
	// PaidAt is the payment time, nil until paid.
	PaidAt *time.Time `json:"paidAt"`
	// FulfilledAt is the delivery time, nil until fulfilled.
	FulfilledAt *time.Time `json:"fulfilledAt"`
	// RefundedAt is the refund time, nil unless refunded.
	RefundedAt *time.Time `json:"refundedAt"`
}

// Checkout is the result of the creation of a checkout.
type Checkout struct {
	// Order is the order (pending until paid).
	Order Order `json:"order"`
	// CheckoutURL is the hosted payment page to send the player to.
	CheckoutURL string `json:"checkoutUrl"`
	// Replayed is true when the response was the replay of an earlier request with the same idempotency key (the order
	// is then returned in its current state).
	Replayed bool `json:"-"`
}
