// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"testing"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type recoverFixture struct {
	tenants *tenantsStore
	journal *journalStore
	audit   *auditSink
	work    *unitOfWork
	paused  *pausedFake
	handler RecoverWorkspace
}

// newRecoverFixture is an installation of one workspace, `acme`, in the given state, and another
// whose short name only starts the same.
func newRecoverFixture(record adminrepo.TenantRecord) *recoverFixture {
	record.ID, record.Slug = lifecycleTenant, "acme"
	f := &recoverFixture{
		tenants: &tenantsStore{record: record, moveOK: true, listed: []adminrepo.TenantRecord{
			{ID: shared.ID("018f2a1b-0000-7000-8000-0000000000b2"), Slug: "acme-two"},
			record,
		}},
		journal: &journalStore{}, audit: &auditSink{}, work: &unitOfWork{}, paused: &pausedFake{resumed: 2},
	}
	shift := LifecycleShift{
		Tenants: f.tenants, Journal: f.journal, Audit: f.audit,
		UnitOfWork: f.work, Clock: clock.Fixed(now), IDs: &sequentialIDs{},
	}
	f.handler = RecoverWorkspace{
		Tenants: f.tenants, UnitOfWork: f.work,
		Resume: ResumeTenant{shift},
		Cancel: CancelTenantDeletion{
			Tenants: f.tenants, Automations: f.paused, Journal: f.journal, Audit: f.audit,
			UnitOfWork: f.work, Clock: clock.Fixed(now), IDs: &sequentialIDs{},
		},
	}
	return f
}

// UC-INS-03, the rest of the way back: the machine resumes a suspended workspace by its short name,
// with the edge, the trail entry and the journal entry the API's resume leaves, recorded as the
// machine's.
func TestTheMachineResumesASuspendedWorkspace(t *testing.T) {
	f := newRecoverFixture(adminrepo.TenantRecord{Status: domain.TenantSuspended})

	if err := f.handler.ResumeBySlug(t.Context(), " Acme "); err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if len(f.tenants.moved) != 1 || f.tenants.moved[0] != "SUSPENDED->ACTIVE" {
		t.Errorf("moves %v", f.tenants.moved)
	}
	if len(f.work.scopes) != 2 || !f.work.scopes[0].Installation || f.work.scopes[1].TenantID != lifecycleTenant {
		t.Errorf("scopes %+v, want the installation's read, then the workspace's transaction", f.work.scopes)
	}
	if len(f.audit.entries) != 1 {
		t.Fatalf("audit %+v", f.audit.entries)
	}
	entry := f.audit.entries[0]
	if entry.Action != TenantResumedAction || entry.ActorKind != appshared.ActorSystem ||
		entry.ActorLabel != machineLabel || !entry.ActorID.IsZero() || entry.TenantID != lifecycleTenant {
		t.Errorf("audit entry %+v", entry)
	}
	if len(f.journal.entries) != 1 || f.journal.entries[0].Action != journalResumed ||
		f.journal.entries[0].ActorLabel != machineLabel || f.journal.entries[0].TenantSlug != "acme" {
		t.Errorf("journal %+v", f.journal.entries)
	}
}

// The machine calls a pending deletion off by the short name, and says where the workspace stands.
func TestTheMachineCancelsAPendingDeletion(t *testing.T) {
	for _, before := range []domain.TenantStatus{domain.TenantActive, domain.TenantSuspended, ""} {
		t.Run("requested from "+string(before), func(t *testing.T) {
			f := newRecoverFixture(adminrepo.TenantRecord{
				Status: domain.TenantPendingDeletion, StatusBeforeDeletion: before, PurgeAfter: now,
			})
			want := before
			if want == "" {
				want = domain.TenantSuspended
			}

			status, err := f.handler.CancelDeletionBySlug(t.Context(), "acme")
			if err != nil || status != want {
				t.Fatalf("cancelling answered (%s, %v), want %s", status, err, want)
			}
			if f.paused.calls != 1 {
				t.Errorf("the rules were asked %d times", f.paused.calls)
			}
			if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantDeletionCancelledAction ||
				f.audit.entries[0].ActorKind != appshared.ActorSystem || f.audit.entries[0].ActorLabel != machineLabel {
				t.Errorf("audit %+v", f.audit.entries)
			}
			if len(f.journal.entries) != 1 || f.journal.entries[0].ActorLabel != machineLabel ||
				f.journal.entries[0].Details["status"] != string(want) {
				t.Errorf("journal %+v", f.journal.entries)
			}
		})
	}
}

// The machine meets the guards the API meets, and a repeat records nothing.
func TestTheMachineDoorKeepsTheGuards(t *testing.T) {
	cases := []struct {
		name   string
		record adminrepo.TenantRecord
		slug   string
		act    string
		code   string
	}{
		{"an unknown short name", adminrepo.TenantRecord{Status: domain.TenantSuspended}, "acm", "resume", "admin.tenant_not_found"},
		{"an empty short name", adminrepo.TenantRecord{Status: domain.TenantSuspended}, " ", "cancel", "admin.tenant_not_found"},
		{"resuming a pending workspace", adminrepo.TenantRecord{Status: domain.TenantPendingDeletion}, "acme", "resume", "admin.tenant_leaving"},
		{"cancelling once the purge began", adminrepo.TenantRecord{
			Status: domain.TenantPendingDeletion, PurgeStartedAt: now,
		}, "acme", "cancel", "admin.tenant_deletion_underway"},
		{"resuming an active workspace", adminrepo.TenantRecord{Status: domain.TenantActive}, "acme", "resume", ""},
		{"cancelling where nothing is pending", adminrepo.TenantRecord{Status: domain.TenantSuspended}, "acme", "cancel", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRecoverFixture(c.record)
			var err error
			if c.act == "resume" {
				err = f.handler.ResumeBySlug(t.Context(), c.slug)
			} else {
				_, err = f.handler.CancelDeletionBySlug(t.Context(), c.slug)
			}
			got := ""
			if err != nil {
				got = shared.AsError(err).DetailCode
			}
			if got != c.code || (c.code == "" && err != nil) {
				t.Errorf("answered %v, want %q", err, c.code)
			}
			if len(f.audit.entries) != 0 || len(f.journal.entries) != 0 || len(f.tenants.moved) != 0 {
				t.Errorf("recorded %+v / %+v / %v", f.audit.entries, f.journal.entries, f.tenants.moved)
			}
		})
	}
}

// The door has no route: a type that offered a descriptor could be registered, and the registry
// would then hand the machine's own `admin:tenants` actor to whoever reaches a route.
func TestTheMachineDoorCannotBeRegistered(t *testing.T) {
	if _, ok := any(RecoverWorkspace{}).(interface{ Descriptor() usecase.Descriptor }); ok {
		t.Error("RecoverWorkspace offers a descriptor")
	}
}
