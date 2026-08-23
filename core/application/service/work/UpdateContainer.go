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
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	RenameContainerName         = "RenameContainer"
	UpdateContainerPoliciesName = "UpdateContainerPolicies"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	//
	// Two codes rather than one, because the two answer different questions of the trail. "Who
	// changed how this collection works" is an administrative question and "who renamed it" is not,
	// and an auditor filtering on one must not have to read the change list of the other to find out
	// which it is looking at.
	ContainerRenamedAction         audit.Action = "container.renamed"
	ContainerPoliciesUpdatedAction audit.Action = "container.policies_updated"
)

// ContainerWriter is what every use case that changes an existing container shares.
//
// One dependency set rather than one per verb: they read the same container, ask the same
// permission question, and owe the same four writes - the row, the event, the change log entry and
// the audit entry. What differs between them is which domain method decides the new state.
type ContainerWriter struct {
	Containers repository.Containers
	// Policies is where the auto_assign key of the policies document is stored: its own row, so
	// the rotation's state can be locked (see work.AutoAssignPolicy). Only UpdateContainerPolicies
	// writes it, for the reason only TrashContainer uses Queue - the dependency set is shared, the
	// verbs are not.
	Policies   repository.AutoAssignPolicies
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
	// Queue is where a deletion asks for the tenant's cleanup to be scheduled. Only TrashContainer
	// uses it; the reasoning is at LifecycleWriter.Queue.
	Queue queue.Queue
}

// RenameContainer changes a hub or a collection's own descriptive fields.
//
// What it does not change: how the collection works (UpdateContainerPolicies), where it sits
// (MoveContainer), and whether it is writable at all (ArchiveContainer). A single endpoint writing
// all of them would need one audit entry covering everything and one event nobody could subscribe
// to narrowly.
type RenameContainer struct {
	Writer ContainerWriter
}

// UpdateContainerPolicies replaces a collection's policies.
type UpdateContainerPolicies struct {
	Writer ContainerWriter
}

// RenameContainerCommand is the input, typed.
type RenameContainerCommand struct {
	ContainerID shared.ID
	// Attributes carries a pointer per field, so that "set it to nothing" and "do not touch it" stay
	// two different requests all the way down from the merge patch that expressed them.
	Attributes domain.ContainerAttributes
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read none
	// and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// UpdateContainerPoliciesCommand is the input, typed.
type UpdateContainerPoliciesCommand struct {
	ContainerID shared.ID
	// Policies is the whole document rather than the keys that were sent. This is a PUT: a key the
	// caller omitted is the default, not what happens to be stored.
	Policies        domain.ContainerPolicies
	ExpectedVersion int
}

// Execute renames the container and returns it as it now stands.
func (h RenameContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd RenameContainerCommand,
) (domain.Container, error) {
	return h.Writer.change(ctx, actor, containerChange{
		containerID:     cmd.ContainerID,
		action:          ContainerRenamedAction,
		expectedVersion: cmd.ExpectedVersion,
		apply: func(container domain.Container, now time.Time) (domain.Container, []domain.FieldChange, error) {
			return container.Renamed(cmd.Attributes, now)
		},
		store: repository.Containers.SetAttributes,
		announce: func(id shared.ID, container domain.Container, changes []domain.FieldChange,
			by event.Actor, at time.Time,
		) (event.Envelope, error) {
			return event.NewContainerRenamed(id, container, changes, by, at, event.Cause{})
		},
		// Name, description, icon and colour are all user content, so the trail records that they
		// changed and a hash of each side rather than the values (audit.md §4). The entry outlives
		// the container by design, and a name kept in clear text here would be a copy that no
		// deletion ever reaches - while "who renamed this, and when" is answerable without it
		// (rule 10, ADR-0017, ADR-0018).
		classification: audit.Sensitive,
	})
}

// Execute replaces the policies and returns the collection as it now stands.
func (h UpdateContainerPolicies) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateContainerPoliciesCommand,
) (domain.Container, error) {
	return h.Writer.change(ctx, actor, containerChange{
		containerID:     cmd.ContainerID,
		action:          ContainerPoliciesUpdatedAction,
		expectedVersion: cmd.ExpectedVersion,
		apply: func(container domain.Container, now time.Time) (domain.Container, []domain.FieldChange, error) {
			return container.WithPolicies(cmd.Policies, now)
		},
		// The document's two keys live in two places: the completion policy in the container's
		// column, the assignment policy in its own row (work.AutoAssignPolicy says why). One
		// store writes both inside the caller's transaction, so the document can never half-move.
		store: func(repo repository.Containers, ctx context.Context, container domain.Container, expected int) error {
			if err := repo.SetPolicies(ctx, container, expected); err != nil {
				return err
			}
			return h.storeAutoAssign(ctx, container)
		},
		announce: func(id shared.ID, container domain.Container, changes []domain.FieldChange,
			by event.Actor, at time.Time,
		) (event.Envelope, error) {
			return event.NewContainerPoliciesUpdated(id, container, changes, by, at, event.Cause{})
		},
		// A policy is a closed set of values this installation defined, not something a person typed.
		// It is recorded in clear text because an auditor asking "when did this collection start
		// rolling up" has no other way to answer it, and there is no personal data in "ROLLUP".
		classification: audit.Open,
	})
}

