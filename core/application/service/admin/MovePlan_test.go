// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/service/platform"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// SetPlan moves the fake's one workspace, guarded on its version as the statement is.
func (s *tenantsStore) SetPlan(_ context.Context, planID shared.ID, version int, _ time.Time) (bool, error) {
	if version != s.record.Version || !s.record.PurgeStartedAt.IsZero() {
		return false, nil
	}
	s.record.Version++
	s.record.PlanName = s.planNames[planID]
	s.moved = append(s.moved, "plan->"+s.record.PlanName)
	return true, nil
}

func (s *planStore) Hold(ctx context.Context, name string) (shared.ID, error) {
	held, err := s.Find(ctx, name)
	return held.ID, err
}

type moveFixture struct {
	plans    planFixture
	tenants  *tenantsStore
	audit    *auditSink
	quotas   *quotaStoreFake
	registry *usecase.Registry
}

func newMoveFixture(t *testing.T) moveFixture {
	t.Helper()
	plans := newPlanFixture(t)
	if _, err := plans.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput()); err != nil {
		t.Fatalf("creating the plan: %v", err)
	}
	family, _ := plans.plans.Find(t.Context(), "family")
	tenants := &tenantsStore{
		record: adminrepo.TenantRecord{
			ID: lifecycleTenant, Slug: "maria", Status: domain.TenantActive, Version: 4,
		},
		planNames: map[shared.ID]string{family.ID: "family"},
	}
	items := int64(20000)
	quotas := &quotaStoreFake{}
	quotas.overrides.Items = &items
	sink := &auditSink{}
	f := moveFixture{plans: plans, tenants: tenants, audit: sink, quotas: quotas}
	f.registry = moveRegistry(t, f, platform.Recorder{})
	return f
}

// moveRegistry serves the move over the fixture's stores, telling the platform through recorder.
func moveRegistry(t *testing.T, f moveFixture, recorder platform.Recorder) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, MoveTenantPlan{
		Writer: f.plans.writer, Tenants: f.tenants, Quotas: f.quotas, Audit: f.audit, Platform: recorder,
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func moveTo(plan string, version ...int) usecase.Input {
	in := usecase.Input{"tenant_id": lifecycleTenant.String(), "plan": plan}
	if len(version) > 0 {
		in["expected_version"] = version[0]
	}
	return in
}

// A move: the workspace on the plan, its version raised, the trail and the journal written, and
// the answer naming what rests and which exceptions stay (UC-INS-13 checks 2, 6, 9).
func TestAWorkspaceMovesToAPlan(t *testing.T) {
	f := newMoveFixture(t)
	out, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family", 4))
	if err != nil {
		t.Fatalf("moving: %v", err)
	}
	tenant, _ := out["tenant"].(usecase.Output)
	if tenant["plan"] != "family" || tenant["version"] != 5 {
		t.Errorf("the workspace answered %v", tenant)
	}
	if resting, _ := out["features_resting"].([]any); len(resting) != 1 || resting[0] != "WEBHOOKS" {
		t.Errorf("features resting %v", out["features_resting"])
	}
	if kept, _ := out["exceptions_kept"].([]string); len(kept) != 1 || kept[0] != "items" {
		t.Errorf("exceptions kept %v", out["exceptions_kept"])
	}
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantPlanChangedAction ||
		f.audit.entries[0].TenantID != lifecycleTenant {
		t.Errorf("trail %+v", f.audit.entries)
	}
	last := f.plans.journal.entries[len(f.plans.journal.entries)-1]
	if last.Action != "tenant.plan_changed" || last.Details["plan_to"] != "family" || last.TenantSlug != "maria" {
		t.Errorf("journal %+v", last)
	}
}

// Repeated, the move to the plan the workspace is on writes nothing again.
func TestAMoveToThePlanItIsOnWritesNothing(t *testing.T) {
	f := newMoveFixture(t)
	if _, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family")); err != nil {
		t.Fatalf("moving again: %v", err)
	}
	if len(f.tenants.moved) != 1 || len(f.audit.entries) != 1 {
		t.Errorf("a repeated move wrote %d moves and %d trail entries", len(f.tenants.moved), len(f.audit.entries))
	}
}

