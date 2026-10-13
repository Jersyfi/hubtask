// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// managedStore is the account table's managed side over the in-memory account store, so the grant's
// own lookup finds the account the creation wrote in the same transaction.
type managedStore struct {
	*accountStore
	hashes    map[shared.ID]string
	drawnAt   map[shared.ID]time.Time
	insertErr error
}

func (s *managedStore) FindBySignInName(_ context.Context, name string) (domain.Account, error) {
	for _, account := range s.byID {
		if account.SignInName != "" && account.SignInName == name {
			return account, nil
		}
	}
	return domain.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
}

func (s *managedStore) InsertManaged(_ context.Context, account domain.Account, hash string, at time.Time) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.byID[account.ID] = account
	s.inserted = append(s.inserted, account)
	s.hashes[account.ID], s.drawnAt[account.ID] = hash, at
	return nil
}

func (s *managedStore) HasManaged(context.Context) (bool, error) {
	for _, account := range s.byID {
		if account.SignInName != "" {
			return true, nil
		}
	}
	return false, nil
}

var _ repository.ManagedAccounts = (*managedStore)(nil)

var managedID = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")

type managedFixture struct {
	registry   *usecase.Registry
	accounts   *managedStore
	grants     *grantStore
	authorizer *authorizer
	audit      *auditSink
	policy     *workspacePolicyFixture
	stepUp     *stepUpFake
}

func newManagedFixture(t *testing.T) *managedFixture {
	t.Helper()
	f := &managedFixture{
		accounts: &managedStore{
			accountStore: newAccounts(),
			hashes:       map[shared.ID]string{}, drawnAt: map[shared.ID]time.Time{},
		},
		grants: newGrants(), authorizer: &authorizer{}, audit: &auditSink{},
		policy: newWorkspacePolicyFixture(now), stepUp: &stepUpFake{satisfied: true},
	}
	ids := ids{next: managedID}
	registry, err := usecase.NewRegistry(nil, CreateManagedAccount{
		Managed: f.accounts, Authorizer: f.authorizer,
		Grant: GrantMembership{
			Grants: f.grants, Accounts: f.accounts.accountStore, Authorizer: f.authorizer,
			Audit: f.audit, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: ids,
			StepUp: f.stepUp,
		},
		Rule: f.policy.writer.Resolver, Passwords: &passwordsFake{}, Entropy: &countingEntropy{},
		Audit: f.audit, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: ids,
		Text: text.Composing{},
	}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.registry = registry
	return f
}

func lenaInput() usecase.Input {
	return usecase.Input{
		"display_name": "Lena", "sign_in_name": "Lena", "role": "CONTRIBUTOR",
		"scope_type": "HUB", "scope_id": "01936f2a-7c1e-7000-8000-0000000000d1",
	}
}

func (f *managedFixture) create(t *testing.T, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), CreateManagedAccountName, actor, in)
}

// UC-ID-20 checks 1 and 2: an owner or administrator gives Lena an account with a sign-in name and
// a role at a hub; Hubtask draws the start password, answers it once, and keeps only its hash.
func TestAnAdministratorCreatesAManagedAccountAndSeesItsStartPasswordOnce(t *testing.T) {
	f := newManagedFixture(t)

	out, err := f.create(t, admin(), lenaInput())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	account, _ := out["account"].(usecase.Output)
	if account.String("sign_in_name") != "lena" || account.String("status") != "ACTIVE" {
		t.Errorf("the account reads %v", account)
	}
	if _, held := account["email"]; held {
		t.Error("a managed account answers an address")
	}
	start := out.String("start_password")
	if len(start) != 19 || strings.Count(start, "-") != 3 {
		t.Errorf("the start password %q is not four groups of four", start)
	}
	if f.accounts.hashes[managedID] != "hash:"+start {
		t.Error("what was kept is not the start password's hash")
	}
	if !f.accounts.drawnAt[managedID].Equal(now) {
		t.Error("the start password's mark was not set")
	}

	membership, _ := out["membership"].(usecase.Output)
	if membership.String("role") != "CONTRIBUTOR" || membership.String("scope_type") != "HUB" ||
		membership.String("account_id") != managedID.String() {
		t.Errorf("the membership reads %v", membership)
	}

	if len(f.audit.entries) != 2 || f.audit.entries[0].Action != ManagedAccountCreatedAction ||
		f.audit.entries[1].Action != MembershipGrantedAction {
		t.Fatalf("the trail holds %v", f.audit.entries)
	}
	for _, entry := range f.audit.entries {
		if written := fmt.Sprint(entry.Changes); strings.Contains(written, start) || strings.Contains(written, "password") {
			t.Errorf("the trail carries the start password: %s", written)
		}
	}
	name, _ := f.audit.entries[0].Changes["sign_in_name"].(map[string]any)
	if _, hashed := name["to_hash"]; !hashed || strings.Contains(fmt.Sprint(name), "lena") {
		t.Errorf("the trail keeps the sign-in name as %v, want it hashed", name)
	}
}

