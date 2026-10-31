// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Kept figures against PostgreSQL as the application role (migration 0292, UC-WRK-29): a kept
// count answers while the collection is unchanged, every write in the workspace's context moves
// the collection on in its own transaction, nothing crosses a workspace, and the recount repairs a
// count that a write outside the workspace's context left wrong. Tenant B, for figureReader's reason.

var keptMoment = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)

func openStructure() work.EffectiveStructure {
	return work.EffectiveStructure{Values: work.StructureValues{
		Counts: &work.Counts{Figure: work.FigureOpen},
		Levels: []work.Level{{Type: work.ItemTask}},
		Fields: map[work.ItemType]work.LevelFields{
			work.ItemTask: {Capabilities: []work.Capability{work.CapabilityCompletion}},
		},
	}}
}

func keptFigureReader(ctx context.Context, t *testing.T) workservice.FigureReader {
	t.Helper()
	return workservice.FigureReader{
		Figures: postgres.NewFigureRepository(), Clock: portclock.Fixed(keptMoment),
		Profiles: postgres.NewCapabilityProfileRepository(), Kept: postgres.NewKeptFigureRepository(),
	}
}

func openFigure(ctx context.Context, t *testing.T, collection work.Container) work.CollectionFigure {
	t.Helper()
	figures := keptFigureReader(ctx, t).Count(ctx, figureReader(), postgres.NewUnitOfWork(appPool(ctx, t)),
		[]work.Container{collection}, map[shared.ID]work.EffectiveStructure{collection.ID: openStructure()})
	figure, counted := figures[collection.ID]
	if !counted || figure.Unavailable {
		t.Fatalf("no figure: %+v", figures)
	}
	return figure
}

