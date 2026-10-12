// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"fmt"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/text"
)

const setupCode = "7K3M-Q9TX-2WHR"

// setupCodes is the code table, in memory: one code, a state, and the claims it saw.
type setupCodes struct {
	code     string
	expires  time.Time
	done     bool
	claims   int
	issued   []domain.SetupCode
	unsetErr error
}

func (s *setupCodes) Unset(context.Context) (bool, error) { return !s.done, s.unsetErr }

func (s *setupCodes) Issue(_ context.Context, _ shared.ID, code domain.SetupCode, _, _ time.Time) (bool, error) {
	if s.done {
		return false, nil
	}
	s.issued = append(s.issued, code)
	return true, nil
}

func (s *setupCodes) Claim(_ context.Context, code domain.SetupCode, now time.Time) (repository.SetupClaim, error) {
	s.claims++
	known, _ := domain.ParseSetupCode(s.code)
	switch {
	case s.done:
		return repository.SetupAlreadyDone, nil
	case code.Canonical() == "" || code.Canonical() != known.Canonical():
		return repository.SetupWrong, nil
	case !now.Before(s.expires):
		return repository.SetupExpired, nil
	}
	s.done = true
	return repository.SetupClaimed, nil
}

// seeder records the workspace it was asked to write.
type seeder struct {
	tenants []domain.Tenant
	owners  []domain.Account
	actor   appshared.ActorContext
}

func (s *seeder) SeedWorkspace(
	_ context.Context, actor appshared.ActorContext, tenant domain.Tenant, owner domain.Account, _ time.Time,
) (shared.ID, shared.ID, error) {
	s.tenants = append(s.tenants, tenant)
	s.owners = append(s.owners, owner)
	s.actor = actor
	return shared.ID("018f2a1b-0000-7000-8000-00000000a001"), shared.ID("018f2a1b-0000-7000-8000-00000000a002"), nil
}

// firstOwners is the activation, recording the hash it stored.
type firstOwners struct{ hash string }

func (o *firstOwners) ActivateFirstOwner(_ context.Context, _ shared.ID, passwordHash string, _ time.Time) (bool, error) {
	o.hash = passwordHash
	return true, nil
}

type journalFake struct{ entries []adminrepo.InstanceEvent }

func (j *journalFake) Record(_ context.Context, event adminrepo.InstanceEvent) error {
	j.entries = append(j.entries, event)
	return nil
}

func (j *journalFake) Page(context.Context, string, int) ([]adminrepo.InstanceEvent, adminrepo.PageInfo, error) {
	return j.entries, adminrepo.PageInfo{}, nil
}

// countingIDs draws a fresh identifier each time, so the workspace and its owner are told apart.
type countingIDs struct{ n int }

func (c *countingIDs) NewID() shared.ID {
	c.n++
	return shared.ID(fmt.Sprintf("018f2a1b-0000-7000-8000-%012x", 0x5e7000+c.n))
}

type setupFixture struct {
	session   *sessionFixture
	codes     *setupCodes
	seeder    *seeder
	journal   *journalFake
	operators *operatorRegister
	owners    *firstOwners
	handler   SetUpInstallation
}

func newSetupFixture(tenancy env.TenancyMode) *setupFixture {
	f := &setupFixture{
		session:   newSessionFixture(now),
		codes:     &setupCodes{code: setupCode, expires: now.Add(domain.SetupCodeLifetime)},
		seeder:    &seeder{},
		journal:   &journalFake{},
		operators: &operatorRegister{accounts: map[shared.ID]bool{}},
		owners:    &firstOwners{},
	}
	f.session.writer.IDs = &countingIDs{}
	f.handler = SetUpInstallation{
		Writer: f.session.writer, Codes: f.codes, Seeder: f.seeder, Owners: f.owners,
		Operators: f.operators, Journal: f.journal, Text: text.Composing{}, Tenancy: tenancy,
	}
	return f
}

func (f *setupFixture) invoke(t *testing.T, input usecase.Input) (usecase.Output, error) {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, f.handler.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return registry.Invoke(t.Context(), SetUpInstallationName, appshared.Anonymous("en", "UTC"), input)
}

func setupInput(extra map[string]any) usecase.Input {
	in := usecase.Input{
		"code": setupCode, "display_name": "Family Winkel", "owner_email": "jerome@example.eu",
		"owner_display_name": "Jérôme", "password": "seven blue lanterns above the harbour",
	}
	for key, value := range extra {
		in[key] = value
	}
	return in
}

