// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	UpdateBucketName  = "UpdateBucket"
	ReorderBucketName = "ReorderBucket"

	// The audit codes. Two rather than one, for the reason the container's rename and policy update
	// have two: "who renamed this column" and "who rearranged the board" are different questions of
	// the trail, and an auditor filtering on one must not have to read the change list of the other
	// to find out which it is looking at (audit.md §2).
	BucketUpdatedAction   audit.Action = "bucket.updated"
	BucketReorderedAction audit.Action = "bucket.reordered"
)

// BucketWriter is what every use case that changes an existing column shares.
//
// One dependency set rather than one per verb: they read the same bucket, ask the same permission
// question of the same collection, and owe the same four writes - the row, the event, the change
// log entry and the audit entry. What differs between them is which domain method decides the new
// state, which is what bucketChange carries.
type BucketWriter struct {
	Buckets    repository.Buckets
	Containers repository.Containers
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
	// Text brings the names people type to normal form C on the way in (i18n-l10n.md §5).
	Text text.Normalizer
	// Boards keeps one done column per board and places a deleted column's entries where the board
	// says (domain-model.md §3.5a); unwired, a done mark is only the column's own.
	Boards Boards
	// Activity writes the history of an entry a moved done mark completes; unwired, none.
	Activity ActivityJournal
}

// causeDoneMoved is the history's why of an entry completed because the done mark moved onto the
// column it stands in.
const causeDoneMoved = "DONE_COLUMN_MOVED"

// settleDone keeps column and completion together once the done mark moved from one column to
// another (domain-model.md §3.5a): the completed entries of the old done column go with the mark,
// and the open ones standing in the new done column are completed, each with its history.
func (w BucketWriter) settleDone(
	ctx context.Context, actor appshared.ActorContext, collectionID, from, to shared.ID, now time.Time,
) error {
	if w.Boards.Statuses == nil || to.IsZero() {
		return nil
	}
	if !from.IsZero() {
		moved, err := w.Boards.Statuses.PlaceByCompletion(ctx, collectionID, true, from, to, now)
		if err != nil {
			return err
		}
		if err := w.recordEntries(ctx, actor, collectionID, moved, domain.FieldBucketID, idOrNil(to)); err != nil {
			return err
		}
	}
	completed, err := w.Boards.Statuses.CompleteInColumn(ctx, collectionID, to, actor.AccountID, now)
	if err != nil {
		return err
	}
	return w.recordCompleted(ctx, actor, collectionID, completed, causeDoneMoved, now)
}

// recordCompleted writes what completing entries in bulk owes each of them: its change for offline
// clients and its history - who: the actor, why: the cause. No completion event is announced, so
// neither a notification nor a rule runs for an entry nobody completed by hand (§3.5a).
func (w BucketWriter) recordCompleted(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, entries []repository.MovedEntry, cause string, now time.Time,
) error {
	if err := w.recordEntries(ctx, actor, collectionID, entries, domain.FieldCompletion, map[string]any{
		"is_completed": true, "completed_at": now.UTC(), "completed_by": actor.AccountID.String(),
	}); err != nil {
		return err
	}
	if w.Activity.Entries == nil {
		return nil
	}
	for _, entry := range entries {
		if err := w.Activity.RecordStep(ctx, actor, actor.TenantID, entry.ID, collectionID, activity.ItemCompleted,
			activity.ChangeSet(activity.Full, activity.Field{Name: "cause", Detail: activity.WithValues, To: cause}), now); err != nil {
			return err
		}
	}
	return nil
}

// recordEntries tells offline clients the field a bulk write set on each entry.
func (w BucketWriter) recordEntries(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, entries []repository.MovedEntry, field string, value any,
) error {
	for _, entry := range entries {
		if err := w.Changes.Record(ctx, changelog.Change{
			TenantID: actor.TenantID, Entity: itemTarget, EntityID: entry.ID, Op: changelog.Upsert,
			ContainerID: collectionID, ActorID: actor.AccountID, HLC: w.HLC.Next(),
			Field: field, Payload: map[string]any{field: value, "version": entry.Version},
		}); err != nil {
			return err
		}
	}
	return nil
}

