// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 J\u00e9rôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// A sign-in name is folded the way an address is and judged on what remains: one name in every
// spelling a person might type, and nothing that could be mistaken for an address or read two ways
// off a printed slip.
func TestASignInNameIsFoldedAndJudged(t *testing.T) {
	cases := []struct {
		name, raw, want, refusal string
	}{
		{"lower case", "lena", "lena", ""},
		{"capitals fold", "LeNa", "lena", ""},
		{"surrounding space trimmed", "  lena  ", "lena", ""},
		{"exactly three", "abc", "abc", ""},
		{"exactly thirty-two", strings.Repeat("a", 32), strings.Repeat("a", 32), ""},
		{"another script", "Ελένη", "ελένη", ""},
		{"a dot, a dash, an underscore", "oma.erika-2_x", "oma.erika-2_x", ""},
		// "\ufb01" is one character that compatibility folding makes two: counted after folding.
		{"widened past the bound", strings.Repeat("\ufb01", 17), "", "accounts.sign_in_name_too_long"},
		{"two characters", "ab", "", "accounts.sign_in_name_too_short"},
		{"empty", "   ", "", "accounts.sign_in_name_too_short"},
		{"thirty-three", strings.Repeat("a", 33), "", "accounts.sign_in_name_too_long"},
		{"an address's sign", "lena@home", "", "accounts.sign_in_name_malformed"},
		{"a space inside", "lena b", "", "accounts.sign_in_name_malformed"},
		{"a no-break space inside", "lena\u00a0b", "", "accounts.sign_in_name_malformed"},
		{"a control character", "lena\u0007", "", "accounts.sign_in_name_malformed"},
		{"a zero-width space", "le\u200bna", "", "accounts.sign_in_name_malformed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SignInName(c.raw, text.Composing{})
			if c.refusal != "" {
				var domainErr *shared.Error
				if !errors.As(err, &domainErr) || domainErr.DetailCode != c.refusal {
					t.Fatalf("got %q, %v; want the refusal %s", got, err, c.refusal)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

// A decomposed name and its composed twin are one name. Written as escapes, because an editor
// composes what it saves and the test would compare a string with itself.
func TestADecomposedNameIsTheComposedOne(t *testing.T) {
	decomposed, composed := "rene\u0301e", "ren\u00e9e"
	if decomposed == composed {
		t.Fatal("the fixture is not decomposed")
	}
	first, err := SignInName(decomposed, text.Composing{})
	if err != nil {
		t.Fatalf("the decomposed name: %v", err)
	}
	second, err := SignInName(composed, text.Composing{})
	if err != nil {
		t.Fatalf("the composed name: %v", err)
	}
	if first != second {
		t.Errorf("two spellings of one name folded to %q and %q", first, second)
	}
	if FoldSignInName(decomposed, text.Composing{}) != second {
		t.Error("the lookup fold differs from the stored fold")
	}
}

// Without a normaliser a name outside ASCII is refused rather than stored unfolded - two spellings
// would be two accounts for one person.
func TestANameOutsideASCIINeedsTheNormaliser(t *testing.T) {
	if _, err := SignInName("Ελένη", nil); err == nil {
		t.Error("a name outside ASCII was folded without a normaliser")
	}
	if got, err := SignInName("Lena", nil); err != nil || got != "lena" {
		t.Errorf("an ASCII name without a normaliser: %q, %v", got, err)
	}
}

// The shape tells a name from an address, which is what lets sign-in look up either in one way.
func TestAnIdentifierIsANameByItsShape(t *testing.T) {
	if !IsSignInName("lena") || IsSignInName("lena@example.org") {
		t.Error("the shape did not tell a name from an address")
	}
}

// A managed account is a person, active at once, with a name and no address; the display name falls
// back to the name rather than to nothing.
func TestOnlyAnAccountWithANameAndNoAddressIsReachedByThePasswordAlone(t *testing.T) {
	cases := []struct {
		name    string
		account Account
		want    bool
	}{
		{"a sign-in name, no address", Account{SignInName: "lena"}, true},
		{"a sign-in name and an address", Account{SignInName: "lena", Email: "lena@example.org"}, false},
		{"an address only", Account{Email: "bert@example.org"}, false},
		{"neither", Account{}, false},
	}
	for _, c := range cases {
		if got := c.account.OnlyThePasswordReaches(); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAManagedAccountIsAnActivePersonWithAName(t *testing.T) {
	account, err := NewManagedAccount(accountID, tenantID, "Lena", "", text.Composing{})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if account.Kind != AccountUser || account.Status != AccountActive {
		t.Errorf("kind %s status %s, want an active person", account.Kind, account.Status)
	}
	if account.SignInName != "lena" || account.Email != "" {
		t.Errorf("name %q address %q", account.SignInName, account.Email)
	}
	if account.DisplayName != "lena" {
		t.Errorf("display name %q, want the name as the fallback", account.DisplayName)
	}
	if err := account.Verify(); err != nil {
		t.Errorf("a managed account may not act: %v", err)
	}
	if _, err := NewManagedAccount("", tenantID, "lena", "Lena", text.Composing{}); err == nil {
		t.Error("an account without an identifier was produced")
	}
}

// The start password is spelt from the drawn bytes alone: four groups of four, from an alphabet
// without the letters that read as digits, and never from fewer bytes than eighty bits need.
func TestAStartPasswordIsSpeltFromTheDrawnBytes(t *testing.T) {
	drawn := make([]byte, StartPasswordBytes)
	for index := range drawn {
		drawn[index] = byte(index * 37)
	}
	spelt, err := StartPassword(drawn)
	if err != nil {
		t.Fatalf("spelling: %v", err)
	}
	groups := strings.Split(spelt, "-")
	if len(groups) != 4 {
		t.Fatalf("%q has %d groups, want 4", spelt, len(groups))
	}
	for _, group := range groups {
		if len(group) != 4 || strings.ContainsAny(group, "ilouILOU") || strings.ToLower(group) != group {
			t.Errorf("group %q of %q", group, spelt)
		}
	}
	again, _ := StartPassword(drawn)
	if again != spelt {
		t.Error("the same bytes spelt two passwords")
	}
	if _, err := StartPassword(drawn[:StartPasswordBytes-1]); err == nil {
		t.Error("fifteen bytes were spelt into a password")
	}
}

// Off is the stricter value of the switch: a workspace may switch managed accounts off under an open
// "on", never on under an "off" above, and a lock holds whatever the level above decided.
func TestManagedAccountsSwitchOffIsStricter(t *testing.T) {
	inForce := DefaultSignInPolicy()
	if !inForce.ManagedAccounts {
		t.Fatal("the product's default is off")
	}
	if err := Tighten(SwitchManagedAccounts, inForce, PolicyPatch{ManagedAccounts: boolOf(false)}); err != nil {
		t.Errorf("switching off under an open on: %v", err)
	}
	off := inForce
	off.ManagedAccounts = false
	if err := Tighten(SwitchManagedAccounts, off, PolicyPatch{ManagedAccounts: boolOf(true)}); err == nil {
		t.Error("a workspace switched on what the level above switched off")
	}

	instance := PolicyLayer{
		Patch: PolicyPatch{ManagedAccounts: boolOf(false)},
		Locks: map[PolicySwitch]bool{SwitchManagedAccounts: true},
	}
	resolved := Effective(instance, PolicyLayer{}, PolicyLayer{Patch: PolicyPatch{ManagedAccounts: boolOf(true)}})
	if resolved.Policy.ManagedAccounts || resolved.LockOf(SwitchManagedAccounts) != LockInstance {
		t.Errorf("the installation's lock did not hold: %v, lock %q",
			resolved.Policy.ManagedAccounts, resolved.LockOf(SwitchManagedAccounts))
	}

	plan := PolicyLayer{Locks: map[PolicySwitch]bool{SwitchManagedAccounts: true}}
	byPlan := Effective(PolicyLayer{}, plan, PolicyLayer{Patch: PolicyPatch{ManagedAccounts: boolOf(false)}})
	if !byPlan.Policy.ManagedAccounts || byPlan.LockOf(SwitchManagedAccounts) != LockPlan {
		t.Errorf("the plan's lock did not hold the default: %v, lock %q",
			byPlan.Policy.ManagedAccounts, byPlan.LockOf(SwitchManagedAccounts))
	}
}
