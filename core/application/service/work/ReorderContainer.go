// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
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

// ReorderContainerName is the catalogue name.
const ReorderContainerName = "ReorderContainer"

// ReorderContainer ranks a container within the level it already sits in.
//
// It exists because MoveContainer cannot express the case that matters most in a sidebar. A move
// requires a `target_parent_id` and ranks the collection there as a consequence - naming the hub a
// collection is already in reorders it, which the operation says itself. A **hub** sits in the
// tenant and in nothing else (I-C1), so it has no parent to name, while `domain-model.md` §3.3
// defines `order_key` as "ordering within the parent context" and every Container answers one. The
// rank of a hub was therefore a field the API reported and nothing could change, and a sidebar
// listed hubs in an order their owner could not touch.
//
// The audit action and the event are MoveContainer's rather than new ones. `container.moved` is
// already documented as covering "a change of hub and a reorder within one", and ContainerMoved
// already says a consumer recognises a reorder by `from_parent_id` matching `parent_id`. A second
// name for the same fact would be a second thing for every consumer to learn and for the change
// log to keep in step.
type ReorderContainer struct {
	Writer ContainerWriter
}

// ReorderContainerCommand is the input, typed.
type ReorderContainerCommand struct {
	ContainerID shared.ID
	// BeforeContainerID is the sibling to land in front of. Empty appends to the end of the level.
	BeforeContainerID shared.ID
	ExpectedVersion   int
}

// Execute ranks the container and returns it as it now stands.
func (h ReorderContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ReorderContainerCommand,
) (domain.Container, error) {
	if cmd.ContainerID.IsZero() {
		return domain.Container{}, containerIDRequired()
	}

	container, err := h.read(ctx, actor, cmd)
	if err != nil {
		return domain.Container{}, err
	}

	// One permission question, and before the transaction: a refusal writes an audit entry, and an
	// entry written inside the transaction would be rolled back with the refusal (audit.md §7).
	//
	// One rather than the two a move asks, because nothing leaves a level here. The container is
	// where it was and stays there; only its rank among its own siblings changes.
	if err := h.authorize(ctx, actor, container); err != nil {
		return domain.Container{}, err
	}

	var ranked domain.Container
	err = h.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Writer.Clock.Now()

		// Read again inside the transaction: the level can have changed since the rank was decided
		// from it, and the neighbour the caller named may have left it.
		fresh, err := findContainer(ctx, h.Writer.Containers, cmd.ContainerID)
		if err != nil {
			return err
		}

		orderKey, err := h.rankIn(ctx, cmd, fresh)
		if err != nil {
			return err
		}

		wanted, changes, err := fresh.Ranked(orderKey, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// It already holds the rank the caller asked for. Nothing is written, no version is
			// spent and nothing is announced.
			if err := ensureContainerVersion(fresh, cmd.ExpectedVersion); err != nil {
				return err
			}
			ranked = fresh
			return nil
		}

		ranked, err = h.write(ctx, actor, fresh, wanted, changes, cmd.ExpectedVersion, now)
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return ranked, nil
}

// write stores the rank and records what it owes: the event outwards, the change log for offline
// clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (h ReorderContainer) write(
	ctx context.Context, actor appshared.ActorContext, before, after domain.Container,
	changes []domain.FieldChange, expectedVersion int, now time.Time,
) (domain.Container, error) {
	expected := expectedVersion
	if expected == 0 {
		expected = before.Version
	}
	// SetRank and not SetPlacement: a reorder changes the rank and nothing else, and the placement
	// statement writes a `parent_id` that a hub does not have.
	if err := h.Writer.Containers.SetRank(ctx, after, expected); err != nil {
		return domain.Container{}, err
	}
	after.Version = expected + 1

	// The parent it came from is the parent it still has, which is exactly how a consumer of
	// ContainerMoved recognises a reorder. For a hub both are absent, and that is the same fact.
	announcement, err := event.NewContainerMoved(
		h.Writer.IDs.NewID(), after, after.ParentID,
		event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return domain.Container{}, err
	}
	if err := h.Writer.Events.Append(ctx, announcement); err != nil {
		return domain.Container{}, err
	}
	// `order_key` is a fractional index and merges by itself, which is the whole reason the rank is
	// a key rather than a number (offline-sync.md §4.2).
	if err := h.Writer.recordChanges(ctx, after, actor, changes); err != nil {
		return domain.Container{}, err
	}
	// A rank and an identifier, so all of it is OPEN in the data catalogue's sense. No name: user
	// content stays out of the trail (rule 10), and "who reordered this" needs none of it.
	if err := h.Writer.recordAudit(
		ctx, after, actor, ContainerMovedAction, audit.Open, changes, now); err != nil {
		return domain.Container{}, err
	}
	return after, nil
}

// rankIn works out the rank the container takes among its own siblings.
//
// The level is the container's own parent, which for a hub is the empty identifier - the port reads
// that as the hub level, so the same call serves both kinds and neither has a branch of its own.
func (h ReorderContainer) rankIn(
	ctx context.Context, cmd ReorderContainerCommand, container domain.Container,
) (string, error) {
	previous, next, err := h.Writer.Containers.Neighbours(
		ctx, container.ParentID, cmd.BeforeContainerID, container.ID)
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

// read resolves the container outside the write transaction, because the permission question needs
// it first.
func (h ReorderContainer) read(
	ctx context.Context, actor appshared.ActorContext, cmd ReorderContainerCommand,
) (domain.Container, error) {
	var container domain.Container

	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			var err error
			container, err = findContainer(ctx, h.Writer.Containers, cmd.ContainerID)
			return err
		})
	if err != nil {
		return domain.Container{}, err
	}
	return container, nil
}

func (h ReorderContainer) authorize(
	ctx context.Context, actor appshared.ActorContext, on domain.Container,
) error {
	return h.Writer.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(on),
		Action:     ContainerMovedAction,
		TokenScope: containersWrite,
		TargetType: containerTarget,
		TargetID:   on.ID,
	})
}

// Descriptor is the catalogue entry.
func (h ReorderContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReorderContainerName,
		Summary: "Ranks a container within the level it already sits in: a hub among the tenant's " +
			"hubs, a collection among its hub's collections. The only way to rank a hub, which " +
			"sits in nothing and so cannot be moved. The rank is a fractional index, so an " +
			"insertion between two neighbours renumbers nothing else. Idempotent: asking for the " +
			"position a container already holds writes nothing.",
		SideEffects: "Writes the rank, announces " + string(event.ContainerMoved) +
			" with the parent unchanged, records the change for offline clients, and writes an " +
			"audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The container to rank.",
			},
			{
				Name: "before_container_id", Kind: usecase.KindID,
				Description: "The sibling to place it before at its own level. Omitted appends to " +
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

func (h ReorderContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}
	beforeID, err := in.ID("before_container_id")
	if err != nil {
		return nil, err
	}

	container, err := h.Execute(ctx, actor, ReorderContainerCommand{
		ContainerID:       containerID,
		BeforeContainerID: beforeID,
		ExpectedVersion:   in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}
