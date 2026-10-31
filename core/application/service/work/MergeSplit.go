// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	MergeWorkItemsName = "MergeWorkItems"
	SplitWorkItemName  = "SplitWorkItem"

	// ItemMergedAction records a merge, besides the link and the archive it is made of, so the
	// trail says the two steps were one decision.
	ItemMergedAction audit.Action = "item.merged"
)

// maxLinksRead bounds the links a merge reads of the entry to find its duplicates link: an entry
// duplicates at most one other, and the read is of the entry's own links.
const maxLinksRead = 500

// MergeWorkItems takes an entry into another that asks the same (UC-SVC-03 check 5): a
// `duplicates` link from the entry to the other, then the entry archived - readable and
// restorable, out of the queues. Nothing is copied and nothing deleted, so a merge made by mistake
// is undone by unarchiving and removing the link.
//
// The two steps are the link's and the archive's own use cases, each in its own transaction, so a
// merge that stopped between them is finished by repeating it: the link already drawn to the
// same entry is taken as drawn (refusal instead of a second link, repair instead of a stuck half).
type MergeWorkItems struct {
	Link    LinkEntries
	Archive ArchiveWorkItem
}

// MergeWorkItemsCommand is the input, typed.
type MergeWorkItemsCommand struct {
	ItemID shared.ID
	IntoID shared.ID
}

// MergeResult names the entry taken in, the one it went into and the link between them.
type MergeResult struct {
	MergedID shared.ID
	IntoID   shared.ID
	LinkID   shared.ID
}

// Execute merges the entry into the other.
func (h MergeWorkItems) Execute(ctx context.Context, actor appshared.ActorContext, cmd MergeWorkItemsCommand) (MergeResult, error) {
	if cmd.ItemID.IsZero() {
		return MergeResult{}, itemIDRequired()
	}
	if cmd.IntoID.IsZero() {
		return MergeResult{}, linkFieldInvalid("/into_id", "items.merge_into_required")
	}
	if cmd.IntoID == cmd.ItemID {
		return MergeResult{}, linkFieldInvalid("/into_id", "items.merge_self")
	}
	linking := h.Link.Linking
	into, intoCollection, err := readItemScope(ctx, linking.UnitOfWork, linking.Items, linking.Containers, actor, cmd.IntoID)
	if err != nil {
		return MergeResult{}, err
	}
	// The merge decides about both entries, so the writer of the other is asked too, as the merged
	// entry's is by the link.
	if err := linking.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems, Path: containerPath(intoCollection),
		Action: ItemMergedAction, TokenScope: itemsWrite, TargetType: itemTarget,
		TargetID: into.ID, On: changing(into),
	}); err != nil {
		return MergeResult{}, err
	}
	if into.IsTrashed() || into.IsArchived() {
		return MergeResult{}, shared.ErrConflict.WithDetail("items.merge_target_closed").
			WithParams(map[string]string{"into_id": into.ID.String()})
	}

	linkID, err := h.drawnBefore(ctx, actor, cmd)
	if err != nil {
		return MergeResult{}, err
	}
	if linkID.IsZero() {
		link, err := h.Link.Execute(ctx, actor, LinkEntriesCommand{
			SourceID: cmd.ItemID, TargetID: cmd.IntoID, Type: domain.LinkTypeDuplicates,
		})
		if err != nil {
			return MergeResult{}, err
		}
		linkID = link.ID
	}
	if _, err := h.Archive.Execute(ctx, actor, LifecycleCommand{ItemID: cmd.ItemID}); err != nil {
		return MergeResult{}, err
	}
	err = linking.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return linking.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: linking.Clock.Now(), Action: ItemMergedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: itemTarget, TargetID: cmd.ItemID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "into_item_id", Classification: audit.Open, To: cmd.IntoID.String()},
				audit.Change{Field: "link_id", Classification: audit.Open, To: linkID.String()},
			),
		})
	})
	if err != nil {
		return MergeResult{}, err
	}
	return MergeResult{MergedID: cmd.ItemID, IntoID: cmd.IntoID, LinkID: linkID}, nil
}

// drawnBefore is the duplicates link a merge that stopped after its link left, or zero.
func (h MergeWorkItems) drawnBefore(ctx context.Context, actor appshared.ActorContext, cmd MergeWorkItemsCommand) (shared.ID, error) {
	var found shared.ID
	err := h.Link.Linking.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		links, err := h.Link.Linking.Links.Of(ctx, cmd.ItemID, maxLinksRead)
		if err != nil {
			return err
		}
		for _, link := range links {
			if link.Type == domain.LinkTypeDuplicates && link.SourceID == cmd.ItemID && link.TargetID == cmd.IntoID {
				found = link.ID
			}
		}
		return nil
	})
	return found, err
}

// Descriptor registers the use case in all three channels.
func (h MergeWorkItems) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionWriteItems,
		Name:       MergeWorkItemsName,
		Summary: "Takes an entry into another that asks the same: the entry is linked as a " +
			"duplicate of the other and archived, so it leaves the queues and stays readable and " +
			"restorable. Nothing is copied and nothing deleted. Repeating a merge finishes it.",
		SideEffects: "Draws a duplicates link (announced, recorded, audited as a link), " +
			"archives the entry (announced, recorded, audited as an archive), and writes one " +
			"audit entry for the merge.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry taken in."},
			{Name: "into_id", Kind: usecase.KindID, Required: true, Description: "The entry it is taken into; it stays open."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemMergedAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the link and the archive a merge is made of each write their own step of the entries' histories.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			intoID, err := in.ID("into_id")
			if err != nil {
				return nil, err
			}
			result, err := h.Execute(ctx, actor, MergeWorkItemsCommand{ItemID: itemID, IntoID: intoID})
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"merged_id": result.MergedID.String(), "into_id": result.IntoID.String(), "link_id": result.LinkID.String(),
			}, nil
		}),
	}
}

