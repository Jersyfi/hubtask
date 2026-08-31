// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package condition

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Entries is the read an expression's `item` needs. Narrow for Containers' reason.
type Entries interface {
	Find(ctx context.Context, id shared.ID) (work.WorkItem, error)
}

// Containers is the read `collection` and `hub` need: a container by its identifier. Narrow rather
// than a repository, so that nothing holding an activation can write through it.
type Containers interface {
	Find(ctx context.Context, id shared.ID) (work.Container, error)
}

// Values is what an expression is told about one event, resolved when it asks.
//
// The laziness is the port's promise and the reason the engine can afford conditions at all: a
// condition naming only `event` costs no reads, and one naming `item` costs one. The engine
// evaluates every enabled rule against every event, so eagerly building `collection`, `hub` and
// `parent` would turn one event into four queries per rule (automation.md §1.2).
//
// The lookups are optional. A caller that has none - the subscriber rendering a dedupe key inside
// the dispatcher's transaction, which may not read the work management context - answers `item` as
// absent rather than failing, and an expression that names it sees a value that is not there.
type Values struct {
	Envelope event.Envelope
	// now is the run's single reading, passed rather than taken: an expression evaluated twice in
	// one run sees one instant (automation.md §1.2).
	Now time.Time
	// subject is the entry a run that no event started is about - a RELATIVE_DATE run measured
	// from one entry's due date (G-08). The envelope's subject wins where there is one, so an
	// event-triggered run is unaffected by this field existing.
	Subject shared.ID
	// payload is the body an inbound delivery carried. Untrusted from end to end: it is read as
	// *data* under one name and never rendered as an instruction to anything (ai-first.md §4,
	// automation.md §1.1).
	Payload    map[string]any
	Entries    Entries
	Containers Containers
	// JumbleID names the entry a JUMBLE_ENTRY run is about; `payload` is rendered from it, lazily
	// and as data (G-10). Zero everywhere else - the envelope's own subject still lets an EVENT
	// rule on a jumble event read the same names.
	JumbleID shared.ID
	Jumble   JumbleEntries
}

// Resolve answers one name.
func (v Values) Resolve(ctx context.Context, name string) (any, bool, error) {
	switch name {
	case VarNow:
		return v.Now, true, nil
	case VarEvent:
		return eventDocument(v.Envelope), true, nil
	case VarActor:
		return actorDocument(v.Envelope), true, nil
	case VarPayload:
		// The body an inbound webhook delivered - or, on a jumble run, the entry's fields as data
		// (G-10). Empty for a run that has neither, which is what an absent document means rather
		// than a failure - a condition written for one trigger and used on another asks about
		// something that is not there.
		if len(v.Payload) > 0 {
			return v.Payload, true, nil
		}
		return v.jumblePayload(ctx)
	case VarTenant:
		// Declared and empty, as the retention pass answers it: the workspace's settings are not
		// something this path reads, and an empty document lets a condition ask without failing.
		return map[string]any{"settings": map[string]any{}}, true, nil
	case VarItem:
		return v.item(ctx)
	case VarParent:
		return v.parent(ctx)
	case VarCollection:
		if id := CollectionOf(v.Envelope); !id.IsZero() {
			return v.container(ctx, id)
		}
		return v.collectionOfSubject(ctx)
	case VarHub:
		return v.hub(ctx)
	default:
		return nil, false, nil
	}
}

// entry is the entry the run is about: the event's subject where there is an event, and the
// command's subject where there is not. One place answers it, so `item`, `parent`, `collection` and
// `hub` cannot disagree about which entry a run concerns.
func (v Values) entry() shared.ID {
	if id := ItemOf(v.Envelope); !id.IsZero() {
		return id
	}
	return v.Subject
}

func (v Values) item(ctx context.Context) (any, bool, error) {
	id := v.entry()
	if id.IsZero() || v.Entries == nil {
		return nil, false, nil
	}

	item, err := v.Entries.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		// The entry is gone between the event and the run. Absent rather than a failure: a
		// condition asking about a deleted entry gets "not there", which is true.
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return ItemDocument(item), true, nil
}

