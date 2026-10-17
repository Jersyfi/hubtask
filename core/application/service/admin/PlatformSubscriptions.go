// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"encoding/base64"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	ListPlatformSubscriptionsName  = "ListPlatformSubscriptions"
	CreatePlatformSubscriptionName = "CreatePlatformSubscription"
	ReadPlatformSubscriptionName   = "ReadPlatformSubscription"

	platformSubscriptionCreatedAction audit.Action = "instance.platform_subscription_created"
	platformSubscriptionReadAction    audit.Action = "instance.platform_subscription_read"

	platformSubscriptionTarget = "platform_subscription"

	// platformSecretBytes is a webhook secret's length: the same signing, the same strength.
	platformSecretBytes = 32

	// platformStepUp is the descriptors' sentence (identity.md §16.3).
	platformStepUp = "sending every workspace's reference and state to an address outside the installation"
)

// PlatformWriter is the control plane's half of the installation's subscriptions for a purchase
// platform (UC-INS-14): the operator's authorisation, multi mode, the sealing of the secret and the
// step-up a new address demands.
type PlatformWriter struct {
	Instance      InstanceWriter
	Subscriptions adminrepo.PlatformSubscriptions
	Encryptor     crypto.Encryptor
	Entropy       clock.Entropy
	StepUp        stepup.Verifier
	Tenancy       env.TenancyMode
	// Enforced is the instance file deciding the subscriptions: a change through the API is
	// refused, as the file's plans are.
	Enforced bool
}

func (w PlatformWriter) authorize(ctx context.Context, actor appshared.ActorContext) error {
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return err
	}
	return requireMulti(w.Tenancy)
}

// writable is authorize for a change: refused while the file decides.
func (w PlatformWriter) writable(ctx context.Context, actor appshared.ActorContext) error {
	if err := w.authorize(ctx, actor); err != nil {
		return err
	}
	if w.Enforced {
		return shared.ErrConflict.WithDetail("admin.instance_enforced_from_file")
	}
	return nil
}

// mint draws a signing secret and seals it for the subscription.
func (w PlatformWriter) mint(ctx context.Context, id shared.ID) (secret.Secret, adminrepo.PlatformSecret, error) {
	material, err := w.Entropy.Bytes(platformSecretBytes)
	if err != nil {
		return secret.Secret{}, adminrepo.PlatformSecret{}, shared.ErrInternal.
			WithDetail("webhooks.secret_undrawable").WithCause(err)
	}
	signing := secret.New(base64.RawURLEncoding.EncodeToString(material))
	sealed, err := w.seal(ctx, id, signing)
	return signing, sealed, err
}

func (w PlatformWriter) seal(ctx context.Context, id shared.ID, signing secret.Secret) (adminrepo.PlatformSecret, error) {
	sealed, err := w.Encryptor.Seal(ctx, signing, crypto.Purpose(integration.PlatformSecretPurposeFor(id)))
	if err != nil {
		return adminrepo.PlatformSecret{}, err
	}
	return adminrepo.PlatformSecret{Ciphertext: sealed.Ciphertext, KeyID: sealed.KeyID}, nil
}

// journal records an act on a subscription in the installation's own evidence: the subscription by
// its id, never its address or secret.
func (w PlatformWriter) journal(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, id shared.ID, details map[string]any,
) error {
	if details == nil {
		details = map[string]any{}
	}
	details["subscription_id"] = id.String()
	return w.Instance.Journal.Record(ctx, adminrepo.InstanceEvent{
		ID: w.Instance.IDs.NewID(), OccurredAt: w.Instance.Clock.Now(),
		Action: string(action), ActorLabel: actor.AccountName, Details: details,
	})
}

// find reads one under the installation's scope.
func (w PlatformWriter) find(ctx context.Context, id shared.ID) (adminrepo.StoredPlatformSubscription, error) {
	var stored adminrepo.StoredPlatformSubscription
	err := w.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		read, err := w.Subscriptions.Find(ctx, id)
		stored = read
		return err
	})
	return stored, err
}

// ListPlatformSubscriptions answers the installation's subscriptions.
type ListPlatformSubscriptions struct{ Writer PlatformWriter }