// UC-INS-01 checks 4 and 6: one step creates the workspace, its owner with the password and the
// owner's register entry, records it in the trail and the journal, and signs the owner in.
func TestTheSetupCreatesTheWorkspaceItsOwnerAndTheOperatorAndSignsIn(t *testing.T) {
	f := newSetupFixture(env.TenancySingle)

	out, err := f.invoke(t, setupInput(nil))
	if err != nil {
		t.Fatalf("the setup was refused: %v", err)
	}
	if out.String("access_token") == "" || out.String("refresh_token") == "" {
		t.Errorf("the setup answered no session pair: %v", out)
	}

	if len(f.seeder.tenants) != 1 {
		t.Fatalf("%d workspaces were written, want one", len(f.seeder.tenants))
	}
	tenant, owner := f.seeder.tenants[0], f.seeder.owners[0]
	if tenant.Slug != "family-winkel" || tenant.DisplayName != "Family Winkel" {
		t.Errorf("workspace = %q %q", tenant.Slug, tenant.DisplayName)
	}
	if owner.Email != "jerome@example.eu" || owner.TenantID != tenant.ID {
		t.Errorf("owner = %+v", owner)
	}
	if f.seeder.actor.AccountID != owner.ID {
		t.Errorf("the workspace's trail would name %q, not its owner", f.seeder.actor.AccountID)
	}
	if f.owners.hash != "hash:seven blue lanterns above the harbour" {
		t.Errorf("the owner's password was not set: %q", f.owners.hash)
	}
	if !f.operators.accounts[owner.ID] {
		t.Error("the owner is not in the operator register")
	}
	if len(f.journal.entries) != 1 || f.journal.entries[0].Action != "instance.set_up" ||
		f.journal.entries[0].TenantID != tenant.ID {
		t.Errorf("journal = %+v", f.journal.entries)
	}
	recorded := false
	for _, entry := range f.session.audit.entries {
		recorded = recorded || (entry.Action == InstallationSetUpAction && entry.TenantID == tenant.ID)
	}
	if !recorded {
		t.Error("the setup is not in the new workspace's trail")
	}
	if len(f.session.sessions.inserted) != 1 || f.session.sessions.inserted[0].AccountID != owner.ID {
		t.Errorf("sessions = %+v", f.session.sessions.inserted)
	}
}

// UC-INS-01 check 3: a wrong or expired code is refused with one sentence, and the setup is refused
// after one successful use. Through the registry, so the input validation runs first.
func TestTheSetupRefusesAWrongAnExpiredAndASpentCode(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(*setupFixture)
		code    string
		want    string
	}{
		{name: "wrong", code: "AAAA-BBBB-CCCC", want: "setup.code_wrong"},
		{name: "not shaped like a code", code: "hello", want: "setup.code_wrong"},
		{name: "expired", prepare: func(f *setupFixture) { f.codes.expires = now }, want: "setup.code_expired"},
		{name: "spent", prepare: func(f *setupFixture) { f.codes.done = true }, want: "setup.already_done"},
		{name: "lower case, no dashes, is the right one", code: "7k3mq9tx2whr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSetupFixture(env.TenancySingle)
			if tc.prepare != nil {
				tc.prepare(f)
			}
			extra := map[string]any{}
			if tc.code != "" {
				extra["code"] = tc.code
			}
			_, err := f.invoke(t, setupInput(extra))
			if detailOf(err) != tc.want {
				t.Fatalf("answered %v, want %q", err, tc.want)
			}
			if tc.want != "" && (len(f.journal.entries) != 0 || len(f.operators.accounts) != 0) {
				t.Error("a refused setup wrote something")
			}
		})
	}
}

// UC-INS-01 check 5: single mode asks for no short name; multi mode requires one and uses it.
func TestTheShortNameIsAskedInMultiModeOnly(t *testing.T) {
	single := newSetupFixture(env.TenancySingle)
	if _, err := single.invoke(t, setupInput(map[string]any{"slug": "family"})); detailOf(err) != "setup.short_name_not_asked" {
		t.Errorf("a short name in single mode answered %v", err)
	}

	multi := newSetupFixture(env.TenancyMulti)
	if _, err := multi.invoke(t, setupInput(nil)); detailOf(err) != "setup.short_name_required" {
		t.Errorf("no short name in multi mode answered %v", err)
	}
	if multi.codes.claims != 0 {
		t.Error("a refused input spent a claim")
	}
	if _, err := multi.invoke(t, setupInput(map[string]any{"slug": "acme"})); err != nil {
		t.Fatalf("a multi-mode setup was refused: %v", err)
	}
	if multi.seeder.tenants[0].Slug != "acme" {
		t.Errorf("slug = %q, want the one given", multi.seeder.tenants[0].Slug)
	}
}

// A password the rule refuses spends nothing: the code is claimed only after it is judged.
func TestARefusedPasswordSpendsNoCode(t *testing.T) {
	f := newSetupFixture(env.TenancySingle)
	_, err := f.invoke(t, setupInput(map[string]any{"password": "short"}))
	if err == nil {
		t.Fatal("a short password was accepted")
	}
	if f.codes.claims != 0 || f.codes.done {
		t.Error("a refused password spent the code")
	}
}

// The manifest's answer, which the card is drawn from (P-05): offered while nothing exists, and
// not after.
func TestTheSetupIsOfferedOnlyWhileTheInstallationIsEmpty(t *testing.T) {
	f := newSetupFixture(env.TenancySingle)
	if open, _ := f.codes.Unset(t.Context()); !open {
		t.Fatal("a fresh installation is not open for setup")
	}
	if _, err := f.invoke(t, setupInput(nil)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if open, _ := f.codes.Unset(t.Context()); open {
		t.Error("the setup is still offered after it ran")
	}
}
