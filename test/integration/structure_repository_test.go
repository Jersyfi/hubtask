// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The structure layers against PostgreSQL: the stored shape read back, the keys beside it kept, and
// a cross-tenant negative per repository method (SG-3).

func listLayer() work.StructureLayer {
	return work.StructureLayer{Preset: work.PresetList, Values: work.StructureValues{
		Levels: []work.Level{{Type: work.ItemTask, Name: "Item"}},
		Fields: map[work.ItemType]work.LevelFields{work.ItemTask: {Capabilities: []work.Capability{
			work.CapabilityCompletion, work.CapabilityHistory,
		}}},
		Views:       &work.Views{Offered: []work.ViewLayout{work.ViewListCollapsed}, Default: work.ViewListCollapsed},
		Counts:      &work.Counts{Figure: work.FigureOpen},
		Completion:  work.CompletionManual,
		Capture:     []work.CaptureToken{},
		Celebration: work.CelebrationSubtle,
		Automation:  []work.AutomationBehaviour{},
	}}
}

// seedHubWithCollection writes a hub and one collection under it into the tenant, the collection
// with its own layer.
func seedHubWithCollection(ctx context.Context, t *testing.T, tenant, author shared.ID) (work.Container, work.Container) {
	t.Helper()
	repo := containerRepo()
	hub := containerIn(tenant, author, freshID(t), freshName(t), "a0")
	collection := containerIn(tenant, author, freshID(t), freshName(t), "a0")
	collection.Type, collection.ParentID = work.ContainerCollection, hub.ID
	collection.Structure = listLayer()

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		if _, err := repo.Insert(ctx, hub); err != nil {
			return err
		}
		return insertErr(repo.Insert(ctx, collection))
	}); err != nil {
		t.Fatalf("writing the containers: %v", err)
	}
	return hub, collection
}

func findIn(ctx context.Context, t *testing.T, tenant, id shared.ID) work.Container {
	t.Helper()
	var found work.Container
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		found, err = containerRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the container: %v", err)
	}
	return found
}

