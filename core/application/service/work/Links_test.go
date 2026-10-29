// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"
	"testing"

	knowledgerepo "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// linkTypeStore is the link type registry double.
type linkTypeStore struct {
	rows  []domain.LinkType
	inUse map[string]bool
}

func (s *linkTypeStore) List(context.Context) ([]domain.LinkType, error) { return s.rows, nil }
func (s *linkTypeStore) Find(_ context.Context, key string) (domain.LinkType, error) {
	for _, row := range s.rows {
		if row.Key == key {
			return row, nil
		}
	}
	return domain.LinkType{}, shared.ErrNotFound.WithDetail("link_types.not_found")
}
func (s *linkTypeStore) Insert(_ context.Context, t domain.LinkType) error {
	s.rows = append(s.rows, t)
	return nil
}
func (s *linkTypeStore) Update(_ context.Context, t domain.LinkType, _ int) error {
	for i, row := range s.rows {
		if row.Key == t.Key {
			s.rows[i] = t
		}
	}
	return nil
}
func (s *linkTypeStore) Delete(_ context.Context, key string) error {
	s.rows = slices.DeleteFunc(s.rows, func(t domain.LinkType) bool { return t.Key == key })
	return nil
}
func (s *linkTypeStore) InUse(_ context.Context, key string) (bool, error) { return s.inUse[key], nil }

// linkStore is the link and reference double: the links, a walk over them, and summaries built
// from the item double.
type linkStore struct {
	links     []domain.EntryLink
	items     *items
	backlinks []repository.Backlink
	locked    int
}

