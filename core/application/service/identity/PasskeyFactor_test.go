// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-03 check 5, UC-ID-22 check 5 and identity.md §9: a passkey is a second factor, so the app
// may go while one remains, the codes stay with it, and new codes can be had with a key alone.

// keysFake is an account's passkeys of the workspace's address, as the rules count them.
type keysFake struct {
	count  int
	locked int
}

func (k *keysFake) CurrentKeys(context.Context, shared.ID) (int, error) { return k.count, nil }

func (k *keysFake) LockAccount(context.Context, shared.ID) error {
	k.locked++
	return nil
}

// holdingKeys gives the fixture's writer that many passkeys, and the codes' store the statement's
// condition: they stay while a factor does.
func holdingKeys(session *sessionFixture, count int) *keysFake {
	keys := &keysFake{count: count}
	session.writer.Keys = keys
	enrollments, _ := session.writer.Enrollments.(*enrollmentsStore)
	if store, ok := session.writer.Recovery.(*recoveryStore); ok {
		store.backed = func() bool {
			_, enrolled := enrollments.rows[account]
			return keys.count > 0 || enrolled
		}
	}
	return keys
}

func liveCodes(t *testing.T, session *sessionFixture) int {
	t.Helper()
	left, err := session.writer.Recovery.Remaining(t.Context(), account)
	if err != nil {
		t.Fatal(err)
	}
	return left
}

// The app goes while a key remains, under every value of the rule, and the codes stay with the key.
func TestTheAppIsRemovableWhileAKeyRemainsUnderEveryRule(t *testing.T) {
	cases := []struct {
		requirement domain.MfaRequirement
		role        domain.Role
	}{
		{domain.MfaForNobody, domain.RoleMember},
		{domain.MfaForAdmins, domain.RoleMember},
		{domain.MfaForAdmins, domain.RoleAdmin},
		{domain.MfaForAdmins, domain.RoleOwner},
		{domain.MfaForEveryone, domain.RoleMember},
	}
	for _, c := range cases {
		t.Run(string(c.requirement)+"/"+string(c.role), func(t *testing.T) {
			fixture := armedUnder(t, c.requirement, c.role)
			keys := holdingKeys(fixture.session, 1)
			if err := turnOff(t, fixture); err != nil {
				t.Fatalf("turning the app off beside a key answered %v", err)
			}
			if _, err := fixture.enroll.Find(t.Context(), account); !errors.Is(err, shared.ErrNotFound) {
				t.Errorf("the app is still armed: %v", err)
			}
			if left := liveCodes(t, fixture.session); left != domain.RecoveryCodeCount {
				t.Errorf("%d codes left beside the key, want all ten", left)
			}
			if keys.locked == 0 {
				t.Error("the write took no lock on the account")
			}
		})
	}
}

// Without a key, the app is the last factor: refused where the rule demands one, and the codes go
// with it where nothing does (UC-ID-03 checks 5 and 6).
func TestTheLastAppIsRefusedWhereDemandedAndTakesTheCodesWhereNot(t *testing.T) {
	demanded := armedUnder(t, domain.MfaForEveryone, domain.RoleMember)
	holdingKeys(demanded.session, 0)
	err := turnOff(t, demanded)
	if !errors.Is(err, shared.ErrForbidden) || !strings.Contains(err.Error(), "auth.mfa_required_by_tenant") {
		t.Fatalf("the last app under Everyone answered %v", err)
	}

	free := armedUnder(t, domain.MfaForNobody, domain.RoleMember)
	holdingKeys(free.session, 0)
	if err := turnOff(t, free); err != nil {
		t.Fatal(err)
	}
	if left := liveCodes(t, free.session); left != 0 {
		t.Errorf("%d codes outlived the last factor", left)
	}
}

