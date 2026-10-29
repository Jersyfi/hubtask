// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"sort"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
)

// The kinds beyond the entry's (offline-sync.md §4.2): a reminder, a recurrence rule, a container,
// a label and a bucket, whole on creation and deletion and field by field between, and applying a
// template. Each is the route's own use case performed as the pushing person - a push grants
// nothing - and a patch is decided field by field against the server's clock (domain.Decide),
// its winners applied under the device's readings so that the next device compares against the
// clock that won.

// owner is what the push knows about one family: the use cases that create, change and delete
// it, the input its identifier goes in under, and how its current state is read back.
type owner struct {
	create, update, remove string
	idKey                  string
	read                   func(ctx context.Context, p PushChanges, actor appshared.ActorContext, m Mutation) (usecase.Output, error)
}

func ownerOf(entity string) owner {
	switch entity {
	case "reminder":
		return owner{create: "CreateReminder", update: "UpdateReminder", remove: "DeleteReminder", idKey: "reminder_id",
			read: func(ctx context.Context, p PushChanges, actor appshared.ActorContext, m Mutation) (usecase.Output, error) {
				return p.listed(ctx, actor, "ListReminders", usecase.Input{"item_id": m.ItemID.String()}, m.EntityID)
			}}
	case "container":
		return owner{create: "CreateContainer", update: "RenameContainer", remove: "TrashContainer", idKey: "container_id",
			read: func(ctx context.Context, p PushChanges, actor appshared.ActorContext, m Mutation) (usecase.Output, error) {
				return p.Catalogue.Invoke(ctx, "GetContainer", actor, usecase.Input{"container_id": m.EntityID.String()})
			}}
	case "label":
		return owner{create: "CreateLabel", update: "UpdateLabel", remove: "DeleteLabel", idKey: "label_id",
			read: func(ctx context.Context, p PushChanges, actor appshared.ActorContext, m Mutation) (usecase.Output, error) {
				return p.listed(ctx, actor, "ListLabels", usecase.Input{
					"collection_id": m.ContainerID.String(), "include_deleted": true,
				}, m.EntityID)
			}}
	case "bucket":
		return owner{create: "CreateBucket", update: "UpdateBucket", remove: "DeleteBucket", idKey: "bucket_id",
			read: func(ctx context.Context, p PushChanges, actor appshared.ActorContext, m Mutation) (usecase.Output, error) {
				return p.listed(ctx, actor, "ListBuckets", usecase.Input{"collection_id": m.ContainerID.String()}, m.EntityID)
			}}
	}
	return owner{}
}

// emptied is how a field the device cleared (a JSON null) reaches the use case: the empty value
// of its kind, the reading the REST route gives a null.
var emptied = map[string]any{
	"wip_limit": 0, "max_count": 0, "is_done_bucket": false, "channels": []any{}, "recipients": []any{},
}

func clearedValue(field string, value any) any {
	if value != nil {
		return value
	}
	if empty, known := emptied[field]; known {
		return empty
	}
	return ""
}