func (s *linkStore) Lock(context.Context) error { s.locked++; return nil }
func (s *linkStore) Insert(_ context.Context, link domain.EntryLink) error {
	for _, l := range s.links {
		if l.Type == link.Type && l.SourceID == link.SourceID && l.TargetID == link.TargetID {
			return shared.ErrConflict.WithDetail("links.exists")
		}
	}
	s.links = append(s.links, link)
	return nil
}
func (s *linkStore) Find(_ context.Context, id shared.ID) (domain.EntryLink, error) {
	for _, l := range s.links {
		if l.ID == id {
			return l, nil
		}
	}
	return domain.EntryLink{}, shared.ErrNotFound.WithDetail("links.not_found")
}
func (s *linkStore) Delete(_ context.Context, id shared.ID) error {
	s.links = slices.DeleteFunc(s.links, func(l domain.EntryLink) bool { return l.ID == id })
	return nil
}
func (s *linkStore) Of(_ context.Context, entry shared.ID, _ int) ([]domain.EntryLink, error) {
	var out []domain.EntryLink
	for _, l := range s.links {
		if l.SourceID == entry || l.TargetID == entry {
			out = append(out, l)
		}
	}
	return out, nil
}
func (s *linkStore) Touching(_ context.Context, ids []shared.ID, _ int) ([]domain.EntryLink, error) {
	var out []domain.EntryLink
	for _, l := range s.links {
		if slices.Contains(ids, l.SourceID) || slices.Contains(ids, l.TargetID) {
			out = append(out, l)
		}
	}
	return out, nil
}
func (s *linkStore) CountFrom(_ context.Context, source shared.ID, linkType string) (int, error) {
	count := 0
	for _, l := range s.links {
		if l.SourceID == source && l.Type == linkType {
			count++
		}
	}
	return count, nil
}
func (s *linkStore) PathExists(_ context.Context, from, to shared.ID, forward, backward []string) (bool, error) {
	seen := map[shared.ID]bool{from: true}
	queue := []shared.ID{from}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		if at == to {
			return true, nil
		}
		for _, l := range s.links {
			var next shared.ID
			switch {
			case slices.Contains(forward, l.Type) && l.SourceID == at:
				next = l.TargetID
			case slices.Contains(backward, l.Type) && l.TargetID == at:
				next = l.SourceID
			default:
				continue
			}
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false, nil
}
func (s *linkStore) Entries(_ context.Context, ids []shared.ID) ([]repository.EntrySummary, error) {
	var out []repository.EntrySummary
	for _, id := range ids {
		if item, ok := s.items.stored[id]; ok {
			out = append(out, repository.EntrySummary{
				ID: item.ID, CollectionID: item.CollectionID, HubID: hubID, Handle: "shop",
				Level: item.Type, Title: item.Title, Number: 7,
			})
		}
	}
	return out, nil
}
func (s *linkStore) Containers(context.Context, []shared.ID) ([]repository.ContainerSummary, error) {
	return nil, nil
}
func (s *linkStore) Backlinks(context.Context, reference.Reference, int) ([]repository.Backlink, error) {
	return s.backlinks, nil
}

// linkReader answers what the reader may see and write: everything but the hidden entries, and
// writes nowhere listed as read-only.
type linkReader struct {
	hidden   map[shared.ID]bool
	readOnly map[shared.ID]bool
}

func (r linkReader) Permitted(_ context.Context, _ appshared.ActorContext, _ access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, path := range paths {
		out[i] = !r.hidden[path[len(path)-1].ID]
	}
	return out, nil
}
func (r linkReader) Permits(_ context.Context, _ appshared.ActorContext, request access.Request) (bool, error) {
	if r.hidden[request.On.ID] {
		return false, nil
	}
	return request.Permission != "WRITE_ITEMS" || !r.readOnly[request.On.ID], nil
}

type linkHarness struct {
	*itemHarness
	types  *linkTypeStore
	links  *linkStore
	reader linkReader
	one    domain.WorkItem
	two    domain.WorkItem
	three  domain.WorkItem
}

func newLinkHarness() *linkHarness {
	h := &linkHarness{itemHarness: newItemHarness(), types: &linkTypeStore{inUse: map[string]bool{}},
		reader: linkReader{hidden: map[shared.ID]bool{}, readOnly: map[shared.ID]bool{}}}
	h.links = &linkStore{items: h.items}
	task := func(id, title string) domain.WorkItem {
		item := domain.WorkItem{
			ID: shared.MustParseID(id), TenantID: tenantID, CollectionID: collectionID,
			Type: domain.ItemTask, Title: title, Version: 1,
		}
		h.items.stored[item.ID] = item
		return item
	}
	h.one = task("0192f000-0000-7000-8000-0000000004a1", "Design")
	h.two = task("0192f000-0000-7000-8000-0000000004a2", "Build")
	h.three = task("0192f000-0000-7000-8000-0000000004a3", "Secret launch")
	return h
}

func (h *linkHarness) linking() Linking {
	return Linking{
		Items: h.items, Containers: h.containers, Types: h.types, Links: h.links, References: h.links,
		Authorizer: h.authorizer, Reader: h.reader, Permits: h.reader, Events: h.events, Audit: h.audit,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{},
		Activity: ActivityJournal{Entries: h.history, IDs: &ids{}},
	}
}

func linkActor() appshared.ActorContext {
	a := actor()
	a.Scopes = []string{"items:read", "items:write", "workspace:manage"}
	return a
}

func detail(err error) string {
	var typed *shared.Error
	if errors.As(err, &typed) {
		return typed.DetailCode
	}
	return ""
}

// UC-PLN-01 checks 1 and 8: a link is drawn from an entry the writer changes to one it sees,
// announced by reference, and recorded; each end reads it from its own side.
func TestALinkIsDrawnAnnouncedAndReadFromBothEnds(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	draw := LinkEntries{Linking: h.linking()}

	link, err := draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.one.ID, TargetID: h.two.ID, Type: domain.LinkTypeDependsOn})
	if err != nil {
		t.Fatal(err)
	}
	if link.SourceID != h.one.ID || link.TargetID != h.two.ID || h.links.locked != 1 {
		t.Errorf("the link %+v (locked %d)", link, h.links.locked)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ItemLinked {
		t.Errorf("events %+v", h.events.appended)
	}
	if last := h.audit.entries[len(h.audit.entries)-1]; last.Action != ItemLinkedAction {
		t.Errorf("audit %+v", last)
	}
	// Both ends' history says a link was drawn, by its type only.
	if len(h.history.entries) != 2 || h.history.entries[0].ItemID != h.one.ID || h.history.entries[1].ItemID != h.two.ID {
		t.Fatalf("history %+v", h.history.entries)
	}
	for _, step := range h.history.entries {
		if step.Verb != activity.ItemLinked {
			t.Errorf("the history says %s", step.Verb)
		}
	}
	write := h.authorizer.requests[len(h.authorizer.requests)-1]
	if write.Permission != "WRITE_ITEMS" || write.On.ID != h.one.ID {
		t.Errorf("the writer was asked %+v", write)
	}

	list := ListEntryLinks{Linking: h.linking()}
	fromTarget, err := list.Execute(ctx, linkActor(), h.two.ID)
	if err != nil {
		t.Fatal(err)
	}
	read := fromTarget.Output(h.two.ID)["links"].([]usecase.Output)
	if len(read) != 1 || read[0]["outward"] != false || read[0]["waits"] != "SOURCE" {
		t.Fatalf("the target reads %+v", read)
	}
	if other := read[0]["other"].(usecase.Output); other["available"] != true || other["title"] != "Design" || other["key"] != "shop-7" {
		t.Errorf("the far end %+v", other)
	}
}

