// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// buckets is the fake board. It answers the rank questions from what is stored rather than from a
// canned pair, because where a new column lands is the thing these tests are about - fixed bounds
// would hide it.
type buckets struct {
	inserted []domain.Bucket
	stored   map[shared.ID]domain.Bucket
	// written is what a change use case wrote, in order, with the version it was written against: a
	// test that only saw the row could not tell a write that honoured an If-Match from one that
	// ignored it.
	written []writtenBucket
	// placedBefore is the column Neighbours was asked about.
	placedBefore shared.ID
	// moved records what MoveItems was asked to do.
	moved []movedItems

	findErr   error
	listErr   error
	insertErr error
	writeErr  error
}

type writtenBucket struct {
	method   string
	bucket   domain.Bucket
	expected int
}

type movedItems struct {
	source, target shared.ID
	at             time.Time
}

func (b *buckets) Find(_ context.Context, id shared.ID) (domain.Bucket, error) {
	if b.findErr != nil {
		return domain.Bucket{}, b.findErr
	}
	bucket, found := b.stored[id]
	if !found {
		return domain.Bucket{}, shared.ErrNotFound
	}
	return bucket, nil
}

func (b *buckets) List(_ context.Context, collection shared.ID) ([]domain.Bucket, error) {
	if b.listErr != nil {
		return nil, b.listErr
	}

	board := make([]domain.Bucket, 0, len(b.stored))
	for _, bucket := range b.stored {
		if bucket.CollectionID == collection && !bucket.IsDeleted() {
			board = append(board, bucket)
		}
	}
	sortBuckets(board)
	return board, nil
}

func (b *buckets) LastOrderKey(_ context.Context, collection shared.ID) (string, error) {
	var last string
	for _, bucket := range b.stored {
		if bucket.CollectionID == collection && bucket.OrderKey > last {
			last = bucket.OrderKey
		}
	}
	return last, nil
}

func (b *buckets) Insert(_ context.Context, bucket domain.Bucket) error {
	if b.insertErr != nil {
		return b.insertErr
	}
	b.inserted = append(b.inserted, bucket)
	b.stored[bucket.ID] = bucket
	return nil
}

func (b *buckets) SetAttributes(_ context.Context, bucket domain.Bucket, expected int) error {
	return b.write("attributes", bucket, expected)
}

func (b *buckets) SetOrderKey(_ context.Context, bucket domain.Bucket, expected int) error {
	return b.write("order_key", bucket, expected)
}

func (b *buckets) SetDeleted(_ context.Context, bucket domain.Bucket, expected int) error {
	return b.write("deleted", bucket, expected)
}

func (b *buckets) write(method string, bucket domain.Bucket, expected int) error {
	if b.writeErr != nil {
		return b.writeErr
	}
	b.written = append(b.written, writtenBucket{method: method, bucket: bucket, expected: expected})
	b.stored[bucket.ID] = bucket
	return nil
}

func (b *buckets) Neighbours(
	_ context.Context, collection, beforeID, movingID shared.ID,
) (string, string, error) {
	b.placedBefore = beforeID

	var anchor string
	for _, bucket := range b.stored {
		if bucket.ID == beforeID && bucket.CollectionID == collection && !bucket.IsDeleted() {
			anchor = bucket.OrderKey
		}
	}

	var previous string
	for _, bucket := range b.stored {
		if bucket.CollectionID != collection || bucket.ID == movingID || bucket.IsDeleted() {
			continue
		}
		if anchor != "" && bucket.OrderKey >= anchor {
			continue
		}
		if bucket.OrderKey > previous {
			previous = bucket.OrderKey
		}
	}
	return previous, anchor, nil
}

func (b *buckets) FirstOther(
	ctx context.Context, collection, excluded shared.ID,
) (domain.Bucket, error) {
	board, err := b.List(ctx, collection)
	if err != nil {
		return domain.Bucket{}, err
	}
	for _, bucket := range board {
		if bucket.ID != excluded {
			return bucket, nil
		}
	}
	return domain.Bucket{}, shared.ErrNotFound
}

func (b *buckets) MoveItems(
	_ context.Context, source, target shared.ID, at time.Time,
) (int, error) {
	b.moved = append(b.moved, movedItems{source: source, target: target, at: at})
	return len(b.moved), nil
}

var _ repository.Buckets = (*buckets)(nil)

