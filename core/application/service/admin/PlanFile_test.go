// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

func fileFamily() map[string]any {
	return map[string]any{
		"name": "family", "display_name": "Family",
		"quotas": map[string]any{"items": map[string]any{"value": float64(5000), "locked": true}},
	}
}

func TestAFileSeedsPlansOnce(t *testing.T) {
	f := newPlanFixture(t)
	if _, err := f.writer.ApplyFile(t.Context(), []map[string]any{fileFamily()}, false, "instance.json"); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if len(f.plans.plans) != 1 || f.journal.entries[0].ActorLabel != "instance.json" {
		t.Fatalf("seeded %d plans, journal %+v", len(f.plans.plans), f.journal.entries)
	}
	business := map[string]any{"name": "business", "display_name": "Business"}
	if _, err := f.writer.ApplyFile(t.Context(), []map[string]any{fileFamily(), business}, false, "instance.json"); err != nil {
		t.Fatal(err)
	}
	if len(f.plans.plans) != 1 {
		t.Error("a seeding file wrote again where plans exist")
	}
}

// An enforcing file is the source: it writes what it names, removes what it no longer names - and
// keeps a plan workspaces are on, which no door can remove, saying so.
func TestAnEnforcingFileIsTheSourceOfThePlans(t *testing.T) {
	f := newPlanFixture(t)
	for _, name := range []string{"family", "legacy", "pilot"} {
		in := familyInput()
		in["name"] = name
		if _, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), in); err != nil {
			t.Fatal(err)
		}
	}
	f.plans.inUse["pilot"] = 2
	f.settings.level.IsEnforcedFromFile = true
	f.settings.level.Source = "instance.json"

	changed := fileFamily()
	changed["display_name"] = "Family plus"
	outcome, err := f.writer.ApplyFile(t.Context(),
		[]map[string]any{changed, {"name": "business", "display_name": "Business"}}, true, "instance.json")
	if err != nil {
		t.Fatalf("enforcing: %v", err)
	}
	family, _ := f.plans.Find(t.Context(), "family")
	if family.DisplayName != "Family plus" {
		t.Errorf("the named plan was not changed: %q", family.DisplayName)
	}
	if _, err := f.plans.Find(t.Context(), "business"); err != nil {
		t.Errorf("the new plan was not created: %v", err)
	}
	if !f.plans.removed["legacy"] || f.plans.removed["pilot"] {
		t.Errorf("removed %v", f.plans.removed)
	}
	if len(outcome.Kept) != 1 || outcome.Kept[0] != "pilot" {
		t.Errorf("kept %v, want the plan workspaces are on", outcome.Kept)
	}

	// While the file enforces, the API's writes are refused; the file's are not.
	_, err = f.registry.Invoke(t.Context(), CreatePlanName, operator(), usecase.Input{"name": "other", "display_name": "Other"})
	if refusalCode(err) != "admin.instance_enforced_from_file" {
		t.Errorf("the API wrote while the file enforces: %v", err)
	}
}

// The file meets the refusals the API meets (UC-INS-13 check 7): one reading, one check.
func TestTheFileAndTheAPIRefuseAPlanAlike(t *testing.T) {
	for name, document := range map[string]map[string]any{
		"under a lock": fileFamily(),
		"a language":   {"name": "family", "display_name": "Family", "localisation": map[string]any{"locale": map[string]any{"value": "de"}}},
		"a bad name":   {"name": "Family Plan", "display_name": "Family"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t)
			f.settings.level.Quotas = identity.QuotaDefaults{
				Limits: map[string]int64{"items": 100}, Locks: map[string]bool{"items": true},
			}
			_, fileErr := f.writer.ApplyFile(t.Context(), []map[string]any{document}, false, "instance.json")
			_, apiErr := f.registry.Invoke(t.Context(), CreatePlanName, operator(), usecase.Input(document))
			if fileErr == nil || refusalCode(fileErr) == "" {
				t.Fatalf("the file wrote %v (%v)", document, fileErr)
			}
			// The API refuses a key its contract does not have before the use case reads it;
			// the file, which has no contract in front of it, is refused by the plan's own reading.
			if apiErr != nil && refusalCode(apiErr) != refusalCode(fileErr) && refusalCode(apiErr) != "usecase.input_invalid" {
				t.Errorf("the file was refused with %q, the API with %q", refusalCode(fileErr), refusalCode(apiErr))
			}
			if len(f.plans.plans) != 0 {
				t.Error("a refused plan was written")
			}
		})
	}
}

func TestASingleModeInstallationSkipsTheFilesPlans(t *testing.T) {
	f := newPlanFixture(t)
	f.writer.Tenancy = env.TenancySingle
	outcome, err := f.writer.ApplyFile(t.Context(), []map[string]any{fileFamily()}, true, "instance.json")
	if err != nil || !outcome.Skipped || len(f.plans.plans) != 0 {
		t.Errorf("single mode answered %+v, %v, with %d plans", outcome, err, len(f.plans.plans))
	}
}

// Ever counts removed plans too, as the real query does.
func (s *planStore) Ever(context.Context) (bool, error) { return len(s.plans) > 0, nil }

// A seeding file wrote its plans once: a plan an operator removed since is not written again, and
// the start does not fail on its taken name.
func TestASeedingFileDoesNotBringBackARemovedPlan(t *testing.T) {
	f := newPlanFixture(t)
	if _, err := f.writer.ApplyFile(t.Context(), []map[string]any{fileFamily()}, false, "instance.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.registry.Invoke(t.Context(), RemovePlanName, operator(), usecase.Input{"name": "family"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.writer.ApplyFile(t.Context(), []map[string]any{fileFamily()}, false, "instance.json"); err != nil {
		t.Errorf("the next start failed: %v", err)
	}
	if !f.plans.removed["family"] {
		t.Error("the removed plan came back")
	}
}

// An enforcing file read at a start where nothing changed writes nothing.
func TestAnEnforcingFileLeavesAnUnchangedPlanAlone(t *testing.T) {
	f := newPlanFixture(t)
	document := fileFamily()
	document["left_out"] = []any{"WEBHOOKS", "AUTOMATION_RULES"}
	if _, err := f.writer.ApplyFile(t.Context(), []map[string]any{document}, true, "instance.json"); err != nil {
		t.Fatal(err)
	}
	entries := len(f.journal.entries)
	family, _ := f.plans.Find(t.Context(), "family")
	if _, err := f.writer.ApplyFile(t.Context(), []map[string]any{document}, true, "instance.json"); err != nil {
		t.Fatal(err)
	}
	again, _ := f.plans.Find(t.Context(), "family")
	if len(f.journal.entries) != entries || again.Version != family.Version {
		t.Errorf("an unchanged plan was written again: %d journal entries (was %d), version %d (was %d)",
			len(f.journal.entries), entries, again.Version, family.Version)
	}
}
