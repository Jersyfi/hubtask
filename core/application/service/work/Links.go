// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	knowledgerepo "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	viewrepo "github.com/Jersyfi/hubtask/core/application/repository/view"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	LinkEntriesName       = "LinkEntries"
	UnlinkEntriesName     = "UnlinkEntries"
	ListEntryLinksName    = "ListEntryLinks"
	ListLinksAmongName    = "ListLinksAmong"
	ResolveReferencesName = "ResolveReferences"

	linkTarget = "entry_link"

	// Stable audit codes (audit.md §2).
	ItemLinkedAction        audit.Action = "item.linked"
	ItemUnlinkedAction      audit.Action = "item.unlinked"
	ReferencesResolveAction audit.Action = "reference.resolved"

	// The bounds of one answer: an entry's links, its backlinks, the entries a timeline asks
	// about, the references one call resolves.
	maxLinksPerEntry   = 500
	maxBacklinks       = 200
	maxEntriesAmong    = 500
	maxLinksAmong      = 2000
	maxReferencesAsked = 100
)

// Linking is what every link and reference use case shares (F-3, P-19, UC-PLN-01).
type Linking struct {
	Items      repository.Items
	Containers repository.Containers
	Types      repository.LinkTypes
	Links      repository.EntryLinks
	References repository.References
	Views      viewrepo.SavedViews
	// Pages resolves a reference to a page (M3); nil resolves none, until the module is wired.
	Pages PageSummaries
	// Boards resolves a reference to a whiteboard and finds the boards an entry is drawn on (M4);
	// nil resolves none.
	Boards BoardSummaries
	// Cis resolves a reference to a configuration item (M6); nil resolves none.
	Cis CiSummaries
	// Goals resolves a reference to a goal or a dashboard (M8); nil resolves none.
	Goals GoalSummaries
	// Groups answers whether a reader is a member of a group a dashboard is shared with; nil
	// resolves a GROUPS dashboard for its owner alone.
	Groups     GroupMembers
	Authorizer Authorizer
	// Reader filters a page by what the reader may see, and Permits answers one question, both
	// without writing a refusal: a reference out of reach is not somebody trying anything.
	Reader     Reader
	Permits    Permitting
	Events     outbox.Events
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Changes and HLC tell offline devices of a drawn or removed link. A nil Changes records
	// nothing, a test's convenience.
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
	// Activity writes the link into the history of both its ends.
	Activity ActivityJournal
}

// PageSummaries answers the pages among identifiers that are not trashed, and the texts that
// point at an object - of which an entry's links keep the pages.
type PageSummaries interface {
	Summaries(ctx context.Context, ids []shared.ID) ([]knowledgerepo.Summary, error)
	Backlinks(ctx context.Context, targetKind string, targetID shared.ID, limit int) ([]knowledgerepo.Backlink, error)
}

// summaryPath is the scope chain an entry's summary is judged against, ending on the entry so
// that an entry shared with the reader alone is visible.
func summaryPath(s repository.EntrySummary) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !s.HubID.IsZero() {
		path = append(path, identity.HubScope(s.HubID))
	}
	return append(path, identity.CollectionScope(s.CollectionID), identity.ItemScope(s.ID))
}

// visibleEntries answers the summaries of the entries among ids that the reader may see. What is
// missing, trashed or out of reach is simply absent: the three are one answer (T-04).
func (l Linking) visibleEntries(
	ctx context.Context, actor appshared.ActorContext, ids []shared.ID,
) (map[shared.ID]repository.EntrySummary, error) {
	visible := map[shared.ID]repository.EntrySummary{}
	if len(ids) == 0 {
		return visible, nil
	}
	var summaries []repository.EntrySummary
	err := l.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := l.References.Entries(ctx, ids)
		summaries = read
		return err
	})
	if err != nil || len(summaries) == 0 {
		return visible, err
	}
	paths := make([][]identity.Scope, 0, len(summaries))
	for _, s := range summaries {
		paths = append(paths, summaryPath(s))
	}
	// Each path ends at its entry (summaryPath), so a share at the entry counts.
	allowed, err := l.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: ItemReadAction, TokenScope: itemsRead, TargetType: itemTarget,
		On: access.ItemSubject{Does: service.ItemRead},
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, s := range summaries {
		if i < len(allowed) && allowed[i] {
			visible[s.ID] = s
		}
	}
	return visible, nil
}

// EntrySummaryOutput is an entry as a mention or a card shows it (schema ReferenceTarget).
func EntrySummaryOutput(s repository.EntrySummary, anchor string) usecase.Output {
	entryType := s.EntryType
	if entryType == "" {
		entryType = domain.SystemEntryTypeKey(s.Level)
	}
	out := usecase.Output{
		"kind": string(reference.KindEntry), "id": s.ID.String(), "anchor": anchorOrNil(anchor),
		"available": true, "title": s.Title, "level": string(s.Level), "entry_type": entryType,
		"completed": s.Completed, "archived": s.Archived, "collection_id": s.CollectionID.String(),
		"number": nil, "key": nil, "start_at": timeOrNil(s.StartAt), "due_at": timeOrNil(s.DueAt),
	}
	if s.Number > 0 {
		out["number"] = s.Number
		if s.Handle != "" {
			out["key"] = s.Handle + "-" + strconv.FormatInt(s.Number, 10)
		}
	}
	return out
}

