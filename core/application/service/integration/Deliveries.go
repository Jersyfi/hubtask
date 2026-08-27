// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	ListWebhookDeliveriesName = "ListWebhookDeliveries"
	ReplayWebhookDeliveryName = "ReplayWebhookDelivery"
	RotateWebhookSecretName   = "RotateWebhookSecret" //nolint:gosec // G101: a use case name, not a credential.

	// DefaultDeliveryPage and MaxDeliveryPage are api-guidelines.md §4's numbers. A delivery log
	// is the one webhook read that is paged: a busy integration produces a row per event, where a
	// workspace has a handful of subscriptions.
	DefaultDeliveryPage = 50
	MaxDeliveryPage     = 200

	// WebhookReplayedAction is the audit code for a delivery sent again by hand. An act rather
	// than machinery: somebody looked at a dead letter and decided the target is ready for it.
	WebhookReplayedAction audit.Action = "webhooks.delivery_replayed"

	deliveryTarget = "webhook_delivery"
)

// Queue is the slice of the job queue this package needs. Declared here rather than imported whole
// so that what the webhooks can enqueue is visible in one place: one kind, and no other.
type Queue interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// ListWebhookDeliveries answers what was delivered and what became of it.
type ListWebhookDeliveries struct{ Writer Writer }

// DeliveryListing is what a listing asks for.
type DeliveryListing struct {
	SubscriptionID shared.ID
	Status         string
	Cursor         shared.ID
	PageSize       int
}

// Execute reads the log.
//
// The subscription is read first and its absence answered as absence, so that a delivery listing
// for a subscription of another tenant says the same thing as one for a subscription that never
// existed (T-04).
func (h ListWebhookDeliveries) Execute(
	ctx context.Context, actor appshared.ActorContext, listing DeliveryListing,
) ([]domain.WebhookDelivery, bool, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, WebhookReadAction, listing.SubscriptionID); err != nil {
		return nil, false, err
	}
	status, err := deliveryStatus(listing.Status)
	if err != nil {
		return nil, false, err
	}

	size := pageSize(listing.PageSize)
	var (
		deliveries []domain.WebhookDelivery
		hasMore    bool
	)
	err = w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := w.Subscriptions.Find(ctx, listing.SubscriptionID); err != nil {
			return err
		}

		// One more than the page, which is how "is there another page" is answered without a
		// second query and without counting the table.
		found, err := w.Deliveries.List(ctx, repository.DeliveryQuery{
			SubscriptionID: listing.SubscriptionID, Status: status,
			Before: listing.Cursor, PageSize: size + 1,
		})
		if err != nil {
			return err
		}
		if len(found) > size {
			hasMore = true
			found = found[:size]
		}
		deliveries = found
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return deliveries, hasMore, nil
}

// ReplayWebhookDelivery sends a dead-lettered delivery again.
type ReplayWebhookDelivery struct {
	Writer Writer
	// Jobs is where the new attempt is queued. The replay does not deliver: it records the attempt
	// and asks for it, exactly as the first attempt was asked for, so that one code path sends
	// every webhook this system has ever sent.
	Jobs Queue
}

// Execute queues the replay.
func (h ReplayWebhookDelivery) Execute(
	ctx context.Context, actor appshared.ActorContext, subscriptionID, deliveryID shared.ID,
) (domain.WebhookDelivery, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, WebhookReplayedAction, subscriptionID); err != nil {
		return domain.WebhookDelivery{}, err
	}

	var replayed domain.WebhookDelivery
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := w.Subscriptions.Find(ctx, subscriptionID)
		if err != nil {
			return err
		}
		previous, err := w.Deliveries.Find(ctx, deliveryID)
		if err != nil {
			return err
		}
		if previous.SubscriptionID != subscriptionID {
			// A delivery of another subscription, named through this one's path. Not found rather
			// than forbidden, for the reason every other read of somebody else's thing is (T-04).
			return shared.ErrNotFound.
				WithDetail("webhooks.delivery_not_found").
				WithParams(map[string]string{"delivery_id": deliveryID.String()})
		}

		now := w.Clock.Now()
		attempt, err := previous.Replayed(w.IDs.NewID(), now)
		if err != nil {
			return err
		}
		attempt.TenantID = actor.TenantID
		if err := w.Deliveries.Insert(ctx, attempt); err != nil {
			return err
		}

		if _, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindWebhookDeliver, TenantID: actor.TenantID, RunAt: now,
			Payload: map[string]any{
				"subscription_id": subscriptionID.String(),
				"delivery_id":     attempt.ID.String(),
				"event_id":        attempt.EventID.String(),
			},
		}); err != nil {
			return err
		}

		replayed = attempt
		return w.recordReplay(ctx, actor, stored.Subscription, attempt, now)
	})
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	return replayed, nil
}

// RotateWebhookSecret issues a new signing secret and keeps the old one verifying for a grace.
type RotateWebhookSecret struct{ Writer Writer }

