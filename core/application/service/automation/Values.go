// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Entries is the read an expression's `item` needs. Narrow for Containers' reason.
type Entries interface {
	Find(ctx context.Context, id shared.ID) (work.WorkItem, error)
}

// eventValues is what an expression is told about one event, resolved when it asks.
//
// The laziness is the port's promise and the reason the engine can afford conditions at all: a
// condition naming only `event` costs no reads, and one naming `item` costs one. The engine
// evaluates every enabled rule against every event, so eagerly building `collection`, `hub` and
// `parent` would turn one event into four queries per rule (automation.md §1.2).
//
// The lookups are optional. A caller that has none - the subscriber rendering a dedupe key inside
// the dispatcher's transaction, which may not read the work management context - answers `item` as
// absent rather than failing, and an expression that names it sees a value that is not there.
type eventValues struct {
	envelope event.Envelope
	// now is the run's single reading, passed rather than taken: an expression evaluated twice in
	// one run sees one instant (automation.md §1.2).
	now        time.Time
	entries    Entries
	containers Containers
}

// Resolve answers one name.
func (v eventValues) Resolve(ctx context.Context, name string) (any, bool, error) {
	switch name {
	case condition.VarNow:
		return v.now, true, nil
	case condition.VarEvent:
		return eventDocument(v.envelope), true, nil
	case condition.VarActor:
		return actorDocument(v.envelope), true, nil
	case condition.VarPayload:
		// The body an inbound webhook delivered. Empty for an event-triggered run, which is what
		// an absent document means rather than a failure - a condition written for one trigger and
		// used on another asks about something that is not there.
		return map[string]any{}, true, nil
	case condition.VarTenant:
		// Declared and empty, as the retention pass answers it: the workspace's settings are not
		// something this path reads, and an empty document lets a condition ask without failing.
		return map[string]any{"settings": map[string]any{}}, true, nil
	case condition.VarItem:
		return v.item(ctx)
	case condition.VarParent:
		return v.parent(ctx)
	case condition.VarCollection:
		return v.container(ctx, collectionOf(v.envelope))
	case condition.VarHub:
		return v.hub(ctx)
	default:
		return nil, false, nil
	}
}

func (v eventValues) item(ctx context.Context) (any, bool, error) {
	id := itemOf(v.envelope)
	if id.IsZero() || v.entries == nil {
		return nil, false, nil
	}

	item, err := v.entries.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		// The entry is gone between the event and the run. Absent rather than a failure: a
		// condition asking about a deleted entry gets "not there", which is true.
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return condition.ItemDocument(item), true, nil
}

func (v eventValues) parent(ctx context.Context) (any, bool, error) {
	id := itemOf(v.envelope)
	if id.IsZero() || v.entries == nil {
		return nil, false, nil
	}

	item, err := v.entries.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if item.ParentID.IsZero() {
		return nil, false, nil
	}

	parent, err := v.entries.Find(ctx, item.ParentID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return condition.ItemDocument(parent), true, nil
}

func (v eventValues) hub(ctx context.Context) (any, bool, error) {
	if id := hubOf(v.envelope); !id.IsZero() {
		return v.container(ctx, id)
	}

	collectionID := collectionOf(v.envelope)
	if collectionID.IsZero() || v.containers == nil {
		return nil, false, nil
	}
	collection, err := v.containers.Find(ctx, collectionID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return v.container(ctx, collection.ParentID)
}

func (v eventValues) container(ctx context.Context, id shared.ID) (any, bool, error) {
	if id.IsZero() || v.containers == nil {
		return nil, false, nil
	}

	container, err := v.containers.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return containerDocument(container), true, nil
}

// eventDocument is the envelope as an expression reads it: what happened and what it was about,
// never the payload's own fields. A condition that wanted those names `item`, which is the entry as
// it stands rather than as it was when the event was written - and the difference matters, because
// a run happens after the fact.
func eventDocument(envelope event.Envelope) map[string]any {
	return map[string]any{
		"id":              envelope.ID.String(),
		"type":            envelope.Type.String(),
		"subject":         envelope.Subject,
		"occurred_at":     envelope.OccurredAt.UTC(),
		"causation_depth": envelope.CausationDepth,
		"replay":          envelope.Replay,
	}
}

// actorDocument is who caused the event. The kind and the identifiers, never a name: a display name
// is user content, and an expression is not a place for one (rule 10).
func actorDocument(envelope event.Envelope) map[string]any {
	out := map[string]any{"kind": string(envelope.Actor.Kind)}
	if !envelope.Actor.ID.IsZero() {
		out["id"] = envelope.Actor.ID.String()
	}
	if !envelope.Actor.OnBehalfOf.IsZero() {
		out["on_behalf_of"] = envelope.Actor.OnBehalfOf.String()
	}
	return out
}

// containerDocument is a hub or a collection as an expression reads it. Snake case and a written
// list, exactly as the entry's projection is and for the same reason.
func containerDocument(container work.Container) map[string]any {
	out := map[string]any{
		"id":       container.ID.String(),
		"type":     string(container.Type),
		"name":     container.Name,
		"archived": container.ArchivedAt != nil,
	}
	if !container.ParentID.IsZero() {
		out["parent_id"] = container.ParentID.String()
	}
	return out
}

// itemOf reads the entry an event is about out of its subject.
//
// The subject rather than the payload, because `<entity>/<id>` is the CloudEvents field that names
// what the event is about and every item event sets it. A payload key would be a second place to
// look, and the two would eventually disagree.
func itemOf(envelope event.Envelope) shared.ID {
	const prefix = "item/"
	if len(envelope.Subject) <= len(prefix) || envelope.Subject[:len(prefix)] != prefix {
		return ""
	}
	return shared.ID(envelope.Subject[len(prefix):])
}
