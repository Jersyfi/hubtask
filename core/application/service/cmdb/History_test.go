// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"slices"
	"testing"

	cmdbrepo "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// chronicle is an in-memory history, read newest first.
type chronicle struct{ steps []domain.Step }

func (c *chronicle) AppendCIHistory(_ context.Context, step domain.Step) error {
	c.steps = append(c.steps, step)
	return nil
}

func (c *chronicle) CIHistory(_ context.Context, ci shared.ID, page workrepo.Page) (cmdbrepo.HistoryPage, error) {
	var out []domain.Step
	for _, s := range slices.Backward(c.steps) {
		if s.CIID == ci && len(out) < page.Size {
			out = append(out, s)
		}
	}
	return cmdbrepo.HistoryPage{Steps: out}, nil
}

func codesOf(t *testing.T, out usecase.Output) []string {
	t.Helper()
	var codes []string
	for _, step := range out["data"].([]usecase.Output) {
		codes = append(codes, step["code"].(string))
	}
	return codes
}

// UC-AST-01 check 13: an object's history says, newest first, who made it, changed it by hand or
// let a source change it, moved its stage and its owner, related it, confirmed it, moved its asset
// and put a file on it - the stage, the person and the file by value, an attribute only by name; a
// reader of another hub learns nothing of it.
func TestAnObjectsHistoryTellsWhoDidWhat(t *testing.T) {
	f, a := assuredFixture()
	history, files := &chronicle{}, newShelf()
	f.inv.History, f.inv.Files, f.inv.Reader = history, files, seeing{}
	run := func(d usecase.Descriptor, in usecase.Input) usecase.Output {
		t.Helper()
		out, err := call(t, d, as(anna), in)
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		return out
	}
	made := run(CreateCi{f.inv}.Descriptor(), usecase.Input{"hub_id": hub.String(), "class": "server", "name": "db1", "attributes": map[string]any{"mac_address": "aa:bb:cc"}})
	db := shared.MustParseID(made["id"].(string))
	run(UpdateCi{f.inv}.Descriptor(), usecase.Input{"ci_id": db.String(), "attributes": map[string]any{"operating_system": "Debian"}})
	run(ReconcileCis{f.inv}.Descriptor(), usecase.Input{"hub_id": hub.String(), "source": "netbox", "items": items(
		map[string]any{"class": "server", "name": "db1", "attributes": map[string]any{"mac_address": "aa:bb:cc", "model": "R740"}})})
	run(SetCiCompliance{f.inv}.Descriptor(), usecase.Input{"ci_id": db.String(), "stage": "MAINTENANCE", "owner_account_id": bert.String()})
	rel := run(RelateCis{f.inv}.Descriptor(), usecase.Input{"source_ci_id": vm1.String(), "target_ci_id": db.String(), "type": "depends_on"})
	run(ConfirmCi{f.inv}.Descriptor(), usecase.Input{"ci_id": db.String()})
	files.objects[manual] = media.Object{ID: manual, Status: media.StatusReady, Usage: media.UsageAttachment, CreatedBy: anna}
	run(AttachCiMedia{f.inv}.Descriptor(), usecase.Input{"ci_id": db.String(), "media_id": manual.String()})
	_ = a

	out := run(ListCiHistory{f.inv}.Descriptor(), usecase.Input{"ci_id": db.String()})
	got := codesOf(t, out)
	want := []string{
		"activity.ci_file_attached", "activity.ci_confirmed", "activity.ci_related", "activity.ci_changed",
		"activity.ci_stage_changed", "activity.ci_changed", "activity.ci_changed", "activity.ci_created",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the history reads %v, want %v", got, want)
	}
	steps := out["data"].([]usecase.Output)
	set := func(i int) usecase.Output { return steps[i]["change_set"].(usecase.Output) }
	if moved := set(0)["media_id"].(usecase.Output); moved["to"] != manual.String() {
		t.Errorf("the file: %v", moved)
	}
	if moved := set(2)["other_ci_id"].(usecase.Output); moved["to"] != vm1.String() || set(2)["direction"] == nil {
		t.Errorf("the relation, seen from its target: %v", set(2))
	}
	if moved := set(3)["owner_account_id"].(usecase.Output); moved["to"] != bert.String() {
		t.Errorf("the owner: %v", set(3))
	}
	if moved := set(4)["stage"].(usecase.Output); moved["from"] != "IN_USE" || moved["to"] != "MAINTENANCE" {
		t.Errorf("the stage: %v", set(4))
	}
	if source := set(5)["source"].(usecase.Output); source["to"] != "netbox" || set(5)["model"].(usecase.Output)["changed"] != true {
		t.Errorf("the source's change: %v", set(5))
	}
	if set(6)["source"] != nil || set(6)["operating_system"].(usecase.Output)["to"] != nil {
		t.Errorf("a change by hand names a source or an attribute's value: %v", set(6))
	}
	if actor := steps[0]["actor"].(usecase.Output); actor["id"] != anna.String() || actor["type"] != "USER" {
		t.Errorf("the actor: %v", actor)
	}
	// The relation's other end tells it too.
	if codes := codesOf(t, run(ListCiHistory{f.inv}.Descriptor(), usecase.Input{"ci_id": vm1.String()})); len(codes) != 1 || codes[0] != "activity.ci_related" {
		t.Errorf("the source end: %v", codes)
	}
	run(UnrelateCis{f.inv}.Descriptor(), usecase.Input{"relation_id": rel["id"]})
	if codes := codesOf(t, run(ListCiHistory{f.inv}.Descriptor(), usecase.Input{"ci_id": vm1.String()})); codes[0] != "activity.ci_unrelated" {
		t.Errorf("after unrelating: %v", codes)
	}

	// A reader outside the private hub is told the vault's history does not exist.
	if _, err := call(t, ListCiHistory{f.inv}.Descriptor(), as(bert), usecase.Input{"ci_id": vault.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("Bert read the vault's history: %v", err)
	}
}

// UC-AST-01 check 13: registering and moving an asset, the stage it carries along, and a round's
// answer each write their step.
func TestAnAssetsMovesAndARoundsAnswerWriteTheirSteps(t *testing.T) {
	f, _ := assuredFixture()
	history := &chronicle{}
	f.inv.History = history
	for _, step := range []struct {
		d  usecase.Descriptor
		in usecase.Input
	}{
		{RegisterCiAsset{Inventory: f.inv}.Descriptor(), usecase.Input{"ci_id": host.String(), "state": "ORDERED"}},
		{MoveCiAsset{Inventory: f.inv}.Descriptor(), usecase.Input{"ci_id": host.String(), "state": "IN_STOCK"}},
	} {
		if _, err := call(t, step.d, as(anna), step.in); err != nil {
			t.Fatal(err)
		}
	}
	started, err := call(t, StartCiCampaign{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"hub_id": hub.String(), "name": "Q4", "classes": []any{"server"}, "due_on": "2026-11-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, AttestCi{Inventory: f.inv}.Descriptor(), as(bert), usecase.Input{
		"campaign_id": started["id"], "ci_id": host.String(), "verdict": "CONFIRMED",
	}); err != nil {
		t.Fatal(err)
	}
	out, err := call(t, ListCiHistory{f.inv}.Descriptor(), as(bert), usecase.Input{"ci_id": host.String()})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"activity.ci_attested", "activity.ci_stage_changed", "activity.ci_asset_moved", "activity.ci_stage_changed", "activity.ci_asset_moved"}
	if got := codesOf(t, out); !slices.Equal(got, want) {
		t.Fatalf("the history reads %v, want %v", got, want)
	}
	steps := out["data"].([]usecase.Output)
	if moved := steps[2]["change_set"].(usecase.Output)["state"].(usecase.Output); moved["from"] != "ORDERED" || moved["to"] != "IN_STOCK" {
		t.Errorf("the move: %v", moved)
	}
	if verdict := steps[0]["change_set"].(usecase.Output)["verdict"].(usecase.Output); verdict["to"] != "CONFIRMED" {
		t.Errorf("the answer: %v", verdict)
	}
}

