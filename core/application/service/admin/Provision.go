// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package admin holds the control plane's use cases: the lifecycle of a tenant, as
// multi-tenancy.md §5 draws it (H-06).
//
// Authorisation here is the scope alone, checked in this layer (rule 2): the role matrix is a
// tenant's internal order, and the operator acting on the control plane is deliberately not a
// member of the tenants they administer. `admin:tenants` is carried only by a personal access
// token minted for exactly this - never by a session (0.6.0 decision 6, catalogue.SessionScopes).
package admin

import (
	"context"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	ProvisionTenantName = "ProvisionTenant"
	tenantTarget        = "tenant"
	adminTenantsScope   = "admin:tenants"

	// TenantProvisionedAction is the audit code, written into the new tenant's own trail: its
	// members are entitled to see how their workspace began (audit.md §6).
	TenantProvisionedAction audit.Action = "tenant.provisioned"

	// The instance journal's action vocabulary. Strings rather than audit.Actions: the journal
	// is not the trail, and its reader is the operator at the database.
	journalProvisioned = "tenant.provisioned"
)

// RedemptionTokens is the one-method slice provisioning needs of the sign-in accounts: storing
// the owner's way in.
type RedemptionTokens interface {
	SetRedemptionToken(
		ctx context.Context, accountID shared.ID, presented domain.Token,
		expiresAt, now time.Time,
	) (bool, error)
}

// The remaining collaborators, each as the one-method slice provisioning actually uses - a seeded
// row is inserted and never read back, and a fake of one method is a test that stays about
// provisioning.
type (
	AccountSeeder interface {
		Insert(ctx context.Context, account domain.Account) error
	}
	MembershipSeeder interface {
		Grant(ctx context.Context, grant domain.Grant) error
	}
	ContainerSeeder interface {
		Insert(ctx context.Context, container work.Container) error
	}
	BucketSeeder interface {
		Insert(ctx context.Context, bucket work.Bucket) error
	}
	LabelSeeder interface {
		Insert(ctx context.Context, label work.Label) error
	}
)

// ProvisionTenantCommand is the input, typed.
type ProvisionTenantCommand struct {
	Slug            string
	DisplayName     string
	DefaultLocale   string
	DefaultTimeZone string
	OwnerEmail      string
	// OwnerDisplayName may be empty; the address stands in until the person introduces
	// themselves.
	OwnerDisplayName string
}

// ProvisionedTenant is what the one answer carries - including the owner's redemption token,
// shown for the only time (T-18's "shown once").
type ProvisionedTenant struct {
	Tenant              domain.Tenant
	OwnerAccountID      shared.ID
	OwnerRedemption     secret.Secret
	DefaultHubID        shared.ID
	ExampleCollectionID shared.ID
}

// ProvisionTenant creates a workspace with its defaults, exactly the §5 table: the tenant row,
// the owner as an invited account with a redemption token, the OWNER membership, a default hub,
// an example collection, standard buckets and labels - one transaction, so a workspace either
// exists whole or not at all. Idempotent under `Idempotency-Key` through the middleware every
// POST already has.
type ProvisionTenant struct {
	Tenants    adminrepo.Tenants
	Journal    adminrepo.Journal
	Accounts   AccountSeeder
	Redemption RedemptionTokens
	Grants     MembershipSeeder
	Containers ContainerSeeder
	Buckets    BucketSeeder
	Labels     LabelSeeder
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	// Renderer names the seeded structure in the workspace's own locale: the names are content
	// once created, so they are written in the language the workspace declared, not in a code
	// (i18n-l10n.md §2, rule 8 - the backend renders here because creation is the boundary).
	Renderer   i18n.Renderer
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
	Entropy    clock.Entropy
	// Tenancy is the installation's mode. Provisioning a second workspace only exists in multi
	// mode: single mode's whole contract is "exactly one tenant, no selection" (§1).
	Tenancy env.TenancyMode
}