// UC-PLN-01 check 2: a loop of dependencies is refused, over depends_on and blocks together; a
// type that allows loops accepts one.
func TestALoopOfDependenciesIsRefused(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	draw := LinkEntries{Linking: h.linking()}
	mustDraw := func(source, target shared.ID, linkType string) {
		t.Helper()
		if _, err := draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: source, TargetID: target, Type: linkType}); err != nil {
			t.Fatalf("%s: %v", linkType, err)
		}
	}
	mustDraw(h.one.ID, h.two.ID, domain.LinkTypeDependsOn) // one waits for two
	mustDraw(h.two.ID, h.three.ID, domain.LinkTypeDependsOn)

	// one blocks three: three waits for one, which waits for two, which waits for three.
	_, err := draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.one.ID, TargetID: h.three.ID, Type: domain.LinkTypeBlocks})
	if detail(err) != "links.cycle" {
		t.Errorf("a loop over blocks and depends_on was drawn: %v", err)
	}
	_, err = draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.three.ID, TargetID: h.one.ID, Type: domain.LinkTypeDependsOn})
	if detail(err) != "links.cycle" {
		t.Errorf("a loop of depends_on was drawn: %v", err)
	}
	// Affects allows loops.
	mustDraw(h.one.ID, h.two.ID, domain.LinkTypeAffects)
	mustDraw(h.two.ID, h.one.ID, domain.LinkTypeAffects)
}

// UC-PLN-01 check 3: an entry duplicates at most one other.
func TestAnEntryDuplicatesAtMostOneOther(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	draw := LinkEntries{Linking: h.linking()}
	if _, err := draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.one.ID, TargetID: h.two.ID, Type: domain.LinkTypeDuplicates}); err != nil {
		t.Fatal(err)
	}
	_, err := draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.one.ID, TargetID: h.three.ID, Type: domain.LinkTypeDuplicates})
	if detail(err) != "links.cardinality" {
		t.Errorf("a second duplicates link was drawn: %v", err)
	}
	_, err = draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.one.ID, TargetID: h.two.ID, Type: "invented"})
	if detail(err) != "links.type_unknown" {
		t.Errorf("an unknown type drew a link: %v", err)
	}
}

