// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strconv"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	TrashContainerName   = "TrashContainer"
	RestoreContainerName = "RestoreContainer"

	// The audit codes. `.deleted` rather than `.trashed`, as domain-model.md §4 names the container
	// half of this - the item half is `.trashed` there, and the names are kept as the document
	// writes them rather than harmonised afterwards.
	ContainerDeletedAction  audit.Action = "container.deleted"
	ContainerRestoredAction audit.Action = "container.restored"
)

// TrashContainer moves a hub or a collection and everything under it into the trash.
//
// The whole subtree, under one batch: a hub takes its collections, and every collection takes its
// entries (I-C2). That is what makes the way back a single decision rather than a walk that has to
// guess what belonged together - and it is why this is the one operation an administrator may not
// perform. Deleting a container takes work nobody was asked about with it, so it is the owner's
// decision (domain-model.md §3.2).
type TrashContainer struct {
	Writer ContainerWriter
}

// RestoreContainer takes one container deletion back out of the trash, whole.
//
// The same permission as the deletion. Whoever owns the decision to delete a hub owns its reversal:
// an administrator who may not delete one may not bring one back either, or an owner's deletion
// would be undoable by somebody the matrix deliberately excluded from making it.
type RestoreContainer struct {
	Writer ContainerWriter
}

// ContainerLifecycleCommand is the input both verbs take.
type ContainerLifecycleCommand struct {
	ContainerID shared.ID
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read none
	// and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute puts the container and its subtree into the trash and returns it as it now stands.
func (h TrashContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ContainerLifecycleCommand,
) (domain.Container, error) {
	return h.Writer.moveThroughTheTrash(ctx, actor, cmd, deletingContainer)
}

// Execute restores the deletion the container belongs to and returns it as it now stands.
func (h RestoreContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ContainerLifecycleCommand,
) (domain.Container, error) {
	return h.Writer.moveThroughTheTrash(ctx, actor, cmd, restoringContainer)
}

// containerVerb is one direction through the trash, as the writer needs to know it.
type containerVerb struct {
	action audit.Action
	// entering says which way this goes. It is read for the two things that genuinely differ - which
	// identifier the batch is, and which repository call runs - rather than passed as three closures
	// that would each have to agree about it.
	entering bool
	announce func(shared.ID, domain.Container, event.Cascade, event.Actor, time.Time) (event.Envelope, error)
	// op is what an offline client is told about each container of the batch: a deletion for one it
	// should drop, an upsert for one that is back (offline-sync.md §3.1).
	op changelog.Operation
}

var (
	deletingContainer = containerVerb{
		action:   ContainerDeletedAction,
		entering: true,
		announce: announceCascade(event.NewContainerDeleted),
		op:       changelog.Delete,
	}
	restoringContainer = containerVerb{
		action:   ContainerRestoredAction,
		announce: announceCascade(event.NewContainerRestored),
		op:       changelog.Upsert,
	}
)

// announceCascade drops the causation argument the writer has nothing to put in.
//
// These are things a person did, so the chain starts here; a lifecycle change caused by another
// event will pass its own when there is one to pass (automation.md §2).
func announceCascade(
	build func(shared.ID, domain.Container, event.Cascade, event.Actor, time.Time, event.Cause) (event.Envelope, error),
) func(shared.ID, domain.Container, event.Cascade, event.Actor, time.Time) (event.Envelope, error) {
	return func(id shared.ID, container domain.Container, cascade event.Cascade,
		actor event.Actor, at time.Time,
	) (event.Envelope, error) {
		return build(id, container, cascade, actor, at, event.Cause{})
	}
}

// moveThroughTheTrash is the whole of both verbs.
//
// It does not go through ContainerWriter.change, and the reason is the cascade: that helper writes
// one row and announces one thing, while this writes two tables and announces one event per
// container the act covered. Forcing both through one shape would mean a helper whose every step
// had a branch in it.
func (w ContainerWriter) moveThroughTheTrash(
	ctx context.Context, actor appshared.ActorContext, cmd ContainerLifecycleCommand,
	verb containerVerb,
) (domain.Container, error) {
	if cmd.ContainerID.IsZero() {
		return domain.Container{}, containerIDRequired()
	}

	// The container is read before the permission question, because the answer depends on it: a
	// membership held at the hub applies downwards, so a path naming only the collection would
	// refuse somebody who does hold the right (domain-model.md §3.2). Nothing read here is trusted
	// afterwards - the state that decides the write is read again inside the transaction.
	current, err := w.read(ctx, actor, cmd.ContainerID)
	if err != nil {
		return domain.Container{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       containerPath(current),
		Action:     verb.action,
		TokenScope: containersWrite,
		TargetType: containerTarget,
		TargetID:   cmd.ContainerID,
	}); err != nil {
		return domain.Container{}, err
	}

	var changed domain.Container
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		container, err := findContainer(ctx, w.Containers, cmd.ContainerID)
		if err != nil {
			return err
		}

		// A fresh identifier on the way in, from the generator port rather than from time or chance
		// (arc42 §8.13); the one already on the row on the way out, read before the transition
		// clears it. Restoring is an act on the deletion, and the deletion is what the batch names.
		batch := container.TrashBatchID
		if verb.entering {
			batch = w.IDs.NewID()
		}

		wanted, changes, err := container.Trashed(now, batch)
		if !verb.entering {
			wanted, changes, err = container.Restored(now)
		}
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// The container already says what the caller asked it to say. Nothing is written, no
			// version is spent and nothing is announced - which is what makes a retry after a lost
			// response harmless rather than merely accepted.
			//
			// The If-Match is still honoured: a caller writing against a version somebody else has
			// moved on is told so even when its own change would have been a no-op.
			if err := ensureContainerVersion(container, cmd.ExpectedVersion); err != nil {
				return err
			}
			changed = container
			return nil
		}

		changed, err = w.writeTrash(ctx, actor, verb, repository.ContainerTrash{
			Container: wanted, BatchID: batch, ExpectedVersion: cmd.ExpectedVersion,
		}, container.Version, now)
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return changed, nil
}