// unavailable is what a reader is shown of an object they may not see, or that is not there: its
// address and nothing else (P-19, UC-PLN-01 check 4).
func unavailable(ref reference.Reference) usecase.Output {
	return usecase.Output{
		"kind": string(ref.Kind), "id": ref.ID.String(), "anchor": anchorOrNil(ref.Anchor), "available": false,
	}
}

func anchorOrNil(anchor string) any {
	if anchor == "" {
		return nil
	}
	return anchor
}

// LinkOutput is a link as one of its ends reads it (schema EntryLink).
func LinkOutput(link domain.EntryLink) usecase.Output {
	kind := link.Timing.Kind
	if kind == "" {
		kind = planning.FinishToStart
	}
	return usecase.Output{
		"id": link.ID.String(), "type": link.Type, "source_id": link.SourceID.String(),
		"target_id": link.TargetID.String(), "created_at": link.CreatedAt,
		"dependency_kind": string(kind), "lag_days": link.Timing.LagDays,
	}
}

// LinkTargetCollectionKey names, in a link's change record, the collection of its target: the
// second place a reader must reach to be told of the link.
const LinkTargetCollectionKey = "target_collection_id"

// LinkRecord is a link as the change log and the replica carry it: the link and the collection of
// each end.
func LinkRecord(link domain.EntryLink, sourceCollection, targetCollection shared.ID) map[string]any {
	out := map[string]any(LinkOutput(link))
	out["source_collection_id"] = sourceCollection.String()
	out[LinkTargetCollectionKey] = targetCollection.String()
	return out
}

// LinkEntriesCommand is the input, typed.
type LinkEntriesCommand struct {
	// ID is the identifier the caller minted (an offline device's, a UUIDv7); zero mints one.
	ID       shared.ID
	SourceID shared.ID
	TargetID shared.ID
	Type     string
}

// LinkEntries draws a link of a type from one entry to another (UC-PLN-01 checks 1-3). The writer
// changes the source, so it needs WRITE_ITEMS there, and it points at the target, so it needs to
// see it: a target out of reach is not found, as a missing one is (T-04).
type LinkEntries struct{ Linking Linking }

// Execute draws the link.
func (h LinkEntries) Execute(ctx context.Context, actor appshared.ActorContext, cmd LinkEntriesCommand) (domain.EntryLink, error) {
	if cmd.SourceID.IsZero() {
		return domain.EntryLink{}, itemIDRequired()
	}
	if cmd.TargetID.IsZero() {
		return domain.EntryLink{}, linkFieldInvalid("/target_id", "links.target_required")
	}
	if !cmd.ID.IsZero() && !cmd.ID.IsUUIDv7() {
		return domain.EntryLink{}, shared.ErrValidation.
			WithDetail("sync.id_not_uuidv7").
			WithFields(shared.FieldError{Path: "/id", Code: "sync.id_not_uuidv7"})
	}
	source, sourceCollection, err := readItemScope(ctx, h.Linking.UnitOfWork, h.Linking.Items, h.Linking.Containers, actor, cmd.SourceID)
	if err != nil {
		return domain.EntryLink{}, err
	}
	if err := h.Linking.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems, Path: containerPath(sourceCollection),
		Action: ItemLinkedAction, TokenScope: itemsWrite, TargetType: itemTarget,
		TargetID: source.ID, On: changing(source),
	}); err != nil {
		return domain.EntryLink{}, err
	}
	target, targetCollection, err := readItemScope(ctx, h.Linking.UnitOfWork, h.Linking.Items, h.Linking.Containers, actor, cmd.TargetID)
	if err != nil {
		return domain.EntryLink{}, err
	}
	sees, err := h.Linking.Permits.Permits(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: containerPath(targetCollection),
		Action: ItemReadAction, TokenScope: itemsRead, TargetType: itemTarget,
		TargetID: target.ID, On: reading(target),
	})
	if err != nil {
		return domain.EntryLink{}, err
	}
	if !sees || target.DeletedAt != nil {
		return domain.EntryLink{}, appshared.ItemNotFound(cmd.TargetID)
	}

	var drawn domain.EntryLink
	err = h.Linking.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := findItem(ctx, h.Linking.Items, cmd.SourceID)
		if err != nil {
			return err
		}
		if err := current.EnsureEditable(); err != nil {
			return err
		}
		if err := h.Linking.Links.Lock(ctx); err != nil {
			return err
		}
		rows, err := h.Linking.Types.List(ctx)
		if err != nil {
			return err
		}
		registry := domain.NewLinkTypes(rows)
		linkType, ok := registry.Find(cmd.Type)
		if !ok {
			return linkFieldInvalid("/type", "links.type_unknown")
		}
		now := h.Linking.Clock.Now()
		id := cmd.ID
		if id.IsZero() {
			id = h.Linking.IDs.NewID()
		}
		link, err := domain.NewEntryLink(domain.NewEntryLinkInput{
			ID: id, TenantID: actor.TenantID, Type: linkType,
			SourceID: current.ID, TargetID: target.ID, CreatedBy: actor.AccountID, Now: now,
		})
		if err != nil {
			return err
		}
		if linkType.Cardinality == domain.LinkOne {
			count, err := h.Linking.Links.CountFrom(ctx, link.SourceID, linkType.Key)
			if err != nil {
				return err
			}
			if count > 0 {
				return shared.ErrConflict.WithDetail("links.cardinality").
					WithParams(map[string]string{"type": linkType.Key})
			}
		}
		if linkType.Cycles == domain.LinkCyclesForbidden {
			from, to, forward, backward := link.ClosesLoopFrom(linkType, registry)
			loops, err := h.Linking.Links.PathExists(ctx, from, to, forward, backward)
			if err != nil {
				return err
			}
			if loops {
				// Which entries the loop runs through is not said: some may be out of the
				// writer's reach.
				return shared.ErrConflict.WithDetail("links.cycle").
					WithParams(map[string]string{"type": linkType.Key})
			}
		}
		if err := h.Linking.Links.Insert(ctx, link); err != nil {
			return err
		}
		drawn = link
		return h.Linking.announce(ctx, actor, link, ItemLinkedAction, now)
	})
	if err != nil {
		return domain.EntryLink{}, err
	}
	return drawn, nil
}

