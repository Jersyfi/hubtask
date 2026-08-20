// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	DeleteBucketName = "DeleteBucket"

	// BucketDeletedAction is the audit code. Stable: an auditor filters on it and a SIEM rule
	// matches on it (audit.md §2).
	BucketDeletedAction audit.Action = "bucket.deleted"
)

// DeleteBucket takes a column off a board and moves its entries somewhere they still exist.
//
// It is not one of the verbs BucketWriter.change covers, and the difference is the point: this one
// writes two rows' worth of state - the column and every entry that was in it - and returns what
// became of them. A verb that only stamped the column would leave the entries pointing at something
// that is no longer on the board, and the foreign key's ON DELETE SET NULL would take a person's
// board apart silently instead.
//
// A soft delete. The row stays so that a change log entry can still name it and a restore stays
// possible (offline-sync.md §7); the partial unique index frees the name at the same moment.
type DeleteBucket struct {
	Writer BucketWriter
}

// DeleteBucketCommand is the input, typed.
type DeleteBucketCommand struct {
	BucketID shared.ID
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read none
	// and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// BucketDeletion is what became of the entries that were in the deleted column.
//
// Reported rather than left to be discovered: a board that silently reassigned a person's cards
// would be indistinguishable from one that lost them.
type BucketDeletion struct {
	BucketID shared.ID
	// TargetBucketID is the column the entries moved to. Zero when this was the last column on the
	// board, in which case the entries now carry none - the state the collection was in before
	// anybody made a board.
	TargetBucketID shared.ID
	MovedItems     int
}

// Execute deletes the column and returns what became of its entries.
func (h DeleteBucket) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DeleteBucketCommand,
) (BucketDeletion, error) {
	w := h.Writer
	if cmd.BucketID.IsZero() {
		return BucketDeletion{}, bucketIDRequired()
	}

	// The collection is read before the permission question, because the answer depends on its
	// path: a membership held at the hub applies downwards (domain-model.md §3.2). Nothing read
	// here is trusted afterwards.
	collection, err := w.readCollectionOf(ctx, actor, cmd.BucketID)
	if err != nil {
		return BucketDeletion{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(collection),
		Action:     BucketDeletedAction,
		TokenScope: bucketsWrite,
		TargetType: bucketTarget,
		TargetID:   cmd.BucketID,
	}); err != nil {
		return BucketDeletion{}, err
	}

	var result BucketDeletion
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		bucket, collection, err := w.readInTransaction(ctx, cmd.BucketID)
		if err != nil {
			return err
		}
		// I-C3: an archived collection is read-only, and so is one whose hub is archived. Re-checked
		// inside the transaction rather than trusted from the read above.
		if err := collection.EnsureEditable(); err != nil {
			return err
		}

		now := w.Clock.Now()
		deleted, changes, err := bucket.Deleted(now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// Already deleted. Nothing is written and nothing is announced, which is what makes a
			// retry after a lost response harmless - and the entries have already been moved, so
			// moving them again would take them off whatever column somebody has since put them on.
			if err := ensureBucketVersion(bucket, cmd.ExpectedVersion); err != nil {
				return err
			}
			result = BucketDeletion{BucketID: bucket.ID}
			return nil
		}

		result, err = h.write(ctx, actor, deleted, collection, changes, bucket.Version,
			cmd.ExpectedVersion, now)
		return err
	})
	if err != nil {
		return BucketDeletion{}, err
	}
	return result, nil
}

// write moves the entries, stamps the column, and records what a deletion owes: the event outwards,
// the change log for offline clients, and the audit entry - all inside the caller's transaction
// (test AT-5).
//
// The entries move before the column is stamped. Not for correctness - the transaction makes the
// two one write either way - but because the fallback is "the leftmost live column other than this
// one", and stamping first would make the deleted column a candidate for nothing while leaving the
// query to answer a question about a board it is already off.
func (h DeleteBucket) write(
	ctx context.Context, actor appshared.ActorContext, deleted domain.Bucket,
	collection domain.Container, changes []domain.FieldChange,
	currentVersion, expectedVersion int, now time.Time,
) (BucketDeletion, error) {
	w := h.Writer

	target, err := h.fallbackFor(ctx, deleted)
	if err != nil {
		return BucketDeletion{}, err
	}

	moved, err := w.Buckets.MoveItems(ctx, deleted.ID, target, now)
	if err != nil {
		return BucketDeletion{}, err
	}

	expected := expectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still what the update matches on.
		expected = currentVersion
	}
	if err := w.Buckets.SetDeleted(ctx, deleted, expected); err != nil {
		return BucketDeletion{}, err
	}
	deleted.Version = expected + 1

	announcement, err := event.NewBucketDeleted(w.IDs.NewID(), deleted, target, moved,
		event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return BucketDeletion{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return BucketDeletion{}, err
	}
	if err := h.recordChange(ctx, deleted, collection, actor); err != nil {
		return BucketDeletion{}, err
	}
	if err := h.recordAudit(ctx, deleted, actor, target, moved, changes, now); err != nil {
		return BucketDeletion{}, err
	}

	return BucketDeletion{BucketID: deleted.ID, TargetBucketID: target, MovedItems: moved}, nil
}

