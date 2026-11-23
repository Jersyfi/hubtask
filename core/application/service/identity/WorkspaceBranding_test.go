// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// brandingStore is the workspace's row as the branding port reads and writes it: the same row the
// workspace's read answers, so a test sees what a client reads afterwards.
type brandingStore struct {
	row    *workspaceStore
	writes []domain.Branding
}

func (s *brandingStore) Lock(context.Context) (domain.Branding, error) {
	return s.row.row.Branding, nil
}

func (s *brandingStore) Set(_ context.Context, branding domain.Branding, _ time.Time) error {
	s.writes = append(s.writes, branding)
	s.row.row.Branding = branding
	return nil
}

var (
	lightLogo  = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	darkLogo   = shared.ID("01936f2a-7c1e-7000-8000-0000000000d2")
	squareMark = shared.ID("01936f2a-7c1e-7000-8000-0000000000d3")
)

// logoPicture is a confirmed PNG the caller staged for the workspace, under a file name the trail
// must never carry.
func logoPicture(id, uploader shared.ID) media.Object {
	return media.Object{
		ID: id, TenantID: tenant, Usage: media.UsageWorkspaceLogo, Status: media.StatusReady,
		ContentType: "image/png", ByteSize: 4096, CreatedBy: uploader, FileName: "acme-secret-logo.png",
	}
}

// branding is the three use cases and the workspace's read through one registry, over the
// workspace fixture with its resolver and the authoriser it is handed.
type branding struct {
	registry *usecase.Registry
	policy   *workspacePolicyFixture
	store    *brandingStore
	objects  *pictures
}

