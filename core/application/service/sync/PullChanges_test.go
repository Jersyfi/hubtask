// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	device = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	// Two hubs, three collections: the shape every depth question needs.
	hub         = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")
	collectionA = shared.ID("01936f2a-7c1e-7000-8000-0000000000c2")
	collectionB = shared.ID("01936f2a-7c1e-7000-8000-0000000000c3")
	otherHub    = shared.ID("01936f2a-7c1e-7000-8000-0000000000c4")
	collectionC = shared.ID("01936f2a-7c1e-7000-8000-0000000000c5")
)

// pulling is the stream's fixture with a pull in front of it, every container readable, and the
// two hubs' collections known to the container store.
func pulling(t *testing.T, entries ...repository.Recorded) (PullChanges, fixture) {
	t.Helper()
	f := streaming(t, entries...)
	for _, id := range []shared.ID{hub, collectionA, collectionB, otherHub, collectionC} {
		f.auth.allowed[id] = true
	}
	f.containers.parents[collectionA] = hub
	f.containers.parents[collectionB] = hub
	f.containers.parents[collectionC] = otherHub
	return PullChanges{Stream: f.stream}, f
}

// cursorAt spells a cursor for a position minted now, the way the fixture's codec spells it.
func cursorAt(f fixture, seq int64) string {
	return f.stream.Encode(Position{Seq: seq, IssuedAt: now})
}

func request(f fixture, seq int64, limit int, scopes ...Scope) PullRequest {
	return PullRequest{DeviceID: device, Cursor: cursorAt(f, seq), Limit: limit, Scopes: scopes}
}

func seqs(records []Record) []int64 {
	out := make([]int64, 0, len(records))
	for _, record := range records {
		out = append(out, record.Seq)
	}
	return out
}

func sameSeqs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAPullAnswersPagesInCursorOrderAndSaysWhetherThereIsMore(t *testing.T) {
	pull, f := pulling(t,
		entry(1, readable), entry(2, readable), entry(3, readable),
		entry(4, readable), entry(5, readable))

	first, err := pull.Pull(t.Context(), actor(), request(f, 0, 2))
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}
	if !sameSeqs(seqs(first.Records), []int64{1, 2}) || !first.More {
		t.Fatalf("first page %v more=%v, want [1 2] with more", seqs(first.Records), first.More)
	}

	// The cursor the page hands back is what the next request sends: no gap, no duplicate.
	second, err := pull.Pull(t.Context(), actor(), PullRequest{
		DeviceID: device, Cursor: f.stream.Encode(first.Cursor), Limit: 2,
	})
	if err != nil {
		t.Fatalf("pulling the second page: %v", err)
	}
	if !sameSeqs(seqs(second.Records), []int64{3, 4}) || !second.More {
		t.Fatalf("second page %v more=%v, want [3 4] with more", seqs(second.Records), second.More)
	}

	last, err := pull.Pull(t.Context(), actor(), PullRequest{
		DeviceID: device, Cursor: f.stream.Encode(second.Cursor), Limit: 2,
	})
	if err != nil {
		t.Fatalf("pulling the last page: %v", err)
	}
	if !sameSeqs(seqs(last.Records), []int64{5}) || last.More {
		t.Errorf("last page %v more=%v, want [5] and no more", seqs(last.Records), last.More)
	}
	if last.Cursor.Seq != 5 {
		t.Errorf("the walk stands at %d, want the last entry read", last.Cursor.Seq)
	}
}

// The C-10 criterion, again: a record the caller may not read is not in the page, and the cursor
// advances past it rather than stalling on it.
func TestAPullWithholdsWhatTheCallerMayNotReadAndAdvancesPastIt(t *testing.T) {
	pull, f := pulling(t, entry(1, readable), entry(2, hidden), entry(3, readable))

	page, err := pull.Pull(t.Context(), actor(), request(f, 0, 3))
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}
	if !sameSeqs(seqs(page.Records), []int64{1, 3}) {
		t.Errorf("page %v, want the two readable records", seqs(page.Records))
	}
	if page.Cursor.Seq != 3 {
		t.Errorf("the cursor stands at %d, want 3 - past the withheld record", page.Cursor.Seq)
	}
	// A full batch was read, so there may be more - even though fewer records were sent.
	if !page.More {
		t.Errorf("a full batch with one record withheld says there is no more")
	}
}

