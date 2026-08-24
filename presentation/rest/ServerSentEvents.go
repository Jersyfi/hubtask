// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
)

// sseWriter frames server-sent events onto a response, and flushes each one.
//
// The flush is the whole reason this type exists rather than a few Fprintf calls. net/http buffers
// a response, so an event that was written and not flushed is an event the client gets when the
// next four kilobytes arrive - which for a quiet workspace is minutes later, and which makes a
// stream indistinguishable from a broken one.
type sseWriter struct {
	w          http.ResponseWriter
	controller *http.ResponseController
}

// event writes one change record: its cursor as the `id`, its entity as the `event`, and the
// record itself as `data`.
//
// The entity is the event name because that is what a client dispatches on - `addEventListener`
// takes a name, and a client interested only in comments should not have to parse every item
// change to find out it is not one.
func (s *sseWriter) event(id string, record syncservice.Record) error {
	payload, err := json.Marshal(changePayload(record))
	if err != nil {
		// The record came out of the database and back through the log; a payload that will not
		// serialise is a defect rather than something to recover from. Ending the stream lets the
		// client reconnect, which is the only useful thing left.
		return err
	}

	var out strings.Builder
	out.WriteString("id: ")
	out.WriteString(id)
	out.WriteString("\nevent: ")
	out.WriteString(sseFieldValue(record.Entity))
	out.WriteString("\ndata: ")
	// One line, because JSON has no newline outside a string and a data field that spanned lines
	// would have to be split across several `data:` lines to stay legal.
	out.Write(payload)
	out.WriteString("\n\n")

	return s.write(out.String())
}

// comment writes a `:` line: the heartbeat, and the goodbye.
//
// A comment rather than an event, because a client must be able to ignore it without knowing what
// it is - the specification says a line beginning with a colon is discarded, which is exactly the
// contract a keep-alive needs.
func (s *sseWriter) comment(text string) error {
	return s.write(": " + sseFieldValue(text) + "\n\n")
}

// retry tells a browser's EventSource how long to wait before reconnecting.
func (s *sseWriter) retry(milliseconds int) error {
	return s.write(fmt.Sprintf("retry: %d\n\n", milliseconds))
}

func (s *sseWriter) write(frame string) error {
	if _, err := s.w.Write([]byte(frame)); err != nil {
		return err
	}
	// Errors are ignored on purpose: a writer that cannot flush is one that buffers, which is
	// slower rather than wrong, and the write above is what reports a connection that has gone.
	_ = s.controller.Flush()
	return nil
}

// sseFieldValue keeps a value on one line.
//
// A newline in an event's name or a comment's text would end the field and let whatever follows be
// read as the next one - the same shape as a header injection, in a protocol whose separator is
// also a line break. The entity name is a constant from the change log today, and this is checked
// anyway: the day one is built from something a person typed, it is checked here already.
func sseFieldValue(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
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
