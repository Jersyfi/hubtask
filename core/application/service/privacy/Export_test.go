// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The access export (Art. 15 and 20). What is under test here is the one thing this export
// changes about a backup: which rows go in. That the archive is a Hubtask archive is the backup's
// concern, and it is that *because* nothing here writes a format of its own.

// rowsOfEntity is a source that hands over what a test put in it.
type rowsOfEntity struct {
	rows map[string][]archive.Record
	seen []string
}

func (r *rowsOfEntity) Records(
	_ context.Context, entity archive.Entity, _ time.Time, yield func(archive.Record) error,
) error {
	r.seen = append(r.seen, entity.Table)
	for _, record := range r.rows[entity.Table] {
		if err := yield(record); err != nil {
			return err
		}
	}
	return nil
}

func upsert(id string, data map[string]any) archive.Record {
	return archive.Record{ID: id, Op: archive.OpUpsert, Data: data}
}

func collect(t *testing.T, source archive.Source, table string) []archive.Record {
	t.Helper()

	var kept []archive.Record
	if err := source.Records(t.Context(), archive.Entity{Name: table, Table: table}, time.Time{},
		func(record archive.Record) error {
			kept = append(kept, record)
			return nil
		}); err != nil {
		t.Fatalf("reading %s: %v", table, err)
	}
	return kept
}

func TestOnlyThePersonsRowsGoIntoTheExport(t *testing.T) {
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000f9")
	inner := &rowsOfEntity{rows: map[string][]archive.Record{
		"comment": {
			upsert("c1", map[string]any{"author_id": subjectID.String(), "body": "mine"}),
			upsert("c2", map[string]any{"author_id": other.String(), "body": "somebody else's"}),
		},
		"work_item": {
			upsert("i1", map[string]any{"created_by": other.String(), "assignee_id": subjectID.String()}),
			upsert("i2", map[string]any{"created_by": subjectID.String()}),
			upsert("i3", map[string]any{"created_by": other.String(), "assignee_id": other.String()}),
		},
		"audit_log": {
			upsert("1", map[string]any{"actor_id": subjectID.String()}),
			upsert("2", map[string]any{"actor_id": other.String()}),
		},
	}}
	counted := 0
	source := subjectSource{inner: inner, subject: subjectID, counted: &counted}

	if kept := collect(t, source, "comment"); len(kept) != 1 || kept[0].ID != "c1" {
		t.Errorf("the comments came out as %v", kept)
	}
	// Both ways of being the person's: what they made, and what was given to them.
	if kept := collect(t, source, "work_item"); len(kept) != 2 {
		t.Errorf("the entries came out as %v", kept)
	}
	// Their own audit entries are part of what is held about them.
	if kept := collect(t, source, "audit_log"); len(kept) != 1 {
		t.Errorf("the trail came out as %v", kept)
	}
	if counted != 4 {
		t.Errorf("%d records were counted", counted)
	}
}

// A table with no column naming a person contributes nothing: the workspace's buckets and labels
// are not somebody's personal data, and an export carrying them would hand one member a copy of
// the workspace.
func TestATableThatNamesNobodyContributesNothing(t *testing.T) {
	inner := &rowsOfEntity{rows: map[string][]archive.Record{
		"bucket": {upsert("b1", map[string]any{"name": "Doing"})},
		"label":  {upsert("l1", map[string]any{"name": "urgent"})},
	}}
	source := subjectSource{inner: inner, subject: subjectID}

	for _, table := range []string{"bucket", "label"} {
		if kept := collect(t, source, table); len(kept) != 0 {
			t.Errorf("%s contributed %v", table, kept)
		}
	}
	// And it is not even read: a filter that consumed the whole table to keep nothing would be a
	// pass over the workspace for every export.
	if len(inner.seen) != 0 {
		t.Errorf("the export read %v", inner.seen)
	}
}

// Somebody who wrote in before they ever had an account is named by address rather than by
// identifier.
func TestAPersonWithNoAccountIsFoundByAddress(t *testing.T) {
	inner := &rowsOfEntity{rows: map[string][]archive.Record{
		"jumble_entry": {
			upsert("j1", map[string]any{"sender": "Anna@Example.org"}),
			upsert("j2", map[string]any{"sender": "somebody@example.org"}),
		},
	}}
	source := subjectSource{inner: inner, email: "anna@example.org"}

	kept := collect(t, source, "jumble_entry")
	if len(kept) != 1 || kept[0].ID != "j1" {
		t.Errorf("the entries came out as %v", kept)
	}
}

// A deletion marker travels whatever it names: leaving it out would produce an archive whose
// restore recreated a row this installation deleted.
func TestADeletionMarkerAlwaysTravels(t *testing.T) {
	inner := &rowsOfEntity{rows: map[string][]archive.Record{
		"comment": {{ID: "c9", Op: archive.OpDelete}},
	}}
	source := subjectSource{inner: inner, subject: subjectID}

	if kept := collect(t, source, "comment"); len(kept) != 1 {
		t.Errorf("the deletion marker came out as %v", kept)
	}
}

