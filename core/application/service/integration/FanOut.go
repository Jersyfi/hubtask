// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// FanOutName is what the deduplication and the logs call this consumer. Stable across versions:
// renaming it would make every event it has already seen look new (core/port/eventbus).
const FanOutName = "webhooks"

// FanOut is the outbox's webhook consumer: one event in, a delivery job per interested
// subscription out.
//
// It does not deliver. It records that a delivery is owed and asks the queue for it, which is what
// keeps one unreachable target from holding up every other subscriber's events - the retry ladder
// belongs to one job, and the dispatcher's own round is over as soon as the rows are written.
//
// # Why a replay reaches no subscriber
//
// It implements Subscriber and deliberately not TakesReplays. A restore would otherwise report
// last month's states to every external system that has ever subscribed, which is the first
// prohibition of backup-restore.md §8.4 - and the dispatcher enforces it for us rather than
// trusting this consumer to remember.
type FanOut struct {
	Subscriptions repository.WebhookSubscriptions
	Deliveries    repository.WebhookDeliveries
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
}

// Name identifies the consumer in the deduplication.
func (FanOut) Name() string { return FanOutName }

// Wants reports whether any subscription could be interested.
//
// It answers true for every type this build emits rather than reading the table, and that is
// deliberate: Wants is asked before the transaction the delivery would be written in, so a lookup
// here would be a query per event per consumer that the fan-out then repeats. The real filter is
// the query in Deliver, which the database answers with an index.
func (FanOut) Wants(event.Type) bool { return true }

// Deliver records what is owed and queues it.
//
// Everything happens in the dispatcher's transaction: the delivery rows, the jobs and the mark
// that this event was consumed commit together or not at all. A job that committed without its row
// would be a delivery nothing could record the outcome of.
func (f FanOut) Deliver(ctx context.Context, envelope event.Envelope) error {
	wanting, err := f.Subscriptions.WantingEvent(ctx, envelope.Type)
	if err != nil {
		return err
	}

	now := f.Clock.Now()
	for _, stored := range wanting {
		subscription := stored.Subscription
		// Asked again here, and not because the query might be wrong: the query answers what the
		// table says, and this answers what the aggregate says about it. The two agreeing is the
		// point - a state check in one place only is a state check somebody can move the table
		// past.
		if !subscription.Wants(envelope.Type) {
			continue
		}

		delivery, err := domain.NewWebhookDelivery(
			f.IDs.NewID(), envelope.TenantID, subscription.ID, envelope.ID, 1, now)
		if err != nil {
			return err
		}
		if err := f.Deliveries.Insert(ctx, delivery); err != nil {
			return err
		}
		if err := f.enqueue(ctx, envelope.TenantID, subscription.ID, delivery); err != nil {
			return err
		}
	}
	return nil
}

func (f FanOut) enqueue(
	ctx context.Context, tenantID, subscriptionID shared.ID, delivery domain.WebhookDelivery,
) error {
	_, err := f.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindWebhookDeliver, TenantID: tenantID, RunAt: delivery.CreatedAt,
		Payload: map[string]any{
			"subscription_id": subscriptionID.String(),
			"delivery_id":     delivery.ID.String(),
			"event_id":        delivery.EventID.String(),
		},
	})
	return err
}

// DisableNotifier is how the owner of a subscription is told it stopped.
//
// The notification category C-09 built rather than a new channel: "auto-disable after sustained
// unreachability, plus a notification to the owner" is one sentence in automation.md §3.1, and the
// second half of it is a thing this system already knows how to do.
type DisableNotifier interface {
	WebhookDisabled(ctx context.Context, tenantID, recipient, subscriptionID shared.ID) error
}

// Outcomes is the half of the webhook use cases the delivery path needs: it reports what happened
// and lets the aggregate decide what that means.
//
// A type of its own rather than methods on Writer, because this is called by a job handler rather
// than by a request, and the two have different rights: a handler has no actor and asks no
// permission - it is the system carrying out an instruction somebody already gave.
type Outcomes struct {
	Subscriptions repository.WebhookSubscriptions
	Audit         audit.Sink
	Notifier      DisableNotifier
	Clock         clock.Clock
}

// Delivered ends the failure run.
func (o Outcomes) Delivered(ctx context.Context, subscriptionID shared.ID) error {
	stored, err := o.Subscriptions.Find(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if stored.Subscription.FailureCount == 0 && stored.Subscription.LastError == "" {
		// Nothing to clear. Skipped rather than written, because a successful delivery per event
		// against a healthy subscription would otherwise be a row update per event.
		return nil
	}
	_, err = o.Subscriptions.Update(ctx, stored.Subscription.Delivered(), stored.Subscription.Version)
	return err
}

// Failed records a failed delivery and, when the run is long enough, disables the subscription and
// tells its owner.
// The parameters are plain rather than a struct, so that the delivery adapter can declare the two
// methods it needs as an interface of its own - an outbound adapter may not import a use case
// (project-structure.md §2), and this is the shape that lets the composition root put the two
// together.
//
// code is a message code of ours, never the target's response body (rule 10). terminal says the
// attempts have stopped - the delivery went to the dead letter - which is what the failure run
// counts: an attempt that will be retried is not yet a failed delivery.
func (o Outcomes) Failed(
	ctx context.Context, subscriptionID shared.ID, code string, terminal bool,
) error {
	if !terminal {
		// An attempt that will be retried is not yet a failed delivery. Counting attempts would
		// disable a subscription after three retries of one event, which is a target that was
		// briefly unreachable rather than one that is gone.
		return nil
	}

	stored, err := o.Subscriptions.Find(ctx, subscriptionID)
	if err != nil {
		return err
	}

	now := o.Clock.Now()
	after, justDisabled := stored.Subscription.Failed(now, code)
	if _, err := o.Subscriptions.Update(ctx, after, stored.Subscription.Version); err != nil {
		return err
	}
	if !justDisabled {
		return nil
	}

	if err := o.recordDisabled(ctx, after, now); err != nil {
		return err
	}
	if o.Notifier == nil {
		return nil
	}
	return o.Notifier.WebhookDisabled(ctx, after.TenantID, after.CreatedBy, after.ID)
}

// recordDisabled writes the trail entry for a subscription the system stopped by itself.
//
// The actor is the system rather than a person, which is the point of recording it: nobody decided
// this, and an entry that named the last person to touch the subscription would be saying they did.
func (o Outcomes) recordDisabled(
	ctx context.Context, subscription domain.WebhookSubscription, at time.Time,
) error {
	return o.Audit.Append(ctx, audit.Entry{
		TenantID:   subscription.TenantID,
		OccurredAt: at,
		Action:     WebhookDisabledAction,
		Outcome:    audit.OutcomeSuccess,
		// Notice: this workspace's events have stopped reaching somewhere they were going, and
		// nobody asked for that.
		Severity:   audit.SeverityNotice,
		ActorKind:  shared.ActorSystem,
		TargetType: webhookTarget,
		TargetID:   subscription.ID,
		Changes: audit.Changes(
			audit.Change{Field: "state", Classification: audit.Open, To: string(subscription.State)},
			audit.Change{Field: "last_error", Classification: audit.Open, To: subscription.LastError},
			audit.Change{
				Field: "failure_count", Classification: audit.Open,
				To: strconv.Itoa(subscription.FailureCount),
			},
		),
	})
}
