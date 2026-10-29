// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-PLN-01 check 8: drawing and removing a link announces it, by reference and without a title.
func TestTheLinkEventsCarryBothEndsAndNoTitle(t *testing.T) {
	source := task()
	link := work.EntryLink{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000003a1"), TenantID: source.TenantID,
		Type: work.LinkTypeBlocks, SourceID: source.ID,
		TargetID: shared.MustParseID("0192f000-0000-7000-8000-0000000003a2"),
	}
	for eventType, build := range map[Type]func() (Envelope, error){
		ItemLinked:   func() (Envelope, error) { return NewItemLinked(eventID, link, source, by(), occurred, Cause{}) },
		ItemUnlinked: func() (Envelope, error) { return NewItemUnlinked(eventID, link, source, by(), occurred, Cause{}) },
	} {
		t.Run(string(eventType), func(t *testing.T) {
			envelope, err := build()
			if err != nil {
				t.Fatal(err)
			}
			if envelope.Type != eventType || envelope.Subject != ItemSubject(source.ID) {
				t.Errorf("envelope %+v", envelope)
			}
			if envelope.Payload["link_type"] != "blocks" || envelope.Payload["target_id"] != link.TargetID.String() ||
				envelope.Payload["link_id"] != link.ID.String() {
				t.Errorf("payload %+v", envelope.Payload)
			}
			if _, snapshot := envelope.Payload["title"]; snapshot {
				t.Error("the event carries a title")
			}
		})
	}
	other := link
	other.SourceID = link.TargetID
	if _, err := NewItemLinked(eventID, other, source, by(), occurred, Cause{}); err == nil {
		t.Error("an event about a link the entry does not start was built")
	}
}
