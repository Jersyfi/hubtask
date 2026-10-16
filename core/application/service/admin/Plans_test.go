// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/plan"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// planStore is the installation's plans, in memory, with the in-use count the real door keeps.
type planStore struct {
	plans   []plan.Plan
	removed map[string]bool
	inUse   map[string]int64
	// removeOnReplace is a removal landing between a change's read and its write.
	removeOnReplace string
}

func newPlanStore() *planStore {
	return &planStore{removed: map[string]bool{}, inUse: map[string]int64{}}
}

func (s *planStore) List(context.Context) ([]plan.Plan, error) {
	listed := []plan.Plan{}
	for _, held := range s.plans {
		if !s.removed[held.Name] {
			listed = append(listed, held)
		}
	}
	return listed, nil
}

func (s *planStore) Find(_ context.Context, name string) (plan.Plan, error) {
	for _, held := range s.plans {
		if held.Name == name && !s.removed[name] {
			return held, nil
		}
	}
	return plan.Plan{}, shared.ErrNotFound.WithDetail("admin.plan_not_found")
}

func (s *planStore) Workspaces(context.Context) (map[string]int64, error) { return s.inUse, nil }

func (s *planStore) Insert(_ context.Context, created plan.Plan, _ shared.ID) error {
	for _, held := range s.plans {
		if held.Name == created.Name {
			return shared.ErrConflict.WithDetail("admin.plan_name_taken")
		}
	}
	s.plans = append(s.plans, created)
	return nil
}

func (s *planStore) Replace(
	_ context.Context, changed plan.Plan, expected *int, _ shared.ID, now time.Time,
) (bool, error) {
	if s.removeOnReplace == changed.Name {
		s.removed[changed.Name] = true
	}
	for i, held := range s.plans {
		if held.ID != changed.ID || s.removed[held.Name] {
			continue
		}
		if expected != nil && *expected != held.Version {
			return false, nil
		}
		changed.Version, changed.UpdatedAt = held.Version+1, now
		s.plans[i] = changed
		return true, nil
	}
	return false, nil
}

func (s *planStore) Remove(_ context.Context, id shared.ID, _ shared.ID, _ time.Time) (adminrepo.RemoveOutcome, error) {
	for _, held := range s.plans {
		if held.ID != id || s.removed[held.Name] {
			continue
		}
		if s.inUse[held.Name] > 0 {
			return adminrepo.PlanInUse, nil
		}
		s.removed[held.Name] = true
		return adminrepo.PlanRemoved, nil
	}
	return adminrepo.PlanNotFound, nil
}

type planFixture struct {
	writer   PlanWriter
	plans    *planStore
	settings *instanceStore
	journal  *journalStore
	registry *usecase.Registry
}

