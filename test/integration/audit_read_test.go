// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	auditservice "github.com/Jersyfi/hubtask/core/application/service/audit"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	auditadapter "github.com/Jersyfi/hubtask/infrastructure/audit"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Reading the trail against a real PostgreSQL (E-09). Row level security, the partitions and the
// chain are the subject here, and none of the three can be tested against a fake of themselves.

// Every test here makes its own tenant. The assertions count entries and walk chains, and a trail
// that a neighbouring test also appends to is not one those assertions can be written against -
// the sequence numbers are per tenant, and so is everything below.
var readerE = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e1")

// slugOf is a tenant slug the schema accepts: lower case, no braces, and short enough for the
// forty character bound `0001_init` puts on it.
func slugOf(tenant shared.ID) string {
	digits := strings.ReplaceAll(tenant.String(), "-", "")
	return "audit-" + digits[len(digits)-20:]
}

// auditTenant seeds a tenant nothing else writes to, with one account to be the actor.
func auditTenant(ctx context.Context, t *testing.T) shared.ID {
	t.Helper()
	tenant := freshID(t)
	admin := adminPool(ctx, t)

	if _, err := admin.Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Audit')`,
		tenant.String(), slugOf(tenant)); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	// The account is a convenience for the actor label. The trail carries no foreign key to it -
	// that is what keeps an entry readable after a deletion (audit.md §2) - so it is here only so
	// that the fixture reads like a real tenant.
	if _, err := admin.Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Eve')`,
		freshID(t).String(), tenant.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	return tenant
}

func auditTrailRepo() postgres.AuditTrailRepository {
	return postgres.NewAuditTrailRepository(pageCursors())
}

// mixedEntries are the thousand AT-2 asks for: several actions, both outcomes an attempt can have,
// two actors and two targets, so that the filters below have something to be wrong about.
func mixedEntries(t *testing.T, tenant shared.ID, count int) []port.Entry {
	t.Helper()

	actions := []port.Action{"container.created", "container.renamed", "auth.login_failed", "membership.role_changed"}
	outcomes := []port.Outcome{port.OutcomeSuccess, port.OutcomeDenied, port.OutcomeFailed}
	other := freshID(t)
	target := freshID(t)

	entries := make([]port.Entry, 0, count)
	for i := range count {
		entry := port.Entry{
			TenantID: tenant, OccurredAt: created.Add(time.Duration(i) * time.Second),
			Action: actions[i%len(actions)], Outcome: outcomes[i%len(outcomes)],
			Severity: port.SeverityInfo, ActorKind: shared.ActorUser,
			ActorID: readerE, ActorLabel: "Eve Beispiel",
			TargetType: "container", TargetID: target,
			Changes: port.Changes(
				port.Change{Field: "index", Classification: port.Open, To: i},
			),
		}
		if i%3 == 0 {
			entry.ActorID, entry.ActorLabel = other, "Somebody Else"
		}
		if i%5 == 0 {
			entry.TargetType, entry.TargetID = "legal_hold", freshID(t)
		}
		entries = append(entries, entry)
	}
	return entries
}

// appendTo writes a batch through the real sink, which is the only thing that builds a chain.
//
// One unit of work for the whole batch rather than one per entry: `write` opens a pool of its own
// every time it is called, and a thousand of those is a thousand pools rather than a thousand
// transactions.
func appendTo(ctx context.Context, t *testing.T, tenant shared.ID, entries []port.Entry) {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	sink := postgres.NewAuditSink(generator{t})

	for _, entry := range entries {
		if err := unitOfWork.Within(ctx, persistence.Scope{TenantID: tenant},
			func(ctx context.Context) error { return sink.Append(ctx, entry) }); err != nil {
			t.Fatalf("appending an audit entry: %v", err)
		}
	}
}

func verifierFor(t *testing.T) auditservice.VerifyAuditChain {
	t.Helper()
	ctx := context.Background()

	return auditservice.VerifyAuditChain{
		Trail:      auditTrailRepo(),
		Chain:      auditadapter.Links{},
		Authorizer: permissive{},
		Audit:      postgres.NewAuditSink(generator{t}),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock:      portclock.Fixed(created),
	}
}

// permissive answers yes. The access model is decided in the application layer and tested there
// with fakes; what this file is about is what the database does underneath it.
type permissive struct{}

func (permissive) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return nil
}

func (permissive) Permits(context.Context, appshared.ActorContext, access.Request) (bool, error) {
	return true, nil
}

