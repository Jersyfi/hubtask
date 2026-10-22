// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A collection's figure against PostgreSQL as the application role (row level security applies):
// every expected number is counted by hand from the rows below, never by the statement under test.
// The edge rows are UX-24's record § 4.

// entryRow is one row to seed; zero values are the column defaults.
type entryRow struct {
	itemType    work.ItemType
	parent      *seededEntry
	completedAt string // RFC 3339, completed when set
	dueAt       string // RFC 3339
	dateOnly    bool
	dueZone     string
	archived    bool
	trashed     bool
	fields      map[string]any
	refs        map[string]string
}

type seededEntry struct {
	id    shared.ID
	path  string
	depth int
}

// seedEntry writes one entry as the database holds it. A due date is written as already announced:
// the package shares one database, and the due-date scan's tests count what is owed per tenant.
func seedEntry(ctx context.Context, t *testing.T, tenant, collection shared.ID, row entryRow) *seededEntry {
	t.Helper()
	id := freshID(t)
	entry := &seededEntry{id: id, path: "/" + id.String() + "/", depth: 1}
	var parent any
	if row.parent != nil {
		parent = row.parent.id.String()
		entry.path = row.parent.path + id.String() + "/"
		entry.depth = row.parent.depth + 1
	}
	itemType := row.itemType
	if itemType == "" {
		itemType = work.ItemTask
	}
	nullable := func(value string) any {
		if value == "" {
			return nil
		}
		return value
	}
	fields, _ := json.Marshal(row.fields)
	if row.fields == nil {
		fields = []byte("{}")
	}
	refs, _ := json.Marshal(row.refs)
	if row.refs == nil {
		refs = []byte("{}")
	}
	author := authorA
	if tenant == tenantB {
		author = authorB
	}
	var completedBy any
	if row.completedAt != "" {
		completedBy = author.String()
	}
	stamp := func(on bool) any {
		if on {
			return "2026-10-20T08:00:00Z"
		}
		return nil
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO work_item (id, tenant_id, collection_id, type, parent_id, path, depth, title, order_key,
		                       created_by, is_completed, completed_at, completed_by, due_at, due_date_only,
		                       due_time_zone, archived_at, deleted_at, custom_fields, custom_field_refs,
		                       due_soon_announced_at, overdue_announced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'Counted', 'a0', $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
		        $12, $12)`,
		id.String(), tenant.String(), collection.String(), string(itemType), parent, entry.path, entry.depth,
		author.String(), row.completedAt != "", nullable(row.completedAt), completedBy, nullable(row.dueAt),
		row.dateOnly, nullable(row.dueZone), stamp(row.archived), stamp(row.trashed), string(fields), string(refs),
	); err != nil {
		t.Fatalf("seeding an entry: %v", err)
	}
	return entry
}

func seedNumberField(ctx context.Context, t *testing.T, tenant, collection shared.ID, key string, deleted bool) shared.ID {
	t.Helper()
	id := freshID(t)
	var deletedAt any
	if deleted {
		deletedAt = "2026-10-01T08:00:00Z"
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO custom_field_definition (id, tenant_id, collection_id, key, kind, deleted_at)
		VALUES ($1, $2, $3, $4, 'NUMBER', $5)`,
		id.String(), tenant.String(), collection.String(), key, deletedAt); err != nil {
		t.Fatalf("seeding a field: %v", err)
	}
	return id
}

func countFigures(
	ctx context.Context, t *testing.T, tenant shared.ID, window work.FigureWindow, requests ...work.FigureRequest,
) map[shared.ID]work.FigureCounts {
	t.Helper()
	var counted map[shared.ID]work.FigureCounts
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		counted, err = postgres.NewFigureRepository().Count(ctx, requests, window)
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return counted
}

// Thursday 22 October 2026, 12:00 in Berlin; the week began on Monday the 19th.
func berlinWindow(t *testing.T) work.FigureWindow {
	t.Helper()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return work.NewFigureWindow(time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC), berlin, "MONDAY")
}

var allLevels = []work.ItemType{work.ItemTask, work.ItemWorkPackage, work.ItemActivity}

