// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	MoveContainerName = "MoveContainer"

	// ContainerMovedAction is the audit code. One code for a change of hub and for a reorder within
	// one, because both are "this collection sits somewhere else now" - an auditor recognises a
	// reorder in the entry by the parent being unchanged (audit.md §2).
	ContainerMovedAction audit.Action = "container.moved"
)

// MoveContainer moves a collection into another hub, and ranks it there.
//
// A hub is refused: it sits in the tenant and in nothing else (I-C1), so there is no destination to
// name. Naming the hub a collection already sits in reorders it there, which is the same operation
// with the same event - a drag within a level and a drag between hubs are one gesture to a person.
//
// Nothing under the collection is written. A container tree is two deep, so a collection has no
// containers below it whose placement would have to follow, and the items it holds reference their
// collection rather than a path through the hubs.
type MoveContainer struct {
	Writer ContainerWriter
}

// MoveContainerCommand is the input, typed.
type MoveContainerCommand struct {
	ContainerID shared.ID
	// TargetParentID is the hub to move into. Required, and not nullable: there is no level above a
	// hub for a collection to be moved to.
	TargetParentID shared.ID
	// BeforeContainerID is the sibling to land in front of at the destination. Empty appends.
	BeforeContainerID shared.ID
	ExpectedVersion   int
}

// Execute moves the collection and returns it as it now stands.
func (h MoveContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd MoveContainerCommand,
) (domain.Container, error) {
	if cmd.ContainerID.IsZero() {
		return domain.Container{}, containerIDRequired()
	}
	if cmd.TargetParentID.IsZero() {
		return domain.Container{}, shared.ErrValidation.
			WithDetail("containers.target_parent_required").
			WithFields(shared.FieldError{
				Path: "/target_parent_id", Code: "containers.target_parent_required",
			})
	}

	container, target, err := h.read(ctx, actor, cmd)
	if err != nil {
		return domain.Container{}, err
	}

	// Two permission questions, and both before the transaction: a refusal writes an audit entry,
	// and an entry written inside the transaction would be rolled back with the refusal
	// (audit.md §7).
	//
	// The destination first, because that is where the collection ends up and the commoner refusal.
	if err := h.authorize(ctx, actor, target, cmd.ContainerID); err != nil {
		return domain.Container{}, err
	}
	// And the source, when the two differ. Taking a collection out of a hub is a change to that hub
	// as much as to the destination, and somebody who may write to only the destination must not be
	// able to reach into a hub they cannot write to and take something out of it.
	if container.ParentID != target.ID {
		if err := h.authorize(ctx, actor, container, cmd.ContainerID); err != nil {
			return domain.Container{}, err
		}
	}

	var moved domain.Container
	err = h.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Writer.Clock.Now()

		// Read again inside the transaction: everything the move was decided from can have changed
		// since, and the destination in particular has to be acceptable at the moment the row is
		// written.
		fresh, destination, err := h.readIn(ctx, cmd)
		if err != nil {
			return err
		}

		orderKey, err := h.rankAt(ctx, cmd, destination)
		if err != nil {
			return err
		}

		wanted, changes, err := fresh.MovedInto(destination, orderKey, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// It already sits where the caller asked for, at the rank it asked for. Nothing is
			// written, no version is spent and nothing is announced.
			if err := ensureContainerVersion(fresh, cmd.ExpectedVersion); err != nil {
				return err
			}
			moved = fresh
			return nil
		}

		moved, err = h.write(ctx, actor, fresh, wanted, changes, cmd.ExpectedVersion, now)
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return moved, nil
}

// write stores the placement and records what the move owes: the event outwards, the change log for
// offline clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (h MoveContainer) write(
	ctx context.Context, actor appshared.ActorContext, before, after domain.Container,
	changes []domain.FieldChange, expectedVersion int, now time.Time,
) (domain.Container, error) {
	expected := expectedVersion
	if expected == 0 {
		expected = before.Version
	}
	// A name already taken at the destination comes back from the unique index as
	// `containers.name_taken`. Checked there rather than beforehand: a check followed by an update is
	// two statements with a gap, and two moves arriving inside that gap both pass the check.
	if err := h.Writer.Containers.SetPlacement(ctx, after, expected); err != nil {
		return domain.Container{}, err
	}
	after.Version = expected + 1

	announcement, err := event.NewContainerMoved(
		h.Writer.IDs.NewID(), after, before.ParentID,
		event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return domain.Container{}, err
	}
	if err := h.Writer.Events.Append(ctx, announcement); err != nil {
		return domain.Container{}, err
	}
	// One entry per field, as every container change records them. `parent_id` merges last writer
	// wins; `order_key` is a fractional index and merges by itself, which is the whole reason the
	// rank is a key rather than a number (offline-sync.md §4.2).
	if err := h.Writer.recordChanges(ctx, after, actor, changes); err != nil {
		return domain.Container{}, err
	}
	// All of it is structure - two identifiers and a rank - so all of it is OPEN in the data
	// catalogue's sense. No name: user content stays out of the trail (rule 10), and "who moved this
	// where" needs none of it.
	if err := h.Writer.recordAudit(
		ctx, after, actor, ContainerMovedAction, audit.Open, changes, now); err != nil {
		return domain.Container{}, err
	}
	return after, nil
}