func auditActor(tenant shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: readerE,
		AccountName: "Eve Beispiel", Scopes: []string{"audit:read", "audit:export"},
	}
}

// Test AT-2: a thousand mixed events, and the chain and the sequence hold over all of them.
func TestAThousandEventsFormOneUnbrokenChain(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	appendTo(ctx, t, tenant, mixedEntries(t, tenant, 1000))

	found, err := verifierFor(t).Execute(ctx, auditActor(tenant), repository.Period{
		From: created.Add(-time.Hour), To: created.Add(2000 * time.Second),
	})
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}

	if !found.Valid {
		t.Fatalf("a chain written by the sink itself does not verify: %+v", found)
	}
	if found.Checked < 1000 {
		t.Errorf("%d entries were checked, want at least the thousand written", found.Checked)
	}
	if found.GapCount != 0 || found.FirstBrokenSeq != 0 {
		t.Errorf("the chain reports %d gaps and a break at %d", found.GapCount, found.FirstBrokenSeq)
	}
	// Nothing anchors yet, and the honest answer is that nothing is sealed (audit.md §3, A-2).
	if !found.SealedUntil.IsZero() {
		t.Errorf("an installation with no anchor claims to be sealed until %s", found.SealedUntil)
	}
}

// And the other half of AT-2: a row edited directly in the database is found, at its own sequence
// number.
//
// The edit is made with the trigger switched off, as the owner - which is what "an operator who
// configured themselves too much power" looks like, and the case the hash chain exists for. The
// application role cannot do this at all, which is what the test above it asserts.
func TestARowEditedInTheDatabaseIsFoundByVerification(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	appendTo(ctx, t, tenant, mixedEntries(t, tenant, 12))

	admin := adminPool(ctx, t)
	var tampered int64
	if err := admin.QueryRow(ctx,
		`SELECT seq FROM audit_log WHERE tenant_id = $1 ORDER BY seq OFFSET 4 LIMIT 1`,
		tenant.String()).Scan(&tampered); err != nil {
		t.Fatalf("choosing a row to tamper with: %v", err)
	}

	if _, err := admin.Exec(ctx, `ALTER TABLE audit_log DISABLE TRIGGER audit_log_no_update`); err != nil {
		t.Fatalf("disabling the immutability trigger: %v", err)
	}
	edited, err := admin.Exec(ctx,
		`UPDATE audit_log SET action = 'container.tampered' WHERE tenant_id = $1 AND seq = $2`,
		tenant.String(), tampered)
	if _, enable := admin.Exec(ctx, `ALTER TABLE audit_log ENABLE TRIGGER audit_log_no_update`); enable != nil {
		t.Fatalf("re-enabling the immutability trigger: %v", enable)
	}
	if err != nil {
		t.Fatalf("tampering with the row: %v", err)
	}
	if edited.RowsAffected() != 1 {
		t.Fatalf("%d rows were edited - the test tampered with nothing", edited.RowsAffected())
	}

	found, verifyErr := verifierFor(t).Execute(ctx, auditActor(tenant), repository.Period{
		From: created.Add(-time.Hour), To: created.Add(2000 * time.Second),
	})
	if verifyErr != nil {
		t.Fatalf("verifying: %v", verifyErr)
	}

	if found.Valid {
		t.Fatal("a row edited in the database verified as intact")
	}
	if found.FirstBrokenSeq != tampered {
		t.Errorf("the break was reported at %d, want %d", found.FirstBrokenSeq, tampered)
	}
	// The finding is itself recorded, and it is the one entry a verification writes.
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'audit.chain_broken'`,
		tenant.String()); rows != 1 {
		t.Errorf("%d entries recorded the broken chain", rows)
	}
}

// The filters §5 asks for, against real rows: period, action prefix, actor, target and outcome.
func TestTheFiltersNarrowTheTrail(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	entries := mixedEntries(t, tenant, 40)
	appendTo(ctx, t, tenant, entries)

	trail := auditTrailRepo()
	page := func(filter repository.Filter) []repository.Record {
		t.Helper()
		filter.Page.Size = 500
		var found repository.RecordPage
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			found, err = trail.Query(ctx, filter)
			return err
		}); err != nil {
			t.Fatalf("reading the trail: %v", err)
		}
		return found.Records
	}

	// A prefix rather than a pattern: `auth.` is every authentication event.
	for _, record := range page(repository.Filter{ActionPrefix: "auth."}) {
		if record.Entry.Action != "auth.login_failed" {
			t.Errorf("the action filter answered %s", record.Entry.Action)
		}
	}
	if len(page(repository.Filter{ActionPrefix: "auth."})) == 0 {
		t.Error("the action filter answered nothing at all")
	}

	for _, record := range page(repository.Filter{Outcome: port.OutcomeDenied}) {
		if record.Entry.Outcome != port.OutcomeDenied {
			t.Errorf("the outcome filter answered %s", record.Entry.Outcome)
		}
	}

	// The target filter §5 requires and the specification lacked: what happened to this object.
	target := entries[1].TargetID
	found := page(repository.Filter{TargetType: "container", TargetID: target})
	if len(found) == 0 {
		t.Fatal("the target filter answered nothing")
	}
	for _, record := range found {
		if record.Entry.TargetID != target {
			t.Errorf("the target filter answered an entry about %s", record.Entry.TargetID)
		}
	}

	// The period, which is exclusive at the end.
	within := page(repository.Filter{From: created, To: created.Add(10 * time.Second)})
	for _, record := range within {
		if record.Entry.OccurredAt.Before(created) || !record.Entry.OccurredAt.Before(created.Add(10*time.Second)) {
			t.Errorf("the period filter answered an entry from %s", record.Entry.OccurredAt)
		}
	}
	if len(within) == 0 {
		t.Error("the period filter answered nothing at all")
	}
}

// The walk pages under the covers, and the paging is what makes it usable over four hundred days:
// it must hand over every entry exactly once.
func TestTheWalkHandsOverEveryEntryOnce(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	appendTo(ctx, t, tenant, mixedEntries(t, tenant, 600))

	seen := map[int64]int{}
	previous := int64(0)
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		return auditTrailRepo().Walk(ctx, repository.Period{}, func(record repository.Record) error {
			seen[record.Seq]++
			if record.Seq <= previous {
				t.Errorf("the walk went backwards: %d after %d", record.Seq, previous)
			}
			previous = record.Seq
			return nil
		})
	}); err != nil {
		t.Fatalf("walking the trail: %v", err)
	}

	if len(seen) < 600 {
		t.Errorf("the walk handed over %d entries, want at least the 600 written", len(seen))
	}
	for seq, times := range seen {
		if times != 1 {
			t.Errorf("entry %d was handed over %d times", seq, times)
		}
	}
}

// A page carries a cursor, and the cursor continues exactly where the page stopped.
func TestThePagesOfTheTrailDoNotOverlap(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	appendTo(ctx, t, tenant, mixedEntries(t, tenant, 25))

	trail := auditTrailRepo()
	seen := map[string]int{}
	cursor := ""

	for range 10 {
		var page repository.RecordPage
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			page, err = trail.Query(ctx, repository.Filter{
				Page: repository.Page{Cursor: cursor, Size: 7},
			})
			return err
		}); err != nil {
			t.Fatalf("reading a page: %v", err)
		}
		for _, record := range page.Records {
			seen[record.ID.String()]++
		}
		if !page.Info.HasMore {
			break
		}
		cursor = page.Info.NextCursor
	}

	for id, times := range seen {
		if times != 1 {
			t.Errorf("entry %s appeared on %d pages", id, times)
		}
	}
	if len(seen) < 25 {
		t.Errorf("the walk over pages saw %d entries, want at least 25", len(seen))
	}
}

// The cross-tenant negative test the rules ask of every new repository method: what one tenant
// reads is its own trail, whatever it asks for.
func TestOneTenantCannotReadAnothersTrail(t *testing.T) {
	ctx := context.Background()
	mine, theirs := auditTenant(ctx, t), auditTenant(ctx, t)
	appendTo(ctx, t, mine, mixedEntries(t, mine, 5))
	appendTo(ctx, t, theirs, mixedEntries(t, theirs, 5))

	trail := auditTrailRepo()
	var page repository.RecordPage
	var walked int

	if err := read(ctx, t, theirs, func(ctx context.Context) error {
		var err error
		if page, err = trail.Query(ctx, repository.Filter{Page: repository.Page{Size: 500}}); err != nil {
			return err
		}
		return trail.Walk(ctx, repository.Period{}, func(record repository.Record) error {
			walked++
			if record.Entry.TenantID != theirs {
				t.Errorf("the walk handed over an entry of %s", record.Entry.TenantID)
			}
			return nil
		})
	}); err != nil {
		t.Fatalf("reading as the second tenant: %v", err)
	}

	for _, record := range page.Records {
		if record.Entry.TenantID != theirs {
			t.Errorf("the page carried an entry of %s", record.Entry.TenantID)
		}
	}
	if len(page.Records) == 0 || walked == 0 {
		t.Fatal("the second tenant read nothing at all, which proves nothing about isolation")
	}
}

// And the anchor, which is read under the same policy.
func TestTheAnchorIsReadPerTenantAndIsEmptyToday(t *testing.T) {
	ctx := context.Background()
	tenant, other := auditTenant(ctx, t), auditTenant(ctx, t)

	var anchor repository.Anchor
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		anchor, err = auditTrailRepo().LatestAnchor(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the anchor: %v", err)
	}
	if !anchor.IsZero() {
		t.Errorf("an installation that anchors nothing answered %+v", anchor)
	}

	// One tenant's anchor is not another's, which is what the policy on the table says.
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO audit_anchor (tenant_id, anchored_at, last_seq, chain_hash)
		VALUES ($1, $2, 3, '\x01') ON CONFLICT DO NOTHING`,
		tenant.String(), created); err != nil {
		t.Fatalf("seeding an anchor: %v", err)
	}

	var ours, alien repository.Anchor
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		ours, err = auditTrailRepo().LatestAnchor(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the anchor: %v", err)
	}
	if err := read(ctx, t, other, func(ctx context.Context) error {
		var err error
		alien, err = auditTrailRepo().LatestAnchor(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the other tenant's anchor: %v", err)
	}
	if ours.IsZero() || ours.LastSeq != 3 {
		t.Errorf("the anchor came back as %+v", ours)
	}
	if !alien.IsZero() {
		t.Errorf("a tenant read another's anchor: %+v", alien)
	}
}

// The partition duty (E-09, audit.md §3): a partition created later carries its own policy and its
// own revoked grants, because a partition addressed directly is a table of its own.
func TestAFreshPartitionCarriesItsPolicyAndItsRevokes(t *testing.T) {
	ctx := context.Background()
	mine, theirs := auditTenant(ctx, t), auditTenant(ctx, t)

	// Far enough ahead that nothing else has written into that month.
	month := created.AddDate(0, 7, 0)
	var name string
	if err := postgres.NewUnitOfWork(appPool(ctx, t)).
		Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			var err error
			name, err = postgres.NewAuditPartitionRepository().Ensure(ctx, month)
			return err
		}); err != nil {
		t.Fatalf("ensuring the partition: %v", err)
	}
	if name == "" {
		t.Fatalf("no partition was created for %s", month.Format("2006-01"))
	}
	dropPartition(ctx, t, name)

	// Two tenants with a row in that partition, written through the parent as the application does.
	appendTo(ctx, t, mine, []port.Entry{futureEntry(mine, month)})
	appendTo(ctx, t, theirs, []port.Entry{futureEntry(theirs, month)})

	app := appPool(ctx, t)
	if _, err := app.Exec(ctx, `SELECT set_config('app.tenant_id', $1, false)`, mine.String()); err != nil {
		t.Fatalf("setting the tenant context: %v", err)
	}

	// Addressing the partition directly is the case `0001_init` measured: without a policy of its
	// own, both tenants' rows would come back through it.
	var visible int
	if err := app.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, name)).Scan(&visible); err != nil {
		t.Fatalf("reading the partition directly: %v", err)
	}
	if visible != 1 {
		t.Errorf("%d rows are visible through %s under one tenant's context - want the one that tenant wrote",
			visible, name)
	}

	// Test AT-1 on the partition: the grant and the trigger, both.
	for _, statement := range []string{
		fmt.Sprintf(`UPDATE %s SET action = 'tampered'`, name),
		fmt.Sprintf(`DELETE FROM %s`, name),
		fmt.Sprintf(`TRUNCATE %s`, name),
	} {
		if _, err := app.Exec(ctx, statement); err == nil {
			t.Errorf("the application role could run: %s", statement)
		}
	}
}