// UC-PLN-01 check 4: nobody links to what they cannot see - answered as a missing entry - and a
// link whose far end the reader may not see shows only its address, never the title.
func TestALinkNeverShowsWhatTheReaderMayNotSee(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	draw := LinkEntries{Linking: h.linking()}
	if _, err := draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.one.ID, TargetID: h.three.ID, Type: domain.LinkTypeBlocks}); err != nil {
		t.Fatal(err)
	}

	h.reader.hidden[h.three.ID] = true
	_, err := LinkEntries{Linking: h.linking()}.Execute(ctx, linkActor(),
		LinkEntriesCommand{SourceID: h.two.ID, TargetID: h.three.ID, Type: domain.LinkTypeBlocks})
	if !errors.Is(err, shared.ErrNotFound) || detail(err) != "items.not_found" {
		t.Errorf("a link to an entry out of sight: %v", err)
	}

	links, err := ListEntryLinks{Linking: h.linking()}.Execute(ctx, linkActor(), h.one.ID)
	if err != nil {
		t.Fatal(err)
	}
	read := links.Output(h.one.ID)["links"].([]usecase.Output)
	other := read[0]["other"].(usecase.Output)
	if other["available"] != false || len(other) != 4 {
		t.Errorf("the hidden end shows %+v", other)
	}
	if _, leaked := other["title"]; leaked {
		t.Error("the hidden end's title leaked")
	}

	resolved, err := ResolveReferences{Linking: h.linking()}.Execute(ctx, linkActor(), []reference.Reference{
		{Kind: reference.KindEntry, ID: h.two.ID, Anchor: "plan"},
		{Kind: reference.KindEntry, ID: h.three.ID},
		{Kind: reference.KindEntry, ID: shared.MustParseID("0192f000-0000-7000-8000-0000000004ff")},
		{Kind: reference.KindPage, ID: h.one.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0]["available"] != true || resolved[0]["title"] != "Build" || resolved[0]["anchor"] != "plan" {
		t.Errorf("a visible entry resolves to %+v", resolved[0])
	}
	for i := 1; i < 4; i++ {
		if resolved[i]["available"] != false || len(resolved[i]) != 4 {
			t.Errorf("reference %d resolves to %+v", i, resolved[i])
		}
	}
	// A hidden entry and a missing one answer the same (T-04).
	if resolved[1]["kind"] != resolved[2]["kind"] || len(resolved[1]) != len(resolved[2]) {
		t.Error("a hidden and a missing entry can be told apart")
	}
}

// UC-PLN-01 check 7: a text pointing at the entry is a backlink for readers who see its entry,
// and left out for the others.
func TestABacklinkShowsOnlyFromWhatTheReaderSees(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	h.links.backlinks = []repository.Backlink{
		{SourceKind: "ENTRY", SourceID: h.one.ID, EntryID: h.one.ID, Strength: reference.Card},
		{SourceKind: "COMMENT", SourceID: shared.MustParseID("0192f000-0000-7000-8000-0000000004c1"), EntryID: h.three.ID, Strength: reference.Mention},
	}
	h.reader.hidden[h.three.ID] = true
	links, err := ListEntryLinks{Linking: h.linking()}.Execute(ctx, linkActor(), h.two.ID)
	if err != nil {
		t.Fatal(err)
	}
	backlinks := links.Output(h.two.ID)["backlinks"].([]usecase.Output)
	if len(backlinks) != 1 || backlinks[0]["strength"] != "CARD" || backlinks[0]["entry"].(usecase.Output)["title"] != "Design" {
		t.Errorf("backlinks %+v", backlinks)
	}
}

// UC-PLN-01 checks 1 and 6: either end's writer removes a link; the timeline reads who waits,
// and leaves out a link with an end out of sight.
func TestTheTimelineReadsWhoWaitsAndEitherEndRemovesALink(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	draw := LinkEntries{Linking: h.linking()}
	first, err := draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.one.ID, TargetID: h.two.ID, Type: domain.LinkTypeBlocks})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := draw.Execute(ctx, linkActor(), LinkEntriesCommand{SourceID: h.two.ID, TargetID: h.three.ID, Type: domain.LinkTypeDependsOn}); err != nil {
		t.Fatal(err)
	}
	h.reader.hidden[h.three.ID] = true
	among, err := ListLinksAmong{Linking: h.linking()}.Execute(ctx, linkActor(), []shared.ID{h.one.ID, h.two.ID, h.three.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(among) != 1 || among[0].Link.ID != first.ID || among[0].Waits != domain.LinkWaitsTarget {
		t.Errorf("the timeline reads %+v", among)
	}

	h.reader.hidden[h.three.ID] = false
	h.reader.readOnly[h.one.ID] = true
	if err := (UnlinkEntries{Linking: h.linking()}).Execute(ctx, linkActor(), first.ID); err != nil {
		t.Fatalf("the target's writer could not remove the link: %v", err)
	}
	if len(h.links.links) != 1 || h.events.appended[len(h.events.appended)-1].Type != event.ItemUnlinked {
		t.Errorf("links %+v", h.links.links)
	}
}

// UC-PLN-01 check 5: a workspace's own type is created, archived instead of deleted while in use,
// and the shipped ones stay as they are.
func TestAWorkspacesOwnLinkTypes(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	registry := LinkTypeRegistry{
		Types: h.types, Authorizer: h.authorizer, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), IDs: &ids{}, Text: text.Composing{},
	}
	created, err := CreateLinkType{Registry: registry}.Execute(ctx, linkActor(), CreateLinkTypeCommand{
		Key: "verifies", Name: "verifies", InverseName: "is verified by",
	})
	if err != nil || created.Version != 1 {
		t.Fatalf("created %+v, %v", created, err)
	}
	if write := h.authorizer.requests[len(h.authorizer.requests)-1]; write.Permission != "STRUCTURE" {
		t.Errorf("creating asked for %s", write.Permission)
	}
	all, err := ListLinkTypes{Registry: registry}.Execute(ctx, linkActor())
	if err != nil || len(all) != 7 {
		t.Fatalf("the registry %v, %v", all, err)
	}
	h.types.inUse["verifies"] = true
	if err := (DeleteLinkType{Registry: registry}).Execute(ctx, linkActor(), "verifies"); detail(err) != "link_types.in_use" {
		t.Errorf("a type in use was deleted: %v", err)
	}
	archive := true
	archived, err := ChangeLinkType{Registry: registry}.Execute(ctx, linkActor(), ChangeLinkTypeCommand{Key: "verifies", Archived: &archive})
	if err != nil || !archived.IsArchived() || archived.Version != 2 {
		t.Errorf("archived %+v, %v", archived, err)
	}
	_, err = LinkEntries{Linking: h.linking()}.Execute(ctx, linkActor(),
		LinkEntriesCommand{SourceID: h.one.ID, TargetID: h.two.ID, Type: "verifies"})
	if detail(err) != "links.type_archived" {
		t.Errorf("an archived type drew a link: %v", err)
	}
	if err := (DeleteLinkType{Registry: registry}).Execute(ctx, linkActor(), domain.LinkTypeBlocks); detail(err) != "link_types.system_fixed" {
		t.Errorf("a shipped type was deleted: %v", err)
	}
}