// Execute reads them.
func (h ListPlatformSubscriptions) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]integration.PlatformSubscription, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return nil, err
	}
	var listed []integration.PlatformSubscription
	err := h.Writer.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := h.Writer.Subscriptions.List(ctx)
			listed = read
			return err
		})
	return listed, err
}

// ReadPlatformSubscription answers one.
type ReadPlatformSubscription struct{ Writer PlatformWriter }

// Execute reads it.
func (h ReadPlatformSubscription) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (integration.PlatformSubscription, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return integration.PlatformSubscription{}, err
	}
	stored, err := h.Writer.find(ctx, id)
	return stored.Subscription, err
}

// CreatePlatformSubscription subscribes a purchase platform: every installation event is sent to
// the target, signed with the secret answered here once.
type CreatePlatformSubscription struct{ Writer PlatformWriter }

// MintedPlatformSubscription is the subscription with its secret, in clear, for the only time.
type MintedPlatformSubscription struct {
	Subscription integration.PlatformSubscription
	Secret       secret.Secret
}

// Execute checks, seals and stores - the step-up first, so a refused proof leaves nothing behind.
func (h CreatePlatformSubscription) Execute(
	ctx context.Context, actor appshared.ActorContext, target, stepUpToken string,
) (MintedPlatformSubscription, error) {
	w := h.Writer
	if err := w.writable(ctx, actor); err != nil {
		return MintedPlatformSubscription{}, err
	}
	subscription, err := integration.NewPlatformSubscription(
		w.Instance.IDs.NewID(), target, false, actor.AccountID, w.Instance.Clock.Now())
	if err != nil {
		return MintedPlatformSubscription{}, err
	}
	if err := stepup.Demand(ctx, w.StepUp, actor.TenantID, actor.AccountID, stepUpToken); err != nil {
		return MintedPlatformSubscription{}, err
	}
	signing, sealed, err := w.mint(ctx, subscription.ID)
	if err != nil {
		return MintedPlatformSubscription{}, err
	}
	err = w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		if err := w.Subscriptions.Insert(ctx, subscription, sealed); err != nil {
			return err
		}
		return w.journal(ctx, actor, platformSubscriptionCreatedAction, subscription.ID, nil)
	})
	if err != nil {
		return MintedPlatformSubscription{}, err
	}
	return MintedPlatformSubscription{Subscription: subscription, Secret: signing}, nil
}

// PlatformSubscriptionOutput is the contract's PlatformSubscription.
func PlatformSubscriptionOutput(s integration.PlatformSubscription) usecase.Output {
	out := usecase.Output{
		"id": s.ID.String(), "target_url": s.TargetURL, "state": string(s.State),
		"dead_letters": s.DeadLetters, "from_file": s.FromFile, "created_at": s.CreatedAt,
		"version": s.Version, "last_delivery_at": nil, "last_error": nil,
	}
	if !s.LastDeliveryAt.IsZero() {
		out["last_delivery_at"] = s.LastDeliveryAt
	}
	if s.LastError != "" {
		out["last_error"] = s.LastError
	}
	return out
}

var (
	platformStepUpField = usecase.Field{
		Name: "step_up_token", Kind: usecase.KindString,
		Description: "The proof POST /auth/step-up answered, consumed by this one request.",
	}
	platformSubscriptionIDField = usecase.Field{
		Name: "subscription_id", Kind: usecase.KindID, Required: true, Description: "The subscription.",
	}
	platformActivityExempt = usecase.ActivityDeclaration{
		Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
	}
)

func (h ListPlatformSubscriptions) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListPlatformSubscriptionsName,
		Summary: "The installation's subscriptions for a purchase platform: each address the " +
			"installation's events are pushed to, with its state, its dead letters and its last " +
			"delivery. The secret is not among the fields.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: platformSubscriptionReadAction, TargetType: platformSubscriptionTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: platformActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			listed, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			rows := make([]usecase.Output, 0, len(listed))
			for _, s := range listed {
				rows = append(rows, PlatformSubscriptionOutput(s))
			}
			return usecase.Output{"data": rows}, nil
		}),
	}
}

