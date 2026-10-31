// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000a001")
	account  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000a002")
	document = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000a003")
	now      = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
)

type passthrough struct{}

func (passthrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passthrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return now }

// memory is the store in memory: one document's log and its snapshots.
type memory struct {
	updates   []domain.Update
	snapshots []domain.Snapshot
	compacted map[int64]bool
}

func (m *memory) Append(_ context.Context, _ domain.Ref, data []byte, author shared.ID, at time.Time) (int64, error) {
	seq := int64(len(m.updates) + 1)
	m.updates = append(m.updates, domain.Update{Seq: seq, Data: data, AuthorID: author, At: at})
	return seq, nil
}

func (m *memory) Head(context.Context, domain.Ref) (domain.Head, error) {
	head := domain.Head{Seq: int64(len(m.updates))}
	if n := len(m.snapshots); n > 0 {
		head.SnapshotSeq = m.snapshots[n-1].UpToSeq
	}
	return head, nil
}

func (m *memory) Snapshot(context.Context, domain.Ref) (domain.Snapshot, bool, error) {
	if len(m.snapshots) == 0 {
		return domain.Snapshot{}, false, nil
	}
	return m.snapshots[len(m.snapshots)-1], true, nil
}

func (m *memory) Since(_ context.Context, _ domain.Ref, after int64, limit int) ([]domain.Update, error) {
	var out []domain.Update
	for _, update := range m.updates {
		if update.Seq > after && len(out) < limit {
			out = append(out, update)
		}
	}
	return out, nil
}

func (m *memory) StoreSnapshot(_ context.Context, _ domain.Ref, snapshot domain.Snapshot) (bool, error) {
	m.snapshots = append(m.snapshots, snapshot)
	if m.compacted == nil {
		m.compacted = map[int64]bool{}
	}
	for seq := int64(1); seq <= snapshot.UpToSeq; seq++ {
		m.compacted[seq] = true
	}
	return true, nil
}

func (m *memory) Erase(context.Context, domain.Ref) error {
	m.updates, m.snapshots = nil, nil
	return nil
}

// gate answers by permission: what the reader holds at the document's place.
type gate struct {
	held     map[service.Permission]bool
	refused  int
	asked    []service.Permission
	silently int
}

func (g *gate) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	g.asked = append(g.asked, request.Permission)
	if !g.held[request.Permission] {
		g.refused++
		return shared.ErrForbidden.WithDetail("authz.forbidden")
	}
	return nil
}

func (g *gate) Permits(_ context.Context, _ appshared.ActorContext, request access.Request) (bool, error) {
	g.silently++
	return g.held[request.Permission], nil
}

type pageLocator struct{ err error }

func (l pageLocator) Locate(context.Context, appshared.ActorContext, shared.ID) (Place, error) {
	if l.err != nil {
		return Place{}, l.err
	}
	return Place{
		Path: []identity.Scope{identity.TenantScope()},
		Read: service.PermissionRead, Write: service.PermissionWriteItems,
	}, nil
}

func documents(held ...service.Permission) (Documents, *memory, *gate) {
	store := &memory{}
	g := &gate{held: map[service.Permission]bool{}}
	for _, p := range held {
		g.held[p] = true
	}
	return Documents{
		Store: store, Kinds: map[domain.Kind]Locator{"page": pageLocator{}},
		Authorizer: g, UnitOfWork: passthrough{}, Clock: fixedClock{},
	}, store, g
}

func reader() appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenant, AccountID: account, Kind: shared.ActorUser,
		Scopes: []string{"items:read", "items:write"}}
}

var page = domain.Ref{Kind: "page", ID: document}

func TestAWriterAppendsInOrderAndAReaderOpensFromTheSnapshot(t *testing.T) {
	docs, store, _ := documents(service.PermissionRead, service.PermissionWriteItems)
	for i := range 4 {
		appended, err := docs.Append(t.Context(), reader(), page, []byte{byte(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		if appended.Seq != int64(i+1) {
			t.Fatalf("update %d landed at %d", i+1, appended.Seq)
		}
	}
	if err := docs.Compact(t.Context(), reader(), page, 3, []byte("merged")); err != nil {
		t.Fatal(err)
	}

	opened, err := docs.Open(t.Context(), reader(), page)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Snapshot == nil || opened.Snapshot.UpToSeq != 3 {
		t.Fatalf("opened from %+v, want the snapshot at 3", opened.Snapshot)
	}
	if len(opened.Updates) != 1 || opened.Updates[0].Seq != 4 {
		t.Errorf("opened with updates %+v, want only the one after the snapshot", opened.Updates)
	}
	if !opened.Writable || opened.Head.Seq != 4 {
		t.Errorf("opened %+v, want writable at head 4", opened)
	}
	if !store.compacted[3] || store.compacted[4] {
		t.Errorf("compacted %v, want the snapshot's updates and no later one", store.compacted)
	}
	if len(store.updates) != 4 {
		t.Errorf("the snapshot removed history: %d updates left, want 4 until retention", len(store.updates))
	}
}

func TestAReaderMayOpenButNotWrite(t *testing.T) {
	docs, store, g := documents(service.PermissionRead)

	opened, err := docs.Open(t.Context(), reader(), page)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Writable {
		t.Error("a reader was offered writing")
	}
	_, err = docs.Append(t.Context(), reader(), page, []byte{1})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a reader's update answered %v, want forbidden", err)
	}
	if err := docs.Compact(t.Context(), reader(), page, 1, []byte{1}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a reader's snapshot answered %v, want forbidden", err)
	}
	if len(store.updates) != 0 || g.refused != 2 {
		t.Errorf("%d updates stored and %d refusals recorded, want none and two", len(store.updates), g.refused)
	}
}

func TestSomebodyWithoutReadIsRefusedAndRecorded(t *testing.T) {
	docs, _, g := documents()

	if _, err := docs.Open(t.Context(), reader(), page); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("open answered %v, want forbidden", err)
	}
	if g.refused != 1 {
		t.Errorf("%d refusals recorded, want one", g.refused)
	}
}

func TestAccessEndedWhileConnectedEndsTheCatchUp(t *testing.T) {
	docs, _, g := documents(service.PermissionRead, service.PermissionWriteItems)
	if _, err := docs.Append(t.Context(), reader(), page, []byte{1}); err != nil {
		t.Fatal(err)
	}
	updates, err := docs.Since(t.Context(), reader(), page, 0)
	if err != nil || len(updates) != 1 {
		t.Fatalf("catching up answered %v, %v", updates, err)
	}

	delete(g.held, service.PermissionRead)
	if _, err := docs.Since(t.Context(), reader(), page, 0); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("catching up after access ended answered %v, want forbidden", err)
	}
}

