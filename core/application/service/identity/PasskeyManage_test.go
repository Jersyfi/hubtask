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
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-ID-22 checks 2 and 4: a person renames one of their passkeys with the session alone, and only
// their own, through the registry as a client reaches it.

func (f *passkeyFixture) rename(
	t *testing.T, actor appshared.ActorContext, id shared.ID, name string,
) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), RenameOwnPasskeyName, actor, usecase.Input{
		"passkey_id": id.String(), "name": name,
	})
}

func TestAPersonRenamesTheirPasskeyAndOnlyTheName(t *testing.T) {
	f := newPasskeyFixture(t)
	f.holding(t, 2)
	before := f.store.held[0]

	out, err := f.rename(t, signedInActor(), before.ID, "  Laptop at work ")
	if err != nil {
		t.Fatal(err)
	}
	passkey, _ := out["passkey"].(usecase.Output)
	if passkey.String("name") != "Laptop at work" || passkey.String("id") != before.ID.String() {
		t.Errorf("answered %v", passkey)
	}
	after := f.store.held[0]
	if after.Name != "Laptop at work" || string(after.CredentialID) != string(before.CredentialID) ||
		!after.CreatedAt.Equal(before.CreatedAt) || f.store.held[1].Name != "Key" {
		t.Errorf("stored %+v beside %+v", after, f.store.held[1])
	}
	// Audited without either name (rule 10), and no fresh proof was asked.
	entries := f.session.audit.entries
	if len(entries) != 1 || entries[0].Action != PasskeyRenamedAction {
		t.Fatalf("audited %+v", entries)
	}
	if text := fmt.Sprintf("%+v", entries[0]); strings.Contains(text, "Laptop") || strings.Contains(text, "Key") {
		t.Errorf("the entry carries a name: %s", text)
	}
	if len(f.stepUp.presented) != 0 {
		t.Errorf("renaming asked for proofs: %v", f.stepUp.presented)
	}
}

func TestRenamingIsRefusedForWhatIsNotTheCallersOrNotAName(t *testing.T) {
	f := newPasskeyFixture(t)
	f.holding(t, 1)
	held := f.store.held[0].ID
	f.store.held = append(f.store.held, f.store.held[0])
	f.store.held[1].ID, f.store.held[1].AccountID = "01936f2a-7c1e-7000-8000-0000000000f1", "someone-else"

	_, err := f.rename(t, signedInActor(), "01936f2a-7c1e-7000-8000-0000000000f1", "Mine now")
	refusedWith(t, err, "auth.passkey_not_found")
	_, err = f.rename(t, signedInActor(), "01936f2a-7c1e-7000-8000-0000000000f9", "Gone")
	refusedWith(t, err, "auth.passkey_not_found")
	// A blank name is a missing one, refused by the registry before the use case sees it.
	_, err = f.rename(t, signedInActor(), held, "   ")
	refusedWith(t, err, "usecase.input_invalid")
	for _, name := range []string{strings.Repeat("ä", 65), "key\u202eevil"} {
		_, err = f.rename(t, signedInActor(), held, name)
		refusedWith(t, err, "auth.passkey_name_invalid")
	}

	token := signedInActor()
	token.Scopes = []string{accountsRead}
	_, err = f.rename(t, token, held, "Read only")
	refusedWith(t, err, "access.insufficient_scope")

	for _, kind := range []appshared.ActorKind{appshared.ActorServiceAccount, appshared.ActorAutomation, appshared.ActorAIAgent} {
		actor := signedInActor()
		actor.Kind = kind
		_, err = f.rename(t, actor, held, "Machine")
		refusedWith(t, err, codePersonOnly)
	}
	if f.store.held[0].Name != "Key" || f.store.held[1].Name != "Key" {
		t.Errorf("a refused rename changed %+v", f.store.held)
	}
	if len(f.session.audit.entries) != 0 {
		t.Errorf("a refusal was audited as a rename: %+v", f.session.audit.entries)
	}
}

// ruleFake is the workspace's rule as the factor reads it: who a second factor is demanded of.
type ruleFake struct{ requirement domain.MfaRequirement }

func (r ruleFake) JudgeSignIn(context.Context, shared.ID, domain.Account, secret.Secret) (SignInVerdict, error) {
	return SignInVerdict{FactorRequired: r.requirement}, nil
}