// UpdateBucket changes a column's own fields.
//
// What it does not change: where the column sits, which is ReorderBucket. A single endpoint writing
// both would need one audit entry covering everything and one event nobody could subscribe to
// narrowly.
type UpdateBucket struct {
	Writer BucketWriter
}

// ReorderBucket moves a column along its board.
type ReorderBucket struct {
	Writer BucketWriter
}

// UpdateBucketCommand is the input, typed.
type UpdateBucketCommand struct {
	BucketID shared.ID
	// Attributes carries a pointer per field, so that "set it to nothing" and "do not touch it" stay
	// two different requests all the way down from the merge patch that expressed them.
	Attributes domain.BucketAttributes
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read none
	// and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// ReorderBucketCommand is the input, typed.
type ReorderBucketCommand struct {
	BucketID shared.ID
	// BeforeBucketID is the column this one goes to the left of. Empty is the right hand end, which
	// is a position like any other rather than a special case.
	BeforeBucketID  shared.ID
	ExpectedVersion int
}

// mayMark refuses clearing the only done column of a board (Board.MarkDone).
func (h UpdateBucket) mayMark(ctx context.Context, bucket domain.Bucket, done *bool) error {
	if done == nil || !h.Writer.Boards.Wired() {
		return nil
	}
	board, err := h.Writer.Boards.Of(ctx, bucket.CollectionID)
	if err != nil {
		return err
	}
	_, err = board.MarkDone(bucket.ID, *done)
	return err
}

// unmarkOthers takes the done mark off the board's other columns once one is marked, each
// written with everything a change to a column owes.
func (h UpdateBucket) unmarkOthers(
	ctx context.Context, actor appshared.ActorContext, marked domain.Bucket, collection domain.Container,
) error {
	if !marked.IsDoneBucket || !h.Writer.Boards.Wired() {
		return nil
	}
	return h.Writer.unmarkOthers(ctx, actor, marked.CollectionID, marked.ID, collection)
}

// unmarkOthers takes the done mark off every column of a board but keep.
func (w BucketWriter) unmarkOthers(
	ctx context.Context, actor appshared.ActorContext, collectionID, keep shared.ID, collection domain.Container,
) error {
	board, err := w.Boards.Of(ctx, collectionID)
	if err != nil {
		return err
	}
	others, err := board.MarkDone(keep, true)
	if err != nil {
		return err
	}
	cleared := false
	for _, other := range others {
		if err := w.setDone(ctx, actor, other.ID, &cleared, collection); err != nil {
			return err
		}
		if err := w.settleDone(ctx, actor, collectionID, other.ID, keep, w.Clock.Now()); err != nil {
			return err
		}
	}
	return nil
}

// setDone writes one column's done mark as an update of that column.
func (w BucketWriter) setDone(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, done *bool, collection domain.Container,
) error {
	current, err := w.Buckets.Find(ctx, id)
	if err != nil {
		return err
	}
	wanted, changes, err := current.Updated(domain.BucketAttributes{IsDoneBucket: done}, w.Text)
	if err != nil || len(changes) == 0 {
		return err
	}
	_, err = w.write(ctx, actor, bucketChange{
		bucketID: id, action: BucketUpdatedAction, store: repository.Buckets.SetAttributes,
		announce: func(id shared.ID, bucket domain.Bucket, changes []domain.FieldChange, by event.Actor, at time.Time) (event.Envelope, error) {
			return event.NewBucketUpdated(id, bucket, changes, by, at, event.Cause{})
		},
		classification: audit.Sensitive,
	}, wanted, collection, changes, current.Version)
	return err
}

// Execute changes the column and returns it as it now stands.
func (h UpdateBucket) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateBucketCommand,
) (domain.Bucket, error) {
	return h.Writer.change(ctx, actor, bucketChange{
		bucketID:        cmd.BucketID,
		action:          BucketUpdatedAction,
		expectedVersion: cmd.ExpectedVersion,
		apply: func(ctx context.Context, bucket domain.Bucket) (domain.Bucket, []domain.FieldChange, error) {
			if err := h.mayMark(ctx, bucket, cmd.Attributes.IsDoneBucket); err != nil {
				return domain.Bucket{}, nil, err
			}
			return bucket.Updated(cmd.Attributes, h.Writer.Text)
		},
		follow: h.unmarkOthers,
		store:  repository.Buckets.SetAttributes,
		announce: func(id shared.ID, bucket domain.Bucket, changes []domain.FieldChange,
			by event.Actor, at time.Time,
		) (event.Envelope, error) {
			return event.NewBucketUpdated(id, bucket, changes, by, at, event.Cause{})
		},
		// The name is user content, so the trail records that it changed and a hash of each side
		// rather than the value (audit.md §4). The limit and the done marker travel with it and are
		// not user content - but one classification decides the whole entry, and hashing a boolean
		// costs an auditor nothing that the change set does not already say.
		classification: audit.Sensitive,
	})
}