// sortBuckets orders a board the way the query does: by rank, then by identifier.
func sortBuckets(board []domain.Bucket) {
	for i := 1; i < len(board); i++ {
		for j := i; j > 0; j-- {
			left, right := board[j-1], board[j]
			if left.OrderKey < right.OrderKey ||
				(left.OrderKey == right.OrderKey && left.ID < right.ID) {
				break
			}
			board[j-1], board[j] = right, left
		}
	}
}

type bucketHarness struct {
	create     CreateBucket
	list       ListBuckets
	buckets    *buckets
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	authorizer *authorizer
	uow        *unitOfWork
}

func newBucketHarness() *bucketHarness {
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	store := &containers{stored: map[shared.ID]domain.Container{}}
	h := &bucketHarness{
		buckets: board, containers: store, events: &events{}, changes: &changes{},
		audit: &sink{}, authorizer: &authorizer{}, uow: &unitOfWork{},
	}
	h.create = CreateBucket{
		Buckets: board, Containers: store, Authorizer: h.authorizer, Events: h.events,
		Changes: h.changes, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.list = ListBuckets{
		Buckets: board, Containers: store, Authorizer: h.authorizer, UnitOfWork: h.uow,
	}
	return h
}

// withCollection puts a hub and a collection in it into the store, which is what every bucket
// hangs off.
func (h *bucketHarness) withCollection() domain.Container {
	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	collection := domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = collection
	return collection
}

func (h *bucketHarness) withBucket(id shared.ID, name, orderKey string) domain.Bucket {
	bucket := domain.Bucket{
		ID: id, TenantID: tenantID, CollectionID: collectionID,
		Name: name, OrderKey: orderKey, Version: 1,
	}
	h.buckets.stored[id] = bucket
	return bucket
}

func bucketCommand() CreateBucketCommand {
	return CreateBucketCommand{CollectionID: collectionID, Name: "  Doing  "}
}

// One write owes four things, and this is the test that says so.
func TestCreatingABucketWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newBucketHarness()
	h.withCollection()
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	bucket, err := h.create.Execute(ctx, actor(), bucketCommand())
	if err != nil {
		t.Fatalf("creating the bucket failed: %v", err)
	}

	if bucket.Name != "Doing" || bucket.Version != 1 || bucket.CollectionID != collectionID {
		t.Errorf("unexpected bucket: %+v", bucket)
	}
	if bucket.TenantID != tenantID {
		t.Errorf("the tenant came from somewhere other than the actor: %s", bucket.TenantID)
	}
	if len(h.buckets.inserted) != 1 {
		t.Fatalf("%d buckets written, want 1", len(h.buckets.inserted))
	}
	if !h.uow.committed {
		t.Error("the transaction did not commit")
	}

	t.Run("the event", func(t *testing.T) {
		if len(h.events.appended) != 1 {
			t.Fatalf("%d events, want 1", len(h.events.appended))
		}
		announcement := h.events.appended[0]
		if announcement.Type != event.BucketCreated {
			t.Errorf("event type %s", announcement.Type)
		}
		if announcement.Payload["id"] != bucket.ID.String() {
			t.Errorf("the event describes another bucket: %v", announcement.Payload["id"])
		}
		if announcement.Payload["collection_id"] != collectionID.String() {
			t.Errorf("the event names another board: %v", announcement.Payload["collection_id"])
		}
	})

	// The visibility filter a pull applies is the hub, not the collection: a device subscribed to
	// the hub has to see the new column appear (offline-sync.md §3.1).
	t.Run("the change for offline clients", func(t *testing.T) {
		if len(h.changes.recorded) != 1 {
			t.Fatalf("%d changes, want 1", len(h.changes.recorded))
		}
		change := h.changes.recorded[0]
		if change.Entity != "bucket" || change.EntityID != bucket.ID {
			t.Errorf("the change describes something else: %+v", change)
		}
		if change.ContainerID != hubID {
			t.Errorf("the change is filed under %s, want the hub", change.ContainerID)
		}
		if change.HLC.IsZero() {
			t.Error("the change carries no clock reading, so nothing can merge it")
		}
	})

	// A bucket's name is user content and is recorded as a fingerprint; the limit and the done
	// marker are not, and are recorded in clear text (rule 10, audit.md §4).
	t.Run("the audit entry", func(t *testing.T) {
		if len(h.audit.entries) != 1 {
			t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
		}
		entry := h.audit.entries[0]
		if entry.Action != BucketCreatedAction || entry.TargetID != bucket.ID {
			t.Errorf("unexpected entry: %+v", entry)
		}
		if entry.Context.RequestID != "01J9REQUEST" {
			t.Errorf("the entry does not carry the request: %+v", entry.Context)
		}
		name, _ := entry.Changes["name"].(map[string]any)
		if name == nil || name["changed"] != true {
			t.Fatalf("the name is not in the trail at all: %+v", entry.Changes)
		}
		if _, readable := name["to"]; readable {
			t.Errorf("the name is in the trail in clear text: %+v", name)
		}
		if entry.Changes[domain.FieldIsDoneBucket] == nil {
			t.Errorf("the done marker is not in the trail: %+v", entry.Changes)
		}
	})
}