// A late retry of an older move cannot undo a newer one: its version is stale.
func TestAStaleMoveIsRefused(t *testing.T) {
	f := newMoveFixture(t)
	if _, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family", 4)); err != nil {
		t.Fatal(err)
	}
	_, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("", 4))
	if !errors.Is(err, shared.ErrVersionConflict) || refusalCode(err) != "admin.tenant_version_moved" {
		t.Errorf("a stale move answered %v", err)
	}
	if f.tenants.record.PlanName != "family" {
		t.Error("the stale move took the workspace off its plan")
	}
}

func TestAMoveToAnUnknownPlanIsRefused(t *testing.T) {
	f := newMoveFixture(t)
	_, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("business"))
	if !errors.Is(err, shared.ErrValidation) || refusalCode(err) != "admin.plan_unknown" {
		t.Errorf("an unknown plan answered %v", err)
	}
}

// A workspace leaving can still be moved until its purge begins; then no more.
func TestAMoveStopsWhereThePurgeBegins(t *testing.T) {
	f := newMoveFixture(t)
	f.tenants.record.Status = domain.TenantPendingDeletion
	if _, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family")); err != nil {
		t.Errorf("a workspace pending deletion was not moved: %v", err)
	}
	f.tenants.record.PurgeStartedAt = time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)
	_, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo(""))
	if refusalCode(err) != "admin.tenant_deletion_underway" {
		t.Errorf("a move after the purge began answered %v", err)
	}
}

// Off plans again: the installation's values and every feature.
func TestAWorkspaceLeavesPlans(t *testing.T) {
	f := newMoveFixture(t)
	if _, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family")); err != nil {
		t.Fatal(err)
	}
	out, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), usecase.Input{"tenant_id": lifecycleTenant.String()})
	if err != nil {
		t.Fatalf("leaving plans: %v", err)
	}
	tenant, _ := out["tenant"].(usecase.Output)
	if tenant["plan"] != nil {
		t.Errorf("the workspace is still on %v", tenant["plan"])
	}
	if resting, _ := out["features_resting"].([]any); len(resting) != 0 {
		t.Errorf("a workspace without a plan rests %v", resting)
	}
}

func TestAMoveIsMultiModes(t *testing.T) {
	f := newMoveFixture(t)
	f.plans.writer.Tenancy = env.TenancySingle
	registry, err := usecase.NewRegistry(nil, MoveTenantPlan{
		Writer: f.plans.writer, Tenants: f.tenants, Quotas: f.quotas, Audit: f.audit,
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family"))
	if refusalCode(err) != "admin.multi_mode_required" {
		t.Errorf("a move in single mode answered %v", err)
	}
}

// A workspace provisioned on a plan starts on it (UC-INS-13 check 8, UC-INS-07's starting plan),
// and an unknown plan is refused before anything is written.
func TestAWorkspaceIsProvisionedOnAPlan(t *testing.T) {
	plans := newPlanFixture(t)
	if _, err := plans.registry.Invoke(t.Context(), CreatePlanName, operator(), familyInput()); err != nil {
		t.Fatal(err)
	}
	family, _ := plans.plans.Find(t.Context(), "family")

	f := newProvisionFixture()
	f.handler.Plans = plans.plans
	f.tenants.record.Version = 1
	f.tenants.planNames = map[shared.ID]string{family.ID: "family"}
	cmd := provisionCommand()
	cmd.Plan = "family"
	provisioned, err := f.handler.Execute(t.Context(), operator(), cmd)
	if err != nil {
		t.Fatalf("provisioning on a plan: %v", err)
	}
	if provisioned.Plan != "family" || provisioned.Version != 2 || f.tenants.record.PlanName != "family" {
		t.Errorf("provisioned on %q at version %d; the row says %q", provisioned.Plan, provisioned.Version, f.tenants.record.PlanName)
	}
	if last := f.journal.entries[len(f.journal.entries)-1]; last.Details["plan"] != "family" {
		t.Errorf("the journal's provisioning entry names plan %v", last.Details["plan"])
	}

	unknown := newProvisionFixture()
	unknown.handler.Plans = plans.plans
	cmd.Plan = "business"
	_, err = unknown.handler.Execute(t.Context(), operator(), cmd)
	if refusalCode(err) != "admin.plan_unknown" {
		t.Errorf("an unknown plan answered %v", err)
	}
	if len(unknown.tenants.inserted) != 0 || len(unknown.accounts.inserted) != 0 {
		t.Error("an unknown plan let the workspace be written")
	}
}
