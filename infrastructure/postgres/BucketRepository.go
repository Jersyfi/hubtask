// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// BucketRepository stores the columns of a collection's board.
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010) - which is why the cross-tenant tests in
// test/integration are the ones that prove this file correct, not a unit test with a fake.
type BucketRepository struct{}

func NewBucketRepository() BucketRepository { return BucketRepository{} }

var _ repository.Buckets = BucketRepository{}

// bucketNameIndex is the constraint that decides whether a name is free in one collection. Named
// explicitly, so that a different unique violation - one added by a later migration - is not
// reported to the client as a duplicate name.
const bucketNameIndex = "bucket_name_uq"

// Find returns the bucket as it is stored, a deleted one included.
func (r BucketRepository) Find(ctx context.Context, id shared.ID) (work.Bucket, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.Bucket{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return work.Bucket{}, err
	}

	row, err := queries.FindBucket(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			// Also the answer when the row belongs to another tenant: row level security removed
			// it from the result, and the caller must not be able to tell the two apart
			// (multi-tenancy.md §2).
			return work.Bucket{}, shared.ErrNotFound.WithDetail("buckets.not_found")
		}
		return work.Bucket{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the bucket: %w", err))
	}
	return bucketFrom(row)
}

// List returns a collection's board, left to right.
func (r BucketRepository) List(
	ctx context.Context, collectionID shared.ID,
) ([]work.Bucket, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListBuckets(ctx, collection)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the buckets: %w", err))
	}

	buckets := make([]work.Bucket, 0, len(rows))
	for _, row := range rows {
		bucket, err := bucketFrom(row)
		if err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, nil
}

// LastOrderKey returns the highest rank on the board, or the empty string when it is empty.
func (r BucketRepository) LastOrderKey(
	ctx context.Context, collectionID shared.ID,
) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return "", err
	}

	key, err := queries.LastBucketOrderKey(ctx, collection)
	if err != nil {
		if IsNoRows(err) {
			// An empty board, not a failure: the first column has nothing to sort after.
			return "", nil
		}
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the last bucket order key: %w", err))
	}
	return key, nil
}

// Insert writes the bucket.
//
// The name check is the unique index rather than a query before the insert: a check followed by an
// insert is two statements with a gap, and two requests arriving inside that gap both pass the
// check.
func (r BucketRepository) Insert(ctx context.Context, bucket work.Bucket) error {
	queries, id, err := structureWrite(ctx, bucket.ID)
	if err != nil {
		return err
	}
	collection, err := uuidOf(bucket.CollectionID)
	if err != nil {
		return err
	}

	err = queries.InsertBucket(ctx, sqlc.InsertBucketParams{
		ID:           id,
		CollectionID: collection,
		Name:         bucket.Name,
		OrderKey:     bucket.OrderKey,
		WipLimit:     optionalCount(bucket.WipLimit),
		IsDoneBucket: bucket.IsDoneBucket,
		ColorToken:   optionalText(bucket.ColorToken),
	})
	if err == nil {
		return nil
	}
	return bucketWriteError(err, bucket, "writing the bucket")
}

// SetAttributes writes a bucket's own fields.
func (r BucketRepository) SetAttributes(
	ctx context.Context, bucket work.Bucket, expectedVersion int,
) error {
	queries, id, err := structureWrite(ctx, bucket.ID)
	if err != nil {
		return err
	}

	affected, err := queries.SetBucketAttributes(ctx, sqlc.SetBucketAttributesParams{
		Name:         bucket.Name,
		WipLimit:     optionalCount(bucket.WipLimit),
		IsDoneBucket: bucket.IsDoneBucket,
		ColorToken:   optionalText(bucket.ColorToken),
		ID:           id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return bucketWriteError(err, bucket, "writing the bucket attributes")
	}
	return bucketConflict(affected, bucket, expectedVersion)
}

// SetOrderKey writes a new rank for one bucket.
func (r BucketRepository) SetOrderKey(
	ctx context.Context, bucket work.Bucket, expectedVersion int,
) error {
	queries, id, err := structureWrite(ctx, bucket.ID)
	if err != nil {
		return err
	}

	affected, err := queries.SetBucketOrderKey(ctx, sqlc.SetBucketOrderKeyParams{
		OrderKey: bucket.OrderKey,
		ID:       id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return bucketWriteError(err, bucket, "writing the bucket order key")
	}
	return bucketConflict(affected, bucket, expectedVersion)
}

// SetDeleted writes the bucket's deletion stamp.
func (r BucketRepository) SetDeleted(
	ctx context.Context, bucket work.Bucket, expectedVersion int,
) error {
	queries, id, err := structureWrite(ctx, bucket.ID)
	if err != nil {
		return err
	}

	// A nil stamp would be a bucket that is not deleted, which is not what this method is for. The
	// domain has already set it; passing the zero time would write "deleted in the year one".
	if bucket.DeletedAt == nil {
		return shared.ErrInternal.WithDetail("buckets.identity_incomplete")
	}

	affected, err := queries.SetBucketDeleted(ctx, sqlc.SetBucketDeletedParams{
		DeletedAt: timestampOf(*bucket.DeletedAt),
		ID:        id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return bucketWriteError(err, bucket, "writing the bucket deletion")
	}
	return bucketConflict(affected, bucket, expectedVersion)
}

// Neighbours returns the ranks either side of a position on one board.
func (r BucketRepository) Neighbours(
	ctx context.Context, collectionID, beforeID, movingID shared.ID,
) (string, string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", "", err
	}

	collection, err := uuidOf(collectionID)
	if err != nil {
		return "", "", err
	}
	before, err := optionalUUID(beforeID)
	if err != nil {
		return "", "", err
	}
	moving, err := uuidOf(movingID)
	if err != nil {
		return "", "", err
	}

	bounds, err := queries.BucketOrderKeyNeighbours(ctx, sqlc.BucketOrderKeyNeighboursParams{
		CollectionID: collection,
		MovingID:     moving,
		BeforeID:     before,
	})
	if err != nil {
		return "", "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the bucket rank bounds: %w", err))
	}
	return bounds.PreviousKey, bounds.NextKey, nil
}

