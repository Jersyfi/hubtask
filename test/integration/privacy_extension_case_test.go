// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	auditrepository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	privacyrepository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The extension through its use case against a real PostgreSQL, as the application role, through
// the real transaction wrapper (UC-PRV-01 checks 9 and 10): the bound a case answers and takes back,
// one winner of two, the watch reading the extended deadline, and the entry as it is stored.

// extensionReceived is when the cases here are recorded: 10 August 2026, 09:00 in Berlin. Before
// `created`, so the audit chain verifier's own clock stands after every entry written here.
var extensionReceived = time.Date(2026, 8, 10, 7, 0, 0, 0, time.UTC)

type extensionWorld struct {
	tenant   shared.ID
	registry *usecase.Registry
}

func (w extensionWorld) actor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: w.tenant, AccountID: readerE, AccountName: "Eve Beispiel",
		Scopes: []string{"privacy:read", "privacy:manage"},
	}
}

// newExtensionWorld seeds a workspace counting in Berlin, and the privacy use cases over it at a
// fixed moment.
func newExtensionWorld(ctx context.Context, t *testing.T, now time.Time) extensionWorld {
	t.Helper()
	tenant := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name, default_time_zone)
		 VALUES ($1, $2, 'Extension', 'Europe/Berlin')`,
		tenant.String(), slugOf(tenant)); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	return extensionWorld{tenant: tenant, registry: extensionRegistry(ctx, t, now)}
}

func extensionRegistry(ctx context.Context, t *testing.T, now time.Time) *usecase.Registry {
	t.Helper()
	cases := privacyservice.Cases{
		Requests: privacyRepo(), Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Jobs: jobQueue(t), Authorizer: permissive{}, Audit: postgres.NewAuditSink(generator{t}),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: portclock.Fixed(now),
		IDs: generator{t},
	}
	registry, err := usecase.NewRegistry(nil,
		privacyservice.CreateDataSubjectRequest{Cases: cases}.Descriptor(),
		privacyservice.ListDataSubjectRequests{Cases: cases}.Descriptor(),
		privacyservice.ExtendDataSubjectRequest{Cases: cases}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return registry
}

func (w extensionWorld) record(ctx context.Context, t *testing.T) string {
	t.Helper()
	out, err := w.registry.Invoke(ctx, privacyservice.CreateDataSubjectRequestName, w.actor(), usecase.Input{
		"kind": string(domain.KindAccess), "subject_email": "ext-" + w.tenant.String() + "@example.org",
		"notes": "Asked by letter, see folder 7",
	})
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	return out.String("id")
}

func (w extensionWorld) extend(ctx context.Context, id, dueOn string) (usecase.Output, error) {
	return w.registry.Invoke(ctx, privacyservice.ExtendDataSubjectRequestName, w.actor(), usecase.Input{
		"request_id": id, "due_on": dueOn, "reason": string(domain.ReasonNumberOfRequests),
		"informed_on": "2026-08-10",
	})
}

// The bound the list answers is sent back unchanged and accepted, and the stored deadline is the
// last second of that day in Berlin - a whole second, which PostgreSQL's microseconds keep.
func TestTheBoundAListAnswersIsAcceptedAndStoredAsTheEndOfThatDay(t *testing.T) {
	ctx := context.Background()
	now := extensionReceived.Add(26 * time.Hour)
	world := newExtensionWorld(ctx, t, extensionReceived)
	id := world.record(ctx, t)

	listed, err := world.registry.Invoke(ctx, privacyservice.ListDataSubjectRequestsName, world.actor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows := listed["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0].String("extendable_until") != "2026-11-10" {
		t.Fatalf("the list answered %v, want extendable_until 2026-11-10", rows)
	}

	later := newExtensionWorld(ctx, t, now)
	later.tenant = world.tenant
	if _, err := later.extend(ctx, id, rows[0].String("extendable_until")); err != nil {
		t.Fatalf("sending the bound back: %v", err)
	}

	stored := findCase(ctx, t, world.tenant, shared.MustParseID(id))
	if got := stored.DueAt.UTC().Format(time.RFC3339Nano); got != "2026-11-10T22:59:59Z" {
		t.Errorf("the deadline is stored as %s, want the end of 10 November in Berlin", got)
	}
	if got := stored.OriginalDueAt.UTC().Format(time.RFC3339); got != "2026-09-09T07:00:00Z" {
		t.Errorf("the original deadline is stored as %s", got)
	}

	// Read back through the list: both dates, and no bound any more.
	again, err := later.registry.Invoke(ctx, privacyservice.ListDataSubjectRequestsName, later.actor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	row := again["data"].([]usecase.Output)[0]
	if _, present := row["extendable_until"]; present || row.String("informed_on") != "2026-08-10" ||
		row["original_due_at"] == nil {
		t.Errorf("the extended case lists as %v", row)
	}
}

// Two extensions of one case at once: exactly one is written, the other is refused as already
// extended.
func TestOfTwoExtensionsAtOnceExactlyOneWins(t *testing.T) {
	ctx := context.Background()
	world := newExtensionWorld(ctx, t, extensionReceived)
	id := world.record(ctx, t)

	var (
		wait    sync.WaitGroup
		start   = make(chan struct{})
		results = make([]error, 2)
	)
	for i, day := range []string{"2026-10-20", "2026-10-30"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, results[i] = world.extend(ctx, id, day)
		}()
	}
	close(start)
	wait.Wait()

	won, refused := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			won++
		case shared.AsError(err) != nil && shared.AsError(err).DetailCode == domain.CodeAlreadyExtended:
			refused++
		default:
			t.Errorf("an extension failed with %v", err)
		}
	}
	if won != 1 || refused != 1 {
		t.Errorf("%d extensions won and %d were refused, want one each", won, refused)
	}
	if entries := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'dsr.extended'`,
		world.tenant.String()); entries != 1 {
		t.Errorf("%d extensions were recorded", entries)
	}
}

