// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// PreviewContainerMoveName is the use case's name.
const PreviewContainerMoveName = "PreviewContainerMove"

// MovePermits answers a permission without recording a refusal: a destination the caller may not
// structure is an obstacle the dialog shows, not somebody trying anything.
type MovePermits interface {
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// MoveFinding is one obstacle or consequence: a message code and its parameters (rule 8).
type MoveFinding struct {
	Code   string
	Params map[string]string
}

// MovePreview is what a move would meet: whether it can be made, what stops it, what changes.
type MovePreview struct {
	Obstacles    []MoveFinding
	Consequences []MoveFinding
}

// errPreviewOnly rolls back the trial placement the preview writes.
var errPreviewOnly = errors.New("a preview writes nothing")

// PreviewContainerMove answers what MoveContainer would do with the same body, writing nothing
// (UC-WRK-39 checks 1-4): its obstacles carry the codes the move refuses with, so the dialog and a
// refusal say the same thing (P-06).
//
// The placement is written in a transaction that is rolled back: the unique index is what
// decides a name taken, and who loses access is asked of the tree as it would stand.
type PreviewContainerMove struct {
	Move      MoveContainer
	Permits   MovePermits
	Leftovers repository.MoveLeftovers
}

// Execute previews the move.
func (h PreviewContainerMove) Execute(ctx context.Context, actor appshared.ActorContext, cmd MoveContainerCommand) (MovePreview, error) {
	if cmd.ContainerID.IsZero() {
		return MovePreview{}, containerIDRequired()
	}
	if cmd.TargetParentID.IsZero() {
		return MovePreview{}, shared.ErrValidation.WithDetail("containers.target_parent_required").
			WithFields(shared.FieldError{Path: "/target_parent_id", Code: "containers.target_parent_required"})
	}
	container, target, err := h.Move.read(ctx, actor, cmd)
	if err != nil {
		return MovePreview{}, err
	}
	// The source's STRUCTURE is the preview's own question: whoever may not change the collection's
	// hub learns nothing about what a move would do there.
	if err := h.Move.authorize(ctx, actor, container, cmd.ContainerID); err != nil {
		return MovePreview{}, err
	}
	var preview MovePreview
	obstacle := func(err error) {
		problem := shared.AsError(err)
		finding := MoveFinding{Code: problem.Code, Params: problem.Params}
		if problem.DetailCode != "" {
			finding.Code = problem.DetailCode
		}
		preview.Obstacles = append(preview.Obstacles, finding)
	}
	if h.Permits != nil && target.ID != container.ParentID {
		allowed, err := h.Permits.Permits(ctx, actor, access.Request{
			Permission: service.PermissionStructure, Path: containerPath(target),
			Action: ContainerMovedAction, TokenScope: containersWrite, TargetType: containerTarget, TargetID: cmd.ContainerID,
		})
		if err != nil {
			return MovePreview{}, err
		}
		if !allowed {
			preview.Obstacles = append(preview.Obstacles, MoveFinding{
				Code: "access.not_permitted", Params: map[string]string{"permission": string(service.PermissionStructure), "hub_id": target.ID.String()},
			})
		}
	}
	err = h.Move.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		fresh, destination, err := h.Move.readIn(ctx, cmd)
		if err != nil {
			return err
		}
		orderKey, err := h.Move.rankAt(ctx, cmd, destination)
		if err != nil {
			return err
		}
		wanted, changes, err := fresh.MovedInto(destination, orderKey, h.Move.Writer.Clock.Now())
		if err != nil {
			// A hub that cannot move, a collection under itself: nothing below is asked.
			obstacle(err)
			return errPreviewOnly
		}
		leaving := fresh.ParentID != destination.ID
		if h.Move.EntryTypes != nil && leaving {
			bound, err := h.Move.EntryTypes.OfHubInCollection(ctx, fresh.ID, fresh.ParentID)
			if err != nil {
				return err
			}
			if bound {
				obstacle(hubBound())
			}
		}
		if err := keepsHubWorkflows(ctx, h.Move.Workflows, fresh.ID, fresh.ParentID, destination.ID); err != nil {
			if shared.AsError(err) == nil || shared.AsError(err).Category == shared.CategoryUnavailable {
				return err
			}
			obstacle(err)
		}
		if !leaving || len(changes) == 0 {
			return errPreviewOnly
		}
		preview.Consequences = append(preview.Consequences, MoveFinding{
			Code: "containers.move_takes_structure", Params: map[string]string{"hub_id": destination.ID.String()},
		})
		if err := h.leftovers(ctx, fresh, &preview); err != nil {
			return err
		}
		// The trial placement: a name taken comes back from the unique index, as the move's does.
		if err := h.Move.Writer.Containers.SetPlacement(ctx, wanted, fresh.Version); err != nil {
			if problem := shared.AsError(err); problem != nil && problem.DetailCode == "containers.name_taken" {
				obstacle(err)
				return errPreviewOnly
			}
			return err
		}
		if h.Move.Revocations != nil {
			loss, err := h.Move.Revocations.AfterContainerMoved(ctx, fresh, wanted)
			if err != nil {
				return err
			}
			if len(loss.Accounts) > 0 {
				ids := make([]string, 0, len(loss.Accounts))
				for _, id := range loss.Accounts {
					ids = append(ids, id.String())
				}
				preview.Consequences = append(preview.Consequences, MoveFinding{
					Code:   "containers.move_loses_access",
					Params: map[string]string{"count": strconv.Itoa(len(ids)), "account_ids": joinIDs(ids)},
				})
			}
		}
		return errPreviewOnly
	})
	if err != nil && !errors.Is(err, errPreviewOnly) {
		return MovePreview{}, err
	}
	return preview, nil
}

