// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	InstantiateTemplateName = "InstantiateTemplate"

	// TemplateInstantiatedAction is the audit code. One entry for one act, whatever the tree's
	// size (audit.md §2).
	TemplateInstantiatedAction audit.Action = "template.instantiated"
)

// InstantiateTemplate stamps a template out into a collection (D-06).
//
// A sibling of C-11's duplicate rather than the same machinery: there is no source tree to read -
// the shape is a document - but everything else is the same shape of problem, and the answers are
// the ones the copy already gave. Every entry carries its own records, because a client
// synchronising the collection has to learn about every row; the references the destination cannot
// carry are reported rather than dropped silently (I-W6); and the whole act is one audit entry and
// one announcement.
type InstantiateTemplate struct {
	Writer      TemplateWriter
	Items       repository.Items
	ItemMembers repository.ItemMembers
	// Visibility answers whether a node's fixed assignee can see the collection the tree lands in.
	// The same question an assignment asks (C-01), asked here because a template written last year
	// may name somebody who has left.
	Visibility Visibility
	Events     outbox.Events
	Activity   ActivityJournal
}

// InstantiateTemplateCommand is the input, typed.
type InstantiateTemplateCommand struct {
	TemplateID   shared.ID
	CollectionID shared.ID
	ParentID     shared.ID
	// Anchor is the day the relative dates count from, and the zero time for "now". A day rather
	// than an instant: "+3 days for a project starting Monday" is about days, and it is read in
	// the caller's own zone.
	Anchor time.Time
	Title  string
}

// InstantiationResult is what an instantiation answers with.
type InstantiationResult struct {
	TemplateID shared.ID
	Root       domain.WorkItem
	// Created is the size of the tree, the root included.
	Created int
	// DroppedReferences is what the destination could not carry, from every entry that lost
	// something. Empty rather than nil: a client that iterates the losses should not have to
	// nil-check the field.
	DroppedReferences []domain.DroppedReference
}

// Execute stamps the template out and returns the tree's root.
func (h InstantiateTemplate) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd InstantiateTemplateCommand,
) (InstantiationResult, error) {
	w := h.Writer
	if cmd.TemplateID.IsZero() {
		return InstantiationResult{}, templateIDRequired()
	}
	if cmd.CollectionID.IsZero() {
		return InstantiationResult{}, shared.ErrValidation.
			WithDetail("templates.collection_required").
			WithFields(shared.FieldError{
				Path: "/collection_id", Code: "templates.collection_required",
			})
	}

	// The template and the destination are read before the permission question, because the answer
	// depends on the destination's path. Nothing read here is trusted afterwards.
	var collection domain.Container
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		collection, err = w.Containers.Find(ctx, cmd.CollectionID)
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return InstantiationResult{}, shared.ErrNotFound.
				WithDetail("containers.not_found").
				WithParams(map[string]string{"container_id": cmd.CollectionID.String()})
		}
		return InstantiationResult{}, err
	}

	// WRITE_ITEMS at the destination, not STRUCTURE: stamping a template out is creating entries,
	// and whoever may create one may create the tree. Defining the template is the structural act,
	// and that is where STRUCTURE is asked (CreateTemplate).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     TemplateInstantiatedAction,
		TokenScope: templatesWrite,
		TargetType: templateTarget,
		TargetID:   cmd.TemplateID,
	}); err != nil {
		return InstantiationResult{}, err
	}

	var result InstantiationResult
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// One reading of the clock for the whole tree, so that every entry it produces agrees
		// about when it came into being.
		now := w.Clock.Now()

		template, err := w.findTemplate(ctx, cmd.TemplateID)
		if err != nil {
			return err
		}
		destination, err := findCollection(ctx, w.Containers, cmd.CollectionID)
		if err != nil {
			return err
		}
		if err := destination.EnsureAcceptsItems(); err != nil {
			return err
		}

		result, err = h.stamp(ctx, actor, template, destination, cmd, now)
		return err
	})
	if err != nil {
		return InstantiationResult{}, err
	}
	return result, nil
}