// dropPartition takes a partition this file created away again. The schema reference test compares
// a migrated database with db/schema.sql object for object, and a partition a test left behind is a
// difference it would report as a drift in the schema.
func dropPartition(ctx context.Context, t *testing.T, name string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := adminPool(ctx, t).Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, name)); err != nil {
			t.Errorf("dropping %s: %v", name, err)
		}
	})
}

func futureEntry(tenant shared.ID, at time.Time) port.Entry {
	return port.Entry{
		TenantID: tenant, OccurredAt: at, Action: "container.created",
		Outcome: port.OutcomeSuccess, Severity: port.SeverityInfo,
		ActorKind: shared.ActorUser, ActorID: readerE, ActorLabel: "Eve Beispiel",
		TargetType: "container",
	}
}

// The duty is idempotent, and it repairs. A partition that has lost its policy - which is what a
// partition created by hand looks like - is brought back into line by the next run.
func TestThePartitionDutyRepairsWhatItFinds(t *testing.T) {
	ctx := context.Background()
	month := created.AddDate(0, 9, 0)
	ensure := func() string {
		t.Helper()
		var name string
		if err := postgres.NewUnitOfWork(appPool(ctx, t)).
			Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
				var err error
				name, err = postgres.NewAuditPartitionRepository().Ensure(ctx, month)
				return err
			}); err != nil {
			t.Fatalf("ensuring the partition: %v", err)
		}
		return name
	}

	name := ensure()
	if name == "" {
		t.Fatal("no partition was created")
	}
	dropPartition(ctx, t, name)
	if again := ensure(); again != name {
		t.Errorf("running the duty twice answered %q and %q", name, again)
	}

	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, fmt.Sprintf(`DROP POLICY tenant_isolation ON %s`, name)); err != nil {
		t.Fatalf("removing the policy: %v", err)
	}
	if _, err := admin.Exec(ctx, fmt.Sprintf(`GRANT UPDATE ON %s TO hubtask_app`, name)); err != nil {
		t.Fatalf("granting what the duty has to take away: %v", err)
	}

	ensure()

	var policies, updates int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM pg_policies WHERE tablename = $1 AND policyname = 'tenant_isolation'`,
		name).Scan(&policies); err != nil {
		t.Fatalf("reading the policies: %v", err)
	}
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.role_table_grants
		 WHERE table_name = $1 AND grantee = 'hubtask_app' AND privilege_type = 'UPDATE'`,
		name).Scan(&updates); err != nil {
		t.Fatalf("reading the grants: %v", err)
	}
	if policies != 1 {
		t.Error("the duty did not restore the policy")
	}
	if updates != 0 {
		t.Error("the duty did not take the UPDATE grant away again")
	}
}

