package gamecoin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

// Catalog management (v0.4): currencies, items, packs. What each call sends, what it returns, the arguments refused before any
// request, and the retry policy.

const (
	currencyJSON = `{"code":"gems","name":"Gems","purchasable":true,"status":"active","createdAt":"2026-10-05T12:00:00.000Z","updatedAt":"2026-10-05T13:00:00.000Z"}`
	itemJSON     = `{"sku":"potion","name":"Potion","description":null,"type":"consumable","price":{"currency":"gems","amount":20},"maxOwned":null,"clientPurchasable":true,"metadata":{"heal":"50"},"status":"active","createdAt":"2026-10-05T12:00:00.000Z","updatedAt":"2026-10-05T13:00:00.000Z"}`
	packJSON     = `{"sku":"gems-500","name":"500 gems","description":null,"priceCents":499,"currency":"EUR","grants":[{"currency":"gems","amount":500,"bonus":50}],"items":[{"sku":"potion","quantity":2}],"badge":"popular","maxPerPlayer":null,"sortOrder":10,"status":"active","availability":{"startsAt":"2026-11-01T00:00:00.000Z","endsAt":null,"stock":1000,"founder":false,"sold":4},"createdAt":"2026-10-05T12:00:00.000Z","updatedAt":"2026-10-05T13:00:00.000Z"}`
)

// sameJSON compares two JSON documents, whatever the order of their keys.
func sameJSON(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Errorf("body %q is not JSON: %v", got, err)
		return
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad expectation %q: %v", want, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func checkCall(t *testing.T, r *rig, method, target, body string) {
	t.Helper()
	req := r.f.only()
	if req.Method != method || req.RequestURI != target {
		t.Errorf("request = %s %s, want %s %s", req.Method, req.RequestURI, method, target)
	}
	if body == "" {
		if req.Body != "" {
			t.Errorf("body = %s, want none", req.Body)
		}
	} else {
		sameJSON(t, req.Body, body)
	}
	if key := req.Header.Get("Idempotency-Key"); key != "" {
		t.Errorf("a catalog call must not carry an idempotency key, got %q", key)
	}
}

func TestCurrenciesRequests(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, reply, method, target, body string
		call                              func(c *gamecoin.Client) error
	}{
		{"list", `{"currencies":[` + currencyJSON + `]}`, http.MethodGet, "/api/v1/currencies", "", func(c *gamecoin.Client) error {
			_, err := c.Currencies.List(ctx, nil)
			return err
		}},
		{"list with archived", `{"currencies":[]}`, http.MethodGet, "/api/v1/currencies?includeArchived=true", "", func(c *gamecoin.Client) error {
			_, err := c.Currencies.List(ctx, &gamecoin.ListParams{IncludeArchived: true})
			return err
		}},
		{"create", currencyJSON, http.MethodPost, "/api/v1/currencies", `{"code":"gems","name":"Gems","purchasable":true}`, func(c *gamecoin.Client) error {
			_, err := c.Currencies.Create(ctx, gamecoin.CreateCurrencyParams{Code: "gems", Name: "Gems", Purchasable: gamecoin.Ptr(true)})
			return err
		}},
		{"create without purchasable", currencyJSON, http.MethodPost, "/api/v1/currencies", `{"code":"gems","name":"Gems"}`, func(c *gamecoin.Client) error {
			_, err := c.Currencies.Create(ctx, gamecoin.CreateCurrencyParams{Code: "gems", Name: "Gems"})
			return err
		}},
		{"get", currencyJSON, http.MethodGet, "/api/v1/currencies/gems", "", func(c *gamecoin.Client) error {
			_, err := c.Currencies.Get(ctx, "gems")
			return err
		}},
		{"update", currencyJSON, http.MethodPatch, "/api/v1/currencies/gems", `{"name":"Shiny","purchasable":false}`, func(c *gamecoin.Client) error {
			_, err := c.Currencies.Update(ctx, "gems", &gamecoin.UpdateCurrencyParams{Name: gamecoin.Ptr("Shiny"), Purchasable: gamecoin.Ptr(false)})
			return err
		}},
		{"update with nil params", currencyJSON, http.MethodPatch, "/api/v1/currencies/gems", `{}`, func(c *gamecoin.Client) error {
			_, err := c.Currencies.Update(ctx, "gems", nil)
			return err
		}},
		{"archive", currencyJSON, http.MethodDelete, "/api/v1/currencies/gems", "", func(c *gamecoin.Client) error {
			_, err := c.Currencies.Archive(ctx, "gems")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.f.ok(tc.reply)
			if err := tc.call(r.client); err != nil {
				t.Fatal(err)
			}
			checkCall(t, r, tc.method, tc.target, tc.body)
		})
	}
}

