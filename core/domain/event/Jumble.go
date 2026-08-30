// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// JumbleEntrySubject names what a jumble event is about, the way every subject here is built.
func JumbleEntrySubject(id shared.ID) string { return "jumble_entry/" + id.String() }

// NewJumbleEntryReceived announces an arrival in the jumble (domain-model.md §4, G-10). Consumers:
// automation - it is what fires a JUMBLE_ENTRY rule - webhooks, and the AI suggestions of 0.7.0.
//
// The payload deliberately carries no content and no sender. An event leaves the installation, and
// the raw text of a mail is exactly the PERSONAL_CONTENT rule 10 keeps out of everything that
// travels (data-protection.md); a subscriber that needs the entry reads it back over the API with
// its own credential, and a rule's conditions read the fields as data through the run's `payload`,
// which never leaves the process.
func NewJumbleEntryReceived(
	id shared.ID, entry jumble.Entry, actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return NewEnvelope(id, JumbleEntryReceived, entry.TenantID,
		JumbleEntrySubject(entry.ID), actor, occurredAt, cause,
		map[string]any{
			"id":               entry.ID.String(),
			"channel":          entry.Channel.String(),
			"attachment_count": len(entry.Attachments),
			"received_at":      entry.ReceivedAt.UTC(),
		})
}

// NewJumbleEntryConverted announces that an entry became work: the conversion produced an item at
// a named destination and settled the entry (domain-model.md §4). Consumers: automation, webhooks.
//
// The payload names the entry and the target item - the provenance pair - and the collection the
// item was created in, so a scoped rule can be matched the way every item event is.
func NewJumbleEntryConverted(
	id shared.ID, entry jumble.Entry, collectionID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if entry.Status != jumble.StatusProcessed || entry.TargetItemID.IsZero() {
		// A conversion event about an unconverted entry is the writer and the event disagreeing,
		// which is a defect rather than something a client sent (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.jumble_entry_not_converted")
	}

	payload := map[string]any{
		"id":             entry.ID.String(),
		"channel":        entry.Channel.String(),
		"target_item_id": entry.TargetItemID.String(),
	}
	if !collectionID.IsZero() {
		payload["collection_id"] = collectionID.String()
	}
	return NewEnvelope(id, JumbleEntryConverted, entry.TenantID,
		JumbleEntrySubject(entry.ID), actor, occurredAt, cause, payload)
}
