// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The due event carries both sides of the move and no snapshot, and which movement it was is read
// from which side is absent: no old value is a set, no new value is a clear (domain-model.md §4).
func TestTheDueEventCarriesBothSidesOfTheMove(t *testing.T) {
	instant := func(hours int) time.Time { return occurred.Add(time.Duration(hours) * time.Hour) }

	withDue := task()
	withDue.Due = &work.DueDate{At: instant(48), DateOnly: true, TimeZone: "Europe/Berlin"}
	bare := task()

	for name, c := range map[string]struct {
		item    work.WorkItem
		old     *work.DueDate
		wantOld bool
		wantNew bool
	}{
		"a set":   {item: withDue, old: nil, wantOld: false, wantNew: true},
		"a move":  {item: withDue, old: &work.DueDate{At: instant(24)}, wantOld: true, wantNew: true},
		"a clear": {item: bare, old: &work.DueDate{At: instant(24)}, wantOld: true, wantNew: false},
	} {
		t.Run(name, func(t *testing.T) {
			envelope, err := NewItemDueChanged(eventID, c.item, c.old, by(), occurred, Cause{})
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}
			if envelope.Type != ItemDueChanged || envelope.Subject != ItemSubject(c.item.ID) {
				t.Errorf("unexpected envelope: %+v", envelope)
			}
			if _, has := envelope.Payload["old_due_at"]; has != c.wantOld {
				t.Errorf("old_due_at present=%v, want %v", has, c.wantOld)
			}
			if _, has := envelope.Payload["new_due_at"]; has != c.wantNew {
				t.Errorf("new_due_at present=%v, want %v", has, c.wantNew)
			}
			if c.wantNew {
				if envelope.Payload["time_zone"] != "Europe/Berlin" ||
					envelope.Payload["due_date_only"] != true {
					t.Errorf("the qualifiers are missing: %+v", envelope.Payload)
				}
			} else {
				if _, has := envelope.Payload["time_zone"]; has {
					t.Error("a clear still names a zone")
				}
			}
			if envelope.Payload["collection_id"] != c.item.CollectionID.String() {
				t.Errorf("the event does not name the collection: %+v", envelope.Payload)
			}
			if _, snapshot := envelope.Payload["title"]; snapshot {
				t.Error("the event carries a snapshot of the entry")
			}
		})
	}
}

// An event about nothing moving means the writer and the event disagree, which is a defect.
func TestADueEventAboutNothingMovingIsRefused(t *testing.T) {
	same := &work.DueDate{At: occurred.Add(24 * time.Hour)}
	item := task()
	item.Due = &work.DueDate{At: occurred.Add(24 * time.Hour)}

	if _, err := NewItemDueChanged(eventID, item, same, by(), occurred, Cause{}); err == nil {
		t.Fatal("an event about an unmoved due date was built")
	}
	bare := task()
	if _, err := NewItemDueChanged(eventID, bare, nil, by(), occurred, Cause{}); err == nil {
		t.Fatal("an event about no due date at all was built")
	}
}

// The two announcements the scheduler makes (D-03). One shape for both, so that a consumer reads
// them with one piece of code: what tells them apart is the type and the threshold.
func TestTheSchedulingAnnouncementsCarryTheDeadlineAndItsThreshold(t *testing.T) {
	due := occurred.Add(24 * time.Hour)
	item := task()
	item.Due = &work.DueDate{At: due, DateOnly: true, TimeZone: "Europe/Berlin"}

	for name, c := range map[string]struct {
		build         func() (Envelope, error)
		wantType      Type
		wantThreshold string
	}{
		"due soon": {
			build: func() (Envelope, error) {
				return NewItemDueSoon(eventID, item, "PT24H", occurred, Cause{})
			},
			wantType: ItemDueSoon, wantThreshold: "PT24H",
		},
		"overdue": {
			build: func() (Envelope, error) {
				return NewItemOverdue(eventID, item, occurred, Cause{})
			},
			wantType: ItemOverdue, wantThreshold: "PT0S",
		},
	} {
		t.Run(name, func(t *testing.T) {
			envelope, err := c.build()
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}

			if envelope.Type != c.wantType || envelope.Subject != ItemSubject(item.ID) {
				t.Errorf("unexpected envelope: %+v", envelope)
			}
			// Nobody caused it: the clock did, and an announcement naming a person would be a
			// notification suppressed as self-caused for somebody who did nothing.
			if envelope.Actor.Kind != shared.ActorSystem || !envelope.Actor.ID.IsZero() {
				t.Errorf("the announcement was caused by %+v", envelope.Actor)
			}
			if envelope.Payload["threshold_spec"] != c.wantThreshold {
				t.Errorf("the threshold is %v", envelope.Payload["threshold_spec"])
			}
			if envelope.Payload["due_date_only"] != true ||
				envelope.Payload["time_zone"] != "Europe/Berlin" {
				t.Errorf("the deadline reads as %+v", envelope.Payload)
			}
			if at, ok := envelope.Payload["due_at"].(time.Time); !ok || !at.Equal(due) {
				t.Errorf("the deadline is %v rather than %v", envelope.Payload["due_at"], due)
			}
		})
	}
}

// An announcement about a deadline that is not there is the caller and the event disagreeing - a
// defect rather than anything a client sent.
func TestASchedulingAnnouncementRefusesAnEntryWithoutADeadline(t *testing.T) {
	if _, err := NewItemOverdue(eventID, task(), occurred, Cause{}); err == nil {
		t.Fatal("an entry with no due date was announced overdue")
	}
	if _, err := NewItemDueSoon(eventID, task(), "PT24H", occurred, Cause{}); err == nil {
		t.Fatal("an entry with no due date was announced as approaching")
	}
}