func TestItemsRequests(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, reply, method, target, body string
		call                              func(c *gamecoin.Client) error
	}{
		{"list", `{"items":[` + itemJSON + `]}`, http.MethodGet, "/api/v1/items", "", func(c *gamecoin.Client) error {
			_, err := c.Items.List(ctx, nil)
			return err
		}},
		{"get encodes the sku", itemJSON, http.MethodGet, "/api/v1/items/a%20b%2Fc", "", func(c *gamecoin.Client) error {
			_, err := c.Items.Get(ctx, "a b/c")
			return err
		}},
		{"create", itemJSON, http.MethodPost, "/api/v1/items",
			`{"sku":"potion","name":"Potion","type":"consumable","price":{"currency":"gems","amount":20},"clientPurchasable":true,"metadata":{"heal":"50"}}`,
			func(c *gamecoin.Client) error {
				_, err := c.Items.Create(ctx, gamecoin.CreateItemParams{
					SKU: "potion", Name: "Potion", Type: gamecoin.ItemConsumable, Price: &gamecoin.Price{Currency: "gems", Amount: 20},
					ClientPurchasable: true, Metadata: map[string]string{"heal": "50"},
				})
				return err
			}},
		{"create with the default limit", itemJSON, http.MethodPost, "/api/v1/items", `{"sku":"x","name":"X","type":"durable"}`, func(c *gamecoin.Client) error {
			_, err := c.Items.Create(ctx, gamecoin.CreateItemParams{SKU: "x", Name: "X", Type: gamecoin.ItemDurable})
			return err
		}},
		{"create with no limit", itemJSON, http.MethodPost, "/api/v1/items", `{"sku":"x","name":"X","type":"durable","maxOwned":null}`, func(c *gamecoin.Client) error {
			_, err := c.Items.Create(ctx, gamecoin.CreateItemParams{SKU: "x", Name: "X", Type: gamecoin.ItemDurable, NoMaxOwned: true})
			return err
		}},
		{"update", itemJSON, http.MethodPatch, "/api/v1/items/potion", `{"name":"Big potion","price":{"currency":"gems","amount":25},"maxOwned":5}`, func(c *gamecoin.Client) error {
			_, err := c.Items.Update(ctx, "potion", &gamecoin.UpdateItemParams{
				Name: gamecoin.Ptr("Big potion"), Price: &gamecoin.Price{Currency: "gems", Amount: 25}, MaxOwned: gamecoin.Ptr(int64(5)),
			})
			return err
		}},
		{"update clearing fields", itemJSON, http.MethodPatch, "/api/v1/items/potion", `{"price":null,"description":null,"maxOwned":null}`, func(c *gamecoin.Client) error {
			_, err := c.Items.Update(ctx, "potion", &gamecoin.UpdateItemParams{Clear: []gamecoin.ItemField{gamecoin.ItemFieldPrice, gamecoin.ItemFieldDescription, gamecoin.ItemFieldMaxOwned}})
			return err
		}},
		{"update with empty metadata", itemJSON, http.MethodPatch, "/api/v1/items/potion", `{"metadata":{}}`, func(c *gamecoin.Client) error {
			_, err := c.Items.Update(ctx, "potion", &gamecoin.UpdateItemParams{Metadata: map[string]string{}})
			return err
		}},
		{"archive", itemJSON, http.MethodDelete, "/api/v1/items/potion", "", func(c *gamecoin.Client) error {
			_, err := c.Items.Archive(ctx, "potion")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.f.ok(tc.reply)
			if err := tc.call(r.client); err != nil {
				t.Fatal(err)
			}
			checkCall(t, r, tc.method, tc.target, tc.body)
		})
	}
}