// The permission question is asked against the collection's path, so that a membership held at the
// hub applies downwards (domain-model.md §3.2), and it is asked before the transaction opens - a
// refusal writes an audit entry, and one written inside would be rolled back with it.
func TestCreatingABucketAsksAboutTheCollectionsPath(t *testing.T) {
	h := newBucketHarness()
	h.withCollection()

	if _, err := h.create.Execute(context.Background(), actor(), bucketCommand()); err != nil {
		t.Fatalf("creating the bucket failed: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want 1", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.Permission != service.PermissionStructure {
		t.Errorf("permission %s, want the structure one", request.Permission)
	}
	if len(request.Path) != 3 {
		t.Errorf("the path is %+v, want tenant, hub and collection", request.Path)
	}
	if request.Action != BucketCreatedAction || request.TargetID != collectionID {
		t.Errorf("unexpected request: %+v", request)
	}
}

func TestARefusedCreateWritesNothing(t *testing.T) {
	h := newBucketHarness()
	h.withCollection()
	h.authorizer.err = shared.ErrForbidden

	if _, err := h.create.Execute(context.Background(), actor(), bucketCommand()); !errors.Is(
		err, shared.ErrForbidden) {
		t.Fatalf("the refusal did not come back: %v", err)
	}
	if len(h.buckets.inserted) != 0 || len(h.events.appended) != 0 || h.uow.writes != 0 {
		t.Error("a refused create wrote something")
	}
}

// Only a collection has a board. A hub holds collections and no entries, so a column on one would
// have nothing to hold.
func TestABucketNeedsACollection(t *testing.T) {
	h := newBucketHarness()
	h.withCollection()

	t.Run("a hub is refused", func(t *testing.T) {
		cmd := bucketCommand()
		cmd.CollectionID = hubID

		_, err := h.create.Execute(context.Background(), actor(), cmd)
		if !errors.Is(err, shared.ErrValidation) {
			t.Fatalf("a bucket on a hub was accepted: %v", err)
		}
		if shared.AsError(err).DetailCode != "items.collection_required" {
			t.Errorf("detail code %s", shared.AsError(err).DetailCode)
		}
	})

	t.Run("no collection at all is refused", func(t *testing.T) {
		cmd := bucketCommand()
		cmd.CollectionID = ""

		_, err := h.create.Execute(context.Background(), actor(), cmd)
		if shared.AsError(err).DetailCode != "items.collection_id_required" {
			t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
		}
	})

	t.Run("a collection nobody has is not found", func(t *testing.T) {
		cmd := bucketCommand()
		cmd.CollectionID = shared.MustParseID("0192f000-0000-7000-8000-00000000009f")

		_, err := h.create.Execute(context.Background(), actor(), cmd)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("an unknown collection was accepted: %v", err)
		}
	})
}

// I-C3: an archived collection is read-only, and so is one whose hub is archived.
func TestABucketIsRefusedOnAnArchivedCollection(t *testing.T) {
	h := newBucketHarness()
	collection := h.withCollection()
	archivedAt := now
	collection.ArchivedAt = &archivedAt
	h.containers.stored[collectionID] = collection

	_, err := h.create.Execute(context.Background(), actor(), bucketCommand())
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a bucket was added to an archived collection: %v", err)
	}
	if shared.AsError(err).DetailCode != "items.collection_archived" {
		t.Errorf("detail code %s", shared.AsError(err).DetailCode)
	}
}