// announce writes what a drawn or removed link owes: the event and the evidence, in the write's
// transaction (test AT-5). The source is read again: its collection names the event.
func (l Linking) announce(
	ctx context.Context, actor appshared.ActorContext, link domain.EntryLink, action audit.Action, now time.Time,
) error {
	source, err := l.Items.Find(ctx, link.SourceID)
	if err != nil {
		return err
	}
	by := event.Actor{Kind: actor.Kind, ID: actor.AccountID}
	build := event.NewItemLinked
	if action == ItemUnlinkedAction {
		build = event.NewItemUnlinked
	}
	announcement, err := build(l.IDs.NewID(), link, source, by, now, event.Cause{})
	if err != nil {
		return err
	}
	if err := l.Events.Append(ctx, announcement); err != nil {
		return err
	}
	if err := l.recordChange(ctx, actor, link, source, action); err != nil {
		return err
	}
	if err := l.recordHistory(ctx, actor, link, source, action, now); err != nil {
		return err
	}
	return l.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: linkTarget, TargetID: link.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "type", Classification: audit.Open, To: link.Type},
			audit.Change{Field: "source_id", Classification: audit.Open, To: link.SourceID.String()},
			audit.Change{Field: "target_id", Classification: audit.Open, To: link.TargetID.String()},
		),
	})
}

// recordHistory writes the link into both ends' history, naming its type and not the other end.
func (l Linking) recordHistory(
	ctx context.Context, actor appshared.ActorContext, link domain.EntryLink, source domain.WorkItem,
	action audit.Action, now time.Time,
) error {
	verb, from, to := activity.ItemLinked, "", link.Type
	if action == ItemUnlinkedAction {
		verb, from, to = activity.ItemUnlinked, link.Type, ""
	}
	target, err := l.Items.Find(ctx, link.TargetID)
	if err != nil {
		return err
	}
	for _, end := range []domain.WorkItem{source, target} {
		if err := l.Activity.record(ctx, actor, end, verb, activity.ChangeSet(activity.Full,
			activity.Field{Name: "link_type", Detail: activity.WithValues, From: from, To: to},
		), now); err != nil {
			return err
		}
	}
	return nil
}

// recordChange files the link for offline devices under its source's collection; a drawn link
// names its target's as well, which a reader must reach too (sync's mayReadOtherEnd).
func (l Linking) recordChange(
	ctx context.Context, actor appshared.ActorContext, link domain.EntryLink, source domain.WorkItem, action audit.Action,
) error {
	if l.Changes == nil {
		return nil
	}
	op, payload := changelog.Delete, map[string]any(nil)
	if action != ItemUnlinkedAction {
		target, err := l.Items.Find(ctx, link.TargetID)
		if err != nil {
			return err
		}
		op, payload = changelog.Upsert, LinkRecord(link, source.CollectionID, target.CollectionID)
	}
	return l.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: linkTarget, EntityID: link.ID, Op: op,
		ContainerID: source.CollectionID, ActorID: actor.AccountID, HLC: l.HLC.Next(), Payload: payload,
	})
}