func TestPacksRequests(t *testing.T) {
	ctx := context.Background()
	starts := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ends := time.Date(2026, 12, 1, 0, 0, 0, 0, time.FixedZone("CET", 3600))
	cases := []struct {
		name, reply, method, target, body string
		call                              func(c *gamecoin.Client) error
	}{
		{"list with archived", `{"packs":[` + packJSON + `]}`, http.MethodGet, "/api/v1/packs?includeArchived=true", "", func(c *gamecoin.Client) error {
			_, err := c.Packs.List(ctx, &gamecoin.ListParams{IncludeArchived: true})
			return err
		}},
		{"get", packJSON, http.MethodGet, "/api/v1/packs/gems-500", "", func(c *gamecoin.Client) error {
			_, err := c.Packs.Get(ctx, "gems-500")
			return err
		}},
		{"create", packJSON, http.MethodPost, "/api/v1/packs",
			`{"sku":"gems-500","name":"500 gems","priceCents":499,"grants":[{"currency":"gems","amount":500,"bonus":50}],"items":[{"sku":"potion","quantity":2}],"badge":"popular","maxPerPlayer":1,"sortOrder":10,` +
				`"availability":{"startsAt":"2026-11-01T00:00:00Z","endsAt":"2026-11-30T23:00:00Z","stock":1000,"founder":true},"status":"active"}`,
			func(c *gamecoin.Client) error {
				_, err := c.Packs.Create(ctx, gamecoin.CreatePackParams{
					SKU: "gems-500", Name: "500 gems", PriceCents: 499,
					Grants: []gamecoin.PackGrant{{Currency: "gems", Amount: 500, Bonus: 50}},
					Items:  []gamecoin.PackItem{{SKU: "potion", Quantity: 2}},
					Badge:  "popular", MaxPerPlayer: 1, SortOrder: 10,
					Availability: &gamecoin.PackAvailabilityParams{StartsAt: &starts, EndsAt: &ends, Stock: gamecoin.Ptr(int64(1000)), Founder: true},
					Status:       gamecoin.PackActive,
				})
				return err
			}},
		{"create keeps it minimal", packJSON, http.MethodPost, "/api/v1/packs", `{"sku":"x","name":"X","priceCents":500,"items":[{"sku":"y","quantity":1}]}`, func(c *gamecoin.Client) error {
			_, err := c.Packs.Create(ctx, gamecoin.CreatePackParams{SKU: "x", Name: "X", PriceCents: 500, Items: []gamecoin.PackItem{{SKU: "y", Quantity: 1}}})
			return err
		}},
		{"update", packJSON, http.MethodPatch, "/api/v1/packs/gems-500",
			`{"priceCents":599,"sortOrder":-3,"availability":{"startsAt":null,"endsAt":null,"stock":null,"founder":false},"status":"active","badge":null}`,
			func(c *gamecoin.Client) error {
				_, err := c.Packs.Update(ctx, "gems-500", &gamecoin.UpdatePackParams{
					PriceCents: gamecoin.Ptr(int64(599)), SortOrder: gamecoin.Ptr(int64(-3)), Availability: &gamecoin.PackAvailabilityParams{},
					Status: gamecoin.PackActive, Clear: []gamecoin.PackField{gamecoin.PackFieldBadge},
				})
				return err
			}},
		{"update with empty lists", packJSON, http.MethodPatch, "/api/v1/packs/gems-500", `{"grants":[],"items":[]}`, func(c *gamecoin.Client) error {
			_, err := c.Packs.Update(ctx, "gems-500", &gamecoin.UpdatePackParams{Grants: []gamecoin.PackGrant{}, Items: []gamecoin.PackItem{}})
			return err
		}},
		{"archive", packJSON, http.MethodDelete, "/api/v1/packs/gems-500", "", func(c *gamecoin.Client) error {
			_, err := c.Packs.Archive(ctx, "gems-500")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.f.ok(tc.reply)
			if err := tc.call(r.client); err != nil {
				t.Fatal(err)
			}
			checkCall(t, r, tc.method, tc.target, tc.body)
		})
	}
}