// The export, over real rows (E-09, audit.md §5). The format is tested with fakes where it is
// decided; what this asks is whether the walk, the projection and the archive fit together over a
// trail a database actually wrote.

// memoryTarget stands in for a backup target. Writing to a real one is E-03's and E-05's subject;
// what matters here is which members an export produces and what is in them.
type memoryTarget struct{ written map[string][]byte }

func (m *memoryTarget) OpenTarget(context.Context, shared.ID, shared.ID) (backupstorage.Store, error) {
	return m, nil
}

func (m *memoryTarget) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	body, err := io.ReadAll(content)
	if err != nil {
		return 0, err
	}
	m.written[key] = body
	return int64(len(body)), nil
}

func (m *memoryTarget) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, shared.ErrNotFound
}
func (m *memoryTarget) List(context.Context, string) ([]backupstorage.Entry, error) { return nil, nil }
func (m *memoryTarget) Stat(context.Context, string) (backupstorage.Entry, error) {
	return backupstorage.Entry{}, shared.ErrNotFound
}
func (m *memoryTarget) Delete(context.Context, string) error { return nil }

func (m *memoryTarget) member(t *testing.T, name string) []byte {
	t.Helper()
	for key, content := range m.written {
		if strings.HasSuffix(key, "/"+name) {
			return content
		}
	}
	t.Fatalf("the archive has no %s", name)
	return nil
}