func TestAClosedCaseOrAnotherWorkspacesIsNotExtended(t *testing.T) {
	ctx := context.Background()
	world := newExtensionWorld(ctx, t, extensionReceived)
	id := world.record(ctx, t)

	// SG-3: from another workspace the case is not there.
	stranger := newExtensionWorld(ctx, t, extensionReceived)
	if _, err := stranger.extend(ctx, id, "2026-10-20"); shared.AsError(err).DetailCode != domain.CodeRequestNotFound {
		t.Errorf("another workspace's extension was answered %v", err)
	}

	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE data_subject_request SET status = 'COMPLETED', completed_at = $2 WHERE id = $1`,
		id, extensionReceived.Add(time.Hour)); err != nil {
		t.Fatalf("completing the case: %v", err)
	}
	if _, err := world.extend(ctx, id, "2026-10-20"); shared.AsError(err).DetailCode != domain.CodeRequestClosed {
		t.Errorf("a completed case's extension was answered %v", err)
	}
	if found := findCase(ctx, t, world.tenant, shared.MustParseID(id)); found.Extended() {
		t.Error("a refused extension was stored")
	}
}

// UC-PRV-01 check 10: the watch reads the extended deadline - a case past its original one and
// before its extended one is neither overdue nor due before the original.
func TestTheWatchReadsTheExtendedDeadline(t *testing.T) {
	ctx := context.Background()
	world := newExtensionWorld(ctx, t, extensionReceived)
	id := world.record(ctx, t)
	if _, err := world.extend(ctx, id, "2026-10-20"); err != nil {
		t.Fatalf("extending: %v", err)
	}

	pastTheOriginal := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	var (
		deadlines privacyrepository.Deadlines
		page      privacyrepository.Page
	)
	if err := read(ctx, t, world.tenant, func(ctx context.Context) error {
		var err error
		if deadlines, err = privacyRepo().Deadlines(ctx, pastTheOriginal); err != nil {
			return err
		}
		page, err = privacyRepo().List(ctx, privacyrepository.Filter{DueBefore: pastTheOriginal, Size: 10})
		return err
	}); err != nil {
		t.Fatalf("reading the watch: %v", err)
	}
	if deadlines.Overdue != 0 || deadlines.Open != 1 {
		t.Errorf("the watch counts %d overdue of %d open", deadlines.Overdue, deadlines.Open)
	}
	if got := deadlines.NextDueAt.UTC().Format(time.RFC3339); got != "2026-10-20T21:59:59Z" {
		t.Errorf("the next deadline is %s, want the extended one", got)
	}
	if len(page.Requests) != 0 {
		t.Errorf("the case still falls due before the original deadline: %+v", page.Requests)
	}
}

// UC-PRV-01 check 10: the entry as stored - the action, the reason and both dates, no note - and
// the chain over it intact.
func TestTheExtensionsEntryIsStoredWithoutTheNotesAndTheChainHolds(t *testing.T) {
	ctx := context.Background()
	world := newExtensionWorld(ctx, t, extensionReceived)
	id := world.record(ctx, t)
	if _, err := world.extend(ctx, id, "2026-10-20"); err != nil {
		t.Fatalf("extending: %v", err)
	}

	var (
		legalBasis, target string
		changes            []byte
	)
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT legal_basis, target_id::text, changes::text FROM audit_log
		 WHERE tenant_id = $1 AND action = 'dsr.extended'`, world.tenant.String()).
		Scan(&legalBasis, &target, &changes); err != nil {
		t.Fatalf("reading the entry: %v", err)
	}
	if legalBasis != "dsr.access" || target != id {
		t.Errorf("the entry names %s for %s", legalBasis, target)
	}
	var recorded map[string]map[string]any
	if err := json.Unmarshal(changes, &recorded); err != nil {
		t.Fatalf("reading the changes: %v", err)
	}
	if recorded["due_at"]["from"] != "2026-09-09T07:00:00Z" || recorded["due_at"]["to"] != "2026-10-20T21:59:59Z" ||
		recorded["extension_reason"]["to"] != "NUMBER_OF_REQUESTS" || recorded["informed_on"]["to"] != "2026-08-10" {
		t.Errorf("the entry records %s", changes)
	}
	if strings.Contains(string(changes), "folder 7") {
		t.Errorf("the entry carries the notes: %s", changes)
	}

	verified, err := verifierFor(t).Execute(ctx, auditActor(world.tenant), auditrepository.Period{
		From: extensionReceived.Add(-time.Hour), To: extensionReceived.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if !verified.Valid || verified.Checked < 2 {
		t.Errorf("the chain over the extension reads %+v", verified)
	}
}