// stamp writes the tree.
func (h InstantiateTemplate) stamp(
	ctx context.Context, actor appshared.ActorContext, template domain.Template,
	destination domain.Container, cmd InstantiateTemplateCommand, now time.Time,
) (InstantiationResult, error) {
	w := h.Writer

	rows, err := w.Profiles.List(ctx)
	if err != nil {
		return InstantiationResult{}, err
	}
	hierarchy, err := service.NewHierarchy(rows, rows)
	if err != nil {
		return InstantiationResult{}, err
	}

	parent, err := h.parentOf(ctx, cmd.ParentID, destination.ID)
	if err != nil {
		return InstantiationResult{}, err
	}
	spot, err := hierarchy.Place(parent, template.RootType)
	if err != nil {
		return InstantiationResult{}, err
	}

	// The cap again, at the moment it is being written rather than only when it was defined: the
	// profiles may have been narrowed since, and this is the transaction that would be large.
	if template.NodeCount() > domain.MaxTemplateNodes {
		return InstantiationResult{}, shared.ErrValidation.
			WithDetail("templates.too_many_nodes").
			WithParams(map[string]string{
				"maximum": strconv.Itoa(domain.MaxTemplateNodes),
				"count":   strconv.Itoa(template.NodeCount()),
			})
	}

	previous, err := h.Items.LastOrderKey(ctx, destination.ID, spot.ParentID)
	if err != nil {
		return InstantiationResult{}, err
	}

	stamp := &instantiation{
		template:    template,
		destination: destination,
		anchor:      h.anchorOf(cmd, actor, now),
		zone:        actor.TimeZone,
		lastRank:    map[shared.ID]string{spot.ParentID: previous},
		dropped:     []domain.DroppedReference{},
	}

	root, err := h.write(ctx, actor, stamp, hierarchy, spot, template.Root, cmd.Title, now)
	if err != nil {
		return InstantiationResult{}, err
	}

	announcement, err := event.NewTemplateInstantiated(w.IDs.NewID(), template.ID, root,
		stamp.created, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return InstantiationResult{}, err
	}
	if err := h.Events.Append(ctx, announcement); err != nil {
		return InstantiationResult{}, err
	}
	if err := h.recordAudit(ctx, actor, template, root, stamp.created, now); err != nil {
		return InstantiationResult{}, err
	}

	return InstantiationResult{
		TemplateID: template.ID, Root: root, Created: stamp.created,
		DroppedReferences: stamp.dropped,
	}, nil
}

// instantiation is the state one stamping carries: where it lands, what it counts from, and what
// it has had to leave behind.
type instantiation struct {
	template    domain.Template
	destination domain.Container
	anchor      time.Time
	zone        string
	// lastRank is the rank of the last entry written at one level, keyed by the parent - the zero
	// identifier for the top level of the destination collection.
	lastRank map[shared.ID]string
	dropped  []domain.DroppedReference
	created  int
}

// write produces one entry and everything under it, parents first so that a child always finds the
// parent it hangs from.
func (h InstantiateTemplate) write(
	ctx context.Context, actor appshared.ActorContext, stamp *instantiation,
	hierarchy service.Hierarchy, spot service.Placement, node domain.TemplateNode,
	titleOverride string, now time.Time,
) (domain.WorkItem, error) {
	w := h.Writer

	profile, err := hierarchy.Profile(node.Type)
	if err != nil {
		return domain.WorkItem{}, err
	}

	rank, err := service.OrderKeyBetween(stamp.lastRank[spot.ParentID], "")
	if err != nil {
		return domain.WorkItem{}, err
	}
	stamp.lastRank[spot.ParentID] = rank

	title := node.Title
	if titleOverride != "" {
		title = titleOverride
	}

	id := w.IDs.NewID()
	item, err := domain.NewWorkItem(domain.NewWorkItemInput{
		ID:           id,
		TenantID:     actor.TenantID,
		CollectionID: stamp.destination.ID,
		Type:         node.Type,
		ParentID:     spot.ParentID,
		Title:        title,
		Notes:        node.Notes,
		// The creator's locale where the template says nothing, exactly as a create decides it
		// (i18n-l10n.md §5): a template is a shape rather than a language.
		ContentLanguage: actor.Locale,
		Profile:         profile,
		Path:            spot.PathOf(id),
		Depth:           spot.Depth,
		OrderKey:        rank,
		CreatedBy:       actor.AccountID,
		Now:             now,
	})
	if err != nil {
		return domain.WorkItem{}, err
	}

	due, err := node.DueAt(stamp.anchor, stamp.zone)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if due != nil && profile.Allows(domain.CapabilityDueDate) {
		item.Due = due
	}
	item.AssigneeID, err = h.assigneeFor(ctx, actor, stamp, node, item, profile)
	if err != nil {
		return domain.WorkItem{}, err
	}

	// The copy's statement rather than the create's: an instantiation writes the fields a node
	// already carries, all at once, because there is nothing to decide about them a second time
	// and writing them through three more statements would spend three versions on an entry that
	// was born a moment ago (db/queries/Work.sql, CopyWorkItem).
	if err := h.Items.InsertCopy(ctx, repository.Copy{Item: item}); err != nil {
		return domain.WorkItem{}, err
	}
	stamp.created++

	if err := h.announce(ctx, actor, item, now); err != nil {
		return domain.WorkItem{}, err
	}

	for _, child := range node.Children {
		childSpot, err := hierarchy.Place(&item, child.Type)
		if err != nil {
			return domain.WorkItem{}, err
		}
		if _, err := h.write(
			ctx, actor, stamp, hierarchy, childSpot, child, "", now,
		); err != nil {
			return domain.WorkItem{}, err
		}
	}
	return item, nil
}

