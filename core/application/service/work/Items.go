// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// The questions every writer in this package asks before it writes, in one place.
//
// They were a copy per writer, identical to the character, which is the state where a fix reaches
// one of them: "the item is missing" and "the collection under it is missing" are answered
// differently on purpose - one is a client's mistake and the other is a defect - and that
// distinction is worth exactly one implementation.

// findItem reads an item, or says it does not exist in the words a client can act on.
func findItem(ctx context.Context, items repository.Items, id shared.ID) (domain.WorkItem, error) {
	item, err := items.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.WorkItem{}, shared.ErrNotFound.
				WithDetail("items.not_found").
				WithParams(map[string]string{"item_id": id.String()})
		}
		return domain.WorkItem{}, err
	}
	return item, nil
}

// findCollection reads the collection an item already belongs to. A missing one under an item that
// exists is a defect rather than a client's mistake: a tenant-scoped foreign key makes it
// unreachable (ADR-0024).
func findCollection(
	ctx context.Context, containers repository.Containers, id shared.ID,
) (domain.Container, error) {
	collection, err := containers.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Container{}, shared.ErrInternal.
				WithDetail("items.collection_missing").WithCause(err)
		}
		return domain.Container{}, err
	}
	return collection, nil
}

// profileOf reads the capability profile in force for one type.
//
// Through the hierarchy rather than off the list directly, for the reason NewHierarchy documents:
// read off a narrowed set alone, the topology comes out wrong. Only the profile is wanted here,
// and building it the same way is what keeps this answer and CreateWorkItem's the same answer.
func profileOf(
	ctx context.Context, profiles metarepo.CapabilityProfiles, itemType domain.ItemType,
) (domain.CapabilityProfile, error) {
	inForce, err := profiles.List(ctx)
	if err != nil {
		return domain.CapabilityProfile{}, err
	}
	system, err := profiles.ListSystem(ctx)
	if err != nil {
		return domain.CapabilityProfile{}, err
	}
	hierarchy, err := service.NewHierarchy(inForce, system)
	if err != nil {
		return domain.CapabilityProfile{}, err
	}
	return hierarchy.Profile(itemType)
}

// ensureExpectedVersion refuses a caller writing against a version that has moved on, even when the
// change it asked for would have been a no-op.
//
// Zero means the caller read no version and accepts whatever is there (api-guidelines.md §5). That
// is not the same as skipping the check: the version in hand is still what the update matches on,
// so a concurrent write between the read and the write is caught either way.
func ensureExpectedVersion(item domain.WorkItem, expected int) error {
	if expected == 0 || expected == item.Version {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("items.version_conflict").
		WithParams(map[string]string{
			"item_id": item.ID.String(), "current_version": strconv.Itoa(item.Version),
		})
}

// ensureBucketOnBoard refuses a column that is not on the collection's board.
//
// Invariant I-W6, checked before the write rather than left to the foreign key: the key would
// accept a column of another collection in the same tenant, and the entry would then render on a
// board it is not on. A deleted column is refused for the same reason - it is off the board, and an
// entry put into one would be nowhere a client draws.
func ensureBucketOnBoard(
	ctx context.Context, buckets repository.Buckets, collectionID, bucketID shared.ID,
) error {
	bucket, err := buckets.Find(ctx, bucketID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrNotFound.
				WithDetail("buckets.not_found").
				WithParams(map[string]string{"bucket_id": bucketID.String()}).
				WithFields(shared.FieldError{Path: "/bucket_id", Code: "buckets.not_found"})
		}
		return err
	}
	if bucket.CollectionID != collectionID {
		return shared.ErrValidation.
			WithDetail("buckets.not_in_collection").
			WithParams(map[string]string{
				"bucket_id": bucketID.String(), "collection_id": collectionID.String(),
			}).
			WithFields(shared.FieldError{Path: "/bucket_id", Code: "buckets.not_in_collection"})
	}
	return bucket.EnsureEditable()
}

// ensureAccountCanSee refuses an account that cannot reach the entry.
//
// Assignment and membership are both decisions about a second person, and one made about somebody
// who gets a 404 on the entry is a piece of work nobody can do - and, once C-04 lands, a
// contributor's write right pointing at nothing. So the account has to hold a membership somewhere
// along the entry's path (domain-model.md §3.2).
//
// One refusal for three situations, deliberately: no membership, another tenant's account, and an
// account that does not exist all come back as `items.account_without_access`. Separating them
// would be an oracle for which identifiers exist in which tenant, which is exactly what T-04
// forbids (multi-tenancy.md §2) - and the fix a client needs is the same in all three cases.
func ensureAccountCanSee(
	ctx context.Context, visibility Visibility, actor appshared.ActorContext,
	accountID shared.ID, collection domain.Container,
) error {
	permitted, err := visibility.CanSee(ctx, actor, accountID, containerPath(collection))
	if err != nil {
		return err
	}
	if permitted {
		return nil
	}
	return shared.ErrValidation.
		WithDetail("items.account_without_access").
		WithParams(map[string]string{"account_id": accountID.String()}).
		WithFields(shared.FieldError{Path: "/account_id", Code: "items.account_without_access"})
}
