// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// SET_ADD and SET_REMOVE (N-07, offline-sync.md §4.2's sets row): the OR-set on the request path.
// The mutation names the set, the element and the device's tag; the server reads the element's
// stored tags, merges the device's in with the rule SetElement.go has held since C-03, and applies
// whatever the merged element says through the use case that owns the set - under the device's
// tag, so the row carries the reading that decided. An addition a later removal already undid,
// and a removal of an element that was never there, change nothing and answer MERGED.

// SetElements is the slice of a set's repository the merge reads: every tag of one set on one
// entry. The three set repositories share the shape (work.ItemLabels, work.ItemMembers,
// media.ItemAttachments).
type SetElements interface {
	Elements(ctx context.Context, itemID shared.ID) ([]work.SetElement, error)
}

// setOwner is what the push knows about one set: where its tags are read, and which use cases
// add to it and remove from it, with the name the element goes in under.
type setOwner struct {
	elements SetElements
	add      string
	remove   string
	element  string
}

// Sets is the table of sets a device may change (N-07). `watchers` is in the contract's enum and
// no use case writes a watcher; it stays out of this table until one does, and the frame refuses
// it by name.
type Sets struct {
	Labels      SetElements
	Members     SetElements
	Attachments SetElements
}

func (s Sets) owner(set work.SetName) (setOwner, bool) {
	switch set {
	case work.SetLabels:
		return setOwner{elements: s.Labels, add: "AddLabel", remove: "RemoveLabel", element: "label_id"}, s.Labels != nil
	case work.SetMembers:
		return setOwner{elements: s.Members, add: "AddMember", remove: "RemoveMember", element: "account_id"}, s.Members != nil
	case work.SetAttachments:
		return setOwner{elements: s.Attachments, add: "AttachMedia", remove: "DetachMedia", element: "media_id"}, s.Attachments != nil
	}
	return setOwner{}, false
}

// setChange applies a SET_ADD or a SET_REMOVE.
func (p PushChanges) setChange(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	if m.ItemID.IsZero() {
		return Result{}, shared.ErrValidation.WithDetail("sync.item_required")
	}
	if m.Element.IsZero() {
		return Result{}, shared.ErrValidation.
			WithDetail("sync.element_required").
			WithFields(shared.FieldError{Path: "/element", Code: "sync.element_required"})
	}
	set := work.SetName(m.Set)
	owner, served := p.Sets.owner(set)
	if !served {
		return Result{}, shared.ErrValidation.
			WithDetail("sync.set_unknown").
			WithParams(map[string]string{"set": m.Set}).
			WithFields(shared.FieldError{Path: "/set", Code: "sync.set_unknown"})
	}
	tag, err := shared.ParseHLC(m.HLC)
	if err != nil {
		return Result{}, err
	}

	// The server's copy, through the use case that reads it, so that a device that may not read
	// the entry gets that refusal and not a merge - and the stored tags of the element.
	before, err := p.Catalogue.Invoke(ctx, "GetWorkItem", actor, usecase.Input{"item_id": m.ItemID.String()})
	if err != nil {
		return Result{}, err
	}
	var stored []work.SetElement
	err = p.Stream.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		stored, err = owner.elements.Elements(ctx, m.ItemID)
		return err
	})
	if err != nil {
		return Result{}, err
	}

	// The merge, the rule written once in SetElement.go: the union of what either side knows,
	// each element's tags the later of the two readings.
	mine := work.SetElement{ElementID: m.Element}
	if m.Kind == domain.SetAdd {
		mine.AddedAt = tag
	} else {
		mine.RemovedAt = tag
	}
	wasPresent := presentIn(stored, m.Element)
	merged := work.MergeSetElements(stored, []work.SetElement{mine})
	nowPresent := presentIn(merged, m.Element)

	if wasPresent == nowPresent {
		// The device's tag lost: an addition a later removal already undid, or a removal of an
		// element that was never there, or a repeat of what the server already holds. Nothing
		// to apply - the winning tag is later than this one, so no future merge would read it -
		// and the device adopts the server's state.
		return Result{OpID: m.OpID, Result: domain.Merged, EntityID: m.ItemID, ServerState: before}, nil
	}

	name := owner.remove
	if nowPresent {
		name = owner.add
	}
	// Under the device's tag, carried the way a field's reading is: the writer stamps the row and
	// the change log entry with it, so the next device compares against the tag that decided.
	applied := appshared.ContextWithReadings(ctx, map[string]shared.HLC{string(set): tag})
	if _, err := p.Catalogue.Invoke(applied, name, actor, usecase.Input{
		"item_id": m.ItemID.String(), owner.element: m.Element.String(),
	}); err != nil {
		return Result{}, err
	}
	after, err := p.Catalogue.Invoke(ctx, "GetWorkItem", actor, usecase.Input{"item_id": m.ItemID.String()})
	if err != nil {
		return Result{}, err
	}
	return Result{OpID: m.OpID, Result: domain.Applied, EntityID: m.ItemID, ServerState: after}, nil
}

// presentIn reports whether the element is in the set the elements describe.
func presentIn(elements []work.SetElement, id shared.ID) bool {
	for _, element := range elements {
		if element.ElementID == id {
			return element.IsPresent()
		}
	}
	return false
}
