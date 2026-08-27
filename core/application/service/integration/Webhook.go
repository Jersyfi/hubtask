// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package integration holds the use cases that let another system reach into this one, and this
// one reach out: today the webhook subscriptions (G-03), later the trigger polling and the jumble's
// inbound routes.
//
// The calendar feeds are deliberately not here. They live beside the saved views, because minting
// one is a read of a view and the rule that decides who may see which view is there - a second
// copy of a visibility rule is how two answers to one security question get into a codebase.
package integration

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	CreateWebhookSubscriptionName = "CreateWebhookSubscription"
	GetWebhookSubscriptionName    = "GetWebhookSubscription"
	ListWebhookSubscriptionsName  = "ListWebhookSubscriptions"
	UpdateWebhookSubscriptionName = "UpdateWebhookSubscription"
	DeleteWebhookSubscriptionName = "DeleteWebhookSubscription"

	// automationScope is the token scope every webhook operation needs. The same one the rule
	// engine will use: subscribing an external system to the event stream and writing a rule that
	// reacts to it are the same power over the same stream.
	automationScope = "automation:manage"

	webhookTarget = "webhook_subscription"

	// The audit codes. A subscription is a standing instruction to send this workspace's events
	// to an address outside it, so every change to one is an act a review looks for (audit.md §2).
	WebhookCreatedAction  audit.Action = "webhooks.subscription_created"
	WebhookUpdatedAction  audit.Action = "webhooks.subscription_updated"
	WebhookDeletedAction  audit.Action = "webhooks.subscription_deleted"
	WebhookRotatedAction  audit.Action = "webhooks.secret_rotated"
	WebhookDisabledAction audit.Action = "webhooks.subscription_disabled"
	// WebhookReadAction is what a listing performs.
	WebhookReadAction audit.Action = "webhooks.subscription_read"

	// SecretBytes is the entropy of a signing secret. The same 32 bytes a personal access token
	// draws: this is an HMAC key, and anything shorter would be the weak half of SHA-256.
	SecretBytes = 32
)

// SecretPurpose binds a sealed secret to the subscription it belongs to, so that a ciphertext
// lifted out of one row and dropped into another no longer opens (core/port/crypto).
func SecretPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose(domain.SecretPurposeFor(id))
}

// Authorizer is the application's own decision point. Declared here rather than imported so that
// what this package needs of it is visible in one place (ADR-0005).
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Writer is what the webhook use cases share.
//
// One dependency set rather than six, for the reason the calendar feeds have one: the rule that
// decides who may touch a subscription is a single rule, and a second copy of it is how two
// answers to one security question get into a codebase.
type Writer struct {
	Subscriptions repository.WebhookSubscriptions
	Deliveries    repository.WebhookDeliveries
	Authorizer    Authorizer
	// Encryptor seals the signing secret. The application never stores a plaintext and the
	// repository never holds a key: the sealing happens here, between the two (E-02).
	Encryptor  crypto.Encryptor
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Entropy is where a signing secret comes from. A port, so production draws from crypto/rand
	// and a test can fix the secret it asserts on (rule 4).
	Entropy clock.Entropy
}

// MintedSubscription is what a creation or a rotation answers: the subscription as it will be
// listed, and the secret that will never be readable again.
//
// The plaintext travels in secret.Secret rather than as a string, so that a struct printed whole -
// the shape that actually leaks - masks it (T-18, rule 10).
type MintedSubscription struct {
	Subscription domain.WebhookSubscription
	Secret       secret.Secret
}

// CreateWebhookSubscription subscribes an external system to the event stream (G-03).
type CreateWebhookSubscription struct{ Writer Writer }

// CreateWebhookSubscriptionCommand is the input, typed.
type CreateWebhookSubscriptionCommand struct {
	TargetURL  string
	EventTypes []string
	Filter     string
}