// A collection's layer is written with the row and read back as stored - its own completion in
// `completion_policy`, the rest under `structure`.
func TestAContainersLayerIsWrittenWithItAndReadBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantA, authorA)

	stored := findIn(ctx, t, tenantA, collection.ID)
	if stored.Version != 1 {
		t.Errorf("version %d, want 1", stored.Version)
	}
	if stored.Structure.Preset != work.PresetList || len(stored.Structure.Values.Moved(listLayer().Values)) != 0 {
		t.Errorf("layer %+v, moved %v", stored.Structure, stored.Structure.Values.Moved(listLayer().Values))
	}
	if stored.Structure.Values.Levels[0].Name != "Item" {
		t.Errorf("the level's name did not survive: %+v", stored.Structure.Values.Levels)
	}

	var keys []string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT array(SELECT jsonb_object_keys(policies) ORDER BY 1) FROM container WHERE id = $1`,
		collection.ID.String()).Scan(&keys); err != nil {
		t.Fatalf("reading the stored document: %v", err)
	}
	if !slices.Equal(keys, []string{"completion_policy", "structure"}) {
		t.Errorf("the policies document holds %v", keys)
	}
}

// Writing the layer moves the version, keeps every other key, and removes the collection's own
// completion when it is reset - the collection follows its hub again.
func TestAContainersLayerIsRewrittenByKey(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE container SET policies = policies || '{"kept":true}' WHERE id = $1`, collection.ID.String()); err != nil {
		t.Fatalf("seeding another key: %v", err)
	}

	changed := findIn(ctx, t, tenantA, collection.ID)
	changed.Structure.Values = changed.Structure.Values.Without(work.SettingCompletion).Without(work.SettingViews)
	changed.UpdatedAt = created
	structures := postgres.NewStructureRepository()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return structures.SetContainerStructure(ctx, changed, changed.Version)
	}); err != nil {
		t.Fatalf("writing the layer: %v", err)
	}

	stored := findIn(ctx, t, tenantA, collection.ID)
	if stored.Version != changed.Version+1 || stored.Structure.Values.Has(work.SettingCompletion) ||
		stored.Structure.Values.Has(work.SettingViews) || !stored.Structure.Values.Has(work.SettingLevels) {
		t.Errorf("stored %+v", stored)
	}
	var kept bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT coalesce((policies->>'kept')::boolean, false) FROM container WHERE id = $1`,
		collection.ID.String()).Scan(&kept); err != nil || !kept {
		t.Errorf("another key of the document was lost: %v", err)
	}

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return structures.SetContainerStructure(ctx, changed, changed.Version)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version was written: %v", err)
	}
}

// The cross-tenant negatives: another workspace's container cannot be written, held or inherited
// from.
func TestAnotherWorkspacesLayersAreOutOfReach(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	structures := postgres.NewStructureRepository()

	stolen := collection
	stolen.Structure = work.StructureLayer{}
	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return structures.SetContainerStructure(ctx, stolen, stolen.Version)
	})
	if err == nil {
		t.Error("tenant B wrote tenant A's layer")
	}
	if stored := findIn(ctx, t, tenantA, collection.ID); stored.Structure.Preset != work.PresetList {
		t.Errorf("tenant A's layer changed: %+v", stored.Structure)
	}

	err = write(ctx, t, tenantB, func(ctx context.Context) error { return structures.HoldContainer(ctx, hub.ID) })
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant B held tenant A's hub: %v", err)
	}

	var heirs []work.Container
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		heirs, err = structures.Heirs(ctx, hub.ID, 10)
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(heirs) != 0 {
		t.Errorf("tenant B sees %d of tenant A's collections", len(heirs))
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		heirs, err = structures.Heirs(ctx, "", 10_000)
		return err
	}); err != nil {
		t.Fatalf("listing the workspace: %v", err)
	}
	for _, heir := range heirs {
		if heir.TenantID != tenantB {
			t.Fatalf("tenant B's workspace lists tenant A's %s", heir.ID)
		}
	}
}

func TestAHubsHeirsAreItsLiveCollections(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := seedHubWithCollection(ctx, t, tenantA, authorA)
	structures := postgres.NewStructureRepository()

	var heirs []work.Container
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := structures.HoldContainer(ctx, hub.ID); err != nil {
			return err
		}
		var err error
		heirs, err = structures.Heirs(ctx, hub.ID, 10)
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(heirs) != 1 || heirs[0].ID != collection.ID || heirs[0].Structure.Preset != work.PresetList {
		t.Errorf("heirs %+v", heirs)
	}
}

// The workspace's layer: stored by key beside the other settings, under the tenant row's version,
// and invisible to another workspace.
func TestTheWorkspacesLayerIsStoredByKeyAndStaysItsOwn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	structures := postgres.NewStructureRepository()
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE tenant SET settings = settings || '{"week_start":"MONDAY"}' WHERE id = $1`, tenantA.String()); err != nil {
		t.Fatalf("seeding a setting: %v", err)
	}
	// The tenants are the package's: leave no lock behind for the next test to meet.
	t.Cleanup(func() {
		_, _ = adminPool(ctx, t).Exec(context.WithoutCancel(ctx),
			`UPDATE tenant SET settings = settings - 'structure' - 'week_start' WHERE id = $1`, tenantA.String())
	})

	var version int
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		_, version, err = structures.Workspace(ctx, repository.LayerRead)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}

	layer := work.WorkspaceStructure{
		Layer:         listLayer(),
		Locks:         map[work.LockKey]shared.ID{"views": authorA, "fields.TASK.DUE_DATE": authorA},
		DefaultPreset: work.PresetTasks,
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, _, err := structures.Workspace(ctx, repository.LayerExclusive); err != nil {
			return err
		}
		return structures.SetWorkspace(ctx, layer, version, created)
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}

	var stored work.WorkspaceStructure
	var after int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, after, err = structures.Workspace(ctx, repository.LayerShared)
		return err
	}); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if after != version+1 || stored.DefaultPreset != work.PresetTasks || stored.Locks["views"] != authorA ||
		len(stored.Layer.Values.Moved(layer.Layer.Values)) != 0 {
		t.Errorf("stored %+v at version %d", stored, after)
	}
	var weekStart string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT settings->>'week_start' FROM tenant WHERE id = $1`,
		tenantA.String()).Scan(&weekStart); err != nil || weekStart != "MONDAY" {
		t.Errorf("another setting was lost: %q, %v", weekStart, err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return structures.SetWorkspace(ctx, layer, version, created)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version was written: %v", err)
	}

	var theirs work.WorkspaceStructure
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		theirs, _, err = structures.Workspace(ctx, repository.LayerRead)
		return err
	}); err != nil {
		t.Fatalf("reading tenant B: %v", err)
	}
	if theirs.DefaultPreset == work.PresetTasks || len(theirs.Locks) != 0 {
		t.Errorf("tenant B reads tenant A's layer: %+v", theirs)
	}
}