// Descriptor is the catalogue entry.
func (h LinkEntries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionWriteItems,
		Name:       LinkEntriesName,
		Summary: "Draws a link of a type from an entry (the source) to another (the target): blocks, " +
			"depends_on, duplicates, runs_on, affects, implements, or one of the workspace's own types. " +
			"Needs write access to the source and sight of the target. A link that would close a loop " +
			"of a type that forbids loops is refused with links.cycle - for a dependency, judged over " +
			"every type one end waits on; a second link of a type of one per entry with " +
			"links.cardinality; one already drawn with links.exists.",
		SideEffects: "Writes the link, writes it into the history of both entries, publishes " +
			string(event.ItemLinked) + " and writes an audit entry.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{
				Name: "id", Kind: usecase.KindID, CallerOnly: true,
				Description: "The identifier the caller minted, a UUIDv7; offline clients assign " +
					"their own. Leave it out and the server mints one.",
			},
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The source entry."},
			{Name: "target_id", Kind: usecase.KindID, Required: true, Description: "The target entry."},
			{Name: "type", Kind: usecase.KindString, Required: true, Description: "The link type's key."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemLinkedAction, TargetType: linkTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemLinked},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			source, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			target, err := in.ID("target_id")
			if err != nil {
				return nil, err
			}
			id, err := in.ID("id")
			if err != nil {
				return nil, err
			}
			link, err := h.Execute(ctx, actor, LinkEntriesCommand{ID: id, SourceID: source, TargetID: target, Type: in.String("type")})
			if err != nil {
				return nil, err
			}
			return LinkOutput(link), nil
		}),
	}
}

// UnlinkEntries removes a link. Whoever may change either end may remove it: a link is about both.
type UnlinkEntries struct{ Linking Linking }

// Execute removes the link.
func (h UnlinkEntries) Execute(ctx context.Context, actor appshared.ActorContext, linkID shared.ID) error {
	if linkID.IsZero() {
		return linkFieldInvalid("/link_id", "links.id_required")
	}
	var link domain.EntryLink
	err := h.Linking.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Linking.Links.Find(ctx, linkID)
		link = found
		return err
	})
	if err != nil {
		return err
	}
	source, sourceCollection, err := readItemScope(ctx, h.Linking.UnitOfWork, h.Linking.Items, h.Linking.Containers, actor, link.SourceID)
	if err != nil {
		return err
	}
	writeSource := access.Request{
		Permission: service.PermissionWriteItems, Path: containerPath(sourceCollection),
		Action: ItemUnlinkedAction, TokenScope: itemsWrite, TargetType: itemTarget,
		TargetID: source.ID, On: changing(source),
	}
	may, err := h.Linking.Permits.Permits(ctx, actor, writeSource)
	if err != nil {
		return err
	}
	if !may {
		target, targetCollection, err := readItemScope(ctx, h.Linking.UnitOfWork, h.Linking.Items, h.Linking.Containers, actor, link.TargetID)
		if err != nil {
			return err
		}
		may, err = h.Linking.Permits.Permits(ctx, actor, access.Request{
			Permission: service.PermissionWriteItems, Path: containerPath(targetCollection),
			Action: ItemUnlinkedAction, TokenScope: itemsWrite, TargetType: itemTarget,
			TargetID: target.ID, On: changing(target),
		})
		if err != nil {
			return err
		}
	}
	if !may {
		// Neither end: asked once more so that the refusal is recorded and answered as every
		// refusal is.
		if err := h.Linking.Authorizer.Authorize(ctx, actor, writeSource); err != nil {
			return err
		}
	} else if err := actor.RequireScope(itemsWrite); err != nil {
		return err
	}
	return h.Linking.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Linking.Links.Delete(ctx, linkID); err != nil {
			return err
		}
		return h.Linking.announce(ctx, actor, link, ItemUnlinkedAction, h.Linking.Clock.Now())
	})
}

// Descriptor is the catalogue entry.
func (h UnlinkEntries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       UnlinkEntriesName,
		Permission: service.PermissionWriteItems,
		Summary:    "Removes a link between two entries. Whoever may change either end may remove it.",
		SideEffects: "Deletes the link, writes its removal into the history of both entries, publishes " +
			string(event.ItemUnlinked) + " and writes an audit entry.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "link_id", Kind: usecase.KindID, Required: true, Description: "The link."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemUnlinkedAction, TargetType: linkTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemUnlinked},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("link_id")
			if err != nil {
				return nil, err
			}
			if err := h.Execute(ctx, actor, id); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}
}

// EntryLinks is an entry's links from both ends and the texts that point at it.
type EntryLinks struct {
	Registry domain.LinkTypes
	Links    []domain.EntryLink
	// Others are the far ends the reader may see; a link whose far end is absent shows it as not
	// available.
	Others    map[shared.ID]repository.EntrySummary
	Backlinks []repository.Backlink
	// Sources are the entries the backlinks' texts belong to, all visible to the reader.
	Sources map[shared.ID]repository.EntrySummary
	// PageBacklinks are the pages pointing at the entry, each in a hub the reader may read (M3).
	PageBacklinks []PageBacklink
	// BoardBacklinks are the whiteboards drawing or mentioning the entry, each in a hub the reader
	// may read (M4).
	BoardBacklinks []BoardBacklink
}

// PageBacklink is a page that points at an entry, and how strongly.
type PageBacklink struct {
	Page     knowledgerepo.Summary
	Strength reference.Strength
}

