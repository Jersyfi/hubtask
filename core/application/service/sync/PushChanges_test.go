// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
)

var (
	opA    = shared.ID("01936f2a-7c1e-7000-8000-000000000f01")
	opB    = shared.ID("01936f2a-7c1e-7000-8000-000000000f02")
	itemX  = shared.ID("01936f2a-7c1e-7000-8000-000000000e01")
	purged = shared.ID("01936f2a-7c1e-7000-8000-000000000e02")
)

// opStore is the operation log in memory.
type opStore struct {
	records map[shared.ID]repository.OpRecord
}

func (s *opStore) Find(_ context.Context, opID shared.ID) (repository.OpRecord, bool, error) {
	record, found := s.records[opID]
	return record, found, nil
}

func (s *opStore) Record(_ context.Context, record repository.OpRecord) error {
	if _, seen := s.records[record.OpID]; !seen {
		s.records[record.OpID] = record
	}
	return nil
}

type tombstoneStore struct{ held map[shared.ID]bool }

func (s tombstoneStore) Holds(_ context.Context, _ string, id shared.ID) (bool, error) {
	return s.held[id], nil
}

// invocation is one call the catalogue saw.
type invocation struct {
	name  string
	actor appshared.ActorContext
	in    usecase.Input
	// device is what the context named at the time of the call.
	device shared.ID
}

// stubCatalogue records what it is asked to run and answers what the test set up, per use case.
type stubCatalogue struct {
	calls   []invocation
	outputs map[string]usecase.Output
	errs    map[string]error
}

func (c *stubCatalogue) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	c.calls = append(c.calls, invocation{name: name, actor: actor, in: in, device: appshared.DeviceFrom(ctx)})
	if err := c.errs[name]; err != nil {
		return nil, err
	}
	return c.outputs[name], nil
}

type pushFixture struct {
	push      PushChanges
	stream    fixture
	ops       *opStore
	catalogue *stubCatalogue
	devices   *deviceStore
}

func pushing(t *testing.T) pushFixture {
	t.Helper()
	pull, f := pulling(t, entry(1, readable), entry(2, readable))
	ops := &opStore{records: map[shared.ID]repository.OpRecord{}}
	catalogue := &stubCatalogue{
		outputs: map[string]usecase.Output{
			"CreateWorkItem": {"id": itemX.String(), "title": "Fix the tap", "version": 1},
			"TrashWorkItem":  {"id": itemX.String(), "deleted_at": now},
			"AddComment":     {"id": "01936f2a-7c1e-7000-8000-000000000c01", "body": "Done?"},
		},
		errs: map[string]error{},
	}
	devices := newDeviceStore()
	return pushFixture{
		push: PushChanges{
			Stream: pull.Stream, Devices: devices, Ops: ops,
			Tombstones: tombstoneStore{held: map[shared.ID]bool{purged: true}},
			Catalogue:  catalogue,
		},
		stream: f, ops: ops, catalogue: catalogue, devices: devices,
	}
}

func pusher() appshared.ActorContext {
	a := signedIn()
	a.Scopes = append(a.Scopes, "items:write")
	return a
}

func creation(opID shared.ID) Mutation {
	return Mutation{
		OpID: opID, Kind: domain.ItemCreate, ItemID: itemX,
		HLC:     "1757937600000:00001:dev-a",
		Payload: map[string]any{"type": "TASK", "title": "Fix the tap", "collection_id": collectionA.String()},
	}
}

func TestAPushAppliesACreationThroughTheCatalogueAsThePushingPerson(t *testing.T) {
	f := pushing(t)

	response, err := f.push.Push(t.Context(), pusher(), PushRequest{
		DeviceID: device, Platform: "hubctl", Mutations: []Mutation{creation(opA)},
	})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Result != domain.Applied ||
		response.Results[0].EntityID != itemX || response.Results[0].ServerState["title"] != "Fix the tap" {
		t.Errorf("the result is %+v", response.Results)
	}
	// The cursor after application is where the log stands, so that the device can skip a pull.
	if response.Cursor.Seq != 2 || !response.ServerTime.Equal(now) {
		t.Errorf("the answer stands at %+v, want the log's head at server time", response.Cursor)
	}

	if len(f.catalogue.calls) != 1 {
		t.Fatalf("%d calls, want one", len(f.catalogue.calls))
	}
	call := f.catalogue.calls[0]
	if call.name != "CreateWorkItem" || call.actor.AccountID != account || call.device != device {
		t.Errorf("performed %q as %s from device %s", call.name, call.actor.AccountID, call.device)
	}
	// The identifier is the mutation's, written after the payload so that the payload cannot
	// move it; the rest of the payload is the use case's input as sent.
	if call.in["id"] != itemX.String() || call.in["title"] != "Fix the tap" || call.in["type"] != "TASK" {
		t.Errorf("the input was %v", call.in)
	}
	// The device was registered by the push.
	if f.devices.devices[device].Platform != "hubctl" {
		t.Errorf("the device was not registered: %+v", f.devices.devices)
	}
	// And the operation is on record, with the answer.
	record, found := f.ops.records[opA]
	if !found || record.Result != domain.Applied || record.DeviceID != device || record.Response["result"] != "APPLIED" {
		t.Errorf("the record is %+v (found=%v)", record, found)
	}
}