// assigneeFor is the node's fixed assignee, when the destination can carry them.
//
// Dropped and reported rather than written blindly (I-W6): a template written last year may name
// somebody who has left the collection, and an entry on somebody who cannot see it is an entry
// nobody can act on. Reported as an ASSIGNEE, which is the kind the copy already uses for exactly
// this.
func (h InstantiateTemplate) assigneeFor(
	ctx context.Context, actor appshared.ActorContext, stamp *instantiation,
	node domain.TemplateNode, item domain.WorkItem, profile domain.CapabilityProfile,
) (shared.ID, error) {
	if node.AssigneeID.IsZero() {
		return noID, nil
	}
	if !profile.Allows(domain.CapabilityAssignment) {
		stamp.dropped = append(stamp.dropped, domain.DroppedReference{
			ItemID: item.ID, Kind: domain.ReferenceAssignee, ID: node.AssigneeID.String(),
			Code: "items.capability_not_supported",
		})
		return noID, nil
	}

	permitted, err := h.Visibility.CanSee(
		ctx, actor, node.AssigneeID, containerPath(stamp.destination))
	if err != nil {
		return noID, err
	}
	if !permitted {
		stamp.dropped = append(stamp.dropped, domain.DroppedAssignee(item.ID, node.AssigneeID))
		return noID, nil
	}
	return node.AssigneeID, nil
}

// announce writes what one created entry owes: the event outwards, the change log entry for
// offline clients, and the step of its own history.
//
// One set per entry, deliberately, exactly as a copy does it: a client synchronising the
// destination has to learn about every row, and one record covering a tree would leave it with a
// root whose children it has never heard of.
func (h InstantiateTemplate) announce(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem, now time.Time,
) error {
	w := h.Writer

	announcement, err := event.NewItemCreated(w.IDs.NewID(), item,
		event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return err
	}
	if err := h.Events.Append(ctx, announcement); err != nil {
		return err
	}
	if err := w.Changes.Record(ctx, changelog.Change{
		TenantID:    item.TenantID,
		Entity:      itemTarget,
		EntityID:    item.ID,
		Op:          changelog.Upsert,
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
		Payload:     announcement.Payload,
	}); err != nil {
		return err
	}
	return h.Activity.record(ctx, actor, item, activity.ItemCreated,
		activity.ChangeSet(activity.Compact), now)
}

// recordAudit writes one entry for one act, whatever the tree's size (audit.md §2). The titles do
// not travel: they are content, and what an auditor needs is that a template was stamped out, by
// whom, into what.
func (h InstantiateTemplate) recordAudit(
	ctx context.Context, actor appshared.ActorContext, template domain.Template,
	root domain.WorkItem, created int, now time.Time,
) error {
	return h.Writer.Audit.Append(ctx, audit.Entry{
		TenantID:   root.TenantID,
		OccurredAt: now,
		Action:     TemplateInstantiatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: templateTarget,
		TargetID:   template.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{
				Field: "collection_id", Classification: audit.Open,
				To: root.CollectionID.String(),
			},
			audit.Change{Field: "root_item_id", Classification: audit.Open, To: root.ID.String()},
			audit.Change{Field: "created", Classification: audit.Open, To: strconv.Itoa(created)},
		),
	})
}