// A board grows to the right unless a column is named to go before.
func TestWhereANewColumnLands(t *testing.T) {
	t.Run("at the right hand end", func(t *testing.T) {
		h := newBucketHarness()
		h.withCollection()
		h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"), "Todo", "a0")

		bucket, err := h.create.Execute(context.Background(), actor(), bucketCommand())
		if err != nil {
			t.Fatalf("creating the bucket failed: %v", err)
		}
		if bucket.OrderKey <= "a0" {
			t.Errorf("the new column ranks %q, want it after a0", bucket.OrderKey)
		}
	})

	t.Run("before a named column", func(t *testing.T) {
		h := newBucketHarness()
		h.withCollection()
		first := h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"), "Todo", "a0")
		h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f2"), "Done", "a1")

		cmd := bucketCommand()
		cmd.BeforeBucketID = first.ID

		bucket, err := h.create.Execute(context.Background(), actor(), cmd)
		if err != nil {
			t.Fatalf("creating the bucket failed: %v", err)
		}
		if bucket.OrderKey >= "a0" {
			t.Errorf("the new column ranks %q, want it before a0", bucket.OrderKey)
		}
	})

	// An anchor that is not on this board is refused rather than appended: a client that positioned
	// a column and received a 201 would believe the board is in an order it is not.
	t.Run("before a column that is not on the board", func(t *testing.T) {
		h := newBucketHarness()
		h.withCollection()

		cmd := bucketCommand()
		cmd.BeforeBucketID = shared.MustParseID("0192f000-0000-7000-8000-0000000000f9")

		_, err := h.create.Execute(context.Background(), actor(), cmd)
		if !errors.Is(err, shared.ErrValidation) {
			t.Fatalf("an unknown anchor was accepted: %v", err)
		}
		if shared.AsError(err).DetailCode != "buckets.before_bucket_not_on_board" {
			t.Errorf("detail code %s", shared.AsError(err).DetailCode)
		}
	})
}

// Zero clears the limit rather than setting an impossible one, which is what the column's CHECK
// makes possible and what saves every layer above from a second flag beside the number.
func TestAWipLimitOfZeroIsNoLimit(t *testing.T) {
	h := newBucketHarness()
	h.withCollection()

	cmd := bucketCommand()
	cmd.WipLimit = 0

	bucket, err := h.create.Execute(context.Background(), actor(), cmd)
	if err != nil {
		t.Fatalf("creating the bucket failed: %v", err)
	}
	if bucket.WipLimit != nil {
		t.Errorf("wip limit %d, want none", *bucket.WipLimit)
	}
}

// The read side never opens a write transaction: a read may be served by a replica, and one that
// asked for a writable transaction would pin every board in the product to the primary.
func TestListingABoardReadsOnly(t *testing.T) {
	h := newBucketHarness()
	h.withCollection()
	h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f2"), "Done", "a1")
	h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"), "Todo", "a0")

	board, err := h.list.Execute(context.Background(), actor(), ListBucketsQuery{
		CollectionID: collectionID,
	})
	if err != nil {
		t.Fatalf("listing the board failed: %v", err)
	}

	if len(board) != 2 || board[0].Name != "Todo" || board[1].Name != "Done" {
		t.Fatalf("the board is %+v, want Todo then Done", board)
	}
	if h.uow.writes != 0 {
		t.Errorf("%d write transactions were opened by a read", h.uow.writes)
	}
}

func TestListingABoardNeedsACollection(t *testing.T) {
	h := newBucketHarness()

	_, err := h.list.Execute(context.Background(), actor(), ListBucketsQuery{})
	if shared.AsError(err).DetailCode != "items.collection_id_required" {
		t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
	}
}

func TestARefusedReadReturnsNoBoard(t *testing.T) {
	h := newBucketHarness()
	h.withCollection()
	h.withBucket(shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"), "Todo", "a0")
	h.authorizer.err = shared.ErrForbidden

	board, err := h.list.Execute(context.Background(), actor(), ListBucketsQuery{
		CollectionID: collectionID,
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal did not come back: %v", err)
	}
	if board != nil {
		t.Errorf("a refused read answered with %+v", board)
	}
}

// The catalogue's untyped input becomes the typed command in one place, for all three channels.
func TestTheBucketDescriptorsCarryWhatTheChannelsNeed(t *testing.T) {
	create := CreateBucket{}.Descriptor()
	if create.Name != CreateBucketName || create.TokenScope != bucketsWrite {
		t.Errorf("unexpected descriptor: %+v", create)
	}
	if !create.Audit.Required || create.Audit.Action != BucketCreatedAction {
		t.Errorf("a create that writes nothing to the trail: %+v", create.Audit)
	}

	list := ListBuckets{}.Descriptor()
	if !list.ReadOnly || list.Audit.Required {
		t.Errorf("a read that is not read only, or that insists on an entry: %+v", list)
	}
}