// Execute rotates.
//
// Signatures are computed with the current secret from this moment on; what the grace buys is the
// subscriber's side of the check. That asymmetry is the whole point of the route - a subscriber
// cannot deploy atomically, and a rotation that took effect on both sides at once would drop every
// event arriving between the call and the deployment.
func (h RotateWebhookSecret) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, grace time.Duration,
) (MintedSubscription, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, WebhookRotatedAction, id); err != nil {
		return MintedSubscription{}, err
	}

	material, err := w.Entropy.Bytes(SecretBytes)
	if err != nil {
		return MintedSubscription{}, shared.ErrInternal.
			WithDetail("webhooks.secret_undrawable").
			WithCause(err)
	}
	signing := secret.New(encodeSecret(material))

	var rotated domain.WebhookSubscription
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := w.Subscriptions.Find(ctx, id)
		if err != nil {
			return err
		}

		now := w.Clock.Now()
		after, err := stored.Subscription.Rotated(now, grace)
		if err != nil {
			return err
		}

		sealed, err := w.Encryptor.Seal(ctx, signing, SecretPurpose(id))
		if err != nil {
			return err
		}
		changed, err := w.Subscriptions.Rotate(ctx, id,
			repository.SealedSecret{Ciphertext: sealed.Ciphertext, KeyID: sealed.KeyID},
			after.PreviousSecretUntil, stored.Subscription.Version)
		if err != nil {
			return err
		}
		if !changed {
			return shared.ErrConflict.
				WithDetail("webhooks.subscription_version_conflict").
				WithParams(map[string]string{"webhook_id": id.String()})
		}

		rotated = after
		return w.recordAudit(ctx, actor, after, WebhookRotatedAction, now, []audit.Change{{
			// When the old secret stops working, which is what a subscriber needs to know and the
			// one thing about a rotation an auditor can act on. The secret itself is nowhere.
			Field: "previous_secret_until", Classification: audit.Open,
			To: graceText(after.PreviousSecretUntil),
		}})
	})
	if err != nil {
		return MintedSubscription{}, err
	}
	return MintedSubscription{Subscription: rotated, Secret: signing}, nil
}