func (h ReadPlatformSubscription) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ReadPlatformSubscriptionName,
		Summary:     "One subscription of the installation, without its secret.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Input:       []usecase.Field{platformSubscriptionIDField},
		Audit: usecase.AuditDeclaration{
			Action: platformSubscriptionReadAction, TargetType: platformSubscriptionTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: platformActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("subscription_id")
			if err != nil {
				return nil, err
			}
			read, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return PlatformSubscriptionOutput(read), nil
		}),
	}
}

func (h CreatePlatformSubscription) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreatePlatformSubscriptionName,
		Summary: "Subscribes a purchase platform to the installation's events: every event is " +
			"POSTed to the target as a signed CloudEvent, retried as a workspace's webhook is, and " +
			"never switched off for being unreachable. The signing secret is answered once. " +
			"Demands a fresh step-up.",
		SideEffects: "Writes the subscription with its sealed secret and one journal entry.",
		TokenScope:  adminTenantsScope,
		StepUp:      platformStepUp,
		Input: []usecase.Field{
			{Name: "target_url", Kind: usecase.KindString, Required: true,
				Description: "Where each event is POSTed."},
			platformStepUpField,
		},
		Audit: usecase.AuditDeclaration{
			Action: platformSubscriptionCreatedAction, TargetType: platformSubscriptionTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: platformActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			minted, err := h.Execute(ctx, actor, in.String("target_url"), in.String("step_up_token"))
			if err != nil {
				return nil, err
			}
			out := PlatformSubscriptionOutput(minted.Subscription)
			out["secret"] = minted.Secret.Reveal()
			return out, nil
		}),
	}
}

const (
	UpdatePlatformSubscriptionName       = "UpdatePlatformSubscription"
	DeletePlatformSubscriptionName       = "DeletePlatformSubscription"
	RotatePlatformSubscriptionSecretName = "RotatePlatformSubscriptionSecret"

	platformSubscriptionChangedAction audit.Action = "instance.platform_subscription_changed"
	platformSubscriptionRemovedAction audit.Action = "instance.platform_subscription_removed"
	platformSubscriptionRotatedAction audit.Action = "instance.platform_subscription_rotated"
)

// UpdatePlatformSubscription changes a subscription's target or pauses and resumes it. A new
// target demands a fresh step-up; pausing and resuming do not - they send less, or what was sent
// before.
type UpdatePlatformSubscription struct{ Writer PlatformWriter }

// UpdatePlatformSubscriptionCommand is the change. Empty fields are left alone.
type UpdatePlatformSubscriptionCommand struct {
	ID              shared.ID
	TargetURL       string
	State           string
	ExpectedVersion int
	StepUpToken     string
}

// Execute changes it, guarded on the version read or the one the caller names.
func (h UpdatePlatformSubscription) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdatePlatformSubscriptionCommand,
) (integration.PlatformSubscription, error) {
	w := h.Writer
	if err := w.writable(ctx, actor); err != nil {
		return integration.PlatformSubscription{}, err
	}
	stored, err := w.find(ctx, cmd.ID)
	if err != nil {
		return integration.PlatformSubscription{}, err
	}
	// Before the step-up: a proof spent on a change that is then refused as stale would be burnt.
	expected := stored.Subscription.Version
	if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != expected {
		return integration.PlatformSubscription{}, shared.ErrVersionConflict.WithDetail("admin.subscription_version_moved")
	}
	changed := stored.Subscription
	if cmd.TargetURL != "" && cmd.TargetURL != changed.TargetURL {
		if changed, err = changed.Retargeted(cmd.TargetURL); err != nil {
			return integration.PlatformSubscription{}, err
		}
		if err := stepup.Demand(ctx, w.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken); err != nil {
			return integration.PlatformSubscription{}, err
		}
	}
	if cmd.State != "" {
		if changed, err = changed.InState(integration.PlatformState(cmd.State)); err != nil {
			return integration.PlatformSubscription{}, err
		}
	}
	if changed == stored.Subscription {
		return changed, nil
	}
	err = w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		written, err := w.Subscriptions.Update(ctx, changed, expected, w.Instance.Clock.Now())
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrVersionConflict.WithDetail("admin.subscription_version_moved")
		}
		changed.Version = expected + 1
		return w.journal(ctx, actor, platformSubscriptionChangedAction, changed.ID, map[string]any{
			"state": string(changed.State), "target_changed": changed.TargetURL != stored.Subscription.TargetURL,
		})
	})
	if err != nil {
		return integration.PlatformSubscription{}, err
	}
	return changed, nil
}

