package gamecoin

import (
	"context"
	"net/http"
)

// CatalogService reads the catalog of the game (GET /catalog). Get it from Client.Catalog.
type CatalogService struct{ c *core }

// Get returns the active currencies, items and packs of the game.
func (s *CatalogService) Get(ctx context.Context) (*Catalog, error) {
	var out Catalog
	if _, err := s.c.do(ctx, request{method: http.MethodGet, path: "/catalog"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