// The archive is named so that a restore can be pointed at it and a backup listing never picks it
// up by accident.
func TestAnExportIsNamedAsWhatItIs(t *testing.T) {
	name := ArchiveName(subjectID, "", now)

	if got := name[:12]; got != "hubtask-dsr-" {
		t.Errorf("the archive is named %q", name)
	}
	if archivePrefix := archive.Prefix(tenantID); len(name) >= len(archivePrefix) &&
		name[:len(archivePrefix)] == archivePrefix {
		t.Error("an export would appear among the workspace's backups")
	}

	// A person with no account is named by their address, which is the only name there is.
	byEmail := ArchiveName("", "anna@example.org", now)
	if byEmail == name || byEmail[:12] != "hubtask-dsr-" {
		t.Errorf("an export for somebody with no account is named %q", byEmail)
	}
}

// An installation-wide case needs an address: an account identifier belongs to one workspace by
// construction, so it cannot name the person anywhere else.
func TestAnInstallationWideExportNeedsAnAddress(t *testing.T) {
	exporter := Exporter{UnitOfWork: &unitOfWork{}}
	request := domain.Request{
		ID: subjectID, Kind: domain.KindAccess, Scope: domain.ScopeInstallation,
		SubjectAccountID: subjectID, TargetID: targetID,
	}

	if _, err := exporter.workspaces(context.Background(), operator(), request); err == nil {
		t.Fatal("an installation-wide case with no address was accepted")
	}
}

// The job carries the case out as the installation, which holds no token scope: the scope was
// asked of the person who recorded and started the case, and asking the job again refused every
// installation-wide case its own job.
func TestTheJobCarriesOutAnInstallationWideCaseWithoutAScope(t *testing.T) {
	exporter := Exporter{UnitOfWork: &unitOfWork{}, Subjects: &tenantsOf{tenants: []shared.ID{tenantID}}}
	request := domain.Request{
		ID: subjectID, Kind: domain.KindAccess, Scope: domain.ScopeInstallation,
		SubjectEmail: "anna@example.org", TargetID: targetID,
	}
	installation := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantID}

	scopes, err := exporter.workspaces(context.Background(), installation, request)
	if err != nil {
		t.Fatalf("the job was refused: %v", err)
	}
	if len(scopes) != 1 {
		t.Errorf("the case reaches %+v", scopes)
	}
}

// accountsByAddress is the person's account in whichever workspace the read runs in.
type accountsByAddress struct {
	id  shared.ID
	err error
}

func (a accountsByAddress) FindByEmail(context.Context, string) (identity.Account, error) {
	if a.err != nil {
		return identity.Account{}, a.err
	}
	return identity.Account{ID: a.id}, nil
}

// In every workspace the person is known by their account there, so their comments - which name
// an account, not an address - are in the archive; where the account has gone, by the address.
func TestEachWorkspaceKnowsThePersonByTheirAccount(t *testing.T) {
	for _, tc := range []struct {
		name     string
		accounts accountsByAddress
		want     shared.ID
	}{
		{"found", accountsByAddress{id: subjectID}, subjectID},
		{"gone meanwhile", accountsByAddress{err: shared.ErrNotFound}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exporter := Exporter{
				UnitOfWork: &unitOfWork{}, Subjects: &tenantsOf{tenants: []shared.ID{tenantID}},
				Accounts: tc.accounts,
			}
			scopes, err := exporter.workspaces(context.Background(), actor(), domain.Request{
				ID: subjectID, Kind: domain.KindAccess, Scope: domain.ScopeInstallation,
				SubjectEmail: "anna@example.org", TargetID: targetID,
			})
			if err != nil {
				t.Fatalf("resolving the workspaces: %v", err)
			}
			if scopes[0].subjectID != tc.want || scopes[0].email != "anna@example.org" {
				t.Errorf("the person is known as %+v", scopes[0])
			}
		})
	}
}

// An ordinary case reaches this workspace and no other, and asks nobody about tenants.
func TestAnOrdinaryCaseReachesThisWorkspaceAlone(t *testing.T) {
	exporter := Exporter{UnitOfWork: &unitOfWork{}}
	request := domain.Request{
		ID: subjectID, Kind: domain.KindAccess, SubjectAccountID: subjectID, TargetID: targetID,
	}

	scopes, err := exporter.workspaces(context.Background(), actor(), request)
	if err != nil {
		t.Fatalf("resolving the workspaces: %v", err)
	}
	if len(scopes) != 1 || scopes[0].tenantID != tenantID {
		t.Errorf("the case reaches %+v", scopes)
	}
}

