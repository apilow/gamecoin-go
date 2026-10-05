package gamecoin_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	gamecoin "github.com/apilow/gamecoin-go"
)

func ledgerEntry(id string) string {
	return fmt.Sprintf(`{"id":%q,"type":"grant","currency":"gems","paidDelta":0,"bonusDelta":1,"balanceAfter":{"paid":0,"bonus":1,"total":1},"reason":null,"metadata":{},"ref":{},"createdAt":"2026-10-05T20:38:27.227Z"}`, id)
}

func ledgerPage(nextCursor string, ids ...string) string {
	entries := make([]string, len(ids))
	for i, id := range ids {
		entries[i] = ledgerEntry(id)
	}
	cursor := "null"
	if nextCursor != "" {
		cursor = fmt.Sprintf("%q", nextCursor)
	}
	return fmt.Sprintf(`{"entries":[%s],"nextCursor":%s}`, strings.Join(entries, ","), cursor)
}

func collect(it *gamecoin.LedgerIterator) []string {
	var ids []string
	for it.Next() {
		ids = append(ids, it.Entry().ID)
	}
	return ids
}

func TestIteratesAcrossSeveralPagesInOrder(t *testing.T) {
	r := newRig(t)
	r.f.ok(ledgerPage("c1", "e1", "e2")).ok(ledgerPage("c2", "e3", "e4")).ok(ledgerPage("", "e5"))

	it := r.client.Ledger.Iterate(context.Background(), user, &gamecoin.LedgerListParams{Currency: "gems", Limit: 2})
	ids := collect(it)

	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"e1", "e2", "e3", "e4", "e5"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	reqs := r.f.requests()
	if len(reqs) != 3 {
		t.Fatalf("%d requests, want 3", len(reqs))
	}
	checkRequest(t, reqs[0], "GET", "/api/v1/players/ext%3Auser-42/ledger?currency=gems&limit=2", "")
	checkRequest(t, reqs[1], "GET", "/api/v1/players/ext%3Auser-42/ledger?currency=gems&limit=2&cursor=c1", "")
	checkRequest(t, reqs[2], "GET", "/api/v1/players/ext%3Auser-42/ledger?currency=gems&limit=2&cursor=c2", "")
}

func TestFetchesPagesLazily(t *testing.T) {
	r := newRig(t)
	r.f.ok(ledgerPage("c1", "e1", "e2")).ok(ledgerPage("", "e3"))

	it := r.client.Ledger.Iterate(context.Background(), user, nil)
	if n := len(r.f.requests()); n != 0 {
		t.Fatalf("%d requests before the first Next", n)
	}

	for i, want := range []string{"e1", "e2"} {
		if !it.Next() || it.Entry().ID != want {
			t.Fatalf("entry %d: want %s", i, want)
		}
	}
	if n := len(r.f.requests()); n != 1 {
		t.Errorf("%d requests while the first page is consumed, want 1", n)
	}
	if !it.Next() || it.Entry().ID != "e3" {
		t.Fatal("want e3")
	}
	if n := len(r.f.requests()); n != 2 {
		t.Errorf("%d requests, want 2", n)
	}
	if it.Next() || it.Err() != nil {
		t.Errorf("the end of the ledger: Next/Err = %v", it.Err())
	}
	if n := len(r.f.requests()); n != 2 {
		t.Errorf("%d requests after the end, want 2", n)
	}
}

func TestStoppingEarlyRequestsNoFurtherPage(t *testing.T) {
	r := newRig(t)
	r.f.ok(ledgerPage("c1", "e1", "e2")).ok(ledgerPage("", "e3"))

	it := r.client.Ledger.Iterate(context.Background(), user, nil)
	var first []string
	for it.Next() {
		first = append(first, it.Entry().ID)
		if len(first) == 2 {
			break
		}
	}

	if !reflect.DeepEqual(first, []string{"e1", "e2"}) || len(r.f.requests()) != 1 {
		t.Errorf("first = %v after %d requests", first, len(r.f.requests()))
	}
}

