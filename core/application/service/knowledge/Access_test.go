// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var carla = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b006")

// The store's access, in memory: the restrictions on the way up, combined as the adapter does.
func (s *store) Readers(_ context.Context, ids []shared.ID) (map[shared.ID]domain.Readers, error) {
	out := map[shared.ID]domain.Readers{}
	for _, id := range ids {
		var sets [][]shared.ID
		for at, depth := id, 0; !at.IsZero() && depth < 16; depth++ {
			page, ok := s.pages[at]
			if !ok {
				break
			}
			if page.Restricted {
				sets = append(sets, append([]shared.ID{}, s.restrictedTo[at]...))
			}
			at = page.ParentID
		}
		out[id] = domain.CombineRestrictions(sets)
	}
	return out, nil
}
func (s *store) RestrictedTo(_ context.Context, id shared.ID) ([]shared.ID, error) {
	return s.restrictedTo[id], nil
}
func (s *store) Restrict(_ context.Context, page domain.Page, accounts []shared.ID, expected int) error {
	if s.pages[page.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	page.Version = expected + 1
	s.pages[page.ID] = page
	s.restrictedTo[page.ID] = accounts
	return nil
}
func (s *store) InsertShare(_ context.Context, share domain.Share) (bool, error) {
	for _, held := range s.shares[share.PageID] {
		if held.AccountID == share.AccountID {
			return false, nil
		}
	}
	s.shares[share.PageID] = append(s.shares[share.PageID], share)
	return true, nil
}
func (s *store) DeleteShare(_ context.Context, page, account shared.ID) (bool, error) {
	before := len(s.shares[page])
	s.shares[page] = slices.DeleteFunc(s.shares[page], func(sh domain.Share) bool { return sh.AccountID == account })
	return len(s.shares[page]) < before, nil
}
func (s *store) Shares(_ context.Context, page shared.ID, _ int) ([]domain.Share, error) {
	return s.shares[page], nil
}
func (s *store) SharedWith(_ context.Context, page, account shared.ID) (bool, error) {
	for _, held := range s.shares[page] {
		if held.AccountID == account {
			return true, nil
		}
	}
	return false, nil
}
func (s *store) PagesSharedWith(ctx context.Context, account shared.ID, _ int) ([]repository.Summary, error) {
	var out []repository.Summary
	for id, shares := range s.shares {
		for _, held := range shares {
			if held.AccountID == account {
				page := s.pages[id]
				readers, _ := s.Readers(ctx, []shared.ID{id})
				out = append(out, repository.Summary{ID: id, HubID: page.HubID, Title: page.Title, Readers: readers[id]})
			}
		}
	}
	return out, nil
}
func (s *store) PersonalSpace(_ context.Context, account shared.ID) (shared.ID, bool, error) {
	hub, ok := s.personal[account]
	return hub, ok, nil
}
func (s *store) ClaimPersonalSpace(_ context.Context, account, hub shared.ID, _ time.Time) (shared.ID, error) {
	if held, ok := s.personal[account]; ok {
		return held, nil
	}
	s.personal[account] = hub
	return hub, nil
}
func (s *store) DailyNote(_ context.Context, hub shared.ID, day time.Time) (shared.ID, bool, error) {
	for _, page := range s.pages {
		if page.HubID == hub && page.DailyFor != nil && page.DailyFor.Equal(day) && !page.IsTrashed() {
			return page.ID, true, nil
		}
	}
	return "", false, nil
}

func (s *store) AttachMedia(_ context.Context, page, object shared.ID) error {
	if s.media == nil {
		s.media = map[shared.ID][]shared.ID{}
	}
	if !slices.Contains(s.media[page], object) {
		s.media[page] = append(s.media[page], object)
	}
	return nil
}
func (s *store) CountMedia(_ context.Context, page shared.ID) (int, error) {
	return len(s.media[page]), nil
}

// changes keeps what the change log was told.
type changes struct{ recorded []changelog.Change }

func (c *changes) Record(_ context.Context, change changelog.Change) error {
	c.recorded = append(c.recorded, change)
	return nil
}

type hlc struct{ n int64 }

func (h *hlc) Next() shared.HLC { h.n++; return shared.HLC{Counter: uint32(h.n), Device: "server"} }

// UC-DOC-07 check 3: a page restricted to fewer people is read, listed, found and opened live by
// them alone, the pages under it too, and its offline copies are taken back.
func TestARestrictedPageIsReadOnlyByThePeopleItNames(t *testing.T) {
	f := newFixture()
	log := &changes{}
	f.space.Changes, f.space.HLC = log, &hlc{}
	ctx := context.Background()
	top := f.create(t, anna, hub, "", "Salaries", "budget lines")
	under := f.create(t, anna, hub, top.ID, "Bonus", "budget bonus")

	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(bert), RestrictPageCommand{PageID: top.ID, AccountIDs: []shared.ID{anna}}); code(err) != "pages.restriction_excludes_you" {
		t.Fatalf("a restriction without its author answered %v", err)
	}
	restricted, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: top.ID, AccountIDs: []shared.ID{anna, carla}})
	if err != nil || !restricted.Restricted {
		t.Fatalf("RestrictPage: %+v %v", restricted, err)
	}
	for _, id := range []shared.ID{top.ID, under.ID} {
		if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(bert), id); code(err) != "pages.not_found" {
			t.Errorf("Bert read %s past the restriction: %v", id, err)
		}
		if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(carla), id); err != nil {
			t.Errorf("Carla, named, could not read %s: %v", id, err)
		}
		if _, err := (Locator{Space: f.space}).Locate(ctx, as(bert), id); code(err) != "pages.not_found" {
			t.Errorf("Bert opened %s's live document: %v", id, err)
		}
	}
	tree, err := (ListPages{Space: f.space}).Execute(ctx, as(bert), ListPagesQuery{HubID: hub})
	if err != nil || len(tree) != 0 {
		t.Errorf("Bert's tree holds %d pages, %v", len(tree), err)
	}
	hits, err := (SearchPages{Space: f.space}).Execute(ctx, as(bert), SearchPagesQuery{Text: "budget", Limit: 10})
	if err != nil || len(hits) != 0 {
		t.Errorf("Bert found %d restricted pages, %v", len(hits), err)
	}
	if _, err := (UpdatePage{Space: f.space}).Execute(ctx, as(bert), UpdatePageCommand{PageID: under.ID, Change: domain.Change{Title: ptr("Mine")}}); code(err) != "pages.not_found" {
		t.Errorf("Bert wrote under the restriction: %v", err)
	}
	dropped := map[shared.ID]bool{}
	for _, change := range log.recorded {
		if change.Op == changelog.Delete {
			dropped[change.EntityID] = true
		}
	}
	if !dropped[top.ID] || !dropped[under.ID] {
		t.Errorf("the offline copies were not taken back: %v", dropped)
	}

	// A page below narrows further and never widens: Bert named below the restriction stays out.
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: under.ID, AccountIDs: []shared.ID{anna, bert}}); err != nil {
		t.Fatalf("restricting below: %v", err)
	}
	if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(bert), under.ID); code(err) != "pages.not_found" {
		t.Errorf("a restriction below widened the one above: %v", err)
	}
	if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(carla), under.ID); code(err) != "pages.not_found" {
		t.Errorf("Carla, not named below, read the narrower page: %v", err)
	}

	// Lifted, the hub decides again.
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: top.ID, Lift: true}); err != nil {
		t.Fatalf("lifting: %v", err)
	}
	if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(bert), top.ID); err != nil {
		t.Errorf("a lifted page is still closed: %v", err)
	}
}