// ListEntryLinks reads an entry's links and backlinks (UC-PLN-01 checks 1, 4 and 7).
type ListEntryLinks struct{ Linking Linking }

// Execute answers the entry's links. A text pointing at it from an entry the reader may not see is
// left out: narrowing hides (P-05).
func (h ListEntryLinks) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) (EntryLinks, error) {
	if itemID.IsZero() {
		return EntryLinks{}, itemIDRequired()
	}
	item, collection, err := readItemScope(ctx, h.Linking.UnitOfWork, h.Linking.Items, h.Linking.Containers, actor, itemID)
	if err != nil {
		return EntryLinks{}, err
	}
	if err := h.Linking.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: containerPath(collection),
		Action: ItemReadAction, TokenScope: itemsRead, TargetType: itemTarget,
		TargetID: item.ID, On: reading(item),
	}); err != nil {
		return EntryLinks{}, err
	}
	var result EntryLinks
	err = h.Linking.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rows, err := h.Linking.Types.List(ctx)
		if err != nil {
			return err
		}
		result.Registry = domain.NewLinkTypes(rows)
		if result.Links, err = h.Linking.Links.Of(ctx, item.ID, maxLinksPerEntry); err != nil {
			return err
		}
		result.Backlinks, err = h.Linking.References.Backlinks(ctx,
			reference.Reference{Kind: reference.KindEntry, ID: item.ID}, maxBacklinks)
		return err
	})
	if err != nil {
		return EntryLinks{}, err
	}
	if result.PageBacklinks, err = h.pageBacklinks(ctx, actor, item.ID); err != nil {
		return EntryLinks{}, err
	}
	if result.BoardBacklinks, err = h.boardBacklinks(ctx, actor, item.ID); err != nil {
		return EntryLinks{}, err
	}
	others := make([]shared.ID, 0, len(result.Links))
	for _, link := range result.Links {
		other, _ := link.Other(item.ID)
		others = append(others, other)
	}
	if result.Others, err = h.Linking.visibleEntries(ctx, actor, others); err != nil {
		return EntryLinks{}, err
	}
	sources := make([]shared.ID, 0, len(result.Backlinks))
	for _, backlink := range result.Backlinks {
		sources = append(sources, backlink.EntryID)
	}
	if result.Sources, err = h.Linking.visibleEntries(ctx, actor, sources); err != nil {
		return EntryLinks{}, err
	}
	kept := make([]repository.Backlink, 0, len(result.Backlinks))
	for _, backlink := range result.Backlinks {
		if _, visible := result.Sources[backlink.EntryID]; visible && backlink.EntryID != item.ID {
			kept = append(kept, backlink)
		}
	}
	result.Backlinks = kept
	return result, nil
}

// pageBacklinks answers the pages that point at the entry and that the reader may read; a page in
// a hub out of reach is left out, as an entry's notes out of reach are (P-05).
func (h ListEntryLinks) pageBacklinks(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) ([]PageBacklink, error) {
	if h.Linking.Pages == nil {
		return nil, nil
	}
	strengths := map[shared.ID]reference.Strength{}
	var ids []shared.ID
	err := h.Linking.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rows, err := h.Linking.Pages.Backlinks(ctx, string(reference.KindEntry), itemID, maxBacklinks)
		for _, row := range rows {
			if row.SourceKind != string(reference.KindPage) {
				continue
			}
			if _, seen := strengths[row.SourceID]; !seen {
				ids = append(ids, row.SourceID)
			}
			if strength := reference.Strength(row.Strength); rank(strength) >= rank(strengths[row.SourceID]) {
				strengths[row.SourceID] = strength
			}
		}
		return err
	})
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	visible, err := h.Linking.visiblePages(ctx, actor, ids, ItemReadAction)
	if err != nil {
		return nil, err
	}
	out := make([]PageBacklink, 0, len(visible))
	for _, id := range ids {
		if page, ok := visible[id]; ok {
			out = append(out, PageBacklink{Page: page, Strength: strengths[id]})
		}
	}
	return out, nil
}

// rank orders the drawings, so a page that both mentions and shows an entry counts as showing it.
func rank(s reference.Strength) int {
	switch s {
	case reference.Embed:
		return 3
	case reference.Card:
		return 2
	case reference.Mention:
		return 1
	}
	return 0
}

