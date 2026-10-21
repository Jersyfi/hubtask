// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The whole object, in the contract's shape, for a device starting from nothing
// (offline-sync.md §3.1). The same builders every channel answers with, exported under one name
// each so that the initial synchronisation and a read over the API cannot describe one object in
// two vocabularies - a field a device learned from a snapshot is a field the delta later moves
// under the same name.

// ContainerSnapshot is a hub or a collection as `GET /containers/{id}` answers it.
func ContainerSnapshot(container domain.Container) usecase.Output { return containerOutput(container) }

// ContainerSnapshots are containers as a read answers them, each with its structure in force,
// resolved in the caller's transaction. Without a wired resolver they carry none.
func ContainerSnapshots(
	ctx context.Context, structure StructureResolver, containers []domain.Container,
) ([]usecase.Output, error) {
	reading := structure.within(repository.LayerRead)
	payloads := make([]usecase.Output, 0, len(containers))
	for _, container := range containers {
		out := containerOutput(container)
		if structure.Wired() {
			effective, err := reading.Of(ctx, container)
			if err != nil {
				return nil, err
			}
			out["structure"] = effectiveStructureOutput(effective)
		}
		payloads = append(payloads, out)
	}
	return payloads, nil
}

// BucketSnapshot is a column as the board lists it.
func BucketSnapshot(bucket domain.Bucket) usecase.Output { return bucketOutput(bucket) }

// LabelSnapshot is a label as the collection lists it.
func LabelSnapshot(label domain.Label) usecase.Output { return labelOutput(label) }

// ItemSnapshot is an entry as `GET /items/{id}` answers it.
func ItemSnapshot(item domain.WorkItem) usecase.Output { return ItemOutput(item) }

// CommentSnapshot is a comment as the thread lists it.
func CommentSnapshot(comment domain.Comment) usecase.Output { return commentOutput(comment) }

// ReminderSnapshot is a reminder as the entry lists it.
func ReminderSnapshot(reminder domain.Reminder) usecase.Output { return reminderOutput(reminder) }

// RecurrenceSnapshot is a series definition as `GET /items/{id}/recurrence` answers it.
func RecurrenceSnapshot(rule domain.RecurrenceRule) usecase.Output { return recurrenceOutput(rule) }

// TemplateSnapshot is a template as `GET /templates/{id}` answers it.
func TemplateSnapshot(template domain.Template) usecase.Output { return templateOutput(template) }
