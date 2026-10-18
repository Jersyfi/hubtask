// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const providerAccountPage = "https://provider.example/account"

// toldTo tells the link to the accounts named, as AccountLinkTeller would for owners and
// administrators.
type toldTo struct {
	owners map[shared.ID]bool
	asked  int
}

func (f *toldTo) For(_ context.Context, actor appshared.ActorContext) (string, error) {
	f.asked++
	if f.owners[actor.AccountID] {
		return providerAccountPage, nil
	}
	return "", nil
}

func pausedLinkOf(err error) (string, string) {
	refused := shared.AsError(err)
	if refused == nil {
		return "", ""
	}
	return refused.DetailCode, refused.Params["account_url"]
}

func suspendedAccount(fixture *sessionFixture, status domain.TenantStatus) {
	held := fixture.accounts.byEmail["bert@example.org"]
	held.TenantStatus = status
	fixture.accounts.byEmail["bert@example.org"] = held
}

// An owner or administrator whose password is the whole proof is told the provider's account link
// with the paused refusal; a member is refused without it; a wrong password learns nothing
// (UC-INS-13 check 11, multi-tenancy.md §4.2).
func TestAPausedWorkspacesOwnerIsToldTheAccountLinkAtSignIn(t *testing.T) {
	for _, c := range []struct {
		name   string
		status domain.TenantStatus
		code   string
		owner  bool
		link   string
	}{
		{"an owner, suspended", domain.TenantSuspended, "access.tenant_suspended", true, providerAccountPage},
		{"an owner, pending deletion", domain.TenantPendingDeletion, "access.tenant_pending_deletion", true, providerAccountPage},
		{"a member, suspended", domain.TenantSuspended, "access.tenant_suspended", false, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			fixture := newSessionFixture(now)
			fixture.withAccount("bert@example.org", "correct horse battery")
			suspendedAccount(fixture, c.status)
			told := &toldTo{owners: map[shared.ID]bool{}}
			if c.owner {
				told.owners[fixture.accounts.byEmail["bert@example.org"].Account.ID] = true
			}
			fixture.writer.AccountLinks = told

			_, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
				Email: "bert@example.org", Password: secret.New("correct horse battery"),
			})
			if code, link := pausedLinkOf(err); code != c.code || link != c.link {
				t.Fatalf("answered %s with %q (%v), want %s with %q", code, link, err, c.code, c.link)
			}
			if len(fixture.sessions.inserted) != 0 {
				t.Fatal("a session was opened into a paused workspace")
			}

			asked := told.asked
			_, err = SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
				Email: "bert@example.org", Password: secret.New("wrong"),
			})
			if code, link := pausedLinkOf(err); code == c.code || link != "" || told.asked != asked {
				t.Fatalf("a wrong password learned %s, %q", code, link)
			}
		})
	}
}

// An owner with a second factor is not told the link on the password alone: the paused refusal
// waits for the second step, and comes with the link once the code proves who they are. A wrong
// code is a wrong code; nothing opens.
func TestAPausedWorkspacesOwnerWithASecondFactorIsToldAfterIt(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	material := enrolled(t, fixture)
	suspendedAccount(fixture, domain.TenantSuspended)
	fixture.writer.AccountLinks = &toldTo{owners: map[shared.ID]bool{account: true}}

	result, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
	})
	if err != nil || result.Challenge == nil {
		t.Fatalf("the password alone answered %+v, %v; want the second step", result, err)
	}
	pending := fixture.writer.Pending.(*pendingStore)
	for token, row := range pending.rows {
		row.TenantStatus = domain.TenantSuspended
		pending.rows[token] = row
	}

	later := now.Add(domain.TotpStepSeconds * time.Second)
	completer := fixture.writer
	completer.Clock = clock.Fixed(later)
	_, _, err = CompleteSignIn{Writer: completer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: result.Challenge.Token, Code: "000000",
	})
	if code, link := pausedLinkOf(err); code == "access.tenant_suspended" || link != "" {
		t.Fatalf("a wrong code answered %s with %q", code, link)
	}

	_, _, err = CompleteSignIn{Writer: completer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: result.Challenge.Token, Code: domain.TotpCode(material, domain.TotpStep(later)),
	})
	if code, link := pausedLinkOf(err); code != "access.tenant_suspended" || link != providerAccountPage {
		t.Fatalf("the completed proof answered %s with %q (%v)", code, link, err)
	}
	if len(fixture.sessions.inserted) != 0 {
		t.Fatal("a session was opened into a paused workspace")
	}
}

type heldFake struct{ held []service.Permission }

func (h heldFake) HeldOnWorkspace(context.Context, appshared.ActorContext) ([]service.Permission, error) {
	return h.held, nil
}

