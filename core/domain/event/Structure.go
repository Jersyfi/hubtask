// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The events of a collection's structure: its buckets and its labels (B-09).
//
// domain-model.md §4 names neither. It names `item.label_added` and `item.label_removed`, which are
// events about an item, and it says nothing about the columns and the vocabulary themselves - so
// these follow the naming scheme rather than a table entry, `de.hubtask.<context>.<entity>.<action>`
// with the entity the thing that changed. A board that could be rearranged without anything being
// announced would be a hole in the contract exactly where a kanban client synchronises.

// NewBucketCreated announces a new column on a collection's board.
//
// The payload is a snapshot rather than a reference, as every event in this system carries
// (domain-model.md §4): a consumer that had to fetch the bucket would produce one request per event
// and read a state that has already moved on.
func NewBucketCreated(id shared.ID, bucket work.Bucket, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return NewEnvelope(id, BucketCreated, bucket.TenantID,
		BucketSubject(bucket.ID), actor, occurredAt, cause, bucketPayload(bucket))
}

// bucketPayload is the snapshot every bucket event carries, in the API's field names, so that a
// webhook payload and a REST response describe the same column in the same words.
//
// `wip_limit` and `color_token` are explicit nulls rather than omitted, unlike a container's
// optional text fields. They are values a board renders: a client that read an absent `wip_limit`
// as "unknown" would have to fetch the bucket to find out, which is what the snapshot exists to
// avoid.
func bucketPayload(bucket work.Bucket) map[string]any {
	payload := map[string]any{
		"id":             bucket.ID.String(),
		"collection_id":  bucket.CollectionID.String(),
		"name":           bucket.Name,
		"order_key":      bucket.OrderKey,
		"wip_limit":      nil,
		"is_done_bucket": bucket.IsDoneBucket,
		"color_token":    nil,
		"deleted_at":     nil,
		"version":        bucket.Version,
	}
	if bucket.WipLimit != nil {
		payload["wip_limit"] = *bucket.WipLimit
	}
	if bucket.ColorToken != "" {
		payload["color_token"] = bucket.ColorToken
	}
	if bucket.DeletedAt != nil {
		payload["deleted_at"] = bucket.DeletedAt.UTC()
	}
	return payload
}

// BucketSubject is what a bucket event is about. Kept next to the event so that the two cannot
// drift: a consumer filtering on the subject and a producer writing it read the same line.
func BucketSubject(id shared.ID) string { return "bucket/" + id.String() }

// NewBucketUpdated announces that a column's own fields changed.
//
// A snapshot and the change set beside it, as every change event in this system carries: the
// snapshot is what the column now is, and the change set is what a field change trigger is written
// against - a rule fires on "it became the done column" or on "it stopped being one", and only the
// second needs the value that went.
func NewBucketUpdated(id shared.ID, bucket work.Bucket, changes []work.FieldChange,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newBucketChange(id, BucketUpdated, bucket, changes, actor, occurredAt, cause)
}

// NewBucketReordered announces that a column sits elsewhere on its board.
func NewBucketReordered(id shared.ID, bucket work.Bucket, changes []work.FieldChange,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newBucketChange(id, BucketReordered, bucket, changes, actor, occurredAt, cause)
}

func newBucketChange(id shared.ID, eventType Type, bucket work.Bucket,
	changes []work.FieldChange, actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if len(changes) == 0 {
		// An event announcing that nothing changed. The writer does not write when nothing moved, so
		// reaching this means the two disagree - a defect rather than something a client sent
		// (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.change_set_empty")
	}

	changeSet := make(map[string]any, len(changes))
	for _, change := range changes {
		changeSet[change.Field] = map[string]any{"from": change.From, "to": change.To}
	}

	payload := bucketPayload(bucket)
	payload["change_set"] = changeSet
	return NewEnvelope(id, eventType, bucket.TenantID,
		BucketSubject(bucket.ID), actor, occurredAt, cause, payload)
}