// leftovers names what stays pointing at the hub the collection leaves: entries in its releases
// and clocks measured by its SLA policies. The move refuses neither; the person is told.
func (h PreviewContainerMove) leftovers(ctx context.Context, collection domain.Container, preview *MovePreview) error {
	if h.Leftovers == nil {
		return nil
	}
	releases, err := h.Leftovers.KeptReleaseEntries(ctx, collection.ID, collection.ParentID)
	if err != nil {
		return err
	}
	if releases > 0 {
		preview.Consequences = append(preview.Consequences, MoveFinding{
			Code: "containers.move_keeps_releases", Params: map[string]string{"count": strconv.Itoa(releases), "hub_id": collection.ParentID.String()},
		})
	}
	clocks, err := h.Leftovers.KeptSLAClocks(ctx, collection.ID, collection.ParentID)
	if err != nil {
		return err
	}
	if clocks > 0 {
		preview.Consequences = append(preview.Consequences, MoveFinding{
			Code: "containers.move_keeps_sla_policies", Params: map[string]string{"count": strconv.Itoa(clocks), "hub_id": collection.ParentID.String()},
		})
	}
	return nil
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += id
	}
	return out
}

// Descriptor is the catalogue entry.
func (h PreviewContainerMove) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       PreviewContainerMoveName,
		Permission: service.PermissionStructure,
		Summary: "Answers what moving a collection to a hub would meet, writing nothing: whether it can " +
			"be made, its obstacles - the codes the move refuses with, a destination without STRUCTURE " +
			"among them - and its consequences: who loses read access, the destination's structure, " +
			"entries kept in the old hub's releases and clocks kept on its SLA policies.",
		SideEffects: "None. Reads only.",
		TokenScope:  containersWrite,
		ReadOnly:    true,
		Input:       MoveContainer{}.Descriptor().Input,
		Audit: usecase.AuditDeclaration{
			Action: ContainerMovedAction, TargetType: containerTarget, Required: false,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a preview writes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids := map[string]shared.ID{}
			for _, field := range []string{"container_id", "target_parent_id", "before_container_id"} {
				id, err := in.ID(field)
				if err != nil {
					return nil, err
				}
				ids[field] = id
			}
			preview, err := h.Execute(ctx, actor, MoveContainerCommand{
				ContainerID: ids["container_id"], TargetParentID: ids["target_parent_id"],
				BeforeContainerID: ids["before_container_id"], ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"movable":      len(preview.Obstacles) == 0,
				"obstacles":    findingsOutput(preview.Obstacles),
				"consequences": findingsOutput(preview.Consequences),
			}, nil
		}),
	}
}

func findingsOutput(findings []MoveFinding) []usecase.Output {
	out := make([]usecase.Output, 0, len(findings))
	for _, finding := range findings {
		params := map[string]any{}
		for k, v := range finding.Params {
			params[k] = v
		}
		out = append(out, usecase.Output{"code": finding.Code, "params": params})
	}
	return out
}
