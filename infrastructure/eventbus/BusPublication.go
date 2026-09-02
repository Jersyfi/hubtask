// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package eventbus

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// Events is the slice of the outbox this handler reads: one event, by its identifier. The same
// slice the webhook delivery reads, and for the same reason - the job carries an identifier and
// renders the body at each attempt, so a retry publishes what the first attempt would have.
type Events interface {
	FindEvent(ctx context.Context, id shared.ID) (event.Envelope, error)
}

// Bus is what the publication needs of a transport, and it is an interface here so that the
// handler can be tested without a broker.
type Bus interface {
	Publish(ctx context.Context, tenantID shared.ID, eventType string, payload []byte) error
}

// Publication is the `bus.publish` job: read the event, render it, put it on the bus (H-14).
//
// It is a job rather than a step of the dispatch because a subscriber may not call the outside
// world inside the dispatcher's transaction (core/port/eventbus). What it buys beyond that is the
// queue's own retry ladder and dead letter: a bus that is down for an hour is an hour of retries
// rather than an hour of held transactions, and the outbox holds the events throughout.
type Publication struct {
	Events     Events
	Bus        Bus
	UnitOfWork persistence.UnitOfWork
	// Source is what the CloudEvent's `source` says, and it is this installation's base URL - the
	// same value the webhook delivery renders with, so one event has one identity whichever
	// transport carries it.
	Source string
	// Signals counts what was published and what was refused. Nil records nothing.
	Signals PublicationSignals
}

// PublicationSignals is the metric slice this handler uses. An interface rather than the adapter,
// so an outbound adapter does not learn about another one (project-structure.md §2).
type PublicationSignals interface {
	BusPublished(ctx context.Context, eventType string)
	BusRefused(ctx context.Context, reason string)
}

var (
	_ queue.Handler  = Publication{}
	_ queue.Detached = Publication{}
)

// OwnsItsTransactions declares the exception this handler needs (queue.Detached).
//
// A publish is a network call, and holding the runner's transaction open across it is what
// observability-reliability.md §8 forbids. What it gives up is atomicity between the publish and
// the job's completion, which is acceptable for the reason the interface asks about: the delivery
// guarantee is at-least-once (ADR-0007), consumers deduplicate on the event id, and JetStream's
// own message id does the same one layer down.
func (Publication) OwnsItsTransactions() {}

// Run publishes one event.
//
// A bus that is unreachable comes back as an error and not as a quiet success, which is the
// opposite of the webhook delivery's choice and is right for the opposite reason: a webhook target
// that refuses is somebody else's system behaving, recorded in a delivery log an operator reads,
// while a bus that refuses is *this* installation's dependency being down. There is no second log
// for it, so the queue's ladder and its dead letter are the record.
func (p Publication) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	eventID, err := publicationEvent(job)
	if err != nil {
		return queue.Result{}, err
	}

	var envelope event.Envelope
	var expired bool
	err = p.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: job.TenantID}, func(ctx context.Context) error {
		envelope, err = p.Events.FindEvent(ctx, eventID)
		if errors.Is(err, shared.ErrNotFound) {
			// Swept before its publication ran. The outbox's retention outliving the retry ladder
			// is a misconfiguration, and this is what it looks like from here: the job is done,
			// because there is nothing left that could ever be published.
			expired = true
			return nil
		}
		return err
	})
	if err != nil {
		return queue.Result{}, err
	}
	if expired {
		p.refused(ctx, "event_expired")
		return queue.Result{}, nil
	}

	payload, err := json.Marshal(ToCloudEvent(envelope, p.Source))
	if err != nil {
		return queue.Result{}, shared.ErrInternal.WithDetail("bus.body_unrenderable").WithCause(err)
	}

	if err := p.Bus.Publish(ctx, envelope.TenantID, envelope.Type.String(), payload); err != nil {
		p.refused(ctx, "unavailable")
		return queue.Result{}, err
	}

	if p.Signals != nil {
		// The type and never the subject: a subject carries a tenant identifier, and a metric
		// label carrying one is how a Prometheus dies and how a tenant becomes visible in a
		// dashboard (observability-reliability.md §3.2, rule 10).
		p.Signals.BusPublished(ctx, envelope.Type.String())
	}
	return queue.Result{}, nil
}

func (p Publication) refused(ctx context.Context, reason string) {
	if p.Signals != nil {
		p.Signals.BusRefused(ctx, reason)
	}
}

// publicationEvent reads the one field the job carries.
func publicationEvent(job queue.Job) (shared.ID, error) {
	raw, present := job.Payload["event_id"].(string)
	if !present {
		return "", shared.ErrInternal.WithDetail("bus.payload_incomplete")
	}
	id, err := shared.ParseID(raw)
	if err != nil {
		return "", shared.ErrInternal.WithDetail("bus.payload_incomplete").WithCause(err)
	}
	return id, nil
}
