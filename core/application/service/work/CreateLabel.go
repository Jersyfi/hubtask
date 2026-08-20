// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
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
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateLabelName = "CreateLabel"
	ListLabelsName  = "ListLabels"
	labelTarget     = "label"

	// The token scopes. A label is structure rather than content, so it shares the container's
	// scopes for the reason a bucket does: a token that may reorganise a workspace may define its
	// vocabulary (security.md §5).
	labelsWrite = containersWrite
	labelsRead  = containersRead

	// LabelCreatedAction is the audit code. Stable: an auditor filters on it and a SIEM rule
	// matches on it (audit.md §2).
	LabelCreatedAction audit.Action = "label.created"
	// LabelReadAction is the audit code of an attempted read. Declared even though an ordinary read
	// writes no entry: a *refused* read does, and it is recorded against the action that was
	// refused rather than against a generic "denied" (audit.md §4).
	LabelReadAction audit.Action = "label.read"
)

// CreateLabel adds a label to a collection's vocabulary.
//
// A collection rather than a hub, and rather than the tenant: a label is a vocabulary the people
// working in one collection agree on, and a workspace-wide list would make every collection pay for
// every other's (domain-model.md §3.5).
type CreateLabel struct {
	Labels     repository.Labels
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

// CreateLabelCommand is the input, typed.
type CreateLabelCommand struct {
	CollectionID shared.ID
	Name         string
	ColorToken   string
	Description  string
}

// Execute creates the label and returns it.
func (h CreateLabel) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateLabelCommand,
) (domain.Label, error) {
	if cmd.CollectionID.IsZero() {
		return domain.Label{}, collectionIDRequired()
	}

	// The collection is read before the permission question, because the answer depends on its
	// path: a membership held at the hub applies downwards (domain-model.md §3.2). Nothing read
	// here is trusted afterwards - the state that decides the write is read again in the
	// transaction.
	collection, err := h.readCollection(ctx, actor, cmd.CollectionID)
	if err != nil {
		return domain.Label{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(collection),
		Action:     LabelCreatedAction,
		TokenScope: labelsWrite,
		TargetType: labelTarget,
		// The label does not exist yet, so the refusal names the vocabulary it would have joined.
		TargetID: cmd.CollectionID,
	}); err != nil {
		return domain.Label{}, err
	}

	var created domain.Label
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		collection, err := findNamedCollection(ctx, h.Containers, cmd.CollectionID)
		if err != nil {
			return err
		}
		// The same gate a new entry passes: a hub has no vocabulary, and a trashed or archived
		// collection is read-only (I-C2, I-C3).
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}

		label, err := domain.NewLabel(domain.NewLabelInput{
			ID:           h.IDs.NewID(),
			TenantID:     actor.TenantID,
			CollectionID: cmd.CollectionID,
			Name:         cmd.Name,
			ColorToken:   cmd.ColorToken,
			Description:  cmd.Description,
		})
		if err != nil {
			return err
		}

		if err := h.Labels.Insert(ctx, label); err != nil {
			return err
		}

		now := h.Clock.Now()
		// One snapshot, two recipients. The event goes outwards as a public contract, the change
		// log goes to synchronising clients (offline-sync.md §10) - but they describe the same
		// state, and building it twice is how the two come to disagree.
		announcement, err := event.NewLabelCreated(
			h.IDs.NewID(), label, event.Actor{Kind: actor.Kind, ID: actor.AccountID},
			now, event.Cause{})
		if err != nil {
			return err
		}
		if err := h.Events.Append(ctx, announcement); err != nil {
			return err
		}
		if err := h.recordChange(ctx, label, collection, actor, announcement.Payload); err != nil {
			return err
		}
		if err := h.recordAudit(ctx, label, actor, now); err != nil {
			return err
		}

		created = label
		return nil
	})
	if err != nil {
		return domain.Label{}, err
	}
	return created, nil
}

// readCollection reads the collection outside the write transaction, because the permission check
// needs it first. Read-only, so it may be served by a replica (multi-tenancy.md §7).
func (h CreateLabel) readCollection(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Container, error) {
	var collection domain.Container

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findNamedCollection(ctx, h.Containers, id)
		collection = found
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return collection, nil
}

// recordChange writes what an offline client has to be told (offline-sync.md §3.1).
//
// The merge rule for every field of a label is last writer wins per field, decided by the HLC -
// which is the rule the Definition of Done asks to be stated for each new field. `version` is
// derived server-side and never merged. Which entries carry the label is not a field of the label
// at all: it is an item's set, and it merges as an OR-set (work.MergeSetElements).
func (h CreateLabel) recordChange(
	ctx context.Context, label domain.Label, collection domain.Container,
	actor appshared.ActorContext, snapshot map[string]any,
) error {
	return h.Changes.Record(ctx, changelog.Change{
		TenantID: label.TenantID,
		Entity:   labelTarget,
		EntityID: label.ID,
		Op:       changelog.Upsert,
		// The visibility filter a pull applies: the hub above the collection, so that a device
		// subscribed to the hub sees the new label appear.
		ContainerID: firstNonZero(collection.ParentID, label.CollectionID),
		ActorID:     actor.AccountID,
		HLC:         h.HLC.Next(),
		Payload:     snapshot,
	})
}

