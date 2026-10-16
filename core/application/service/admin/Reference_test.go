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
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// SetReference writes the fake's one workspace's reference, guarded as the statement is; a reference
// in taken stands for one another workspace holds.
func (s *tenantsStore) SetReference(_ context.Context, reference string, version int, _ time.Time) (bool, error) {
	if s.taken[reference] {
		return false, shared.ErrConflict.WithDetail("admin.reference_taken")
	}
	if version != s.record.Version || !s.record.PurgeStartedAt.IsZero() {
		return false, nil
	}
	s.record.Version++
	s.record.ExternalReference = reference
	s.moved = append(s.moved, "reference")
	return true, nil
}

// tenantFinder answers the fake's workspace as the query does.
type tenantFinder struct{ tenants *tenantsStore }

func (f tenantFinder) Query(_ context.Context, query adminrepo.TenantQuery) ([]adminrepo.TenantRecord, adminrepo.PageInfo, error) {
	record := f.tenants.record
	if (!query.ID.IsZero() && query.ID != record.ID) || (query.Reference != "" && query.Reference != record.ExternalReference) {
		return nil, adminrepo.PageInfo{}, nil
	}
	return []adminrepo.TenantRecord{record}, adminrepo.PageInfo{}, nil
}

type referenceFixture struct {
	tenants  *tenantsStore
	audit    *auditSink
	journal  *journalStore
	registry *usecase.Registry
	writer   PlanWriter
}