// storeAutoAssign brings the policy row in line with the document that was just written: a
// definition is upserted, an absent key deletes the row. The identifier is minted fresh on every
// write and used only when the scope has no row yet - the upsert keeps an existing row's identity,
// so nothing that recorded it is severed by a reconfiguration.
func (h UpdateContainerPolicies) storeAutoAssign(
	ctx context.Context, container domain.Container,
) error {
	if container.AutoAssign == nil {
		return h.Writer.Policies.Delete(ctx, domain.AutoAssignScopeCollection, container.ID)
	}
	return h.Writer.Policies.Upsert(ctx, domain.AutoAssignPolicy{
		ID:         h.Writer.IDs.NewID(),
		TenantID:   container.TenantID,
		ScopeType:  domain.AutoAssignScopeCollection,
		ScopeID:    container.ID,
		Strategy:   container.AutoAssign.Strategy,
		Candidates: container.AutoAssign.Candidates,
		Enabled:    container.AutoAssign.Enabled,
	})
}

// containerChange is one verb's differences from the others: what it applies, what it stores, what
// it announces, and how sensitive what it changed is.
type containerChange struct {
	containerID     shared.ID
	action          audit.Action
	expectedVersion int
	apply           func(domain.Container, time.Time) (domain.Container, []domain.FieldChange, error)
	store           func(repository.Containers, context.Context, domain.Container, int) error
	announce        func(shared.ID, domain.Container, []domain.FieldChange, event.Actor, time.Time) (event.Envelope, error)
	classification  audit.Classification
}

// change is the whole of what a change to an existing container owes, once.
func (w ContainerWriter) change(
	ctx context.Context, actor appshared.ActorContext, change containerChange,
) (domain.Container, error) {
	if change.containerID.IsZero() {
		return domain.Container{}, containerIDRequired()
	}

	// The container is read before the permission question, because the answer depends on it: a
	// membership held at the hub applies downwards, so a path naming only the collection would refuse
	// somebody who does hold the right (domain-model.md §3.2). Nothing read here is trusted
	// afterwards - the state that decides the write is read again inside the transaction.
	current, err := w.read(ctx, actor, change.containerID)
	if err != nil {
		return domain.Container{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(current),
		Action:     change.action,
		TokenScope: containersWrite,
		TargetType: containerTarget,
		TargetID:   change.containerID,
	}); err != nil {
		return domain.Container{}, err
	}

	var updated domain.Container
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		container, err := findContainer(ctx, w.Containers, change.containerID)
		if err != nil {
			return err
		}

		wanted, changes, err := change.apply(container, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// The container already says what the caller asked it to say. Nothing is written, no
			// version is spent and nothing is announced - which is what makes a client that echoes the
			// whole object back harmless rather than merely accepted.
			//
			// The If-Match is still honoured: a caller writing against a version somebody else has
			// moved on is told so even when its own change would have been a no-op, because the state
			// it was reasoning about is not the state that is there.
			if err := ensureContainerVersion(container, change.expectedVersion); err != nil {
				return err
			}
			updated = container
			return nil
		}

		updated, err = w.write(ctx, actor, change, wanted, changes, container.Version, now)
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return updated, nil
}

// write stores the change and records what it owes: the event outwards, the change log for offline
// clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (w ContainerWriter) write(
	ctx context.Context, actor appshared.ActorContext, change containerChange,
	after domain.Container, changes []domain.FieldChange, currentVersion int, now time.Time,
) (domain.Container, error) {
	expected := change.expectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still the one the update matches on, so a concurrent write
		// between the read and here is still caught.
		expected = currentVersion
	}
	if err := change.store(w.Containers, ctx, after, expected); err != nil {
		return domain.Container{}, err
	}
	after.Version = expected + 1

	// Built from the stored state rather than from the command, so that what the event says and what
	// the row holds cannot disagree.
	announcement, err := change.announce(
		w.IDs.NewID(), after, changes, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now)
	if err != nil {
		return domain.Container{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return domain.Container{}, err
	}
	if err := w.recordChanges(ctx, after, actor, changes); err != nil {
		return domain.Container{}, err
	}
	if err := w.recordAudit(ctx, after, actor, change.action, change.classification, changes, now); err != nil {
		return domain.Container{}, err
	}
	return after, nil
}