// Execute moves the column and returns it as it now stands.
func (h ReorderBucket) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ReorderBucketCommand,
) (domain.Bucket, error) {
	return h.Writer.change(ctx, actor, bucketChange{
		bucketID:        cmd.BucketID,
		action:          BucketReorderedAction,
		expectedVersion: cmd.ExpectedVersion,
		apply: func(ctx context.Context, bucket domain.Bucket) (domain.Bucket, []domain.FieldChange, error) {
			orderKey, err := h.Writer.rankFor(ctx, bucket, cmd.BeforeBucketID)
			if err != nil {
				return domain.Bucket{}, nil, err
			}
			return bucket.Reordered(orderKey)
		},
		store: repository.Buckets.SetOrderKey,
		announce: func(id shared.ID, bucket domain.Bucket, changes []domain.FieldChange,
			by event.Actor, at time.Time,
		) (event.Envelope, error) {
			return event.NewBucketReordered(id, bucket, changes, by, at, event.Cause{})
		},
		// A rank is a fractional index this server produced. There is no personal data in "a1", and
		// an auditor asking when a board was rearranged has no other way to answer it.
		classification: audit.Open,
	})
}

// rankFor is where a column lands: between the one named and whatever is below it, or after the
// last one when nothing is named.
//
// The moving column is excluded from its own board: a reorder would otherwise measure the position
// against the rank it is leaving, and "put it back where it is" would have no room to land in.
//
// A repeat is harmless rather than free. The board looks the same either way, but the key it lands
// on is a fresh one between the same neighbours, so the row is written again - which is what a
// fractional index costs, and the same thing ReorderWorkItem does for the same reason.
func (w BucketWriter) rankFor(
	ctx context.Context, bucket domain.Bucket, beforeID shared.ID,
) (string, error) {
	if beforeID == bucket.ID {
		// A column before itself has no position. Cheap to check, and the neighbour lookup would
		// otherwise answer with the bounds of a board this column has been removed from.
		return "", shared.ErrValidation.
			WithDetail("buckets.before_bucket_is_self").
			WithParams(map[string]string{"bucket_id": bucket.ID.String()}).
			WithFields(shared.FieldError{
				Path: "/before_bucket_id", Code: "buckets.before_bucket_is_self",
			})
	}

	previous, next, err := w.Buckets.Neighbours(ctx, bucket.CollectionID, beforeID, bucket.ID)
	if err != nil {
		return "", err
	}
	if !beforeID.IsZero() && next == "" {
		// The anchor is not on this board - deleted, in another collection, or invented. Refused
		// rather than appended: a client that positioned a column and received a 200 would believe
		// the board is in an order it is not.
		return "", shared.ErrValidation.
			WithDetail("buckets.before_bucket_not_on_board").
			WithParams(map[string]string{"bucket_id": beforeID.String()}).
			WithFields(shared.FieldError{
				Path: "/before_bucket_id", Code: "buckets.before_bucket_not_on_board",
			})
	}
	return service.OrderKeyBetween(previous, next)
}

// bucketChange is one verb's differences from the others: what it applies, what it stores, what it
// announces, and how sensitive what it changed is.
type bucketChange struct {
	bucketID        shared.ID
	action          audit.Action
	expectedVersion int
	apply           func(context.Context, domain.Bucket) (domain.Bucket, []domain.FieldChange, error)
	store           func(repository.Buckets, context.Context, domain.Bucket, int) error
	announce        func(shared.ID, domain.Bucket, []domain.FieldChange, event.Actor, time.Time) (event.Envelope, error)
	classification  audit.Classification
	// follow writes what the change owes the rest of the board, in its transaction; nil for none.
	follow func(context.Context, appshared.ActorContext, domain.Bucket, domain.Container) error
}