// FirstOther returns the collection's leftmost remaining bucket, ignoring one.
func (r BucketRepository) FirstOther(
	ctx context.Context, collectionID, excludedID shared.ID,
) (work.Bucket, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.Bucket{}, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return work.Bucket{}, err
	}
	excluded, err := uuidOf(excludedID)
	if err != nil {
		return work.Bucket{}, err
	}

	row, err := queries.FirstBucket(ctx, sqlc.FirstBucketParams{
		CollectionID: collection,
		ExcludedID:   excluded,
	})
	if err != nil {
		if IsNoRows(err) {
			// The column being deleted was the last one. Not a failure: the items then carry no
			// bucket, which is the state the collection was in before anybody made a board.
			return work.Bucket{}, shared.ErrNotFound.WithDetail("buckets.not_found")
		}
		return work.Bucket{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the first bucket: %w", err))
	}
	return bucketFrom(row)
}

// MoveItems moves every item out of one bucket and into another.
func (r BucketRepository) MoveItems(
	ctx context.Context, sourceID, targetID shared.ID, at time.Time,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	source, err := uuidOf(sourceID)
	if err != nil {
		return 0, err
	}
	// A zero target takes the items out of any bucket at all, which is what the last column of a
	// board leaves behind.
	target, err := optionalUUID(targetID)
	if err != nil {
		return 0, err
	}

	affected, err := queries.MoveItemsBetweenBuckets(ctx, sqlc.MoveItemsBetweenBucketsParams{
		TargetBucketID: target,
		UpdatedAt:      timestampOf(at),
		SourceBucketID: source,
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("moving the items out of %s: %w", sourceID, err))
	}
	return int(affected), nil
}

// structureWrite is the preamble every bucket and label write shares: the queries of the caller's
// transaction, and the identifier as the driver's type.
func structureWrite(ctx context.Context, id shared.ID) (*sqlc.Queries, pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	return queries, key, nil
}

// bucketWriteError translates what the driver reports. A name already taken is the unique index
// speaking, and it is the one failure a client can act on.
//
// No name in the message: the error text reaches the log, and a bucket's name is user content
// (rule 10, ADR-0017). The conflict carries it, because that answer goes to the client that sent it.
func bucketWriteError(err error, bucket work.Bucket, what string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == bucketNameIndex {
		return shared.ErrConflict.
			WithDetail("buckets.name_taken").
			WithParams(map[string]string{"name": bucket.Name})
	}
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s of %s: %w", what, bucket.ID, err))
}

// bucketConflict turns "no row matched" into the version conflict it is. A row belonging to another
// tenant is the same answer: row level security removed it from the update's reach, and a caller
// must not be able to tell that apart from a version that moved (multi-tenancy.md §2).
func bucketConflict(affected int64, bucket work.Bucket, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("buckets.version_conflict").
		WithParams(map[string]string{
			"bucket_id": bucket.ID.String(), "expected_version": strconv.Itoa(expectedVersion),
		})
}

func bucketFrom(row sqlc.Bucket) (work.Bucket, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return work.Bucket{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return work.Bucket{}, err
	}
	collectionID, err := idFrom(row.CollectionID)
	if err != nil {
		return work.Bucket{}, err
	}

	return work.Bucket{
		ID:           id,
		TenantID:     tenantID,
		CollectionID: collectionID,
		Name:         row.Name,
		OrderKey:     row.OrderKey,
		WipLimit:     countFrom(row.WipLimit),
		IsDoneBucket: row.IsDoneBucket,
		ColorToken:   textFrom(row.ColorToken),
		DeletedAt:    optionalTime(row.DeletedAt),
		Version:      int(row.Version),
	}, nil
}

// optionalCount turns "no limit" into SQL NULL. The domain says nil, and the column's CHECK refuses
// zero, so there is no value that could stand in for the absence.
func optionalCount(value *int) *int32 {
	if value == nil {
		return nil
	}
	//nolint:gosec // G115: a work in progress limit is a small positive number the domain has checked
	count := int32(*value)
	return &count
}

func countFrom(value *int32) *int {
	if value == nil {
		return nil
	}
	count := int(*value)
	return &count
}

// textFrom is the counterpart of optionalText: SQL NULL is the empty string, which is what the
// domain reads as "not set".
func textFrom(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
