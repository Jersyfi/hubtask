// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const rehashPassword = "correct horse battery"

// oldCostFixture holds an account whose stored hash the hasher reads as written under a lower
// cost: "old:" marks it, and the hasher's Hash writes "hash:" - the current cost.
func oldCostFixture() (*sessionFixture, *passwordsFake) {
	fixture := newSessionFixture(now)
	fixture.withAccount("bert@example.org", rehashPassword)
	found := fixture.accounts.byEmail["bert@example.org"]
	found.PasswordHash = secret.New("old:" + rehashPassword)
	fixture.accounts.byEmail["bert@example.org"] = found
	passwords := &passwordsFake{valid: rehashPassword, staleBelow: "old:"}
	fixture.writer.Passwords = passwords
	return fixture, passwords
}

func (f *sessionFixture) signInWith(ctx context.Context, password string) (SignInResult, error) {
	return SignIn{Writer: f.writer}.Execute(ctx, SignInCommand{
		Email: "bert@example.org", Password: secret.New(password), RemoteAddr: "203.0.113.7:1",
	})
}

// T-02: a hash written under a lower cost is brought up to the current one by the next sign-in
// that proves the password - a compare-and-set on the hash it verified - and counted once.
func TestASignInAgainstAnOldCostHashStoresANewHashAndCountsOnce(t *testing.T) {
	fixture, _ := oldCostFixture()

	result, err := fixture.signInWith(t.Context(), rehashPassword)
	if err != nil || result.Pair == nil {
		t.Fatalf("signing in: %v (pair %v)", err, result.Pair)
	}
	if len(fixture.accounts.rehashed) != 1 {
		t.Fatalf("%d rehashes written, want one", len(fixture.accounts.rehashed))
	}
	written := fixture.accounts.rehashed[0]
	if written.account != account || written.current != "old:"+rehashPassword ||
		written.replacement != "hash:"+rehashPassword || !written.at.Equal(now) {
		t.Errorf("rehash %+v, want the old hash replaced by the current one at the clock's now", written)
	}
	if fixture.signals.rehashed != 1 {
		t.Errorf("counted %d rehashes, want one", fixture.signals.rehashed)
	}

	// The next sign-in finds a current hash and leaves it alone.
	if _, err := fixture.signInWith(t.Context(), rehashPassword); err != nil {
		t.Fatalf("signing in again: %v", err)
	}
	if len(fixture.accounts.rehashed) != 1 || fixture.signals.rehashed != 1 {
		t.Errorf("a current hash was rehashed again (%d writes, %d counted)",
			len(fixture.accounts.rehashed), fixture.signals.rehashed)
	}
}

// A hash at the current cost costs a sign-in nothing extra: nothing written, nothing counted.
func TestASignInAgainstACurrentHashWritesNothing(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.withAccount("bert@example.org", rehashPassword)
	fixture.writer.Passwords = &passwordsFake{staleBelow: "old:"}

	if _, err := fixture.signInWith(t.Context(), rehashPassword); err != nil {
		t.Fatalf("signing in: %v", err)
	}
	if len(fixture.accounts.rehashed) != 0 || fixture.signals.rehashed != 0 {
		t.Errorf("a current hash was rehashed (%d writes, %d counted)",
			len(fixture.accounts.rehashed), fixture.signals.rehashed)
	}
}

// Nothing proved, nothing rehashed: a wrong password and a refused account leave the hash alone.
func TestARefusedSignInRehashesNothing(t *testing.T) {
	for name, arrange := range map[string]func(*sessionFixture) (string, error){
		"a wrong password": func(*sessionFixture) (string, error) { return "a wrong password!", domain.ErrSignInFailed() },
		"a disabled account": func(f *sessionFixture) (string, error) {
			found := f.accounts.byEmail["bert@example.org"]
			found.Account.Status = domain.AccountDisabled
			f.accounts.byEmail["bert@example.org"] = found
			return rehashPassword, found.Account.Verify()
		},
	} {
		fixture, _ := oldCostFixture()
		password, refusal := arrange(fixture)

		if _, err := fixture.signInWith(t.Context(), password); err == nil || err.Error() != refusal.Error() {
			t.Fatalf("%s: answered %v, want %v", name, err, refusal)
		}
		if len(fixture.accounts.rehashed) != 0 || fixture.signals.rehashed != 0 {
			t.Errorf("%s: rehashed (%d writes, %d counted)", name,
				len(fixture.accounts.rehashed), fixture.signals.rehashed)
		}
	}
}

// A sign-in that owes a further step has still proved the password: the hash is upgraded too.
func TestAChallengedSignInStillRehashes(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(30)}
	fixture.session.writer.Passwords = &passwordsFake{staleBelow: "hash:"}

	if result := fixture.signsIn(t, rehashPassword); result.Challenge == nil {
		t.Fatal("a password below the tightened minimum signed straight in")
	}
	if len(fixture.session.accounts.rehashed) != 1 || fixture.session.signals.rehashed != 1 {
		t.Errorf("a challenged sign-in rehashed %d times and counted %d, want once each",
			len(fixture.session.accounts.rehashed), fixture.session.signals.rehashed)
	}
}

// Upkeep never refuses a proven password: a store that fails leaves the old hash, which still
// verifies, and the sign-in goes on uncounted.
func TestAFailedRehashDoesNotFailTheSignIn(t *testing.T) {
	fixture, _ := oldCostFixture()
	fixture.accounts.rehashFail = true

	result, err := fixture.signInWith(t.Context(), rehashPassword)
	if err != nil || result.Pair == nil {
		t.Fatalf("a failed rehash failed the sign-in: %v", err)
	}
	if fixture.signals.rehashed != 0 {
		t.Errorf("a failed rehash was counted %d times", fixture.signals.rehashed)
	}
}

// The write does not follow the request's cancellation: a client that leaves once the password
// has proved right does not take the upgrade with it.
func TestARehashOutlivesTheRequestsCancellation(t *testing.T) {
	fixture, _ := oldCostFixture()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _ = fixture.signInWith(ctx, rehashPassword)
	if len(fixture.accounts.rehashCtxErr) != 1 || fixture.accounts.rehashCtxErr[0] != nil {
		t.Errorf("the rehash saw %v, want one write under a live context", fixture.accounts.rehashCtxErr)
	}
}
