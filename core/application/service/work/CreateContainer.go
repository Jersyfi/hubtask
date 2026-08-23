// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package work holds the work management use cases: containers and the items inside them.
package work

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
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
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The identity of this use case, in the one place that decides it. The registry derives the REST
// operation, the MCP tool and the automation action from the name (usecase.Descriptor).
const (
	CreateContainerName = "CreateContainer"
	containerTarget     = "container"
	containersWrite     = "containers:write"

	// ContainerCreatedAction is the audit code. Stable: an auditor filters on it and a SIEM rule
	// matches on it (audit.md §2).
	ContainerCreatedAction audit.Action = "container.created"
)

// Authorizer is the slice of the authorisation service this use case needs. An interface rather
// than the service, so the use case can be tested without a membership table.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// CreateContainerCommand is the input, typed. The registry hands the use case an untyped map from
// whichever channel the call arrived through; this is what that map becomes once, here.
type CreateContainerCommand struct {
	Type     domain.ContainerType
	ParentID shared.ID
	Name     string

	Description string
	Icon        string
	ColorToken  string
}

// CreateContainer creates a hub or a collection.
//
// It is the reference use case of this milestone: everything a write in this system owes is
// visible in one Execute - the permission check before the transaction, the invariants in the
// domain, and inside one transaction the row, the event, the change log entry for offline clients
// and the audit entry. A later use case that leaves one of them out is visibly different from
// this one, which is the point of having a reference at all.
type CreateContainer struct {
	Containers repository.Containers
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// Execute creates the container and returns it.
func (h CreateContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateContainerCommand,
) (domain.Container, error) {
	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       authorizationPath(cmd.ParentID),
		Action:     ContainerCreatedAction,
		TokenScope: containersWrite,
		TargetType: containerTarget,
		// The container does not exist yet, so the refusal names what it would have been created
		// in. For a hub that is the tenant itself, which the entry already carries.
		TargetID: cmd.ParentID,
	}); err != nil {
		return domain.Container{}, err
	}

	var created domain.Container
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.checkParent(ctx, cmd); err != nil {
			return err
		}

		orderKey, err := h.nextOrderKey(ctx, cmd.ParentID)
		if err != nil {
			return err
		}

		now := h.Clock.Now()
		container, err := domain.NewContainer(domain.NewContainerInput{
			ID:          h.IDs.NewID(),
			TenantID:    actor.TenantID,
			Type:        cmd.Type,
			ParentID:    cmd.ParentID,
			Name:        cmd.Name,
			Description: cmd.Description,
			Icon:        cmd.Icon,
			ColorToken:  cmd.ColorToken,
			OrderKey:    orderKey,
			CreatedBy:   actor.AccountID,
			Now:         now,
		})
		if err != nil {
			return err
		}

		if err := h.Containers.Insert(ctx, container); err != nil {
			return err
		}

		// One snapshot, two recipients. The event goes outwards as a public contract, the change
		// log goes to synchronising clients (offline-sync.md §10) - but they describe the same
		// state, and building it twice is how the two come to disagree.
		announcement, err := event.NewContainerCreated(
			h.IDs.NewID(), container, event.Actor{Kind: actor.Kind, ID: actor.AccountID},
			now, event.Cause{})
		if err != nil {
			return err
		}
		if err := h.Events.Append(ctx, announcement); err != nil {
			return err
		}
		if err := h.recordChange(ctx, container, actor, announcement.Payload); err != nil {
			return err
		}
		if err := h.recordAudit(ctx, container, actor, now); err != nil {
			return err
		}

		created = container
		return nil
	})
	if err != nil {
		return domain.Container{}, err
	}
	return created, nil
}