func TestAnEmptyLedgerYieldsNothingAfterOneRequest(t *testing.T) {
	r := newRig(t)
	r.f.ok(ledgerPage(""))

	it := r.client.Ledger.Iterate(context.Background(), user, nil)

	if it.Next() || it.Err() != nil || len(r.f.requests()) != 1 {
		t.Errorf("Err = %v after %d requests", it.Err(), len(r.f.requests()))
	}
}

func TestAnEmptyPageWithACursorIsSkipped(t *testing.T) {
	r := newRig(t)
	r.f.ok(ledgerPage("c1")).ok(ledgerPage("", "e1"))

	it := r.client.Ledger.Iterate(context.Background(), user, nil)

	if ids := collect(it); !reflect.DeepEqual(ids, []string{"e1"}) || it.Err() != nil {
		t.Errorf("ids = %v, err = %v", ids, it.Err())
	}
}

func TestAStartingCursorResumesFromThatPage(t *testing.T) {
	r := newRig(t)
	r.f.ok(ledgerPage("c3", "e5")).ok(ledgerPage("", "e6"))

	it := r.client.Ledger.Iterate(context.Background(), user, &gamecoin.LedgerListParams{Cursor: "c2"})
	ids := collect(it)

	reqs := r.f.requests()
	if !reflect.DeepEqual(ids, []string{"e5", "e6"}) || len(reqs) != 2 {
		t.Fatalf("ids = %v after %d requests", ids, len(reqs))
	}
	if !strings.HasSuffix(reqs[0].RequestURI, "ledger?cursor=c2") || !strings.HasSuffix(reqs[1].RequestURI, "ledger?cursor=c3") {
		t.Errorf("targets = %s, %s", reqs[0].RequestURI, reqs[1].RequestURI)
	}
}

func TestAnErrorOnALaterPageIsReturnedAfterTheEarlierEntries(t *testing.T) {
	r := newRig(t, gamecoin.WithMaxRetries(0))
	r.f.ok(ledgerPage("c1", "e1")).apiError(400, "VALIDATION_FAILED", "bad cursor", `{"fieldErrors":{"cursor":["CURSOR_INVALID"]}}`)

	it := r.client.Ledger.Iterate(context.Background(), user, nil)
	ids := collect(it)

	if !reflect.DeepEqual(ids, []string{"e1"}) {
		t.Errorf("ids = %v", ids)
	}
	if gcErr := asError(t, it.Err()); gcErr.Code != gamecoin.CodeValidationFailed {
		t.Errorf("error = %+v", gcErr)
	}
	if it.Next() {
		t.Error("an iterator that failed stays finished")
	}
}

func TestATransientFailureOnAPageIsRetriedTransparently(t *testing.T) {
	r := newRig(t)
	r.f.ok(ledgerPage("c1", "e1")).apiError(503, "INTERNAL", "blip", "").ok(ledgerPage("", "e2"))

	it := r.client.Ledger.Iterate(context.Background(), user, nil)
	ids := collect(it)

	if !reflect.DeepEqual(ids, []string{"e1", "e2"}) || it.Err() != nil || len(r.f.requests()) != 3 {
		t.Errorf("ids = %v, err = %v, %d requests", ids, it.Err(), len(r.f.requests()))
	}
}

func TestIterationCanBeCancelled(t *testing.T) {
	r := newRig(t)
	r.f.ok(ledgerPage("c1", "e1")).ok(ledgerPage("", "e2"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	it := r.client.Ledger.Iterate(ctx, user, nil)
	var ids []string
	for it.Next() {
		ids = append(ids, it.Entry().ID)
		cancel()
	}

	if !reflect.DeepEqual(ids, []string{"e1"}) || !errors.Is(it.Err(), context.Canceled) || len(r.f.requests()) != 1 {
		t.Errorf("ids = %v, err = %v, %d requests", ids, it.Err(), len(r.f.requests()))
	}
}

func TestAnInvalidArgumentIsReportedByErrAfterTheFirstNext(t *testing.T) {
	r := newRig(t)

	it := r.client.Ledger.Iterate(context.Background(), user, &gamecoin.LedgerListParams{Limit: -1})

	if it.Next() || !isArgError(it.Err()) || len(r.f.requests()) != 0 {
		t.Errorf("Err = %v after %d requests", it.Err(), len(r.f.requests()))
	}
}