func newPlanFixture(t *testing.T) planFixture {
	t.Helper()
	instance, settings, journal := newInstanceWriter(newRegister())
	plans := newPlanStore()
	writer := PlanWriter{Instance: instance, Plans: plans, Tenancy: env.TenancyMulti}
	registry, err := usecase.NewRegistry(nil,
		ListPlans{writer}.Descriptor(), ReadPlan{writer}.Descriptor(), CreatePlan{writer}.Descriptor(),
		ChangePlan{writer}.Descriptor(), RemovePlan{writer}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return planFixture{writer: writer, plans: plans, settings: settings, journal: journal, registry: registry}
}

func familyInput() usecase.Input {
	return usecase.Input{
		"name": "family", "display_name": "Family",
		"quotas":   map[string]any{"items": map[string]any{"value": 5000, "locked": true}},
		"left_out": []any{"WEBHOOKS"},
	}
}

func refusalCode(err error) string {
	var refusal *shared.Error
	if errors.As(err, &refusal) {
		return refusal.DetailCode
	}
	return ""
}

func TestAPlanIsCreatedReadAndListed(t *testing.T) {
	f := newPlanFixture(t)

	out, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if out.String("name") != "family" || out.Int("version") != 1 {
		t.Errorf("created %v", out)
	}
	quotas, _ := out["quotas"].(usecase.Output)
	items, _ := quotas["items"].(usecase.Output)
	if items["value"] != int64(5000) || items["locked"] != true {
		t.Errorf("the plan's limit answered %v", items)
	}
	if leftOut, _ := out["left_out"].([]any); len(leftOut) != 1 || leftOut[0] != "WEBHOOKS" {
		t.Errorf("left out %v", out["left_out"])
	}
	if _, held := out["localisation"]; held {
		t.Error("a plan answered a localisation area")
	}

	f.plans.inUse["family"] = 3
	read, err := f.registry.Invoke(t.Context(), ReadPlanName, operator(), usecase.Input{"name": "family"})
	if err != nil || read["workspaces"] != int64(3) {
		t.Errorf("read %v, %v", read["workspaces"], err)
	}
	listed, err := f.registry.Invoke(t.Context(), ListPlansName, operator(), usecase.Input{})
	if rows, _ := listed["data"].([]usecase.Output); err != nil || len(rows) != 1 {
		t.Errorf("listed %v, %v", listed, err)
	}

	// UC-INS-13 check 6: changing a plan is in the instance journal, by its short name only.
	if len(f.journal.entries) != 1 || f.journal.entries[0].Action != "plan.created" ||
		f.journal.entries[0].Details["plan"] != "family" {
		t.Errorf("journal %+v", f.journal.entries)
	}
}

func TestAShortNameIsNeverTakenTwice(t *testing.T) {
	f := newPlanFixture(t)
	if _, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.registry.Invoke(t.Context(), RemovePlanName, operator(), usecase.Input{"name": "family"}); err != nil {
		t.Fatalf("removing: %v", err)
	}
	_, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput())
	if refusalCode(err) != "admin.plan_name_taken" {
		t.Errorf("a removed plan's name was taken again: %v", err)
	}
}

func TestAPlanIsRefusedWhatTheInstallationDecided(t *testing.T) {
	f := newPlanFixture(t)
	f.settings.level.Quotas = identity.QuotaDefaults{
		Limits: map[string]int64{"items": 1000}, Locks: map[string]bool{"items": true},
	}
	_, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput())
	if refusalCode(err) != "admin.plan_under_installation_lock" {
		t.Errorf("a limit under the installation's lock answered %v", err)
	}

	in := familyInput()
	delete(in, "quotas")
	in["localisation"] = map[string]any{"locale": map[string]any{"value": "de"}}
	_, err = f.registry.Invoke(t.Context(), CreatePlanName, operator(), in)
	if refusalCode(err) != "admin.plan_localisation" && !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a plan with a language answered %v", err)
	}
	if len(f.plans.plans) != 0 {
		t.Error("a refused plan was written")
	}
}

func TestAPlanChangesWithTheVersionItWasReadAt(t *testing.T) {
	f := newPlanFixture(t)
	if _, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput()); err != nil {
		t.Fatal(err)
	}

	change := usecase.Input{"name": "family", "display_name": "Family plus", "expected_version": 1}
	out, err := f.registry.Invoke(t.Context(), ChangePlanName, operator(), change)
	if err != nil || out.Int("version") != 2 || out.String("display_name") != "Family plus" {
		t.Fatalf("changed %v, %v", out, err)
	}
	// A PUT: what the body does not name is no longer decided.
	if quotas, _ := out["quotas"].(usecase.Output); quotas["items"].(usecase.Output)["set"] != false {
		t.Errorf("the limit the change left out still stands: %v", quotas["items"])
	}

	_, err = f.registry.Invoke(t.Context(), ChangePlanName, operator(), change)
	if !errors.Is(err, shared.ErrVersionConflict) || refusalCode(err) != "admin.plan_version_moved" {
		t.Errorf("a save on a stale version answered %v", err)
	}

	_, err = f.registry.Invoke(t.Context(), ChangePlanName, operator(),
		usecase.Input{"name": "nobody", "display_name": "Nobody"})
	if refusalCode(err) != "admin.plan_not_found" {
		t.Errorf("an unknown plan answered %v", err)
	}
}