// Execute provisions the workspace and answers the owner's way in.
func (h ProvisionTenant) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ProvisionTenantCommand,
) (ProvisionedTenant, error) {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return ProvisionedTenant{}, err
	}
	if h.Tenancy != env.TenancyMulti {
		return ProvisionedTenant{}, shared.ErrForbidden.WithDetail("admin.multi_mode_required")
	}

	now := h.Clock.Now()
	tenant, err := domain.NewTenant(domain.NewTenantInput{
		ID: h.IDs.NewID(), Slug: cmd.Slug, DisplayName: cmd.DisplayName,
		DefaultLocale: cmd.DefaultLocale, DefaultTimeZone: cmd.DefaultTimeZone, Now: now,
	})
	if err != nil {
		return ProvisionedTenant{}, err
	}

	ownerName := cmd.OwnerDisplayName
	if ownerName == "" {
		ownerName = cmd.OwnerEmail
	}
	owner, err := domain.Invite(h.IDs.NewID(), tenant.ID, cmd.OwnerEmail, ownerName)
	if err != nil {
		return ProvisionedTenant{}, err
	}

	material, err := h.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return ProvisionedTenant{}, shared.ErrInternal.
			WithDetail("auth.session_unmintable").WithCause(err)
	}
	redemption, err := domain.NewRedemptionToken(tenant.ID, material)
	if err != nil {
		return ProvisionedTenant{}, err
	}

	result := ProvisionedTenant{
		Tenant: tenant, OwnerAccountID: owner.ID,
		OwnerRedemption: secret.New(redemption.Secret()),
	}

	// The new tenant's own scope: the first write it ever sees is already bounded to it, and
	// the tenant_self policy's WITH CHECK is what lets the row land (migration 0067).
	scope := persistence.Scope{TenantID: tenant.ID, ActorID: actor.AccountID}
	err = h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := h.Tenants.Insert(ctx, adminrepo.TenantRecord{
			ID: tenant.ID, Slug: tenant.Slug, DisplayName: tenant.DisplayName,
			Status: tenant.Status, DefaultLocale: tenant.DefaultLocale,
			DefaultTimeZone: tenant.DefaultTimeZone, CreatedAt: tenant.CreatedAt,
		}); err != nil {
			return err
		}

		if err := h.Accounts.Insert(ctx, owner); err != nil {
			return err
		}
		if _, err := h.Redemption.SetRedemptionToken(
			ctx, owner.ID, redemption, now.Add(domain.RedemptionLifetime).UTC(), now,
		); err != nil {
			return err
		}

		grant, err := domain.NewGrant(
			h.IDs.NewID(), tenant.ID, owner.ID, shared.ID(""), domain.TenantScope(), domain.RoleOwner)
		if err != nil {
			return err
		}
		if err := h.Grants.Grant(ctx, grant); err != nil {
			return err
		}

		hubID, collectionID, err := h.seedStructure(ctx, tenant, owner, actor, now)
		if err != nil {
			return err
		}
		result.DefaultHubID = hubID
		result.ExampleCollectionID = collectionID

		if err := h.recordAudit(ctx, tenant, owner, actor, now); err != nil {
			return err
		}
		return h.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: h.IDs.NewID(), OccurredAt: now, Action: journalProvisioned,
			TenantID: tenant.ID, TenantSlug: tenant.Slug, ActorLabel: actor.AccountName,
			Details: map[string]any{
				"owner_account_id": owner.ID.String(),
				"default_hub_id":   hubID.String(),
			},
		})
	})
	if err != nil {
		return ProvisionedTenant{}, err
	}
	return result, nil
}

