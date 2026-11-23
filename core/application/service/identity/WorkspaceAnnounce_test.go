// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// announcements counts the workspace's records, per workspace.
type announcements struct{ told []shared.ID }

func (a *announcements) AnnounceWorkspace(_ context.Context, actor appshared.ActorContext) error {
	a.told = append(a.told, actor.TenantID)
	return nil
}

// announcingBranding is the branding fixture with the record wired into the workspace's writer, as
// the composition root wires it.
func announcingBranding(t *testing.T) (*branding, *announcements) {
	t.Helper()
	told := &announcements{}
	b := newBranding(t, nil, logoPicture(lightLogo, account), logoPicture(squareMark, account))
	b.policy.writer.Announce = told
	brander := WorkspaceBrander{Writer: b.policy.writer, Branding: b.store, Objects: b.objects}
	registry, err := usecase.NewRegistry(nil,
		UpdateWorkspace{Writer: b.policy.writer}.Descriptor(),
		SetWorkspaceHeader{Brander: brander}.Descriptor(),
		SetWorkspacePicture{Brander: brander}.Descriptor(),
		RemoveWorkspacePicture{Brander: brander}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	b.registry = registry
	return b, told
}

// UC-INS-22 check 7: each change of the workspace's name or of its look, and switching its own name
// and logo off, writes the workspace's record once, so that every member's other device reads the
// workspace again without signing in again.
func TestEveryChangeOfTheNameOrTheLookTellsTheOtherDevicesOnce(t *testing.T) {
	b, told := announcingBranding(t)
	actor := workspaceActor()
	steps := []struct {
		name string
		run  func() error
	}{
		{"setting the logo", func() error { _, err := b.setPicture(actor, domain.PictureLogoLight, lightLogo); return err }},
		{"setting the symbol", func() error { _, err := b.setPicture(actor, domain.PictureSymbol, squareMark); return err }},
		{"choosing the logo alone", func() error { _, err := b.setHeader(actor, domain.HeaderLogo); return err }},
		{"removing the logo", func() error { _, err := b.removePicture(actor, domain.PictureLogoLight); return err }},
		{"renaming the workspace", func() error {
			_, err := b.registry.Invoke(context.Background(), UpdateWorkspaceName, actor,
				usecase.Input{"display_name": "Acme Choir"})
			return err
		}},
		{"switching its own name and logo off", func() error {
			_, err := UpdateWorkspace{Writer: b.policy.writer}.Execute(context.Background(), actor, UpdateWorkspaceCommand{
				SignIn:      WorkspacePolicyChange{Policy: domain.PolicyPatch{OwnBranding: boolOf(false)}},
				StepUpToken: "hbt_stp_x",
			})
			return err
		}},
	}
	for _, step := range steps {
		before := len(told.told)
		if err := step.run(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if len(told.told) != before+1 || told.told[len(told.told)-1] != tenant {
			t.Errorf("%s told the other devices %d times, want once for the workspace", step.name, len(told.told)-before)
		}
	}
}

// What moves nothing another device draws tells nobody: the same choice again, a picture that is
// not there, the workspace's language; and a refused change tells nobody either.
func TestWhatChangesNothingTheOthersDrawTellsNobody(t *testing.T) {
	b, told := announcingBranding(t)
	// As the store reads it back: settled, the product's name.
	b.policy.store.row.Branding.Header = domain.HeaderProduct
	actor := workspaceActor()
	if _, err := b.setHeader(actor, domain.HeaderProduct); err != nil {
		t.Fatal(err)
	}
	if _, err := b.removePicture(actor, domain.PictureLogoDark); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.Invoke(context.Background(), UpdateWorkspaceName, actor,
		usecase.Input{"default_locale": "de"}); err != nil {
		t.Fatal(err)
	}
	if len(b.policy.store.updates) != 1 {
		t.Fatalf("the language was written %d times, so this proves nothing", len(b.policy.store.updates))
	}
	if _, err := b.setHeader(actor, domain.HeaderLogo); err == nil {
		t.Fatal("the logo alone without a picture was taken")
	}
	if len(told.told) != 0 {
		t.Errorf("told the other devices %d times, want none", len(told.told))
	}
}