func TestCatalogManagementResults(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.f.ok(currencyJSON).ok(itemJSON).ok(packJSON).ok(`{"packs":[` + packJSON + `,` + packJSON + `]}`)

	currency, err := r.client.Currencies.Get(ctx, "gems")
	if err != nil {
		t.Fatal(err)
	}
	if currency.Code != "gems" || !currency.Purchasable || currency.Status != gamecoin.CatalogActive ||
		!currency.CreatedAt.Equal(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)) || currency.UpdatedAt.Sub(currency.CreatedAt) != time.Hour {
		t.Errorf("currency = %+v", currency)
	}

	item, err := r.client.Items.Get(ctx, "potion")
	if err != nil {
		t.Fatal(err)
	}
	if item.Price == nil || item.Price.Amount != 20 || item.MaxOwned != nil || item.Metadata["heal"] != "50" || item.Type != gamecoin.ItemConsumable {
		t.Errorf("item = %+v", item)
	}

	pack, err := r.client.Packs.Get(ctx, "gems-500")
	if err != nil {
		t.Fatal(err)
	}
	if pack.SortOrder != 10 || pack.Status != gamecoin.PackActive || pack.Badge == nil || *pack.Badge != "popular" || len(pack.Grants) != 1 || pack.Items[0].SKU != "potion" {
		t.Errorf("pack = %+v", pack)
	}
	a := pack.Availability
	if a == nil || a.StartsAt == nil || !a.StartsAt.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) || a.EndsAt != nil || a.Stock == nil || *a.Stock != 1000 || a.Founder || a.Sold != 4 {
		t.Errorf("availability = %+v", a)
	}

	packs, err := r.client.Packs.List(ctx, nil)
	if err != nil || len(packs) != 2 {
		t.Errorf("List = %d packs, %v", len(packs), err)
	}
}

func TestCatalogManagementWrongShapeIsInvalidResponse(t *testing.T) {
	r := newRig(t)
	r.f.ok(`{"code":"gems"}`)
	_, err := r.client.Currencies.Get(context.Background(), "gems")
	expectCode(t, "wrong shape", err, gamecoin.CodeInvalidResponse, 200)
}

func TestCatalogManagementErrorsCarryDetails(t *testing.T) {
	r := newRig(t)
	r.f.apiError(409, "CONFLICT", "in use", `{"blockedBy":{"packs":["gems-500"],"items":[]}}`)
	_, err := r.client.Currencies.Archive(context.Background(), "gems")
	gcErr := expectCode(t, "blocked", err, gamecoin.CodeConflict, 409)
	if _, ok := gcErr.Details["blockedBy"]; !ok {
		t.Errorf("Details = %v", gcErr.Details)
	}
}