// The five entries every depth question is asked over: the hub's own record, one in each of its
// collections, the other hub's, and one in the other hub's collection.
func depthEntries() []repository.Recorded {
	return []repository.Recorded{
		entry(1, hub), entry(2, collectionA), entry(3, collectionB),
		entry(4, otherHub), entry(5, collectionC),
	}
}

func TestEachDepthOfAScopeIsATableTest(t *testing.T) {
	cases := map[string]struct {
		scopes []Scope
		want   []int64
	}{
		"no scope is everything the caller may read": {
			want: []int64{1, 2, 3, 4, 5},
		},
		"SELF on a hub is the hub's own record": {
			scopes: []Scope{{ContainerID: hub, Depth: DepthSelf}}, want: []int64{1},
		},
		"CHILDREN on a hub adds its collections": {
			scopes: []Scope{{ContainerID: hub, Depth: DepthChildren}}, want: []int64{1, 2, 3},
		},
		"SUBTREE on a hub is every level below": {
			scopes: []Scope{{ContainerID: hub, Depth: DepthSubtree}}, want: []int64{1, 2, 3},
		},
		"SELF on a collection is what it holds": {
			scopes: []Scope{{ContainerID: collectionA, Depth: DepthSelf}}, want: []int64{2},
		},
		"SUBTREE on a collection is the same, having nothing below": {
			scopes: []Scope{{ContainerID: collectionA, Depth: DepthSubtree}}, want: []int64{2},
		},
		"the depth left empty is the contract's default, SUBTREE": {
			scopes: []Scope{{ContainerID: hub}}, want: []int64{1, 2, 3},
		},
		"two scopes are a union": {
			scopes: []Scope{
				{ContainerID: collectionA, Depth: DepthSelf},
				{ContainerID: collectionC, Depth: DepthSelf},
			},
			want: []int64{2, 5},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			pull, f := pulling(t, depthEntries()...)
			page, err := pull.Pull(t.Context(), actor(), request(f, 0, 10, tc.scopes...))
			if err != nil {
				t.Fatalf("pulling: %v", err)
			}
			if got := seqs(page.Records); !sameSeqs(got, tc.want) {
				t.Errorf("page %v, want %v", got, tc.want)
			}
		})
	}
}

// The acceptance criterion: a pull that names no scope and one that names the hub answer the same
// records for an entry under that hub.
func TestAnEntryUnderTheHubIsAnsweredWithAndWithoutTheScope(t *testing.T) {
	pull, f := pulling(t, entry(1, collectionA))

	unscoped, err := pull.Pull(t.Context(), actor(), request(f, 0, 10))
	if err != nil {
		t.Fatalf("pulling unscoped: %v", err)
	}
	scoped, err := pull.Pull(t.Context(), actor(),
		request(f, 0, 10, Scope{ContainerID: hub, Depth: DepthSubtree}))
	if err != nil {
		t.Fatalf("pulling scoped: %v", err)
	}
	if !sameSeqs(seqs(unscoped.Records), []int64{1}) || !sameSeqs(seqs(scoped.Records), []int64{1}) {
		t.Errorf("unscoped %v, scoped %v, want the entry in both", seqs(unscoped.Records), seqs(scoped.Records))
	}
}

// A scope narrows after the permission check, never instead of it: naming the hub does not hand a
// device a collection under it that the caller may not read.
func TestAScopeNarrowsAfterThePermissionCheckNotInsteadOfIt(t *testing.T) {
	pull, f := pulling(t, depthEntries()...)
	f.auth.allowed[collectionB] = false

	page, err := pull.Pull(t.Context(), actor(),
		request(f, 0, 10, Scope{ContainerID: hub, Depth: DepthSubtree}))
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}
	if got := seqs(page.Records); !sameSeqs(got, []int64{1, 2}) {
		t.Errorf("page %v, want the hub and its readable collection only", got)
	}
}

