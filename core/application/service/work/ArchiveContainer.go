// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ArchiveContainerName   = "ArchiveContainer"
	UnarchiveContainerName = "UnarchiveContainer"

	// The audit codes. Two, not one: an auditor asking what was taken out of use must not have to
	// read a change list to find out whether the entry means it went or came back (audit.md §2).
	ContainerArchivedAction   audit.Action = "container.archived"
	ContainerUnarchivedAction audit.Action = "container.unarchived"
)

// ArchiveContainer makes a hub or a collection read-only, and everything under it with it.
//
// Nothing below is written. A collection inherits an archived hub's state through the read rather
// than through a stamp of its own (I-C3), so archiving a hub costs one row however many collections
// sit in it - and unarchiving it restores exactly what it covered, leaving a collection archived in
// its own right archived.
type ArchiveContainer struct {
	Writer ContainerWriter
}

// UnarchiveContainer makes a hub or a collection writable again.
type UnarchiveContainer struct {
	Writer ContainerWriter
}

// ArchiveContainerCommand is the input, typed. It is the same shape for both verbs: an identifier,
// and the version the caller read.
type ArchiveContainerCommand struct {
	ContainerID     shared.ID
	ExpectedVersion int
}

// Execute archives the container and returns it as it now stands.
func (h ArchiveContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ArchiveContainerCommand,
) (domain.Container, error) {
	return h.Writer.change(ctx, actor, containerChange{
		containerID:     cmd.ContainerID,
		action:          ContainerArchivedAction,
		expectedVersion: cmd.ExpectedVersion,
		apply: func(container domain.Container, now time.Time) (domain.Container, []domain.FieldChange, error) {
			return container.Archived(now)
		},
		store: repository.Containers.SetArchived,
		announce: func(id shared.ID, container domain.Container, _ []domain.FieldChange,
			by event.Actor, at time.Time,
		) (event.Envelope, error) {
			// A snapshot without a change set, as domain-model.md §4 gives this event: what changed is
			// the lifecycle itself, and `archived_at` in the snapshot already says so.
			return event.NewContainerArchived(id, container, by, at, event.Cause{})
		},
		// A timestamp is not user content, so it goes into the trail as it stands. "When was this
		// taken out of use" is the question the entry exists to answer.
		classification: audit.Open,
	})
}

// Execute unarchives the container and returns it as it now stands.
func (h UnarchiveContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ArchiveContainerCommand,
) (domain.Container, error) {
	return h.Writer.change(ctx, actor, containerChange{
		containerID:     cmd.ContainerID,
		action:          ContainerUnarchivedAction,
		expectedVersion: cmd.ExpectedVersion,
		apply: func(container domain.Container, now time.Time) (domain.Container, []domain.FieldChange, error) {
			return container.Unarchived(now)
		},
		store: repository.Containers.SetArchived,
		announce: func(id shared.ID, container domain.Container, _ []domain.FieldChange,
			by event.Actor, at time.Time,
		) (event.Envelope, error) {
			return event.NewContainerUnarchived(id, container, by, at, event.Cause{})
		},
		classification: audit.Open,
	})
}

// Descriptor is the catalogue entry.
func (h ArchiveContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ArchiveContainerName,
		Summary: "Archives a hub or a collection: it becomes read-only, and everything under it " +
			"inherits that. A collection in an archived hub refuses writes without being archived " +
			"itself, and reports effective_archived to say so. Idempotent: archiving an archived " +
			"container succeeds and writes nothing.",
		SideEffects: "Writes the archive stamp, announces " + string(event.ContainerArchived) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The hub or collection to archive.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read. Omitted means the caller read none and accepts " +
					"whatever is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerArchivedAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ArchiveContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := archiveCommand(in)
	if err != nil {
		return nil, err
	}
	container, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}

// Descriptor is the catalogue entry.
func (h UnarchiveContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UnarchiveContainerName,
		Summary: "Makes an archived hub or collection writable again. Only its own archiving is " +
			"lifted: a collection archived in its own right stays archived when its hub is " +
			"unarchived, because unarchiving restores what was archived and not what was merely " +
			"covered by it. Idempotent.",
		SideEffects: "Clears the archive stamp, announces " + string(event.ContainerUnarchived) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The hub or collection to unarchive.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read. Omitted means the caller read none and accepts " +
					"whatever is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerUnarchivedAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UnarchiveContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := archiveCommand(in)
	if err != nil {
		return nil, err
	}
	container, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}

// archiveCommand is the mapping both verbs share. One implementation, because two copies of three
// lines is two places for the field name to be misspelled.
func archiveCommand(in usecase.Input) (ArchiveContainerCommand, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return ArchiveContainerCommand{}, err
	}
	return ArchiveContainerCommand{
		ContainerID: containerID, ExpectedVersion: in.Int("expected_version"),
	}, nil
}