func TestCatalogManagementRefusesBadArgumentsBeforeAnyRequest(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	bad := map[string]func() error{
		"empty currency code": func() error { _, err := r.client.Currencies.Get(ctx, ""); return err },
		"create without a name": func() error {
			_, err := r.client.Currencies.Create(ctx, gamecoin.CreateCurrencyParams{Code: "gems"})
			return err
		},
		"update to an empty name": func() error {
			_, err := r.client.Currencies.Update(ctx, "gems", &gamecoin.UpdateCurrencyParams{Name: gamecoin.Ptr("")})
			return err
		},
		"item without a type": func() error {
			_, err := r.client.Items.Create(ctx, gamecoin.CreateItemParams{SKU: "x", Name: "X"})
			return err
		},
		"item with an unknown type": func() error {
			_, err := r.client.Items.Create(ctx, gamecoin.CreateItemParams{SKU: "x", Name: "X", Type: "weapon"})
			return err
		},
		"item price of 0": func() error {
			_, err := r.client.Items.Create(ctx, gamecoin.CreateItemParams{SKU: "x", Name: "X", Type: gamecoin.ItemDurable, Price: &gamecoin.Price{Currency: "gems"}})
			return err
		},
		"max owned of 0": func() error {
			_, err := r.client.Items.Create(ctx, gamecoin.CreateItemParams{SKU: "x", Name: "X", Type: gamecoin.ItemDurable, MaxOwned: gamecoin.Ptr(int64(0))})
			return err
		},
		"max owned and no max owned": func() error {
			_, err := r.client.Items.Create(ctx, gamecoin.CreateItemParams{SKU: "x", Name: "X", Type: gamecoin.ItemDurable, MaxOwned: gamecoin.Ptr(int64(2)), NoMaxOwned: true})
			return err
		},
		"item field both set and cleared": func() error {
			_, err := r.client.Items.Update(ctx, "x", &gamecoin.UpdateItemParams{Price: &gamecoin.Price{Currency: "gems", Amount: 1}, Clear: []gamecoin.ItemField{gamecoin.ItemFieldPrice}})
			return err
		},
		"item unknown clear field": func() error {
			_, err := r.client.Items.Update(ctx, "x", &gamecoin.UpdateItemParams{Clear: []gamecoin.ItemField{"name"}})
			return err
		},
		"pack with a negative price": func() error {
			_, err := r.client.Packs.Create(ctx, gamecoin.CreatePackParams{SKU: "x", Name: "X", PriceCents: -1})
			return err
		},
		"pack grant without a currency": func() error {
			_, err := r.client.Packs.Create(ctx, gamecoin.CreatePackParams{SKU: "x", Name: "X", PriceCents: 500, Grants: []gamecoin.PackGrant{{Amount: 5}}})
			return err
		},
		"pack item quantity of 0": func() error {
			_, err := r.client.Packs.Create(ctx, gamecoin.CreatePackParams{SKU: "x", Name: "X", PriceCents: 500, Items: []gamecoin.PackItem{{SKU: "y"}}})
			return err
		},
		"pack badge out of the list": func() error {
			_, err := r.client.Packs.Create(ctx, gamecoin.CreatePackParams{SKU: "x", Name: "X", PriceCents: 500, Badge: "hot"})
			return err
		},
		"archived on create": func() error {
			_, err := r.client.Packs.Create(ctx, gamecoin.CreatePackParams{SKU: "x", Name: "X", PriceCents: 500, Status: gamecoin.PackArchived})
			return err
		},
		"a stock of 0": func() error {
			_, err := r.client.Packs.Update(ctx, "x", &gamecoin.UpdatePackParams{Availability: &gamecoin.PackAvailabilityParams{Stock: gamecoin.Ptr(int64(0))}})
			return err
		},
		"a status out of the list": func() error {
			_, err := r.client.Packs.Update(ctx, "x", &gamecoin.UpdatePackParams{Status: "deleted"})
			return err
		},
		"pack max per player of 0": func() error {
			_, err := r.client.Packs.Update(ctx, "x", &gamecoin.UpdatePackParams{MaxPerPlayer: gamecoin.Ptr(int64(0))})
			return err
		},
		"pack field both set and cleared": func() error {
			_, err := r.client.Packs.Update(ctx, "x", &gamecoin.UpdatePackParams{Badge: gamecoin.Ptr("popular"), Clear: []gamecoin.PackField{gamecoin.PackFieldBadge}})
			return err
		},
	}
	for name, call := range bad {
		err := call()
		if !errors.Is(err, gamecoin.ErrInvalidArgument) {
			t.Errorf("%s: err = %v, want ErrInvalidArgument", name, err)
		}
	}
	if n := len(r.f.requests()); n != 0 {
		t.Errorf("%d requests were sent", n)
	}
}