// checkParent enforces I-C1 and I-C3 against the container the new one would sit in.
func (h CreateContainer) checkParent(ctx context.Context, cmd CreateContainerCommand) error {
	if cmd.ParentID.IsZero() {
		// A hub sits in the tenant, which needs no lookup. That a hub may not name a parent is
		// the constructor's business, and it is checked there.
		return nil
	}

	parent, err := h.Containers.Find(ctx, cmd.ParentID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The same answer whether it does not exist or belongs to another tenant. Anything
			// else would confirm the existence of another tenant's data (multi-tenancy.md §2).
			return shared.ErrNotFound.
				WithDetail("containers.parent_not_found").
				WithParams(map[string]string{"parent_id": cmd.ParentID.String()})
		}
		return err
	}
	return parent.EnsureAcceptsChildren(cmd.Type)
}

// nextOrderKey ranks the new container after its last sibling.
func (h CreateContainer) nextOrderKey(ctx context.Context, parentID shared.ID) (string, error) {
	last, err := h.Containers.LastOrderKey(ctx, parentID)
	if err != nil {
		return "", err
	}
	return service.OrderKeyAfter(last)
}

// recordChange writes what an offline client has to be told (offline-sync.md §3.1).
//
// The merge rule for every field here is last writer wins per field, decided by the HLC - which
// is the rule the Definition of Done asks to be stated for each new field. A container has no set
// and no ordered collection of its own; `order_key` is a fractional index and merges by itself,
// and `version` is derived server-side and never merged.
func (h CreateContainer) recordChange(
	ctx context.Context, container domain.Container, actor appshared.ActorContext, snapshot map[string]any,
) error {
	return h.Changes.Record(ctx, changelog.Change{
		TenantID: container.TenantID,
		Entity:   containerTarget,
		EntityID: container.ID,
		Op:       changelog.Upsert,
		// The visibility filter a pull applies. For a collection that is the hub above it, so a
		// device subscribed to the hub sees the new collection appear.
		ContainerID: firstNonZero(container.ParentID, container.ID),
		ActorID:     actor.AccountID,
		HLC:         h.HLC.Next(),
		Payload:     snapshot,
	})
}

// recordAudit writes the evidence, inside the same transaction as the change (test AT-5).
//
// There is no target label. A container's name is user content, and user content stays out of the
// trail (rule 10) - it is recorded below as a fingerprint, which is enough to see that two
// entries concern the same name and not enough to read it (audit.md §4).
func (h CreateContainer) recordAudit(
	ctx context.Context, container domain.Container, actor appshared.ActorContext, now time.Time,
) error {
	var parent any
	if !container.ParentID.IsZero() {
		parent = container.ParentID.String()
	}

	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   container.TenantID,
		OccurredAt: now,
		Action:     ContainerCreatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: containerTarget,
		TargetID:   container.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "type", Classification: audit.Open, To: string(container.Type)},
			audit.Change{Field: "parent_id", Classification: audit.Open, To: parent},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: container.Name},
		),
	})
}

func authorizationPath(parentID shared.ID) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !parentID.IsZero() {
		path = append(path, identity.HubScope(parentID))
	}
	return path
}

func firstNonZero(ids ...shared.ID) shared.ID {
	for _, id := range ids {
		if !id.IsZero() {
			return id
		}
	}
	return ""
}

// containerOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema Container), so that a REST response, an MCP tool result and an
// automation action result describe the container in the same words.
func containerOutput(container domain.Container) usecase.Output {
	out := usecase.Output{
		"id":        container.ID.String(),
		"type":      string(container.Type),
		"parent_id": nil,
		"name":      container.Name,
		"order_key": container.OrderKey,
		// The lifecycle timestamps are always present, as null when unset. A create never sets
		// either, but the read side returns the same projection - and a field that appears only
		// once a container has been archived is a field a client cannot rely on (I-C2, I-C3).
		"archived_at": timeOrNil(container.ArchivedAt),
		// Whether the container is read-only, which is not derivable from the fields above: a
		// collection in an archived hub carries no stamp of its own (I-C3). `archived_at` beside it
		// says which of the two it is, so a client offers "unarchive" on the right object.
		"effective_archived": container.IsEffectivelyArchived(),
		"deleted_at":         timeOrNil(container.DeletedAt),
		// The policies document, with the keys that have a use case. It is always here rather than
		// only on a configured collection, so that a client reads the behaviour off the response
		// instead of knowing what an absent document means.
		"policies": map[string]any{
			"completion_policy": string(container.CompletionPolicy.OrDefault()),
			"auto_assign":       autoAssignOutput(container.AutoAssign),
		},
		"created_at": container.CreatedAt,
		"updated_at": container.UpdatedAt,
		"version":    container.Version,
	}
	if !container.ParentID.IsZero() {
		out["parent_id"] = container.ParentID.String()
	}
	for field, value := range map[string]string{
		"description": container.Description,
		"icon":        container.Icon,
		"color_token": container.ColorToken,
	} {
		if value != "" {
			out[field] = value
		}
	}
	return out
}

// autoAssignOutput is the auto_assign key of the policies document: null for no policy, the
// definition without the rotation state for one - the state is the server's bookkeeping, not part
// of what a client configures. The same shape every channel and the event snapshot carry.
func autoAssignOutput(policy *domain.AutoAssignDefinition) any {
	if policy == nil {
		return nil
	}
	candidates := make([]any, 0, len(policy.Candidates))
	for _, candidate := range policy.Candidates {
		candidates = append(candidates, map[string]any{
			"kind": string(candidate.Kind), "id": candidate.ID.String(),
		})
	}
	return map[string]any{
		"strategy":   string(policy.Strategy),
		"candidates": candidates,
		"enabled":    policy.Enabled,
	}
}

// timeOrNil is how an optional instant reaches a projection: the value, or an explicit null. Nothing
// here decides that an absent timestamp means anything - the field says "not archived" by being null,
// and leaving it out entirely would say "this server does not know about archiving".
func timeOrNil(at *time.Time) any {
	if at == nil {
		return nil
	}
	return *at
}

// Descriptor is the catalogue entry: what the use case is called, what it needs, what it records,
// and how to run it. Registering it is what makes it reachable through REST, MCP and automation
// at once (arc42 §4).
func (h CreateContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateContainerName,
		Summary: "Creates a hub or a collection. A hub is a top level workspace and takes no " +
			"parent; a collection takes the identifier of the hub it belongs to. The name has to " +
			"be unique among the containers at the same level.",
		SideEffects: "Writes the container, announces " + string(event.ContainerCreated) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{
				Name: "type", Kind: usecase.KindString, Required: true,
				Enum:        []string{string(domain.ContainerHub), string(domain.ContainerCollection)},
				Description: "HUB for a top level workspace, COLLECTION for a list inside one.",
			},
			{
				Name: "name", Kind: usecase.KindString, Required: true,
				Description: "Up to 200 characters, on one line, unique among its siblings.",
			},
			{
				Name: "parent_id", Kind: usecase.KindID,
				Description: "The hub a collection belongs to. Omitted for a hub.",
			},
			{Name: "description", Kind: usecase.KindString},
			{Name: "icon", Kind: usecase.KindString},
			{
				Name: "color_token", Kind: usecase.KindString,
				Description: "A theme token rather than a colour value, so clients render it in their own palette.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerCreatedAction, TargetType: containerTarget,
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

// invoke is the adapter between the catalogue's untyped input and the typed command. It is the
// only place that mapping happens, for all three channels.
func (h CreateContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	parentID, err := in.ID("parent_id")
	if err != nil {
		return nil, err
	}

	container, err := h.Execute(ctx, actor, CreateContainerCommand{
		Type:        domain.ContainerType(in.String("type")),
		ParentID:    parentID,
		Name:        in.String("name"),
		Description: in.String("description"),
		Icon:        in.String("icon"),
		ColorToken:  in.String("color_token"),
	})
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}