// rankAt works out the rank the collection takes at its destination.
//
// The bounds come from the database and the key from the ordering service, which is what makes an
// insertion between two neighbours renumber nothing: a fractional index needs only the two keys
// either side (offline-sync.md §4.2).
func (h MoveContainer) rankAt(
	ctx context.Context, cmd MoveContainerCommand, destination domain.Container,
) (string, error) {
	previous, next, err := h.Writer.Containers.Neighbours(
		ctx, destination.ID, cmd.BeforeContainerID, cmd.ContainerID)
	if err != nil {
		return "", err
	}
	if !cmd.BeforeContainerID.IsZero() && next == "" {
		// The sibling named is not at this level. Its own answer rather than a silent append: a
		// client that asked for a position and got the end of the list has been ignored.
		return "", shared.ErrValidation.
			WithDetail("containers.before_container_not_in_level").
			WithParams(map[string]string{"before_container_id": cmd.BeforeContainerID.String()}).
			WithFields(shared.FieldError{
				Path: "/before_container_id", Code: "containers.before_container_not_in_level",
			})
	}
	return service.OrderKeyBetween(previous, next)
}

// read resolves the collection and its destination outside the write transaction, because the
// permission questions need both first.
func (h MoveContainer) read(
	ctx context.Context, actor appshared.ActorContext, cmd MoveContainerCommand,
) (domain.Container, domain.Container, error) {
	var container, target domain.Container

	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			var err error
			container, target, err = h.readIn(ctx, cmd)
			return err
		})
	if err != nil {
		return domain.Container{}, domain.Container{}, err
	}
	return container, target, nil
}

// readIn resolves both containers inside whichever transaction the caller opened.
func (h MoveContainer) readIn(
	ctx context.Context, cmd MoveContainerCommand,
) (domain.Container, domain.Container, error) {
	container, err := findContainer(ctx, h.Writer.Containers, cmd.ContainerID)
	if err != nil {
		return domain.Container{}, domain.Container{}, err
	}

	if cmd.TargetParentID == cmd.ContainerID {
		// Reading it a second time would answer with the container itself, and MovedInto would refuse
		// it - but naming what is wrong with the request is this layer's job, and the read is wasted.
		return container, container, nil
	}

	target, err := h.Writer.Containers.Find(ctx, cmd.TargetParentID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The same answer whether it does not exist or belongs to another tenant
			// (multi-tenancy.md §2).
			return domain.Container{}, domain.Container{}, shared.ErrNotFound.
				WithDetail("containers.parent_not_found").
				WithParams(map[string]string{"parent_id": cmd.TargetParentID.String()})
		}
		return domain.Container{}, domain.Container{}, err
	}
	return container, target, nil
}

func (h MoveContainer) authorize(
	ctx context.Context, actor appshared.ActorContext, on domain.Container, targetID shared.ID,
) error {
	return h.Writer.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(on),
		Action:     ContainerMovedAction,
		TokenScope: containersWrite,
		TargetType: containerTarget,
		TargetID:   targetID,
	})
}

// Descriptor is the catalogue entry.
func (h MoveContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: MoveContainerName,
		Summary: "Moves a collection into another hub and ranks it there. Naming the hub it already " +
			"sits in reorders it. The name has to be free at the destination; a collision is refused " +
			"rather than resolved. A hub is the top level and cannot be moved. Idempotent: a move " +
			"that asks for where the collection already sits writes nothing.",
		SideEffects: "Writes the placement, announces " + string(event.ContainerMoved) +
			", records one change per field for offline clients, and writes an audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The collection to move.",
			},
			{
				Name: "target_parent_id", Kind: usecase.KindID, Required: true,
				Description: "The hub it moves into.",
			},
			{
				Name: "before_container_id", Kind: usecase.KindID,
				Description: "The sibling to place it before at the destination. Omitted appends to " +
					"the end of the level.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read. Omitted means the caller read none and accepts " +
					"whatever is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerMovedAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a container is not an item, and the history is an item's: `ActivityEntry` is " +
				"keyed on `itemId` (domain-model.md §3.5) and `/items/{id}/activity` is its only " +
				"reader. A container's own history has nowhere to be read from yet.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h MoveContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}
	targetParentID, err := in.ID("target_parent_id")
	if err != nil {
		return nil, err
	}
	beforeID, err := in.ID("before_container_id")
	if err != nil {
		return nil, err
	}

	container, err := h.Execute(ctx, actor, MoveContainerCommand{
		ContainerID:       containerID,
		TargetParentID:    targetParentID,
		BeforeContainerID: beforeID,
		ExpectedVersion:   in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}