// A workspace-wide template (#626) reaches a device through the pull, and under a scope too: it
// stands above every hub, so a device holding one hub still receives the templates it can apply.
func TestAWorkspaceWideTemplateIsAnsweredWithAndWithoutAScope(t *testing.T) {
	template := entry(1, "")
	template.Entity = "template"
	pull, f := pulling(t, template)
	f.auth.workspace = true

	unscoped, err := pull.Pull(t.Context(), actor(), request(f, 0, 10))
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}
	scoped, err := pull.Pull(t.Context(), actor(),
		request(f, 0, 10, Scope{ContainerID: hub, Depth: DepthSelf}))
	if err != nil {
		t.Fatalf("pulling scoped: %v", err)
	}
	if !sameSeqs(seqs(unscoped.Records), []int64{1}) || !sameSeqs(seqs(scoped.Records), []int64{1}) {
		t.Errorf("unscoped %v, scoped %v, want the template in both", seqs(unscoped.Records), seqs(scoped.Records))
	}

	f.auth.workspace = false
	page, err := pull.Pull(t.Context(), actor(), request(f, 0, 10))
	if err != nil {
		t.Fatalf("pulling as a stranger to the workspace: %v", err)
	}
	if len(page.Records) != 0 {
		t.Errorf("the template was sent to somebody who may not read the workspace: %v", seqs(page.Records))
	}
}

func TestAPullValidatesItsRequest(t *testing.T) {
	cases := map[string]struct {
		request PullRequest
		code    string
	}{
		"a device is required": {
			request: PullRequest{Cursor: "1@0"}, code: "sync.device_required",
		},
		"a negative limit is out of range": {
			request: PullRequest{DeviceID: device, Cursor: "1@0", Limit: -1},
			code:    "sync.limit_out_of_range",
		},
		"a limit over the maximum is refused rather than clamped": {
			request: PullRequest{DeviceID: device, Cursor: "1@0", Limit: PullLimitMax + 1},
			code:    "sync.limit_out_of_range",
		},
		"a scope needs its container": {
			request: PullRequest{DeviceID: device, Cursor: "1@0", Scopes: []Scope{{Depth: DepthSelf}}},
			code:    "sync.scope_container_required",
		},
		"a depth the contract does not name is refused": {
			request: PullRequest{
				DeviceID: device, Cursor: "1@0",
				Scopes: []Scope{{ContainerID: hub, Depth: "EVERYTHING"}},
			},
			code: "sync.scope_depth_unknown",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			pull, _ := pulling(t)
			_, err := pull.Pull(t.Context(), actor(), tc.request)
			if err == nil {
				t.Fatalf("the request was accepted")
			}
			if got := shared.AsError(err).DetailCode; got != tc.code {
				t.Errorf("refused with %q, want %q", got, tc.code)
			}
		})
	}
}

// Until N-02, a device with no cursor is told the initial synchronisation is not served rather
// than handed an empty page and a fresh cursor - which would be a client believing it is current.
func TestANullCursorIsRefusedUntilTheInitialSynchronisationExists(t *testing.T) {
	pull, _ := pulling(t, entry(1, readable))

	_, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device})
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("got %v, want the unavailable error", err)
	}
	if got := shared.AsError(err).DetailCode; got != "sync.initial_sync_unavailable" {
		t.Errorf("refused with %q", got)
	}
}

// The stream's two cursor refusals, through the pull: not copied, the same code path.
func TestAPullRefusesTheSameCursorsTheStreamRefuses(t *testing.T) {
	pull, f := pulling(t, entry(1, readable))

	stale := f.stream.Encode(Position{Seq: 0, IssuedAt: now.Add(-window - time.Hour)})
	_, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: stale})
	if got := shared.AsError(err).DetailCode; got != "sync.cursor_too_old" {
		t.Errorf("a cursor past the window was refused with %q", got)
	}

	_, err = pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: "not a cursor"})
	if got := shared.AsError(err).DetailCode; got != "sync.cursor_invalid" {
		t.Errorf("a forged cursor was refused with %q", got)
	}
}

func TestAPageCarriesTheServersClockAndTheWindow(t *testing.T) {
	pull, f := pulling(t, entry(1, readable))

	page, err := pull.Pull(t.Context(), actor(), request(f, 0, 0))
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}
	if !page.ServerTime.Equal(now) {
		t.Errorf("server time %v, want the clock's %v", page.ServerTime, now)
	}
	if page.Window != window {
		t.Errorf("window %v, want %v", page.Window, window)
	}
}

func TestATokenWithoutTheReadScopeGetsNoPage(t *testing.T) {
	pull, f := pulling(t, entry(1, readable))
	limited := actor()
	limited.Scopes = []string{"items:write"}

	_, err := pull.Pull(t.Context(), limited, request(f, 0, 10))
	if err == nil {
		t.Fatalf("a token without items:read was handed a page")
	}
}
