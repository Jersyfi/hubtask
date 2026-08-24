// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The read half of the change log (C-10): the walk the stream and `:pull` share, and a cross-tenant
// negative for each of its methods (gate SG-3).

// recordChange writes one entry for the tenant and returns the container it names.
func recordChange(
	ctx context.Context, t *testing.T, tenant, actor, container shared.ID, entity string,
) shared.ID {
	t.Helper()

	reading, err := shared.HLC{}.Tick(created, "server-1")
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}
	entityID := freshID(t)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenant, Entity: entity, EntityID: entityID,
			Op: changelog.Upsert, ContainerID: container, ActorID: actor,
			HLC: reading, Payload: map[string]any{"title": "Review the quote"},
		})
	}); err != nil {
		t.Fatalf("recording the change: %v", err)
	}
	return entityID
}

func readChanges(
	ctx context.Context, t *testing.T, tenant shared.ID, after int64, batch int,
) []changelog.Recorded {
	t.Helper()

	var entries []changelog.Recorded
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		entries, err = postgres.NewChangeLog().After(ctx, after, batch)
		return err
	}); err != nil {
		t.Fatalf("reading the change log: %v", err)
	}
	return entries
}

func latestSeq(ctx context.Context, t *testing.T, tenant shared.ID) int64 {
	t.Helper()

	var latest int64
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		latest, err = postgres.NewChangeLog().Latest(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the head of the change log: %v", err)
	}
	return latest
}

func TestTheChangeLogIsWalkedInCursorOrder(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	start := latestSeq(ctx, t, tenantA)
	container := freshID(t)
	first := recordChange(ctx, t, tenantA, authorA, container, "work_item")
	second := recordChange(ctx, t, tenantA, authorA, container, "work_item")

	entries := readChanges(ctx, t, tenantA, start, 10)
	if len(entries) != 2 {
		t.Fatalf("%d entries after the cursor, want the two just written", len(entries))
	}
	if entries[0].EntityID != first || entries[1].EntityID != second {
		t.Errorf("the walk is out of order: %v, %v", entries[0].EntityID, entries[1].EntityID)
	}
	if entries[0].Seq >= entries[1].Seq {
		t.Errorf("the sequence did not advance: %d then %d", entries[0].Seq, entries[1].Seq)
	}
	if entries[0].Seq <= start {
		t.Errorf("an entry at %d came back for a cursor of %d", entries[0].Seq, start)
	}

	// Everything the writer recorded comes back out, parsed rather than as text.
	entry := entries[0]
	if entry.Entity != "work_item" || entry.Op != changelog.Upsert {
		t.Errorf("entry %+v", entry)
	}
	if entry.ContainerID != container || entry.ActorID != authorA {
		t.Errorf("the references did not survive: %+v", entry)
	}
	if entry.HLC.IsZero() {
		t.Error("the clock did not come back, and every merge needs it")
	}
	if entry.Payload["title"] != "Review the quote" {
		t.Errorf("payload %v", entry.Payload)
	}
	if entry.OccurredAt.IsZero() {
		t.Error("the entry has no time")
	}

	// Resuming from the last cursor returns nothing: no gap, and no duplicate.
	if rest := readChanges(ctx, t, tenantA, entries[1].Seq, 10); len(rest) != 0 {
		t.Errorf("%d entries after the last cursor, want none", len(rest))
	}
}

// A batch is a batch, and the cursor of its last entry is what continues the walk. There is no
// `has_more` to compute: a short page is the end of the log.
func TestTheWalkIsBatchedAndResumesWithoutAGap(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	start := latestSeq(ctx, t, tenantA)
	container := freshID(t)
	written := make([]shared.ID, 0, 5)
	for range 5 {
		written = append(written, recordChange(ctx, t, tenantA, authorA, container, "work_item"))
	}

	var seen []shared.ID
	cursor := start
	for range 10 {
		batch := readChanges(ctx, t, tenantA, cursor, 2)
		if len(batch) == 0 {
			break
		}
		for _, entry := range batch {
			seen = append(seen, entry.EntityID)
		}
		cursor = batch[len(batch)-1].Seq
	}

	if len(seen) != len(written) {
		t.Fatalf("the walk saw %d entries, want %d", len(seen), len(written))
	}
	for i, id := range written {
		if seen[i] != id {
			t.Errorf("entry %d is %s, want %s", i, seen[i], id)
		}
	}
}