// change is the whole of what a change to an existing column owes, once.
func (w BucketWriter) change(
	ctx context.Context, actor appshared.ActorContext, change bucketChange,
) (domain.Bucket, error) {
	if change.bucketID.IsZero() {
		return domain.Bucket{}, bucketIDRequired()
	}

	// The bucket and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	collection, err := w.readCollectionOf(ctx, actor, change.bucketID)
	if err != nil {
		return domain.Bucket{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(collection),
		Action:     change.action,
		TokenScope: bucketsWrite,
		TargetType: bucketTarget,
		TargetID:   change.bucketID,
	}); err != nil {
		return domain.Bucket{}, err
	}

	var updated domain.Bucket
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		bucket, collection, err := w.readInTransaction(ctx, change.bucketID)
		if err != nil {
			return err
		}
		// I-C3: an archived collection is read-only, and so is one whose hub is archived. Re-checked
		// inside the transaction rather than trusted from the read above - it may have been archived
		// since the client last looked.
		if err := collection.EnsureEditable(); err != nil {
			return err
		}

		wanted, changes, err := change.apply(ctx, bucket)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// The column already says what the caller asked it to say. Nothing is written, no version
			// is spent and nothing is announced - which is what makes a client that echoes the whole
			// object back harmless rather than merely accepted.
			//
			// The If-Match is still honoured: a caller writing against a version somebody else has
			// moved on is told so even when its own change would have been a no-op, because the state
			// it was reasoning about is not the state that is there.
			if err := ensureBucketVersion(bucket, change.expectedVersion); err != nil {
				return err
			}
			updated = bucket
			return nil
		}

		if updated, err = w.write(ctx, actor, change, wanted, collection, changes, bucket.Version); err != nil {
			return err
		}
		if change.follow == nil {
			return nil
		}
		return change.follow(ctx, actor, updated, collection)
	})
	if err != nil {
		return domain.Bucket{}, err
	}
	return updated, nil
}

// write stores the change and records what it owes: the event outwards, the change log for offline
// clients, and the audit entry - all inside the caller's transaction (test AT-5).
func (w BucketWriter) write(
	ctx context.Context, actor appshared.ActorContext, change bucketChange,
	after domain.Bucket, collection domain.Container, changes []domain.FieldChange, currentVersion int,
) (domain.Bucket, error) {
	expected := change.expectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still the one the update matches on, so a concurrent write
		// between the read and here is still caught.
		expected = currentVersion
	}
	if err := change.store(w.Buckets, ctx, after, expected); err != nil {
		return domain.Bucket{}, err
	}
	after.Version = expected + 1

	now := w.Clock.Now()
	// Built from the stored state rather than from the command, so that what the event says and what
	// the row holds cannot disagree.
	announcement, err := change.announce(
		w.IDs.NewID(), after, changes, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now)
	if err != nil {
		return domain.Bucket{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return domain.Bucket{}, err
	}
	if err := w.recordChanges(ctx, after, collection, actor, changes); err != nil {
		return domain.Bucket{}, err
	}
	if err := w.recordAudit(ctx, after, actor, change, changes, now); err != nil {
		return domain.Bucket{}, err
	}
	return after, nil
}