// Output is the shape every door returns (schema EntryLinks).
func (r EntryLinks) Output(itemID shared.ID) usecase.Output {
	links := make([]usecase.Output, 0, len(r.Links))
	for _, link := range r.Links {
		other, outward := link.Other(itemID)
		out := LinkOutput(link)
		out["outward"] = outward
		if summary, visible := r.Others[other]; visible {
			out["other"] = EntrySummaryOutput(summary, "")
		} else {
			out["other"] = unavailable(reference.Reference{Kind: reference.KindEntry, ID: other})
		}
		if t, ok := r.Registry.Find(link.Type); ok {
			out["waits"] = string(t.Waits)
		}
		links = append(links, out)
	}
	backlinks := make([]usecase.Output, 0, len(r.Backlinks))
	for _, backlink := range r.Backlinks {
		backlinks = append(backlinks, usecase.Output{
			"source_kind": backlink.SourceKind, "source_id": backlink.SourceID.String(),
			"anchor": anchorOrNil(backlink.Anchor), "strength": string(backlink.Strength),
			"entry": EntrySummaryOutput(r.Sources[backlink.EntryID], ""),
		})
	}
	pageBacklinks := make([]usecase.Output, 0, len(r.PageBacklinks))
	for _, backlink := range r.PageBacklinks {
		pageBacklinks = append(pageBacklinks, usecase.Output{
			"strength": string(backlink.Strength), "page": pageTarget(backlink.Page, nil),
		})
	}
	boardBacklinks := make([]usecase.Output, 0, len(r.BoardBacklinks))
	for _, backlink := range r.BoardBacklinks {
		boardBacklinks = append(boardBacklinks, usecase.Output{
			"strength": string(backlink.Strength), "whiteboard": boardTarget(backlink.ID, backlink.HubID, backlink.Title, nil),
		})
	}
	return usecase.Output{
		"links": links, "backlinks": backlinks, "page_backlinks": pageBacklinks, "whiteboard_backlinks": boardBacklinks,
	}
}

// Descriptor is the catalogue entry.
func (h ListEntryLinks) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListEntryLinksName,
		Permission: service.PermissionRead,
		Summary: "Reads an entry's links from both ends - each with the far end's title, key and state, " +
			"or only its identifier and available: false where the reader may not see it - and its " +
			"backlinks: the notes and comments that mention it, as a card or inline, from entries the " +
			"reader may see.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: itemTarget, Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			links, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return links.Output(id), nil
		}),
	}
}

// Dependency is a link among the entries asked about, both ends visible, with who waits.
type Dependency struct {
	Link  domain.EntryLink
	Waits domain.LinkWaits
}

// ListLinksAmong reads the links between the entries a view shows - the timeline's dependencies
// (UC-PLN-01 check 6). A link with an end the reader may not see is left out.
type ListLinksAmong struct{ Linking Linking }

// Execute answers the links with both ends among ids and visible.
func (h ListLinksAmong) Execute(ctx context.Context, actor appshared.ActorContext, ids []shared.ID) ([]Dependency, error) {
	if err := actor.RequireScope(itemsRead); err != nil {
		return nil, err
	}
	if len(ids) > maxEntriesAmong {
		return nil, shared.ErrValidation.WithDetail("links.too_many_entries").
			WithParams(map[string]string{"maximum": strconv.Itoa(maxEntriesAmong)})
	}
	asked := make(map[shared.ID]bool, len(ids))
	for _, id := range ids {
		asked[id] = true
	}
	var (
		registry domain.LinkTypes
		links    []domain.EntryLink
	)
	err := h.Linking.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rows, err := h.Linking.Types.List(ctx)
		if err != nil {
			return err
		}
		registry = domain.NewLinkTypes(rows)
		links, err = h.Linking.Links.Touching(ctx, ids, maxLinksAmong)
		return err
	})
	if err != nil {
		return nil, err
	}
	among := make([]domain.EntryLink, 0, len(links))
	for _, link := range links {
		if asked[link.SourceID] && asked[link.TargetID] {
			among = append(among, link)
		}
	}
	visible, err := h.Linking.visibleEntries(ctx, actor, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Dependency, 0, len(among))
	for _, link := range among {
		_, sourceVisible := visible[link.SourceID]
		_, targetVisible := visible[link.TargetID]
		if !sourceVisible || !targetVisible {
			continue
		}
		waits := domain.LinkWaitsNone
		if t, ok := registry.Find(link.Type); ok {
			waits = t.Waits
		}
		out = append(out, Dependency{Link: link, Waits: waits})
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ListLinksAmong) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionRead,
		Name:       ListLinksAmongName,
		Summary: "Reads the links between the given entries (at most 500), each with which end waits " +
			"for the other (SOURCE, TARGET or NONE) - what a timeline draws as dependencies. A link " +
			"with an end the caller may not see is left out.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "item_ids", Kind: usecase.KindIDList, Required: true, Description: "The entries a view shows."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: itemTarget, Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids, err := in.IDList("item_ids")
			if err != nil {
				return nil, err
			}
			dependencies, err := h.Execute(ctx, actor, ids)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(dependencies))
			for _, d := range dependencies {
				out := LinkOutput(d.Link)
				out["waits"] = string(d.Waits)
				data = append(data, out)
			}
			return usecase.Output{"data": data}, nil
		}),
	}
}

// ResolveReferences resolves references for the reader (P-19, F-3): each answers what a mention, a
// card or a live view shows of it, or only its address and available: false where the reader may
// not see it, it is not there, or its module does not exist yet - the three are one answer, so a
// reference never tells whether something exists (UC-PLN-01 check 4).
type ResolveReferences struct{ Linking Linking }

