// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The access export (E-10, Art. 15 and 20). What is under test here is the one thing this export
// changes about a backup: which rows go in. That the archive is a Hubtask archive is E-04's and
// E-05's, and it is that *because* nothing here writes a format of its own.

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

// And it needs the credential that says so, whoever asked for it.
func TestAnInstallationWideExportNeedsTheInstanceScope(t *testing.T) {
	exporter := Exporter{UnitOfWork: &unitOfWork{}}
	request := domain.Request{
		ID: subjectID, Kind: domain.KindAccess, Scope: domain.ScopeInstallation,
		SubjectEmail: "anna@example.org", TargetID: targetID,
	}

	if _, err := exporter.workspaces(context.Background(), actor(), request); err == nil {
		t.Fatal("an installation-wide case ran without the instance scope")
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
type tenantsOf struct{ tenants []shared.ID }

func (t *tenantsOf) SetStatus(context.Context, shared.ID, string, time.Time) (bool, error) {
	return true, nil
}

func (t *tenantsOf) Tenants(context.Context, string) ([]shared.ID, error) { return t.tenants, nil }

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

// An installation-wide case is a loop rather than a wider query: one archive per workspace, each
// under that workspace's own tenant context, with an entry in each.
func TestAnInstallationWideExportCollectsWorkspaceByWorkspace(t *testing.T) {
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	target := &memoryTarget{written: map[string][]byte{}}
	sink, snapshot := &auditSink{}, &snapshotter{}
	subjects := &tenantsOf{tenants: []shared.ID{tenantID, elsewhere}}

	written, err := newExporter(target, subjects, sink, snapshot).
		Export(context.Background(), operator(), domain.Request{
			ID: subjectID, Kind: domain.KindPortability, Scope: domain.ScopeInstallation,
			SubjectEmail: "anna@example.org", TargetID: targetID,
		})
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if written.Tenants != 2 {
		t.Fatalf("the export reached %d workspaces", written.Tenants)
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