// DeletePlatformSubscription unsubscribes; the deliveries go with it, the events stay readable.
type DeletePlatformSubscription struct{ Writer PlatformWriter }

// Execute removes it.
func (h DeletePlatformSubscription) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	w := h.Writer
	if err := w.writable(ctx, actor); err != nil {
		return err
	}
	return w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		removed, err := w.Subscriptions.Delete(ctx, id)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("admin.subscription_not_found")
		}
		return w.journal(ctx, actor, platformSubscriptionRemovedAction, id, nil)
	})
}

// RotatePlatformSubscriptionSecret issues a new signing secret, answered once; deliveries are
// signed with it from this moment.
type RotatePlatformSubscriptionSecret struct{ Writer PlatformWriter }

// Execute rotates it, the step-up first.
func (h RotatePlatformSubscriptionSecret) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, stepUpToken string,
) (MintedPlatformSubscription, error) {
	w := h.Writer
	if err := w.writable(ctx, actor); err != nil {
		return MintedPlatformSubscription{}, err
	}
	stored, err := w.find(ctx, id)
	if err != nil {
		return MintedPlatformSubscription{}, err
	}
	if err := stepup.Demand(ctx, w.StepUp, actor.TenantID, actor.AccountID, stepUpToken); err != nil {
		return MintedPlatformSubscription{}, err
	}
	signing, sealed, err := w.mint(ctx, id)
	if err != nil {
		return MintedPlatformSubscription{}, err
	}
	err = w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		rotated, err := w.Subscriptions.Rotate(ctx, id, sealed, w.Instance.Clock.Now())
		if err != nil {
			return err
		}
		if !rotated {
			return shared.ErrNotFound.WithDetail("admin.subscription_not_found")
		}
		return w.journal(ctx, actor, platformSubscriptionRotatedAction, id, nil)
	})
	if err != nil {
		return MintedPlatformSubscription{}, err
	}
	stored.Subscription.Version++
	return MintedPlatformSubscription{Subscription: stored.Subscription, Secret: signing}, nil
}

func (h UpdatePlatformSubscription) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdatePlatformSubscriptionName,
		Summary: "Changes a subscription of the installation: its target, which demands a fresh " +
			"step-up, or its state - paused, it is sent nothing, and the platform reads what it " +
			"missed from the event feed. Never switched off for being unreachable.",
		SideEffects: "Writes the subscription and one journal entry.",
		TokenScope:  adminTenantsScope,
		StepUp:      platformStepUp,
		Input: []usecase.Field{
			platformSubscriptionIDField,
			{Name: "target_url", Kind: usecase.KindString, Description: "A new target."},
			{Name: "state", Kind: usecase.KindString, Description: "ACTIVE or PAUSED."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read (If-Match)."},
			platformStepUpField,
		},
		Audit: usecase.AuditDeclaration{
			Action: platformSubscriptionChangedAction, TargetType: platformSubscriptionTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: platformActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("subscription_id")
			if err != nil {
				return nil, err
			}
			changed, err := h.Execute(ctx, actor, UpdatePlatformSubscriptionCommand{
				ID: id, TargetURL: in.String("target_url"), State: in.String("state"),
				ExpectedVersion: in.Int("expected_version"), StepUpToken: in.String("step_up_token"),
			})
			if err != nil {
				return nil, err
			}
			return PlatformSubscriptionOutput(changed), nil
		}),
	}
}

func (h DeletePlatformSubscription) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        DeletePlatformSubscriptionName,
		Summary:     "Unsubscribes a purchase platform: the subscription goes with its deliveries; the events stay readable.",
		SideEffects: "Removes the subscription and its deliveries, and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		Destructive: true,
		Input:       []usecase.Field{platformSubscriptionIDField},
		Audit: usecase.AuditDeclaration{
			Action: platformSubscriptionRemovedAction, TargetType: platformSubscriptionTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: platformActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("subscription_id")
			if err != nil {
				return nil, err
			}
			return usecase.Output{}, h.Execute(ctx, actor, id)
		}),
	}
}