func TestAPlanAWorkspaceIsOnIsNotRemoved(t *testing.T) {
	f := newPlanFixture(t)
	if _, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput()); err != nil {
		t.Fatal(err)
	}
	f.plans.inUse["family"] = 1
	_, err := f.registry.Invoke(t.Context(), RemovePlanName, operator(), usecase.Input{"name": "family"})
	if !errors.Is(err, shared.ErrConflict) || refusalCode(err) != "admin.plan_in_use" {
		t.Errorf("removing a plan in use answered %v", err)
	}
}

func TestAPlanIsNotWrittenWhileTheFileEnforces(t *testing.T) {
	f := newPlanFixture(t)
	f.settings.level.IsEnforcedFromFile = true
	f.settings.level.Source = "/etc/hubtask/instance.yaml"
	_, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput())
	if refusalCode(err) != "admin.instance_enforced_from_file" {
		t.Errorf("a create while the file enforces answered %v", err)
	}
}

func TestPlansAreMultiModes(t *testing.T) {
	for _, tenancy := range []env.TenancyMode{env.TenancySingle, ""} {
		f := newPlanFixture(t)
		f.writer.Tenancy = tenancy
		registry, err := usecase.NewRegistry(nil, ListPlans{f.writer}.Descriptor(), CreatePlan{f.writer}.Descriptor())
		if err != nil {
			t.Fatal(err)
		}
		for name, in := range map[string]usecase.Input{ListPlansName: {}, CreatePlanName: familyInput()} {
			_, err := registry.Invoke(t.Context(), name, operator(), in)
			if refusalCode(err) != "admin.multi_mode_required" {
				t.Errorf("%s in tenancy %q answered %v", name, tenancy, err)
			}
		}
	}
}

func TestPlansDemandTheRegister(t *testing.T) {
	f := newPlanFixture(t)
	f.writer.Instance.Operators = newRegister(secondOperator)
	registry, err := usecase.NewRegistry(nil, ListPlans{f.writer}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Invoke(t.Context(), ListPlansName, operator(), usecase.Input{}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a caller outside the register answered %v", err)
	}
}

// Changing and removing a plan are in the instance journal too, by short name (UC-INS-13 check 6).
func TestChangingAndRemovingAPlanAreInTheJournal(t *testing.T) {
	f := newPlanFixture(t)
	if _, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.registry.Invoke(t.Context(), ChangePlanName, operator(),
		usecase.Input{"name": "family", "display_name": "Family plus"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.registry.Invoke(t.Context(), RemovePlanName, operator(), usecase.Input{"name": "family"}); err != nil {
		t.Fatal(err)
	}
	actions := []string{}
	for _, entry := range f.journal.entries {
		if entry.Details["plan"] != "family" {
			t.Errorf("%s names %v", entry.Action, entry.Details)
		}
		actions = append(actions, entry.Action)
	}
	if len(actions) != 3 || actions[1] != "plan.changed" || actions[2] != "plan.removed" {
		t.Errorf("journal %v", actions)
	}
}

// A change racing a removal answers the plan as gone, not as a version to read again.
func TestAChangeToAPlanRemovedMeanwhileFindsItGone(t *testing.T) {
	f := newPlanFixture(t)
	if _, err := f.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput()); err != nil {
		t.Fatal(err)
	}
	f.plans.removeOnReplace = "family"
	_, err := f.registry.Invoke(t.Context(), ChangePlanName, operator(),
		usecase.Input{"name": "family", "display_name": "Family plus"})
	if refusalCode(err) != "admin.plan_not_found" {
		t.Errorf("a change racing a removal answered %v", err)
	}
}