type linksFake struct{}

func (linksFake) AccountLink(context.Context, shared.ID) (string, error) {
	return providerAccountPage, nil
}

// Who is told: whoever holds STRUCTURE on the workspace itself - its owners and administrators - and
// nobody else, signed out least of all.
func TestTheAccountLinkIsToldToOwnersAndAdministratorsOnly(t *testing.T) {
	signedIn := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: account}
	for _, c := range []struct {
		name  string
		held  []service.Permission
		actor appshared.ActorContext
		want  string
	}{
		{"an owner or administrator", []service.Permission{service.PermissionRead, service.PermissionStructure}, signedIn, providerAccountPage},
		{"a member", []service.Permission{service.PermissionRead, service.PermissionWriteItems, service.PermissionAutomation}, signedIn, ""},
		{"an auditor", []service.Permission{service.PermissionReadConfiguration, service.PermissionAuditRead}, signedIn, ""},
		{"somebody whose roles sit on a hub", []service.Permission{}, signedIn, ""},
		{"nobody signed in", []service.Permission{service.PermissionStructure}, appshared.Anonymous("en", "UTC"), ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := AccountLinkTeller{Held: heldFake{held: c.held}, Links: linksFake{}}.For(t.Context(), c.actor)
			if err != nil || got != c.want {
				t.Fatalf("For = %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

type brokenRule struct{}

func (brokenRule) JudgeSignIn(context.Context, shared.ID, domain.Account, secret.Secret) (SignInVerdict, error) {
	return SignInVerdict{}, shared.ErrUnavailable
}

// Whether the password is the whole proof could not be read: the paused sentence, without the link,
// and never a step that might lead on to a session.
func TestAPausedSignInWhoseRuleCannotBeReadIsRefusedPlainly(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	suspendedAccount(fixture, domain.TenantSuspended)
	fixture.writer.Rule = brokenRule{}
	fixture.writer.AccountLinks = &toldTo{owners: map[shared.ID]bool{account: true}}

	result, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
	})
	if code, link := pausedLinkOf(err); code != "access.tenant_suspended" || link != "" || result.Challenge != nil {
		t.Fatalf("answered %s with %q and %+v", code, link, result.Challenge)
	}
}

// An administrator the workspace would route into enrolling a second factor gets no enrolment in a
// paused workspace - it would end in a session - but the paused refusal; their password is the
// whole proof they have, so it carries the link.
func TestAPausedWorkspaceRoutesNobodyIntoEnrolment(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	fixture.writer.Policy = policyFake{required: true}
	fixture.writer.Memberships = membershipsFake{roles: []domain.Role{domain.RoleAdmin}}
	suspendedAccount(fixture, domain.TenantSuspended)
	fixture.writer.AccountLinks = &toldTo{owners: map[shared.ID]bool{account: true}}

	result, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
	})
	if code, link := pausedLinkOf(err); code != "access.tenant_suspended" || link != providerAccountPage || result.Challenge != nil {
		t.Fatalf("answered %s with %q and %+v", code, link, result.Challenge)
	}
}

// The passkey proves the person on its own: the paused refusal carries the link for an owner or
// administrator.
func TestAPausedWorkspacesOwnerIsToldTheAccountLinkAtThePasskeyDoor(t *testing.T) {
	f := newPasskeySignInFixture(t)
	held := f.store.holders[string(adaCredential)]
	held.TenantStatus = domain.TenantSuspended
	f.store.holders[string(adaCredential)] = held
	f.writer.Session.AccountLinks = &toldTo{owners: map[shared.ID]bool{held.Account.ID: true}}
	f.register(t)

	_, err := f.signInAsAda(t)
	if code, link := pausedLinkOf(err); code != "access.tenant_suspended" || link != providerAccountPage {
		t.Fatalf("answered %s with %q (%v)", code, link, err)
	}
}

// The account link is no key of a workspace's own sign-in policy: a workspace that sends one has it
// ignored, as any key the write does not know, and nothing of it reaches what the workspace stores.
func TestAWorkspaceCannotSetTheAccountLink(t *testing.T) {
	change, err := policyPatchFrom(map[string]any{
		string(domain.LinkAccount): "https://workspace.example/account",
		string(domain.LinkImprint): "https://workspace.example/imprint",
	})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if _, held := change.Legal[domain.LinkAccount]; held {
		t.Fatalf("the workspace's change carries the account link: %v", change.Legal)
	}
	if change.Legal[domain.LinkImprint] != "https://workspace.example/imprint" {
		t.Errorf("the workspace's own link was lost: %v", change.Legal)
	}
}