// fallbackFor is where a deleted column's entries go: the leftmost column left on the board.
//
// Derived rather than read from a stored `default_bucket_id`. That key is documented on the
// policies document and no use case writes it, so a stored default would be a value nothing keeps
// up to date - a column deleted while the key still named it would send entries into a bucket that
// is no longer on the board. An empty board is not a failure: the entries then carry no column,
// which is the state the collection was in before anybody made one.
func (h DeleteBucket) fallbackFor(
	ctx context.Context, deleted domain.Bucket,
) (shared.ID, error) {
	target, err := h.Writer.Buckets.FirstOther(ctx, deleted.CollectionID, deleted.ID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return target.ID, nil
}

// recordChange writes what an offline client has to be told: the column is gone.
//
// A deletion rather than an upsert, and it carries no payload - there is nothing left to describe,
// and a tombstone with content would be a copy of the deleted object living on in the log
// (offline-sync.md §7). What became of the entries is not in this entry: each of them is its own
// row with its own merge, and a client learns their new column from the event or by reading them.
func (h DeleteBucket) recordChange(
	ctx context.Context, bucket domain.Bucket, collection domain.Container,
	actor appshared.ActorContext,
) error {
	return h.Writer.Changes.Record(ctx, changelog.Change{
		TenantID: bucket.TenantID,
		Entity:   bucketTarget,
		EntityID: bucket.ID,
		Op:       changelog.Delete,
		// The visibility filter a pull applies: the hub above the collection, so that a device
		// subscribed to the hub sees the column disappear (offline-sync.md §3.1).
		ContainerID: firstNonZero(collection.ParentID, bucket.CollectionID),
		ActorID:     actor.AccountID,
		HLC:         h.Writer.HLC.Next(),
	})
}

// recordAudit writes the evidence: that the column went, and where its entries went with it.
//
// The destination and the count are recorded in clear text. They are identifiers and a number this
// installation produced rather than anything a person typed, and "how many entries did this move"
// is precisely what an auditor asking about a deletion needs (audit.md §4). The stamp itself is
// open for the same reason.
func (h DeleteBucket) recordAudit(
	ctx context.Context, bucket domain.Bucket, actor appshared.ActorContext,
	target shared.ID, moved int, changes []domain.FieldChange, now time.Time,
) error {
	recorded := make([]audit.Change, 0, len(changes)+3)
	for _, change := range changes {
		recorded = append(recorded, audit.Change{
			Field: change.Field, Classification: audit.Open, From: change.From, To: change.To,
		})
	}
	recorded = append(recorded,
		audit.Change{
			Field: "collection_id", Classification: audit.Open, To: bucket.CollectionID.String(),
		},
		audit.Change{Field: "target_bucket_id", Classification: audit.Open, To: idOrNil(target)},
		audit.Change{Field: "moved_items", Classification: audit.Open, To: moved},
	)

	return h.Writer.Audit.Append(ctx, audit.Entry{
		TenantID:   bucket.TenantID,
		OccurredAt: now,
		Action:     BucketDeletedAction,
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

// deletionOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema BucketDeletion).
func deletionOutput(deletion BucketDeletion) usecase.Output {
	return usecase.Output{
		"bucket_id":        deletion.BucketID.String(),
		"target_bucket_id": idOrNil(deletion.TargetBucketID),
		"moved_items":      deletion.MovedItems,
	}
}

// Descriptor is the catalogue entry.
func (h DeleteBucket) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteBucketName,
		Summary: "Takes a column off a collection's board. Its entries are not deleted with it: " +
			"they move to the leftmost remaining column, or out of any column when this was the " +
			"last one. What became of them is in the answer. Idempotent: deleting a column that is " +
			"already gone succeeds, writes nothing and moves nothing.",
		SideEffects: "Moves the entries out of the column, writes its deletion stamp, announces " +
			string(event.BucketDeleted) + ", records a deletion for offline clients, and writes an " +
			"audit entry.",
		TokenScope: bucketsWrite,
		Input: []usecase.Field{
			{
				Name: "bucket_id", Kind: usecase.KindID, Required: true,
				Description: "The column to take off the board.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read, from the If-Match header over REST. Omitted " +
					"means the caller read none and accepts whatever is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: BucketDeletedAction, TargetType: bucketTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a board's columns are the collection's configuration. Nothing happened to any " +
				"entry: what moved is where the columns sit, not what somebody did to their work.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteBucket) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	bucketID, err := in.ID("bucket_id")
	if err != nil {
		return nil, err
	}

	deletion, err := h.Execute(ctx, actor, DeleteBucketCommand{
		BucketID:        bucketID,
		ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return deletionOutput(deletion), nil
}