// Execute creates the subscription and answers its signing secret, once.
//
// Whether the target may be dialled at all is not asked here, and that is deliberate: the guard
// that would refuse a private range is an adapter's, because only the adapter knows what the
// operator released (T-07). What this asks is whether the caller may subscribe anything.
func (h CreateWebhookSubscription) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateWebhookSubscriptionCommand,
) (MintedSubscription, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, WebhookCreatedAction, shared.ID("")); err != nil {
		return MintedSubscription{}, err
	}
	if actor.AccountID.IsZero() {
		// A subscription records who created it, and the trail is unreadable without that.
		return MintedSubscription{}, shared.ErrForbidden.WithDetail("webhooks.author_required")
	}

	material, err := w.Entropy.Bytes(SecretBytes)
	if err != nil {
		return MintedSubscription{}, shared.ErrInternal.
			WithDetail("webhooks.secret_undrawable").
			WithCause(err)
	}
	signing := secret.New(encodeSecret(material))

	var created domain.WebhookSubscription
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		subscription, err := domain.NewWebhookSubscription(domain.NewWebhookSubscriptionInput{
			ID: w.IDs.NewID(), TenantID: actor.TenantID,
			TargetURL: cmd.TargetURL, EventTypes: cmd.EventTypes, Filter: cmd.Filter,
			CreatedBy: actor.AccountID, Now: now,
		})
		if err != nil {
			return err
		}

		sealed, err := w.Encryptor.Seal(ctx, signing, SecretPurpose(subscription.ID))
		if err != nil {
			return err
		}
		if err := w.Subscriptions.Insert(ctx, repository.StoredSubscription{
			Subscription: subscription,
			Secret:       repository.SealedSecret{Ciphertext: sealed.Ciphertext, KeyID: sealed.KeyID},
		}); err != nil {
			return err
		}

		created = subscription
		return w.recordAudit(ctx, actor, subscription, WebhookCreatedAction, now, nil)
	})
	if err != nil {
		return MintedSubscription{}, err
	}
	return MintedSubscription{Subscription: created, Secret: signing}, nil
}

// GetWebhookSubscription answers one subscription.
type GetWebhookSubscription struct{ Writer Writer }

func (h GetWebhookSubscription) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.WebhookSubscription, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, WebhookReadAction, id); err != nil {
		return domain.WebhookSubscription{}, err
	}

	var found domain.WebhookSubscription
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := w.Subscriptions.Find(ctx, id)
		found = stored.Subscription
		return err
	})
	if err != nil {
		return domain.WebhookSubscription{}, err
	}
	return found, nil
}

// ListWebhookSubscriptions answers the workspace's subscriptions.
type ListWebhookSubscriptions struct{ Writer Writer }

func (h ListWebhookSubscriptions) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.WebhookSubscription, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, WebhookReadAction, shared.ID("")); err != nil {
		return nil, err
	}

	var subscriptions []domain.WebhookSubscription
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Subscriptions.List(ctx)
		subscriptions = found
		return err
	})
	if err != nil {
		return nil, err
	}
	return subscriptions, nil
}

// UpdateWebhookSubscriptionCommand is the input. Every field is a pointer, so that an omitted one
// is left alone and an explicit change is told apart from an absent field.
type UpdateWebhookSubscriptionCommand struct {
	ID              shared.ID
	TargetURL       *string
	EventTypes      *[]string
	Filter          *string
	State           *string
	ExpectedVersion int
}

// UpdateWebhookSubscription changes a subscription.
type UpdateWebhookSubscription struct{ Writer Writer }

// Execute applies the change.
//
// Re-enabling a subscription that unreachability disabled goes through here like any other change,
// and is audited like one: somebody decided the target is reachable again, and the trail says who.
// What cannot be set by hand is DISABLED - that is the system's conclusion from a run of failures,
// and a state somebody can type is a state that says nothing about why.
func (h UpdateWebhookSubscription) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateWebhookSubscriptionCommand,
) (domain.WebhookSubscription, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, WebhookUpdatedAction, cmd.ID); err != nil {
		return domain.WebhookSubscription{}, err
	}

	var updated domain.WebhookSubscription
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := w.Subscriptions.Find(ctx, cmd.ID)
		if err != nil {
			return err
		}

		before := stored.Subscription
		after, err := applyChange(before, cmd)
		if err != nil {
			return err
		}

		expected := before.Version
		if cmd.ExpectedVersion > 0 {
			expected = cmd.ExpectedVersion
		}
		changed, err := w.Subscriptions.Update(ctx, after, expected)
		if err != nil {
			return err
		}
		if !changed {
			return shared.ErrConflict.
				WithDetail("webhooks.subscription_version_conflict").
				WithParams(map[string]string{"webhook_id": cmd.ID.String()})
		}

		after.Version = expected + 1
		updated = after
		return w.recordAudit(ctx, actor, after, WebhookUpdatedAction, w.Clock.Now(),
			changesBetween(before, after))
	})
	if err != nil {
		return domain.WebhookSubscription{}, err
	}
	return updated, nil
}

