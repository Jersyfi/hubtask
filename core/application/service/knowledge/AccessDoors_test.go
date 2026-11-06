// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func accessDoors(t *testing.T, s Space) *usecase.Registry {
	t.Helper()
	doors, err := usecase.NewRegistry(nil,
		RestrictPage{s}.Descriptor(), ReadPageAccess{s}.Descriptor(), SharePage{s}.Descriptor(), UnsharePage{s}.Descriptor(),
		ListSharedPages{s}.Descriptor(), ReadPersonalSpace{s}.Descriptor(), OpenDailyNote{s}.Descriptor(),
		PublishPage{s}.Descriptor(), DiscardPageDraft{s}.Descriptor(), ReadSpaceDrafts{s}.Descriptor(), SetSpaceDrafts{s}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return doors
}

// UC-DOC-07 check 3 and UC-DOC-08 checks 1-3 through the doors: a page is restricted to the
// people named, its access read back; it is shared with somebody outside its hub only by who may,
// listed among what is shared with them, and the share taken away; malformed input writes nothing.
func TestAPagesReadersAreChosenThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := accessDoors(t, f.space)
	page := f.create(t, anna, private, "", "Onboarding", "Welcome")
	at := page.ID.String()

	if _, err := doors.Invoke(ctx, RestrictPageName, as(anna), usecase.Input{"page_id": at, "account_ids": []any{"anybody"}}); err == nil {
		t.Fatal("a reader who is no id was accepted")
	}
	if _, err := doors.Invoke(ctx, RestrictPageName, as(bert), usecase.Input{"page_id": at, "account_ids": []any{bert.String()}}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert restricted a page of Anna's private hub: %v", err)
	}
	if f.store.pages[page.ID].Restricted {
		t.Fatal("a refused restriction was written")
	}
	restricted, err := doors.Invoke(ctx, RestrictPageName, as(anna), usecase.Input{"page_id": at, "account_ids": []any{anna.String(), carla.String()}})
	if err != nil || restricted["restricted"] != true {
		t.Fatalf("restricting: %v %v", restricted, err)
	}
	access, err := doors.Invoke(ctx, ReadPageAccessName, as(anna), usecase.Input{"page_id": at})
	if err != nil || len(access["restricted_to"].([]string)) != 2 || access["may_share"] != true || access["public_link"] != nil {
		t.Fatalf("reading the access: %v %v", access, err)
	}
	if _, err := doors.Invoke(ctx, ReadPageAccessName, as(bert), usecase.Input{"page_id": at}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert read the access of a page out of reach: %v", err)
	}
	if out, err := doors.Invoke(ctx, RestrictPageName, as(anna), usecase.Input{"page_id": at, "lift": true}); err != nil || out["restricted"] != false {
		t.Fatalf("lifting: %v %v", out, err)
	}

	if _, err := doors.Invoke(ctx, SharePageName, as(bert), usecase.Input{"page_id": at, "account_id": carla.String()}); err == nil {
		t.Fatal("Bert, outside the hub, shared its page")
	}
	if _, err := doors.Invoke(ctx, SharePageName, as(anna), usecase.Input{"page_id": at, "account_id": "Bert"}); err == nil {
		t.Fatal("a share with somebody who is no id was accepted")
	}
	if len(f.store.shares[page.ID]) != 0 {
		t.Fatal("a refused share was written")
	}
	share, err := doors.Invoke(ctx, SharePageName, as(anna), usecase.Input{"page_id": at, "account_id": bert.String()})
	if err != nil || share == nil {
		t.Fatalf("sharing: %v %v", share, err)
	}
	listed, err := doors.Invoke(ctx, ListSharedPagesName, as(bert), usecase.Input{})
	if err != nil || len(listed["data"].([]usecase.Output)) != 1 {
		t.Fatalf("Bert's shared pages: %v %v", listed, err)
	}
	if _, err := doors.Invoke(ctx, UnsharePageName, as(carla), usecase.Input{"page_id": at, "account_id": bert.String()}); err == nil {
		t.Fatal("somebody outside the hub took the share away")
	}
	if _, err := doors.Invoke(ctx, UnsharePageName, as(anna), usecase.Input{"page_id": at, "account_id": bert.String()}); err != nil {
		t.Fatalf("taking the share away: %v", err)
	}
	if _, err := doors.Invoke(ctx, UnsharePageName, as(anna), usecase.Input{"page_id": at, "account_id": bert.String()}); code(err) != "pages.share_not_found" {
		t.Fatalf("a share taken away twice: %v", err)
	}
	var shares, removals int
	for _, entry := range f.trail.entries {
		switch entry.Action {
		case PageSharedAction:
			shares++
		case PageUnsharedAction:
			removals++
		}
	}
	if shares != 1 || removals != 1 {
		t.Fatalf("the trail holds %d shares and %d removals", shares, removals)
	}
}

