// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// UC-INS-19 checks 1, 2, 5, 6, 7, 8 and 11; UC-INS-20 check 4: the operator changes a workspace's
// address with the checks of provisioning, confirmed by a password and never a passkey, refused
// while it would lock out the caller or the last owner; the old host answers nothing at once and
// another workspace takes it; the members are told once; the trail and the journal record it.

var (
	addressOwner   = shared.ID("018f2a1b-0000-7000-8000-0000000000c1")
	secondOwner    = shared.ID("018f2a1b-0000-7000-8000-0000000000c2")
	otherTenant    = shared.ID("018f2a1b-0000-7000-8000-0000000000b9")
	mailedMember   = shared.ID("018f2a1b-0000-7000-8000-0000000000d1")
	unmailedMember = shared.ID("018f2a1b-0000-7000-8000-0000000000d2")
)

// hostsTable is the installation's `tenant_host` and slugs, across workspaces: what resolves where.
type hostsTable struct {
	main    map[shared.ID]string
	slugs   map[shared.ID]string
	tenant  shared.ID
	revoked int
}

func (h *hostsTable) resolve(host string) shared.ID {
	for tenant, main := range h.main {
		if main == host {
			return tenant
		}
	}
	return ""
}

func (h *hostsTable) ChangeAddress(
	_ context.Context, slug string, row domain.TenantHost, _ time.Time,
) (string, error) {
	if owner := h.resolve(row.Host); owner != "" {
		return "", shared.ErrConflict.WithDetail("admin.host_taken")
	}
	for tenant, held := range h.slugs {
		if held == slug && tenant != h.tenant {
			return "", shared.ErrConflict.WithDetail("admin.slug_taken")
		}
	}
	previous := h.main[h.tenant]
	h.main[h.tenant], h.slugs[h.tenant] = row.Host, slug
	return previous, nil
}

func (h *hostsTable) EndSessions(context.Context, time.Time) (int, error) {
	h.revoked++
	return 3, nil
}

type audienceFake struct{ mailed, unmailed []shared.ID }

func (a audienceFake) PeopleToTell(context.Context) ([]shared.ID, []shared.ID, error) {
	return a.mailed, a.unmailed, nil
}

type addressNoticesFake struct {
	calls    int
	mailed   []shared.ID
	unmailed []shared.ID
	from, to string
}

func (n *addressNoticesFake) AddressChanged(
	_ context.Context, _ shared.ID, from, to string, _ time.Time, mailed, unmailed []shared.ID,
) error {
	n.calls++
	n.from, n.to, n.mailed, n.unmailed = from, to, mailed, unmailed
	return nil
}

type ownersFake struct{ owners []shared.ID }

func (o ownersFake) LockOwners(context.Context) error { return nil }
func (o ownersFake) OwnerPaths(context.Context) ([]identityrepo.OwnerPath, error) {
	paths := []identityrepo.OwnerPath{}
	for _, owner := range o.owners {
		paths = append(paths, identityrepo.OwnerPath{AccountID: owner})
	}
	return paths, nil
}

type waysFake map[shared.ID]domain.WaysHeld

func (w waysFake) WaysOf(_ context.Context, _, accountID shared.ID) (domain.WaysHeld, error) {
	return w[accountID], nil
}

// methodStepUp is the verifier that says which method gave the proof.
type methodStepUp struct {
	stepUpFake
	method stepup.Method
}

func (s *methodStepUp) SatisfiedBy(ctx context.Context, account shared.ID, token string) (stepup.Method, bool, error) {
	satisfied, err := s.Satisfied(ctx, account, token)
	return s.method, satisfied, err
}

type addressFixture struct {
	handler ChangeTenantAddress
	hosts   *hostsTable
	notices *addressNoticesFake
	stepUp  *methodStepUp
	journal *journalStore
	audit   *auditSink
	tenants *tenantsStore
	ways    waysFake
}

const addressProof = "hbt_sup_address"

func newAddressFixture(hosts env.WorkspaceHosts) *addressFixture {
	instance, _, _ := newInstanceWriter(newRegister(operatorID))
	f := &addressFixture{
		hosts: &hostsTable{
			tenant: lifecycleTenant,
			main:   map[shared.ID]string{lifecycleTenant: "meier.tasks.example.net"},
			slugs:  map[shared.ID]string{lifecycleTenant: "meier"},
		},
		notices: &addressNoticesFake{},
		stepUp:  &methodStepUp{stepUpFake: stepUpFake{expect: addressProof}, method: stepup.MethodPassword},
		journal: &journalStore{}, audit: &auditSink{},
		tenants: &tenantsStore{record: adminrepo.TenantRecord{
			ID: lifecycleTenant, Slug: "meier", DisplayName: "Meier family",
			Status: domain.TenantActive, Host: "meier.tasks.example.net",
		}},
		ways: waysFake{addressOwner: {Password: true}},
	}
	f.handler = ChangeTenantAddress{
		Instance: instance, Tenants: f.tenants, Addresses: f.hosts,
		Audience: audienceFake{mailed: []shared.ID{addressOwner, mailedMember}, unmailed: []shared.ID{unmailedMember}},
		Notices:  f.notices, Owners: ownersFake{owners: []shared.ID{addressOwner}}, Ways: f.ways,
		Journal: f.journal, Audit: f.audit, StepUp: f.stepUp, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), IDs: &ids{}, Entropy: clock.FixedEntropy{},
		Tenancy: env.TenancyMulti, WorkspaceHosts: hosts, InstallationHost: "tasks.example.net",
	}
	return f
}