// under puts Bert under the rule given with the roles given, holding keys passkeys and, where app,
// the authenticator app; the codes stay while any factor does.
func (f *passkeyFixture) under(
	t *testing.T, requirement domain.MfaRequirement, app bool, keys int, roles ...domain.Role,
) *recoveryStore {
	t.Helper()
	f.writer.Session.Rule = ruleFake{requirement: requirement}
	f.writer.Session.Memberships = membershipsFake{roles: roles}
	enrollments, _ := f.writer.Session.Enrollments.(*enrollmentsStore)
	if app {
		enrollments.rows[account] = &repository.MfaEnrollment{AccountID: account, ConfirmedAt: now.Add(-time.Hour)}
	}
	f.holding(t, keys)
	for i := range f.store.held {
		f.store.held[i].RPID = "acme.hubtask.example"
	}
	recovery, _ := f.writer.Session.Recovery.(*recoveryStore)
	_ = recovery.Replace(t.Context(), account, nil, []string{"AAAA-BBBB-CCCC-DDDD"}, now)
	recovery.backed = func() bool {
		_, enrolled := enrollments.rows[account]
		return enrolled || len(f.store.held) > 0
	}
	f.register(t)
	return recovery
}

func (f *passkeyFixture) remove(t *testing.T, actor appshared.ActorContext, id shared.ID, proof string) error {
	t.Helper()
	_, err := f.registry.Invoke(t.Context(), RemoveOwnPasskeyName, actor, usecase.Input{
		"passkey_id": id.String(), "step_up_token": proof,
	})
	return err
}

// UC-ID-22 check 5: the last key is not removed where the rule demands a factor of the person, under
// each value of mfa_required_for, and the refusal comes before the proof is spent.
func TestTheLastKeyIsRefusedWhereTheRuleDemandsAFactor(t *testing.T) {
	for _, c := range []struct {
		requirement domain.MfaRequirement
		role        domain.Role
		refused     bool
	}{
		{domain.MfaForNobody, domain.RoleMember, false},
		{domain.MfaForNobody, domain.RoleOwner, false},
		{domain.MfaForAdmins, domain.RoleMember, false},
		{domain.MfaForAdmins, domain.RoleAdmin, true},
		{domain.MfaForAdmins, domain.RoleOwner, true},
		{domain.MfaForEveryone, domain.RoleMember, true},
	} {
		t.Run(string(c.requirement)+"/"+string(c.role), func(t *testing.T) {
			f := newPasskeyFixture(t)
			recovery := f.under(t, c.requirement, false, 1, c.role)
			err := f.remove(t, signedInActor(), f.store.held[0].ID, "hbt_sup_proof")
			if c.refused {
				refusedWith(t, err, "auth.mfa_required_by_tenant")
				if len(f.stepUp.presented) != 0 || len(f.store.held) != 1 {
					t.Errorf("the refusal spent %v, left %d", f.stepUp.presented, len(f.store.held))
				}
				return
			}
			if err != nil {
				t.Fatalf("removing where nothing demands a factor: %v", err)
			}
			if len(f.store.held) != 0 {
				t.Error("the passkey is still held")
			}
			if left, _ := recovery.Remaining(t.Context(), account); left != 0 {
				t.Errorf("%d codes outlived the last factor", left)
			}
		})
	}
}

// Where something else answers the rule - the app, another key - the key goes, and the codes stay; a
// key of another address is no factor and always goes.
func TestAKeyGoesWhereAnotherFactorRemains(t *testing.T) {
	for name, c := range map[string]struct {
		app   bool
		keys  int
		stale bool
	}{
		"beside the app":           {true, 1, false},
		"beside another key":       {false, 2, false},
		"made for another address": {false, 1, true},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPasskeyFixture(t)
			recovery := f.under(t, domain.MfaForEveryone, c.app, c.keys, domain.RoleMember)
			if c.stale {
				f.store.held[0].RPID = "old.acme.example"
			}
			target := f.store.held[0].ID
			if err := f.remove(t, signedInActor(), target, "hbt_sup_proof"); err != nil {
				t.Fatalf("removing: %v", err)
			}
			for _, held := range f.store.held {
				if held.ID == target {
					t.Fatal("the passkey is still held")
				}
			}
			left, _ := recovery.Remaining(t.Context(), account)
			if want := map[bool]int{true: 0, false: 1}[c.stale]; left != want {
				t.Errorf("%d codes left, want %d", left, want)
			}
			if f.store.locked[len(f.store.locked)-1] != account {
				t.Error("the write took no lock on the account")
			}
		})
	}
}