// entity applies a mutation of a family beyond the entry's.
func (p PushChanges) entity(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	family, shape, _ := domain.FamilyOf(m.Kind)
	if family.Entity == "recurrence_rule" {
		return p.recurrence(ctx, actor, m, shape)
	}
	if m.EntityID.IsZero() {
		return Result{}, shared.ErrValidation.
			WithDetail("sync.entity_required").
			WithFields(shared.FieldError{Path: "/entity_id", Code: "sync.entity_required"})
	}
	if family.Entity == "reminder" && m.ItemID.IsZero() {
		return Result{}, shared.ErrValidation.WithDetail("sync.item_required")
	}
	own := ownerOf(family.Entity)
	switch shape {
	case domain.ShapeCreation:
		in := inputOf(m.Payload)
		in["id"] = m.EntityID.String()
		if family.Entity == "reminder" {
			in["item_id"] = m.ItemID.String()
		}
		out, err := p.Catalogue.Invoke(ctx, own.create, actor, in)
		if err != nil {
			return Result{}, err
		}
		return Result{OpID: m.OpID, Result: domain.Applied, EntityID: m.EntityID, ServerState: out}, nil
	case domain.ShapeDeletion:
		in := usecase.Input{own.idKey: m.EntityID.String()}
		if family.Entity == "reminder" {
			in["item_id"] = m.ItemID.String()
		}
		if _, err := p.Catalogue.Invoke(ctx, own.remove, actor, in); err != nil {
			return Result{}, err
		}
		return Result{OpID: m.OpID, Result: domain.Applied, EntityID: m.EntityID}, nil
	}

	// A patch: decided field by field, the winners applied in one call of the use case that owns
	// them. Read first, so that a device that may not read the object gets that refusal.
	if (family.Entity == "label" || family.Entity == "bucket") && m.ContainerID.IsZero() {
		return Result{}, shared.ErrValidation.
			WithDetail("sync.container_required").
			WithFields(shared.FieldError{Path: "/container_id", Code: "sync.container_required"})
	}
	if _, err := own.read(ctx, p, actor, m); err != nil {
		return Result{}, err
	}
	winners, err := p.decide(ctx, actor, family, m.EntityID, m)
	if err != nil {
		return Result{}, err
	}
	var after usecase.Output
	if len(winners) > 0 {
		in := usecase.Input{own.idKey: m.EntityID.String()}
		if family.Entity == "reminder" {
			in["item_id"] = m.ItemID.String()
		}
		for _, field := range sortedFields(winners) {
			in[field] = clearedValue(field, m.Fields[field].Value)
		}
		if after, err = p.Catalogue.Invoke(appshared.ContextWithReadings(ctx, winners), own.update, actor, in); err != nil {
			return Result{}, err
		}
	} else if after, err = own.read(ctx, p, actor, m); err != nil {
		return Result{}, err
	}
	result := domain.Merged
	if len(winners) == len(m.Fields) {
		result = domain.Applied
	}
	return Result{OpID: m.OpID, Result: result, EntityID: m.EntityID, ServerState: after}, nil
}

// recurrence applies the entry's one series: a creation where it has none, a removal, and field by
// field otherwise - a creation for an entry another device already gave a series merges with it,
// because two series on one entry is not a state anybody made. SetRecurrence takes the series
// whole, so the fields the device lost travel with the server's values.
func (p PushChanges) recurrence(ctx context.Context, actor appshared.ActorContext, m Mutation, shape domain.Shape) (Result, error) {
	if m.ItemID.IsZero() {
		return Result{}, shared.ErrValidation.WithDetail("sync.item_required")
	}
	itemID := m.ItemID.String()
	if shape == domain.ShapeDeletion {
		if _, err := p.Catalogue.Invoke(ctx, "RemoveRecurrence", actor, usecase.Input{"item_id": itemID}); err != nil {
			return Result{}, err
		}
		return Result{OpID: m.OpID, Result: domain.Applied, EntityID: m.EntityID}, nil
	}

	before, err := p.Catalogue.Invoke(ctx, "GetRecurrence", actor, usecase.Input{"item_id": itemID})
	if err != nil && shared.AsError(err).Category != shared.CategoryNotFound {
		return Result{}, err
	}
	if err != nil {
		// No series yet: a creation, whichever kind said so - a patch of a series a later push
		// removed is the device's whole series again.
		in := inputOf(m.Payload)
		for field, change := range m.Fields {
			in[field] = clearedValue(field, change.Value)
		}
		in["item_id"] = itemID
		if !m.EntityID.IsZero() {
			in["id"] = m.EntityID.String()
		}
		out, err := p.Catalogue.Invoke(ctx, "SetRecurrence", actor, in)
		if err != nil {
			return Result{}, err
		}
		return Result{OpID: m.OpID, Result: domain.Applied, EntityID: shared.ID(stringOf(out["id"])), ServerState: out}, nil
	}

	if shape == domain.ShapeCreation {
		// The whole series as fields, each under the creation's reading.
		m.Fields = make(map[string]FieldChange, len(m.Payload))
		for field, value := range m.Payload {
			if err := domain.RecurrenceFamily().Judge(field); err != nil {
				return Result{}, err
			}
			m.Fields[field] = FieldChange{Value: value, HLC: m.HLC}
		}
	}
	ruleID := shared.ID(stringOf(before["id"]))
	winners, err := p.decide(ctx, actor, domain.RecurrenceFamily(), ruleID, m)
	if err != nil {
		return Result{}, err
	}
	after := before
	if len(winners) > 0 {
		in := usecase.Input{"item_id": itemID}
		for _, field := range []string{"rrule", "time_zone", "mode", "horizon_days", "ends_at", "max_count"} {
			value := before[field]
			if _, won := winners[field]; won {
				value = m.Fields[field].Value
			}
			if at, isTime := value.(time.Time); isTime {
				value = at.UTC().Format(time.RFC3339)
			}
			in[field] = clearedValue(field, value)
		}
		if after, err = p.Catalogue.Invoke(appshared.ContextWithReadings(ctx, winners), "SetRecurrence", actor, in); err != nil {
			return Result{}, err
		}
	}
	result := domain.Merged
	if len(winners) == len(m.Fields) {
		result = domain.Applied
	}
	return Result{OpID: m.OpID, Result: result, EntityID: ruleID, ServerState: after}, nil
}