// A deletion carries no payload by design, and it comes back as nothing rather than as an empty
// change set - which is a different statement.
func TestADeletionComesBackWithoutAPayload(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	start := latestSeq(ctx, t, tenantA)
	reading, err := shared.HLC{}.Tick(created, "server-1")
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenantA, Entity: "work_item", EntityID: freshID(t),
			Op: changelog.Delete, ContainerID: freshID(t), ActorID: authorA, HLC: reading,
		})
	}); err != nil {
		t.Fatalf("recording the deletion: %v", err)
	}

	entries := readChanges(ctx, t, tenantA, start, 10)
	if len(entries) != 1 {
		t.Fatalf("%d entries, want the deletion", len(entries))
	}
	if entries[0].Op != changelog.Delete {
		t.Errorf("op %q", entries[0].Op)
	}
	if entries[0].Payload != nil {
		t.Errorf("the deletion carries a payload: %v", entries[0].Payload)
	}
}

// Gate SG-3, and the acceptance criterion's first half: a client sees only its own tenant's
// records. Row level security narrows the walk rather than failing it, which is the stronger
// property - a query that errored would still have had to be trusted not to match.
func TestTheChangeWalkSeesOnlyItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	startA := latestSeq(ctx, t, tenantA)
	startB := latestSeq(ctx, t, tenantB)

	inA := recordChange(ctx, t, tenantA, authorA, freshID(t), "work_item")
	inB := recordChange(ctx, t, tenantB, authorB, freshID(t), "work_item")

	for _, tc := range []struct {
		name    string
		tenant  shared.ID
		from    int64
		wanted  shared.ID
		refused shared.ID
	}{
		{"tenant A", tenantA, startA, inA, inB},
		{"tenant B", tenantB, startB, inB, inA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, entry := range readChanges(ctx, t, tc.tenant, tc.from, 100) {
				if entry.EntityID == tc.refused {
					t.Errorf("the walk returned another tenant's entry %s", entry.EntityID)
				}
			}

			// And its own is there, so the test is about the boundary rather than about an empty
			// answer.
			var found bool
			for _, entry := range readChanges(ctx, t, tc.tenant, tc.from, 100) {
				if entry.EntityID == tc.wanted {
					found = true
				}
			}
			if !found {
				t.Errorf("the walk did not return this tenant's own entry %s", tc.wanted)
			}
		})
	}

	// Latest is the head of *this* tenant's log. A head that counted everybody's writes would hand
	// a new client a cursor that skips its own first change.
	if latest := latestSeq(ctx, t, tenantA); latest <= startA {
		t.Errorf("the head of tenant A's log did not move: %d", latest)
	}
	headB := latestSeq(ctx, t, tenantB)
	entriesB := readChanges(ctx, t, tenantB, startB, 100)
	if len(entriesB) == 0 || entriesB[len(entriesB)-1].Seq != headB {
		t.Errorf("the head %d is not the last entry tenant B can see", headB)
	}
}

// A workspace nothing has happened in has a head of zero rather than an error, which is what lets a
// client open a stream on a new workspace and be told about its first change.
func TestAnUntouchedWorkspaceHasAHeadOfZero(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	fresh := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Fresh')`,
		fresh.String(), "tenant-"+fresh.String()[:8]); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}

	if latest := latestSeq(ctx, t, fresh); latest != 0 {
		t.Errorf("the head of an untouched workspace is %d, want 0", latest)
	}
	if entries := readChanges(ctx, t, fresh, 0, 10); len(entries) != 0 {
		t.Errorf("%d entries in an untouched workspace", len(entries))
	}
}

// The wake-up of ADR-0007, end to end: a change recorded through the repository rings the doorbell
// on a listener that is watching that workspace, and rings nobody else's.
//
// Here rather than in the adapter's own tests because what is under test is the trigger: the
// fan-out is proved without a database, and whether a `NOTIFY` reaches a `LISTEN` is a property of
// the schema.
func TestRecordingAChangeWakesTheListener(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	listener := postgres.NewChangeListener(appPool(ctx, t))
	listening, stopListening := context.WithCancel(ctx)
	defer stopListening()
	concurrency.Go(listening, "test.change_listener", listener.Run)

	waitFor(t, 5*time.Second, "the listener to connect", listener.Connected)

	inA, stopA := listener.Subscribe(tenantA)
	defer stopA()
	inB, stopB := listener.Subscribe(tenantB)
	defer stopB()

	recordChange(ctx, t, tenantA, authorA, freshID(t), "work_item")

	select {
	case <-inA:
	case <-time.After(5 * time.Second):
		t.Fatal("a recorded change did not wake the workspace it belongs to")
	}

	// And nobody else's. A doorbell that rang for every workspace would have every stream on the
	// process reading the log on every write in the installation.
	select {
	case <-inB:
		t.Error("a change in one workspace woke another")
	case <-time.After(200 * time.Millisecond):
	}
}