// recordChanges writes what an offline client has to be told: one entry per field that moved.
//
// One entry per field rather than one carrying them all, because the merge rule for these fields is
// last writer wins *per field* (offline-sync.md §4.2). Each entry takes its own HLC, so a device
// that renamed a column while another set its limit keeps both changes - which is precisely what one
// entry covering both would destroy, the later HLC deciding the whole payload and silently
// discarding the other device's field.
//
// A cleared field travels as null rather than as the empty string, which is what the API spells it.
// `order_key` is the exception among the fields here: it is a fractional index and is never empty,
// so it always travels as its value.
func (w BucketWriter) recordChanges(
	ctx context.Context, bucket domain.Bucket, collection domain.Container,
	actor appshared.ActorContext, changes []domain.FieldChange,
) error {
	for _, change := range changes {
		err := w.Changes.Record(ctx, changelog.Change{
			TenantID: bucket.TenantID,
			Entity:   bucketTarget,
			EntityID: bucket.ID,
			Op:       changelog.Upsert,
			// The visibility filter a pull applies: the column's own collection, the choice every
			// change to an entry makes - a grant on the collection alone is on the path of the
			// collection and not of its hub (offline-sync.md §3.1).
			ContainerID: bucket.CollectionID,
			ActorID:     actor.AccountID,
			HLC:         w.HLC.Next(),
			Field:       change.Field,
			Payload:     map[string]any{change.Field: clearedAsNull(change.To)},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// recordAudit writes the evidence: which fields changed, and - where they are not user content -
// what they now say.
func (w BucketWriter) recordAudit(
	ctx context.Context, bucket domain.Bucket, actor appshared.ActorContext,
	change bucketChange, changes []domain.FieldChange, now time.Time,
) error {
	recorded := make([]audit.Change, 0, len(changes)+1)
	for _, moved := range changes {
		recorded = append(recorded, audit.Change{
			Field: moved.Field, Classification: change.classification,
			From: moved.From, To: moved.To,
		})
	}
	recorded = append(recorded, audit.Change{
		Field: "collection_id", Classification: audit.Open, To: bucket.CollectionID.String(),
	})

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   bucket.TenantID,
		OccurredAt: now,
		Action:     change.action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: bucketTarget,
		TargetID:   bucket.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(recorded...),
	})
}

// readCollectionOf reads the collection a column belongs to, outside the write transaction, because
// the permission check needs its path first. Read-only, so it may be served by a replica
// (multi-tenancy.md §7).
func (w BucketWriter) readCollectionOf(
	ctx context.Context, actor appshared.ActorContext, bucketID shared.ID,
) (domain.Container, error) {
	var collection domain.Container

	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, found, err := w.readInTransaction(ctx, bucketID)
		collection = found
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return collection, nil
}

// readInTransaction reads the column and the collection it sits on. The collection is read through
// findCollection rather than findNamedCollection: the client named the bucket, and a bucket whose
// collection is missing is a defect that a tenant-scoped foreign key makes unreachable (ADR-0024).
func (w BucketWriter) readInTransaction(
	ctx context.Context, bucketID shared.ID,
) (domain.Bucket, domain.Container, error) {
	bucket, err := findBucket(ctx, w.Buckets, bucketID)
	if err != nil {
		return domain.Bucket{}, domain.Container{}, err
	}
	collection, err := findCollection(ctx, w.Containers, bucket.CollectionID)
	if err != nil {
		return domain.Bucket{}, domain.Container{}, err
	}
	return bucket, collection, nil
}

// findBucket reads a column a client named, or says it does not exist in the words a client can act
// on.
func findBucket(
	ctx context.Context, buckets repository.Buckets, id shared.ID,
) (domain.Bucket, error) {
	bucket, err := buckets.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The same answer whether it does not exist or belongs to another tenant. Anything else
			// would confirm the existence of another tenant's data (multi-tenancy.md §2).
			return domain.Bucket{}, shared.ErrNotFound.
				WithDetail("buckets.not_found").
				WithParams(map[string]string{"bucket_id": id.String()})
		}
		return domain.Bucket{}, err
	}
	return bucket, nil
}

func bucketIDRequired() error {
	return shared.ErrValidation.
		WithDetail("buckets.bucket_id_required").
		WithFields(shared.FieldError{Path: "/bucket_id", Code: "buckets.bucket_id_required"})
}

