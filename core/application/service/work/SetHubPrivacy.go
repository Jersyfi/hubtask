// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	SetHubPrivacyName = "SetHubPrivacy"

	// HubPrivacyChangedAction is the audit code: who made a hub private, or shared again. Its own
	// code, because "who shut the administrators out of this hub" is a question an access review
	// asks on its own.
	HubPrivacyChangedAction audit.Action = "container.privacy_changed"
)

// PrivacyRevocations is the slice of access.Revocations a hub turning private needs: who held it
// through the workspace, and the record telling their devices (offline-sync.md §6).
type PrivacyRevocations interface {
	AfterHubMadePrivate(ctx context.Context, hub domain.Container) (access.Loss, error)
	Announce(ctx context.Context, tenantID shared.ID, losses ...access.Loss) error
}

// SetHubPrivacy makes an existing hub private, or shared again (ADR-0073 §2).
//
// Marking needs both the right to create hubs - STRUCTURE on the workspace - and an OWNER
// membership on the hub itself; a role held on the workspace does not make anybody the owner of a
// hub, so an administrator cannot take a shared hub away from everybody else. Unmarking needs an
// OWNER on the hub. A person without STRUCTURE has a private hub by creating one.
type SetHubPrivacy struct {
	Writer ContainerWriter
	// Hubs takes the hub's row before anything is decided, as granting a group does: a group
	// cannot slip in while the hub turns private (identity.md §22).
	Hubs        identityrepository.HubLocks
	Revocations PrivacyRevocations
}

// SetHubPrivacyCommand is the input, typed.
type SetHubPrivacyCommand struct {
	ContainerID     shared.ID
	Private         bool
	ExpectedVersion int
}

// Execute changes the hub's privacy and returns it as it now stands.
func (h SetHubPrivacy) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SetHubPrivacyCommand,
) (domain.Container, error) {
	w := h.Writer
	if cmd.ContainerID.IsZero() {
		return domain.Container{}, containerIDRequired()
	}
	current, err := w.read(ctx, actor, cmd.ContainerID)
	if err != nil {
		return domain.Container{}, err
	}
	if current.Type != domain.ContainerHub {
		return domain.Container{}, domain.ErrPrivateOnlyHubs()
	}
	if err := h.authorize(ctx, actor, current, cmd.Private); err != nil {
		return domain.Container{}, err
	}

	var updated domain.Container
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		if _, _, err := h.Hubs.LockHubOf(ctx, cmd.ContainerID); err != nil {
			return err
		}
		container, err := findContainer(ctx, w.Containers, cmd.ContainerID)
		if err != nil {
			return err
		}
		if cmd.Private && !container.Private {
			held, err := h.Hubs.GroupHoldsRoleUnder(ctx, container.ID)
			if err != nil {
				return err
			}
			if held {
				return shared.ErrConflict.
					WithDetail("containers.private_hub_has_groups").
					WithParams(map[string]string{"container_id": container.ID.String()})
			}
		}

		wanted, changes, err := container.WithPrivacy(cmd.Private, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			if err := ensureContainerVersion(container, cmd.ExpectedVersion); err != nil {
				return err
			}
			updated = container
			return nil
		}

		updated, err = w.write(ctx, actor, privacyChange(cmd), wanted, changes, container.Version, now)
		if err != nil {
			return err
		}
		if !cmd.Private {
			return nil
		}
		// In the same transaction, after the write: the resolution then sees the hub private, and
		// asks every workspace role whether it still reads it.
		loss, err := h.Revocations.AfterHubMadePrivate(ctx, updated)
		if err != nil {
			return err
		}
		return h.Revocations.Announce(ctx, updated.TenantID, loss)
	})
	if err != nil {
		return domain.Container{}, err
	}
	return updated, nil
}

// authorize asks what ADR-0073 §2 asks, before the transaction so that a refusal is recorded.
//
// OWNER on the hub itself is asked as DELETE_CONTAINER along the hub alone: OWNER is the one role
// carrying it, and a path without the workspace leaves the workspace's roles out of the answer.
func (h SetHubPrivacy) authorize(
	ctx context.Context, actor appshared.ActorContext, hub domain.Container, private bool,
) error {
	request := access.Request{
		Action: HubPrivacyChangedAction, TokenScope: containersWrite,
		TargetType: containerTarget, TargetID: hub.ID,
	}
	if private {
		structure := request
		structure.Permission = service.PermissionStructure
		structure.Path = []identity.Scope{identity.TenantScope()}
		if err := h.Writer.Authorizer.Authorize(ctx, actor, structure); err != nil {
			return err
		}
	}
	owner := request
	owner.Permission = service.PermissionDeleteContainer
	owner.Path = []identity.Scope{identity.HubScope(hub.ID)}
	return h.Writer.Authorizer.Authorize(ctx, actor, owner)
}

// privacyChange is how the shared writer stores and announces this change: the privacy column,
// and container.policies_updated with `private` in its change set - privacy is how the hub works
// for others, the policies event's subject, and no new event type is needed for one field.
func privacyChange(cmd SetHubPrivacyCommand) containerChange {
	return containerChange{
		containerID:     cmd.ContainerID,
		action:          HubPrivacyChangedAction,
		expectedVersion: cmd.ExpectedVersion,
		store: func(repo repository.Containers, ctx context.Context, container domain.Container, expected int) error {
			return repo.SetPrivate(ctx, container, expected)
		},
		announce: func(id shared.ID, container domain.Container, changes []domain.FieldChange,
			by event.Actor, at time.Time,
		) (event.Envelope, error) {
			return event.NewContainerPoliciesUpdated(id, container, changes, by, at, event.Cause{})
		},
		// A flag, not anything a person typed.
		classification: audit.Open,
	}
}

// Descriptor is the catalogue entry.
func (h SetHubPrivacy) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetHubPrivacyName,
		Summary: "Makes a hub private - reached only by people holding a role on it or below it, " +
			"the workspace's owners and administrators included - or shared again. Marking needs " +
			"the right to create hubs and OWNER on the hub itself; unmarking needs OWNER on the " +
			"hub. Refused while a group holds a role in the hub, and on a collection. Idempotent.",
		SideEffects: "Writes the flag, announces " + string(event.ContainerPoliciesUpdated) +
			", records a change for offline clients and, when the hub turns private, an " +
			"ACCESS_REVOKED record for every device that loses it; writes an audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The hub.",
			},
			{
				Name: "private", Kind: usecase.KindBool, Required: true,
				Description: "true to make the hub private, false to share it again.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The version last read, from the If-Match header over REST. Omitted means " +
					"the caller read none and accepts whatever is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: HubPrivacyChangedAction, TargetType: containerTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a container is not an item, and the history is an item's: `ActivityEntry` is " +
				"keyed on `itemId` and `/items/{id}/activity` is its only reader. A container's " +
				"own history has nowhere to be read from.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SetHubPrivacy) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}
	container, err := h.Execute(ctx, actor, SetHubPrivacyCommand{
		ContainerID: containerID, Private: in.Bool("private"),
		ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}