// seedStructure writes the §5 defaults: a hub, a collection inside it, three buckets, four
// labels - each with the event and the change entry any creation owes, so a synchronising
// client's first pull sees the same workspace the API answers (offline-sync.md §3.1).
func (h ProvisionTenant) seedStructure(
	ctx context.Context, tenant domain.Tenant, owner domain.Account,
	actor appshared.ActorContext, now time.Time,
) (shared.ID, shared.ID, error) {
	eventActor := event.Actor{Kind: actor.Kind, ID: actor.AccountID}
	name := func(code string) string {
		return h.Renderer.Render(tenant.DefaultLocale, code, nil)
	}

	hub, err := h.seedContainer(ctx, work.NewContainerInput{
		ID: h.IDs.NewID(), TenantID: tenant.ID, Type: work.ContainerHub,
		Name: name("seed.hub.name"), OrderKey: firstOrderKey(),
		CreatedBy: owner.ID, Now: now,
	}, eventActor, now)
	if err != nil {
		return "", "", err
	}

	collection, err := h.seedContainer(ctx, work.NewContainerInput{
		ID: h.IDs.NewID(), TenantID: tenant.ID, Type: work.ContainerCollection,
		ParentID: hub.ID, Name: name("seed.collection.name"), OrderKey: firstOrderKey(),
		CreatedBy: owner.ID, Now: now,
	}, eventActor, now)
	if err != nil {
		return "", "", err
	}

	orderKey := ""
	for _, bucket := range []struct {
		code  string
		done  bool
		color string
	}{
		{"seed.bucket.todo", false, "slate"},
		{"seed.bucket.doing", false, "blue"},
		{"seed.bucket.done", true, "green"},
	} {
		next, err := service.OrderKeyAfter(orderKey)
		if err != nil {
			return "", "", err
		}
		orderKey = next
		if err := h.seedBucket(ctx, work.NewBucketInput{
			ID: h.IDs.NewID(), TenantID: tenant.ID, CollectionID: collection.ID,
			Name: name(bucket.code), OrderKey: orderKey,
			IsDoneBucket: bucket.done, ColorToken: bucket.color,
		}, eventActor, now); err != nil {
			return "", "", err
		}
	}

	for _, label := range []struct{ code, color string }{
		{"seed.label.important", "red"},
		{"seed.label.waiting", "amber"},
		{"seed.label.quick_win", "green"},
		{"seed.label.idea", "violet"},
	} {
		if err := h.seedLabel(ctx, work.NewLabelInput{
			ID: h.IDs.NewID(), TenantID: tenant.ID, CollectionID: collection.ID,
			Name: name(label.code), ColorToken: label.color,
		}, eventActor, now); err != nil {
			return "", "", err
		}
	}

	return hub.ID, collection.ID, nil
}

func (h ProvisionTenant) seedContainer(
	ctx context.Context, in work.NewContainerInput, actor event.Actor, now time.Time,
) (work.Container, error) {
	container, err := work.NewContainer(in)
	if err != nil {
		return work.Container{}, err
	}
	if err := h.Containers.Insert(ctx, container); err != nil {
		return work.Container{}, err
	}
	announcement, err := event.NewContainerCreated(h.IDs.NewID(), container, actor, now, event.Cause{})
	if err != nil {
		return work.Container{}, err
	}
	if err := h.Events.Append(ctx, announcement); err != nil {
		return work.Container{}, err
	}
	visibility := container.ParentID
	if visibility.IsZero() {
		visibility = container.ID
	}
	return container, h.Changes.Record(ctx, changelog.Change{
		TenantID: container.TenantID, Entity: "container", EntityID: container.ID,
		Op: changelog.Upsert, ContainerID: visibility, ActorID: in.CreatedBy,
		HLC: h.HLC.Next(), Payload: announcement.Payload,
	})
}

func (h ProvisionTenant) seedBucket(
	ctx context.Context, in work.NewBucketInput, actor event.Actor, now time.Time,
) error {
	bucket, err := work.NewBucket(in)
	if err != nil {
		return err
	}
	if err := h.Buckets.Insert(ctx, bucket); err != nil {
		return err
	}
	announcement, err := event.NewBucketCreated(h.IDs.NewID(), bucket, actor, now, event.Cause{})
	if err != nil {
		return err
	}
	if err := h.Events.Append(ctx, announcement); err != nil {
		return err
	}
	return h.Changes.Record(ctx, changelog.Change{
		TenantID: bucket.TenantID, Entity: "bucket", EntityID: bucket.ID,
		Op: changelog.Upsert, ContainerID: bucket.CollectionID, ActorID: actor.ID,
		HLC: h.HLC.Next(), Payload: announcement.Payload,
	})
}

func (h ProvisionTenant) seedLabel(
	ctx context.Context, in work.NewLabelInput, actor event.Actor, now time.Time,
) error {
	label, err := work.NewLabel(in)
	if err != nil {
		return err
	}
	if err := h.Labels.Insert(ctx, label); err != nil {
		return err
	}
	announcement, err := event.NewLabelCreated(h.IDs.NewID(), label, actor, now, event.Cause{})
	if err != nil {
		return err
	}
	if err := h.Events.Append(ctx, announcement); err != nil {
		return err
	}
	return h.Changes.Record(ctx, changelog.Change{
		TenantID: label.TenantID, Entity: "label", EntityID: label.ID,
		Op: changelog.Upsert, ContainerID: label.CollectionID, ActorID: actor.ID,
		HLC: h.HLC.Next(), Payload: announcement.Payload,
	})
}