func (h RotatePlatformSubscriptionSecret) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RotatePlatformSubscriptionSecretName,
		Summary: "Issues a new signing secret for a subscription of the installation, answered " +
			"once; deliveries are signed with it from this moment. Demands a fresh step-up.",
		SideEffects: "Seals and stores the new secret, and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		StepUp:      platformStepUp,
		Input:       []usecase.Field{platformSubscriptionIDField, platformStepUpField},
		Audit: usecase.AuditDeclaration{
			Action: platformSubscriptionRotatedAction, TargetType: platformSubscriptionTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: platformActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("subscription_id")
			if err != nil {
				return nil, err
			}
			minted, err := h.Execute(ctx, actor, id, in.String("step_up_token"))
			if err != nil {
				return nil, err
			}
			out := PlatformSubscriptionOutput(minted.Subscription)
			out["secret"] = minted.Secret.Reveal()
			return out, nil
		}),
	}
}

const (
	ListPlatformDeliveriesName        = "ListPlatformDeliveries"
	RedeliverPlatformSubscriptionName = "RedeliverPlatformSubscription"

	platformSubscriptionRedeliveredAction audit.Action = "instance.platform_subscription_redelivered"
	platformDeliveriesReadAction          audit.Action = "instance.platform_deliveries_read"

	// redeliverBatch bounds one statement and one transaction; redeliverRounds one call, so a
	// backlog of thousands is queued in bounded steps rather than in one transaction.
	redeliverBatch  = 100
	redeliverRounds = 100
)

// DeliveryCursors turn the last delivery of a page into an opaque cursor and back.
type DeliveryCursors interface {
	EncodeID(id shared.ID) string
	DecodeID(cursor string) (shared.ID, error)
}

// PlatformJobs is where a redelivered dead letter's attempt is asked for.
type PlatformJobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// ListPlatformDeliveries answers what was sent to a subscription and what became of it, newest
// first, a page at a time (UC-INS-14 check 5).
type ListPlatformDeliveries struct {
	Writer     PlatformWriter
	Deliveries adminrepo.PlatformDeliveries
	Cursors    DeliveryCursors
}

// PlatformDeliveriesPage is one page.
type PlatformDeliveriesPage struct {
	Deliveries []integration.PlatformDelivery
	Next       shared.ID
	More       bool
}

// Execute reads one page.
func (h ListPlatformDeliveries) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, status, cursor string, size int,
) (PlatformDeliveriesPage, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return PlatformDeliveriesPage{}, err
	}
	var before shared.ID
	if cursor != "" {
		decoded, err := h.Cursors.DecodeID(cursor)
		if err != nil {
			return PlatformDeliveriesPage{}, err
		}
		before = decoded
	}
	limit := platformEventsPageSize(size)
	var page PlatformDeliveriesPage
	err := h.Writer.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		if _, err := h.Writer.Subscriptions.Find(ctx, id); err != nil {
			return err
		}
		read, err := h.Deliveries.Page(ctx, id, status, before, limit+1)
		if err != nil {
			return err
		}
		if page.More = len(read) > limit; page.More {
			read = read[:limit]
		}
		page.Deliveries = read
		if page.More {
			page.Next = read[len(read)-1].ID
		}
		return nil
	})
	return page, err
}

// RedeliverPlatformSubscription queues every dead letter of a subscription again, with the event
// identifier it always had and its attempts counting on (UC-INS-14 checks 5 and 6).
type RedeliverPlatformSubscription struct {
	Writer     PlatformWriter
	Deliveries adminrepo.PlatformDeliveries
	Jobs       PlatformJobs
}