// An offline device draws a link under the identifier it minted, so that its later removal of
// that link names the same one; an identifier that is not a UUIDv7 is refused.
func TestALinkKeepsTheIdentifierItsDeviceMinted(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	draw := LinkEntries{Linking: h.linking()}
	minted := shared.ID("01936f2a-7c1e-7000-8000-0000000011a1")

	link, err := draw.Execute(ctx, linkActor(), LinkEntriesCommand{
		ID: minted, SourceID: h.one.ID, TargetID: h.two.ID, Type: domain.LinkTypeDependsOn,
	})
	if err != nil {
		t.Fatal(err)
	}
	if link.ID != minted {
		t.Errorf("the link is %s, want the device's %s", link.ID, minted)
	}

	_, err = draw.Execute(ctx, linkActor(), LinkEntriesCommand{
		ID: shared.ID("6f1c2a52-1b7e-4c47-9a0e-3a0f2f6c9d11"), SourceID: h.two.ID, TargetID: h.one.ID,
		Type: domain.LinkTypeDependsOn,
	})
	if shared.AsError(err).DetailCode != "sync.id_not_uuidv7" {
		t.Errorf("a version 4 identifier answered %v", err)
	}
}

type pageSummaries []knowledgerepo.Summary

func (p pageSummaries) Summaries(_ context.Context, ids []shared.ID) ([]knowledgerepo.Summary, error) {
	var out []knowledgerepo.Summary
	for _, s := range p {
		if slices.Contains(ids, s.ID) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (p pageSummaries) Backlinks(context.Context, string, shared.ID, int) ([]knowledgerepo.Backlink, error) {
	return nil, nil
}

// pagesPointing are pages whose bodies point at every entry asked about.
type pagesPointing struct {
	pageSummaries
	backlinks []knowledgerepo.Backlink
}

func (p pagesPointing) Backlinks(_ context.Context, kind string, _ shared.ID, _ int) ([]knowledgerepo.Backlink, error) {
	if kind != string(reference.KindEntry) {
		return nil, nil
	}
	return p.backlinks, nil
}

// P-19: a page resolves to its title for a reader of its hub; a page in a hub the reader may not
// read answers its address alone, as a missing one does.
func TestAPageReferenceResolvesForAReaderOfItsHub(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	shared1 := shared.MustParseID("0192f000-0000-7000-8000-0000000005a1")
	closed := shared.MustParseID("0192f000-0000-7000-8000-0000000005a2")
	page := shared.MustParseID("0192f000-0000-7000-8000-0000000005b1")
	secret := shared.MustParseID("0192f000-0000-7000-8000-0000000005b2")
	h.reader.hidden[closed] = true
	linking := h.linking()
	linking.Pages = pageSummaries{
		{ID: page, HubID: shared1, Title: "Handbook", Icon: "📘"},
		{ID: secret, HubID: closed, Title: "Salaries"},
	}
	resolved, err := ResolveReferences{Linking: linking}.Execute(ctx, linkActor(), []reference.Reference{
		{Kind: reference.KindPage, ID: page}, {Kind: reference.KindPage, ID: secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0]["available"] != true || resolved[0]["title"] != "Handbook" || resolved[0]["hub_id"] != shared1.String() {
		t.Errorf("a readable page resolves to %+v", resolved[0])
	}
	if resolved[1]["available"] != false || len(resolved[1]) != 4 {
		t.Errorf("a page out of reach resolves to %+v", resolved[1])
	}
}

// An entry's backlinks name the pages pointing at it, the strongest drawing winning, and only the
// pages of a hub the reader may read (UC-DOC-04 check 4; P-05).
func TestAnEntrysBacklinksNameThePagesTheReaderMayRead(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	item := h.one.ID
	open := shared.MustParseID("0192f000-0000-7000-8000-0000000005c1")
	closed := shared.MustParseID("0192f000-0000-7000-8000-0000000005c2")
	handbook := shared.MustParseID("0192f000-0000-7000-8000-0000000005d1")
	salaries := shared.MustParseID("0192f000-0000-7000-8000-0000000005d2")
	h.reader.hidden[closed] = true
	linking := h.linking()
	linking.Pages = pagesPointing{
		pageSummaries: pageSummaries{{ID: handbook, HubID: open, Title: "Handbook"}, {ID: salaries, HubID: closed, Title: "Salaries"}},
		backlinks: []knowledgerepo.Backlink{
			{SourceKind: "PAGE", SourceID: handbook, HubID: open, Strength: "MENTION"},
			{SourceKind: "PAGE", SourceID: handbook, HubID: open, Strength: "CARD"},
			{SourceKind: "PAGE", SourceID: salaries, HubID: closed, Strength: "MENTION"},
			{SourceKind: "ENTRY", SourceID: item, EntryID: item, Strength: "MENTION"},
		},
	}
	links, err := ListEntryLinks{Linking: linking}.Execute(ctx, linkActor(), item)
	if err != nil {
		t.Fatal(err)
	}
	if len(links.PageBacklinks) != 1 || links.PageBacklinks[0].Page.ID != handbook || links.PageBacklinks[0].Strength != reference.Card {
		t.Fatalf("page backlinks %+v", links.PageBacklinks)
	}
	out := links.Output(item)["page_backlinks"].([]usecase.Output)
	if page := out[0]["page"].(usecase.Output); page["title"] != "Handbook" || page["available"] != true {
		t.Errorf("a page backlink answers %+v", out[0])
	}
}
