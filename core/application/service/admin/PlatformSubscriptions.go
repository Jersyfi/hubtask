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