// recordAudit writes the provisioning into the new tenant's own trail: its members are entitled
// to see how their workspace began, and by whom (audit.md §6).
func (h ProvisionTenant) recordAudit(
	ctx context.Context, tenant domain.Tenant, owner domain.Account,
	actor appshared.ActorContext, now time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID: tenant.ID, OccurredAt: now, Action: TenantProvisionedAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: tenantTarget, TargetID: tenant.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "slug", Classification: audit.Open, To: tenant.Slug},
			audit.Change{Field: "display_name", Classification: audit.Sensitive, To: tenant.DisplayName},
			audit.Change{Field: "owner_account_id", Classification: audit.Open, To: owner.ID.String()},
		),
	})
}

func firstOrderKey() string {
	key, err := service.OrderKeyAfter("")
	if err != nil {
		// OrderKeyAfter("") cannot fail; the signature says error because later keys can.
		return "m"
	}
	return key
}

// Descriptor is the catalogue entry (usecase.Descriptor's contract).
func (h ProvisionTenant) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ProvisionTenantName,
		Summary: "Creates a workspace with its defaults: the tenant, an invited owner whose " +
			"redemption token is answered once, a default hub, an example collection, standard " +
			"buckets and labels. Multi mode only, and idempotent under an Idempotency-Key.",
		SideEffects: "Writes the tenant and its seeded structure, announces the structure's " +
			"events, records the provisioning in the new tenant's audit trail and in the " +
			"instance journal.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{
				Name: "slug", Kind: usecase.KindString, Required: true,
				Description: "The subdomain the workspace answers on: 3-40 characters of " +
					"lower-case letters, digits and hyphens, unique across the installation.",
			},
			{Name: "display_name", Kind: usecase.KindString, Required: true},
			{
				Name: "default_locale", Kind: usecase.KindString,
				Description: "BCP 47; the installation's default when omitted.",
			},
			{
				Name: "default_time_zone", Kind: usecase.KindString,
				Description: "An IANA name; UTC when omitted.",
			},
			{Name: "owner_email", Kind: usecase.KindString, Required: true},
			{Name: "owner_display_name", Kind: usecase.KindString},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantProvisionedAction, TargetType: tenantTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items; the history is an " +
				"item's (domain-model.md §3.5). The evidence lives in the audit trail and the " +
				"instance journal instead.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ProvisionTenant) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	provisioned, err := h.Execute(ctx, actor, ProvisionTenantCommand{
		Slug:             in.String("slug"),
		DisplayName:      in.String("display_name"),
		DefaultLocale:    in.String("default_locale"),
		DefaultTimeZone:  in.String("default_time_zone"),
		OwnerEmail:       in.String("owner_email"),
		OwnerDisplayName: in.String("owner_display_name"),
	})
	if err != nil {
		return nil, err
	}

	out := adminTenantOutput(adminrepo.TenantRecord{
		ID: provisioned.Tenant.ID, Slug: provisioned.Tenant.Slug,
		DisplayName: provisioned.Tenant.DisplayName, Status: provisioned.Tenant.Status,
		DefaultLocale:   provisioned.Tenant.DefaultLocale,
		DefaultTimeZone: provisioned.Tenant.DefaultTimeZone,
		CreatedAt:       provisioned.Tenant.CreatedAt,
	})
	out["owner_account_id"] = provisioned.OwnerAccountID.String()
	// The one appearance of the owner's way in (T-18's "shown once").
	out["owner_redemption_token"] = provisioned.OwnerRedemption.Reveal()
	out["default_hub_id"] = provisioned.DefaultHubID.String()
	out["example_collection_id"] = provisioned.ExampleCollectionID.String()
	return out, nil
}

// adminTenantOutput is the AdminTenant shape of the contract, shared by the listing and the
// provisioning answer.
func adminTenantOutput(record adminrepo.TenantRecord) usecase.Output {
	out := usecase.Output{
		"id":                record.ID.String(),
		"slug":              record.Slug,
		"display_name":      record.DisplayName,
		"status":            string(record.Status),
		"default_locale":    record.DefaultLocale,
		"default_time_zone": record.DefaultTimeZone,
		"created_at":        record.CreatedAt,
		"purge_after":       nil,
	}
	if !record.PurgeAfter.IsZero() {
		out["purge_after"] = record.PurgeAfter
	}
	return out
}