func TestRemovingAsksAFreshProofOfTheSessionAndOnlyForTheCallersOwn(t *testing.T) {
	f := newPasskeyFixture(t)
	f.under(t, domain.MfaForNobody, false, 1, domain.RoleMember)
	held := f.store.held[0].ID

	// Not the caller's: refused before the proof.
	refusedWith(t, f.remove(t, signedInActor(), "01936f2a-7c1e-7000-8000-0000000000f9", "hbt_sup_proof"),
		"auth.passkey_not_found")
	if len(f.stepUp.presented) != 0 {
		t.Errorf("an unknown passkey spent the proof: %v", f.stepUp.presented)
	}
	// No proof: the methods are asked for.
	refusedWith(t, f.remove(t, signedInActor(), held, ""), "auth.step_up_required")
	// A token has no session to prove afresh.
	token := signedInActor()
	token.TokenID = "01936f2a-7c1e-7000-8000-0000000000e9"
	refusedWith(t, f.remove(t, token, held, "hbt_sup_proof"), "auth.step_up_session_required")
	// Machines hold no passkeys.
	machine := signedInActor()
	machine.Kind = appshared.ActorServiceAccount
	refusedWith(t, f.remove(t, machine, held, "hbt_sup_proof"), codePersonOnly)
	if len(f.store.held) != 1 || len(f.session.audit.entries) != 0 {
		t.Fatalf("a refusal removed or audited: %+v %+v", f.store.held, f.session.audit.entries)
	}

	// With the proof it goes, audited without its name (rule 10).
	if err := f.remove(t, signedInActor(), held, "hbt_sup_proof"); err != nil {
		t.Fatal(err)
	}
	entries := f.session.audit.entries
	if len(entries) != 1 || entries[0].Action != PasskeyRemovedAction {
		t.Fatalf("audited %+v", entries)
	}
	if strings.Contains(fmt.Sprintf("%+v", entries[0]), "Key") {
		t.Errorf("the entry carries the name: %+v", entries[0])
	}
	// Gone at once: the list no longer has it, and removing it again finds nothing.
	refusedWith(t, f.remove(t, signedInActor(), held, "hbt_sup_proof"), "auth.passkey_not_found")
}

// P-05: the list says a passkey can be removed exactly where removing it would pass before the
// proof, and why not where it would not.
func TestTheListSaysWhetherAPasskeyCanBeRemovedAndWhyNot(t *testing.T) {
	for name, c := range map[string]struct {
		requirement domain.MfaRequirement
		app         bool
		keys        int
		noProof     bool
		can         bool
		reason      string
	}{
		"the last key under a rule":   {domain.MfaForEveryone, false, 1, false, false, codeFactorRequired},
		"beside the app under a rule": {domain.MfaForEveryone, true, 1, false, true, ""},
		"two keys under a rule":       {domain.MfaForEveryone, false, 2, false, true, ""},
		"the last key, no rule":       {domain.MfaForNobody, false, 1, false, true, ""},
		"no proof to give":            {domain.MfaForNobody, false, 1, true, false, codeStepUpNoMethod},
		"none held":                   {domain.MfaForEveryone, false, 0, false, true, ""},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPasskeyFixture(t)
			if c.noProof {
				f.writer.StepUp = &noMethodStepUp{}
			}
			f.under(t, c.requirement, c.app, c.keys, domain.RoleMember)
			out, err := f.list(t, signedInActor())
			if err != nil {
				t.Fatal(err)
			}
			if out["can_remove"] != c.can || out.String("cannot_remove_reason") != c.reason {
				t.Errorf("can_remove %v, reason %q", out["can_remove"], out.String("cannot_remove_reason"))
			}
			if c.can || len(f.store.held) == 0 {
				return
			}
			// And removing is refused with the reason the list gave.
			refusedWith(t, f.remove(t, signedInActor(), f.store.held[0].ID, "hbt_sup_proof"), "forbidden")
		})
	}
}