// Execute resolves the references in the order they were asked.
func (h ResolveReferences) Execute(ctx context.Context, actor appshared.ActorContext, refs []reference.Reference) ([]usecase.Output, error) {
	if err := actor.RequireScope(itemsRead); err != nil {
		return nil, err
	}
	if len(refs) > maxReferencesAsked {
		return nil, shared.ErrValidation.WithDetail("references.too_many").
			WithParams(map[string]string{"maximum": strconv.Itoa(maxReferencesAsked)})
	}
	var entries, containers, pages, boards, cis, goals []shared.ID
	for _, ref := range refs {
		switch ref.Kind {
		case reference.KindEntry:
			entries = append(entries, ref.ID)
		case reference.KindHub, reference.KindCollection:
			containers = append(containers, ref.ID)
		case reference.KindPage:
			pages = append(pages, ref.ID)
		case reference.KindWhiteboard:
			boards = append(boards, ref.ID)
		case reference.KindCI:
			cis = append(cis, ref.ID)
		case reference.KindGoal:
			goals = append(goals, ref.ID)
		}
	}
	visibleGoals, err := h.Linking.visibleGoals(ctx, actor, goals, ReferencesResolveAction)
	if err != nil {
		return nil, err
	}
	visibleCis, err := h.Linking.visibleCis(ctx, actor, cis, ReferencesResolveAction)
	if err != nil {
		return nil, err
	}
	visibleBoards, err := h.Linking.visibleBoards(ctx, actor, boards, ReferencesResolveAction)
	if err != nil {
		return nil, err
	}
	visiblePages, err := h.Linking.visiblePages(ctx, actor, pages, ReferencesResolveAction)
	if err != nil {
		return nil, err
	}
	visibleEntries, err := h.Linking.visibleEntries(ctx, actor, entries)
	if err != nil {
		return nil, err
	}
	visibleContainers, err := h.visibleContainers(ctx, actor, containers)
	if err != nil {
		return nil, err
	}
	out := make([]usecase.Output, 0, len(refs))
	for _, ref := range refs {
		switch ref.Kind {
		case reference.KindEntry:
			if summary, ok := visibleEntries[ref.ID]; ok {
				out = append(out, EntrySummaryOutput(summary, ref.Anchor))
				continue
			}
		case reference.KindHub, reference.KindCollection:
			if summary, ok := visibleContainers[ref.ID]; ok && string(summary.Kind) == string(ref.Kind) {
				resolved := usecase.Output{
					"kind": string(ref.Kind), "id": ref.ID.String(), "anchor": anchorOrNil(ref.Anchor),
					"available": true, "title": summary.Name, "archived": summary.Archived, "key": nil,
				}
				if summary.Handle != "" {
					resolved["key"] = summary.Handle
				}
				out = append(out, resolved)
				continue
			}
		case reference.KindWhiteboard:
			if summary, ok := visibleBoards[ref.ID]; ok {
				elements := summary.Elements
				target := boardTarget(summary.Board.ID, summary.Board.HubID, summary.Board.Title, &elements)
				target["anchor"] = anchorOrNil(ref.Anchor)
				out = append(out, target)
				continue
			}
		case reference.KindCI:
			if ci, ok := visibleCis[ref.ID]; ok {
				out = append(out, ciTarget(ci, ref.Anchor))
				continue
			}
		case reference.KindPage:
			if summary, ok := visiblePages[ref.ID]; ok {
				target := pageTarget(summary, h.Linking.Clock)
				target["anchor"] = anchorOrNil(ref.Anchor)
				out = append(out, target)
				continue
			}
		case reference.KindGoal:
			if g, ok := visibleGoals[ref.ID]; ok {
				out = append(out, goalTarget(g, ref.Anchor))
				continue
			}
		case reference.KindDashboard:
			d, ok, err := h.Linking.visibleDashboard(ctx, actor, ref.ID, ReferencesResolveAction)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, dashboardTarget(d, ref.Anchor))
				continue
			}
		case reference.KindView:
			resolved, ok, err := h.view(ctx, actor, ref)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, resolved)
				continue
			}
		}
		out = append(out, unavailable(ref))
	}
	return out, nil
}

func (h ResolveReferences) visibleContainers(
	ctx context.Context, actor appshared.ActorContext, ids []shared.ID,
) (map[shared.ID]repository.ContainerSummary, error) {
	visible := map[shared.ID]repository.ContainerSummary{}
	if len(ids) == 0 {
		return visible, nil
	}
	var summaries []repository.ContainerSummary
	err := h.Linking.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := h.Linking.References.Containers(ctx, ids)
		summaries = read
		return err
	})
	if err != nil || len(summaries) == 0 {
		return visible, err
	}
	paths := make([][]identity.Scope, 0, len(summaries))
	for _, s := range summaries {
		path := []identity.Scope{identity.TenantScope()}
		if s.Kind == domain.ContainerHub {
			path = append(path, identity.HubScope(s.ID))
		} else {
			path = append(path, identity.HubScope(s.ParentID), identity.CollectionScope(s.ID))
		}
		paths = append(paths, path)
	}
	allowed, err := h.Linking.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: ContainerReadAction, TokenScope: containersRead,
		TargetType: containerTarget,
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, s := range summaries {
		if i < len(allowed) && allowed[i] {
			visible[s.ID] = s
		}
	}
	return visible, nil
}

