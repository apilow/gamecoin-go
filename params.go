package gamecoin

// ProfileField names a field of a player profile, to say which ones PlayerProfile.Clear erases.
type ProfileField string

// The fields of a player profile.
const (
	// FieldDisplayName is the display name.
	FieldDisplayName ProfileField = "displayName"
	// FieldEmail is the e-mail address.
	FieldEmail ProfileField = "email"
	// FieldCountry is the country.
	FieldCountry ProfileField = "country"
	// FieldBirthYear is the birth year.
	FieldBirthYear ProfileField = "birthYear"
)

// PlayerProfile holds the fields Players.Upsert can set. A nil field leaves the value unchanged; to erase a field, list
// it in Clear. A field cannot be both set and cleared.
type PlayerProfile struct {
	// DisplayName is the display name (200 characters at most).
	DisplayName *string
	// Email is the e-mail address.
	Email *string
	// Country is the ISO 3166-1 alpha-2 country.
	Country *string
	// BirthYear is the birth year.
	BirthYear *int
	// Clear lists the fields to erase (sent as null).
	Clear []ProfileField
}

// GrantParams is the request of Wallet.Grant: it credits a currency to a player, always to the bonus bucket.
type GrantParams struct {
	// Currency is the currency code ("gems"). Required.
	Currency string
	// Amount is a positive integer. Required.
	Amount int64
	// Reason is a free-form reason (500 characters at most), kept in the ledger. Optional.
	Reason string
	// Metadata is free-form string metadata, kept in the ledger. Optional.
	Metadata map[string]string
}

// SpendParams is the request of Wallet.Spend: it debits a currency from a player.
type SpendParams struct {
	// Currency is the currency code ("gems"). Required.
	Currency string
	// Amount is a positive integer. Required.
	Amount int64
	// Reason is a free-form reason (500 characters at most), kept in the ledger. Optional.
	Reason string
	// Metadata is free-form string metadata, kept in the ledger. Optional.
	Metadata map[string]string
}

// InventoryParams is the request of Inventory.Grant and Inventory.Consume.
type InventoryParams struct {
	// SKU is the item sku. Required.
	SKU string
	// Quantity is a positive integer; 0 means 1.
	Quantity int64
}

// PurchaseParams is the request of Purchases.Create: it buys an item with its currency price.
type PurchaseParams struct {
	// SKU is the item sku. Required.
	SKU string
	// Quantity is a positive integer; 0 means 1.
	Quantity int64
}

// CheckoutParams is the request of Checkouts.Create: it starts the purchase of a pack with real money.
type CheckoutParams struct {
	// PackSKU is the pack sku. Required.
	PackSKU string
	// SuccessURL is where the player goes after paying ("?order=…&status=fulfilled" is appended). Optional.
	SuccessURL string
	// CancelURL is where the player goes when giving up. Optional.
	CancelURL string
	// Country is the ISO 3166-1 alpha-2 country of the player, for the VAT. Optional.
	Country string
}

// RefundParams is the optional request of Orders.Refund.
type RefundParams struct {
	// Reason is a free-form reason (500 characters at most).
	Reason string
}

// LedgerListParams filters and pages a ledger listing. The zero value lists the first page with the API defaults.
type LedgerListParams struct {
	// Currency restricts the listing to the entries of one currency.
	Currency string
	// Limit is the page size, 1 to 100 (0 means the API default, 20).
	Limit int
	// Cursor is the cursor of the page to fetch (LedgerPage.NextCursor of the previous page). Optional.
	Cursor string
}