func newBranding(t *testing.T, authorizer Authorizer, objects ...media.Object) *branding {
	t.Helper()
	policy := newWorkspacePolicyFixture(now)
	if authorizer != nil {
		policy.writer.Authorizer = authorizer
	}
	b := &branding{policy: policy, store: &brandingStore{row: policy.store}, objects: newPictures(objects...)}
	brander := WorkspaceBrander{Writer: policy.writer, Branding: b.store, Objects: b.objects}
	registry, err := usecase.NewRegistry(nil,
		ReadWorkspace{Writer: policy.writer}.Descriptor(),
		SetWorkspaceHeader{Brander: brander}.Descriptor(),
		SetWorkspacePicture{Brander: brander}.Descriptor(),
		RemoveWorkspacePicture{Brander: brander}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	b.registry = registry
	return b
}

func (b *branding) setPicture(actor appshared.ActorContext, picture domain.Picture, id shared.ID) (usecase.Output, error) {
	return b.registry.Invoke(context.Background(), SetWorkspacePictureName, actor,
		usecase.Input{"picture": string(picture), "media_id": id.String()})
}

func (b *branding) removePicture(actor appshared.ActorContext, picture domain.Picture) (usecase.Output, error) {
	return b.registry.Invoke(context.Background(), RemoveWorkspacePictureName, actor,
		usecase.Input{"picture": string(picture)})
}

func (b *branding) setHeader(actor appshared.ActorContext, header domain.HeaderShows) (usecase.Output, error) {
	return b.registry.Invoke(context.Background(), SetWorkspaceHeaderName, actor,
		usecase.Input{"header": string(header)})
}

// originOf reads the level a refusal names.
func originOf(err error) string {
	if coded := shared.AsError(err); coded != nil {
		return coded.Params["origin"]
	}
	return ""
}

// changeOf reads one field's change as the trail stores it.
func changeOf(changes map[string]any, field string) map[string]any {
	change, _ := changes[field].(map[string]any)
	return change
}

// UC-INS-22 checks 1 and 13: the three pictures are stored and answered with the workspace, each
// in its own place; the replaced picture loses its reference; every change is in the trail as the
// media object's identifier, never its file name.
func TestTheThreePicturesAreStoredAnsweredAndTrailedByIdentifierAlone(t *testing.T) {
	b := newBranding(t, nil,
		logoPicture(lightLogo, account), logoPicture(darkLogo, account), logoPicture(squareMark, account))

	for _, set := range []struct {
		picture domain.Picture
		id      shared.ID
	}{{domain.PictureLogoLight, lightLogo}, {domain.PictureLogoDark, darkLogo}, {domain.PictureSymbol, squareMark}} {
		if _, err := b.setPicture(workspaceActor(), set.picture, set.id); err != nil {
			t.Fatalf("setting %s: %v", set.picture, err)
		}
	}
	read := readWorkspace(t, b.registry)
	answered, _ := read["branding"].(usecase.Output)
	if answered["logo_light_media_id"] != lightLogo.String() || answered["logo_dark_media_id"] != darkLogo.String() ||
		answered["symbol_media_id"] != squareMark.String() || answered["header"] != "PRODUCT" || answered["allowed"] != true {
		t.Fatalf("the workspace answers %v", answered)
	}

	// The dark logo replaced by the symbol's picture: the old one is released.
	if _, err := b.setPicture(workspaceActor(), domain.PictureLogoDark, squareMark); err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if b.objects.counts[darkLogo] != 0 || b.objects.counts[squareMark] != 2 || b.objects.counts[lightLogo] != 1 {
		t.Errorf("reference counts %v", b.objects.counts)
	}

	entries := b.policy.audit.entries
	if len(entries) != 4 {
		t.Fatalf("%d trail entries, want four", len(entries))
	}
	last := entries[3]
	if last.Action != WorkspacePictureSetAction || last.TargetID != tenant || len(last.Changes) != 1 {
		t.Fatalf("the entry is %+v", last)
	}
	change := changeOf(last.Changes, "logo_dark_media_id")
	if change["from"] != darkLogo.String() || change["to"] != squareMark.String() {
		t.Errorf("the entry names %v", change)
	}
	for _, entry := range entries {
		if strings.Contains(fmt.Sprintf("%+v", entry), "acme-secret-logo") {
			t.Errorf("a file name is in the trail: %+v", entry)
		}
	}
}

// UC-INS-22 check 2: what is not a workspace's picture is refused with the stated code, and
// nothing is written - a document, an SVG, a picture staged for something else, one not yet
// confirmed, and one somebody else uploaded, which answers as missing.
func TestAFileThatIsNoWorkspacePictureIsRefusedWithItsCode(t *testing.T) {
	stranger := shared.ID("01936f2a-7c1e-7000-8000-0000000000e9")
	pdf := logoPicture(shared.ID("01936f2a-7c1e-7000-8000-0000000000f1"), account)
	pdf.ContentType = "application/pdf"
	svg := logoPicture(shared.ID("01936f2a-7c1e-7000-8000-0000000000f2"), account)
	svg.ContentType = "image/svg+xml"
	avatar := logoPicture(shared.ID("01936f2a-7c1e-7000-8000-0000000000f3"), account)
	avatar.Usage = media.UsageAvatar
	pending := logoPicture(shared.ID("01936f2a-7c1e-7000-8000-0000000000f4"), account)
	pending.Status = media.StatusPending
	theirs := logoPicture(shared.ID("01936f2a-7c1e-7000-8000-0000000000f5"), stranger)

	cases := []struct {
		name   string
		object media.Object
		want   string
	}{
		{"a document", pdf, "media.workspace_logo_not_an_image"},
		{"an SVG", svg, "media.workspace_logo_not_an_image"},
		{"a person's picture", avatar, "media.usage_mismatch"},
		{"an upload not confirmed", pending, "media.not_ready"},
		{"somebody else's upload", theirs, "media.not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBranding(t, nil, tc.object)
			_, err := b.setPicture(workspaceActor(), domain.PictureLogoLight, tc.object.ID)
			if detailCode(err) != tc.want {
				t.Fatalf("answered %v, want %q", err, tc.want)
			}
			if len(b.store.writes) != 0 || len(b.objects.counts) != 0 || len(b.policy.audit.entries) != 0 {
				t.Errorf("a refusal wrote %v, counted %v, trailed %d", b.store.writes, b.objects.counts, len(b.policy.audit.entries))
			}
		})
	}
	b := newBranding(t, nil)
	if _, err := b.setPicture(workspaceActor(), domain.PictureLogoLight, lightLogo); detailCode(err) != "media.not_found" {
		t.Errorf("an object that does not exist answered %v", err)
	}
	if _, err := b.registry.Invoke(context.Background(), SetWorkspacePictureName, workspaceActor(),
		usecase.Input{"picture": "FAVICON", "media_id": lightLogo.String()}); err == nil {
		t.Error("a fourth picture was accepted")
	}
}