// The whole export: an archive per workspace, and the entry each workspace gets about it.

// tenantRows is the generic row reader a backup uses, with one row per table.
type tenantRows struct{ rows map[string][]backuprepo.Row }

func (t *tenantRows) Rows(
	_ context.Context, table string, _ time.Time, yield func(backuprepo.Row) error,
) error {
	for _, row := range t.rows[table] {
		if err := yield(row); err != nil {
			return err
		}
	}
	return nil
}

func (t *tenantRows) Tombstones(
	context.Context, string, time.Time, func(backuprepo.Tombstone) error,
) error {
	return nil
}

func (t *tenantRows) MediaLocation(
	context.Context, string,
) (backuprepo.MediaLocation, error) {
	return backuprepo.MediaLocation{}, shared.ErrNotFound
}

// memoryTarget is a backup target that keeps what it was given.
type memoryTarget struct {
	written map[string][]byte
	opened  []shared.ID
}

func (m *memoryTarget) OpenTarget(
	_ context.Context, tenantID, _ shared.ID,
) (backupstorage.Store, error) {
	m.opened = append(m.opened, tenantID)
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
func (m *memoryTarget) List(context.Context, string) ([]backupstorage.Entry, error) {
	return nil, nil
}
func (m *memoryTarget) Stat(context.Context, string) (backupstorage.Entry, error) {
	return backupstorage.Entry{}, shared.ErrNotFound
}
func (m *memoryTarget) Delete(context.Context, string) error { return nil }

// snapshotter is the read-only snapshot a backup takes, with a time the test can predict.
type snapshotter struct{ scopes []persistence.Scope }

func (s *snapshotter) WithinSnapshot(
	ctx context.Context, scope persistence.Scope, fn func(context.Context, time.Time) error,
) error {
	s.scopes = append(s.scopes, scope)
	return fn(ctx, now)
}

// tenantsOf answers which workspaces an address is a member of.
type tenantsOf struct {
	tenants []shared.ID
	asked   int
}

func (t *tenantsOf) SetStatus(context.Context, shared.ID, string, time.Time) (bool, error) {
	return true, nil
}

func (t *tenantsOf) Tenants(context.Context, string) ([]shared.ID, error) {
	t.asked++
	return t.tenants, nil
}

func newExporter(target *memoryTarget, subjects *tenantsOf, sink *auditSink, snapshot *snapshotter) Exporter {
	return Exporter{
		Requests: newRequestStore(), Subjects: subjects, Targets: target,
		Rows: &tenantRows{rows: map[string][]backuprepo.Row{
			"comment": {{ID: "c1", Data: map[string]any{"author_id": subjectID.String()}}},
			"bucket":  {{ID: "b1", Data: map[string]any{"name": "Doing"}}},
		}},
		Audit: sink, Snapshot: snapshot, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), IDs: &idSource{},
		SchemaVersion: "44", ProductVersion: "0.4.5",
	}
}

func TestAnExportWritesOneArchiveAndRecordsIt(t *testing.T) {
	target := &memoryTarget{written: map[string][]byte{}}
	sink, snapshot := &auditSink{}, &snapshotter{}

	written, err := newExporter(target, &tenantsOf{}, sink, snapshot).
		Export(context.Background(), actor(), domain.Request{
			ID: subjectID, Kind: domain.KindAccess, SubjectAccountID: subjectID, TargetID: targetID,
		})
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}

	if written.Tenants != 1 || written.Archive == "" {
		t.Errorf("the export produced %+v", written)
	}
	// One record: the person's comment, and not the workspace's bucket.
	if written.Records != 1 {
		t.Errorf("%d records went in", written.Records)
	}
	// It is a Hubtask archive: the members are the format's, because nothing here writes a format
	// of its own.
	var manifest, checksums bool
	for key := range target.written {
		manifest = manifest || strings.HasSuffix(key, archive.ManifestName)
		checksums = checksums || strings.HasSuffix(key, archive.ChecksumsName)
	}
	if !manifest || !checksums {
		t.Errorf("the archive holds %v", target.written)
	}

	entry := sink.entries[len(sink.entries)-1]
	if entry.Action != ExportedAction || entry.LegalBasis != "dsr.access" {
		t.Errorf("the export was recorded as %s / %q", entry.Action, entry.LegalBasis)
	}
}

