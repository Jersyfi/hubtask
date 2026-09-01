// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
)

var lifecycleTenant = shared.ID("018f2a1b-0000-7000-8000-0000000000b1")

type shiftFixture struct {
	tenants *tenantsStore
	journal *journalStore
	audit   *auditSink
	work    *unitOfWork
	shift   LifecycleShift
}

func newShiftFixture(status domain.TenantStatus) *shiftFixture {
	f := &shiftFixture{
		tenants: &tenantsStore{
			record: adminrepo.TenantRecord{
				ID: lifecycleTenant, Slug: "acme", Status: status,
			},
			moveOK: true,
		},
		journal: &journalStore{}, audit: &auditSink{}, work: &unitOfWork{},
	}
	f.shift = LifecycleShift{
		Tenants: f.tenants, Journal: f.journal, Audit: f.audit,
		UnitOfWork: f.work, Clock: clock.Fixed(now), IDs: &sequentialIDs{},
	}
	return f
}

func TestSuspensionMovesTheEdgeAndLeavesEvidenceTwice(t *testing.T) {
	f := newShiftFixture(domain.TenantActive)

	err := (SuspendTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant)
	if err != nil {
		t.Fatalf("suspending: %v", err)
	}

	if len(f.tenants.moved) != 1 || f.tenants.moved[0] != "ACTIVE->SUSPENDED" {
		t.Errorf("moves %v", f.tenants.moved)
	}
	// The transaction is the target tenant's own - even the control plane reaches no row its
	// transaction was not opened for.
	if len(f.work.scopes) != 1 || f.work.scopes[0].TenantID != lifecycleTenant {
		t.Errorf("scope %+v", f.work.scopes)
	}
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantSuspendedAction ||
		f.audit.entries[0].Severity != audit.SeverityWarning {
		t.Errorf("audit %+v", f.audit.entries)
	}
	if len(f.journal.entries) != 1 || f.journal.entries[0].Action != journalSuspended ||
		f.journal.entries[0].TenantSlug != "acme" {
		t.Errorf("journal %+v", f.journal.entries)
	}
}

func TestResumingIsOneWriteBack(t *testing.T) {
	f := newShiftFixture(domain.TenantSuspended)

	if err := (ResumeTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant); err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if len(f.tenants.moved) != 1 || f.tenants.moved[0] != "SUSPENDED->ACTIVE" {
		t.Errorf("moves %v", f.tenants.moved)
	}
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantResumedAction {
		t.Errorf("audit %+v", f.audit.entries)
	}
}

// The state the operator wanted already stands: a retry is a success that records nothing -
// noise per retry is not evidence.
func TestMovingToTheStandingStatusIsANoOp(t *testing.T) {
	cases := []struct {
		name   string
		status domain.TenantStatus
		run    func(*shiftFixture) error
	}{
		{"suspending the suspended", domain.TenantSuspended, func(f *shiftFixture) error {
			return (SuspendTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant)
		}},
		{"resuming the active", domain.TenantActive, func(f *shiftFixture) error {
			return (ResumeTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newShiftFixture(c.status)
			if err := c.run(f); err != nil {
				t.Fatalf("the retry answered %v", err)
			}
			if len(f.tenants.moved)+len(f.audit.entries)+len(f.journal.entries) != 0 {
				t.Error("a no-op wrote something")
			}
		})
	}
}

// A workspace already leaving refuses both edges: the deletion request is not undone by a
// lifecycle write (multi-tenancy.md §5's statechart has no such edge).
func TestALeavingWorkspaceRefusesBothEdges(t *testing.T) {
	for _, run := range []func(*shiftFixture) error{
		func(f *shiftFixture) error {
			return (SuspendTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant)
		},
		func(f *shiftFixture) error {
			return (ResumeTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant)
		},
	} {
		f := newShiftFixture(domain.TenantPendingDeletion)
		err := run(f)
		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != "admin.tenant_leaving" {
			t.Errorf("answer %v, want admin.tenant_leaving", err)
		}
	}
}

func TestAnUnknownTenantIsNotFound(t *testing.T) {
	f := newShiftFixture(domain.TenantActive)
	f.tenants.findErr = shared.ErrNotFound.WithDetail("admin.tenant_not_found")

	err := (SuspendTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("answer %v, want not found", err)
	}
}

func TestTheLifecycleDemandsTheAdminScope(t *testing.T) {
	f := newShiftFixture(domain.TenantActive)
	actor := operator()
	actor.Scopes = nil

	err := (SuspendTenant{f.shift}).Execute(t.Context(), actor, lifecycleTenant)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("answer %v, want forbidden", err)
	}
	if len(f.work.scopes) != 0 {
		t.Error("a transaction opened for a refused request")
	}
}