func newReferenceFixture(t *testing.T) referenceFixture {
	t.Helper()
	plans := newPlanFixture(t)
	tenants := &tenantsStore{
		record: adminrepo.TenantRecord{ID: lifecycleTenant, Slug: "maria", Status: domain.TenantActive, Version: 4},
		taken:  map[string]bool{"cus_taken": true},
	}
	sink := &auditSink{}
	registry, err := usecase.NewRegistry(nil, UpdateTenant{
		Writer: plans.writer, Tenants: tenants, Finder: tenantFinder{tenants}, Audit: sink,
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	return referenceFixture{tenants: tenants, audit: sink, journal: plans.journal, registry: registry, writer: plans.writer}
}

func referenceTo(reference any, version ...int) usecase.Input {
	in := usecase.Input{"tenant_id": lifecycleTenant.String(), "external_reference": reference}
	if len(version) > 0 {
		in["expected_version"] = version[0]
	}
	return in
}

// UC-INS-13 check 10: the platform's reference is set, changed and cleared; the trail says that it
// changed and never what it is (rule 10), and the journal records the act without the value.
func TestAPlatformsReferenceIsSetChangedAndCleared(t *testing.T) {
	f := newReferenceFixture(t)
	out, err := f.registry.Invoke(t.Context(), UpdateTenantName, operator(), referenceTo("cus_8f2a91", 4))
	if err != nil {
		t.Fatalf("setting: %v", err)
	}
	if out["external_reference"] != "cus_8f2a91" || out["version"] != 5 {
		t.Errorf("answered %v", out)
	}
	if _, err := f.registry.Invoke(t.Context(), UpdateTenantName, operator(), referenceTo(nil)); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if f.tenants.record.ExternalReference != "" {
		t.Errorf("the reference stands: %q", f.tenants.record.ExternalReference)
	}

	if len(f.audit.entries) != 2 {
		t.Fatalf("trail %+v", f.audit.entries)
	}
	change, _ := f.audit.entries[0].Changes["external_reference"].(map[string]any)
	if change["changed"] != true || change["to"] != nil || change["from"] != nil {
		t.Errorf("the trail holds %v", f.audit.entries[0].Changes)
	}
	for _, entry := range f.journal.entries {
		if entry.Action == string(TenantReferenceChangedAction) && len(entry.Details) != 0 {
			t.Errorf("the journal holds %v", entry.Details)
		}
	}
}

func TestTheSameReferenceAgainWritesNothing(t *testing.T) {
	f := newReferenceFixture(t)
	f.tenants.record.ExternalReference = "cus_8f2a91"
	if _, err := f.registry.Invoke(t.Context(), UpdateTenantName, operator(), referenceTo("cus_8f2a91")); err != nil {
		t.Fatal(err)
	}
	if len(f.tenants.moved) != 0 || len(f.audit.entries) != 0 {
		t.Error("an unchanged reference was written")
	}
}

func TestAReferenceIsRefused(t *testing.T) {
	for name, c := range map[string]struct {
		in    usecase.Input
		setup func(*tenantsStore)
		want  string
	}{
		"held by another workspace": {in: referenceTo("cus_taken"), want: "admin.reference_taken"},
		"with a line break":         {in: referenceTo("cus\n8f2a91"), want: "admin.reference_invalid"},
		"on a stale version":        {in: referenceTo("cus_8f2a91", 3), want: "admin.tenant_version_moved"},
		"once the purge has begun": {in: referenceTo("cus_8f2a91"), want: "admin.tenant_deletion_underway",
			setup: func(s *tenantsStore) { s.record.PurgeStartedAt = time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC) }},
	} {
		t.Run(name, func(t *testing.T) {
			f := newReferenceFixture(t)
			if c.setup != nil {
				c.setup(f.tenants)
			}
			_, err := f.registry.Invoke(t.Context(), UpdateTenantName, operator(), c.in)
			if refusalCode(err) != c.want {
				t.Errorf("answered %v, want %s", err, c.want)
			}
			var refusal *shared.Error
			if errors.As(err, &refusal) && len(refusal.Params) != 0 {
				t.Errorf("the refusal carries %v", refusal.Params)
			}
		})
	}
}

// A workspace leaving keeps its reference through the grace; the platform can clear it.
func TestAReferenceIsClearedOnAWorkspaceLeaving(t *testing.T) {
	f := newReferenceFixture(t)
	f.tenants.record.Status = domain.TenantPendingDeletion
	f.tenants.record.ExternalReference = "cus_8f2a91"
	if _, err := f.registry.Invoke(t.Context(), UpdateTenantName, operator(), referenceTo(nil)); err != nil {
		t.Errorf("clearing on a workspace pending deletion: %v", err)
	}
}

func TestAReferenceIsMultiModes(t *testing.T) {
	f := newReferenceFixture(t)
	f.writer.Tenancy = env.TenancySingle
	registry, err := usecase.NewRegistry(nil, UpdateTenant{
		Writer: f.writer, Tenants: f.tenants, Finder: tenantFinder{f.tenants}, Audit: f.audit,
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Invoke(t.Context(), UpdateTenantName, operator(), referenceTo("x")); refusalCode(err) != "admin.multi_mode_required" {
		t.Errorf("single mode answered %v", err)
	}
}

func TestAWorkspaceIsProvisionedWithAReference(t *testing.T) {
	f := newProvisionFixture()
	f.tenants.record.Version = 1
	cmd := provisionCommand()
	cmd.ExternalReference = "cus_8f2a91"
	provisioned, err := f.handler.Execute(t.Context(), operator(), cmd)
	if err != nil || provisioned.ExternalReference != "cus_8f2a91" || f.tenants.record.ExternalReference != "cus_8f2a91" {
		t.Errorf("provisioned %+v, %v", provisioned, err)
	}

	invalid := newProvisionFixture()
	cmd.ExternalReference = " cus"
	if _, err := invalid.handler.Execute(t.Context(), operator(), cmd); refusalCode(err) != "admin.reference_invalid" ||
		len(invalid.tenants.inserted) != 0 {
		t.Errorf("an invalid reference answered %v, with %d workspaces written", err, len(invalid.tenants.inserted))
	}
}

func TestOneWorkspaceIsReadAndTheQueryIsBounded(t *testing.T) {
	f := newReferenceFixture(t)
	reader := TenantReader{Writer: f.writer, Finder: tenantFinder{f.tenants}}
	f.tenants.record.ExternalReference = "cus_8f2a91"
	read, err := ReadTenant{reader}.Execute(t.Context(), operator(), lifecycleTenant)
	if err != nil || read.ExternalReference != "cus_8f2a91" {
		t.Errorf("read %+v, %v", read, err)
	}
	if _, err := (ReadTenant{reader}).Execute(t.Context(), operator(), shared.ID("018f2a1b-0000-7000-8000-0000000000ff")); refusalCode(err) != "admin.tenant_not_found" {
		t.Errorf("an unknown workspace answered %v", err)
	}
	if _, _, err := (QueryTenants{reader}).Execute(t.Context(), operator(), adminrepo.TenantQuery{Status: "GONE"}); refusalCode(err) != "admin.tenant_status_unknown" {
		t.Errorf("an unknown state answered %v", err)
	}
	found, _, err := QueryTenants{reader}.Execute(t.Context(), operator(), adminrepo.TenantQuery{Reference: "cus_8f2a91", Size: 1000})
	if err != nil || len(found) != 1 {
		t.Errorf("by reference: %v, %v", found, err)
	}
}

func TestAQueryForAPlanAndForNoneIsRefused(t *testing.T) {
	f := newReferenceFixture(t)
	reader := TenantReader{Writer: f.writer, Finder: tenantFinder{f.tenants}}
	_, _, err := QueryTenants{reader}.Execute(t.Context(), operator(), adminrepo.TenantQuery{Plan: "family", WithoutPlan: true})
	if refusalCode(err) != "admin.tenant_query_conflicting" {
		t.Errorf("a plan and no plan answered %v", err)
	}
}
