// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
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
			// The same answer the authorisation service gives for an entry out of reach, from the
			// same constructor: two spellings of "not found" would be an oracle for which
			// identifiers exist (T-04, appshared.ItemNotFound).
			return domain.WorkItem{}, appshared.ItemNotFound(id)
		}
		return domain.WorkItem{}, err
	}
	return item, nil
}

// readItemScope reads the entry a request is about together with the collection it lives in,
// read-only and outside any write transaction, because the permission check needs both first: the
// collection for the path a membership is resolved along, and the entry for the assignee the
// narrowing is measured against (C-04).
//
// One helper rather than the copy each writer held. Those copies read the entry and threw it away,
// which was harmless while nothing about the entry decided anything - and stopped being harmless
// the moment "assigned only" became a rule rather than a sentence in a document.
//
// Nothing it reads is trusted afterwards: the state that decides the write is read again inside the
// transaction that writes, since anything read before it can have changed by the time it commits.
func readItemScope(
	ctx context.Context, uow persistence.UnitOfWork, items repository.Items,
	containers repository.Containers, actor appshared.ActorContext, itemID shared.ID,
) (domain.WorkItem, domain.Container, error) {
	var (
		item       domain.WorkItem
		collection domain.Container
	)

	err := uow.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findItem(ctx, items, itemID)
		if err != nil {
			return err
		}
		item = found
		collection, err = findCollection(ctx, containers, found.CollectionID)
		return err
	})
	if err != nil {
		return domain.WorkItem{}, domain.Container{}, err
	}
	return item, collection, nil
}

// changing, commenting and reading name what a request does to an entry, for the one decision
// point that applies the matrix's qualifiers to it (access.ItemSubject, C-04).
//
// Three constructors rather than a literal at every call site, because the assignee is the field
// that is easy to leave out and impossible to notice missing: an omitted one reads as "this entry
// belongs to nobody", which refuses a contributor on their own work.
func changing(item domain.WorkItem) access.ItemSubject {
	return access.ItemSubject{Does: service.ItemChange, ID: item.ID, Assignee: item.AssigneeID}
}

func commenting(item domain.WorkItem) access.ItemSubject {
	return access.ItemSubject{Does: service.ItemComment, ID: item.ID, Assignee: item.AssigneeID}
}

func reading(item domain.WorkItem) access.ItemSubject {
	return access.ItemSubject{Does: service.ItemRead, ID: item.ID, Assignee: item.AssigneeID}
}

// findParentItem reads the entry a request wants to place something under, where absent is the
// caller's mistake and says so - unlike the entry being acted on, whose absence is a plain
// not-found.
func findParentItem(
	ctx context.Context, items repository.Items, id shared.ID,
) (domain.WorkItem, error) {
	parent, err := items.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.WorkItem{}, shared.ErrNotFound.
				WithDetail("items.parent_not_found").
				WithParams(map[string]string{"parent_id": id.String()}).
				WithFields(shared.FieldError{Path: "/target_parent_id", Code: "items.parent_not_found"})
		}
		return domain.WorkItem{}, err
	}
	return parent, nil
}

// findTargetCollection reads a collection a placement named, where absent is the caller's mistake
// and says so - unlike the collection an entry already sits in, whose absence is a defect.
//
// Distinct from findNamedCollection, which answers the same question for the operations anchored to
// a collection: the field the finding hangs on is `target_collection_id` here and `collection_id`
// there, and a client that highlights the field it sent has to be told the one it sent.
func findTargetCollection(
	ctx context.Context, containers repository.Containers, id shared.ID,
) (domain.Container, error) {
	collection, err := containers.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Container{}, shared.ErrNotFound.
				WithDetail("items.collection_not_found").
				WithParams(map[string]string{"collection_id": id.String()}).
				WithFields(shared.FieldError{
					Path: "/target_collection_id", Code: "items.collection_not_found",
				})
		}
		return domain.Container{}, err
	}
	return collection, nil
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