// Execute queues them in bounded batches, each in a transaction of its own; a dead letter another
// redelivery took meanwhile is queued by that one alone.
func (h RedeliverPlatformSubscription) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (int, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return 0, err
	}
	stored, err := w.find(ctx, id)
	if err != nil {
		return 0, err
	}
	if stored.Subscription.State == integration.PlatformPaused {
		return 0, shared.ErrConflict.WithDetail("admin.subscription_paused")
	}
	queued := 0
	for range redeliverRounds {
		var batch []shared.ID
		err := w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			now := w.Instance.Clock.Now()
			requeued, err := h.Deliveries.Requeue(ctx, id, now, redeliverBatch)
			if err != nil {
				return err
			}
			for _, delivery := range requeued {
				if _, err := h.Jobs.Enqueue(ctx, queue.Request{
					Kind: queue.KindPlatformDeliver, RunAt: now.UTC(),
					Payload: map[string]any{"delivery_id": delivery.String()},
				}); err != nil {
					return err
				}
			}
			batch = requeued
			if len(requeued) == 0 {
				return nil
			}
			// Journalled with its batch, so what was queued is recorded however a later batch ends.
			return w.journal(ctx, actor, platformSubscriptionRedeliveredAction, id, map[string]any{"queued": len(requeued)})
		})
		if err != nil {
			return queued, err
		}
		queued += len(batch)
		if len(batch) < redeliverBatch {
			break
		}
	}
	return queued, nil
}

// PlatformDeliveryOutput is the contract's PlatformDelivery.
func PlatformDeliveryOutput(d integration.PlatformDelivery) usecase.Output {
	out := usecase.Output{
		"id": d.ID.String(), "subscription_id": d.SubscriptionID.String(), "event_id": d.EventID.String(),
		"event_type": d.EventType, "attempt": d.Attempt, "status": string(d.Status),
		"response_status": nil, "error_code": nil, "next_attempt_at": nil,
		"created_at": d.CreatedAt, "updated_at": d.UpdatedAt,
	}
	if d.ResponseStatus != 0 {
		out["response_status"] = d.ResponseStatus
	}
	if d.ErrorCode != "" {
		out["error_code"] = d.ErrorCode
	}
	if !d.NextAttemptAt.IsZero() {
		out["next_attempt_at"] = d.NextAttemptAt
	}
	return out
}

func (h ListPlatformDeliveries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListPlatformDeliveriesName,
		Summary: "What was sent to a subscription of the installation and what became of it, newest " +
			"first: one row per event with its attempts, status, the answer the target gave and the " +
			"next attempt. DEAD_LETTER is what a redelivery acts on.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			platformSubscriptionIDField,
			{Name: "status", Kind: usecase.KindString, Description: "PENDING, SUCCEEDED, FAILED or DEAD_LETTER."},
			{Name: "cursor", Kind: usecase.KindString, Description: "Where the last page ended."},
			{Name: "limit", Kind: usecase.KindInt, Description: "How many at most: fifty by default, two hundred at most."},
		},
		Audit: usecase.AuditDeclaration{
			Action: platformDeliveriesReadAction, TargetType: platformSubscriptionTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: platformActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("subscription_id")
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, id, in.String("status"), in.String("cursor"), in.Int("limit"))
			if err != nil {
				return nil, err
			}
			rows := make([]usecase.Output, 0, len(page.Deliveries))
			for _, d := range page.Deliveries {
				rows = append(rows, PlatformDeliveryOutput(d))
			}
			var next any
			if page.More {
				next = h.Cursors.EncodeID(page.Next)
			}
			return usecase.Output{"data": rows, "page": map[string]any{"next_cursor": next, "has_more": page.More}}, nil
		}),
	}
}

func (h RedeliverPlatformSubscription) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RedeliverPlatformSubscriptionName,
		Summary: "Queues every dead letter of a subscription of the installation again, with the " +
			"event identifier it always had, its attempts counting on. Refused while the " +
			"subscription is paused.",
		SideEffects: "Returns the dead letters to pending, queues their attempts, and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		Input:       []usecase.Field{platformSubscriptionIDField},
		Audit: usecase.AuditDeclaration{
			Action: platformSubscriptionRedeliveredAction, TargetType: platformSubscriptionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: platformActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("subscription_id")
			if err != nil {
				return nil, err
			}
			queued, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"queued": queued}, nil
		}),
	}
}
