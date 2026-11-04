// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"strings"
	"testing"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

type minterFake struct {
	minted MintedReset
	asked  [][2]shared.ID
}

func (m *minterFake) MintMachineReset(_ context.Context, tenantID, accountID shared.ID) (MintedReset, error) {
	m.asked = append(m.asked, [2]shared.ID{tenantID, accountID})
	return m.minted, nil
}

type baseFake string

func (b baseFake) Of(context.Context) (string, error) { return string(b), nil }

func newResetLinkFixture(minted MintedReset, base string) (IssueResetLink, *minterFake, *journalStore) {
	register := newRegister()
	register.addresses = map[string]shared.ID{"acme\x00jerome@example.eu": adaAccount}
	writer, _, journal := newInstanceWriter(register)
	workspaces := newRecoverFixture(adminrepo.TenantRecord{})
	minter := &minterFake{minted: minted}
	return IssueResetLink{
		Workspaces: workspaces.handler, Writer: writer, Minter: minter, Addresses: baseFake(base),
	}, minter, journal
}

// Fifth answers 5: a self-hoster without mail gets the reset link from the machine - minted by the
// reset's own minting for the account the address names in that workspace, journalled as the
// machine's without the address or the token, the token in the fragment.
func TestTheMachineIssuesAResetLink(t *testing.T) {
	issue, minter, journal := newResetLinkFixture(MintedReset{Token: secret.New("tok/en")}, "https://tasks.example.eu/")

	link, err := issue.Execute(t.Context(), " Acme ", "jerome@example.eu")
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	if link != "https://tasks.example.eu/reset#token=tok%2Fen" {
		t.Errorf("link = %q", link)
	}
	if len(minter.asked) != 1 || minter.asked[0] != [2]shared.ID{lifecycleTenant, adaAccount} {
		t.Errorf("minted for %v", minter.asked)
	}
	if len(journal.entries) != 1 {
		t.Fatalf("journal = %+v", journal.entries)
	}
	entry := journal.entries[0]
	if entry.Action != "instance.reset_link_issued" || entry.ActorLabel != "machine" ||
		entry.Details["account_id"] != adaAccount.String() {
		t.Errorf("journal entry = %+v", entry)
	}
	for _, value := range entry.Details {
		if text, _ := value.(string); strings.Contains(text, "@") || strings.Contains(text, "tok") {
			t.Errorf("the journal holds the address or the token: %+v", entry.Details)
		}
	}
}

// A workspace without the password connects its provider through the same link's other key.
func TestTheMachinesLinkConnectsWhereThePasswordIsOff(t *testing.T) {
	issue, _, _ := newResetLinkFixture(MintedReset{Token: secret.New("abc"), Connect: true}, "https://tasks.example.eu")
	link, err := issue.Execute(t.Context(), "acme", "jerome@example.eu")
	if err != nil || link != "https://tasks.example.eu/reset#connect=abc" {
		t.Errorf("link = %q, %v", link, err)
	}
}

// Nothing is minted or journalled for an address that names nobody, a workspace that does not
// exist, an installation with no address, or an account no password reset opens.
func TestTheMachineRefusesAResetLinkItCannotIssue(t *testing.T) {
	cases := []struct {
		name, slug, email, base string
		minted                  MintedReset
		code                    string
		minted0                 bool
	}{
		{"unknown address", "acme", "nobody@example.eu", "https://x.eu", MintedReset{Token: secret.New("t")}, "admin.operator_unknown_account", true},
		{"unknown workspace", "nowhere", "jerome@example.eu", "https://x.eu", MintedReset{Token: secret.New("t")}, "admin.tenant_not_found", true},
		{"no address", "acme", "jerome@example.eu", "", MintedReset{Token: secret.New("t")}, "admin.reset_link_no_address", true},
		{"provider only", "acme", "jerome@example.eu", "https://x.eu", MintedReset{}, "admin.reset_link_provider", false},
		{"incomplete", "acme", " ", "https://x.eu", MintedReset{Token: secret.New("t")}, "admin.operator_incomplete", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			issue, minter, journal := newResetLinkFixture(c.minted, c.base)
			_, err := issue.Execute(t.Context(), c.slug, c.email)
			if shared.AsError(err).DetailCode != c.code {
				t.Errorf("answered %v, want %s", err, c.code)
			}
			if c.minted0 && len(minter.asked) != 0 {
				t.Errorf("minted although refused first: %v", minter.asked)
			}
			if len(journal.entries) != 0 {
				t.Errorf("a refused link was journalled: %+v", journal.entries)
			}
		})
	}
}