// UC-AST-01 check 13: an object a source brings in says so in its first step, and a merge is told
// on both sides - the object merged away names the one it went into, the one that stays names the
// object it took in.
func TestASourcesObjectAndAMergeWriteTheirSteps(t *testing.T) {
	f := newFixture()
	history := &chronicle{}
	f.inv.History = history
	brought, err := call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "source": "netbox", "items": items(
		map[string]any{"class": "server", "name": "sw1", "attributes": map[string]any{"mac_address": "cc:dd"}})})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(outcomes(brought), []string{"CREATE"}) {
		t.Fatalf("the source's object: %v", outcomes(brought))
	}
	var created *domain.Step
	for i := range history.steps {
		if history.steps[i].Verb == domain.VerbCreated {
			created = &history.steps[i]
		}
	}
	if created == nil || created.Changes["source"] != (domain.Moved{To: "netbox"}) {
		t.Fatalf("the first step does not name the source: %+v", created)
	}

	for _, name := range []string{"one", "two"} {
		if _, err := call(t, CreateCi{f.inv}.Descriptor(), as(anna), usecase.Input{
			"hub_id": hub.String(), "class": "server", "name": name, "attributes": map[string]any{"asset_tag": "T-" + name},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for id, ci := range f.m.cis {
		if ci.Name == "one" || ci.Name == "two" {
			f.m.ids[id]["mac_address"] = "aa:bb"
		}
	}
	if _, err := call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "source": "netbox", "items": items(
		map[string]any{"class": "server", "name": "x", "attributes": map[string]any{"mac_address": "aa:bb"}})}); err != nil || len(f.m.dups) != 1 {
		t.Fatalf("no duplicate: %v", err)
	}
	dup := f.m.dups[0]
	survivor, other := dup.CIs[0], dup.CIs[1]
	if _, err := call(t, ResolveCiDuplicate{f.inv}.Descriptor(), as(anna), usecase.Input{"duplicate_id": dup.ID.String(), "survivor_id": survivor.String()}); err != nil {
		t.Fatal(err)
	}
	kept, err := call(t, ListCiHistory{f.inv}.Descriptor(), as(anna), usecase.Input{"ci_id": survivor.String()})
	if err != nil {
		t.Fatal(err)
	}
	steps := kept["data"].([]usecase.Output)
	if codesOf(t, kept)[0] != "activity.ci_merged_in" || steps[0]["change_set"].(usecase.Output)["merged_from"].(usecase.Output)["to"] != other.String() {
		t.Errorf("the survivor's history: %v", steps[0])
	}
	gone, err := call(t, ListCiHistory{f.inv}.Descriptor(), as(anna), usecase.Input{"ci_id": other.String()})
	if err != nil {
		t.Fatal(err)
	}
	if codes := codesOf(t, gone); codes[0] != "activity.ci_merged" {
		t.Errorf("the merged object's history: %v", codes)
	}
}
