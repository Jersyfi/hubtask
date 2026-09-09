// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"time"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/presentation/stream"
)

// changeEvent writes one change record: its cursor as the `id`, its entity as the `event`, and the
// record itself as `data`.
//
// The entity is the event name because that is what a client dispatches on - `addEventListener`
// takes a name, and a client interested only in comments should not have to parse every item
// change to find out it is not one.
//
// The framing itself is `presentation/stream`'s, shared with the agent stream: this function is
// the part that is this endpoint's own, which is what goes in the `data`.
func changeEvent(w *stream.Writer, id string, record syncservice.Record) error {
	payload, err := json.Marshal(changePayload(record))
	if err != nil {
		// The record came out of the database and back through the log; a payload that will not
		// serialise is a defect rather than something to recover from. Ending the stream lets the
		// client reconnect, which is the only useful thing left.
		return err
	}
	return w.Event(id, record.Entity, string(payload))
}

// changePayload is what a client receives for one change.
//
// Built by hand rather than by serialising the record, because the wire shape is a contract and a
// struct's field names are not. It carries what offline-sync.md §3.3 promises a pull carries: what
// changed, where it sits, who did it, when, under which clock, and the changed fields themselves.
func changePayload(record syncservice.Record) map[string]any {
	payload := map[string]any{
		"seq":       record.Seq,
		"entity":    record.Entity,
		"entity_id": record.EntityID.String(),
		"op":        string(record.Op),
		// The moment the change was recorded, so a client can render "changed at" without a second
		// request and an operator reading a captured stream can line it up against a log. Not the
		// cursor, and deliberately not usable as one: ordering is the sequence's job, and a
		// timestamp that two concurrent transactions can share would not be a total order
		// (ADR-0021).
		"occurred_at": record.OccurredAt.UTC().Format(time.RFC3339Nano),
		"hlc":         record.HLC.String(),
	}
	if !record.ContainerID.IsZero() {
		payload["container_id"] = record.ContainerID.String()
	}
	if !record.ActorID.IsZero() {
		payload["actor_id"] = record.ActorID.String()
	}
	if !record.DeviceID.IsZero() {
		payload["device_id"] = record.DeviceID.String()
	}
	// Absent rather than null on a deletion: a tombstone carries no content by design, and `null`
	// would say "the change set is empty", which is a different statement (offline-sync.md §4.2).
	if record.Payload != nil {
		payload["payload"] = record.Payload
	}
	return payload
}
