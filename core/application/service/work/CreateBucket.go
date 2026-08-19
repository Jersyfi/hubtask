// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateBucketName = "CreateBucket"
	ListBucketsName  = "ListBuckets"
	bucketTarget     = "bucket"

	// The token scopes. A bucket is structure rather than content, so it shares the container's
	// scopes: a token that may reorganise a workspace may add a column to a board, and one that may
	// read the workspace may read it (security.md §5).
	bucketsWrite = containersWrite
	bucketsRead  = containersRead

	// BucketCreatedAction is the audit code. Stable: an auditor filters on it and a SIEM rule
	// matches on it (audit.md §2).
	BucketCreatedAction audit.Action = "bucket.created"
	// BucketReadAction is the audit code of an attempted read. Declared even though an ordinary read
	// writes no entry: a *refused* read does, and it is recorded against the action that was refused
	// rather than against a generic "denied" (audit.md §4).
	BucketReadAction audit.Action = "bucket.read"
)

// CreateBucket adds a column to a collection's board.
//
// Only a collection has a board. A hub holds collections and no items, so a column on one would
// have nothing to hold - which is the same reasoning that keeps items off a hub, and it is
// Container.EnsureAcceptsItems that answers it.
type CreateBucket struct {
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
}

// CreateBucketCommand is the input, typed.
type CreateBucketCommand struct {
	CollectionID shared.ID
	Name         string

	// WipLimit is a number of which zero means "no limit", exactly as the empty string means "not
	// set" for the text fields (domain.BucketAttributes).
	WipLimit     int
	IsDoneBucket bool
	ColorToken   string
	// BeforeBucketID is the column the new one goes to the left of. Empty means the right hand end,
	// which is where a board grows unless somebody says otherwise.
	BeforeBucketID shared.ID
}

// Execute creates the bucket and returns it.
func (h CreateBucket) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateBucketCommand,
) (domain.Bucket, error) {
	if cmd.CollectionID.IsZero() {
		return domain.Bucket{}, collectionIDRequired()
	}

	// The collection is read before the permission question, because the answer depends on it: a
	// membership held at the hub applies downwards, so a path naming only the collection would
	// refuse somebody who does hold the right (domain-model.md §3.2). Nothing read here is trusted
	// afterwards - the state that decides the write is read again inside the transaction.
	collection, err := h.readCollection(ctx, actor, cmd.CollectionID)
	if err != nil {
		return domain.Bucket{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       containerPath(collection),
		Action:     BucketCreatedAction,
		TokenScope: bucketsWrite,
		TargetType: bucketTarget,
		// The bucket does not exist yet, so the refusal names the board it would have joined.
		TargetID: cmd.CollectionID,
	}); err != nil {
		return domain.Bucket{}, err
	}

	var created domain.Bucket
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		collection, err := findNamedCollection(ctx, h.Containers, cmd.CollectionID)
		if err != nil {
			return err
		}
		// The same gate a new item passes: a hub has no board, and a trashed or archived collection
		// is read-only (I-C2, I-C3). Re-checked inside the transaction rather than trusted from the
		// read above - the collection may have been archived since.
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}

		orderKey, err := h.orderKey(ctx, cmd.CollectionID, cmd.BeforeBucketID)
		if err != nil {
			return err
		}

		bucket, err := domain.NewBucket(domain.NewBucketInput{
			ID:           h.IDs.NewID(),
			TenantID:     actor.TenantID,
			CollectionID: cmd.CollectionID,
			Name:         cmd.Name,
			OrderKey:     orderKey,
			WipLimit:     &cmd.WipLimit,
			IsDoneBucket: cmd.IsDoneBucket,
			ColorToken:   cmd.ColorToken,
		})
		if err != nil {
			return err
		}

		if err := h.Buckets.Insert(ctx, bucket); err != nil {
			return err
		}

		now := h.Clock.Now()
		// One snapshot, two recipients. The event goes outwards as a public contract, the change
		// log goes to synchronising clients (offline-sync.md §10) - but they describe the same
		// state, and building it twice is how the two come to disagree.
		announcement, err := event.NewBucketCreated(
			h.IDs.NewID(), bucket, event.Actor{Kind: actor.Kind, ID: actor.AccountID},
			now, event.Cause{})
		if err != nil {
			return err
		}
		if err := h.Events.Append(ctx, announcement); err != nil {
			return err
		}
		if err := h.recordChange(ctx, bucket, collection, actor, announcement.Payload); err != nil {
			return err
		}
		if err := h.recordAudit(ctx, bucket, actor, now); err != nil {
			return err
		}

		created = bucket
		return nil
	})
	if err != nil {
		return domain.Bucket{}, err
	}
	return created, nil
}

