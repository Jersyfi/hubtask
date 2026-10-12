// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// pausedFake stands for the automation package's switch: how often it was asked, and how many it
// says it switched back on.
type pausedFake struct {
	calls   int
	resumed int
	err     error
}

func (p *pausedFake) Resume(context.Context, shared.ID, time.Time) (int, error) {
	p.calls++
	return p.resumed, p.err
}

type cancelFixture struct {
	tenants *tenantsStore
	paused  *pausedFake
	journal *journalStore
	audit   *auditSink
	work    *unitOfWork
	handler CancelTenantDeletion
}

func newCancelFixture(before domain.TenantStatus) *cancelFixture {
	f := &cancelFixture{
		tenants: &tenantsStore{record: adminrepo.TenantRecord{
			ID: lifecycleTenant, Slug: "acme", DisplayName: "Acme GmbH",
			Status: domain.TenantPendingDeletion, StatusBeforeDeletion: before,
			PurgeAfter: now.Add(DeletionGrace),
		}},
		paused:  &pausedFake{resumed: 3},
		journal: &journalStore{}, audit: &auditSink{}, work: &unitOfWork{},
	}
	f.handler = CancelTenantDeletion{
		Tenants: f.tenants, Automations: f.paused, Journal: f.journal, Audit: f.audit,
		UnitOfWork: f.work, Clock: clock.Fixed(now), IDs: &sequentialIDs{},
	}
	return f
}

func (f *cancelFixture) registry(t *testing.T) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, f.handler.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return registry
}

func (f *cancelFixture) cancel(t *testing.T, actor appshared.ActorContext) (usecase.Output, error) {
	t.Helper()
	return f.registry(t).Invoke(t.Context(), CancelTenantDeletionName, actor, usecase.Input{
		"tenant_id": lifecycleTenant.String(),
	})
}

// The workspace returns where the request found it, the rules come back on, and both records say so.
func TestACancelReturnsTheWorkspaceWhereTheRequestFoundIt(t *testing.T) {
	cases := []struct {
		name   string
		before domain.TenantStatus
		want   domain.TenantStatus
	}{
		{"an active workspace comes back active", domain.TenantActive, domain.TenantActive},
		{"a suspended one comes back suspended", domain.TenantSuspended, domain.TenantSuspended},
		// Requested by an older binary, or pending before the status was kept: nobody regains
		// access the operator did not choose.
		{"one whose earlier status is unknown comes back suspended", "", domain.TenantSuspended},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newCancelFixture(c.before)

			out, err := f.cancel(t, operator())
			if err != nil {
				t.Fatalf("cancelling: %v", err)
			}
			if out.String("status") != string(c.want) || f.tenants.record.Status != c.want {
				t.Errorf("answered %v, the row reads %s, want %s", out, f.tenants.record.Status, c.want)
			}
			if !f.tenants.record.PurgeAfter.IsZero() {
				t.Error("the deadline still stands")
			}
			if f.paused.calls != 1 {
				t.Errorf("the rules were asked %d times, want once", f.paused.calls)
			}
			if len(f.work.scopes) != 1 || f.work.scopes[0].TenantID != lifecycleTenant {
				t.Errorf("scopes %+v", f.work.scopes)
			}
			if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantDeletionCancelledAction ||
				f.audit.entries[0].Severity != audit.SeverityInfo {
				t.Errorf("audit %+v", f.audit.entries)
			}
			if len(f.journal.entries) != 1 {
				t.Fatalf("journal %+v", f.journal.entries)
			}
			entry := f.journal.entries[0]
			if entry.Action != journalDeletionCancelled || entry.TenantSlug != "acme" ||
				entry.Details["status"] != string(c.want) || entry.Details["automations_resumed"] != 3 {
				t.Errorf("journal entry %+v", entry)
			}
		})
	}
}

// Nothing pending is nothing to call off: the state the operator wanted stands, and nothing is
// written - an entry per retry would put noise where evidence lives.
func TestCancellingAWorkspaceNotPendingChangesNothing(t *testing.T) {
	for _, status := range []domain.TenantStatus{domain.TenantActive, domain.TenantSuspended} {
		t.Run(string(status), func(t *testing.T) {
			f := newCancelFixture("")
			f.tenants.record.Status, f.tenants.record.PurgeAfter = status, time.Time{}

			out, err := f.cancel(t, operator())
			if err != nil || out.String("status") != string(status) {
				t.Fatalf("answered (%v, %v)", out, err)
			}
			if f.tenants.cancels != 0 || f.paused.calls != 0 ||
				len(f.audit.entries) != 0 || len(f.journal.entries) != 0 {
				t.Error("a cancel of nothing wrote something")
			}
		})
	}
}

// Once the grace job has marked the purge, the bytes are going: no cancel lands, whether the mark
// was there when the cancel read the row or arrived between the read and the statement.
func TestACancelIsRefusedOnceThePurgeHasBegun(t *testing.T) {
	cases := map[string]func(*cancelFixture){
		"marked before the read":   func(f *cancelFixture) { f.tenants.record.PurgeStartedAt = now },
		"marked during the cancel": func(f *cancelFixture) { f.tenants.purgeRacing = true },
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCancelFixture(domain.TenantActive)
			arrange(f)

			_, err := f.cancel(t, operator())
			var refusal *shared.Error
			if !errors.As(err, &refusal) || !errors.Is(err, shared.ErrConflict) ||
				refusal.DetailCode != "admin.tenant_deletion_underway" {
				t.Fatalf("answer %v, want admin.tenant_deletion_underway", err)
			}
			if f.tenants.record.Status != domain.TenantPendingDeletion || f.paused.calls != 0 ||
				len(f.audit.entries) != 0 || len(f.journal.entries) != 0 {
				t.Error("a refused cancel changed something")
			}
		})
	}
}

// A legal hold does not stop a cancel: it is how a held workspace leaves PENDING_DELETION
// (data-protection.md §5). The use case reads no hold at all; past the deadline is no obstacle.
func TestAHeldWorkspacePastItsDeadlineIsCancelled(t *testing.T) {
	f := newCancelFixture(domain.TenantActive)
	f.tenants.record.PurgeAfter = now.Add(-48 * time.Hour)

	out, err := f.cancel(t, operator())
	if err != nil || out.String("status") != string(domain.TenantActive) {
		t.Fatalf("answered (%v, %v)", out, err)
	}
}

func TestACancelNeedsTheAdminScopeAndAWorkspace(t *testing.T) {
	f := newCancelFixture(domain.TenantActive)
	member := operator()
	member.Scopes = nil
	if _, err := f.cancel(t, member); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("without the scope: %v", err)
	}
	f.tenants.findErr = shared.ErrNotFound
	if _, err := f.cancel(t, operator()); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a workspace that is gone: %v", err)
	}
	if f.tenants.cancels != 0 || f.paused.calls != 0 {
		t.Error("a refused cancel reached a write")
	}
}

// A cancel that cannot switch the rules back on is refused whole, never half done.
func TestACancelWithoutTheRulesSwitchIsRefused(t *testing.T) {
	f := newCancelFixture(domain.TenantActive)
	f.handler.Automations = nil
	if _, err := f.cancel(t, operator()); !errors.Is(err, shared.ErrInternal) {
		t.Errorf("answer %v, want the internal refusal", err)
	}

	failing := newCancelFixture(domain.TenantActive)
	failing.paused.err = shared.ErrUnavailable
	if _, err := failing.cancel(t, operator()); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("a failing switch answered %v", err)
	}
	if len(failing.journal.entries) != 0 {
		t.Error("the journal records a cancel whose transaction failed")
	}
}
