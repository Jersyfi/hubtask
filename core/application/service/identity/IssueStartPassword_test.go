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
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// UC-ID-20 check 5: for a managed account only, an owner or administrator issues a new start
// password after a fresh proof; it is recorded, every session of the account ends, and nothing of
// the second factor is touched.

// startPasswordStore holds what the renewal writes. `managed` is what the writing statement checks:
// an account no longer managed when the write lands changes nothing.
type startPasswordStore struct {
	accounts *accountStore
	hashes   map[shared.ID]string
	marked   map[shared.ID]time.Time
	spent    []shared.ID
	gone     bool
}

func (s *startPasswordStore) IssueStartPassword(
	_ context.Context, id shared.ID, hash string, at time.Time,
) (bool, error) {
	account, found := s.accounts.byID[id]
	if !found || s.gone || !account.OnlyThePasswordReaches() {
		return false, nil
	}
	s.hashes[id], s.marked[id] = hash, at
	return true, nil
}

func (s *startPasswordStore) SpendPending(_ context.Context, id shared.ID, _ time.Time) (int, error) {
	s.spent = append(s.spent, id)
	return 1, nil
}

var _ repository.StartPasswords = (*startPasswordStore)(nil)

type sessionEnder struct{ ended []shared.ID }

func (s *sessionEnder) RevokeAll(_ context.Context, id shared.ID, _ time.Time) (int, error) {
	s.ended = append(s.ended, id)
	return 3, nil
}

const (
	lenaID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000f1")
	bertID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000f2")
	robotID  = shared.ID("01936f2a-7c1e-7000-8000-0000000000f3")
	erasedID = shared.ID("01936f2a-7c1e-7000-8000-0000000000f4")
	carlaID  = shared.ID("01936f2a-7c1e-7000-8000-0000000000f5")
	jonasID  = shared.ID("01936f2a-7c1e-7000-8000-0000000000f6")
)

type renewalFixture struct {
	registry   *usecase.Registry
	accounts   *accountStore
	store      *startPasswordStore
	sessions   *sessionEnder
	authorizer *authorizer
	audit      *auditSink
	stepUp     *stepUpFake
}

func newRenewalFixture(t *testing.T) *renewalFixture {
	t.Helper()
	accounts := newAccounts()
	for _, account := range []domain.Account{
		{ID: lenaID, TenantID: tenant, Kind: domain.AccountUser, SignInName: "lena", DisplayName: "Lena", Status: domain.AccountActive},
		{ID: bertID, TenantID: tenant, Kind: domain.AccountUser, Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive},
		{ID: robotID, TenantID: tenant, Kind: domain.AccountServiceAccount, DisplayName: "Robot", Status: domain.AccountActive},
		{ID: erasedID, TenantID: tenant, Kind: domain.AccountUser, DisplayName: "Former user", Status: domain.AccountAnonymized},
		{ID: carlaID, TenantID: tenant, Kind: domain.AccountUser, Email: "carla@example.org", DisplayName: "Carla", Status: domain.AccountInvited},
		// Upgraded: the sign-in name kept, an address confirmed.
		{ID: jonasID, TenantID: tenant, Kind: domain.AccountUser, SignInName: "jonas", Email: "jonas@example.org", DisplayName: "Jonas", Status: domain.AccountActive},
	} {
		accounts.byID[account.ID] = account
	}
	f := &renewalFixture{
		accounts: accounts,
		store: &startPasswordStore{
			accounts: accounts, hashes: map[shared.ID]string{}, marked: map[shared.ID]time.Time{},
		},
		sessions: &sessionEnder{}, authorizer: &authorizer{}, audit: &auditSink{},
		stepUp: &stepUpFake{satisfied: true},
	}
	registry, err := usecase.NewRegistry(nil, IssueStartPassword{
		Accounts: accounts, StartPasswords: f.store, Sessions: f.sessions,
		Authorizer: f.authorizer, StepUp: f.stepUp, Passwords: &passwordsFake{},
		Entropy: &countingEntropy{}, Audit: f.audit, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.registry = registry
	return f
}

func (f *renewalFixture) renew(t *testing.T, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), IssueStartPasswordName, actor, in)
}

func renewalOf(id shared.ID) usecase.Input {
	return usecase.Input{"account_id": id.String(), "step_up_token": "hbt_sup_fresh"}
}

