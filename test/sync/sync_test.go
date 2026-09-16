// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

// Package sync is the evidence table of offline-sync.md §11 as a test package (N-14): one test
// per row, SY-1 to SY-12, against a real PostgreSQL.
//
// Ten of the rows arrived as integration tests with the task that built what they prove, and
// they stay where the fixtures they lean on live - the shared workspace of test/integration, its
// catalogue of use cases, its push. This package names each of them by file and function and
// fails when one goes missing, so that the table stays a table of tests rather than of intentions.
// The two that need a walk of their own rather than a use case - SY-4, a thousand reorderings
// converging without renumbering, and SY-11, the cross-tenant pull - are written here, over a
// workspace of this package's own.
package sync

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

var (
	installationSecret = secret.New("sync evidence installation secret")
	ids                = clockadapter.NewUUIDv7(clockadapter.System{})
	now                = time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
)

func freshID(t *testing.T) shared.ID {
	t.Helper()
	return ids.NewID()
}

// workspace is one tenant of this package's own: an owner, a hub and a collection.
type workspace struct {
	tenant, owner, hub, collection shared.ID
}

type suite struct {
	ctx   context.Context
	admin *pgxpool.Pool
	uow   persistence.UnitOfWork
}

func newSuite(t *testing.T) *suite {
	t.Helper()
	ctx := context.Background()
	return &suite{ctx: ctx, admin: dbtest.AdminPool(ctx, t), uow: postgres.NewUnitOfWork(dbtest.AppPool(ctx, t))}
}

func (s *suite) workspace(t *testing.T, name string) workspace {
	t.Helper()
	w := workspace{tenant: freshID(t), owner: freshID(t), hub: freshID(t), collection: freshID(t)}
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, $3)`,
			[]any{w.tenant.String(), "sync-" + w.tenant.String()[24:], name}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Anna')`,
			[]any{w.owner.String(), w.tenant.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
			[]any{freshID(t).String(), w.tenant.String(), w.owner.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', NULL, $3, 'a0', $5), ($4, $2, 'COLLECTION', $1, 'Errands', 'a0', $5)`,
			[]any{w.hub.String(), w.tenant.String(), name + " hub", w.collection.String(), w.owner.String()}},
	} {
		if _, err := s.admin.Exec(s.ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}
	return w
}