// writeTrash stores the cascade and records what it owes: the event outwards, one change log entry
// per container the act covered, and the audit entry - all inside the caller's transaction
// (test AT-5).
func (w ContainerWriter) writeTrash(
	ctx context.Context, actor appshared.ActorContext, verb containerVerb,
	trash repository.ContainerTrash, currentVersion int, now time.Time,
) (domain.Container, error) {
	if trash.ExpectedVersion == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still the one the update matches on, so a concurrent write
		// between the read and here is still caught.
		trash.ExpectedVersion = currentVersion
	}

	var cascade repository.Cascade
	var err error
	if verb.entering {
		cascade, err = w.Containers.TrashSubtree(ctx, trash)
	} else {
		cascade, err = w.Containers.RestoreBatch(ctx, trash)
	}
	if err != nil {
		return domain.Container{}, err
	}

	after := trash.Container
	after.Version = trash.ExpectedVersion + 1

	// Built from the stored state rather than from the command, so that what the event says and what
	// the row holds cannot disagree.
	announcement, err := verb.announce(w.IDs.NewID(), after, event.Cascade{
		Collections: len(cascade.Collections), Items: cascade.Items,
	}, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now)
	if err != nil {
		return domain.Container{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return domain.Container{}, err
	}
	if err := w.recordTrashChanges(ctx, after, cascade, actor, verb); err != nil {
		return domain.Container{}, err
	}
	if err := w.recordTrashAudit(ctx, after, cascade, actor, verb, now); err != nil {
		return domain.Container{}, err
	}
	if verb.entering {
		// The clock starts here, so the sweep that will read it is asked for here. The reasoning is
		// at LifecycleWriter.Queue; a restore takes things off the clock and asks for nothing.
		if err := scheduleRetention(ctx, w.Queue, after.TenantID); err != nil {
			return domain.Container{}, err
		}
	}
	return after, nil
}

