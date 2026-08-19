// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var eventBucket = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")

func eventBucketIn(collection shared.ID) work.Bucket {
	return work.Bucket{
		ID: eventBucket, TenantID: eventTenant, CollectionID: collection,
		Name: "Doing", OrderKey: "a1", Version: 1,
	}
}

// The snapshot is what a consumer reacts to. A consumer that had to fetch the column would produce
// one request per event and read a state that has already moved on.
func TestTheBucketCreatedEventCarriesTheColumn(t *testing.T) {
	bucket := eventBucketIn(eventCollection)

	envelope, err := NewBucketCreated(eventID, bucket, by(), occurred, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	if envelope.Type != BucketCreated {
		t.Errorf("event type %s", envelope.Type)
	}
	if envelope.Subject != BucketSubject(eventBucket) {
		t.Errorf("subject %q", envelope.Subject)
	}
	if envelope.TenantID != eventTenant {
		t.Errorf("the event names tenant %s", envelope.TenantID)
	}

	payload := envelope.Payload
	if payload["id"] != eventBucket.String() || payload["collection_id"] != eventCollection.String() {
		t.Errorf("the event describes another column: %+v", payload)
	}
	if payload["name"] != "Doing" || payload["order_key"] != "a1" {
		t.Errorf("unexpected payload: %+v", payload)
	}
	if payload["is_done_bucket"] != false {
		t.Errorf("is_done_bucket is %v", payload["is_done_bucket"])
	}
}

// The values a board renders travel as explicit nulls rather than as omissions: a subscriber that
// had to tell "no limit" from "this producer does not know about limits" would have to fetch the
// column, which is what the snapshot exists to avoid.
func TestAnUnsetBoardValueTravelsAsNull(t *testing.T) {
	envelope, err := NewBucketCreated(eventID, eventBucketIn(eventCollection), by(), occurred, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	for _, field := range []string{"wip_limit", "color_token", "deleted_at"} {
		value, present := envelope.Payload[field]
		if !present {
			t.Errorf("%s is absent rather than null", field)
		}
		if value != nil {
			t.Errorf("%s is %v, want null", field, value)
		}
	}
}

func TestASetBoardValueTravelsAsItsValue(t *testing.T) {
	limit := 4
	deleted := occurred.Add(time.Hour)

	bucket := eventBucketIn(eventCollection)
	bucket.WipLimit, bucket.IsDoneBucket = &limit, true
	bucket.ColorToken, bucket.DeletedAt = "surface.green", &deleted

	envelope, err := NewBucketCreated(eventID, bucket, by(), occurred, Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	payload := envelope.Payload
	switch {
	case payload["wip_limit"] != 4:
		t.Errorf("wip_limit is %v", payload["wip_limit"])
	case payload["color_token"] != "surface.green":
		t.Errorf("color_token is %v", payload["color_token"])
	case payload["is_done_bucket"] != true:
		t.Errorf("is_done_bucket is %v", payload["is_done_bucket"])
	case payload["deleted_at"] != deleted.UTC():
		t.Errorf("deleted_at is %v", payload["deleted_at"])
	}
}

// The snapshot and the change set answer different questions: the snapshot is what the column now
// is, and the change set is what a field change trigger is written against - a rule fires on "it
// became the done column" or on "it stopped being one", and only the second needs the value that
// went.
func TestTheBucketChangeEventsCarryBothTheSnapshotAndTheChangeSet(t *testing.T) {
	bucket := eventBucketIn(eventCollection)
	renamed := []work.FieldChange{{Field: work.FieldName, From: "Doing", To: "In progress"}}

	events := map[Type]func() (Envelope, error){
		BucketUpdated: func() (Envelope, error) {
			return NewBucketUpdated(eventID, bucket, renamed, by(), occurred, Cause{})
		},
		BucketReordered: func() (Envelope, error) {
			return NewBucketReordered(eventID, bucket,
				[]work.FieldChange{{Field: work.FieldOrderKey, From: "a0", To: "a1"}},
				by(), occurred, Cause{})
		},
	}

	for eventType, build := range events {
		t.Run(string(eventType), func(t *testing.T) {
			envelope, err := build()
			if err != nil {
				t.Fatalf("building the event: %v", err)
			}
			if envelope.Type != eventType {
				t.Errorf("event type %s", envelope.Type)
			}
			if envelope.Payload["id"] != eventBucket.String() {
				t.Errorf("the event describes another column: %v", envelope.Payload["id"])
			}
			changeSet, _ := envelope.Payload["change_set"].(map[string]any)
			if len(changeSet) != 1 {
				t.Fatalf("the change set is %+v, want one field", changeSet)
			}
		})
	}
}

// An event announcing that nothing changed means the writer and the event disagree, which is a
// defect rather than something a client sent.
func TestABucketEventRefusesAnEmptyChangeSet(t *testing.T) {
	if _, err := NewBucketReordered(
		eventID, eventBucketIn(eventCollection), nil, by(), occurred, Cause{}); err == nil {
		t.Fatal("an event with no change set was built")
	}
}
