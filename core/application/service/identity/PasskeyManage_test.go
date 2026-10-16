// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"fmt"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