// recordAudit writes the evidence, inside the same transaction as the change (test AT-5).
//
// The name and the description are user content and are recorded as fingerprints: enough to see
// that two entries concern the same label, not enough to read it (rule 10, audit.md §4). The colour
// is a theme token this installation defined and carries no personal data.
func (h CreateLabel) recordAudit(
	ctx context.Context, label domain.Label, actor appshared.ActorContext, now time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   label.TenantID,
		OccurredAt: now,
		Action:     LabelCreatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: labelTarget,
		TargetID:   label.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{
				Field: "collection_id", Classification: audit.Open,
				To: label.CollectionID.String(),
			},
			audit.Change{Field: domain.FieldName, Classification: audit.Sensitive, To: label.Name},
			audit.Change{
				Field: domain.FieldColorToken, Classification: audit.Open, To: label.ColorToken,
			},
		),
	})
}

// labelOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema Label). The description is an explicit null rather than an omission,
// for the reason a bucket's limit is: a client renders the label from this.
func labelOutput(label domain.Label) usecase.Output {
	out := usecase.Output{
		"id":            label.ID.String(),
		"collection_id": label.CollectionID.String(),
		"name":          label.Name,
		"color_token":   label.ColorToken,
		"description":   nil,
		"version":       label.Version,
	}
	if label.Description != "" {
		out["description"] = label.Description
	}
	return out
}

// Descriptor is the catalogue entry.
func (h CreateLabel) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateLabelName,
		Summary: "Adds a label to a collection's vocabulary. The name has to be free in the " +
			"collection, compared without regard to case or accents. The colour is required: a " +
			"label is rendered as a chip and nothing else, so with none a client would have to " +
			"invent one. Only a collection has a vocabulary; a hub is refused.",
		SideEffects: "Writes the label, announces " + string(event.LabelCreated) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: labelsWrite,
		Input: []usecase.Field{
			{
				Name: "collection_id", Kind: usecase.KindID, Required: true,
				Description: "The collection whose vocabulary the label joins.",
			},
			{
				Name: "name", Kind: usecase.KindString, Required: true,
				Description: "Up to 120 characters, on one line, free in this collection.",
			},
			{
				Name: "color_token", Kind: usecase.KindString, Required: true,
				Description: "A theme token rather than a colour value, so clients render it in " +
					"their own palette.",
			},
			{
				Name: "description", Kind: usecase.KindString,
				Description: "What the label means, for the people who have to agree on it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: LabelCreatedAction, TargetType: labelTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a collection's vocabulary is its configuration. Deleting a label does not rewrite " +
				"the entries that carried it, so nothing happened to any of them.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateLabel) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}

	label, err := h.Execute(ctx, actor, CreateLabelCommand{
		CollectionID: collectionID,
		Name:         in.String("name"),
		ColorToken:   in.String("color_token"),
		Description:  in.String("description"),
	})
	if err != nil {
		return nil, err
	}
	return labelOutput(label), nil
}

// ListLabels reads a collection's vocabulary.
//
// Read-only throughout: the transaction may be served by a read replica (multi-tenancy.md §7).
type ListLabels struct {
	Labels     repository.Labels
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListLabelsQuery is the input, typed.
type ListLabelsQuery struct {
	CollectionID shared.ID
}

// Execute returns the collection's labels, by name.
//
// Not paged, for the reason a board is not: a vocabulary people have to agree on is as long as a
// person can hold in their head, and the contract returns a plain array (api-guidelines.md §2).
func (h ListLabels) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListLabelsQuery,
) ([]domain.Label, error) {
	if query.CollectionID.IsZero() {
		return nil, collectionIDRequired()
	}

	var collection domain.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findNamedCollection(ctx, h.Containers, query.CollectionID)
		collection = found
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     LabelReadAction,
		TokenScope: labelsRead,
		TargetType: labelTarget,
		TargetID:   query.CollectionID,
	}); err != nil {
		return nil, err
	}

	var vocabulary []domain.Label
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		vocabulary, err = h.Labels.List(ctx, query.CollectionID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return vocabulary, nil
}

// Descriptor is the catalogue entry.
func (h ListLabels) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListLabelsName,
		Summary: "Lists a collection's labels by name. Deleted ones are not in it. Not paged: a " +
			"vocabulary people have to agree on is as long as a person can hold in their head. A " +
			"hub has no vocabulary and comes back empty.",
		SideEffects: "None. Reads only.",
		TokenScope:  labelsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "collection_id", Kind: usecase.KindID, Required: true,
				Description: "The collection whose vocabulary is wanted.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: LabelReadAction, TargetType: labelTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListLabels) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}

	vocabulary, err := h.Execute(ctx, actor, ListLabelsQuery{CollectionID: collectionID})
	if err != nil {
		return nil, err
	}

	data := make([]usecase.Output, 0, len(vocabulary))
	for _, label := range vocabulary {
		data = append(data, labelOutput(label))
	}
	return usecase.Output{"data": data}, nil
}