// anchorOf is the day the relative dates count from: the one the request named, or today.
//
// Read in the caller's own zone, which is what makes "+3 days" mean the same thing to the person
// who wrote the template and the person using it (i18n-l10n.md §4, and the same rule the query
// language's @today follows).
func (h InstantiateTemplate) anchorOf(
	cmd InstantiateTemplateCommand, actor appshared.ActorContext, now time.Time,
) time.Time {
	if !cmd.Anchor.IsZero() {
		return cmd.Anchor
	}

	location := time.UTC
	if actor.TimeZone != "" {
		if loaded, err := time.LoadLocation(actor.TimeZone); err == nil {
			location = loaded
		}
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

// parentOf reads the entry a tree hangs from, or nil for the top level of the collection.
func (h InstantiateTemplate) parentOf(
	ctx context.Context, parentID, collectionID shared.ID,
) (*domain.WorkItem, error) {
	if parentID.IsZero() {
		return nil, nil
	}

	parent, err := findItem(ctx, h.Items, parentID)
	if err != nil {
		return nil, err
	}
	if parent.CollectionID != collectionID {
		return nil, shared.ErrValidation.
			WithDetail("items.parent_not_in_collection").
			WithFields(shared.FieldError{
				Path: "/parent_id", Code: "items.parent_not_in_collection",
			})
	}
	if err := parent.EnsureEditable(); err != nil {
		return nil, err
	}
	return &parent, nil
}

// Descriptor is the catalogue entry.
func (h InstantiateTemplate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: InstantiateTemplateName,
		Summary: "Stamps a template out into a collection: an entry tree whose relative dates " +
			"have become absolute ones, anchored either at a date the request names or at today " +
			"in the caller's own time zone. What the destination cannot carry is reported rather " +
			"than dropped silently - an assignee who cannot see it, a type its profiles no " +
			"longer permit. Needs the right to create entries there, not the right to define " +
			"templates.",
		SideEffects: "Writes every entry of the tree with its own event, change log entry and " +
			"history step, announces " + string(event.TemplateInstantiated) + " once, and writes " +
			"one audit entry.",
		TokenScope: templatesWrite,
		Input: []usecase.Field{
			{
				Name: "template_id", Kind: usecase.KindID, Required: true,
				Description: "The template to stamp out.",
			},
			{
				Name: "collection_id", Kind: usecase.KindID, Required: true,
				Description: "The collection the tree lands in.",
			},
			{
				Name: "parent_id", Kind: usecase.KindID,
				Description: "The entry the tree hangs from. Omitted puts it at the top level.",
			},
			{
				Name: "anchor_date", Kind: usecase.KindString,
				Description: "The day the relative dates count from, as YYYY-MM-DD. Omitted " +
					"anchors at today.",
			},
			{
				Name: "title", Kind: usecase.KindString,
				Description: "The root entry's title, overriding the template's own.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TemplateInstantiatedAction, TargetType: templateTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h InstantiateTemplate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	templateID, err := in.ID("template_id")
	if err != nil {
		return nil, err
	}
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}
	parentID, err := in.ID("parent_id")
	if err != nil {
		return nil, err
	}

	var anchor time.Time
	if raw := in.String("anchor_date"); raw != "" {
		location := time.UTC
		if actor.TimeZone != "" {
			if loaded, zoneErr := time.LoadLocation(actor.TimeZone); zoneErr == nil {
				location = loaded
			}
		}
		day, parseErr := time.ParseInLocation("2006-01-02", raw, location)
		if parseErr != nil {
			return nil, shared.ErrValidation.
				WithDetail("templates.anchor_invalid").
				WithParams(map[string]string{"value": raw}).
				WithFields(shared.FieldError{
					Path: "/anchor_date", Code: "templates.anchor_invalid",
				})
		}
		anchor = day
	}

	result, err := h.Execute(ctx, actor, InstantiateTemplateCommand{
		TemplateID: templateID, CollectionID: collectionID, ParentID: parentID,
		Anchor: anchor, Title: in.String("title"),
	})
	if err != nil {
		return nil, err
	}

	dropped := make([]usecase.Output, 0, len(result.DroppedReferences))
	for _, reference := range result.DroppedReferences {
		dropped = append(dropped, usecase.Output{
			"item_id":   reference.ItemID.String(),
			"kind":      string(reference.Kind),
			"reference": reference.ID,
			"code":      reference.Code,
		})
	}
	return usecase.Output{
		"template_id":        result.TemplateID.String(),
		"root_item_id":       result.Root.ID.String(),
		"created":            result.Created,
		"dropped_references": dropped,
	}, nil
}