func TestOpenEntriesAndTheirSumAreCounted(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	_, bare := seedHubWithCollection(ctx, t, tenantA, authorA)
	old := seedNumberField(ctx, t, tenantA, collection.ID, "amount", true)
	live := seedNumberField(ctx, t, tenantA, collection.ID, "amount", false)

	in := func(row entryRow) *seededEntry { return seedEntry(ctx, t, tenantA, collection.ID, row) }
	ref := map[string]string{"amount": live.String()}
	in(entryRow{fields: map[string]any{"amount": 2.5}, refs: ref})
	first := in(entryRow{fields: map[string]any{"amount": 3}, refs: ref})
	in(entryRow{fields: map[string]any{"amount": 0}, refs: ref})                                         // 0 is a value
	in(entryRow{fields: map[string]any{"amount": 100}, refs: map[string]string{"amount": old.String()}}) // the old definition's
	in(entryRow{})                                                                                       // open, no value
	in(entryRow{archived: true, fields: map[string]any{"amount": 7}, refs: ref})                         // put aside
	in(entryRow{completedAt: "2026-10-21T08:00:00Z", fields: map[string]any{"amount": 7}, refs: ref})
	in(entryRow{trashed: true, fields: map[string]any{"amount": 7}, refs: ref})
	in(entryRow{itemType: work.ItemWorkPackage, parent: first}) // a part of a row, not a row

	window := berlinWindow(t)
	counted := countFigures(ctx, t, tenantA, window,
		work.FigureRequest{CollectionID: collection.ID, Kind: work.FigureFieldSum, Offered: allLevels, FieldKey: "amount"},
		work.FigureRequest{CollectionID: bare.ID, Kind: work.FigureFieldSum, Offered: allLevels, FieldKey: "amount"},
	)
	got := counted[collection.ID]
	if got.Open != 5 || got.Summed != 3 || got.Sum == nil || *got.Sum != "5.5" {
		t.Errorf("open %d, summed %d, sum %v", got.Open, got.Summed, got.Sum)
	}
	if none := counted[bare.ID]; none.Sum != nil || none.Open != 0 {
		t.Errorf("a collection without the field: %+v", none)
	}

	// The level carries no custom fields: the request names no key, and the sum is unknown.
	unkeyed := countFigures(ctx, t, tenantA, window,
		work.FigureRequest{CollectionID: collection.ID, Kind: work.FigureFieldSum, Offered: allLevels})
	if got := unkeyed[collection.ID]; got.Sum != nil || got.Open != 5 {
		t.Errorf("without a key: %+v", got)
	}
}

func TestWhatIsDueByTheEndOfTheReadersDayIsCounted(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	in := func(row entryRow) *seededEntry { return seedEntry(ctx, t, tenantA, collection.ID, row) }

	in(entryRow{dueAt: "2026-10-22T21:59:00Z"})                                                 // 23:59 tonight in Berlin: due
	in(entryRow{dueAt: "2026-10-22T22:00:00Z"})                                                 // midnight: tomorrow
	in(entryRow{dueAt: "2026-10-20T08:00:00Z"})                                                 // earlier: still due
	in(entryRow{dueAt: "2026-10-21T11:00:00Z", dateOnly: true, dueZone: "Pacific/Auckland"})    // 22 Oct in Auckland: due
	in(entryRow{dueAt: "2026-10-23T07:00:00Z", dateOnly: true, dueZone: "America/Los_Angeles"}) // 23 Oct there: not yet
	in(entryRow{dueAt: "2026-10-22T00:00:00Z", dateOnly: true})                                 // 22 Oct, no zone: UTC, due
	in(entryRow{dueAt: "2026-10-22T08:00:00Z", completedAt: "2026-10-22T09:00:00Z"})            // done
	in(entryRow{dueAt: "2026-10-22T08:00:00Z", archived: true})                                 // put aside
	in(entryRow{dueAt: "2026-10-22T08:00:00Z", trashed: true})                                  // trashed
	task := in(entryRow{})
	in(entryRow{itemType: work.ItemActivity, parent: task, dueAt: "2026-10-22T08:00:00Z"}) // a step due today

	window := berlinWindow(t)
	counted := countFigures(ctx, t, tenantA, window,
		work.FigureRequest{CollectionID: collection.ID, Kind: work.FigureDue, Offered: allLevels, Dated: allLevels})
	if got := counted[collection.ID].Due; got != 5 {
		t.Errorf("due %d, want 5", got)
	}

	// A level whose fields carry no due date: its dates do not count.
	undated := countFigures(ctx, t, tenantA, window, work.FigureRequest{
		CollectionID: collection.ID, Kind: work.FigureDue, Offered: allLevels,
		Dated: []work.ItemType{work.ItemTask, work.ItemWorkPackage},
	})
	if got := undated[collection.ID].Due; got != 4 {
		t.Errorf("due without the activity's dates %d, want 4", got)
	}
}