// orderKey ranks the new column: to the left of the one named, or after the last one on the board.
//
// The two are one question asked of the same neighbour lookup, so that "insert here" and "append"
// cannot end up with two different notions of what a rank is. Appending reads the last rank instead
// of the bounds, because that is the cheaper half of the same answer and it is the common case.
func (h CreateBucket) orderKey(
	ctx context.Context, collectionID, beforeID shared.ID,
) (string, error) {
	if beforeID.IsZero() {
		last, err := h.Buckets.LastOrderKey(ctx, collectionID)
		if err != nil {
			return "", err
		}
		return service.OrderKeyAfter(last)
	}

	// Nothing is moving, so no bucket is excluded from its own board: the new column has no rank to
	// be measured against yet.
	previous, next, err := h.Buckets.Neighbours(ctx, collectionID, beforeID, "")
	if err != nil {
		return "", err
	}
	if next == "" {
		// The anchor is not on this board - deleted, in another collection, or invented. Refused
		// rather than appended: a client that positioned a column and received a 201 would believe
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

// readCollection reads the collection outside the write transaction, because the permission check
// needs it first. Read-only, so it may be served by a replica (multi-tenancy.md §7).
func (h CreateBucket) readCollection(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Container, error) {
	var collection domain.Container

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findNamedCollection(ctx, h.Containers, id)
		collection = found
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}
	return collection, nil
}

// recordChange writes what an offline client has to be told (offline-sync.md §3.1).
//
// The merge rule for every field of a bucket is last writer wins per field, decided by the HLC -
// which is the rule the Definition of Done asks to be stated for each new field. `order_key` is a
// fractional index and merges by itself, and `version` is derived server-side and never merged. A
// bucket carries no set of its own; the one set in this milestone is an item's labels, which merge
// as an OR-set (work.MergeSetElements).
func (h CreateBucket) recordChange(
	ctx context.Context, bucket domain.Bucket, collection domain.Container,
	actor appshared.ActorContext, snapshot map[string]any,
) error {
	return h.Changes.Record(ctx, changelog.Change{
		TenantID: bucket.TenantID,
		Entity:   bucketTarget,
		EntityID: bucket.ID,
		Op:       changelog.Upsert,
		// The visibility filter a pull applies: the hub above the collection, so that a device
		// subscribed to the hub sees the new column appear.
		ContainerID: firstNonZero(collection.ParentID, bucket.CollectionID),
		ActorID:     actor.AccountID,
		HLC:         h.HLC.Next(),
		Payload:     snapshot,
	})
}

// recordAudit writes the evidence, inside the same transaction as the change (test AT-5).
//
// A bucket's name is user content, so the trail records it as a fingerprint rather than in clear
// text: enough to see that two entries concern the same column, not enough to read it (rule 10,
// audit.md §4). The limit and the done marker are not - they are a number and a boolean this
// installation defined, and an auditor asking "when did this board start meaning done" has no other
// way to answer it.
func (h CreateBucket) recordAudit(
	ctx context.Context, bucket domain.Bucket, actor appshared.ActorContext, now time.Time,
) error {
	var limit any
	if bucket.WipLimit != nil {
		limit = *bucket.WipLimit
	}

	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   bucket.TenantID,
		OccurredAt: now,
		Action:     BucketCreatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: bucketTarget,
		TargetID:   bucket.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{
				Field: "collection_id", Classification: audit.Open,
				To: bucket.CollectionID.String(),
			},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: bucket.Name},
			audit.Change{Field: domain.FieldWipLimit, Classification: audit.Open, To: limit},
			audit.Change{
				Field: domain.FieldIsDoneBucket, Classification: audit.Open, To: bucket.IsDoneBucket,
			},
		),
	})
}

// findNamedCollection reads the collection a client named, or says it does not exist in the words a
// client can act on. Shared by every bucket and label use case, all of which are anchored to one.
//
// Distinct from findCollection, which reads the collection *under* an item that exists - where a
// missing row is a defect rather than a client's mistake (ADR-0024).
func findNamedCollection(
	ctx context.Context, containers repository.Containers, id shared.ID,
) (domain.Container, error) {
	collection, err := containers.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The same answer whether it does not exist or belongs to another tenant. Anything else
			// would confirm the existence of another tenant's data (multi-tenancy.md §2).
			return domain.Container{}, shared.ErrNotFound.
				WithDetail("items.collection_not_found").
				WithParams(map[string]string{"collection_id": id.String()}).
				WithFields(shared.FieldError{
					Path: "/collection_id", Code: "items.collection_not_found",
				})
		}
		return domain.Container{}, err
	}
	return collection, nil
}

func collectionIDRequired() error {
	return shared.ErrValidation.
		WithDetail("items.collection_id_required").
		WithFields(shared.FieldError{Path: "/collection_id", Code: "items.collection_id_required"})
}

// bucketOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema Bucket), so that a REST response, an MCP tool result and an automation
// action result describe the column in the same words.
//
// `wip_limit` and `color_token` are explicit nulls rather than omitted, unlike a container's
// optional text: they are values a board renders, and a client that read an absent limit as
// "unknown" would have to fetch the bucket to find out.
func bucketOutput(bucket domain.Bucket) usecase.Output {
	out := usecase.Output{
		"id":             bucket.ID.String(),
		"collection_id":  bucket.CollectionID.String(),
		"name":           bucket.Name,
		"order_key":      bucket.OrderKey,
		"wip_limit":      nil,
		"is_done_bucket": bucket.IsDoneBucket,
		"color_token":    nil,
		"version":        bucket.Version,
	}
	if bucket.WipLimit != nil {
		out["wip_limit"] = *bucket.WipLimit
	}
	if bucket.ColorToken != "" {
		out["color_token"] = bucket.ColorToken
	}
	return out
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h CreateBucket) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateBucketName,
		Summary: "Adds a column to a collection's board. The name has to be unique in the " +
			"collection, compared without regard to case or accents. Only a collection has a " +
			"board: a hub holds collections and no entries, so a column on one would have nothing " +
			"to hold. The new column goes to the right of the existing ones.",
		SideEffects: "Writes the bucket, announces " + string(event.BucketCreated) +
			", records a change for offline clients, and writes an audit entry.",
		TokenScope: bucketsWrite,
		Input: []usecase.Field{
			{
				Name: "collection_id", Kind: usecase.KindID, Required: true,
				Description: "The collection whose board the column joins.",
			},
			{
				Name: "name", Kind: usecase.KindString, Required: true,
				Description: "Up to 120 characters, on one line, free in this collection.",
			},
			{
				Name: "wip_limit", Kind: usecase.KindInt,
				Description: "How many entries this column should hold at once. Advisory: nothing " +
					"refuses the entry that exceeds it, the column is meant to turn red. Omitted " +
					"or zero means no limit.",
			},
			{
				Name: "is_done_bucket", Kind: usecase.KindBool,
				Description: "Whether this column means finished. Stored and reported; completing " +
					"an entry is its own operation.",
			},
			{
				Name: "color_token", Kind: usecase.KindString,
				Description: "A theme token rather than a colour value, so clients render it in " +
					"their own palette.",
			},
			{
				Name: "before_bucket_id", Kind: usecase.KindID,
				Description: "The column the new one goes to the left of. Omitted puts it at the " +
					"right hand end, which is where a board grows unless somebody says otherwise.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: BucketCreatedAction, TargetType: bucketTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all three
// channels at once.
func (h CreateBucket) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}
	beforeID, err := in.ID("before_bucket_id")
	if err != nil {
		return nil, err
	}

	bucket, err := h.Execute(ctx, actor, CreateBucketCommand{
		CollectionID:   collectionID,
		Name:           in.String("name"),
		WipLimit:       in.Int("wip_limit"),
		IsDoneBucket:   in.Bool("is_done_bucket"),
		ColorToken:     in.String("color_token"),
		BeforeBucketID: beforeID,
	})
	if err != nil {
		return nil, err
	}
	return bucketOutput(bucket), nil
}

// ListBuckets reads a collection's board.
//
// Read-only throughout, which is not a detail: the transaction may be served by a read replica
// (multi-tenancy.md §7), and a read that opened a write transaction would pin every board in the
// product to the primary.
type ListBuckets struct {
	Buckets    repository.Buckets
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListBucketsQuery is the input, typed.
type ListBucketsQuery struct {
	CollectionID shared.ID
}

// Execute returns the collection's board, left to right.
//
// Not paged, unlike the container and item lists: a board has as many columns as fit on a screen,
// and the contract returns a plain array (api-guidelines.md §2). A collection that grows thousands
// of them has a different problem than pagination solves.
func (h ListBuckets) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListBucketsQuery,
) ([]domain.Bucket, error) {
	if query.CollectionID.IsZero() {
		return nil, collectionIDRequired()
	}

	// The collection is read first, because the permission question needs its path: a membership at
	// the hub applies downwards (domain-model.md §3.2).
	var collection domain.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := findNamedCollection(ctx, h.Containers, query.CollectionID)
		collection = found
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     BucketReadAction,
		TokenScope: bucketsRead,
		TargetType: bucketTarget,
		TargetID:   query.CollectionID,
	}); err != nil {
		return nil, err
	}

	var board []domain.Bucket
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		board, err = h.Buckets.List(ctx, query.CollectionID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return board, nil
}

// Descriptor is the catalogue entry.
func (h ListBuckets) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListBucketsName,
		Summary: "Lists a collection's board, left to right. Deleted columns are not in it. Not " +
			"paged: a board has as many columns as fit on a screen. A hub has no board and comes " +
			"back empty.",
		SideEffects: "None. Reads only.",
		TokenScope:  bucketsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "collection_id", Kind: usecase.KindID, Required: true,
				Description: "The collection whose board is wanted.",
			},
		},
		// Required is false and the action is still named: an ordinary read is not an auditable
		// event (audit.md §4 lists none), and a refused one is - recorded by the authorisation
		// service against this action.
		Audit: usecase.AuditDeclaration{
			Action: BucketReadAction, TargetType: bucketTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListBuckets) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}

	board, err := h.Execute(ctx, actor, ListBucketsQuery{CollectionID: collectionID})
	if err != nil {
		return nil, err
	}

	data := make([]usecase.Output, 0, len(board))
	for _, bucket := range board {
		data = append(data, bucketOutput(bucket))
	}
	return usecase.Output{"data": data}, nil
}