func TestAnExportCarriesTheTrailItWalked(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	appendTo(ctx, t, tenant, mixedEntries(t, tenant, 120))

	target := &memoryTarget{written: map[string][]byte{}}
	archivist := auditservice.Archivist{
		Trail: auditTrailRepo(), Targets: target,
		Encryptor:  keylessInstallation{},
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock:      portclock.Fixed(created), ProductVersion: "integration",
	}

	manifest, err := archivist.Write(ctx, auditservice.ArchiveRequest{
		ExportID: freshID(t), TenantID: tenant, TargetID: freshID(t),
		Period: repository.Period{From: created.Add(-time.Hour), To: created.Add(2000 * time.Second)},
		Format: auditservice.FormatJSONL,
	})
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}

	if manifest.Entries != 120 {
		t.Errorf("the export holds %d entries, want the 120 written", manifest.Entries)
	}
	if manifest.FirstSeq != 1 || manifest.LastSeq != 120 {
		t.Errorf("the export covers seq %d..%d", manifest.FirstSeq, manifest.LastSeq)
	}

	lines := strings.Split(strings.TrimSpace(string(target.member(t, "entries.jsonl"))), "\n")
	if len(lines) != 120 {
		t.Fatalf("%d lines were written", len(lines))
	}
	// The evidence a reader is left with: every line carries its own hash, and the manifest names
	// the stretch of chain, which is what `:verify` can be compared against.
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("the first line is not JSON: %v", err)
	}
	if first["hash"] == "" || first["seq"] == nil {
		t.Errorf("an exported entry carries no place in the chain: %v", first)
	}
}