// decide reads the server's clock per field of the object and answers the fields the device won.
func (p PushChanges) decide(
	ctx context.Context, actor appshared.ActorContext, family domain.Family, id shared.ID, m Mutation,
) (map[string]shared.HLC, error) {
	if len(m.Fields) == 0 {
		return nil, shared.ErrValidation.
			WithDetail("sync.fields_required").
			WithFields(shared.FieldError{Path: "/fields", Code: "sync.fields_required"})
	}
	sent := make(map[string]shared.HLC, len(m.Fields))
	for field, change := range m.Fields {
		if err := family.Judge(field); err != nil {
			return nil, err
		}
		reading, err := shared.ParseHLC(change.HLC)
		if err != nil {
			return nil, err
		}
		sent[field] = reading
	}
	var clocks map[string]shared.HLC
	if p.Clocks != nil {
		err := p.Stream.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			clocks, err = p.Clocks.Of(ctx, family.Entity, id)
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	winners, _ := domain.Decide(sent, clocks)
	return winners, nil
}

// template applies a template: InstantiateTemplate with the device's body. Its entries arrive as
// ordinary creations; the result names the root.
func (p PushChanges) template(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	out, err := p.Catalogue.Invoke(ctx, "InstantiateTemplate", actor, inputOf(m.Payload))
	if err != nil {
		return Result{}, err
	}
	return Result{OpID: m.OpID, Result: domain.Applied, EntityID: shared.ID(stringOf(out["root_item_id"])), ServerState: out}, nil
}

// listed reads one object out of the list its use case answers, as `{data: [...]}`. Not found is
// the refusal a missing object earns.
func (p PushChanges) listed(
	ctx context.Context, actor appshared.ActorContext, name string, in usecase.Input, id shared.ID,
) (usecase.Output, error) {
	out, err := p.Catalogue.Invoke(ctx, name, actor, in)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	switch data := out["data"].(type) {
	case []usecase.Output:
		for _, row := range data {
			rows = append(rows, row)
		}
	case []map[string]any:
		rows = data
	case []any:
		for _, row := range data {
			if object, ok := row.(map[string]any); ok {
				rows = append(rows, object)
			}
		}
	}
	for _, row := range rows {
		if stringOf(row["id"]) == id.String() {
			return row, nil
		}
	}
	return nil, shared.ErrNotFound.WithDetail("sync.entity_not_found")
}

func sortedFields(fields map[string]shared.HLC) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