// A restriction whose people are all shut out is lifted by somebody managing the hub's members -
// never narrowed by them, and never read without being lifted.
func TestALockedOutRestrictionIsLiftedNotRead(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	page := f.create(t, anna, hub, "", "Plans", "")
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: page.ID, AccountIDs: []shared.ID{anna}}); err != nil {
		t.Fatal(err)
	}
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(bert), RestrictPageCommand{PageID: page.ID, AccountIDs: []shared.ID{bert}}); code(err) != "pages.not_found" {
		t.Errorf("a person kept out restricted it again: %v", err)
	}
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(bert), RestrictPageCommand{PageID: page.ID, Lift: true}); err != nil {
		t.Errorf("a manager of the hub could not lift it: %v", err)
	}
}

// UC-DOC-08 checks 1 and 2: a page shared with somebody outside its hub is read by them, alone and
// read-only - not its comments, its history, the pages under it or its breadcrumb - and listed
// among what is shared with them; a share never passes a restriction, and they may leave it.
func TestASharedPageIsReadOutsideItsHubAndNothingMore(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	parent := f.create(t, anna, private, "", "Handbook", "")
	page := f.create(t, anna, private, parent.ID, "Onboarding", "Welcome")
	under := f.create(t, anna, private, page.ID, "Day one", "")

	if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(bert), page.ID); err == nil {
		t.Fatal("Bert read the page before it was shared")
	}
	if _, err := (SharePage{Space: f.space}).Execute(ctx, as(bert), page.ID, carla); err == nil {
		t.Error("Bert, outside the hub, shared its page")
	}
	if _, err := (SharePage{Space: f.space}).Execute(ctx, as(anna), page.ID, bert); err != nil {
		t.Fatalf("SharePage: %v", err)
	}
	read, err := (ReadPage{Space: f.space}).Execute(ctx, as(bert), page.ID)
	if err != nil || !read.Shared || read.Writable || len(read.Ancestors) != 0 || read.Page.Body != "Welcome" {
		t.Fatalf("Bert's shared read: %+v %v", read, err)
	}
	if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(bert), under.ID); err == nil {
		t.Error("the share opened the page under it")
	}
	if _, err := (ListPageComments{Space: f.space}).Execute(ctx, as(bert), page.ID); err == nil {
		t.Error("the share opened the comments")
	}
	if _, err := (UpdatePage{Space: f.space}).Execute(ctx, as(bert), UpdatePageCommand{PageID: page.ID, Change: domain.Change{Title: ptr("Mine")}}); err == nil {
		t.Error("the share let Bert write")
	}
	listed, err := (ListSharedPages{Space: f.space}).Execute(ctx, as(bert))
	if err != nil || len(listed) != 1 || listed[0].ID != page.ID {
		t.Errorf("Bert's shared pages: %+v %v", listed, err)
	}

	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: parent.ID, AccountIDs: []shared.ID{anna}}); err != nil {
		t.Fatal(err)
	}
	if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(bert), page.ID); err == nil {
		t.Error("the share passed a restriction above")
	}
	if _, err := (SharePage{Space: f.space}).Execute(ctx, as(anna), page.ID, carla); code(err) != "pages.share_restricted" {
		t.Errorf("a share past the restriction answered %v", err)
	}
	if err := (UnsharePage{Space: f.space}).Execute(ctx, as(bert), page.ID, bert); err != nil {
		t.Errorf("Bert could not leave his share: %v", err)
	}
	if err := (UnsharePage{Space: f.space}).Execute(ctx, as(bert), page.ID, bert); code(err) != "pages.share_not_found" {
		t.Errorf("a share left twice answered %v", err)
	}
}