func (f *addressFixture) change(t *testing.T, actor appshared.ActorContext, cmd ChangeTenantAddressCommand) (AddressChange, error) {
	t.Helper()
	cmd.TenantID = lifecycleTenant
	return f.handler.Execute(t.Context(), actor, cmd)
}

func TestAnOperatorChangesAWorkspacesAddress(t *testing.T) {
	f := newAddressFixture(env.HostsSubdomain)

	changed, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: "familie-meier", StepUpToken: addressProof})
	if err != nil {
		t.Fatalf("change: %v", err)
	}
	if changed.From != "meier.tasks.example.net" || changed.To != "familie-meier.tasks.example.net" {
		t.Fatalf("from %q to %q", changed.From, changed.To)
	}
	// Check 7: the old host resolves to nothing at once; the new one to the workspace.
	if f.hosts.resolve("meier.tasks.example.net") != "" || f.hosts.resolve("familie-meier.tasks.example.net") != lifecycleTenant {
		t.Fatalf("hosts after the change: %v", f.hosts.main)
	}
	// ... and another workspace takes the old name at once.
	f.hosts.tenant = otherTenant
	if _, err := f.hosts.ChangeAddress(t.Context(), "meier", domain.TenantHost{Host: "meier.tasks.example.net"}, now); err != nil {
		t.Fatalf("another workspace taking the old name: %v", err)
	}
	if f.hosts.revoked != 1 || f.stepUp.consumed != 1 {
		t.Errorf("sessions ended %d times, step-up consumed %d", f.hosts.revoked, f.stepUp.consumed)
	}
	// Check 8: the members are told once, by mail or at sign-in.
	if f.notices.calls != 1 || len(f.notices.mailed) != 2 || len(f.notices.unmailed) != 1 ||
		f.notices.from != "meier.tasks.example.net" || f.notices.to != "familie-meier.tasks.example.net" {
		t.Errorf("notices %+v", f.notices)
	}
	// Check 11: the trail and the journal.
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantAddressChangedAction ||
		f.audit.entries[0].Severity != audit.SeverityWarning ||
		changedTo(f.audit.entries[0], "host") != "familie-meier.tasks.example.net" ||
		changedTo(f.audit.entries[0], "slug") != "familie-meier" {
		t.Errorf("trail %+v", f.audit.entries)
	}
	if len(f.journal.entries) != 1 || f.journal.entries[0].Action != journalAddressChanged ||
		f.journal.entries[0].Details["from"] != "meier.tasks.example.net" ||
		f.journal.entries[0].Details["to"] != "familie-meier.tasks.example.net" {
		t.Errorf("journal %+v", f.journal.entries)
	}
}

func TestADryRunChangesNothingAndCountsWhoIsTold(t *testing.T) {
	f := newAddressFixture(env.HostsSubdomain)

	counted, err := f.change(t, operator(), ChangeTenantAddressCommand{DryRun: true})
	if err != nil || counted.Mailed != 2 || counted.ToldAtSignIn != 1 || counted.From != "meier.tasks.example.net" {
		t.Fatalf("counts %+v, %v", counted, err)
	}
	checked, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: "familie-meier", DryRun: true})
	if err != nil || checked.To != "familie-meier.tasks.example.net" {
		t.Fatalf("dry run %+v, %v", checked, err)
	}
	if f.stepUp.consumed != 0 || f.notices.calls != 0 || len(f.audit.entries) != 0 ||
		f.hosts.main[lifecycleTenant] != "meier.tasks.example.net" {
		t.Fatal("a dry run changed something")
	}
}

func TestTheNewAddressIsRefusedAtTheField(t *testing.T) {
	f := newAddressFixture(env.HostsSubdomain)
	f.handler.Settings = nil
	for slug, code := range map[string]string{
		"admin": "admin.slug_reserved", "-x": "admin.slug_invalid", "meier": "admin.address_unchanged",
	} {
		if _, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: slug, DryRun: true}); detailOf(err) != code {
			t.Errorf("%q: %v, want %s", slug, err, code)
		}
	}
	f.hosts.slugs[otherTenant] = "taken"
	if _, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: "taken", StepUpToken: addressProof}); detailOf(err) != "admin.slug_taken" {
		t.Errorf("a taken name: %v", err)
	}
}

