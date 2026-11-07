// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

func (h *linkHarness) merger() MergeWorkItems {
	return MergeWorkItems{
		Link: LinkEntries{Linking: h.linking()},
		Archive: ArchiveWorkItem{Lifecycle: LifecycleWriter{
			Items: h.items, Containers: h.containers, Authorizer: h.authorizer,
			Events: h.events, Changes: h.changes, Audit: h.audit,
			Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
			UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{}, Queue: &jobs{},
		}},
	}
}

// UC-SVC-03 check 5: a merge links the entry as a duplicate of the other and archives it, asking
// the writer of both; nothing is copied or deleted.
func TestAMergeLinksTheEntryAsADuplicateAndArchivesIt(t *testing.T) {
	h := newLinkHarness()

	result, err := h.merger().Execute(context.Background(), linkActor(), MergeWorkItemsCommand{ItemID: h.one.ID, IntoID: h.two.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.links.links) != 1 {
		t.Fatalf("links %+v, want one", h.links.links)
	}
	link := h.links.links[0]
	if link.Type != domain.LinkTypeDuplicates || link.SourceID != h.one.ID || link.TargetID != h.two.ID || result.LinkID != link.ID {
		t.Errorf("the link %+v, the answer %+v", link, result)
	}
	if !h.items.stored[h.one.ID].IsArchived() || h.items.stored[h.two.ID].IsArchived() {
		t.Error("the merged entry is not archived, or the other one is")
	}
	if last := h.audit.entries[len(h.audit.entries)-1]; last.Action != ItemMergedAction || last.TargetID != h.one.ID {
		t.Errorf("the last audit entry %+v, want the merge", last)
	}
	if len(h.items.stored) != 3 {
		t.Errorf("%d entries stored, want the three there were", len(h.items.stored))
	}
	asked := map[shared.ID]bool{}
	for _, request := range h.authorizer.requests {
		if request.Permission == "WRITE_ITEMS" {
			asked[request.On.ID] = true
		}
	}
	if !asked[h.one.ID] || !asked[h.two.ID] {
		t.Errorf("the writer was asked about %v, want both entries", asked)
	}
}

// A merge that stopped after its link is finished by repeating it, and a merge repeated after it
// was done answers the same, without a second link.
func TestARepeatedMergeFinishesItAndDrawsNoSecondLink(t *testing.T) {
	h := newLinkHarness()
	link, err := LinkEntries{Linking: h.linking()}.Execute(context.Background(), linkActor(), LinkEntriesCommand{
		SourceID: h.one.ID, TargetID: h.two.ID, Type: domain.LinkTypeDuplicates,
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		result, err := h.merger().Execute(context.Background(), linkActor(), MergeWorkItemsCommand{ItemID: h.one.ID, IntoID: h.two.ID})
		if err != nil {
			t.Fatal(err)
		}
		if result.LinkID != link.ID {
			t.Errorf("answered link %s, want the one drawn before", result.LinkID)
		}
	}
	if len(h.links.links) != 1 || !h.items.stored[h.one.ID].IsArchived() {
		t.Errorf("%d links, archived %v", len(h.links.links), h.items.stored[h.one.ID].IsArchived())
	}
}

func TestAMergeIsRefusedIntoItselfIntoAClosedEntryAndBesideAnotherDuplicate(t *testing.T) {
	h := newLinkHarness()
	ctx := context.Background()
	if _, err := h.merger().Execute(ctx, linkActor(), MergeWorkItemsCommand{ItemID: h.one.ID, IntoID: h.one.ID}); detail(err) != "items.merge_self" {
		t.Errorf("a merge into itself answered %v", err)
	}

	archived := h.items.stored[h.three.ID]
	stamp := now
	archived.ArchivedAt = &stamp
	h.items.stored[h.three.ID] = archived
	if _, err := h.merger().Execute(ctx, linkActor(), MergeWorkItemsCommand{ItemID: h.one.ID, IntoID: h.three.ID}); !errors.Is(err, shared.ErrConflict) || detail(err) != "items.merge_target_closed" {
		t.Errorf("a merge into an archived entry answered %v", err)
	}

	archived.ArchivedAt = nil
	h.items.stored[h.three.ID] = archived
	if _, err := (LinkEntries{Linking: h.linking()}).Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.one.ID, TargetID: h.three.ID, Type: domain.LinkTypeDuplicates}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.merger().Execute(ctx, linkActor(), MergeWorkItemsCommand{ItemID: h.one.ID, IntoID: h.two.ID}); detail(err) != "links.cardinality" {
		t.Errorf("a merge of an entry already a duplicate of a third answered %v", err)
	}
	if h.items.stored[h.one.ID].IsArchived() {
		t.Error("a refused merge archived the entry")
	}
}

// UC-SVC-03 check 6: a split makes a new entry with the title and notes given, made for the
// entry's requester, and links it from the entry; a link that would be refused is refused before
// anything is copied.
func TestASplitIsMadeForTheSameRequesterAndLinkedFromTheEntry(t *testing.T) {
	d := newDuplicateHarness(t)
	original := d.withTask()
	requester := colleagueAccountID
	original.CreatedBy = requester
	d.items.stored[original.ID] = original
	links := &linkStore{items: d.items}
	split := SplitWorkItem{Duplicate: d.handler, Link: LinkEntries{Linking: Linking{
		Items: d.items, Containers: d.containers, Types: &linkTypeStore{}, Links: links, References: links,
		Authorizer: &authorizer{}, Permits: linkReader{}, Events: d.events, Audit: d.audit,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{},
		Activity: ActivityJournal{Entries: d.history, IDs: &ids{}},
	}}}
	notes := "  The printer on the second floor  "

	result, err := split.Execute(t.Context(), linkActor(), SplitWorkItemCommand{
		ItemID: original.ID, Title: " The printer ", Notes: &notes, LinkType: domain.LinkTypeAffects,
	})
	if err != nil {
		t.Fatal(err)
	}
	made := result.Item
	if made.Title != "The printer" || made.Notes != "The printer on the second floor" || made.CreatedBy != requester {
		t.Errorf("the split entry %q / %q by %s, want it trimmed and made for the requester", made.Title, made.Notes, made.CreatedBy)
	}
	if len(links.links) != 1 || links.links[0].SourceID != original.ID || links.links[0].TargetID != made.ID {
		t.Errorf("links %+v, want one from the entry to the new one", links.links)
	}

	before := len(d.items.copies)
	if _, err := split.Execute(t.Context(), linkActor(), SplitWorkItemCommand{ItemID: original.ID, Title: "More", LinkType: "invented"}); detail(err) != "links.type_unknown" {
		t.Errorf("a split with an unknown link type answered %v", err)
	}
	if _, err := split.Execute(t.Context(), linkActor(), SplitWorkItemCommand{ItemID: original.ID, Title: " "}); detail(err) != "items.title_empty" {
		t.Errorf("a split without a title answered %v", err)
	}
	if len(d.items.copies) != before {
		t.Error("a refused split copied the entry all the same")
	}
}