// keylessInstallation is the default: no master key, so no signature - and the manifest says so
// rather than writing something that looks like one.
type keylessInstallation struct{}

func (keylessInstallation) Seal(context.Context, secret.Secret, crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{}, shared.ErrUnavailable.WithDetail("crypto.no_encryption_key")
}

func (keylessInstallation) Open(context.Context, crypto.Sealed, crypto.Purpose) (secret.Secret, error) {
	return secret.Secret{}, shared.ErrUnavailable.WithDetail("crypto.unknown_key")
}
func (keylessInstallation) ActiveKeyID() string { return "" }

// The `AUDITOR` role, tried against the database (E-09, audit.md §5): the trail and no content.
//
// It is asked of the real authorisation service and a real membership row, because the role has
// three halves that have to agree - the enum value the column constrains, the resolution that finds
// the membership, and the matrix that says what it carries. A unit test can only ever prove the
// third.
func TestAnAuditorReadsTheTrailAndNoContent(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	auditor := freshID(t)

	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Iris Auditor')`,
		auditor.String(), tenant.String()); err != nil {
		t.Fatalf("seeding the auditor's account: %v", err)
	}
	if _, err := admin.Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		 VALUES ($1, $2, $3, 'TENANT', 'AUDITOR')`,
		freshID(t).String(), tenant.String(), auditor.String()); err != nil {
		t.Fatalf("granting the AUDITOR role: %v", err)
	}

	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  postgres.NewUnitOfWork(appPool(ctx, t)),
		Audit:       postgres.NewAuditSink(generator{t}),
		Clock:       portclock.Fixed(created),
	}
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: auditor,
		AccountName: "Iris Auditor", Scopes: []string{"audit:read", "containers:read", "items:read"},
	}

	permits := func(permission domainservice.Permission) bool {
		t.Helper()
		allowed, err := authorizer.Permits(ctx, actor, access.Request{
			Permission: permission, Path: []identity.Scope{identity.TenantScope()},
		})
		if err != nil {
			t.Fatalf("asking about %s: %v", permission, err)
		}
		return allowed
	}

	if !permits(domainservice.PermissionAuditRead) {
		t.Error("an AUDITOR cannot read the trail, which is the whole of what the role is for")
	}
	for _, refused := range []domainservice.Permission{
		domainservice.PermissionRead,
		domainservice.PermissionWriteItems,
		domainservice.PermissionStructure,
		domainservice.PermissionManageMembers,
		domainservice.PermissionDeleteContainer,
	} {
		if permits(refused) {
			t.Errorf("an AUDITOR may %s", refused)
		}
	}

	// And the attempt itself: a use case over content refuses them, and the refusal is recorded.
	before := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND outcome = 'DENIED'`, tenant.String())
	err := authorizer.Authorize(ctx, actor, access.Request{
		Permission: domainservice.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     "container.read", TargetType: "container", TargetID: tenant,
	})
	if err == nil {
		t.Fatal("an AUDITOR read content")
	}
	if after := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND outcome = 'DENIED'`,
		tenant.String()); after != before+1 {
		t.Errorf("%d denied entries, want %d", after, before+1)
	}
}

// Somebody who is an auditor *and* a member holds both, which is what a union over the memberships
// buys and what "the highest role wins" could not express: one of the two would have taken the
// other away.
func TestAnAuditorWhoIsAlsoAMemberKeepsBoth(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	both := freshID(t)

	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Ivo Both')`,
		both.String(), tenant.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	for _, role := range []string{"AUDITOR", "MEMBER"} {
		if _, err := admin.Exec(ctx,
			`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
			 VALUES ($1, $2, $3, 'TENANT', $4)`,
			freshID(t).String(), tenant.String(), both.String(), role); err != nil {
			t.Fatalf("granting %s: %v", role, err)
		}
	}

	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  postgres.NewUnitOfWork(appPool(ctx, t)),
		Audit:       postgres.NewAuditSink(generator{t}),
		Clock:       portclock.Fixed(created),
	}
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: both, AccountName: "Ivo Both",
	}

	for _, permission := range []domainservice.Permission{
		domainservice.PermissionAuditRead, domainservice.PermissionRead,
		domainservice.PermissionWriteItems,
	} {
		allowed, err := authorizer.Permits(ctx, actor, access.Request{
			Permission: permission, Path: []identity.Scope{identity.TenantScope()},
		})
		if err != nil {
			t.Fatalf("asking about %s: %v", permission, err)
		}
		if !allowed {
			t.Errorf("holding both memberships does not carry %s", permission)
		}
	}
}