// visiblePages answers the pages among ids the reader may read: READ at the page's hub, the
// knowledge module's rule.
func (l Linking) visiblePages(
	ctx context.Context, actor appshared.ActorContext, ids []shared.ID, action audit.Action,
) (map[shared.ID]knowledgerepo.Summary, error) {
	visible := map[shared.ID]knowledgerepo.Summary{}
	if len(ids) == 0 || l.Pages == nil {
		return visible, nil
	}
	var summaries []knowledgerepo.Summary
	err := l.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := l.Pages.Summaries(ctx, ids)
		summaries = read
		return err
	})
	if err != nil || len(summaries) == 0 {
		return visible, err
	}
	paths := make([][]identity.Scope, 0, len(summaries))
	for _, s := range summaries {
		paths = append(paths, []identity.Scope{identity.TenantScope(), identity.HubScope(s.HubID)})
	}
	allowed, err := l.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: action, TokenScope: itemsRead, TargetType: "page",
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, s := range summaries {
		// A restricted page is the people's it names alone (knowledge.Readers).
		if i < len(allowed) && allowed[i] && s.Readers.Admits(actor.AccountID) {
			visible[s.ID] = s
		}
	}
	return visible, nil
}

// view resolves a saved view: its name and layout where the reader sees it - what a live view
// draws in place with the reader's own query answers.
func (h ResolveReferences) view(ctx context.Context, actor appshared.ActorContext, ref reference.Reference) (usecase.Output, bool, error) {
	if h.Linking.Views == nil {
		return nil, false, nil
	}
	var saved view.SavedView
	err := h.Linking.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Linking.Views.Find(ctx, ref.ID)
		saved = found
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	visible, err := viewVisibleTo(ctx, h.Linking.UnitOfWork, h.Linking.Containers, h.Linking.Permits, actor, saved)
	if err != nil || !visible {
		return nil, false, err
	}
	return usecase.Output{
		"kind": string(ref.Kind), "id": ref.ID.String(), "anchor": anchorOrNil(ref.Anchor),
		"available": true, "title": saved.Name, "layout": string(saved.Layout),
	}, true, nil
}

// Descriptor is the catalogue entry.
func (h ResolveReferences) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ResolveReferencesName,
		Permission: service.PermissionRead,
		Summary: "Resolves up to 100 references - each a kind (ENTRY, HUB, COLLECTION, VIEW, PAGE, and the " +
			"later modules' WHITEBOARD, DASHBOARD, GOAL, SPRINT), an id and an optional anchor - " +
			"to what a mention, a card or a live view shows: the title, key and state the caller may " +
			"see. One the caller may not see, or that is not there, answers only its address and " +
			"available: false, never its title. Text writes a reference as a Markdown link to " +
			"hubtask:<kind>/<id>#<anchor>; an image of it draws a card or a live view.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "references", Kind: usecase.KindList, Required: true, Description: "The references: objects with kind, id and optional anchor."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ReferencesResolveAction, TargetType: "reference", Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			refs, err := referencesOf(in["references"])
			if err != nil {
				return nil, err
			}
			resolved, err := h.Execute(ctx, actor, refs)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"data": resolved}, nil
		}),
	}
}

// referencesOf reads the list a door sent: objects with kind, id and anchor.
func referencesOf(raw any) ([]reference.Reference, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, linkFieldInvalid("/references", "references.list_required")
	}
	refs := make([]reference.Reference, 0, len(list))
	for i, item := range list {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, linkFieldInvalid("/references/"+strconv.Itoa(i), "references.object_required")
		}
		kind, _ := object["kind"].(string)
		rawID, _ := object["id"].(string)
		anchor, _ := object["anchor"].(string)
		id, err := shared.ParseID(rawID)
		if err != nil {
			return nil, linkFieldInvalid("/references/"+strconv.Itoa(i)+"/id", "references.id_required")
		}
		ref, err := reference.New(reference.Kind(kind), id, anchor)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func linkFieldInvalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// pageTarget is a readable page as a reference shows it (schema ReferenceTarget). Without a clock
// it does not say whether the page is due for review.
func pageTarget(summary knowledgerepo.Summary, clock interface{ Now() time.Time }) usecase.Output {
	icon := any(nil)
	if summary.Icon != "" {
		icon = summary.Icon
	}
	out := usecase.Output{
		"kind": string(reference.KindPage), "id": summary.ID.String(), "anchor": nil,
		"available": true, "title": summary.Title, "archived": summary.Archived,
		"hub_id": summary.HubID.String(), "icon": icon,
	}
	if clock != nil {
		out["due_for_review"] = summary.VerifiedUntil != nil && !clock.Now().Before(*summary.VerifiedUntil)
	}
	return out
}