// ensureBucketVersion refuses a caller writing against a version that has moved on, even when the
// change it asked for would have been a no-op. Zero means the caller read no version and accepts
// whatever is there (api-guidelines.md §5).
func ensureBucketVersion(bucket domain.Bucket, expected int) error {
	if expected == 0 || expected == bucket.Version {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("buckets.version_conflict").
		WithParams(map[string]string{
			"bucket_id": bucket.ID.String(), "current_version": strconv.Itoa(bucket.Version),
		})
}

// Descriptor is the catalogue entry.
func (h UpdateBucket) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionStructure,
		Name:       UpdateBucketName,
		Summary: "Changes a column's own fields: what it is called, how much it should hold, " +
			"whether it means finished, what colour it is. A field that is not sent is left alone; " +
			"sending one as empty clears it. Where the column sits is not here - that is " +
			"ReorderBucket. Idempotent: an update that asks for what is already stored succeeds, " +
			"writes nothing and announces nothing.",
		SideEffects: "Writes the changed fields, announces " + string(event.BucketUpdated) +
			" with a change set, records one change per field for offline clients, and writes an " +
			"audit entry.",
		TokenScope: bucketsWrite,
		Input: []usecase.Field{
			{
				Name: "bucket_id", Kind: usecase.KindID, Required: true,
				Description: "The column to change.",
			},
			{
				Name: "name", Kind: usecase.KindString,
				Description: "The new name: one line, at most 120 characters, not empty, and free " +
					"in this collection. Omitted leaves the name as it is.",
			},
			{
				Name: "wip_limit", Kind: usecase.KindInt,
				Description: "The new limit. Zero removes it, omitted leaves it as it is.",
			},
			{
				Name: "is_done_bucket", Kind: usecase.KindBool,
				Description: "Whether this column means finished. Omitted leaves it as it is.",
			},
			{
				Name: "color_token", Kind: usecase.KindString,
				Description: "A theme token rather than a colour value. Empty clears it, omitted " +
					"leaves it as it is.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The version last read, from the If-Match header over REST. Omitted " +
					"means the caller read none and accepts whatever is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: BucketUpdatedAction, TargetType: bucketTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a board's columns are the collection's configuration. Nothing happened to any " +
				"entry: what moved is where the columns sit, not what somebody did to their work.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all three
// channels at once.
func (h UpdateBucket) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	bucketID, err := in.ID("bucket_id")
	if err != nil {
		return nil, err
	}

	// The optional readers rather than the plain ones, because the difference between the two is
	// this use case's whole contract: a caller that sent no `color_token` wants it left alone, and
	// one that sent an empty `color_token` wants it gone.
	cmd := UpdateBucketCommand{
		BucketID: bucketID,
		Attributes: domain.BucketAttributes{
			Name:       in.OptionalString("name"),
			ColorToken: in.OptionalString("color_token"),
		},
		ExpectedVersion: in.Int("expected_version"),
	}
	// Present rather than a value read, for the two fields whose value carries no "absent"
	// spelling of its own: 0 is how a caller clears the limit and false is a state somebody may
	// want, so neither can stand in for "the field was not sent".
	if in.Present("wip_limit") {
		limit := in.Int("wip_limit")
		cmd.Attributes.WipLimit = &limit
	}
	if in.Present("is_done_bucket") {
		done := in.Bool("is_done_bucket")
		cmd.Attributes.IsDoneBucket = &done
	}
	if cmd.Attributes.IsEmpty() {
		return nil, shared.ErrValidation.
			WithDetail("buckets.update_empty").
			WithFields(shared.FieldError{Path: "/", Code: "buckets.update_empty"})
	}

	bucket, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return bucketOutput(bucket), nil
}

// Descriptor is the catalogue entry.
func (h ReorderBucket) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionStructure,
		Name:       ReorderBucketName,
		Summary: "Moves a column along its board. An omitted before_bucket_id moves it to the right " +
			"hand end. A column cannot leave its collection, so this is the whole of what moving one " +
			"means. Asking for the position it already holds succeeds and leaves the board looking " +
			"the same; the rank it is given may differ, because a rank is a key between its " +
			"neighbours rather than a number.",
		SideEffects: "Writes the new rank, announces " + string(event.BucketReordered) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: bucketsWrite,
		Input: []usecase.Field{
			{
				Name: "bucket_id", Kind: usecase.KindID, Required: true,
				Description: "The column to move.",
			},
			{
				Name: "before_bucket_id", Kind: usecase.KindID,
				Description: "The column this one goes to the left of. Omitted moves it to the " +
					"right hand end.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The version last read, from the If-Match header over REST.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: BucketReorderedAction, TargetType: bucketTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a board's columns are the collection's configuration. Nothing happened to any " +
				"entry: what moved is where the columns sit, not what somebody did to their work.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReorderBucket) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	bucketID, err := in.ID("bucket_id")
	if err != nil {
		return nil, err
	}
	beforeID, err := in.ID("before_bucket_id")
	if err != nil {
		return nil, err
	}

	bucket, err := h.Execute(ctx, actor, ReorderBucketCommand{
		BucketID:        bucketID,
		BeforeBucketID:  beforeID,
		ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return bucketOutput(bucket), nil
}
