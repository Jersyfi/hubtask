// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type changeLog struct{ changes []changelog.Change }

func (l *changeLog) Record(_ context.Context, change changelog.Change) error {
	l.changes = append(l.changes, change)
	return nil
}

type hlcs struct{ reading shared.HLC }

func (h *hlcs) Next() shared.HLC {
	next, _ := h.reading.Tick(start, "server")
	h.reading = next
	return next
}

func (l *changeLog) of(entity string, id shared.ID) []changelog.Change {
	var out []changelog.Change
	for _, c := range l.changes {
		if c.Entity == entity && c.EntityID == id {
			out = append(out, c)
		}
	}
	return out
}

// A hub's inventory is read offline (UC-AST-01 check 10, offline-sync.md §13): a CI made, changed
// and given compliance fields, and a relation made and removed, each tell the change log what now
// stands, filed under the hub; a merge tells it of the retired duplicate and of the relations it
// handed over.
func TestEveryWriteOfACIReachesTheChangeLog(t *testing.T) {
	f := newFixture()
	log := &changeLog{}
	f.inv.Changes, f.inv.HLC = log, &hlcs{}
	mk := func(class, name string) shared.ID {
		out, err := call(t, CreateCi{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "class": class, "name": name})
		if err != nil {
			t.Fatal(err)
		}
		return shared.MustParseID(out["id"].(string))
	}
	app, vm := mk("application_service", "Shop"), mk("virtual_machine", "web1")
	if created := log.of(CIEntity, app); len(created) != 1 || created[0].ContainerID != hub ||
		created[0].Op != changelog.Upsert || created[0].Payload["name"] != "Shop" {
		t.Fatalf("the creation was recorded as %+v", created)
	}
	if _, err := call(t, SetCiCompliance{f.inv}.Descriptor(), as(anna), usecase.Input{"ci_id": app.String(), "quantity": 3}); err != nil {
		t.Fatal(err)
	}
	if changed := log.of(CIEntity, app); len(changed) != 2 || changed[1].Payload["quantity"] != 3 || changed[1].Payload["version"] != 2 {
		t.Fatalf("the compliance change was recorded as %+v", changed)
	}

	rel, err := call(t, RelateCis{f.inv}.Descriptor(), as(anna), usecase.Input{"source_ci_id": app.String(), "target_ci_id": vm.String(), "type": "runs_on"})
	if err != nil {
		t.Fatal(err)
	}
	relID := shared.MustParseID(rel["id"].(string))
	if put := log.of(RelationEntity, relID); len(put) != 1 || put[0].ContainerID != hub || put[0].Payload["source_ci_id"] != app.String() {
		t.Fatalf("the relation was recorded as %+v", put)
	}
	if _, err := call(t, UnrelateCis{f.inv}.Descriptor(), as(anna), usecase.Input{"relation_id": relID.String()}); err != nil {
		t.Fatal(err)
	}
	if gone := log.of(RelationEntity, relID); len(gone) != 2 || gone[1].Op != changelog.Delete || gone[1].Payload != nil || gone[1].ContainerID != hub {
		t.Fatalf("the removal was recorded as %+v", gone)
	}
}

// A merge retires the duplicate and moves its relations to the survivor; the change log is told
// of the retired CI, of each moved relation as gone and of the survivor's as they now stand.
func TestAMergeTellsTheChangeLogWhatMoved(t *testing.T) {
	f := newFixture()
	log := &changeLog{}
	f.inv.Changes, f.inv.HLC = log, &hlcs{}
	var ids []shared.ID
	for _, name := range []string{"one", "two"} {
		out, err := call(t, CreateCi{f.inv}.Descriptor(), as(anna), usecase.Input{
			"hub_id": hub.String(), "class": "server", "name": name, "attributes": map[string]any{"asset_tag": "T-" + name},
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, shared.MustParseID(out["id"].(string)))
	}
	app, err := call(t, CreateCi{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "class": "application_service", "name": "Shop"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		f.m.ids[id]["mac_address"] = "aa:bb"
	}
	if _, err := call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "source": "netbox", "items": items(
		map[string]any{"class": "server", "name": "x", "attributes": map[string]any{"mac_address": "aa:bb"}})}); err != nil {
		t.Fatal(err)
	}
	dup := f.m.dups[0]
	survivor, other := dup.CIs[0], dup.CIs[1]
	rel, err := call(t, RelateCis{f.inv}.Descriptor(), as(anna), usecase.Input{"source_ci_id": app["id"], "target_ci_id": other.String(), "type": "depends_on"})
	if err != nil {
		t.Fatal(err)
	}
	relID := shared.MustParseID(rel["id"].(string))
	if _, err := call(t, ResolveCiDuplicate{f.inv}.Descriptor(), as(anna), usecase.Input{"duplicate_id": dup.ID.String(), "survivor_id": survivor.String()}); err != nil {
		t.Fatal(err)
	}
	retired := log.of(CIEntity, other)
	if last := retired[len(retired)-1]; last.Payload["merged_into"] != survivor.String() {
		t.Fatalf("the duplicate was last recorded as %+v", last.Payload)
	}
	moved := log.of(RelationEntity, relID)
	if len(moved) != 3 || moved[1].Op != changelog.Delete || moved[2].Op != changelog.Upsert {
		t.Fatalf("the moved relation was recorded as %+v", moved)
	}
	if moved[2].Payload["target_ci_id"] != survivor.String() {
		t.Fatalf("the relation ends at %v, want the survivor %s", moved[2].Payload["target_ci_id"], survivor)
	}
}

// A class defined tells the change log the workspace's whole catalogue, the shipped classes with
// the new one, under the workspace and no hub.
func TestAClassDefinedReachesTheChangeLogAsTheCatalogue(t *testing.T) {
	f := newFixture()
	log := &changeLog{}
	f.inv.Changes, f.inv.HLC = log, &hlcs{}
	if _, err := call(t, DefineCiClass{f.inv}.Descriptor(), as(anna), usecase.Input{"key": "printer", "parent": "hardware", "name": "Printer"}); err != nil {
		t.Fatal(err)
	}
	put := log.of(CatalogueEntity, tenant)
	if len(put) != 1 || !put[0].ContainerID.IsZero() {
		t.Fatalf("the catalogue was recorded as %+v", put)
	}
	found := map[string]bool{}
	for _, c := range put[0].Payload["classes"].([]usecase.Output) {
		found[c["key"].(string)] = true
	}
	if !found["printer"] || !found["server"] {
		t.Fatalf("the catalogue holds %v", found)
	}
}