func TestAnAdministratorIssuesANewStartPasswordForAManagedAccount(t *testing.T) {
	f := newRenewalFixture(t)

	out, err := f.renew(t, admin(), renewalOf(lenaID))
	if err != nil {
		t.Fatalf("renewing: %v", err)
	}
	start := out.String("start_password")
	if len(start) != 19 || strings.Count(start, "-") != 3 {
		t.Errorf("the start password %q is not four groups of four", start)
	}
	account, _ := out["account"].(usecase.Output)
	if account.String("sign_in_name") != "lena" {
		t.Errorf("the answer names %v, want the sign-in name the slip prints", account)
	}
	if f.store.hashes[lenaID] != "hash:"+start || !f.store.marked[lenaID].Equal(now) {
		t.Error("what was kept is not the new start password's hash with its mark")
	}
	if len(f.sessions.ended) != 1 || f.sessions.ended[0] != lenaID {
		t.Errorf("sessions ended for %v, want Lena's", f.sessions.ended)
	}
	if len(f.store.spent) != 1 || f.store.spent[0] != lenaID {
		t.Errorf("pending steps spent for %v, want Lena's", f.store.spent)
	}
	if len(f.stepUp.presented) != 1 {
		t.Errorf("the proof was asked %d times, want once", len(f.stepUp.presented))
	}

	if len(f.audit.entries) != 1 {
		t.Fatalf("the trail holds %v", f.audit.entries)
	}
	entry := f.audit.entries[0]
	if entry.Action != StartPasswordIssuedAction || entry.Severity != audit.SeverityWarning ||
		entry.TargetID != lenaID || entry.ActorID != adminID {
		t.Errorf("the trail entry reads %+v", entry)
	}
	if written := fmt.Sprint(entry.Changes); strings.Contains(written, start) || strings.Contains(written, "hash") {
		t.Errorf("the trail carries the start password: %s", written)
	}
}

// The task's acceptance: the action does not exist for an account with an address - an ordinary
// one, an invited one, one upgraded from managed - nor for a machine or an erased person; and the
// fresh proof is not spent on a refused target.
func TestANewStartPasswordIsRefusedForEveryAccountThatIsNotManaged(t *testing.T) {
	for name, target := range map[string]shared.ID{
		"an address":            bertID,
		"an invitation":         carlaID,
		"an address and a name": jonasID,
		"a service account":     robotID,
		"an erased person":      erasedID,
		"nobody here":           "01936f2a-7c1e-7000-8000-0000000000ff",
	} {
		t.Run(name, func(t *testing.T) {
			f := newRenewalFixture(t)
			_, err := f.renew(t, admin(), renewalOf(target))
			want := "accounts.not_managed"
			if name == "nobody here" {
				want = "accounts.not_found"
			}
			if got := detailOf(err); got != want {
				t.Fatalf("answered %q (%v), want %s", got, err, want)
			}
			if len(f.stepUp.presented) != 0 {
				t.Error("the proof was spent on a refused target")
			}
			if len(f.store.hashes)+len(f.sessions.ended)+len(f.store.spent)+len(f.audit.entries) != 0 {
				t.Error("something was written")
			}
		})
	}
}

func TestANewStartPasswordIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(f *renewalFixture, in usecase.Input, actor *appshared.ActorContext)
		want    string
	}{
		{"no fresh proof", func(f *renewalFixture, in usecase.Input, _ *appshared.ActorContext) {
			delete(in, "step_up_token")
			f.stepUp.satisfied = false
		}, "auth.step_up_required"},
		{"the actor may not manage people", func(f *renewalFixture, _ usecase.Input, _ *appshared.ActorContext) {
			f.authorizer.refuse = shared.ErrForbidden.WithDetail("access.denied")
		}, "access.denied"},
		{"an automation rule runs it", func(_ *renewalFixture, _ usecase.Input, actor *appshared.ActorContext) {
			actor.Kind, actor.RunsForRule = shared.ActorServiceAccount, "01936f2a-7c1e-7000-8000-0000000000e1"
		}, "accounts.managed_needs_a_person"},
		{"an address confirmed between the read and the write", func(f *renewalFixture, _ usecase.Input, _ *appshared.ActorContext) {
			f.store.gone = true
		}, "accounts.not_managed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRenewalFixture(t)
			in, actor := renewalOf(lenaID), admin()
			c.arrange(f, in, &actor)

			_, err := f.renew(t, actor, in)
			if got := detailOf(err); got != c.want {
				t.Fatalf("answered %q (%v), want %s", got, err, c.want)
			}
			if len(f.store.hashes)+len(f.sessions.ended)+len(f.audit.entries) != 0 {
				t.Error("something was written")
			}
		})
	}
}

// Nobody chooses a start password, at a renewal either.
func TestNobodyChoosesTheNewStartPassword(t *testing.T) {
	f := newRenewalFixture(t)
	in := renewalOf(lenaID)
	in["password"] = "chosen by the parent"
	_, err := f.renew(t, admin(), in)
	refusal := shared.AsError(err)
	if refusal == nil || len(refusal.Fields) != 1 || refusal.Fields[0].Code != "usecase.field_unknown" {
		t.Errorf("a chosen password answered %v", err)
	}
}

// The workspace's question is the one asked, as for a creation: an administrator of one hub only is
// not the workspace's.
func TestTheRenewalAsksTheWorkspace(t *testing.T) {
	f := newRenewalFixture(t)
	if _, err := f.renew(t, admin(), renewalOf(lenaID)); err != nil {
		t.Fatalf("renewing: %v", err)
	}
	if len(f.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want one", len(f.authorizer.requests))
	}
	asked := f.authorizer.requests[0]
	if len(asked.Path) != 1 || asked.Path[0] != domain.TenantScope() || asked.Action != StartPasswordIssuedAction ||
		asked.TargetID != lenaID {
		t.Errorf("the question was %+v", asked)
	}
}