// UC-DOC-07 checks 1 and 2 through the doors: the daily note opens in a personal space made on
// first use and read back; a day of no shape is refused.
func TestADailyNoteOpensThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	hubs := &privateHubs{}
	f.space.PersonalHubs, f.space.Words = hubs, words{}
	f.clock.at = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	doors := accessDoors(t, f.space)

	if _, err := doors.Invoke(ctx, OpenDailyNoteName, as(anna), usecase.Input{"day": "tomorrow"}); err == nil {
		t.Fatal("a day of no shape opened a note")
	}
	if hubs.made != 0 {
		t.Fatal("a refused note made a personal space")
	}
	note, err := doors.Invoke(ctx, OpenDailyNoteName, as(anna), usecase.Input{})
	if err != nil || note["title"] != "2026-10-29" || note["hub_id"] != private.String() {
		t.Fatalf("today's note: %v %v", note, err)
	}
	again, err := doors.Invoke(ctx, OpenDailyNoteName, as(anna), usecase.Input{"day": "2026-10-29"})
	if err != nil || again["id"] != note["id"] || hubs.made != 1 {
		t.Fatalf("today's note again: %v %v, %d spaces", again, err, hubs.made)
	}
	if space, err := doors.Invoke(ctx, ReadPersonalSpaceName, as(anna), usecase.Input{}); err != nil || space["hub_id"] != private.String() {
		t.Fatalf("the personal space: %v %v", space, err)
	}
}

// UC-DOC-15 checks 1-3 through the doors: who writes the hub switches its space to drafts,
// a reader cannot; a page waits until published, a draft is discarded by a writer alone; the
// space reads how many drafts wait.
func TestDraftsArePublishedThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := draftsFixture()
	doors := accessDoors(t, f.space)
	space := usecase.Input{"hub_id": hub.String(), "drafts": true}

	if _, err := doors.Invoke(ctx, SetSpaceDraftsName, as(carl), space); err == nil {
		t.Fatal("a reader switched the space")
	}
	if out, err := doors.Invoke(ctx, ReadSpaceDraftsName, as(carl), usecase.Input{"hub_id": hub.String()}); err != nil || out["drafts"] != false {
		t.Fatalf("the space before: %v %v", out, err)
	}
	if out, err := doors.Invoke(ctx, SetSpaceDraftsName, as(anna), space); err != nil || out["drafts"] != true {
		t.Fatalf("switching on: %v %v", out, err)
	}
	if _, err := doors.Invoke(ctx, ReadSpaceDraftsName, as(bert), usecase.Input{"hub_id": private.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert read the private hub's space: %v", err)
	}

	page := f.create(t, anna, hub, "", "Handbook", "v1")
	if _, err := doors.Invoke(ctx, PublishPageName, as(carl), usecase.Input{"page_id": page.ID.String()}); err == nil {
		t.Fatal("a reader published the page")
	}
	published, err := doors.Invoke(ctx, PublishPageName, as(anna), usecase.Input{"page_id": page.ID.String(), "expected_version": page.Version})
	if err != nil || published["restricted"] != false {
		t.Fatalf("publishing: %v %v", published, err)
	}
	if _, err := (UpdatePage{Space: f.space}).Execute(ctx, as(bert), UpdatePageCommand{PageID: page.ID, Change: domain.Change{Body: ptr("v2")}}); err != nil {
		t.Fatal(err)
	}
	if out, err := doors.Invoke(ctx, ReadSpaceDraftsName, as(carl), usecase.Input{"hub_id": hub.String()}); err != nil || out.Int("pending") != 1 {
		t.Fatalf("one draft waits: %v %v", out, err)
	}
	if _, err := doors.Invoke(ctx, DiscardPageDraftName, as(carl), usecase.Input{"page_id": page.ID.String()}); err == nil {
		t.Fatal("a reader discarded a draft")
	}
	if _, err := doors.Invoke(ctx, DiscardPageDraftName, as(bert), usecase.Input{"page_id": page.ID.String()}); err != nil {
		t.Fatalf("discarding: %v", err)
	}
	if out, _ := doors.Invoke(ctx, ReadSpaceDraftsName, as(carl), usecase.Input{"hub_id": hub.String()}); out.Int("pending") != 0 {
		t.Fatalf("a discarded draft still waits: %v", out)
	}
	if _, err := doors.Invoke(ctx, DiscardPageDraftName, as(bert), usecase.Input{"page_id": "draft"}); err == nil {
		t.Fatal("a page that is no id was discarded")
	}
}
