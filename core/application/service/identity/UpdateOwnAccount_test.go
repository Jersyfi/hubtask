// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
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

// Rename writes the name alone, as the statement does.
func (s *accountStore) Rename(_ context.Context, account domain.Account, _ time.Time) error {
	held, found := s.byID[account.ID]
	if !found {
		return shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	held.DisplayName = account.DisplayName
	s.byID[account.ID] = held
	return nil
}

var _ repository.AccountNames = (*accountStore)(nil)

func renameHandler(accounts *accountStore, sink *auditSink) UpdateOwnAccount {
	return UpdateOwnAccount{
		Accounts: accounts, Names: accounts, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), Text: text.Composing{},
	}
}

// writer is the person signed in, with the scope a session carries.
func writer() appshared.ActorContext {
	actor := admin()
	actor.Scopes = []string{accountsWrite}
	return actor
}

func named(value string) *string { return &value }

func TestAPersonRenamesThemselvesAndTheTrailSaysSoWithoutTheName(t *testing.T) {
	accounts := newAccounts(settled(t))
	sink := &auditSink{}

	updated, err := renameHandler(accounts, sink).
		Execute(t.Context(), writer(), UpdateOwnAccountCommand{DisplayName: named("  Anna Müller ")})
	if err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if updated.DisplayName != "Anna Müller" || accounts.byID[adminID].DisplayName != "Anna Müller" {
		t.Fatalf("renamed to %q, stored %q", updated.DisplayName, accounts.byID[adminID].DisplayName)
	}
	// The preferences are untouched: one column, one concern.
	if accounts.byID[adminID].Locale != "de" {
		t.Errorf("the locale changed to %q", accounts.byID[adminID].Locale)
	}

	if len(sink.entries) != 1 {
		t.Fatalf("audit entries %v, want one", sink.entries)
	}
	entry := sink.entries[0]
	if entry.Action != AccountRenamedAction || entry.TargetID != adminID {
		t.Errorf("entry %+v", entry)
	}
	// Rule 10: the name is personal; the change is recorded masked, never in the open.
	masked, ok := entry.Changes["display_name"].(map[string]any)
	if !ok || len(entry.Changes) != 1 || masked["changed"] != true || masked["to"] != nil {
		t.Errorf("changes %+v, want the name masked as changed", entry.Changes)
	}
}

func TestTheSameNameAgainWritesNothing(t *testing.T) {
	accounts := newAccounts(settled(t))
	sink := &auditSink{}

	if _, err := renameHandler(accounts, sink).
		Execute(t.Context(), writer(), UpdateOwnAccountCommand{DisplayName: named("Anna")}); err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if _, err := renameHandler(accounts, sink).
		Execute(t.Context(), writer(), UpdateOwnAccountCommand{}); err != nil {
		t.Fatalf("sending nothing: %v", err)
	}
	if len(sink.entries) != 0 {
		t.Errorf("audit entries %v, want none", sink.entries)
	}
}

func TestARenameIsRefusedWhereNoPersonOrNoNameIsThere(t *testing.T) {
	machine, err := domain.NewServiceAccount(invitedID, tenant, "Nightly export", nil)
	if err != nil {
		t.Fatalf("preparing the service account: %v", err)
	}
	asMachine := writer()
	asMachine.AccountID = invitedID
	nobody := writer()
	nobody.AccountID = ""

	cases := []struct {
		name   string
		actor  appshared.ActorContext
		raw    string
		detail string
	}{
		{name: "a service account", actor: asMachine, raw: "Something else", detail: "accounts.name_needs_a_person"},
		{name: "the system itself", actor: nobody, raw: "Anyone", detail: "accounts.self_required"},
		{name: "a token without the scope", actor: admin(), raw: "Anna B.", detail: "access.insufficient_scope"},
		{name: "an empty name", actor: writer(), raw: "  ", detail: "accounts.display_name_empty"},
		{name: "a name too long", actor: writer(), raw: strings.Repeat("a", 201), detail: "accounts.display_name_too_long"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			accounts := newAccounts(settled(t), machine)
			sink := &auditSink{}
			_, err := renameHandler(accounts, sink).
				Execute(t.Context(), c.actor, UpdateOwnAccountCommand{DisplayName: named(c.raw)})
			if shared.AsError(err).DetailCode != c.detail {
				t.Fatalf("got %v, want %s", err, c.detail)
			}
			if accounts.byID[adminID].DisplayName != "Anna" || accounts.byID[invitedID].DisplayName != "Nightly export" {
				t.Errorf("a refused rename changed a name")
			}
			if len(sink.entries) != 0 {
				t.Errorf("audit entries %v, want none", sink.entries)
			}
		})
	}
}

func TestTheRenameIsDeclaredWithItsAudit(t *testing.T) {
	descriptor := UpdateOwnAccount{}.Descriptor()
	if descriptor.Name != UpdateOwnAccountName || descriptor.TokenScope != accountsWrite {
		t.Errorf("descriptor %s with scope %s", descriptor.Name, descriptor.TokenScope)
	}
	if descriptor.Audit.Action != AccountRenamedAction || !descriptor.Audit.Required {
		t.Errorf("audit %+v", descriptor.Audit)
	}

	accounts := newAccounts(settled(t))
	out, err := renameHandler(accounts, &auditSink{}).Descriptor().Handler.Invoke(
		t.Context(), writer(), usecase.Input{"display_name": "Anna B."})
	if err != nil {
		t.Fatalf("through the handler: %v", err)
	}
	if out["display_name"] != "Anna B." {
		t.Errorf("answered %v", out)
	}
}
