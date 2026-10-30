// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
)

// The entity mutations (offline-sync.md §4.4): a device's offline write of the workspace's
// vocabulary - a type, a link type, a link, a workflow's draft, a custom role - performed through
// the use cases a person would call, under the merge rule its entity declares.

// entityWriters names the use cases that write one entity. A table in code, for the appliers'
// reason: a stored use case name would be a stored capability.
type entityWriters struct {
	create, change, remove string
	// key is the name the create and change inputs give the entity's key; removeKey the delete's.
	// Empty for an entity whose key the use cases do not read: the caller's own timer.
	key, removeKey string
	// patch is what a patch performs where it is not the change: a timer's patch stops it.
	patch string
	// createOnly are what only a creation sets. An upsert of an entity that exists drops them, and
	// the server's value stands: a level, a direction, a cardinality is not changed by a second
	// device that happened to create the same key.
	createOnly []string
}

// writersOf is every entity a push writes. A workflow is written to its draft: publishing moves
// entries between statuses and is decided online, against the board as it is.
func writersOf(entity domain.Entity) (entityWriters, bool) {
	writers, found := map[domain.Entity]entityWriters{
		domain.EntityEntryType: {
			create: "CreateEntryType", change: "ChangeEntryType", remove: "DeleteEntryType",
			key: "key", removeKey: "key", createOnly: []string{"level"},
		},
		domain.EntityLinkType: {
			create: "CreateLinkType", change: "ChangeLinkType", remove: "DeleteLinkType",
			key: "key", removeKey: "key", createOnly: []string{"direction", "cardinality", "cycles", "waits"},
		},
		domain.EntityWorkflow: {
			create: "CreateWorkflow", change: "SaveWorkflowDraft", remove: "DeleteWorkflow",
			key: "key", removeKey: "key",
		},
		domain.EntityCustomRole: {
			create: "CreateRole", change: "ChangeRole", remove: "DeleteRole",
			key: "key", removeKey: "key",
		},
		domain.EntityEntryLink: {
			create: "LinkEntries", remove: "UnlinkEntries", key: "id", removeKey: "link_id",
		},
		// A start, a stop and a discard queued offline, at the device's moments (UC-TIM-01).
		domain.EntityTimer: {
			create: "StartTimer", patch: "StopTimer", remove: "DiscardTimer",
		},
	}[entity]
	return writers, found
}

// judgeEntity answers what the shape of an entity mutation says: an entity the contract does not
// name, a missing key, a patch of a presence. Each is a refusal worth recording.
func judgeEntity(m Mutation) error {
	if !m.Entity.Valid() {
		return shared.ErrValidation.
			WithDetail("sync.entity_unknown").
			WithParams(map[string]string{"entity": string(m.Entity)}).
			WithFields(shared.FieldError{Path: "/entity", Code: "sync.entity_unknown"})
	}
	if m.EntityKey == "" {
		return shared.ErrValidation.
			WithDetail("sync.entity_key_required").
			WithFields(shared.FieldError{Path: "/entity_key", Code: "sync.entity_key_required"})
	}
	if !m.Entity.Rule().Admits(m.Kind) {
		return shared.ErrValidation.
			WithDetail("sync.entity_not_patchable").
			WithParams(map[string]string{"entity": string(m.Entity)}).
			WithFields(shared.FieldError{Path: "/kind", Code: "sync.entity_not_patchable"})
	}
	return nil
}

// entity applies one entity mutation.
func (p PushChanges) entity(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	result, err := p.applyEntity(ctx, actor, m)
	if err != nil || m.Entity != domain.EntityTimer {
		return result, err
	}
	// The timer is the caller's own, held under their account as the walk hands it out: a start
	// answers it running, a stop or a discard answers nothing - the record a stop wrote and the
	// timer's removal arrive with the stream.
	result.EntityID = actor.AccountID
	if m.Kind != domain.EntityUpsert {
		result.ServerState = nil
	}
	return result, nil
}

func (p PushChanges) applyEntity(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	writers, found := writersOf(m.Entity)
	if !found {
		return Result{}, judgeEntity(m)
	}
	switch m.Kind {
	case domain.EntityDelete:
		return p.removeEntity(ctx, actor, m, writers)
	case domain.EntityPatch:
		change := writers.change
		if writers.patch != "" {
			change = writers.patch
		}
		out, err := p.Catalogue.Invoke(ctx, change, actor, entityInput(m, writers, true))
		if err != nil {
			return Result{}, err
		}
		return entityApplied(m, out), nil
	default:
		return p.upsertEntity(ctx, actor, m, writers)
	}
}

// upsertEntity creates the entity, or changes it when the server holds it already. The change is
// tried first: a creation of a key that exists fails in the database and takes the transaction
// with it, while a change of one that does not is a plain "not found".
func (p PushChanges) upsertEntity(
	ctx context.Context, actor appshared.ActorContext, m Mutation, writers entityWriters,
) (Result, error) {
	if writers.change != "" {
		out, err := p.Catalogue.Invoke(ctx, writers.change, actor, entityInput(m, writers, true))
		if err == nil {
			return entityApplied(m, out), nil
		}
		if shared.AsError(err).Category != shared.CategoryNotFound {
			return Result{}, err
		}
	}
	out, err := p.Catalogue.Invoke(ctx, writers.create, actor, entityInput(m, writers, false))
	if err != nil {
		return Result{}, err
	}
	return entityApplied(m, out), nil
}

// removeEntity deletes the entity. One already gone is applied under either rule: what the device
// wanted is true, and a refusal would hold a mutation in its queue that no later push can apply. A
// deletion is not versioned: the use case's own guards - a type in use, a system type - decide.
func (p PushChanges) removeEntity(
	ctx context.Context, actor appshared.ActorContext, m Mutation, writers entityWriters,
) (Result, error) {
	in := usecase.Input{}
	if writers.removeKey != "" {
		in[writers.removeKey] = m.EntityKey
	}
	_, err := p.Catalogue.Invoke(ctx, writers.remove, actor, in)
	if err != nil && shared.AsError(err).Category != shared.CategoryNotFound {
		return Result{}, err
	}
	result := Result{OpID: m.OpID, Result: domain.Applied}
	if m.Entity == domain.EntityEntryLink {
		result.EntityID = shared.ID(m.EntityKey)
	}
	return result, nil
}

// entityInput is the payload as the use case's input, with the mutation's key written after it so
// that the payload cannot move it. A change carries the base version as the expected one - the
// versioned rule - and leaves out what only a creation sets.
func entityInput(m Mutation, writers entityWriters, change bool) usecase.Input {
	in := inputOf(m.Payload)
	delete(in, "expected_version")
	if change {
		for _, field := range writers.createOnly {
			delete(in, field)
		}
		if m.BaseVersion != nil && *m.BaseVersion > 0 {
			in["expected_version"] = *m.BaseVersion
		}
	}
	if writers.key != "" {
		in[writers.key] = m.EntityKey
	}
	return in
}

// entityApplied is an applied entity mutation, answered with the entity as the server now holds it.
func entityApplied(m Mutation, out usecase.Output) Result {
	result := Result{OpID: m.OpID, Result: domain.Applied, ServerState: out}
	if id, ok := out["id"].(string); ok {
		result.EntityID = shared.ID(id)
	} else if m.Entity == domain.EntityEntryLink {
		result.EntityID = shared.ID(m.EntityKey)
	}
	return result
}