// applyChange is the whole of what an update may do, in one place so that the two callers - the
// use case and its test - cannot disagree about it.
func applyChange(
	before domain.WebhookSubscription, cmd UpdateWebhookSubscriptionCommand,
) (domain.WebhookSubscription, error) {
	after := before

	if cmd.TargetURL != nil {
		target, err := domain.TargetURL(*cmd.TargetURL)
		if err != nil {
			return domain.WebhookSubscription{}, err
		}
		after.TargetURL = target
	}
	if cmd.EventTypes != nil {
		types, err := domain.SubscribedTypes(*cmd.EventTypes)
		if err != nil {
			return domain.WebhookSubscription{}, err
		}
		after.EventTypes = types
	}
	if cmd.Filter != nil {
		if err := domain.RefuseFilter(*cmd.Filter); err != nil {
			return domain.WebhookSubscription{}, err
		}
		after.Filter = ""
	}
	if cmd.State == nil {
		return after, nil
	}

	switch domain.SubscriptionState(*cmd.State) {
	case domain.SubscriptionActive:
		after = after.Enabled()
	case domain.SubscriptionPaused:
		after = after.Paused()
	default:
		// DISABLED is the system's conclusion from a run of failures, not a value to type. A
		// subscription somebody wants stopped is paused, and the two say different things to
		// whoever reads the list.
		return domain.WebhookSubscription{}, shared.ErrValidation.
			WithDetail("webhooks.state_not_settable").
			WithParams(map[string]string{"state": *cmd.State}).
			WithFields(shared.FieldError{Path: "/state", Code: "webhooks.state_not_settable"})
	}
	return after, nil
}

// DeleteWebhookSubscription unsubscribes.
type DeleteWebhookSubscription struct{ Writer Writer }

// Execute removes the subscription and, by cascade, its deliveries: a delivery log for a
// subscription nobody can reach any more is a record of attempts against an address the workspace
// no longer knows.
func (h DeleteWebhookSubscription) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) error {
	w := h.Writer
	if err := w.authorize(ctx, actor, WebhookDeletedAction, id); err != nil {
		return err
	}

	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := w.Subscriptions.Find(ctx, id)
		if err != nil {
			return err
		}

		removed, err := w.Subscriptions.Delete(ctx, id)
		if err != nil {
			return err
		}
		if !removed {
			// Somebody else deleted it between the read and the write. Not an error: the caller
			// asked for it to be gone and it is gone.
			return nil
		}
		return w.recordAudit(ctx, actor, stored.Subscription, WebhookDeletedAction, w.Clock.Now(), nil)
	})
}

// authorize is the one permission question every webhook operation asks.
//
// AUTOMATION rather than MANAGE_MEMBERS: subscribing an external system to the event stream and
// writing a rule that reacts to it are the same power over the same stream, and an installation
// that separated them would be saying that one of the two is safe.
func (w Writer) authorize(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, target shared.ID,
) error {
	return w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionAutomation,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     action,
		TokenScope: automationScope,
		TargetType: webhookTarget,
		TargetID:   target,
	})
}

