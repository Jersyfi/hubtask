// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	busTenant  = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	busEventID = shared.ID("01936f2a-7c1e-7000-8000-0000000000a2")
	busActor   = shared.ID("01936f2a-7c1e-7000-8000-0000000000a3")
	busMoment  = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
)

// readOnly runs the function and nothing else. The handler's transaction is the runner's business;
// what is under test is what it does inside one.
type readOnly struct{ err error }

func (r readOnly) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return r.WithinReadOnly(ctx, persistence.Scope{}, fn)
}

func (r readOnly) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	if r.err != nil {
		return r.err
	}
	return fn(ctx)
}

type storedEvents struct {
	envelope event.Envelope
	err      error
}

func (s storedEvents) FindEvent(context.Context, shared.ID) (event.Envelope, error) {
	return s.envelope, s.err
}

type recordingBus struct {
	published [][]byte
	tenants   []shared.ID
	types     []string
	err       error
}

func (b *recordingBus) Publish(_ context.Context, tenantID shared.ID, eventType string, payload []byte) error {
	if b.err != nil {
		return b.err
	}
	b.tenants = append(b.tenants, tenantID)
	b.types = append(b.types, eventType)
	b.published = append(b.published, payload)
	return nil
}

type busSignals struct {
	published []string
	refused   []string
}

func (s *busSignals) BusPublished(_ context.Context, eventType string) {
	s.published = append(s.published, eventType)
}

func (s *busSignals) BusRefused(_ context.Context, reason string) {
	s.refused = append(s.refused, reason)
}

func anEvent(t *testing.T) event.Envelope {
	t.Helper()
	envelope, err := event.NewEnvelope(
		busEventID, event.ItemCreated, busTenant, "item/"+busEventID.String(),
		event.Actor{Kind: shared.ActorUser, ID: busActor}, busMoment, event.Cause{},
		map[string]any{"title": "a task"})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

func aPublication(bus *recordingBus, events storedEvents, signals *busSignals) Publication {
	return Publication{
		Events: events, Bus: bus, UnitOfWork: readOnly{},
		Source: "https://hubtask.example", Signals: signals,
	}
}

func aJob() queue.Job {
	return queue.Job{
		Kind: queue.KindBusPublish, TenantID: busTenant,
		Payload: map[string]any{"event_id": busEventID.String()},
	}
}

// The event is read back at each attempt and rendered as the CloudEvent every other transport
// carries, so one event has one identity whichever way it travels.
func TestThePublicationRendersTheEventAndPutsItOnTheBus(t *testing.T) {
	bus := &recordingBus{}
	signals := &busSignals{}

	if _, err := aPublication(bus, storedEvents{envelope: anEvent(t)}, signals).Run(context.Background(), aJob()); err != nil {
		t.Fatalf("publishing: %v", err)
	}

	if len(bus.published) != 1 {
		t.Fatalf("%d messages published", len(bus.published))
	}
	if bus.tenants[0] != busTenant {
		t.Errorf("published for tenant %s", bus.tenants[0])
	}
	var cloudEvent map[string]any
	if err := json.Unmarshal(bus.published[0], &cloudEvent); err != nil {
		t.Fatalf("the payload is not JSON: %v", err)
	}
	if cloudEvent["specversion"] != "1.0" || cloudEvent["id"] != busEventID.String() {
		t.Errorf("the payload is not the CloudEvent: %v", cloudEvent)
	}
	if cloudEvent["source"] != "https://hubtask.example" {
		t.Errorf("source = %v", cloudEvent["source"])
	}
	if len(signals.published) != 1 {
		t.Errorf("the publication was not counted: %+v", signals)
	}
}

// A bus that is unreachable fails the job, which is the opposite of the webhook delivery's choice
// and right for the opposite reason: a webhook target that refuses is somebody else's system
// behaving and is recorded in a delivery log, while a bus that refuses is this installation's
// dependency being down and has no second log. The queue's ladder is the record.
func TestAnUnreachableBusFailsTheJob(t *testing.T) {
	bus := &recordingBus{err: busUnavailable(errors.New("no connection"))}
	signals := &busSignals{}

	_, err := aPublication(bus, storedEvents{envelope: anEvent(t)}, signals).Run(context.Background(), aJob())
	if err == nil {
		t.Fatal("a bus that refused was reported as a publication")
	}
	if shared.AsError(err).Category != shared.CategoryUnavailable {
		t.Errorf("the job failed with %s", shared.AsError(err).Category)
	}
	if len(signals.refused) != 1 || signals.refused[0] != "unavailable" {
		t.Errorf("the refusal was counted as %v", signals.refused)
	}
}

// An event swept before its publication ran is the outbox's retention outliving the retry ladder -
// a misconfiguration, and this is what it looks like from here. The job is done, because nothing
// remains that could ever be published, and retrying for ever would fill the dead letter with jobs
// whose subject no longer exists.
func TestAnEventThatIsGoneEndsTheJobRatherThanRetryingForEver(t *testing.T) {
	bus := &recordingBus{}
	signals := &busSignals{}
	events := storedEvents{err: shared.ErrNotFound}

	if _, err := aPublication(bus, events, signals).Run(context.Background(), aJob()); err != nil {
		t.Fatalf("a swept event failed the job: %v", err)
	}
	if len(bus.published) != 0 {
		t.Error("something was published for an event that no longer exists")
	}
	if len(signals.refused) != 1 || signals.refused[0] != "event_expired" {
		t.Errorf("counted as %v", signals.refused)
	}
}

// A job whose payload is not what the fan-out writes is a defect in this system, not a transport
// failure: it fails as an internal error rather than being retried against a bus that would answer
// the same way eight times.
func TestAJobWithoutItsEventIdentifierIsAnInternalError(t *testing.T) {
	bus := &recordingBus{}
	publication := aPublication(bus, storedEvents{envelope: anEvent(t)}, &busSignals{})

	for _, payload := range []map[string]any{
		{},
		{"event_id": ""},
		{"event_id": "not-a-uuid"},
		{"event_id": 42},
	} {
		job := aJob()
		job.Payload = payload
		_, err := publication.Run(context.Background(), job)
		if err == nil {
			t.Fatalf("a job carrying %v was accepted", payload)
		}
		if shared.AsError(err).Category != shared.CategoryInternal {
			t.Errorf("a job carrying %v failed as %s", payload, shared.AsError(err).Category)
		}
	}
	if len(bus.published) != 0 {
		t.Error("a malformed job published something")
	}
}

// The handler owns its transactions, and it has to: a publish is a network call, and holding the
// runner's transaction open across one is what observability-reliability.md §8 forbids.
func TestThePublicationOwnsItsTransactions(t *testing.T) {
	var handler any = Publication{}
	if _, detached := handler.(queue.Detached); !detached {
		t.Error("the publication runs inside the runner's transaction, across a network call")
	}
}
