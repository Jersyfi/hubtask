// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"testing"
	"time"

	configrepo "github.com/Jersyfi/hubtask/core/application/repository/config"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	firstCollection  = "0190a000-0000-7000-8000-0000000000c1"
	secondCollection = "0190a000-0000-7000-8000-0000000000c2"
	sampleItem       = "0190a000-0000-7000-8000-0000000000e1"
)

// configuration.md §6: a hub's document reads its sample entries by key with the collection's
// handle, adds a sample through AddSampleEntry - in the hub's first collection where the block
// names none - and removes one by trashing its entry.
func TestAHubsDocumentCarriesItsSampleEntries(t *testing.T) {
	s := hubSamplesSection(shared.MustParseID(supportHub))
	p := places{
		key: map[string]string{firstCollection: "intake", secondCollection: "changes"},
		id:  map[string]string{"intake": firstCollection, "changes": secondCollection},
		hub: supportHub, children: map[string]bool{firstCollection: true, secondCollection: true},
		order: []string{firstCollection, secondCollection},
	}
	key, block := s.read(map[string]any{"key": "vpn", "item_id": sampleItem, "title": "VPN drops", "entry_type": "incident", "collection_id": secondCollection})
	block = s.present(p, block)
	if key != "vpn" || block["collection"] != "changes" || block["title"] != "VPN drops" || block["entry_type"] != "incident" {
		t.Fatalf("read %q %v", key, block)
	}

	now := current{Places: p}
	ops, err := s.opsAt(now, domain.BlockChange{Section: sectionSamples, Key: "printer", Op: domain.OpAdd,
		After: domain.Block{"title": "Printer offline", "entry_type": "incident", "collection": "changes"}}, nil)
	if err != nil || len(ops) != 1 || ops[0].UseCase != AddSampleEntryName || ops[0].Input["collection_id"] != secondCollection ||
		ops[0].Input["key"] != "printer" || ops[0].Input["hub_id"] != supportHub || ops[0].Input["entry_type"] != "incident" {
		t.Fatalf("add %+v, %v", ops, err)
	}
	ops, err = s.opsAt(now, domain.BlockChange{Section: sectionSamples, Key: "mail", Op: domain.OpAdd, After: domain.Block{"title": "Mail late"}}, nil)
	if err != nil || ops[0].Input["collection_id"] != firstCollection {
		t.Fatalf("a block naming no collection %+v, %v", ops, err)
	}
	if _, err := s.opsAt(now, domain.BlockChange{Section: sectionSamples, Key: "x", Op: domain.OpAdd,
		After: domain.Block{"title": "X", "collection": "elsewhere"}}, nil); err == nil {
		t.Fatal("a collection of no hub's was taken")
	}
	if _, err := s.opsAt(current{Places: places{children: map[string]bool{}}}, domain.BlockChange{Section: sectionSamples, Key: "x",
		Op: domain.OpAdd, After: domain.Block{"title": "X"}}, nil); err == nil {
		t.Fatal("a hub without a collection took a sample")
	}
	ops, err = s.opsAt(now, domain.BlockChange{Section: sectionSamples, Key: "vpn", Op: domain.OpRemove}, map[string]any{"item_id": sampleItem})
	if err != nil || len(ops) != 1 || ops[0].UseCase != "TrashWorkItem" || ops[0].Input["item_id"] != sampleItem {
		t.Fatalf("remove %+v, %v", ops, err)
	}
	for _, attribute := range []string{"title", "entry_type", "collection"} {
		if !s.isFixed(attribute) {
			t.Errorf("%s changes a sample a person may have worked on", attribute)
		}
	}
}

// configuration.md §6: a template's sample entries are optional - a hub's plan carries them only
// when asked, and only the first time the hub takes the template; an upgrade neither adds nor
// removes them.
func TestATemplatesSamplesAreOfferedOnceAndOnlyWhenAsked(t *testing.T) {
	f := hubFixture()
	samples := map[string]any{"printer": map[string]any{"title": "Printer offline"}}
	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "desk", "name": "Desk",
		"document":     map[string]any{"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}}},
		"hub_document": map[string]any{"scheme": map[string]any{"every_type": map[string]any{"workflow": "service"}}, "samples": samples},
	})
	take := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "desk"})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": take["document"], "plan_hash": take["plan_hash"], "template": take["template"],
	})
	f.world.preview = usecase.Output{"entries": 2, "moves": []usecase.Output{}, "removed": []usecase.Output{}}

	plain := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "desk", "hub_id": supportHub})
	if document := plain["document"].(map[string]any); document["samples"] != nil {
		t.Fatalf("samples taken unasked: %v", document)
	}
	asked := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "desk", "hub_id": supportHub, "samples": true})
	if document := asked["document"].(map[string]any); document["samples"] == nil {
		t.Fatalf("samples asked for and not offered: %v", document)
	}
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": plain["document"], "plan_hash": plain["plan_hash"], "template": plain["template"],
		"mode": plain["mode"], "hub_id": supportHub, "acknowledge": true,
	})
	again := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "desk", "hub_id": supportHub, "samples": true})
	if document := again["document"].(map[string]any); document["samples"] != nil {
		t.Fatalf("a hub that took the template is offered its samples again: %v", document)
	}

	original := domain.Document{"samples": domain.Section{"a": domain.Block{}}, "views": domain.Section{}}
	if stripped := withoutSamples(original); stripped["samples"] != nil || original["samples"] == nil || stripped["views"] == nil {
		t.Fatalf("withoutSamples %v from %v", stripped, original)
	}
}