// recordAudit writes the evidence. The target address is in it, and the secret never is: where a
// workspace's events are being sent is exactly what an auditor is looking for, and the key that
// signs them is exactly what they are not (rule 10).
func (w Writer) recordAudit(
	ctx context.Context, actor appshared.ActorContext, subscription domain.WebhookSubscription,
	action audit.Action, at time.Time, extra []audit.Change,
) error {
	changes := append([]audit.Change{
		{Field: "target_url", Classification: audit.Open, To: subscription.TargetURL},
		{Field: "event_types", Classification: audit.Open, To: joinTypes(subscription.EventTypes)},
		{Field: "state", Classification: audit.Open, To: string(subscription.State)},
	}, extra...)

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   subscription.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		// Notice rather than info, on InviteAccount's reasoning: this workspace's events now go
		// somewhere outside it, or stop going, or go somewhere else. That is the class of event a
		// review looks for.
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: webhookTarget,
		TargetID:   subscription.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// changesBetween records what an update actually moved, so that the trail says "the address
// changed" rather than only "somebody edited this".
func changesBetween(before, after domain.WebhookSubscription) []audit.Change {
	var changes []audit.Change
	if before.TargetURL != after.TargetURL {
		changes = append(changes, audit.Change{
			Field: "target_url_before", Classification: audit.Open, To: before.TargetURL,
		})
	}
	if before.State != after.State {
		changes = append(changes, audit.Change{
			Field: "state_before", Classification: audit.Open, To: string(before.State),
		})
	}
	return changes
}

// subscriptionOutput is the projection every channel gets. The secret is not in it: it exists in
// the answer to the creation and to a rotation, and nowhere else.
func subscriptionOutput(subscription domain.WebhookSubscription) usecase.Output {
	types := make([]any, 0, len(subscription.EventTypes))
	for _, wanted := range subscription.EventTypes {
		types = append(types, wanted.String())
	}

	out := usecase.Output{
		"id":            subscription.ID.String(),
		"target_url":    subscription.TargetURL,
		"event_types":   types,
		"state":         string(subscription.State),
		"failure_count": subscription.FailureCount,
		"created_at":    subscription.CreatedAt.UTC(),
		"version":       subscription.Version,
		"filter":        nil,
		"last_error":    nil,
	}
	if subscription.Filter != "" {
		out["filter"] = subscription.Filter
	}
	if subscription.LastError != "" {
		out["last_error"] = subscription.LastError
	}
	return out
}

func joinTypes(types []event.Type) string {
	names := make([]string, 0, len(types))
	for _, wanted := range types {
		names = append(names, wanted.String())
	}
	return strings.Join(names, " ")
}

// encodeSecret renders the drawn bytes as the string a subscriber configures.
//
// base64url without padding, for the reason a personal access token uses it: the value is pasted
// into configuration files, environment variables and web forms, and a `+` or a `/` in one of
// those is a support ticket.
func encodeSecret(material []byte) string {
	return base64.RawURLEncoding.EncodeToString(material)
}

// Descriptor is the catalogue entry.
func (h CreateWebhookSubscription) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateWebhookSubscriptionName,
		Summary: "Subscribes an external system to this workspace's event stream and answers " +
			"the signing secret, once. What arrives at the target is the CloudEvent, identical " +
			"to the one used internally. The target is an egress channel and is treated as one: " +
			"a private range or the cloud metadata address is refused unless the installation " +
			"has deliberately released private networks.",
		SideEffects: "Writes the subscription and an audit entry, and answers a secret.",
		TokenScope:  automationScope,
		Input: []usecase.Field{
			{
				Name: "target_url", Kind: usecase.KindString, Required: true,
				Description: "Where the CloudEvent is POSTed. http or https, and without " +
					"credentials in it - the signature is how a subscriber knows the call is ours.",
			},
			{
				Name: "event_types", Kind: usecase.KindList, Required: true,
				Description: "The types to receive, as /meta/capabilities lists them. One this " +
					"build does not emit is refused rather than stored.",
			},
			{
				Name: "filter", Kind: usecase.KindString,
				Description: "A CEL expression narrowing the subscription. Refused until the " +
					"expression engine lands, rather than accepted and ignored.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: WebhookCreatedAction, TargetType: webhookTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A subscription is about the workspace's whole stream rather than about one " +
				"entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateWebhookSubscription) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	types, err := in.StringList("event_types")
	if err != nil {
		return nil, err
	}

	minted, err := h.Execute(ctx, actor, CreateWebhookSubscriptionCommand{
		TargetURL: in.String("target_url"), EventTypes: types, Filter: in.String("filter"),
	})
	if err != nil {
		return nil, err
	}

	// The one place the signing secret is ever answered, beside a rotation. Every channel gets it
	// here and no channel can get it again - the projection every other call uses does not carry
	// it.
	out := subscriptionOutput(minted.Subscription)
	out["secret"] = minted.Secret.Reveal()
	return out, nil
}

// Descriptor is the catalogue entry.
func (h GetWebhookSubscription) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        GetWebhookSubscriptionName,
		Summary:     "One webhook subscription, with its state and its failure run. Not its secret.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "webhook_id", Kind: usecase.KindID, Required: true, Description: "Which subscription."},
		},
		Audit: usecase.AuditDeclaration{
			Action: WebhookReadAction, TargetType: webhookTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetWebhookSubscription) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("webhook_id")
	if err != nil {
		return nil, err
	}
	subscription, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return subscriptionOutput(subscription), nil
}