// SY-7: a duplicate push of the same mutations takes effect exactly once and answers the same.
func TestADuplicatePushTakesEffectExactlyOnce(t *testing.T) {
	f := pushing(t)
	request := PushRequest{DeviceID: device, Mutations: []Mutation{creation(opA)}}

	first, err := f.push.Push(t.Context(), pusher(), request)
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	second, err := f.push.Push(t.Context(), pusher(), request)
	if err != nil {
		t.Fatalf("pushing again: %v", err)
	}
	if len(f.catalogue.calls) != 1 {
		t.Errorf("the catalogue was asked %d times, want once", len(f.catalogue.calls))
	}
	if second.Results[0].Result != first.Results[0].Result || second.Results[0].EntityID != first.Results[0].EntityID ||
		second.Results[0].ServerState["title"] != first.Results[0].ServerState["title"] {
		t.Errorf("the repeat answered %+v, the first %+v", second.Results[0], first.Results[0])
	}
}

// Partial success is the rule: a refused mutation is a result with the use case's own code, it
// does not stop the ones after it, and the refusal is recorded so that a repeat answers the same.
func TestARefusedMutationIsAResultAndDoesNotBlockTheNext(t *testing.T) {
	f := pushing(t)
	f.catalogue.errs["TrashWorkItem"] = shared.ErrForbidden.WithDetail("access.forbidden")

	deletion := Mutation{OpID: opA, Kind: domain.ItemDelete, ItemID: itemX}
	response, err := f.push.Push(t.Context(), pusher(), PushRequest{
		DeviceID: device, Mutations: []Mutation{deletion, creation(opB)},
	})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if len(response.Results) != 2 {
		t.Fatalf("%d results, want two", len(response.Results))
	}
	refused, applied := response.Results[0], response.Results[1]
	if refused.Result != domain.Rejected || refused.Error == nil ||
		refused.Error.Code != "forbidden" || refused.Error.MessageCode != "access.forbidden" {
		t.Errorf("the refusal is %+v", refused)
	}
	if applied.Result != domain.Applied {
		t.Errorf("the mutation after the refusal was not applied: %+v", applied)
	}

	// The refusal is on record, and a repeat does not try again.
	if record, found := f.ops.records[opA]; !found || record.Result != domain.Rejected {
		t.Errorf("the refusal was not recorded: %+v", record)
	}
	calls := len(f.catalogue.calls)
	if _, err := f.push.Push(t.Context(), pusher(), PushRequest{DeviceID: device, Mutations: []Mutation{deletion}}); err != nil {
		t.Fatalf("repeating: %v", err)
	}
	if len(f.catalogue.calls) != calls {
		t.Errorf("the repeat of a refusal asked the catalogue again")
	}
}

// A dependency that could not answer is not a refusal: the push ends with the error, the
// mutation is not recorded, and the client pushes the queue again.
func TestADependencyFailureEndsThePushWithoutARecord(t *testing.T) {
	f := pushing(t)
	f.catalogue.errs["CreateWorkItem"] = shared.ErrUnavailable.WithDetail("postgres.query_failed")

	_, err := f.push.Push(t.Context(), pusher(), PushRequest{DeviceID: device, Mutations: []Mutation{creation(opA)}})
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("the push answered %v, want the dependency's error", err)
	}
	if _, found := f.ops.records[opA]; found {
		t.Errorf("a mutation nobody applied was recorded")
	}
}

// SY-2: a device three hours out does not outvote everybody - its readings are replaced by server
// readings that keep its counter and identifier.
func TestAReadingBeyondTheSkewIsBoundedToServerTime(t *testing.T) {
	f := pushing(t)
	threeHoursOut, _ := shared.NewHLC(now.Add(3*time.Hour), 7, "dev-a")
	inTime, _ := shared.NewHLC(now.Add(time.Minute), 1, "dev-a")

	bounded, err := f.push.bound(t.Context(), Mutation{
		OpID: opA, Kind: domain.ItemPatch, HLC: threeHoursOut.String(),
		Fields: map[string]FieldChange{
			"title":  {Value: "x", HLC: threeHoursOut.String()},
			"due_at": {Value: "y", HLC: inTime.String()},
		},
	})
	if err != nil {
		t.Fatalf("bounding: %v", err)
	}
	want, _ := shared.NewHLC(now, 7, "dev-a")
	if bounded.HLC != want.String() || bounded.Fields["title"].HLC != want.String() {
		t.Errorf("bounded to %s and %s, want %s", bounded.HLC, bounded.Fields["title"].HLC, want)
	}
	if bounded.Fields["due_at"].HLC != inTime.String() {
		t.Errorf("a reading within the skew was changed to %s", bounded.Fields["due_at"].HLC)
	}

	_, err = f.push.bound(t.Context(), Mutation{OpID: opA, HLC: "not a clock"})
	if got := shared.AsError(err).DetailCode; got != "sync.hlc_malformed" {
		t.Errorf("a malformed reading was answered %q", got)
	}
}

