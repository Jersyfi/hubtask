// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	annasPicture = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c201")
	privatePic   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c202")
	pendingPic   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c203")
)

// uploads are Anna's two pictures and Bert's upload not confirmed; which pages hold them is the
// store's.
type uploads struct{ store *store }

func (uploads) Find(_ context.Context, id shared.ID) (media.Object, error) {
	switch id {
	case annasPicture, privatePic:
		return media.Object{ID: id, Status: media.StatusReady, CreatedBy: anna}, nil
	case pendingPic:
		return media.Object{ID: id, Status: media.StatusPending, CreatedBy: bert}, nil
	}
	return media.Object{}, shared.ErrNotFound.WithDetail("media.not_found")
}

func (u uploads) ReferencingPages(_ context.Context, id shared.ID) ([]shared.ID, error) {
	var out []shared.ID
	for page, held := range u.store.media {
		for _, object := range held {
			if object == id && !u.store.pages[page].IsTrashed() {
				out = append(out, page)
			}
		}
	}
	return out, nil
}

// UC-DOC-14 check 1: a writer attaches their own upload to a page, and the trail names it.
// UC-DOC-14 check 2: a picture a page they may read holds is attached too (copied between pages);
// one only a page out of reach holds, an unconfirmed upload or none at all is not found, and
// somebody who may not write in the page's hub attaches nothing.
func TestAPictureIsAttachedToAPage(t *testing.T) {
	f := newFixture()
	f.space.Media = uploads{store: f.store}
	ctx := context.Background()
	page := f.create(t, anna, hub, "", "Handbook", "")
	if _, err := (AttachPageMedia{Space: f.space}).Descriptor().Handler.Invoke(ctx, as(anna),
		usecase.Input{"page_id": page.ID.String(), "media_id": annasPicture.String()}); err != nil {
		t.Fatal(err)
	}
	if got := f.store.media[page.ID]; len(got) != 1 || got[0] != annasPicture {
		t.Fatalf("held %v", got)
	}
	if last := f.trail.entries[len(f.trail.entries)-1]; last.Action != PageMediaAttachedAction {
		t.Fatalf("recorded %v", last.Action)
	}
	secret := f.create(t, anna, private, "", "Notes", "")
	if err := (AttachPageMedia{Space: f.space}).Execute(ctx, as(anna), secret.ID, privatePic); err != nil {
		t.Fatal(err)
	}

	other := f.create(t, bert, hub, "", "Copy", "")
	if err := (AttachPageMedia{Space: f.space}).Execute(ctx, as(bert), other.ID, annasPicture); err != nil {
		t.Errorf("a picture a readable page holds: %v", err)
	}
	for _, c := range []struct {
		name   string
		object shared.ID
	}{
		{"a picture only a private page holds", privatePic},
		{"an upload not confirmed", pendingPic},
		{"no upload at all", shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c299")},
	} {
		if err := (AttachPageMedia{Space: f.space}).Execute(ctx, as(bert), other.ID, c.object); code(err) != "pages.media_not_found" {
			t.Errorf("%s: got %v", c.name, err)
		}
	}
	if err := (AttachPageMedia{Space: f.space}).Execute(ctx, as(bert), secret.ID, annasPicture); code(err) == "" {
		t.Error("somebody outside the private hub attached a file to its page")
	}
}

// UC-DOC-14 check 3: whoever reads a page holding a picture reads the picture - by the hub, or by
// a share alone -, and a restriction that keeps somebody from the page keeps them from it too.
func TestAPagesPictureIsReadByThePagesReaders(t *testing.T) {
	f := newFixture()
	f.space.Media = uploads{store: f.store}
	ctx := context.Background()
	reach := PageMediaReach{Space: f.space}
	open := f.create(t, anna, hub, "", "Handbook", "")
	secret := f.create(t, anna, private, "", "Notes", "")

	if reads, err := reach.ReadsAny(ctx, as(bert), []shared.ID{open.ID}); err != nil || !reads {
		t.Fatalf("a reader of the hub: %v, %v", reads, err)
	}
	if reads, _ := reach.ReadsAny(ctx, as(bert), []shared.ID{secret.ID}); reads {
		t.Fatal("somebody outside the private hub read its page's picture")
	}
	if _, err := (SharePage{Space: f.space}).Execute(ctx, as(anna), secret.ID, bert); err != nil {
		t.Fatal(err)
	}
	if reads, _ := reach.ReadsAny(ctx, as(bert), []shared.ID{secret.ID}); !reads {
		t.Fatal("the person a page is shared with did not read its picture")
	}
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: open.ID, AccountIDs: []shared.ID{anna}}); err != nil {
		t.Fatal(err)
	}
	if reads, _ := reach.ReadsAny(ctx, as(bert), []shared.ID{open.ID}); reads {
		t.Fatal("a restriction did not keep its picture from somebody it does not name")
	}
	if reads, _ := reach.ReadsAny(ctx, as(anna), []shared.ID{open.ID}); !reads {
		t.Fatal("the person a restriction names did not read the picture")
	}
}
