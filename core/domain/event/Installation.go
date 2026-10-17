// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The installation's events: what a provider's purchase platform learns about a workspace
// (UC-INS-14, multi-tenancy.md §4.3). A list of their own rather than members of `types`: a
// workspace's webhook subscribes to `types`, and a workspace must never subscribe to what the
// installation tells its platform about every workspace.
const (
	WorkspaceCreated           Type = "de.hubtask.installation.workspace.created.v1"
	WorkspacePlanChanged       Type = "de.hubtask.installation.workspace.plan_changed.v1"
	WorkspaceSuspended         Type = "de.hubtask.installation.workspace.suspended.v1"
	WorkspaceResumed           Type = "de.hubtask.installation.workspace.resumed.v1"
	WorkspaceDeletionRequested Type = "de.hubtask.installation.workspace.deletion_requested.v1"
	WorkspaceDeletionCancelled Type = "de.hubtask.installation.workspace.deletion_cancelled.v1"
	WorkspaceDeleted           Type = "de.hubtask.installation.workspace.deleted.v1"
	WorkspaceOwnerChanged      Type = "de.hubtask.installation.workspace.owner_changed.v1"
	WorkspaceLimitReached      Type = "de.hubtask.installation.workspace.limit_reached.v1"
	WorkspaceAiBudgetSpent     Type = "de.hubtask.installation.workspace.ai_budget_spent.v1"
	WorkspaceExportFinished    Type = "de.hubtask.installation.workspace.export_finished.v1"
)

var installationTypes = [...]Type{
	WorkspaceCreated, WorkspacePlanChanged, WorkspaceSuspended, WorkspaceResumed,
	WorkspaceDeletionRequested, WorkspaceDeletionCancelled, WorkspaceDeleted,
	WorkspaceOwnerChanged, WorkspaceLimitReached, WorkspaceAiBudgetSpent, WorkspaceExportFinished,
}

// InstallationTypes returns every installation event type.
func InstallationTypes() []Type { return installationTypes[:] }

// Installation reports whether the type is one of the installation's.
func (t Type) Installation() bool {
	for _, known := range installationTypes {
		if known == t {
			return true
		}
	}
	return false
}

// WorkspaceDeletedStatus is the status a `deleted` event reports: the row it would be read from is
// gone, and a receiver keeping the newest state needs a value that says so.
const WorkspaceDeletedStatus = "DELETED"

// WorkspaceState is what every installation event carries about its workspace: identifiers and
// states, never content (rule 10). No slug and no display name - a slug is often a family's name.
// A receiver keeps the event with the highest version and needs no ordering (UC-INS-14 check 2).
type WorkspaceState struct {
	ID shared.ID
	// ExternalReference is the platform's own reference, empty when none is set and after the
	// purge, which removes it.
	ExternalReference string
	Status            string
	// Plan is the plan's short name, empty without a plan.
	Plan    string
	Version int64
}

func (s WorkspaceState) payload() map[string]any {
	return map[string]any{
		"id":                 s.ID.String(),
		"external_reference": nullable(s.ExternalReference),
		"status":             s.Status,
		"plan":               nullable(s.Plan),
		"version":            s.Version,
	}
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// NewWorkspaceEvent builds an installation event about one workspace. `extra` holds the event's
// own fields beside `workspace` - identifiers, codes and moments, never text a person wrote.
//
// The envelope's tenant is the workspace the event is about, which is what the CloudEvent's
// `tenantid` reports; the event belongs to the installation all the same, and is stored and read
// there.
func NewWorkspaceEvent(id shared.ID, eventType Type, state WorkspaceState, actor Actor,
	occurredAt time.Time, extra map[string]any,
) (Envelope, error) {
	switch {
	case id.IsZero(), state.ID.IsZero(), state.Status == "", occurredAt.IsZero():
		return Envelope{}, shared.ErrInternal.WithDetail("events.envelope_incomplete")
	case !eventType.Installation():
		return Envelope{}, shared.ErrInternal.
			WithDetail("events.type_unknown").
			WithParams(map[string]string{"type": string(eventType)})
	}

	payload := map[string]any{"workspace": state.payload()}
	for key, value := range extra {
		if key == "workspace" {
			continue
		}
		payload[key] = value
	}

	return Envelope{
		ID:            id,
		Type:          eventType,
		TenantID:      state.ID,
		Subject:       "workspace/" + state.ID.String(),
		Actor:         actor,
		OccurredAt:    occurredAt.UTC(),
		ReceivedAt:    occurredAt.UTC(),
		CorrelationID: id,
		Payload:       payload,
	}, nil
}