// The notification is a doorbell, not a letter: the payload is the tenant and nothing else, so no
// user content travels through a channel with no tenant boundary (rule 10).
func TestTheNotificationCarriesOnlyTheWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	conn, err := appPool(ctx, t).Acquire(ctx)
	if err != nil {
		t.Fatalf("acquiring a connection: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+postgres.ChangeChannel); err != nil {
		t.Fatalf("listening: %v", err)
	}

	recordChange(ctx, t, tenantA, authorA, freshID(t), "work_item")

	waiting, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	notification, err := conn.Conn().WaitForNotification(waiting)
	if err != nil {
		t.Fatalf("waiting for the notification: %v", err)
	}

	if notification.Payload != tenantA.String() {
		t.Errorf("the payload is %q, want the workspace identifier alone", notification.Payload)
	}
}

// The acceptance of C-10, against a real database and the real service: a client sees only its own
// tenant's records, never a record for a container it may not read, and `Last-Event-ID` resumes
// with no gap and no duplicate.

// streamFor builds the service the way the composition root builds it.
func streamFor(ctx context.Context, t *testing.T) syncservice.StreamChanges {
	t.Helper()

	return syncservice.StreamChanges{
		Changes:    postgres.NewChangeLog(),
		Containers: containerRepo(),
		Authorizer: access.Service{
			Memberships: postgres.NewMembershipRepository(),
			UnitOfWork:  postgres.NewUnitOfWork(appPool(ctx, t)),
			Audit:       postgres.NewAuditSink(clockadapter.NewUUIDv7(clockadapter.System{})),
			Clock:       clockadapter.System{},
		},
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Cursors:    streamCursors{codec: security.NewStreamCursorCodec(secret.New("integration test installation secret"))},
		Clock:      clockadapter.System{},
		Window:     90 * 24 * time.Hour,
		Batch:      50,
	}
}

// streamCursors is the composition root's two-line bridge, repeated here for the same reason it
// exists there: the application says `sync.Position` and the codec says `security.StreamPosition`.
type streamCursors struct{ codec security.StreamCursorCodec }

func (c streamCursors) Encode(position syncservice.Position) string {
	return c.codec.Encode(security.StreamPosition{Seq: position.Seq, IssuedAt: position.IssuedAt})
}

func (c streamCursors) Decode(cursor string) (syncservice.Position, error) {
	decoded, err := c.codec.Decode(cursor)
	if err != nil {
		return syncservice.Position{}, err
	}
	return syncservice.Position{Seq: decoded.Seq, IssuedAt: decoded.IssuedAt}, nil
}

func streamActor(tenant, account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: account, AccountName: "Anna", Kind: shared.ActorUser,
		Scopes: []string{"items:read"},
	}
}

// hubFor creates a hub as a container and gives one account a role on it. The membership fixtures
// of this package seed roles without containers, which is enough for the membership tests and not
// enough here: the stream resolves a container before it asks about it, and a change naming a hub
// that does not exist is withheld for that reason rather than for a permission.
func hubWithRole(ctx context.Context, t *testing.T, tenant, account shared.ID, role string) shared.ID {
	t.Helper()

	hub := freshID(t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		 VALUES ($1, $2, 'HUB', $3, 'a0', $4)`,
		hub.String(), tenant.String(), freshName(t), account.String()); err != nil {
		t.Fatalf("seeding the hub: %v", err)
	}
	if role == "" {
		return hub
	}
	if _, err := admin.Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		 VALUES ($1, $2, $3, 'HUB', $4, $5)`,
		freshID(t).String(), tenant.String(), account.String(), hub.String(), role); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}
	return hub
}

