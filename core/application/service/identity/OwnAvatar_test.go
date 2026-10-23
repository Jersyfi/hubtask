// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// SetAvatar writes the picture alone, as the statement does.
func (s *accountStore) SetAvatar(_ context.Context, accountID, mediaID shared.ID, _ time.Time) error {
	held, found := s.byID[accountID]
	if !found {
		return shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	held.AvatarMediaID = mediaID
	s.byID[accountID] = held
	return nil
}

var _ repository.AccountAvatars = (*accountStore)(nil)

// pictures is the media port as far as a picture needs it: Find and the counter. The rest of the
// port is never reached here, and the embedded interface would say so loudly.
type pictures struct {
	mediarepo.Objects
	stored map[shared.ID]media.Object
	counts map[shared.ID]int
}

func newPictures(objects ...media.Object) *pictures {
	p := &pictures{stored: map[shared.ID]media.Object{}, counts: map[shared.ID]int{}}
	for _, object := range objects {
		p.stored[object.ID] = object
	}
	return p
}

func (p *pictures) Find(_ context.Context, id shared.ID) (media.Object, error) {
	object, ok := p.stored[id]
	if !ok {
		return media.Object{}, shared.ErrNotFound.WithDetail("media.not_found")
	}
	return object, nil
}

func (p *pictures) AdjustRefCount(_ context.Context, id shared.ID, delta int) error {
	p.counts[id] += delta
	return nil
}

var (
	pictureID = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")
	otherPic  = shared.ID("01936f2a-7c1e-7000-8000-0000000000c2")
)

func picture(id, uploader shared.ID) media.Object {
	return media.Object{
		ID: id, TenantID: tenant, Usage: media.UsageAvatar, Status: media.StatusReady,
		ContentType: "image/png", ByteSize: 2048, CreatedBy: uploader,
	}
}

func avatarHandlers(accounts *accountStore, objects *pictures, sink *auditSink) (SetOwnAvatar, RemoveOwnAvatar) {
	set := SetOwnAvatar{
		Accounts: accounts, Avatars: accounts, Objects: objects, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
	remove := RemoveOwnAvatar{
		Accounts: accounts, Avatars: accounts, Objects: objects, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
	return set, remove
}

func TestAPersonShowsAPictureAndReplacesItAndTakesItAway(t *testing.T) {
	accounts := newAccounts(settled(t))
	objects := newPictures(picture(pictureID, adminID), picture(otherPic, adminID))
	sink := &auditSink{}
	set, remove := avatarHandlers(accounts, objects, sink)

	updated, err := set.Execute(t.Context(), writer(), pictureID)
	if err != nil {
		t.Fatalf("setting: %v", err)
	}
	if updated.AvatarMediaID != pictureID || accounts.byID[adminID].AvatarMediaID != pictureID {
		t.Fatalf("answered %q, stored %q", updated.AvatarMediaID, accounts.byID[adminID].AvatarMediaID)
	}
	if out := accountOutput(updated); out["avatar_media_id"] != pictureID.String() {
		t.Errorf("the projection carries %v", out["avatar_media_id"])
	}
	if out := accountSummaryOutput(updated); out["avatar_media_id"] != pictureID.String() {
		t.Errorf("what the others see carries %v", out["avatar_media_id"])
	}

	if _, err := set.Execute(t.Context(), writer(), otherPic); err != nil {
		t.Fatalf("replacing: %v", err)
	}
	// The replaced one lost its reference, so the reconciliation takes it; the new one holds one.
	if objects.counts[pictureID] != 0 || objects.counts[otherPic] != 1 {
		t.Errorf("reference counts %v", objects.counts)
	}

	removed, err := remove.Execute(t.Context(), writer())
	if err != nil {
		t.Fatalf("removing: %v", err)
	}
	if !removed.AvatarMediaID.IsZero() || !accounts.byID[adminID].AvatarMediaID.IsZero() {
		t.Errorf("the picture stayed: %q", accounts.byID[adminID].AvatarMediaID)
	}
	if objects.counts[otherPic] != 0 {
		t.Errorf("reference counts %v", objects.counts)
	}
	if _, present := accountOutput(removed)["avatar_media_id"]; present {
		t.Error("an account without a picture still projects one")
	}

	actions := []string{}
	for _, entry := range sink.entries {
		actions = append(actions, string(entry.Action))
		if entry.TargetID != adminID {
			t.Errorf("entry %+v names another target", entry)
		}
	}
	want := []string{"account.avatar_set", "account.avatar_set", "account.avatar_removed"}
	if len(actions) != len(want) || actions[0] != want[0] || actions[2] != want[2] {
		t.Errorf("audit actions %v, want %v", actions, want)
	}
}

func TestTheSamePictureAgainOrNoneToRemoveWritesNothing(t *testing.T) {
	accounts := newAccounts(settled(t))
	objects := newPictures(picture(pictureID, adminID))
	sink := &auditSink{}
	set, remove := avatarHandlers(accounts, objects, sink)

	if _, err := remove.Execute(t.Context(), writer()); err != nil {
		t.Fatalf("removing what is not there: %v", err)
	}
	if _, err := set.Execute(t.Context(), writer(), pictureID); err != nil {
		t.Fatalf("setting: %v", err)
	}
	if _, err := set.Execute(t.Context(), writer(), pictureID); err != nil {
		t.Fatalf("setting again: %v", err)
	}
	if len(sink.entries) != 1 || objects.counts[pictureID] != 1 {
		t.Errorf("entries %d, count %d - want one of each", len(sink.entries), objects.counts[pictureID])
	}
}

func TestAPictureIsRefusedUnlessItIsOnesOwnConfirmedImage(t *testing.T) {
	machine, err := domain.NewServiceAccount(invitedID, tenant, "Nightly export", nil)
	if err != nil {
		t.Fatalf("preparing the service account: %v", err)
	}
	asMachine := writer()
	asMachine.AccountID = invitedID

	strangers := picture(pictureID, invitedID)
	pending := picture(pictureID, adminID)
	pending.Status = media.StatusPending
	cover := picture(pictureID, adminID)
	cover.Usage = media.UsageCover
	svg := picture(pictureID, adminID)
	svg.ContentType = "image/svg+xml"

	cases := []struct {
		name   string
		object *media.Object
		detail string
	}{
		{name: "somebody else's upload", object: &strangers, detail: "media.not_found"},
		{name: "an identifier that names nothing", detail: "media.not_found"},
		{name: "one not yet confirmed", object: &pending, detail: "media.not_ready"},
		{name: "one staged as a cover", object: &cover, detail: "media.usage_mismatch"},
		{name: "an SVG", object: &svg, detail: "media.avatar_not_an_image"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			accounts := newAccounts(settled(t))
			objects := newPictures()
			if c.object != nil {
				objects.stored[c.object.ID] = *c.object
			}
			sink := &auditSink{}
			set, _ := avatarHandlers(accounts, objects, sink)
			_, err := set.Execute(t.Context(), writer(), pictureID)
			if shared.AsError(err).DetailCode != c.detail {
				t.Fatalf("got %v, want %s", err, c.detail)
			}
			if !accounts.byID[adminID].AvatarMediaID.IsZero() || len(sink.entries) != 0 || len(objects.counts) != 0 {
				t.Error("a refused picture changed something")
			}
		})
	}

	t.Run("a service account", func(t *testing.T) {
		accounts := newAccounts(settled(t), machine)
		objects := newPictures(picture(pictureID, invitedID))
		set, remove := avatarHandlers(accounts, objects, &auditSink{})
		if _, err := set.Execute(t.Context(), asMachine, pictureID); shared.AsError(err).DetailCode != "accounts.picture_needs_a_person" {
			t.Errorf("setting got %v", err)
		}
		if _, err := remove.Execute(t.Context(), asMachine); shared.AsError(err).DetailCode != "accounts.picture_needs_a_person" {
			t.Errorf("removing got %v", err)
		}
	})

	t.Run("a token without the scope", func(t *testing.T) {
		accounts := newAccounts(settled(t))
		set, _ := avatarHandlers(accounts, newPictures(picture(pictureID, adminID)), &auditSink{})
		if _, err := set.Execute(t.Context(), admin(), pictureID); shared.AsError(err).DetailCode != "access.insufficient_scope" {
			t.Errorf("got %v", err)
		}
	})
}

func TestThePictureUseCasesAreDeclaredWithTheirAudit(t *testing.T) {
	set, remove := SetOwnAvatar{}.Descriptor(), RemoveOwnAvatar{}.Descriptor()
	if set.Name != SetOwnAvatarName || set.TokenScope != accountsWrite || set.Audit.Action != AvatarSetAction {
		t.Errorf("set is declared %+v", set)
	}
	if remove.Name != RemoveOwnAvatarName || remove.TokenScope != accountsWrite || remove.Audit.Action != AvatarRemovedAction {
		t.Errorf("remove is declared %+v", remove)
	}
}