// UC-INS-22 checks 3 and 4: the choice is stored and answered; the logo alone without a picture is
// refused; removing the last picture under the logo alone returns the header to the product's
// name in the same write, and the trail says both.
func TestTheHeadersChoiceIsStoredAndFollowsTheLastPicture(t *testing.T) {
	b := newBranding(t, nil, logoPicture(squareMark, account))

	if _, err := b.setHeader(workspaceActor(), domain.HeaderLogo); detailCode(err) != "workspace.logo_alone_needs_a_picture" {
		t.Fatalf("the logo alone without a picture answered %v", err)
	}
	out, err := b.setHeader(workspaceActor(), domain.HeaderWorkspaceName)
	if err != nil || out["header"] != "WORKSPACE_NAME" {
		t.Fatalf("the workspace's name: %v, %v", out, err)
	}
	if answered, _ := readWorkspace(t, b.registry)["branding"].(usecase.Output); answered["header"] != "WORKSPACE_NAME" {
		t.Errorf("the workspace answers %v", answered)
	}
	if _, err := b.setPicture(workspaceActor(), domain.PictureSymbol, squareMark); err != nil {
		t.Fatalf("the symbol: %v", err)
	}
	if _, err := b.setHeader(workspaceActor(), domain.HeaderLogo); err != nil {
		t.Fatalf("the logo alone with the symbol: %v", err)
	}

	writes := len(b.store.writes)
	out, err = b.removePicture(workspaceActor(), domain.PictureSymbol)
	if err != nil || out["header"] != "PRODUCT" || out["symbol_media_id"] != nil {
		t.Fatalf("removing the last picture answered %v, %v", out, err)
	}
	if len(b.store.writes) != writes+1 {
		t.Errorf("%d writes for one removal", len(b.store.writes)-writes)
	}
	if b.objects.counts[squareMark] != 0 {
		t.Errorf("the removed picture still counts %d", b.objects.counts[squareMark])
	}
	entry := b.policy.audit.entries[len(b.policy.audit.entries)-1]
	if entry.Action != WorkspacePictureRemovedAction || changeOf(entry.Changes, "symbol_media_id")["from"] != squareMark.String() ||
		changeOf(entry.Changes, "header_shows")["to"] != "PRODUCT" {
		t.Errorf("the removal's entry is %+v", entry)
	}

	// Removing what is not there writes nothing and answers as it stands.
	trailed := len(b.policy.audit.entries)
	if _, err := b.removePicture(workspaceActor(), domain.PictureLogoDark); err != nil {
		t.Fatalf("removing nothing: %v", err)
	}
	if len(b.policy.audit.entries) != trailed || len(b.store.writes) != writes+1 {
		t.Error("removing a picture that is not there wrote something")
	}
}

// UC-INS-22 check 11: where the installation or a plan switched the branding off, every change is
// refused naming who did, and nothing is written; the workspace's read says so too.
func TestALockAboveRefusesEveryChangeAndNamesItsOrigin(t *testing.T) {
	off := domain.PolicyLayer{
		Patch: domain.PolicyPatch{OwnBranding: boolOf(false)},
		Locks: map[domain.PolicySwitch]bool{domain.SwitchOwnBranding: true},
	}
	for _, level := range []string{"INSTANCE", "PLAN"} {
		t.Run(level, func(t *testing.T) {
			b := newBranding(t, nil, logoPicture(lightLogo, account))
			if level == "INSTANCE" {
				b.policy.instance.level.Policy = off
			} else {
				b.policy.store.row.Plan = domain.WorkspacePlan{Name: "family", Policy: off}
			}
			b.policy.store.row.Branding = domain.Branding{Header: domain.HeaderLogo, LogoLight: lightLogo}

			attempts := map[string]func() (usecase.Output, error){
				"the choice": func() (usecase.Output, error) { return b.setHeader(workspaceActor(), domain.HeaderProduct) },
				"a picture": func() (usecase.Output, error) {
					return b.setPicture(workspaceActor(), domain.PictureLogoDark, lightLogo)
				},
				"a removal": func() (usecase.Output, error) { return b.removePicture(workspaceActor(), domain.PictureLogoLight) },
			}
			for name, attempt := range attempts {
				_, err := attempt()
				if !errors.Is(err, shared.ErrForbidden) || detailCode(err) != "workspace.branding_off" ||
					originOf(err) != level {
					t.Errorf("%s answered %v from %q, want workspace.branding_off from %s", name, err, originOf(err), level)
				}
			}
			if len(b.store.writes) != 0 || len(b.policy.audit.entries) != 0 {
				t.Errorf("a refused change wrote %v", b.store.writes)
			}
			read := readWorkspace(t, b.registry)
			answered, _ := read["branding"].(usecase.Output)
			policy, _ := read["sign_in_policy"].(usecase.Output)
			row, _ := policy["own_branding"].(usecase.Output)
			if answered["allowed"] != false || row["value"] != false || row["lock"] != level {
				t.Errorf("the read answers branding %v and the switch %v", answered, row)
			}
		})
	}
}