// recordChanges writes what an offline client has to be told: one entry per field that moved.
//
// One entry per field rather than one carrying them all, because the merge rule for these fields is
// last writer wins *per field* (offline-sync.md §4.2). Each entry takes its own HLC, so a device
// that renamed a collection while another set its colour keeps both changes - which is precisely
// what one entry covering both would destroy, the later HLC deciding the whole payload and silently
// discarding the other device's field.
//
// The payload names only the field that moved. `version` and `updated_at` are derived and never
// merged, and a payload that repeated the untouched fields would let a stale value for one of them
// win a merge it should never have been in.
//
// A cleared field travels as null rather than as the empty string. Every field a container change
// touches holds "not set" as the empty string in the domain, and the API spells that null - a client
// merging `""` into a timestamp or an icon would be merging a value this system never renders.
func (w ContainerWriter) recordChanges(
	ctx context.Context, container domain.Container, actor appshared.ActorContext,
	changes []domain.FieldChange,
) error {
	for _, change := range changes {
		err := w.Changes.Record(ctx, changelog.Change{
			TenantID: container.TenantID,
			Entity:   containerTarget,
			EntityID: container.ID,
			Op:       changelog.Upsert,
			// The visibility filter a pull applies. For a collection that is the hub above it, so a
			// device subscribed to the hub sees the change (offline-sync.md §3.1).
			ContainerID: firstNonZero(container.ParentID, container.ID),
			ActorID:     actor.AccountID,
			HLC:         w.HLC.Next(),
			Payload:     map[string]any{change.Field: clearedAsNull(change.To)},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// clearedAsNull is how "the field is now empty" reaches a payload: as null, which is what the API
// spells it.
func clearedAsNull(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// recordAudit writes the evidence: which fields changed, and - where they are not user content -
// what they now say.
func (w ContainerWriter) recordAudit(
	ctx context.Context, container domain.Container, actor appshared.ActorContext,
	action audit.Action, classification audit.Classification,
	changes []domain.FieldChange, now time.Time,
) error {
	recorded := make([]audit.Change, 0, len(changes)+1)
	for _, moved := range changes {
		recorded = append(recorded, audit.Change{
			Field: moved.Field, Classification: classification,
			From: moved.From, To: moved.To,
		})
	}
	recorded = append(recorded,
		audit.Change{Field: "type", Classification: audit.Open, To: string(container.Type)})

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   container.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: containerTarget,
		TargetID:   container.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(recorded...),
	})
}

// read reads the container outside the write transaction, because the permission check needs it
// first. Read-only, so it may be served by a replica (multi-tenancy.md §7).
func (w ContainerWriter) read(
	ctx context.Context, actor appshared.ActorContext, containerID shared.ID,
) (domain.Container, error) {
	var container domain.Container

	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findContainer(ctx, w.Containers, containerID)
		container = found
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return container, nil
}

// findContainer reads a container a client named, or says it does not exist in the words a client
// can act on. Distinct from findCollection, which reads the collection *under* an item that
// exists - where a missing row is a defect rather than a client's mistake.
func findContainer(
	ctx context.Context, containers repository.Containers, id shared.ID,
) (domain.Container, error) {
	container, err := containers.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The same answer whether it does not exist or belongs to another tenant. Anything else
			// would confirm the existence of another tenant's data (multi-tenancy.md §2).
			return domain.Container{}, shared.ErrNotFound.
				WithDetail("containers.not_found").
				WithParams(map[string]string{"container_id": id.String()})
		}
		return domain.Container{}, err
	}
	return container, nil
}

func containerIDRequired() error {
	return shared.ErrValidation.
		WithDetail("containers.container_id_required").
		WithFields(shared.FieldError{Path: "/container_id", Code: "containers.container_id_required"})
}

// ensureContainerVersion refuses a caller writing against a version that has moved on, even when
// the change it asked for would have been a no-op. Zero means the caller read no version and
// accepts whatever is there (api-guidelines.md §5).
func ensureContainerVersion(container domain.Container, expected int) error {
	if expected == 0 || expected == container.Version {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("containers.version_conflict").
		WithParams(map[string]string{
			"container_id": container.ID.String(), "current_version": strconv.Itoa(container.Version),
		})
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h RenameContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RenameContainerName,
		Summary: "Changes a hub or collection's own descriptive fields: its name, description, icon " +
			"and colour. A field that is not sent is left alone; sending one as empty clears it. The " +
			"name stays unique among the containers at the same level. Idempotent: an update that " +
			"asks for what is already stored succeeds, writes nothing and announces nothing.",
		SideEffects: "Writes the changed fields, announces " + string(event.ContainerRenamed) +
			" with a change set, records one change per field for offline clients, and writes an audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The hub or collection to change.",
			},
			{
				Name: "name", Kind: usecase.KindString,
				Description: "The new name: one line, at most 200 characters, not empty, and free at " +
					"this level. Omitted leaves the name as it is.",
			},
			{
				Name: "description", Kind: usecase.KindString,
				Description: "The new description. Empty clears it, omitted leaves it as it is.",
			},
			{
				Name: "icon", Kind: usecase.KindString,
				Description: "The new icon. Empty clears it, omitted leaves it as it is.",
			},
			{
				Name: "color_token", Kind: usecase.KindString,
				Description: "A theme token rather than a colour value, so clients render it in their " +
					"own palette. Empty clears it, omitted leaves it as it is.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, from the If-Match header over REST. Omitted means " +
					"the caller read none and accepts whatever is there; a version that has moved on " +
					"since is refused rather than overwritten.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerRenamedAction, TargetType: containerTarget,
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

// invoke is the adapter between the catalogue's untyped input and the typed command, for all three
// channels at once.
func (h RenameContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}

	// OptionalString rather than String, because the difference between the two is this use case's
	// whole contract: a caller that sent no `icon` wants it left alone, and one that sent an empty
	// `icon` wants it gone. A channel that could not say which would clear the icon of every client
	// that only meant to rename something.
	cmd := RenameContainerCommand{
		ContainerID: containerID,
		Attributes: domain.ContainerAttributes{
			Name:        in.OptionalString("name"),
			Description: in.OptionalString("description"),
			Icon:        in.OptionalString("icon"),
			ColorToken:  in.OptionalString("color_token"),
		},
		ExpectedVersion: in.Int("expected_version"),
	}
	if cmd.Attributes.IsEmpty() {
		return nil, shared.ErrValidation.
			WithDetail("containers.update_empty").
			WithFields(shared.FieldError{Path: "/", Code: "containers.update_empty"})
	}

	container, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}

// Descriptor is the catalogue entry.
func (h UpdateContainerPolicies) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateContainerPoliciesName,
		Summary: "Replaces a collection's policies - how it works, as opposed to what it is called. " +
			"A key that is not sent falls back to its default rather than keeping the stored value: " +
			"this replaces the document. A hub carries no policies and is refused. Idempotent: " +
			"writing what is already stored succeeds, writes nothing and announces nothing.",
		SideEffects: "Writes the policies, announces " + string(event.ContainerPoliciesUpdated) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The collection to configure.",
			},
			{
				Name: "completion_policy", Kind: usecase.KindString,
				Enum: []string{string(domain.CompletionManual), string(domain.CompletionRollup)},
				Description: "MANUAL leaves an item's parent alone when the item is completed. ROLLUP " +
					"completes the parent when its last open child is completed, and reopens it when " +
					"any child is reopened. Omitted means MANUAL, which is the default.",
			},
			{
				Name: "auto_assign", Kind: usecase.KindObject,
				Description: "How what is created in this collection is handed out, as " +
					"{strategy, candidates, enabled}. strategy is FIXED, RANDOM_MEMBER, " +
					"RANDOM_GROUP_MEMBER, ROUND_ROBIN or LEAST_LOADED; candidates is the ordered " +
					"pool as [{kind: ACCOUNT|GROUP, id}] - groups for RANDOM_GROUP_MEMBER, one " +
					"account for FIXED, accounts otherwise; enabled (default true) makes the " +
					"policy apply to everything created here, while a disabled one waits for a " +
					"create that asks with auto_assign. Omitted or null means no automatic " +
					"assignment.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, from the If-Match header over REST. Omitted means " +
					"the caller read none and accepts whatever is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerPoliciesUpdatedAction, TargetType: containerTarget,
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

func (h UpdateContainerPolicies) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}

	// String rather than OptionalString, and that is the difference from a rename: an absent key here
	// is the default rather than "leave it alone", because this replaces the document.
	autoAssign, err := domain.ParseAutoAssignDefinition(in["auto_assign"])
	if err != nil {
		return nil, err
	}
	container, err := h.Execute(ctx, actor, UpdateContainerPoliciesCommand{
		ContainerID: containerID,
		Policies: domain.ContainerPolicies{
			CompletionPolicy: domain.CompletionPolicy(in.String("completion_policy")),
			AutoAssign:       autoAssign,
		},
		ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}