func (v Values) parent(ctx context.Context) (any, bool, error) {
	id := v.entry()
	if id.IsZero() || v.Entries == nil {
		return nil, false, nil
	}

	item, err := v.Entries.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if item.ParentID.IsZero() {
		return nil, false, nil
	}

	parent, err := v.Entries.Find(ctx, item.ParentID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return ItemDocument(parent), true, nil
}

func (v Values) hub(ctx context.Context) (any, bool, error) {
	if id := HubOf(v.Envelope); !id.IsZero() {
		return v.container(ctx, id)
	}

	collectionID := CollectionOf(v.Envelope)
	if collectionID.IsZero() {
		var err error
		if collectionID, err = v.subjectCollection(ctx); err != nil {
			return nil, false, err
		}
	}
	if collectionID.IsZero() || v.Containers == nil {
		return nil, false, nil
	}
	collection, err := v.Containers.Find(ctx, collectionID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return v.container(ctx, collection.ParentID)
}

// collectionOfSubject answers `collection` for a run no event started, by reading the entry the run
// is about. One read, and only when an expression asks - the laziness the whole activation is built
// on (automation.md §1.2).
func (v Values) collectionOfSubject(ctx context.Context) (any, bool, error) {
	id, err := v.subjectCollection(ctx)
	if err != nil {
		return nil, false, err
	}
	return v.container(ctx, id)
}

// subjectCollection is the collection the run's entry sits in, and zero when there is no entry.
func (v Values) subjectCollection(ctx context.Context) (shared.ID, error) {
	id := v.entry()
	if id.IsZero() || v.Entries == nil {
		return "", nil
	}

	item, err := v.Entries.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return item.CollectionID, nil
}

func (v Values) container(ctx context.Context, id shared.ID) (any, bool, error) {
	if id.IsZero() || v.Containers == nil {
		return nil, false, nil
	}

	container, err := v.Containers.Find(ctx, id)
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
func ItemOf(envelope event.Envelope) shared.ID {
	const prefix = "item/"
	if len(envelope.Subject) <= len(prefix) || envelope.Subject[:len(prefix)] != prefix {
		return ""
	}
	return shared.ID(envelope.Subject[len(prefix):])
}

// CollectionOf reads the collection out of an event's payload.
//
// Two shapes, because there are two kinds of event. An item event names the collection it is in; a
// container event *is* the container, so a collection names itself and a hub names nothing - a
// hub-scoped rule matches the hub's own events through the hub identity below.
func CollectionOf(envelope event.Envelope) shared.ID {
	if id := idAt(envelope.Payload, "collection_id"); !id.IsZero() {
		return id
	}
	if kind, _ := envelope.Payload["type"].(string); kind == string(work.ContainerCollection) {
		return idAt(envelope.Payload, "id")
	}
	return ""
}

// HubOf reads a hub the event is directly about, which is the case a collection lookup cannot
// cover.
func HubOf(envelope event.Envelope) shared.ID {
	if kind, _ := envelope.Payload["type"].(string); kind == string(work.ContainerHub) {
		return idAt(envelope.Payload, "id")
	}
	return ""
}

// idAt reads one identifier out of a payload document, and zero for anything that is not one.
func idAt(payload map[string]any, key string) shared.ID {
	text, _ := payload[key].(string)
	id, err := shared.ParseID(text)
	if err != nil {
		return ""
	}
	return id
}

// JumbleEntries is the read `payload` costs on a jumble run: the entry, by its identifier. Narrow
// for the reason every lookup here is.
type JumbleEntries interface {
	Find(ctx context.Context, id shared.ID) (jumble.Entry, error)
}

// JumbleEntryOf reads the entry a jumble event is about out of its subject, ItemOf's shape.
func JumbleEntryOf(envelope event.Envelope) shared.ID {
	const prefix = "jumble_entry/"
	if len(envelope.Subject) <= len(prefix) || envelope.Subject[:len(prefix)] != prefix {
		return ""
	}
	return shared.ID(envelope.Subject[len(prefix):])
}

// jumbleEntry is the entry this activation would read `payload` from: the one named outright - a
// JUMBLE_ENTRY run carries it - or the one the envelope is about, which is what lets an EVENT rule
// on jumble.entry_received read the same names.
func (v Values) jumbleEntry() shared.ID {
	if !v.JumbleID.IsZero() {
		return v.JumbleID
	}
	return JumbleEntryOf(v.Envelope)
}

// jumblePayload loads the entry and renders it as the CEL `payload` document (automation.md §1.1):
// the fields as *data*, under the discipline ai-first.md rules - matched, never rendered as
// instructions to anything. Lazy, like every read here: a condition that never names `payload`
// costs no read.
func (v Values) jumblePayload(ctx context.Context) (any, bool, error) {
	id := v.jumbleEntry()
	if id.IsZero() || v.Jumble == nil {
		return map[string]any{}, true, nil
	}

	entry, err := v.Jumble.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		// Settled and swept between the arrival and the run. An empty document is honest: what
		// the entry said is no longer knowable.
		return map[string]any{}, true, nil
	}
	if err != nil {
		return nil, false, err
	}

	return map[string]any{
		"id":               entry.ID.String(),
		"channel":          entry.Channel.String(),
		"sender":           entry.Sender,
		"raw_subject":      entry.RawSubject,
		"raw_body":         entry.RawBody,
		"status":           string(entry.Status),
		"attachment_count": len(entry.Attachments),
		"received_at":      entry.ReceivedAt.UTC(),
	}, true, nil
}