func TestCatalogManagementRetries(t *testing.T) {
	ctx := context.Background()

	t.Run("a create is retried on 429 only", func(t *testing.T) {
		r := newRig(t)
		r.f.apiError(429, "RATE_LIMITED", "slow down", "", "Retry-After", "2").ok(currencyJSON)
		if _, err := r.client.Currencies.Create(ctx, gamecoin.CreateCurrencyParams{Code: "gems", Name: "Gems"}); err != nil {
			t.Fatal(err)
		}
		if n := len(r.f.requests()); n != 2 {
			t.Errorf("%d requests, want 2", n)
		}
		if len(r.sleeps) != 1 || r.sleeps[0] != 2*time.Second {
			t.Errorf("sleeps = %v", r.sleeps)
		}
	})

	creates := map[string]func(c *gamecoin.Client) error{
		"currency": func(c *gamecoin.Client) error {
			_, err := c.Currencies.Create(ctx, gamecoin.CreateCurrencyParams{Code: "gems", Name: "Gems"})
			return err
		},
		"item": func(c *gamecoin.Client) error {
			_, err := c.Items.Create(ctx, gamecoin.CreateItemParams{SKU: "x", Name: "X", Type: gamecoin.ItemDurable})
			return err
		},
		"pack": func(c *gamecoin.Client) error {
			_, err := c.Packs.Create(ctx, gamecoin.CreatePackParams{SKU: "x", Name: "X", PriceCents: 500, Items: []gamecoin.PackItem{{SKU: "y", Quantity: 1}}})
			return err
		},
	}
	for name, create := range creates {
		t.Run("a "+name+" create is not retried after a 5xx or a network error", func(t *testing.T) {
			r := newRig(t)
			r.f.apiError(502, "INTERNAL", "bad gateway", "")
			if err := create(r.client); err == nil {
				t.Fatal("expected an error")
			}
			if n := len(r.f.requests()); n != 1 {
				t.Errorf("%d requests after a 502, want 1", n)
			}
			r = newRig(t)
			r.f.drop()
			if err := create(r.client); err == nil {
				t.Fatal("expected an error")
			}
			if n := len(r.f.requests()); n != 1 {
				t.Errorf("%d requests after a dropped connection, want 1", n)
			}
		})
	}

	t.Run("reads, updates and archives are retried on 5xx", func(t *testing.T) {
		calls := map[string]func(c *gamecoin.Client){
			"update": func(c *gamecoin.Client) {
				_, _ = c.Currencies.Update(ctx, "gems", &gamecoin.UpdateCurrencyParams{Name: gamecoin.Ptr("G")})
			},
			"archive": func(c *gamecoin.Client) { _, _ = c.Items.Archive(ctx, "potion") },
			"get":     func(c *gamecoin.Client) { _, _ = c.Packs.Get(ctx, "x") },
			"list":    func(c *gamecoin.Client) { _, _ = c.Packs.List(ctx, nil) },
		}
		for name, call := range calls {
			r := newRig(t)
			r.f.apiError(503, "INTERNAL", "unavailable", "").ok(`{"unrelated":true}`)
			call(r.client)
			if n := len(r.f.requests()); n != 2 {
				t.Errorf("%s: %d requests, want 2", name, n)
			}
		}
	})
}

func TestVersionIs04(t *testing.T) {
	if gamecoin.Version != "0.4.0" {
		t.Errorf("Version = %q", gamecoin.Version)
	}
}