type privateHubs struct {
	made  int
	named string
}

func (p *privateHubs) MakePrivateHub(_ context.Context, _ appshared.ActorContext, name string) (shared.ID, error) {
	p.made++
	p.named = name
	return private, nil
}

type words struct{}

func (words) Render(locale, code string, _ map[string]string) string { return locale + ":" + code }

// UC-DOC-07 checks 1 and 2: the first daily note makes a personal space - a private hub named in
// the person's language - and today's note in their zone; opening it again answers the same note,
// and another day its own.
func TestADailyNoteOpensInAPersonalSpace(t *testing.T) {
	f := newFixture()
	hubs := &privateHubs{}
	f.space.PersonalHubs, f.space.Words = hubs, words{}
	f.clock.at = time.Date(2026, 10, 29, 23, 30, 0, 0, time.UTC)
	ctx := context.Background()
	actor := as(anna)
	actor.Locale, actor.TimeZone = "de", "Europe/Vienna"

	note, err := (OpenDailyNote{Space: f.space}).Execute(ctx, actor, "")
	if err != nil {
		t.Fatalf("OpenDailyNote: %v", err)
	}
	if note.HubID != private || note.Title != "2026-10-30" || note.DailyFor == nil || hubs.named != "de:"+personalSpaceNameCode {
		t.Fatalf("the first note: %+v, space named %q", note, hubs.named)
	}
	again, err := (OpenDailyNote{Space: f.space}).Execute(ctx, actor, "2026-10-30")
	if err != nil || again.ID != note.ID || hubs.made != 1 {
		t.Errorf("today again: %v %v, %d spaces made", again.ID, err, hubs.made)
	}
	other, err := (OpenDailyNote{Space: f.space}).Execute(ctx, actor, "2026-10-28")
	if err != nil || other.ID == note.ID || other.Title != "2026-10-28" {
		t.Errorf("another day: %+v %v", other, err)
	}
	space, err := ReadPersonalSpace{Space: f.space}.Descriptor().Handler.Invoke(ctx, actor, nil)
	if err != nil || space["hub_id"] != private.String() {
		t.Errorf("ReadPersonalSpace: %v %v", space, err)
	}
	if _, err := (ReadPage{Space: f.space}).Execute(ctx, as(bert), note.ID); err == nil {
		t.Error("somebody else read the daily note")
	}
}