func TestTheWeeksRhythmCountsDaysWithACompletion(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	in := func(row entryRow) *seededEntry { return seedEntry(ctx, t, tenantA, collection.ID, row) }

	in(entryRow{completedAt: "2026-10-18T20:00:00Z"})                 // Sunday 22:00 in Berlin: last week
	in(entryRow{completedAt: "2026-10-18T23:30:00Z"})                 // Monday 01:30 in Berlin
	in(entryRow{completedAt: "2026-10-21T08:00:00Z"})                 // Wednesday
	in(entryRow{completedAt: "2026-10-21T15:00:00Z"})                 // Wednesday again, somebody else
	in(entryRow{completedAt: "2026-10-20T08:00:00Z", archived: true}) // Tuesday, put away since
	in(entryRow{completedAt: "2026-10-22T09:00:00Z", trashed: true})  // trashed
	in(entryRow{})                                                    // open, or reopened

	counted := countFigures(ctx, t, tenantA, berlinWindow(t),
		work.FigureRequest{CollectionID: collection.ID, Kind: work.FigureRhythm, Offered: allLevels})
	want := []string{"2026-10-19", "2026-10-20", "2026-10-21"}
	got := slices.Clone(counted[collection.ID].Days)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("days %v, want %v", got, want)
	}

	sunday := work.NewFigureWindow(time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC), berlinWindow(t).Location, "SUNDAY")
	counted = countFigures(ctx, t, tenantA, sunday,
		work.FigureRequest{CollectionID: collection.ID, Kind: work.FigureRhythm, Offered: allLevels})
	got = slices.Clone(counted[collection.ID].Days)
	slices.Sort(got)
	if !slices.Equal(got, []string{"2026-10-18", "2026-10-19", "2026-10-20", "2026-10-21"}) {
		t.Errorf("a week from Sunday: %v", got)
	}
}

func TestProgressCountsLeavesAsTheyStand(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	in := func(row entryRow) *seededEntry { return seedEntry(ctx, t, tenantA, collection.ID, row) }

	project := in(entryRow{})
	electrics := in(entryRow{itemType: work.ItemWorkPackage, parent: project})
	in(entryRow{itemType: work.ItemActivity, parent: electrics, completedAt: "2026-10-20T08:00:00Z"})
	in(entryRow{itemType: work.ItemActivity, parent: electrics})
	in(entryRow{completedAt: "2026-10-19T08:00:00Z", archived: true}) // done and put away: still done
	in(entryRow{})
	in(entryRow{completedAt: "2026-10-19T08:00:00Z", trashed: true})
	trashedParent := in(entryRow{})
	in(entryRow{itemType: work.ItemWorkPackage, parent: trashedParent, trashed: true}) // its only child is trashed

	cases := []struct {
		name        string
		offered     []work.ItemType
		done, total int
	}{
		// Leaves: two activities, the archived task, the open task, the task whose child is trashed.
		{"every level", allLevels, 2, 5},
		// Activities hidden: the work package is a leaf now.
		{"without activities", []work.ItemType{work.ItemTask, work.ItemWorkPackage}, 1, 4},
		// Work packages hidden: the activities under one are hidden with it, and its task is a leaf.
		{"an activity under a hidden work package", []work.ItemType{work.ItemTask, work.ItemActivity}, 1, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counted := countFigures(ctx, t, tenantA, berlinWindow(t),
				work.FigureRequest{CollectionID: collection.ID, Kind: work.FigureProgress, Offered: tc.offered})
			if got := counted[collection.ID]; got.Done != tc.done || got.Total != tc.total {
				t.Errorf("done %d of %d, want %d of %d", got.Done, got.Total, tc.done, tc.total)
			}
		})
	}
}