// SplitWorkItem makes a new entry beside an entry for a part of what it asks (UC-SVC-03 check 6):
// a copy of the entry alone with the title and notes given, made for the same requester - its
// creator is the entry's, so the portal lists it among the requester's requests and its replies
// reach them - and, where a link type is named, linked from the entry.
//
// Whatever can refuse the link is asked before the copy is made: the writer on the entry, the
// type, and a type of one per entry already used. A new entry closes no loop.
type SplitWorkItem struct {
	Duplicate DuplicateWorkItem
	Link      LinkEntries
}

// SplitWorkItemCommand is the input, typed.
type SplitWorkItemCommand struct {
	ItemID   shared.ID
	Title    string
	Notes    *string
	LinkType string
}

// Execute splits the part off.
func (h SplitWorkItem) Execute(ctx context.Context, actor appshared.ActorContext, cmd SplitWorkItemCommand) (DuplicateResult, error) {
	if cmd.ItemID.IsZero() {
		return DuplicateResult{}, itemIDRequired()
	}
	title, err := domain.ItemTitle(cmd.Title, h.Duplicate.Text)
	if err != nil {
		return DuplicateResult{}, err
	}
	var notes *string
	if cmd.Notes != nil {
		kept, err := domain.ItemNotes(*cmd.Notes, h.Duplicate.Text)
		if err != nil {
			return DuplicateResult{}, err
		}
		notes = &kept
	}
	if cmd.LinkType != "" {
		if err := h.mayLink(ctx, actor, cmd); err != nil {
			return DuplicateResult{}, err
		}
	}

	plan, err := h.Duplicate.plan(ctx, actor, DuplicateWorkItemCommand{ItemID: cmd.ItemID, Title: title})
	if err != nil {
		return DuplicateResult{}, err
	}
	plan.createdBy = plan.source.CreatedBy
	plan.notes = notes
	result, err := h.Duplicate.perform(ctx, actor, plan)
	if err != nil {
		return DuplicateResult{}, err
	}
	if cmd.LinkType != "" {
		if _, err := h.Link.Execute(ctx, actor, LinkEntriesCommand{
			SourceID: cmd.ItemID, TargetID: result.Item.ID, Type: cmd.LinkType,
		}); err != nil {
			return DuplicateResult{}, err
		}
	}
	return result, nil
}

// mayLink asks what could refuse the link, before anything is written.
func (h SplitWorkItem) mayLink(ctx context.Context, actor appshared.ActorContext, cmd SplitWorkItemCommand) error {
	linking := h.Link.Linking
	source, collection, err := readItemScope(ctx, linking.UnitOfWork, linking.Items, linking.Containers, actor, cmd.ItemID)
	if err != nil {
		return err
	}
	if err := linking.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems, Path: containerPath(collection),
		Action: ItemLinkedAction, TokenScope: itemsWrite, TargetType: itemTarget,
		TargetID: source.ID, On: changing(source),
	}); err != nil {
		return err
	}
	return linking.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rows, err := linking.Types.List(ctx)
		if err != nil {
			return err
		}
		linkType, ok := domain.NewLinkTypes(rows).Find(cmd.LinkType)
		if !ok {
			return linkFieldInvalid("/link_type", "links.type_unknown")
		}
		if linkType.Cardinality != domain.LinkOne {
			return nil
		}
		count, err := linking.Links.CountFrom(ctx, source.ID, linkType.Key)
		if err != nil {
			return err
		}
		if count > 0 {
			return shared.ErrConflict.WithDetail("links.cardinality").WithParams(map[string]string{"type": linkType.Key})
		}
		return nil
	})
}

// Descriptor registers the use case in all three channels.
func (h SplitWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionWriteItems,
		Name:       SplitWorkItemName,
		Summary: "Makes a new entry beside an entry for a part of what it asks: a copy of the " +
			"entry alone with the title and notes given, made for the same requester, carrying " +
			"neither the comments nor the history, and linked from the entry by the link type " +
			"named. What the copy cannot carry is reported.",
		SideEffects: "Writes one new entry as a copy does (announced, recorded, audited as a " +
			"duplication) and, where a link type is named, draws the link (announced, recorded, " +
			"audited as a link).",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry a part is split off."},
			{Name: "title", Kind: usecase.KindString, Required: true, Description: "The new entry's title."},
			{Name: "notes", Kind: usecase.KindString, Description: "The new entry's notes; omitted keeps the entry's."},
			{Name: "link_type", Kind: usecase.KindString, Description: "The key of a link type drawn from the entry to the new one; omitted draws none."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemDuplicatedAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the copy and the link a split is made of each write their own step of the entries' histories.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			cmd := SplitWorkItemCommand{ItemID: itemID, Title: in.String("title"), LinkType: in.String("link_type")}
			if in.Present("notes") {
				notes := in.String("notes")
				cmd.Notes = &notes
			}
			result, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			answer, err := h.Duplicate.Gate.Answer(ctx, h.Duplicate.UnitOfWork, actor, result.Item)
			if err != nil {
				return nil, err
			}
			return duplicateOutput(result, answer), nil
		}),
	}
}