// asWorkspace runs one statement in the workspace's context, as the transaction wrapper sets it.
func asWorkspace(ctx context.Context, t *testing.T, tenant shared.ID, sql string, args ...any) {
	t.Helper()
	tx, err := appPool(ctx, t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenant.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func keptRows(ctx context.Context, t *testing.T, collection shared.ID) (rows int, open int) {
	t.Helper()
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT count(*), coalesce(max((counts->>'open')::int), -1)
		  FROM collection_figure WHERE collection_id = $1`, collection.String()).Scan(&rows, &open); err != nil {
		t.Fatal(err)
	}
	return rows, open
}

func keptState(ctx context.Context, t *testing.T, tenant shared.ID, key repository.KeptFigureKey) repository.KeptFigureState {
	t.Helper()
	var states map[repository.KeptFigureKey]repository.KeptFigureState
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		states, err = postgres.NewKeptFigureRepository().States(ctx, []repository.KeptFigureKey{key})
		return err
	}); err != nil {
		t.Fatalf("reading the state: %v", err)
	}
	return states[key]
}

func TestAKeptFigureMovesWithEveryWriteInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantB, authorB)
	first := seedEntry(ctx, t, tenantB, collection.ID, entryRow{})
	seedEntry(ctx, t, tenantB, collection.ID, entryRow{})
	seedEntry(ctx, t, tenantB, collection.ID, entryRow{})

	if got := openFigure(ctx, t, collection).Counts.Open; got != 3 {
		t.Fatalf("open %d, want 3", got)
	}
	if rows, open := keptRows(ctx, t, collection.ID); rows != 1 || open != 3 {
		t.Fatalf("kept %d rows, open %d", rows, open)
	}
	key := repository.KeptFigureKey{
		CollectionID: collection.ID, Kind: work.FigureOpen,
		BasisKey: work.FigureRequest{Kind: work.FigureOpen, Offered: []work.ItemType{work.ItemTask}}.
			Basis(work.FigureWindow{}).Key(),
	}
	before := keptState(ctx, t, tenantB, key)
	if !before.Kept.Present || before.Kept.Changes != before.Changes {
		t.Fatalf("the kept count does not answer: %+v", before)
	}

	// An entry completed in the workspace's context: the same transaction moves the collection on.
	asWorkspace(ctx, t, tenantB, `UPDATE work_item SET is_completed = true, completed_at = now() WHERE id = $1`,
		first.id.String())
	after := keptState(ctx, t, tenantB, key)
	if after.Changes <= before.Changes {
		t.Fatalf("changes %d after %d", after.Changes, before.Changes)
	}
	if got := openFigure(ctx, t, collection).Counts.Open; got != 2 {
		t.Fatalf("after the completion open %d, want 2", got)
	}

	// A workspace-wide definition is every collection's change.
	moved := keptState(ctx, t, tenantB, key).Changes
	asWorkspace(ctx, t, tenantB, `
		INSERT INTO custom_field_definition (id, tenant_id, collection_id, key, kind)
		VALUES ($1, $2, NULL, 'kept_amount', 'NUMBER')`, freshID(t).String(), tenantB.String())
	if got := keptState(ctx, t, tenantB, key).Changes; got <= moved {
		t.Fatalf("a workspace-wide definition left changes at %d", got)
	}

	// The fold moves no number.
	settled := keptState(ctx, t, tenantB, key).Changes
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := postgres.NewKeptFigureRepository().Fold(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := keptState(ctx, t, tenantB, key).Changes; got != settled {
		t.Fatalf("the fold moved changes from %d to %d", settled, got)
	}

	// Another workspace sees neither the kept count nor the changes, and its passes leave them.
	if foreign := keptState(ctx, t, tenantA, key); foreign.Kept.Present || foreign.Changes != 0 {
		t.Fatalf("tenant A sees %+v", foreign)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		kept := postgres.NewKeptFigureRepository()
		if _, err := kept.Fold(ctx); err != nil {
			return err
		}
		_, err := kept.Prune(ctx, keptMoment.Add(time.Hour))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if rows, _ := keptRows(ctx, t, collection.ID); rows != 1 {
		t.Fatalf("another workspace's prune took the kept count: %d rows", rows)
	}
}

func TestTheRecountRepairsAFigureADriftLeftWrong(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := seedHubWithCollection(ctx, t, tenantB, authorB)
	seedEntry(ctx, t, tenantB, collection.ID, entryRow{})
	seedEntry(ctx, t, tenantB, collection.ID, entryRow{})

	if got := openFigure(ctx, t, collection).Counts.Open; got != 2 {
		t.Fatalf("open %d, want 2", got)
	}
	// Written by a role without the workspace's context, as a repair by hand would be: the
	// collection's number of changes does not move, and the kept count is now wrong.
	seedEntry(ctx, t, tenantB, collection.ID, entryRow{})
	if got := openFigure(ctx, t, collection).Counts.Open; got != 2 {
		t.Fatalf("the drift is not there to repair: open %d", got)
	}

	recount := workservice.RecountFigures{
		Kept: postgres.NewKeptFigureRepository(), Figures: postgres.NewFigureRepository(),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: portclock.Fixed(keptMoment.Add(10 * time.Minute)),
	}
	system := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantB}
	outcome, err := recount.Execute(ctx, system)
	if err != nil {
		t.Fatalf("recounting: %v", err)
	}
	if outcome.Repaired < 1 {
		t.Fatalf("outcome %+v", outcome)
	}
	if rows, open := keptRows(ctx, t, collection.ID); rows != 1 || open != 3 {
		t.Fatalf("after the recount kept %d rows, open %d", rows, open)
	}
	if got := openFigure(ctx, t, collection).Counts.Open; got != 3 {
		t.Fatalf("after the recount open %d, want 3", got)
	}

	// A second pass finds nothing to repair.
	again, err := recount.Execute(ctx, system)
	if err != nil {
		t.Fatal(err)
	}
	if again.Repaired != 0 {
		t.Fatalf("the second pass repaired %d", again.Repaired)
	}
}