// Check 2: nobody chooses the start password - a body naming one is refused by the registry before
// anything is written.
func TestNobodyChoosesTheStartPassword(t *testing.T) {
	f := newManagedFixture(t)
	in := lenaInput()
	in["password"] = "chosen by the parent"

	_, err := f.create(t, admin(), in)
	refusal := shared.AsError(err)
	if refusal == nil || len(refusal.Fields) != 1 || refusal.Fields[0].Code != "usecase.field_unknown" {
		t.Errorf("a chosen password answered %v", err)
	}
	if len(f.accounts.inserted) != 0 {
		t.Error("an account was written")
	}
}

// Every refusal of the creation, each before anything is written.
func TestACreationIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(f *managedFixture, in usecase.Input, actor *appshared.ActorContext)
		want    string
	}{
		{"a name too short", func(_ *managedFixture, in usecase.Input, _ *appshared.ActorContext) {
			in["sign_in_name"] = "ab"
		}, "accounts.sign_in_name_too_short"},
		{"a name with @", func(_ *managedFixture, in usecase.Input, _ *appshared.ActorContext) {
			in["sign_in_name"] = "lena@home"
		}, "accounts.sign_in_name_malformed"},
		{"a name taken in another spelling", func(f *managedFixture, _ usecase.Input, _ *appshared.ActorContext) {
			f.accounts.byID["01936f2a-7c1e-7000-8000-0000000000c9"] = domain.Account{
				ID: "01936f2a-7c1e-7000-8000-0000000000c9", SignInName: "lena", Status: domain.AccountActive,
			}
		}, "accounts.sign_in_name_taken"},
		{"the workspace switched them off", func(f *managedFixture, _ usecase.Input, _ *appshared.ActorContext) {
			f.policy.store.row.Settings.SignIn.ManagedAccounts = boolOf(false)
		}, "accounts.managed_accounts_off"},
		{"the installation locked them off", func(f *managedFixture, _ usecase.Input, _ *appshared.ActorContext) {
			f.policy.instance.level.Policy.Patch = domain.PolicyPatch{ManagedAccounts: boolOf(false)}
			f.policy.instance.level.Policy.Locks = map[domain.PolicySwitch]bool{domain.SwitchManagedAccounts: true}
		}, "accounts.managed_accounts_off"},
		{"an automation rule runs it", func(_ *managedFixture, _ usecase.Input, actor *appshared.ActorContext) {
			actor.Kind, actor.RunsForRule = shared.ActorServiceAccount, "01936f2a-7c1e-7000-8000-0000000000e1"
		}, "accounts.managed_needs_a_person"},
		{"the role is OWNER and no step-up came", func(f *managedFixture, in usecase.Input, _ *appshared.ActorContext) {
			in["role"], in["scope_type"] = "OWNER", "TENANT"
			delete(in, "scope_id")
			f.stepUp.satisfied = false
		}, "auth.step_up_required"},
		{"the actor may not manage people", func(f *managedFixture, _ usecase.Input, _ *appshared.ActorContext) {
			f.authorizer.refuse = shared.ErrForbidden.WithDetail("access.denied")
		}, "access.denied"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newManagedFixture(t)
			in, actor := lenaInput(), admin()
			c.arrange(f, in, &actor)

			_, err := f.create(t, actor, in)
			if got := detailOf(err); got != c.want {
				t.Fatalf("answered %q (%v), want %s", got, err, c.want)
			}
			if len(f.accounts.inserted) != 0 || len(f.grants.granted) != 0 {
				t.Error("something was written")
			}
		})
	}
}

// The managed account's role is placed through the grant's own ceiling (domain-model.md §3.2): an
// administrator does not make an owner this way either, and no account is left behind.
func TestAManagedAccountIsNotPlacedAboveItsCreatorsRole(t *testing.T) {
	f := newManagedFixture(t)
	f.authorizer.refuseGrant = shared.ErrForbidden.WithDetail("access.role_above_own")
	in := lenaInput()
	in["role"], in["scope_type"] = "OWNER", "TENANT"
	delete(in, "scope_id")

	_, err := f.create(t, admin(), in)
	if got := detailOf(err); got != "access.role_above_own" {
		t.Fatalf("answered %q (%v), want access.role_above_own", got, err)
	}
	if len(f.authorizer.requests) != 2 || f.authorizer.requests[1].Grants != domain.RoleOwner {
		t.Errorf("the grant was asked as %+v, want it to name OWNER", f.authorizer.requests)
	}
	if len(f.accounts.inserted) != 0 || len(f.grants.granted) != 0 {
		t.Error("something was written")
	}
}

// The creation asks the workspace's permission first and the grant's at its scope: an administrator
// of one hub only is refused at the first.
func TestTheCreationAsksTheWorkspaceAndTheScope(t *testing.T) {
	f := newManagedFixture(t)
	if _, err := f.create(t, admin(), lenaInput()); err != nil {
		t.Fatalf("creating: %v", err)
	}
	if len(f.authorizer.requests) != 2 {
		t.Fatalf("%d permission questions, want the workspace's and the scope's", len(f.authorizer.requests))
	}
	first, second := f.authorizer.requests[0], f.authorizer.requests[1]
	if len(first.Path) != 1 || first.Path[0] != domain.TenantScope() || first.Action != ManagedAccountCreatedAction {
		t.Errorf("the first question was %+v", first)
	}
	if len(second.Path) != 2 || second.Path[1].Type != domain.ScopeHub {
		t.Errorf("the second question was %+v", second)
	}
}
