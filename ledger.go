package gamecoin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

// LedgerService reads the ledger of a player (GET /players/{player}/ledger). Get it from Client.Ledger.
type LedgerService struct{ c *core }

// List returns one page of the ledger, newest entries first. A nil params lists the first page with the API defaults.
func (s *LedgerService) List(ctx context.Context, player PlayerRef, params *LedgerListParams) (*LedgerPage, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	query, err := ledgerQuery(params)
	if err != nil {
		return nil, err
	}
	var out LedgerPage
	if _, err := s.c.do(ctx, request{method: http.MethodGet, path: path + "/ledger", query: query}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Iterate walks every entry of the ledger, page after page, lazily: a page is fetched only when the previous one is
// exhausted. LedgerListParams.Cursor resumes from a given page; a nil params walks the whole ledger with the default page
// size.
//
//	it := client.Ledger.Iterate(ctx, player, &gamecoin.LedgerListParams{Currency: "gems", Limit: 50})
//	for it.Next() {
//		entry := it.Entry()
//		// ...
//	}
//	if err := it.Err(); err != nil {
//		// a page could not be fetched
//	}
//
// An invalid argument is reported by Err right away; Next then returns false without sending anything.
func (s *LedgerService) Iterate(ctx context.Context, player PlayerRef, params *LedgerListParams) *LedgerIterator {
	it := &LedgerIterator{service: s, ctx: ctx, player: player}
	if params != nil {
		it.params = *params
	}
	it.cursor = it.params.Cursor
	// Arguments are checked right away (Err reports them); requests start with the first call to Next.
	if _, err := playerPath(player); err != nil {
		it.err = err
	} else if _, err := ledgerQuery(&it.params); err != nil {
		it.err = err
	}
	return it
}

// LedgerIterator walks the entries of a ledger. Create it with LedgerService.Iterate. It is not safe for concurrent use.
type LedgerIterator struct {
	service *LedgerService
	ctx     context.Context
	player  PlayerRef
	params  LedgerListParams

	cursor  string
	page    []LedgerEntry
	index   int // index of the next entry of page to return
	done    bool
	current LedgerEntry
	err     error
}

// Next advances to the next entry and reports whether there is one. It fetches the next page when needed. When it
// returns false, check Err to tell the end of the ledger from a failure.
func (it *LedgerIterator) Next() bool {
	for {
		if it.err != nil {
			return false
		}
		if it.index < len(it.page) {
			it.current = it.page[it.index]
			it.index++
			return true
		}
		if it.done {
			return false
		}

		params := it.params
		params.Cursor = it.cursor
		page, err := it.service.List(it.ctx, it.player, &params)
		if err != nil {
			it.err = err
			return false
		}
		it.page = page.Entries
		it.index = 0
		it.cursor = ""
		if page.NextCursor != nil {
			it.cursor = *page.NextCursor
		}
		if it.cursor == "" {
			// the last page: its entries are still to be returned, then the loop ends
			it.done = true
		}
	}
}

// Entry returns the current entry. It is only meaningful after Next returned true.
func (it *LedgerIterator) Entry() LedgerEntry { return it.current }

// Err returns the error that stopped the iteration, nil when it reached the end of the ledger (or is not finished).
func (it *LedgerIterator) Err() error { return it.err }

func ledgerQuery(params *LedgerListParams) (string, error) {
	if params == nil {
		return "", nil
	}
	var parts []string
	if params.Currency != "" {
		parts = append(parts, "currency="+escapeSegment(params.Currency))
	}
	if params.Limit != 0 {
		if params.Limit < 1 || params.Limit > 100 {
			return "", argError("the ledger limit must be an integer from 1 to 100")
		}
		parts = append(parts, "limit="+strconv.Itoa(params.Limit))
	}
	if params.Cursor != "" {
		parts = append(parts, "cursor="+escapeSegment(params.Cursor))
	}
	return strings.Join(parts, "&"), nil
}