// Test AT-2, the half a thousand sequential entries cannot reach: writers that overlap.
//
// The defect this pins down was real and shipped. `LastAuditEntry` ordered the tail by
// `occurred_at DESC, seq DESC`, and every caller takes its own `Clock.Now()` *before* it queues for
// the per-tenant advisory lock - so the newest timestamp is not always the highest sequence number.
// A transaction that read such a tail continued from a number that was already taken, and the
// unique index could not stop it: a partitioned table's unique index has to carry the partition
// key, and `(tenant_id, occurred_at, seq)` lets one `seq` appear twice under two timestamps. Eight
// concurrent writes were enough to produce duplicated sequence numbers, a chain that no longer
// verified, and an `audit.chain_broken` entry reporting tampering that never happened.
//
// The entries here carry deliberately *descending* timestamps, which is what a set of callers whose
// clocks disagree looks like from the database's side, and what makes this test fail against the
// old ordering every time rather than once in a while.
func TestOverlappingWritersDoNotReuseASequenceNumber(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)

	const writers = 8
	entries := mixedEntries(t, tenant, writers)
	for i := range entries {
		entries[i].OccurredAt = created.Add(time.Duration(writers-i) * time.Second)
	}

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	sink := postgres.NewAuditSink(generator{t})

	var wg sync.WaitGroup
	failures := make(chan error, writers)
	for _, entry := range entries {
		wg.Add(1)
		go func(entry port.Entry) {
			defer wg.Done()
			if err := unitOfWork.Within(ctx, persistence.Scope{TenantID: tenant},
				func(ctx context.Context) error { return sink.Append(ctx, entry) }); err != nil {
				failures <- err
			}
		}(entry)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatalf("appending concurrently: %v", err)
	}

	// The numbering itself, read straight from the table: one row per sequence number, and no
	// number skipped. This is the assertion the verifier's own report is derived from, and it is
	// checked here rather than through it so that a break is unambiguous.
	rows, err := adminPool(ctx, t).Query(ctx,
		`SELECT seq, count(*) FROM audit_log WHERE tenant_id = $1 GROUP BY seq ORDER BY seq`,
		tenant.String())
	if err != nil {
		t.Fatalf("reading the chain: %v", err)
	}
	defer rows.Close()

	expected := int64(1)
	for rows.Next() {
		var seq, count int64
		if err := rows.Scan(&seq, &count); err != nil {
			t.Fatalf("reading the chain: %v", err)
		}
		if count != 1 {
			t.Errorf("sequence number %d was written %d times - two writers continued from one tail",
				seq, count)
		}
		if seq != expected {
			t.Errorf("sequence number %d follows %d - the chain has a hole", seq, expected-1)
		}
		expected = seq + 1
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the chain: %v", err)
	}
	if expected-1 != writers {
		t.Errorf("%d entries are in the chain, %d were written", expected-1, writers)
	}

	// And the verdict the operator would get from `hubctl audit verify`.
	found, err := verifierFor(t).Execute(ctx, auditActor(tenant), repository.Period{
		From: created.Add(-time.Hour), To: created.Add(2000 * time.Second),
	})
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if !found.Valid || found.GapCount != 0 {
		t.Errorf("a chain written by overlapping writers does not verify: %+v", found)
	}
}