// sampleWorld is the catalogue and the marks the sample use cases reach.
type sampleWorld struct {
	catalogue
	created, trashed []usecase.Input
	marks            map[string]shared.ID
	listed           []configrepo.Sample
}

func (w *sampleWorld) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	switch name {
	case "GetContainer":
		if in.String("container_id") == supportHub {
			return w.catalogue.Invoke(ctx, name, actor, in)
		}
		parent := supportHub
		if in.String("container_id") == secondCollection {
			parent = "0190a8f0-0000-7000-8000-0000000000f2"
		}
		return usecase.Output{"id": in.String("container_id"), "type": "COLLECTION", "parent_id": parent}, nil
	case "CreateWorkItem":
		w.created = append(w.created, in)
		return usecase.Output{"id": sampleItem, "title": in["title"], "entry_type": in["entry_type"]}, nil
	case "TrashWorkItem":
		w.trashed = append(w.trashed, in)
		return usecase.Output{}, nil
	}
	return w.catalogue.Invoke(ctx, name, actor, in)
}

func (w *sampleWorld) Mark(_ context.Context, _, item shared.ID, key string, _ time.Time) (bool, error) {
	if _, held := w.marks[key]; held {
		return false, nil
	}
	w.marks[key] = item
	return true, nil
}

func (w *sampleWorld) List(context.Context, shared.ID) ([]configrepo.Sample, error) {
	return w.listed, nil
}

func sampleFixture() (fixture, *sampleWorld) {
	f := hubFixture()
	w := &sampleWorld{catalogue: catalogue{f.world}, marks: map[string]shared.ID{}}
	f.desk.Catalogue, f.desk.Samples = w, w
	return f, w
}

// configuration.md §6: a sample is a task created in a collection of the hub and marked under its
// key; a key taken and a collection of another hub are refused before anything is marked.
func TestASampleEntryIsAnEntryOfTheHubMarkedByItsKey(t *testing.T) {
	f, w := sampleFixture()
	add := AddSampleEntry{f.desk}.Descriptor()
	out := invoke(t, add, person, usecase.Input{
		"hub_id": supportHub, "key": "printer", "collection_id": firstCollection, "title": "Printer offline", "entry_type": "incident",
	})
	if out["item_id"] != sampleItem || out["key"] != "printer" || len(w.created) != 1 {
		t.Fatalf("added %v, created %v", out, w.created)
	}
	if c := w.created[0]; c["type"] != "TASK" || c["collection_id"] != firstCollection || c["entry_type"] != "incident" {
		t.Errorf("created %v", c)
	}
	if w.marks["printer"] != shared.ID(sampleItem) {
		t.Errorf("not marked: %v", w.marks)
	}
	_, err := add.Handler.Invoke(t.Context(), person, usecase.Input{"hub_id": supportHub, "key": "printer", "collection_id": firstCollection, "title": "Again"})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.sample_taken" {
		t.Errorf("a key taken twice: %v", err)
	}
	_, err = add.Handler.Invoke(t.Context(), person, usecase.Input{"hub_id": supportHub, "key": "vpn", "collection_id": secondCollection, "title": "VPN"})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.collection_unknown" || len(w.created) != 2 {
		t.Errorf("another hub's collection: %v, created %d", err, len(w.created))
	}
	_, err = add.Handler.Invoke(t.Context(), person, usecase.Input{"hub_id": supportHub, "key": "Bad Key", "collection_id": firstCollection, "title": "X"})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.key_invalid" {
		t.Errorf("a key that is no key: %v", err)
	}
}

// configuration.md §6: removing a hub's samples trashes exactly the marked entries, in one act.
func TestRemovingTheSamplesTrashesOnlyTheMarkedEntries(t *testing.T) {
	f, w := sampleFixture()
	w.listed = []configrepo.Sample{{Key: "a", ItemID: sampleItem}, {Key: "b", ItemID: "0190a000-0000-7000-8000-0000000000e2"}}
	out := invoke(t, RemoveSampleEntries{f.desk}.Descriptor(), person, usecase.Input{"hub_id": supportHub})
	if out["removed"] != 2 || len(w.trashed) != 2 || w.trashed[0]["item_id"] != sampleItem {
		t.Fatalf("removed %v, trashed %v", out, w.trashed)
	}
	listed := invoke(t, ListSampleEntries{f.desk}.Descriptor(), person, usecase.Input{"hub_id": supportHub})
	if rows := listed["samples"].([]any); len(rows) != 2 || rows[0].(map[string]any)["key"] != "a" {
		t.Errorf("listed %v", listed)
	}
}