// SG-3: another workspace's collection counts nothing, whatever its identifier.
func TestAnotherWorkspacesCollectionCountsNothing(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	seedEntry(ctx, t, tenantA, collection.ID, entryRow{completedAt: "2026-10-21T08:00:00Z"})
	seedEntry(ctx, t, tenantA, collection.ID, entryRow{dueAt: "2026-10-21T08:00:00Z"})

	requests := []work.FigureRequest{}
	for _, kind := range []work.Figure{work.FigureOpen, work.FigureDue, work.FigureRhythm, work.FigureProgress} {
		requests = append(requests, work.FigureRequest{
			CollectionID: collection.ID, Kind: kind, Offered: allLevels, Dated: allLevels,
		})
	}
	counted := countFigures(ctx, t, tenantB, berlinWindow(t), requests...)
	if got := counted[collection.ID]; got.Open != 0 || got.Due != 0 || len(got.Days) != 0 || got.Total != 0 || got.Sum != nil {
		t.Errorf("tenant B counted %+v in tenant A's collection", got)
	}
}

// The statement leaves out exactly the entries the entry gate marks hidden: for every combination
// of levels, under the installation's bound and under one that no longer holds the activity, the
// due count over every chain the matrix allows equals the entries Offered.Hidden shows.
func TestTheFigureLeavesOutWhatTheEntryGateHides(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	due := "2026-10-22T08:00:00Z"
	in := func(itemType work.ItemType, parent *seededEntry) *seededEntry {
		return seedEntry(ctx, t, tenantA, collection.ID, entryRow{itemType: itemType, parent: parent, dueAt: due})
	}
	type seeded struct {
		itemType  work.ItemType
		ancestors []work.ItemType
	}
	var entries []seeded
	add := func(itemType work.ItemType, parent *seededEntry, ancestors ...work.ItemType) *seededEntry {
		entries = append(entries, seeded{itemType, ancestors})
		return in(itemType, parent)
	}
	task := add(work.ItemTask, nil)
	pack := add(work.ItemWorkPackage, task, work.ItemTask)
	add(work.ItemActivity, pack, work.ItemWorkPackage, work.ItemTask)
	add(work.ItemActivity, task, work.ItemTask)

	var inForce, system []work.CapabilityProfile
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		profiles := postgres.NewCapabilityProfileRepository()
		var err error
		if inForce, err = profiles.List(ctx); err != nil {
			return err
		}
		system, err = profiles.ListSystem(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the bound: %v", err)
	}
	var withoutActivities []work.CapabilityProfile
	for _, profile := range inForce {
		if profile.Type == work.ItemActivity {
			continue
		}
		profile.AllowedChildTypes = slices.DeleteFunc(slices.Clone(profile.AllowedChildTypes),
			func(child work.ItemType) bool { return child == work.ItemActivity })
		withoutActivities = append(withoutActivities, profile)
	}
	bounds := map[string]work.StructureBound{
		"the installation's bound":     work.NewStructureBound(inForce, system),
		"a bound without the activity": work.NewStructureBound(withoutActivities, system),
	}

	window := berlinWindow(t)
	for name, bound := range bounds {
		for mask := 1; mask < 8; mask++ {
			var levels []work.Level
			for i, itemType := range work.ItemTypes() {
				if mask&(1<<i) != 0 {
					levels = append(levels, work.Level{Type: itemType})
				}
			}
			offered := work.NewOffered(bound, work.EffectiveStructure{Values: work.StructureValues{Levels: levels}})
			want := 0
			for _, entry := range entries {
				if !offered.Hidden(entry.itemType, entry.ancestors).Level {
					want++
				}
			}
			request := work.FigureRequest{CollectionID: collection.ID, Kind: work.FigureDue, Dated: work.ItemTypes()}
			counted := countFigures(ctx, t, tenantA, window, request.Offer(offered))
			if got := counted[collection.ID].Due; got != want {
				t.Errorf("%s, levels %v: the statement counts %d, the entry gate shows %d", name, levels, got, want)
			}
		}
	}
}