// Descriptor is the catalogue entry.
func (h ListWebhookSubscriptions) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListWebhookSubscriptionsName,
		Summary: "The workspace's webhook subscriptions, newest first, with each one's state " +
			"and how many consecutive failures it has seen. The secrets are not among the " +
			"fields: each is answered once, at creation and at each rotation.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: WebhookReadAction, TargetType: webhookTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListWebhookSubscriptions) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	subscriptions, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		rows = append(rows, subscriptionOutput(subscription))
	}
	return usecase.Output{"data": rows}, nil
}

// Descriptor is the catalogue entry.
func (h UpdateWebhookSubscription) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateWebhookSubscriptionName,
		Summary: "Changes a subscription's target, event types, filter or state. An omitted " +
			"field is left alone. DISABLED cannot be set by hand - it is what a run of failures " +
			"produces, and a subscription somebody wants stopped is paused instead. Re-enabling " +
			"one that unreachability disabled is a write like any other and is audited as one.",
		SideEffects: "Writes the subscription and an audit entry.",
		TokenScope:  automationScope,
		Input: []usecase.Field{
			{Name: "webhook_id", Kind: usecase.KindID, Required: true, Description: "Which subscription."},
			{Name: "target_url", Kind: usecase.KindString, Description: "The new address. Omitted leaves it."},
			{Name: "event_types", Kind: usecase.KindList, Description: "The new set of types. Omitted leaves them."},
			{Name: "filter", Kind: usecase.KindString, Description: "Refused until the expression engine lands."},
			{
				Name: "state", Kind: usecase.KindString, Enum: []string{"ACTIVE", "PAUSED"},
				Description: "ACTIVE resumes or re-enables; PAUSED stops deliveries deliberately.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read. A mismatch is a conflict rather than an overwrite.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: WebhookUpdatedAction, TargetType: webhookTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason creating one is exempt: a subscription is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateWebhookSubscription) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("webhook_id")
	if err != nil {
		return nil, err
	}

	cmd := UpdateWebhookSubscriptionCommand{ID: id, ExpectedVersion: in.Int("expected_version")}
	cmd.TargetURL = in.OptionalString("target_url")
	cmd.Filter = in.OptionalString("filter")
	cmd.State = in.OptionalString("state")
	if in.Present("event_types") {
		types, err := in.StringList("event_types")
		if err != nil {
			return nil, err
		}
		cmd.EventTypes = &types
	}

	subscription, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return subscriptionOutput(subscription), nil
}

// Descriptor is the catalogue entry.
func (h DeleteWebhookSubscription) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteWebhookSubscriptionName,
		Summary: "Unsubscribes. The deliveries go with the subscription: a delivery log for a " +
			"subscription nobody can reach any more is a record of attempts against an address " +
			"the workspace no longer knows.",
		SideEffects: "Removes the subscription and its deliveries, and writes an audit entry.",
		TokenScope:  automationScope,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "webhook_id", Kind: usecase.KindID, Required: true, Description: "Which subscription."},
		},
		Audit: usecase.AuditDeclaration{
			Action: WebhookDeletedAction, TargetType: webhookTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason creating one is exempt: a subscription is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteWebhookSubscription) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("webhook_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