// item seeds one live entry at the rank given.
func (s *suite) item(t *testing.T, w workspace, orderKey string) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by)
		VALUES ($1, $2, $3, 'TASK', $4, 1, $5, $6, $7)`,
		id.String(), w.tenant.String(), w.collection.String(), domain.RootPath(id),
		"Entry "+orderKey, orderKey, w.owner.String()); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	return id
}

func (s *suite) actor(w workspace) appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: w.tenant, AccountID: w.owner, AccountName: "Anna", Kind: shared.ActorUser,
		Scopes: []string{"items:read", "items:write"},
	}
}

func (s *suite) authorizer(t *testing.T) access.Service {
	t.Helper()
	return access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: s.uow,
		Audit: postgres.NewAuditSink(ids), Clock: clock.Fixed(now),
	}
}

// reorder is the use case as the server wires it, over the real repositories.
func (s *suite) reorder(t *testing.T) work.ReorderWorkItem {
	t.Helper()
	fixed := clock.Fixed(now)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-sync-evidence")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	cursors := security.NewCursorCodec(installationSecret)
	return work.ReorderWorkItem{Placement: work.PlacementWriter{
		Items: postgres.NewItemRepository(cursors), Buckets: postgres.NewBucketRepository(),
		Containers: postgres.NewContainerRepository(cursors), Profiles: postgres.NewCapabilityProfileRepository(),
		Authorizer: s.authorizer(t), Events: postgres.NewOutbox(noJobs{}), Changes: postgres.NewChangeLog(),
		Audit: postgres.NewAuditSink(ids), Activity: work.ActivityJournal{Entries: postgres.NewActivityRepository(cursors), IDs: ids},
		UnitOfWork: s.uow, Clock: fixed, IDs: ids, HLC: hybrid,
	}}
}

// noJobs is a queue nothing is asked of: a reorder enqueues no job, and the outbox's wake-up is
// not what these rows prove.
type noJobs struct{}

func (noJobs) Enqueue(context.Context, queue.Request) (shared.ID, error) { return "", nil }
func (noJobs) Claim(context.Context, queue.Lease) ([]queue.Job, error)   { return nil, nil }
func (noJobs) Hold(context.Context, queue.Job) error                     { return nil }
func (noJobs) Complete(context.Context, queue.Job) error                 { return nil }
func (noJobs) Repeat(context.Context, queue.Job, time.Time) error        { return nil }
func (noJobs) Fail(context.Context, queue.Failure) error                 { return nil }
func (noJobs) Depth(context.Context) ([]queue.Depth, error)              { return nil, nil }

// pull is the pull as the server wires it, over this package's cursors.
func (s *suite) pull(t *testing.T) syncservice.PullChanges {
	t.Helper()
	return syncservice.PullChanges{
		Stream: syncservice.StreamChanges{
			Changes: postgres.NewChangeLog(), Containers: postgres.NewContainerRepository(security.NewCursorCodec(installationSecret)),
			Authorizer: s.authorizer(t), UnitOfWork: s.uow,
			Cursors: streamCursors{codec: security.NewStreamCursorCodec(installationSecret)},
			Epochs:  postgres.NewEpochRepository(),
			Clock:   clockadapter.System{}, Window: 90 * 24 * time.Hour, Batch: 50,
		},
		Snapshot: postgres.NewSnapshotRepository(),
	}
}

type streamCursors struct{ codec security.StreamCursorCodec }

func (c streamCursors) Encode(position syncservice.Position) string {
	return c.codec.Encode(security.StreamPosition{
		Seq: position.Seq, IssuedAt: position.IssuedAt, Epoch: position.Epoch, Kind: position.Kind, After: position.After,
	})
}

func (c streamCursors) Decode(cursor string) (syncservice.Position, error) {
	decoded, err := c.codec.Decode(cursor)
	if err != nil {
		return syncservice.Position{}, err
	}
	return syncservice.Position{
		Seq: decoded.Seq, IssuedAt: decoded.IssuedAt, Epoch: decoded.Epoch, Kind: decoded.Kind, After: decoded.After,
	}, nil
}

func (s *suite) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.admin.QueryRow(s.ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return n
}

// SY-4: a thousand reorderings, each landing between two neighbours as a client computes its
// rank (offline-sync.md §4.2, fractional index), produce an order that stays strictly increasing
// and unique, and no reordering rewrites any key but the moved entry's own - which is what "no
// renumbering" means, and what lets two devices reorder offline without either's keys being
// invalidated by the other's.
func TestSY4AThousandReorderingsConvergeWithoutRenumbering(t *testing.T) {
	s := newSuite(t)
	w := s.workspace(t, "SY-4")
	const entries = 25
	keys := make([]string, 0, entries)
	items := make([]shared.ID, 0, entries)
	previous := ""
	for range entries {
		key, err := service.OrderKeyAfter(previous)
		if err != nil {
			t.Fatalf("seeding a rank: %v", err)
		}
		keys = append(keys, key)
		items = append(items, s.item(t, w, key))
		previous = key
	}
	reorder, actor := s.reorder(t), s.actor(w)

	// The order as this test holds it, moved along with every step: the client's picture, which
	// the server's must equal at the end.
	order := append([]shared.ID(nil), items...)
	rank := map[shared.ID]string{}
	for i, id := range items {
		rank[id] = keys[i]
	}
	// A fixed sequence of positions rather than a random source: the walk is the same on every
	// run, and a failure names a step somebody can replay.
	random := &steps{state: 4}
	const moves = 1000
	changed := 0
	for step := range moves {
		from := random.next(len(order))
		moving := order[from]
		order = append(order[:from], order[from+1:]...)
		to := random.next(len(order) + 1)
		var before, after string
		if to > 0 {
			before = rank[order[to-1]]
		}
		if to < len(order) {
			after = rank[order[to]]
		}
		key, err := service.OrderKeyBetween(before, after)
		if err != nil {
			t.Fatalf("step %d: computing the rank between %q and %q: %v", step, before, after, err)
		}
		if _, err := reorder.Execute(s.ctx, actor, work.ReorderWorkItemCommand{ItemID: moving, OrderKey: key}); err != nil {
			t.Fatalf("step %d: the reorder was refused: %v", step, err)
		}
		if rank[moving] != key {
			// A move back into the slot an entry already holds computes the key it already has,
			// and writes nothing - which is a change log entry that is rightly not there.
			changed++
		}
		rank[moving] = key
		order = append(order[:to], append([]shared.ID{moving}, order[to:]...)...)
	}

	// The server's order is the client's, strictly increasing, and every key is the one the
	// client computed - nothing was renumbered. Byte order, as the repository orders: a rank key
	// is a string compared bytewise, and a locale's collation would put "Z" after "h".
	rows, err := s.admin.Query(s.ctx, `SELECT id, order_key FROM work_item WHERE collection_id = $1 ORDER BY order_key COLLATE "C"`, w.collection.String())
	if err != nil {
		t.Fatalf("reading the order: %v", err)
	}
	defer rows.Close()
	var stored []shared.ID
	last := ""
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			t.Fatalf("reading a row: %v", err)
		}
		if key <= last {
			t.Errorf("the order is not strictly increasing at %q after %q", key, last)
		}
		if rank[shared.ID(id)] != key {
			t.Errorf("%s holds %q, the client computed %q: a key was rewritten", id, key, rank[shared.ID(id)])
		}
		stored = append(stored, shared.ID(id))
		last = key
	}
	if len(stored) != len(order) {
		t.Fatalf("%d entries stored, %d held", len(stored), len(order))
	}
	for i := range order {
		if stored[i] != order[i] {
			t.Fatalf("the server's order diverges from the client's at position %d", i)
		}
	}
	// One change log entry per move that changed a key, each naming the moved entry's own key
	// and nothing else's: the count is the proof that no move touched a neighbour.
	if got := s.count(t, `SELECT count(*) FROM change_log WHERE tenant_id = $1 AND entity = 'item' AND payload ? 'order_key'`, w.tenant.String()); got != changed {
		t.Errorf("%d change log entries carry an order key, want one per move that changed one (%d of %d)", got, changed, moves)
	}
}

// steps is a linear congruential sequence, deterministic and good enough to spread a thousand
// moves over a collection.
type steps struct{ state uint64 }

func (s *steps) next(bound int) int {
	s.state = s.state*6364136223846793005 + 1442695040888963407
	return int((s.state >> 33) % uint64(bound))
}

// SY-11: a pull never returns another tenant's changes - the change log is behind row level
// security like every table, and neither the delta, the walk nor a scope naming the other
// tenant's container reaches across.
func TestSY11APullNeverReturnsAnotherTenantsChanges(t *testing.T) {
	s := newSuite(t)
	mine, theirs := s.workspace(t, "SY-11 mine"), s.workspace(t, "SY-11 theirs")
	theirItem := s.item(t, theirs, "a0")
	if err := s.uow.Within(s.ctx, persistence.Scope{TenantID: theirs.tenant}, func(ctx context.Context) error {
		reading, err := shared.HLC{}.Tick(now, "server-sync-evidence")
		if err != nil {
			return err
		}
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: theirs.tenant, Entity: "item", EntityID: theirItem, Op: changelog.Upsert,
			ContainerID: theirs.collection, ActorID: theirs.owner, HLC: reading,
			Payload: map[string]any{"title": "Theirs"},
		})
	}); err != nil {
		t.Fatalf("recording the other tenant's change: %v", err)
	}
	pull, actor := s.pull(t), s.actor(mine)

	leaked := func(records []syncservice.Record) []shared.ID {
		var out []shared.ID
		for _, record := range records {
			if record.TenantID == theirs.tenant || record.EntityID == theirItem ||
				record.EntityID == theirs.hub || record.EntityID == theirs.collection {
				out = append(out, record.EntityID)
			}
		}
		return out
	}
	// The walk from nothing, page by page.
	request := syncservice.PullRequest{DeviceID: freshID(t), Limit: 100}
	var cursor syncservice.Position
	for pages := 0; ; pages++ {
		page, err := pull.Pull(s.ctx, actor, request)
		if err != nil {
			t.Fatalf("walking: %v", err)
		}
		if got := leaked(page.Records); len(got) != 0 {
			t.Fatalf("the walk delivered another tenant's rows: %v", got)
		}
		if !page.More {
			cursor = page.Cursor
			break
		}
		request.Cursor = pull.Encode(page.Cursor)
		if pages > 100 {
			t.Fatalf("the walk does not end")
		}
	}
	// The delta since, and the delta narrowed to the other tenant's own collection by a scope a
	// client could forge: the scope narrows after the permission decides, and the permission
	// never sees a container of another tenant.
	for name, scopes := range map[string][]syncservice.Scope{
		"the delta":                       nil,
		"a scope naming their collection": {{ContainerID: theirs.collection, Depth: syncservice.DepthSubtree}},
	} {
		page, err := pull.Pull(s.ctx, actor, syncservice.PullRequest{DeviceID: request.DeviceID, Cursor: pull.Encode(cursor), Limit: 100, Scopes: scopes})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := leaked(page.Records); len(got) != 0 {
			t.Errorf("%s delivered another tenant's change: %v", name, got)
		}
	}
	// And the log itself, under my tenant's transaction, holds none of theirs.
	if err := s.uow.WithinReadOnly(s.ctx, persistence.Scope{TenantID: mine.tenant}, func(ctx context.Context) error {
		entries, err := postgres.NewChangeLog().After(ctx, 0, 1000)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.TenantID == theirs.tenant || entry.EntityID == theirItem {
				t.Errorf("the change log read across the tenant boundary: %+v", entry)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("reading the log: %v", err)
	}
}

// The ten rows the tasks wrote where their fixtures live: named here by file and function, and
// checked to exist, so that a rename or a deletion in test/integration reads as a hole in §11.
var referenced = []struct {
	row      string
	file     string
	function string
}{
	{"SY-1", "push_test.go", "TestTwoDevicesChangingDifferentFieldsBothSurvive"},
	{"SY-2", "push_test.go", "TestADeviceThreeHoursOutIsBoundedAndStillLands"},
	{"SY-3", "push_test.go", "TestConcurrentLabelChangesFromTwoDevicesYieldTheOrSetResult"},
	{"SY-5", "sync_retention_test.go", "TestADeviceBackAfterTheWindowStartsOverWithoutThePurgedEntry"},
	{"SY-6", "revocation_test.go", "TestAccessRevokedOfflineIsDeliveredAndThePushRefused"},
	{"SY-7", "push_test.go", "TestADuplicatePushCreatesTheEntryExactlyOnce"},
	{"SY-8", "push_test.go", "TestADoubleCompletionFromTwoDevicesProducesOneFollowUp"},
	{"SY-9", "sync_fanout_test.go", "TestFourHundredOfflineChangesToFortyEntriesOweFortyDeliveries"},
	{"SY-10", "push_test.go", "TestADisplacedNotesVersionIsFindableAgainAfterTheMerge"},
	{"SY-12", "push_test.go", "TestAMoveThatWouldCloseACycleIsRejected"},
}

func TestTheEvidenceTableIsCoveredRowByRow(t *testing.T) {
	for _, entry := range referenced {
		t.Run(entry.row, func(t *testing.T) {
			path := filepath.Join("..", "integration", entry.file)
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%s names %s, which cannot be read: %v", entry.row, path, err)
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ParseComments)
			if err != nil {
				t.Fatalf("%s: parsing %s: %v", entry.row, path, err)
			}
			found := false
			for _, declaration := range parsed.Decls {
				if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == entry.function {
					found = true
					if function.Doc == nil || !strings.Contains(function.Doc.Text(), entry.row) {
						t.Errorf("%s in %s does not say it is %s", entry.function, entry.file, entry.row)
					}
				}
			}
			if !found {
				t.Errorf("%s names %s in %s, and there is no such test", entry.row, entry.function, entry.file)
			}
		})
	}
}