// recordTrashChanges writes what offline clients have to be told (offline-sync.md §3.1).
//
// One entry for the container itself and one for each collection the act covered - and nothing for
// the entries below them. The split is not arbitrary: the change log's visibility filter is the
// container a device subscribes to, so a device that follows one collection rather than the hub
// above it would never see a change filed under the hub alone. The entries need no separate entry
// for the same reason a subtree deletion needs none: a client that holds a collection drops what is
// in it when the collection goes, and one entry per row would put a hub's two hundred entries into
// the log for one act.
//
// A deletion carries no payload. There is nothing left to describe, and a tombstone with content
// would be a copy of the deleted container living on in the log.
func (w ContainerWriter) recordTrashChanges(
	ctx context.Context, container domain.Container, cascade repository.Cascade,
	actor appshared.ActorContext, verb containerVerb,
) error {
	covered := append([]shared.ID{container.ID}, cascade.Collections...)
	for _, id := range covered {
		// The visibility filter a pull applies. For a collection that is the hub above it; for the
		// container itself, itself - which is what a device subscribed to the hub reads.
		scope := id
		if id == container.ID && !container.ParentID.IsZero() {
			scope = container.ParentID
		}

		err := w.Changes.Record(ctx, changelog.Change{
			TenantID:    container.TenantID,
			Entity:      containerTarget,
			EntityID:    id,
			Op:          verb.op,
			ContainerID: scope,
			ActorID:     actor.AccountID,
			HLC:         w.HLC.Next(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// recordTrashAudit writes the evidence.
//
// The size of what went travels with it. "Somebody deleted a hub" and "somebody deleted a hub with
// four collections and eight hundred entries in it" are different events to whoever has to answer
// for the second, and an auditor should not have to reconstruct that from the entries themselves.
// No name and no title: user content stays out of the trail (rule 10).
func (w ContainerWriter) recordTrashAudit(
	ctx context.Context, container domain.Container, cascade repository.Cascade,
	actor appshared.ActorContext, verb containerVerb, now time.Time,
) error {
	deletedAt := ""
	if container.DeletedAt != nil {
		deletedAt = container.DeletedAt.UTC().Format(time.RFC3339Nano)
	}

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   container.TenantID,
		OccurredAt: now,
		Action:     verb.action,
		Outcome:    audit.OutcomeSuccess,
		// A notice rather than an info: this is the one act that takes work nobody was asked about
		// with it, and a trail an operator skims should not bury it among the renames (audit.md §2).
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: containerTarget,
		TargetID:   container.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "type", Classification: audit.Open, To: string(container.Type)},
			audit.Change{Field: domain.FieldDeletedAt, Classification: audit.Open, To: deletedAt},
			audit.Change{
				Field: "collections", Classification: audit.Open,
				To: strconv.Itoa(len(cascade.Collections)),
			},
			audit.Change{
				Field: "items", Classification: audit.Open, To: strconv.Itoa(cascade.Items),
			},
		),
	})
}

// Descriptor is the catalogue entry.
func (h TrashContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: TrashContainerName,
		Summary: "Moves a hub or a collection and everything under it into the trash: a hub takes " +
			"its collections, and every collection takes its entries. A soft delete - it all stays " +
			"for the tenant's retention period and comes back as one act until the retention job " +
			"removes it for good. Anything already in the trash from an earlier deletion keeps that " +
			"deletion rather than joining this one. Needs the owner's right to delete a container. " +
			"Idempotent.",
		SideEffects: "Writes the deletion stamp over the subtree in both tables, announces " +
			string(event.ContainerDeleted) + ", records a deletion for offline clients per " +
			"container covered, and writes an audit entry.",
		TokenScope:  containersWrite,
		Destructive: true,
		Input:       containerLifecycleInput("The hub or collection to move to the trash."),
		Audit: usecase.AuditDeclaration{
			Action: ContainerDeletedAction, TargetType: containerTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RestoreContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RestoreContainerName,
		Summary: "Takes one container deletion back out of the trash, whole. Exactly what went in " +
			"together comes back: a separate, younger deletion inside the same subtree stays where " +
			"it is. A container archived when it was deleted comes back archived. Needs the same " +
			"right as the deletion. Idempotent.",
		SideEffects: "Clears the deletion stamp over the batch in both tables, announces " +
			string(event.ContainerRestored) + ", records a change for offline clients per container " +
			"covered, and writes an audit entry.",
		TokenScope: containersWrite,
		Input:      containerLifecycleInput("The hub or collection whose deletion is to be reversed."),
		Audit: usecase.AuditDeclaration{
			Action: ContainerRestoredAction, TargetType: containerTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// containerLifecycleInput is the input both verbs declare. One list, because they take the same
// thing and a client that learned one from /meta/capabilities should not find the other spelled
// differently.
func containerLifecycleInput(description string) []usecase.Field {
	return []usecase.Field{
		{Name: "container_id", Kind: usecase.KindID, Required: true, Description: description},
		{
			Name: "expected_version", Kind: usecase.KindInt,
			Description: "The version last read, from the If-Match header over REST. Omitted means " +
				"the caller read none and accepts whatever is there; a version that has moved on " +
				"since is refused rather than overwritten.",
		},
	}
}

func (h TrashContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return invokeContainerLifecycle(ctx, actor, in, h.Execute)
}

func (h RestoreContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	return invokeContainerLifecycle(ctx, actor, in, h.Execute)
}

// invokeContainerLifecycle is the adapter between the catalogue's untyped input and the typed
// command, for all three channels at once.
func invokeContainerLifecycle(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
	execute func(context.Context, appshared.ActorContext, ContainerLifecycleCommand) (domain.Container, error),
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}
	container, err := execute(ctx, actor, ContainerLifecycleCommand{
		ContainerID: containerID, ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}