// recordReplay writes the evidence for a delivery sent again by hand.
func (w Writer) recordReplay(
	ctx context.Context, actor appshared.ActorContext,
	subscription domain.WebhookSubscription, delivery domain.WebhookDelivery, at time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   subscription.TenantID,
		OccurredAt: at,
		Action:     WebhookReplayedAction,
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

// deliveryStatus reads the filter. An unknown one is refused rather than treated as "everything":
// a caller who mistyped DEAD_LETTER and got the whole log back would read it as an empty dead
// letter.
func deliveryStatus(raw string) (domain.DeliveryStatus, error) {
	switch domain.DeliveryStatus(raw) {
	case "":
		return "", nil
	case domain.DeliveryPending, domain.DeliverySucceeded,
		domain.DeliveryFailed, domain.DeliveryDeadLetter:
		return domain.DeliveryStatus(raw), nil
	default:
		return "", shared.ErrValidation.
			WithDetail("webhooks.delivery_status_unknown").
			WithParams(map[string]string{"status": raw}).
			WithFields(shared.FieldError{Path: "/status", Code: "webhooks.delivery_status_unknown"})
	}
}

func pageSize(requested int) int {
	switch {
	case requested <= 0:
		return DefaultDeliveryPage
	case requested > MaxDeliveryPage:
		return MaxDeliveryPage
	default:
		return requested
	}
}

func graceText(until time.Time) string {
	if until.IsZero() {
		// A rotation with no grace: the old secret stopped working at the same moment the new one
		// started, which is what a leak calls for.
		return "none"
	}
	return until.UTC().Format(time.RFC3339)
}

// deliveryOutput is the projection every channel gets.
func deliveryOutput(delivery domain.WebhookDelivery) usecase.Output {
	out := usecase.Output{
		"id":              delivery.ID.String(),
		"subscription_id": delivery.SubscriptionID.String(),
		"event_id":        delivery.EventID.String(),
		"attempt":         delivery.Attempt,
		"status":          string(delivery.Status),
		"created_at":      delivery.CreatedAt.UTC(),
		"response_status": nil,
		"error_code":      nil,
		"next_attempt_at": nil,
	}
	if delivery.ResponseStatus != 0 {
		out["response_status"] = delivery.ResponseStatus
	}
	if delivery.ErrorCode != "" {
		out["error_code"] = delivery.ErrorCode
	}
	if !delivery.NextAttemptAt.IsZero() {
		out["next_attempt_at"] = delivery.NextAttemptAt.UTC()
	}
	return out
}

// Descriptor is the catalogue entry.
func (h ListWebhookDeliveries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListWebhookDeliveriesName,
		Summary: "One subscription's delivery attempts, newest first, with the status, the " +
			"response the target gave and when the next attempt is due. A run of eight failures " +
			"ends in DEAD_LETTER, which is where an operator looks and what a replay acts on.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "webhook_id", Kind: usecase.KindID, Required: true, Description: "Which subscription."},
			{
				Name: "status", Kind: usecase.KindString,
				Enum:        []string{"PENDING", "SUCCEEDED", "FAILED", "DEAD_LETTER"},
				Description: "Narrow to one outcome. DEAD_LETTER is the one an operator usually wants.",
			},
			{Name: "cursor", Kind: usecase.KindID, Description: "The next_cursor of the previous page."},
			{Name: "size", Kind: usecase.KindInt, Description: "How many, at most 200."},
		},
		Audit: usecase.AuditDeclaration{
			Action: WebhookReadAction, TargetType: webhookTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListWebhookDeliveries) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("webhook_id")
	if err != nil {
		return nil, err
	}
	cursor, err := in.ID("cursor")
	if err != nil {
		return nil, err
	}

	deliveries, hasMore, err := h.Execute(ctx, actor, DeliveryListing{
		SubscriptionID: id, Status: in.String("status"), Cursor: cursor, PageSize: in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(deliveries))
	for _, delivery := range deliveries {
		rows = append(rows, deliveryOutput(delivery))
	}

	page := map[string]any{"next_cursor": nil, "has_more": hasMore}
	if hasMore && len(deliveries) > 0 {
		// The cursor is the last identifier of the page. Identifiers are time-ordered, so "older
		// than this one" is the next page and no opaque encoding is needed.
		page["next_cursor"] = deliveries[len(deliveries)-1].ID.String()
	}
	return usecase.Output{"data": rows, "page": page}, nil
}

// Descriptor is the catalogue entry.
func (h ReplayWebhookDelivery) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReplayWebhookDeliveryName,
		Summary: "Sends a dead-lettered delivery again, after whatever made it fail has been " +
			"fixed. The event identifier is the one it always had, so a subscriber that " +
			"deduplicates on X-Hubtask-Event-Id sees the repeat for what it is; the attempt " +
			"counter carries on rather than resetting, so the log stays a true account of how " +
			"many times this event was sent. Only a dead-lettered delivery can be replayed.",
		SideEffects: "Records a new attempt, queues it, and writes an audit entry.",
		TokenScope:  automationScope,
		Input: []usecase.Field{
			{Name: "webhook_id", Kind: usecase.KindID, Required: true, Description: "Which subscription."},
			{Name: "delivery_id", Kind: usecase.KindID, Required: true, Description: "Which dead-lettered delivery."},
		},
		Audit: usecase.AuditDeclaration{
			Action: WebhookReplayedAction, TargetType: deliveryTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A delivery is about the workspace's stream rather than about one entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReplayWebhookDelivery) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	subscriptionID, err := in.ID("webhook_id")
	if err != nil {
		return nil, err
	}
	deliveryID, err := in.ID("delivery_id")
	if err != nil {
		return nil, err
	}

	replayed, err := h.Execute(ctx, actor, subscriptionID, deliveryID)
	if err != nil {
		return nil, err
	}
	return deliveryOutput(replayed), nil
}

// Descriptor is the catalogue entry.
func (h RotateWebhookSecret) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RotateWebhookSecretName,
		Summary: "Issues a new signing secret and answers it once, keeping the old one verifying " +
			"for a grace period. That overlap is the point: a subscriber cannot deploy " +
			"atomically, and a rotation taking effect instantly would drop every event arriving " +
			"between the call and the deployment. Signatures use the new secret from this moment " +
			"on; the grace is the subscriber's side of the check. Zero retires the old secret at " +
			"once, which is what a leak calls for.",
		SideEffects: "Replaces the secret, stamps the grace, writes an audit entry, and answers a secret.",
		TokenScope:  automationScope,
		Input: []usecase.Field{
			{Name: "webhook_id", Kind: usecase.KindID, Required: true, Description: "Which subscription."},
			{
				Name: "grace_seconds", Kind: usecase.KindInt,
				Description: "How long the previous secret keeps verifying. At most seven days; " +
					"omitted means a day; zero retires it at once.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: WebhookRotatedAction, TargetType: webhookTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A credential is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RotateWebhookSecret) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("webhook_id")
	if err != nil {
		return nil, err
	}

	// Absent means the default; an explicit zero means "retire it now". The two are different
	// instructions, and Present is what tells them apart.
	grace := domain.DefaultRotationGrace
	if in.Present("grace_seconds") {
		grace = time.Duration(in.Int("grace_seconds")) * time.Second
	}

	minted, err := h.Execute(ctx, actor, id, grace)
	if err != nil {
		return nil, err
	}

	out := subscriptionOutput(minted.Subscription)
	out["secret"] = minted.Secret.Reveal()
	return out, nil
}