// The profile reads a key as a factor: the codes are counted beside it, and the app's own answer is
// the app alone.
func TestTheProfileCountsAKeyAsASecondFactor(t *testing.T) {
	for _, c := range []struct {
		name             string
		app              bool
		keys             int
		requirement      domain.MfaRequirement
		held, armed, off bool
	}{
		{"key alone", false, 1, domain.MfaForEveryone, true, false, false},
		{"app and key under a rule", true, 1, domain.MfaForEveryone, true, true, true},
		{"app alone under a rule", true, 0, domain.MfaForEveryone, true, true, false},
		{"neither", false, 0, domain.MfaForNobody, false, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var fixture *stepFixture
			if c.app {
				fixture = armedUnder(t, c.requirement, domain.RoleMember)
			} else {
				fixture = keyOnlyUnder(t, c.requirement)
			}
			holdingKeys(fixture.session, c.keys)
			own, err := ownAccountOf(fixture).ExecuteWithRecovery(t.Context(), signedInActor())
			if err != nil {
				t.Fatal(err)
			}
			if own.HasSecondFactor != c.held || own.HasAuthenticatorApp != c.armed ||
				own.AuthenticatorAppRemovable != c.off {
				t.Errorf("held %v, app %v, removable %v", own.HasSecondFactor, own.HasAuthenticatorApp,
					own.AuthenticatorAppRemovable)
			}
			if c.held != (own.RecoveryCodesRemaining >= 0) {
				t.Errorf("codes answered as %d", own.RecoveryCodesRemaining)
			}
		})
	}
}

// keyOnlyUnder is a person without the app in a workspace whose rule is the one given, holding ten
// codes as a first key leaves them.
func keyOnlyUnder(t *testing.T, requirement domain.MfaRequirement) *stepFixture {
	t.Helper()
	fixture := armedUnder(t, requirement, domain.RoleMember)
	delete(fixture.enroll.rows, account)
	return fixture
}

func ownAccountOf(fixture *stepFixture) GetOwnAccount {
	return GetOwnAccount{
		Accounts: fixture.session.writer.People, UnitOfWork: &unitOfWork{},
		Recovery: fixture.session.writer.Recovery, Enrollments: fixture.enroll,
		Keys: fixture.session.writer.Keys, Factor: fixture.session.writer,
	}
}

// New codes with a key alone; refused before the proof, which stays unspent, where nothing is held.
func TestNewCodesNeedAFactorOfEitherKindAndAskItBeforeTheProof(t *testing.T) {
	keyOnly := newRecoveryFixture(t)
	delete(keyOnly.writer.Enrollments.(*enrollmentsStore).rows, account)
	holdingKeys(keyOnly, 1)
	if _, err := (RegenerateRecoveryCodes{Writer: keyOnly.writer}).Execute(
		t.Context(), recoveryActor(), keyOnly.proof); err != nil {
		t.Fatalf("new codes beside a key answered %v", err)
	}

	for name, arrange := range map[string]func(*sessionFixture){
		"nothing held": func(f *sessionFixture) { delete(f.writer.Enrollments.(*enrollmentsStore).rows, account) },
		"an enrolment begun": func(f *sessionFixture) {
			f.writer.Enrollments.(*enrollmentsStore).rows[account] = &repository.MfaEnrollment{AccountID: account}
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRecoveryFixture(t)
			arrange(fixture)
			holdingKeys(fixture, 0)
			_, err := RegenerateRecoveryCodes{Writer: fixture.writer}.Execute(t.Context(), recoveryActor(), fixture.proof)
			if !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("answered %v, want a conflict", err)
			}
			// The proof is still there to spend: the refusal came first.
			holdingKeys(fixture, 1)
			if _, err := (RegenerateRecoveryCodes{Writer: fixture.writer}).Execute(
				t.Context(), recoveryActor(), fixture.proof); err != nil {
				t.Errorf("the proof was spent by the refusal: %v", err)
			}
		})
	}
}

// P-02: until the sign-in's step can ask for a key, a key does not stand in for the app there - a
// person the rule covers who holds only a key is led into the app's setup, never into a session.
func TestASignInDoesNotTakeAKeyItCannotAskFor(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MfaRequiredFor: requirementOf(domain.MfaForEveryone)}
	holdingKeys(fixture.session, 2)

	result := fixture.signsIn(t, "correct horse battery")

	if result.Pair != nil {
		t.Fatal("a person the rule covers signed in with the password alone beside a key")
	}
	if result.Challenge == nil || result.Challenge.Methods[0] != methodEnroll {
		t.Fatalf("the challenge owes %v, want the app's setup", result.Challenge)
	}
}