// A member of one hub sees the changes below it and nothing else - not the other hub in the same
// workspace, and not the other workspace at all. Judged per record in the application layer, never
// by trusting what the connection asked for.
func TestTheStreamCarriesOnlyWhatTheCallerMayRead(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	// Somebody with a role on exactly one hub. Deliberately not the package's Anna, who is a
	// tenant administrator and would therefore see everything - which would make the test pass for
	// the wrong reason.
	member := seedAccount(ctx, t, tenantA)
	permitted := hubWithRole(ctx, t, tenantA, member, "MEMBER")
	forbidden := hubWithRole(ctx, t, tenantA, member, "")

	stream := streamFor(ctx, t)
	actor := streamActor(tenantA, member)

	from, err := stream.Resume(ctx, actor, "")
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}

	visible := recordChange(ctx, t, tenantA, member, permitted, "work_item")
	invisible := recordChange(ctx, t, tenantA, member, forbidden, "work_item")
	elsewhere := recordChange(ctx, t, tenantB, authorB, freshID(t), "work_item")

	batch, err := stream.Next(ctx, actor, from)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	seen := map[shared.ID]bool{}
	for _, record := range batch.Records {
		seen[record.EntityID] = true
	}
	if !seen[visible] {
		t.Error("the caller was not sent a change in the hub they hold a role on")
	}
	if seen[invisible] {
		t.Error("the caller was sent a change in a hub they hold nothing on")
	}
	if seen[elsewhere] {
		t.Error("the caller was sent another tenant's change")
	}

	// The cursor still advanced past what was withheld: one that stalled on a container somebody
	// may not read would re-read it forever.
	if batch.Cursor.Seq <= from.Seq {
		t.Error("the cursor did not advance past records the caller may not see")
	}

	// Losing the role stops the records, without the connection being told anything: the judgement
	// is made at the moment the record is read, not when the stream was opened.
	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM membership WHERE tenant_id = $1 AND account_id = $2 AND scope_id = $3`,
		tenantA.String(), member.String(), permitted.String()); err != nil {
		t.Fatalf("revoking the membership: %v", err)
	}
	after := recordChange(ctx, t, tenantA, member, permitted, "work_item")

	revoked, err := stream.Next(ctx, actor, batch.Cursor)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	for _, record := range revoked.Records {
		if record.EntityID == after {
			t.Error("a record arrived for a container the caller had just lost access to")
		}
	}
}

// `Last-Event-ID` resumes with no gap and no duplicate — the criterion, through the real cursor.
func TestLastEventIDResumesWithNoGapAndNoDuplicate(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	member := seedAccount(ctx, t, tenantA)
	hub := hubWithRole(ctx, t, tenantA, member, "MEMBER")
	stream := streamFor(ctx, t)
	actor := streamActor(tenantA, member)

	from, err := stream.Resume(ctx, actor, "")
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}

	written := make([]shared.ID, 0, 6)
	for range 6 {
		written = append(written, recordChange(ctx, t, tenantA, member, hub, "work_item"))
	}

	first, err := stream.Next(ctx, actor, from)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(first.Records) != len(written) {
		t.Fatalf("%d records, want %d", len(first.Records), len(written))
	}

	// The client is cut off after the third record and comes back with that record's cursor -
	// which is what a browser's EventSource does by itself.
	interrupted := stream.Encode(first.Records[2].Cursor)
	resumed, err := stream.Resume(ctx, actor, interrupted)
	if err != nil {
		t.Fatalf("resuming from %q: %v", interrupted, err)
	}

	rest, err := stream.Next(ctx, actor, resumed)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	got := make([]shared.ID, 0, len(rest.Records))
	for _, record := range rest.Records {
		got = append(got, record.EntityID)
	}
	want := written[3:]
	if len(got) != len(want) {
		t.Fatalf("resumed with %d records, want %d - a gap or a duplicate", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d is %s, want %s", i, got[i], want[i])
		}
	}
}

// A cursor older than the offline window is the one answer that is safe: the log no longer holds
// everything that happened, so a delta across the gap would silently omit whatever was pruned.
func TestACursorOlderThanTheWindowIsRefusedEndToEnd(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	stream := streamFor(ctx, t)
	actor := streamActor(tenantA, authorA)

	stale := stream.Encode(syncservice.Position{
		Seq: 1, IssuedAt: time.Now().Add(-91 * 24 * time.Hour),
	})

	_, err := stream.Resume(ctx, actor, stale)
	if err == nil {
		t.Fatal("a cursor older than the window was accepted")
	}
	if !errors.Is(err, shared.ErrGone) {
		t.Errorf("a stale cursor reported %v, want gone", err)
	}
	if got := shared.AsError(err).DetailCode; got != "sync.cursor_too_old" {
		t.Errorf("detail %q", got)
	}
}