func TestWhatIsNotAppliedIsAnsweredAndNotRecorded(t *testing.T) {
	cases := map[string]struct {
		mutation Mutation
		code     string
		recorded bool
	}{
		"no operation identifier": {
			mutation: Mutation{Kind: domain.ItemCreate, ItemID: itemX}, code: "sync.op_id_required",
		},
		"a kind the contract does not name": {
			mutation: Mutation{OpID: opA, Kind: "ITEM_RENAME"}, code: "sync.kind_unknown",
		},
		"a kind this build does not apply yet": {
			mutation: Mutation{OpID: opA, Kind: domain.SetAdd, ItemID: itemX}, code: "sync.kind_unavailable",
		},
		"a malformed reading": {
			mutation: Mutation{OpID: opA, Kind: domain.ItemCreate, ItemID: itemX, HLC: "x"}, code: "sync.hlc_malformed",
		},
		"a purged entry": {
			mutation: Mutation{OpID: opA, Kind: domain.ItemDelete, ItemID: purged}, code: "sync.gone", recorded: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := pushing(t)
			response, err := f.push.Push(t.Context(), pusher(), PushRequest{DeviceID: device, Mutations: []Mutation{tc.mutation}})
			if err != nil {
				t.Fatalf("pushing: %v", err)
			}
			result := response.Results[0]
			if result.Result != domain.Rejected || result.Error == nil || result.Error.MessageCode != tc.code {
				t.Errorf("answered %+v, want a rejection with %s", result, tc.code)
			}
			if _, found := f.ops.records[opA]; found != tc.recorded {
				t.Errorf("recorded=%v, want %v", found, tc.recorded)
			}
			if len(f.catalogue.calls) != 0 {
				t.Errorf("the catalogue was asked")
			}
		})
	}
}

func TestAPushIsRefusedAtTheDoor(t *testing.T) {
	f := pushing(t)
	f.devices.devices[deviceB] = domain.Device{ID: deviceB, AccountID: otherPerson}

	readOnly := signedIn()
	if _, err := f.push.Push(t.Context(), readOnly, PushRequest{DeviceID: device}); err == nil {
		t.Errorf("a token without items:write pushed")
	}
	if _, err := f.push.Push(t.Context(), pusher(), PushRequest{DeviceID: deviceB}); shared.AsError(err).DetailCode != "sync.device_foreign" {
		t.Errorf("another account's device pushed: %v", err)
	}
	tooMany := make([]Mutation, PushLimit+1)
	if _, err := f.push.Push(t.Context(), pusher(), PushRequest{DeviceID: device, Mutations: tooMany}); shared.AsError(err).DetailCode != "sync.push_too_large" {
		t.Errorf("a push over the limit was answered %v", err)
	}
	if len(f.catalogue.calls) != 0 {
		t.Errorf("a refused push reached the catalogue")
	}
}

func TestACommentTravelsWithTheClientsIdentifierOnTheEntry(t *testing.T) {
	f := pushing(t)
	commentID := "01936f2a-7c1e-7000-8000-000000000c01"

	response, err := f.push.Push(t.Context(), pusher(), PushRequest{DeviceID: device, Mutations: []Mutation{{
		OpID: opA, Kind: domain.CommentAdd, ItemID: itemX,
		Payload: map[string]any{"id": commentID, "body": "Done?"},
	}}})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	call := f.catalogue.calls[0]
	if call.name != "AddComment" || call.in["item_id"] != itemX.String() || call.in["id"] != commentID || call.in["body"] != "Done?" {
		t.Errorf("performed %q with %v", call.name, call.in)
	}
	if response.Results[0].EntityID.String() != commentID {
		t.Errorf("the result names %s, want the comment", response.Results[0].EntityID)
	}
}

func TestAStoredResultRoundTripsThroughTheRecord(t *testing.T) {
	result := Result{
		OpID: opA, Result: domain.Conflict, EntityID: itemX,
		ServerState: map[string]any{"title": "x"},
		Conflict:    &ConflictDetail{Field: "notes", Mine: "a", Theirs: "b", PreservedCommentID: purged},
		Error:       &ResultError{Code: "conflict", MessageCode: "sync.conflict"},
	}
	back := resultFromRecord(recordOf(device, result, now))
	if back.OpID != opA || back.Result != domain.Conflict || back.EntityID != itemX ||
		back.ServerState["title"] != "x" || back.Conflict == nil || back.Conflict.Field != "notes" ||
		back.Conflict.PreservedCommentID != purged || back.Error == nil || back.Error.MessageCode != "sync.conflict" {
		t.Errorf("came back as %+v", back)
	}
}