func TestAnUnknownKindIsNotFound(t *testing.T) {
	docs, _, _ := documents(service.PermissionRead)
	_, err := docs.Open(t.Context(), reader(), domain.Ref{Kind: "whiteboard", ID: document})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an unregistered kind answered %v, want not found", err)
	}
}

func TestASnapshotPastTheHeadOrBehindTheLastIsRefused(t *testing.T) {
	docs, _, _ := documents(service.PermissionRead, service.PermissionWriteItems)
	for range 3 {
		if _, err := docs.Append(t.Context(), reader(), page, []byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := docs.Compact(t.Context(), reader(), page, 4, []byte{1}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a snapshot past the head answered %v", err)
	}
	if err := docs.Compact(t.Context(), reader(), page, 2, []byte{1}); err != nil {
		t.Fatal(err)
	}
	if err := docs.Compact(t.Context(), reader(), page, 2, []byte{1}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a second snapshot at the same place answered %v, want a conflict", err)
	}
}

func TestAnOversizedUpdateIsRefusedBeforeAnythingIsAsked(t *testing.T) {
	docs, _, g := documents(service.PermissionRead, service.PermissionWriteItems)
	_, err := docs.Append(t.Context(), reader(), page, make([]byte, domain.MaxUpdateBytes+1))
	if !errors.Is(err, shared.ErrValidation) || len(g.asked) != 0 {
		t.Errorf("an oversized update answered %v after %d questions", err, len(g.asked))
	}
}

func TestTheWriterIsToldWhenCompactionIsDue(t *testing.T) {
	docs, _, _ := documents(service.PermissionRead, service.PermissionWriteItems)
	var last Appended
	for range domain.CompactAfter {
		var err error
		if last, err = docs.Append(t.Context(), reader(), page, []byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	if !last.CompactionDue {
		t.Error("the writer was not asked for a snapshot after the bound")
	}
}

// presenceLocator places an entry's presence: read as the entry, never written.
type presenceLocator struct{ on access.ItemSubject }

func (l presenceLocator) Locate(context.Context, appshared.ActorContext, shared.ID) (Place, error) {
	return Place{
		Path: []identity.Scope{identity.TenantScope()}, Read: service.PermissionRead,
		PresenceOnly: true, On: l.on,
	}, nil
}

// recordingGate keeps the requests it was asked, to see what they name.
type recordingGate struct {
	gate
	requests []access.Request
}

func (g *recordingGate) Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error {
	g.requests = append(g.requests, request)
	return g.gate.Authorize(ctx, actor, request)
}

func TestAPresenceOnlyDocumentIsOpenedButNeverWritten(t *testing.T) {
	entry := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000a004")
	subject := access.ItemSubject{Does: service.ItemRead, ID: entry}
	store := &memory{}
	g := &recordingGate{gate: gate{held: map[service.Permission]bool{
		service.PermissionRead: true, service.PermissionWriteItems: true,
	}}}
	docs := Documents{
		Store: store, Kinds: map[domain.Kind]Locator{"entry": presenceLocator{on: subject}},
		Authorizer: g, UnitOfWork: passthrough{}, Clock: fixedClock{},
	}
	ref := domain.Ref{Kind: "entry", ID: entry}

	opened, err := docs.Open(t.Context(), reader(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Writable {
		t.Error("a presence-only document was offered for writing")
	}
	if len(g.requests) != 1 || g.requests[0].On != subject {
		t.Errorf("the opening asked %+v, want one request naming the entry", g.requests)
	}
	if _, err := docs.Append(t.Context(), reader(), ref, []byte{1}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("an update to a presence-only document answered %v, want forbidden", err)
	}
	if err := docs.Compact(t.Context(), reader(), ref, 1, []byte{1}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a snapshot of a presence-only document answered %v, want forbidden", err)
	}
	if len(store.updates) != 0 || len(store.snapshots) != 0 {
		t.Errorf("stored %d updates and %d snapshots, want none", len(store.updates), len(store.snapshots))
	}
}