// An installation-wide case is a loop rather than a wider query, each workspace read under its own
// tenant context, with an entry in each - and what it writes is one archive (UC-PRV-06 check 3): one
// object whose members are the workspaces' archives, named by the case and not the person.
func TestAnInstallationWideExportCollectsWorkspaceByWorkspace(t *testing.T) {
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	target := &memoryTarget{written: map[string][]byte{}}
	sink, snapshot := &auditSink{}, &snapshotter{}
	subjects := &tenantsOf{tenants: []shared.ID{tenantID, elsewhere}}
	exporter := newExporter(target, subjects, sink, snapshot)
	exporter.Accounts = accountsByAddress{id: subjectID}

	written, err := exporter.Export(context.Background(), actor(), domain.Request{
		ID: subjectID, Kind: domain.KindPortability, Scope: domain.ScopeInstallation,
		SubjectEmail: "anna@example.org", TargetID: targetID,
	})
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if written.Tenants != 2 {
		t.Fatalf("the export reached %d workspaces", written.Tenants)
	}
	// The person's comment in each workspace, found by their account there.
	if written.Records != 2 {
		t.Errorf("%d records went in, want the comment from each workspace", written.Records)
	}

	if len(target.written) != 1 {
		t.Fatalf("the target holds %d objects, want one archive", len(target.written))
	}
	body, found := target.written[written.Archive]
	if !found || !strings.HasSuffix(written.Archive, ".zip") ||
		!strings.Contains(written.Archive, subjectID.String()) || strings.Contains(written.Archive, "anna") {
		t.Fatalf("the case names %q, the target holds %v", written.Archive, keysOf(target.written))
	}
	bundled, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	manifests := map[string]bool{}
	for _, member := range bundled.File {
		if strings.HasSuffix(member.Name, "/"+archive.ManifestName) {
			manifests[strings.TrimSuffix(member.Name, "/"+archive.ManifestName)] = true
		}
	}
	if !manifests[tenantID.String()] || !manifests[elsewhere.String()] || len(manifests) != 2 {
		t.Errorf("the archive holds the workspaces %v", manifests)
	}

	// Each archive is written under its own workspace's context - never one query across tenants.
	if len(snapshot.scopes) != 2 ||
		snapshot.scopes[0].TenantID != tenantID || snapshot.scopes[1].TenantID != elsewhere {
		t.Errorf("the snapshots were taken in %+v", snapshot.scopes)
	}

	// And every workspace it touched has the entry audit.md §5 asks for: the occasion is
	// documented where its own administrator can see it.
	byTenant := map[shared.ID]audit.Action{}
	for _, entry := range sink.entries {
		byTenant[entry.TenantID] = entry.Action
	}
	if byTenant[tenantID] != ExportedAction {
		t.Errorf("the caller's own workspace recorded %s", byTenant[tenantID])
	}
	if byTenant[elsewhere] != CollectedAction {
		t.Errorf("the other workspace recorded %s", byTenant[elsewhere])
	}
	for _, entry := range sink.entries {
		if !strings.Contains(fmt.Sprint(entry.Changes), written.Archive) {
			t.Errorf("the entry in %s does not name the archive: %v", entry.TenantID, entry.Changes)
		}
	}
}

// One workspace's failure fails the whole archive: the target keeps no half of it.
func TestAnArchiveThatFailsHalfwayLeavesNothing(t *testing.T) {
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	target := &memoryTarget{written: map[string][]byte{}}
	exporter := newExporter(target, &tenantsOf{tenants: []shared.ID{tenantID, elsewhere}},
		&auditSink{}, &snapshotter{})
	exporter.Snapshot = &failingSnapshot{after: 1}

	_, err := exporter.Export(context.Background(), actor(), domain.Request{
		ID: subjectID, Kind: domain.KindAccess, Scope: domain.ScopeInstallation,
		SubjectEmail: "anna@example.org", TargetID: targetID,
	})
	if err == nil {
		t.Fatal("an export whose second workspace failed succeeded")
	}
	if len(target.written) != 0 {
		t.Errorf("the target kept %v", keysOf(target.written))
	}
}

// failingSnapshot lets the first snapshots through and fails the next one.
type failingSnapshot struct{ after, taken int }

func (f *failingSnapshot) WithinSnapshot(
	ctx context.Context, _ persistence.Scope, fn func(context.Context, time.Time) error,
) error {
	f.taken++
	if f.taken > f.after {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed")
	}
	return fn(ctx, now)
}

func keysOf(written map[string][]byte) []string {
	keys := make([]string, 0, len(written))
	for key := range written {
		keys = append(keys, key)
	}
	return keys
}

// A case with no target is refused before anything is opened: an export has to be put somewhere.
func TestAnExportWithNoTargetIsRefused(t *testing.T) {
	target := &memoryTarget{written: map[string][]byte{}}

	_, err := newExporter(target, &tenantsOf{}, &auditSink{}, &snapshotter{}).
		Export(context.Background(), actor(), domain.Request{
			ID: subjectID, Kind: domain.KindAccess, SubjectAccountID: subjectID,
		})
	if err == nil {
		t.Fatal("an export with nowhere to write was accepted")
	}
	if len(target.opened) != 0 {
		t.Error("a target was opened for an export that could not be written")
	}
}