func TestTheListingReadsThroughTheInstallationScope(t *testing.T) {
	f := newShiftFixture(domain.TenantActive)
	f.tenants.listed = []adminrepo.TenantRecord{
		{ID: lifecycleTenant, Slug: "acme", Status: domain.TenantActive},
	}
	handler := ListTenants{Tenants: f.tenants, UnitOfWork: f.work}

	records, err := handler.Execute(t.Context(), operator())
	if err != nil || len(records) != 1 {
		t.Fatalf("listing (%v, %v)", records, err)
	}
	if len(f.work.scopes) != 1 || !f.work.scopes[0].Installation {
		t.Errorf("scope %+v, want the installation scope", f.work.scopes)
	}

	if _, err := (ListTenants{Tenants: f.tenants, UnitOfWork: f.work}).
		Execute(t.Context(), anonymousActor()); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("an anonymous listing answered %v", err)
	}
}

func anonymousActor() appshared.ActorContext { return appshared.ActorContext{} }

// The four use cases through the registry, exactly as REST, MCP and automation reach them: the
// descriptors validate, the untyped input maps, and the outputs carry the contract's field names.
func TestTheAdminUseCasesRoundTripThroughTheRegistry(t *testing.T) {
	provision := newProvisionFixture()
	shift := newShiftFixture(domain.TenantActive)

	deletion := newDeletionFixture(domain.TenantActive)
	registry, err := usecase.NewRegistry(nil,
		provision.handler.Descriptor(),
		ListTenants{Tenants: shift.tenants, UnitOfWork: shift.work}.Descriptor(),
		SuspendTenant{shift.shift}.Descriptor(),
		ResumeTenant{shift.shift}.Descriptor(),
		deletion.handler.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	out, err := registry.Invoke(t.Context(), ProvisionTenantName, operator(), usecase.Input{
		"slug": "acme", "display_name": "Acme GmbH", "owner_email": "eva@acme.example",
		"default_locale": "de", "default_time_zone": "Europe/Berlin",
	})
	if err != nil {
		t.Fatalf("provisioning through the registry: %v", err)
	}
	for _, field := range []string{
		"id", "slug", "owner_account_id", "owner_redemption_token",
		"default_hub_id", "example_collection_id",
	} {
		if out.String(field) == "" {
			t.Errorf("the output is missing %s: %v", field, out)
		}
	}
	if out.String("status") != string(domain.TenantActive) {
		t.Errorf("status %q", out.String("status"))
	}

	shift.tenants.listed = []adminrepo.TenantRecord{{
		ID: lifecycleTenant, Slug: "acme", DisplayName: "Acme GmbH",
		Status: domain.TenantPendingDeletion, DefaultLocale: "de", DefaultTimeZone: "Europe/Berlin",
		CreatedAt: now, PurgeAfter: now.AddDate(0, 0, 30),
	}}
	out, err = registry.Invoke(t.Context(), ListTenantsName, operator(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing through the registry: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0].String("slug") != "acme" {
		t.Fatalf("listing %v", out)
	}
	if rows[0]["purge_after"] == nil {
		t.Error("the purge deadline is missing from a leaving workspace's row")
	}

	if _, err := registry.Invoke(t.Context(), SuspendTenantName, operator(), usecase.Input{
		"tenant_id": lifecycleTenant.String(),
	}); err != nil {
		t.Fatalf("suspending through the registry: %v", err)
	}
	shift.tenants.record.Status = domain.TenantSuspended
	if _, err := registry.Invoke(t.Context(), ResumeTenantName, operator(), usecase.Input{
		"tenant_id": lifecycleTenant.String(),
	}); err != nil {
		t.Fatalf("resuming through the registry: %v", err)
	}
	if len(shift.tenants.moved) != 2 {
		t.Errorf("moves %v", shift.tenants.moved)
	}

	out, err = registry.Invoke(t.Context(), RequestTenantDeletionName, operator(), usecase.Input{
		"tenant_id": lifecycleTenant.String(), "confirmation": "Acme GmbH",
		"step_up_token": "hbt_sup_proof",
	})
	if err != nil {
		t.Fatalf("requesting the deletion through the registry: %v", err)
	}
	if out.String("tenant_id") != lifecycleTenant.String() || out["purge_after"] == nil {
		t.Errorf("deletion output %v", out)
	}
}