// UC-INS-22 check 11: a workspace's own off is a refusal from the workspace, and the switch open
// above leaves the branding allowed.
func TestTheSwitchOpenAboveAllowsAndAWorkspacesOwnOffRefuses(t *testing.T) {
	b := newBranding(t, nil)
	if _, err := b.setHeader(workspaceActor(), domain.HeaderWorkspaceName); err != nil {
		t.Fatalf("the default refused: %v", err)
	}
	b.policy.store.row.Settings.SignIn = domain.PolicyPatch{OwnBranding: boolOf(false)}
	_, err := b.setHeader(workspaceActor(), domain.HeaderProduct)
	if detailCode(err) != "workspace.branding_off" || originOf(err) != "WORKSPACE" {
		t.Errorf("the workspace's own off answered %v", err)
	}
}

// Rule 2, through the registry and the real authoriser: whoever may change the workspace's name -
// an owner, an administrator, a service account holding the role - changes its branding; a member,
// a contributor, a viewer, a guest and an auditor are refused, and so is a credential without the
// workspace's scope. A refusal writes nothing.
func TestOnlyWhoMayChangeTheWorkspacesNameChangesItsBranding(t *testing.T) {
	cases := []struct {
		name    string
		role    domain.Role
		kind    shared.ActorKind
		scopes  []string
		allowed bool
	}{
		{"an owner", domain.RoleOwner, shared.ActorUser, nil, true},
		{"an administrator", domain.RoleAdmin, shared.ActorUser, nil, true},
		{"a service account as administrator", domain.RoleAdmin, shared.ActorServiceAccount, nil, true},
		{"a member", domain.RoleMember, shared.ActorUser, nil, false},
		{"a contributor", domain.RoleContributor, shared.ActorUser, nil, false},
		{"a viewer", domain.RoleViewer, shared.ActorUser, nil, false},
		{"a guest", domain.RoleGuest, shared.ActorUser, nil, false},
		{"an auditor", domain.RoleAuditor, shared.ActorUser, nil, false},
		{"an administrator's token without workspace:manage", domain.RoleAdmin, shared.ActorUser, []string{workspaceRead}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &auditSink{}
			authoriser := access.Service{
				Memberships: membershipsFake{roles: []domain.Role{tc.role}},
				UnitOfWork:  &unitOfWork{}, Audit: sink, Clock: clock.Fixed(now),
			}
			b := newBranding(t, authoriser, logoPicture(lightLogo, account))
			actor := workspaceActor()
			actor.Kind = tc.kind
			if tc.scopes != nil {
				actor.Scopes = tc.scopes
			}
			_, headerErr := b.setHeader(actor, domain.HeaderWorkspaceName)
			_, pictureErr := b.setPicture(actor, domain.PictureLogoLight, lightLogo)
			_, removeErr := b.removePicture(actor, domain.PictureLogoLight)
			for name, err := range map[string]error{"the choice": headerErr, "a picture": pictureErr, "a removal": removeErr} {
				if tc.allowed && err != nil {
					t.Errorf("%s was refused: %v", name, err)
				}
				if !tc.allowed && !errors.Is(err, shared.ErrForbidden) {
					t.Errorf("%s answered %v, want forbidden", name, err)
				}
			}
			if !tc.allowed && len(b.store.writes) != 0 {
				t.Errorf("a refused caller wrote %v", b.store.writes)
			}
		})
	}
}
