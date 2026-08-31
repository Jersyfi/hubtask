// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"strconv"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

const (
	SendWebhookName = "SendWebhook"

	// WebhookSentAction is a delivery somebody asked for by name - through the API, or through a
	// rule's SEND_WEBHOOK action running as its account. An act rather than machinery, which is
	// why it is audited where the fan-out's ordinary deliveries are not: the fan-out delivers what
	// a subscription asked to receive, and this delivers what a caller decided to send.
	WebhookSentAction audit.Action = "webhooks.delivery_sent"
)

// EventSource reads the event a send names, as it was written.
//
// Narrow rather than the outbox port, for the reason every slice here is: a use case that held the
// outbox could append to it.
type EventSource interface {
	FindEvent(ctx context.Context, id shared.ID) (event.Envelope, error)
}

// SendWebhook delivers one event to one named subscription (G-09, automation.md §1.3).
//
// Through G-03's one pipeline: the same delivery table, the same signature, the same retry ladder
// and the same dead letter. It enqueues rather than calls - the actual HTTP happens on the
// delivery job, outside any transaction, because an external call from inside one holds a database
// connection for as long as somebody else's server feels like taking
// (observability-reliability.md §8). That is also what makes this safe as a rule action: the
// engine's handler runs inside the queue runner's transaction, and what this use case does there
// is two inserts.
//
// The subscription's own event-type filter is deliberately not consulted. The fan-out's filter
// answers "what did this subscription ask to receive"; a send answers "what did this caller decide
// to deliver", and naming the subscription is the point of the call.
type SendWebhook struct {
	Writer Writer
	// Jobs is where the delivery is queued - the same kind, so one code path sends every webhook
	// this system has ever sent.
	Jobs   Queue
	Events EventSource
}

// Execute queues the delivery.
func (h SendWebhook) Execute(
	ctx context.Context, actor appshared.ActorContext, subscriptionID, eventID shared.ID,
) (domain.WebhookDelivery, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, WebhookSentAction, subscriptionID); err != nil {
		return domain.WebhookDelivery{}, err
	}

	var sent domain.WebhookDelivery
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := w.Subscriptions.Find(ctx, subscriptionID)
		if err != nil {
			return err
		}
		if stored.Subscription.State != domain.SubscriptionActive {
			// Refused rather than queued to fail: a paused subscription is somebody's deliberate
			// hold, and a disabled one is a target this system has concluded is unreachable.
			return shared.ErrConflict.
				WithDetail("webhooks.subscription_not_active").
				WithParams(map[string]string{"state": string(stored.Subscription.State)})
		}

		envelope, err := h.Events.FindEvent(ctx, eventID)
		if err != nil {
			return err
		}
		if envelope.Replay {
			// The first prohibition of backup-restore.md §8.4: an event a restore produced reports
			// last month's state, and no transport may hand it to an external system - not even a
			// caller asking by name.
			return shared.ErrValidation.
				WithDetail("webhooks.replay_not_deliverable").
				WithParams(map[string]string{"event_id": eventID.String()})
		}

		now := w.Clock.Now()
		delivery, err := domain.NewWebhookDelivery(
			w.IDs.NewID(), actor.TenantID, subscriptionID, envelope.ID, 1, now)
		if err != nil {
			return err
		}
		if err := w.Deliveries.Insert(ctx, delivery); err != nil {
			return err
		}
		if _, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindWebhookDeliver, TenantID: actor.TenantID, RunAt: now,
			Payload: map[string]any{
				"subscription_id": subscriptionID.String(),
				"delivery_id":     delivery.ID.String(),
				"event_id":        delivery.EventID.String(),
			},
		}); err != nil {
			return err
		}

		sent = delivery
		return w.recordSend(ctx, actor, stored.Subscription, delivery, now)
	})
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	return sent, nil
}

// recordSend writes the evidence for a delivery somebody asked for by name.
func (w Writer) recordSend(
	ctx context.Context, actor appshared.ActorContext,
	subscription domain.WebhookSubscription, delivery domain.WebhookDelivery, at time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   subscription.TenantID,
		OccurredAt: at,
		Action:     WebhookSentAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: deliveryTarget,
		TargetID:   delivery.ID,
		Changes: audit.Changes(
			audit.Change{Field: "subscription_id", Classification: audit.Open, To: subscription.ID.String()},
			audit.Change{Field: "event_id", Classification: audit.Open, To: delivery.EventID.String()},
			audit.Change{Field: "attempt", Classification: audit.Open, To: strconv.Itoa(delivery.Attempt)},
		),
	})
}

// Descriptor is the catalogue entry - and the automation action SEND_WEBHOOK with it, derived as
// every action is (automation.md §1.3).
func (h SendWebhook) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SendWebhookName,
		Summary: "Delivers one event to one named subscription, through the same pipeline every " +
			"delivery takes: the same delivery table, signature, retry ladder and dead letter. " +
			"The subscription's event-type filter is not consulted - naming it is the point - " +
			"but a paused or disabled subscription is refused, and a replayed event is never " +
			"delivered. As a rule action, the rule names the subscription and the run supplies " +
			"the event it is about.",
		SideEffects: "Records a delivery, queues it, and writes an audit entry. The HTTP call " +
			"happens on the delivery job, with its own retries.",
		TokenScope: automationScope,
		Input: []usecase.Field{
			{
				Name: "subscription_id", Kind: usecase.KindID, Required: true,
				Description: "Which subscription receives it.",
			},
			{
				Name: "event_id", Kind: usecase.KindID, Required: true,
				Description: "The event to deliver. A rule leaves this out and the run supplies " +
					"the event it is about.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: WebhookSentAction, TargetType: deliveryTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A delivery is about the workspace's stream rather than about one entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SendWebhook) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	subscriptionID, err := in.ID("subscription_id")
	if err != nil {
		return nil, err
	}
	eventID, err := in.ID("event_id")
	if err != nil {
		return nil, err
	}

	sent, err := h.Execute(ctx, actor, subscriptionID, eventID)
	if err != nil {
		return nil, err
	}
	return deliveryOutput(sent), nil
}