// UC-INS-20 check 4: in assigned mode the new address is a host, with CD-04's refusals.
func TestAnAssignedHostChangesAsAnAddress(t *testing.T) {
	f := newAddressFixture(env.HostsAssigned)
	f.hosts.main[otherTenant] = "tasks-sales.corp.example"

	if _, err := f.change(t, operator(), ChangeTenantAddressCommand{Host: "tasks.example.net", DryRun: true}); detailOf(err) != "admin.host_installation" {
		t.Errorf("the installation's host: %v", err)
	}
	if _, err := f.change(t, operator(), ChangeTenantAddressCommand{Host: "tasks-sales.corp.example", StepUpToken: addressProof}); detailOf(err) != "admin.host_taken" {
		t.Errorf("a taken host: %v", err)
	}
	f.stepUp.expect = addressProof
	changed, err := f.change(t, operator(), ChangeTenantAddressCommand{Host: "tasks-people.corp.example", StepUpToken: addressProof})
	if err != nil || changed.To != "tasks-people.corp.example" || f.hosts.slugs[lifecycleTenant] != "meier" {
		t.Fatalf("assigned change %+v, %v, slug %q", changed, err, f.hosts.slugs[lifecycleTenant])
	}
}

// Check 5: a passkey does not confirm it.
func TestAPasskeyDoesNotConfirmAChangeOfAddress(t *testing.T) {
	f := newAddressFixture(env.HostsSubdomain)
	f.stepUp.method = stepup.MethodPasskey

	_, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: "familie-meier", StepUpToken: addressProof})
	if detailOf(err) != stepup.CodeMethodRefused {
		t.Fatalf("a passkey's proof: %v", err)
	}
	if f.hosts.main[lifecycleTenant] != "meier.tasks.example.net" || f.notices.calls != 0 {
		t.Fatal("the change went through on a passkey")
	}
	_, err = f.change(t, operator(), ChangeTenantAddressCommand{Slug: "familie-meier"})
	var refused *shared.Error
	if !errors.As(err, &refused) || refused.DetailCode != stepup.CodeRequired || refused.Params["purpose"] != "change_address" {
		t.Fatalf("no proof: %v", err)
	}
}

// Check 6: refused while it would lock out the last owner, or the person making it.
func TestAChangeThatLocksSomebodyOutIsRefused(t *testing.T) {
	f := newAddressFixture(env.HostsSubdomain)
	f.ways[addressOwner] = domain.WaysHeld{Passkeys: true}

	_, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: "familie-meier", StepUpToken: addressProof})
	var refused *shared.Error
	if !errors.As(err, &refused) || refused.DetailCode != "admin.address_lockout_owner" || refused.Params["reason"] != "way_in" {
		t.Fatalf("the last owner: %v", err)
	}
	if f.stepUp.consumed != 0 {
		t.Error("a refused change burned the step-up")
	}

	// A second owner with a lasting way in lets it through.
	f.handler.Owners = ownersFake{owners: []shared.ID{addressOwner, secondOwner}}
	f.ways[secondOwner] = domain.WaysHeld{Provider: true}
	if _, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: "familie-meier", DryRun: true}); err != nil {
		t.Fatalf("with another owner: %v", err)
	}

	// The person making it, in their own workspace, with a passkey as their required factor.
	self := operator()
	self.TenantID, self.AccountID = lifecycleTenant, secondOwner
	f.ways[secondOwner] = domain.WaysHeld{Password: true, Passkeys: true, FactorRequired: true}
	f.ways[addressOwner] = domain.WaysHeld{Password: true}
	f.handler.Instance.Operators = newRegister(secondOwner)
	_, err = f.change(t, self, ChangeTenantAddressCommand{Slug: "familie-meier", DryRun: true})
	if !errors.As(err, &refused) || refused.DetailCode != "admin.address_lockout_self" || refused.Params["reason"] != "factor" {
		t.Fatalf("the person making it: %v", err)
	}
}

func TestAChangeOfAddressIsRefusedWhereItCannotBe(t *testing.T) {
	f := newAddressFixture(env.HostsSubdomain)
	f.tenants.record.Status = domain.TenantPendingDeletion
	if _, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: "familie-meier", DryRun: true}); detailOf(err) != "admin.tenant_leaving" {
		t.Errorf("pending deletion: %v", err)
	}
	f = newAddressFixture(env.HostsSubdomain)
	f.handler.Tenancy = env.TenancySingle
	if _, err := f.change(t, operator(), ChangeTenantAddressCommand{Slug: "familie-meier"}); detailOf(err) != "admin.multi_mode_required" {
		t.Errorf("single mode: %v", err)
	}
}
