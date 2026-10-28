// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// An operator changes a workspace's address (multi-tenancy.md §3.1, ADR-0095 §3).
//
// **One transaction** replaces the main row and drops the old one, so the old host answers like an
// address no workspace has at once and another workspace may take it (D-2). Everybody signs in again;
// passkeys stay stored and count as no factor at the new host (identity.md §23.3); nothing else of
// the workspace changes.
//
// **Nobody is locked out** (P-16): refused while the person making it, or every owner, would be left
// without a way in that survives it, and confirmed only through such a way - a password or a fresh
// provider sign-in, never a passkey, whose relying party is the address that ends.

// TenantAddressChangedAction is the workspace's trail entry. WARNING: it ends every session.
const TenantAddressChangedAction audit.Action = "tenant.address_changed"

const (
	ChangeTenantAddressName = "ChangeTenantAddress"
	journalAddressChanged   = "tenant.address_changed"

	// stepUpPurposeAddress is what the demand names beside its methods, so a prompt can say why a
	// passkey is not offered.
	stepUpPurposeAddress = "change_address"
)

// AddressNotices tells them, inside the change's transaction.
type AddressNotices interface {
	AddressChanged(
		ctx context.Context, tenantID shared.ID, from, to string, at time.Time,
		mailed, unmailed []shared.ID,
	) error
}

// LastingWays answers what one person would still sign in with after a change of address. It runs
// its own reads in the person's workspace, so it is asked before the change's transaction opens.
type LastingWays interface {
	WaysOf(ctx context.Context, tenantID, accountID shared.ID) (domain.WaysHeld, error)
}

// ChangeTenantAddress is the use case.
type ChangeTenantAddress struct {
	Instance   InstanceWriter
	Tenants    adminrepo.Tenants
	Addresses  adminrepo.TenantAddresses
	Audience   adminrepo.AddressAudience
	Notices    AddressNotices
	Owners     identityrepo.Owners
	Ways       LastingWays
	Journal    adminrepo.Journal
	Audit      audit.Sink
	StepUp     stepup.Verifier
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Entropy    clock.Entropy
	Domains    text.DomainEncoder
	Tenancy    env.TenancyMode
	// WorkspaceHosts says whether the new address is a short name or a host (ADR-0095 §6).
	WorkspaceHosts env.WorkspaceHosts
	// InstallationHost is the installation's own host, from the configured base URL.
	InstallationHost string
	// Settings is the installation's level, whose reserved short names a new name may not be.
	Settings identityrepo.InstanceSettings
}

// ChangeTenantAddressCommand is the input, typed.
type ChangeTenantAddressCommand struct {
	TenantID shared.ID
	// Slug is the new short name with subdomains, Host the new host with assigned hosts.
	Slug, Host string
	DryRun     bool
	// StepUpToken is the proof, by password or provider, consumed by this one request.
	StepUpToken string
}

// AddressChange is the answer: the workspace as it is (or would be), the two addresses and who is told.
type AddressChange struct {
	Tenant       adminrepo.TenantRecord
	From, To     string
	Mailed       int
	ToldAtSignIn int
	DryRun       bool
}

// Execute changes the address, or with DryRun answers what the change would be.
func (h ChangeTenantAddress) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeTenantAddressCommand,
) (AddressChange, error) {
	if err := h.Instance.authorize(ctx, actor); err != nil {
		return AddressChange{}, err
	}
	if h.Tenancy != env.TenancyMulti {
		return AddressChange{}, shared.ErrForbidden.WithDetail("admin.multi_mode_required")
	}
	if cmd.TenantID.IsZero() {
		return AddressChange{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}

	var (
		record           adminrepo.TenantRecord
		mailed, unmailed []shared.ID
	)
	scope := persistence.Scope{TenantID: cmd.TenantID, ActorID: actor.AccountID}
	err := h.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		found, err := h.find(ctx)
		if err != nil {
			return err
		}
		record = found
		mailed, unmailed, err = h.Audience.PeopleToTell(ctx)
		return err
	})
	if err != nil {
		return AddressChange{}, err
	}
	if record.Status == domain.TenantPendingDeletion {
		return AddressChange{}, shared.ErrConflict.WithDetail("admin.tenant_leaving")
	}
	from := record.Host
	if from == "" {
		from = domain.CanonicalHostOf(record.Slug, h.InstallationHost, true)
	}
	answer := AddressChange{
		Tenant: record, From: from, Mailed: len(mailed), ToldAtSignIn: len(unmailed), DryRun: cmd.DryRun,
	}
	if cmd.DryRun && cmd.Slug == "" && cmd.Host == "" {
		return answer, nil
	}

	level, err := installationLevel(ctx, h.UnitOfWork, h.Settings)
	if err != nil {
		return AddressChange{}, err
	}
	next, err := domain.NewTenantAddress(domain.NewTenantAddressInput{
		Assigned: h.WorkspaceHosts == env.HostsAssigned, RawHost: cmd.Host, RawSlug: cmd.Slug,
		InstallationHost: h.InstallationHost, Domains: h.Domains,
		Current: domain.TenantAddress{Slug: record.Slug, Host: from}, Reserved: level.ReservedShortNames,
	})
	if err != nil {
		return AddressChange{}, err
	}
	answer.To = next.Host
	answer.Tenant.Slug, answer.Tenant.Host = next.Slug, next.Host

	// The lockout first, then the proof: a refused change must not burn a step-up.
	if err := h.refuseLockout(ctx, actor, cmd.TenantID); err != nil {
		return AddressChange{}, err
	}
	if cmd.DryRun {
		return answer, nil
	}
	// Outside the workspace's transaction: the proof lives in the operator's own workspace.
	if err := stepup.DemandOneOf(ctx, h.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken,
		stepUpPurposeAddress, stepup.MethodPassword, stepup.MethodProvider); err != nil {
		return AddressChange{}, err
	}

	material, err := h.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return AddressChange{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	now := h.Clock.Now()
	row, err := domain.NewCanonicalHost(domain.NewCanonicalHostInput{
		TenantID: cmd.TenantID, Host: next.Host,
		Verification: domain.HostVerificationPrefix + base32Mark(material), Now: now,
	})
	if err != nil {
		return AddressChange{}, err
	}

	err = h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		current, err := h.find(ctx)
		if err != nil {
			return err
		}
		if current.Status == domain.TenantPendingDeletion {
			return shared.ErrConflict.WithDetail("admin.tenant_leaving")
		}
		replaced, err := h.Addresses.ChangeAddress(ctx, next.Slug, row, now)
		if err != nil {
			return err
		}
		if replaced != "" {
			answer.From = replaced
		}
		if _, err := h.Addresses.EndSessions(ctx, now); err != nil {
			return err
		}
		if err := h.record(ctx, actor, current, next, answer.From, now); err != nil {
			return err
		}
		if h.Notices != nil {
			if err := h.Notices.AddressChanged(ctx, cmd.TenantID, answer.From, next.Host, now,
				mailed, unmailed); err != nil {
				return err
			}
		}
		answer.Tenant, err = h.find(ctx)
		return err
	})
	if err != nil {
		return AddressChange{}, err
	}
	return answer, nil
}

// refuseLockout refuses a change that leaves the person making it - where it is their own
// workspace - or every owner without a lasting way in (multi-tenancy.md §3.1).
func (h ChangeTenantAddress) refuseLockout(
	ctx context.Context, actor appshared.ActorContext, tenantID shared.ID,
) error {
	if actor.TenantID == tenantID && !actor.AccountID.IsZero() {
		held, err := h.Ways.WaysOf(ctx, tenantID, actor.AccountID)
		if err != nil {
			return err
		}
		if lacking := held.Lacking(); lacking != "" {
			return domain.RefuseLockout(true, lacking)
		}
	}
	var owners []shared.ID
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			paths, err := h.Owners.OwnerPaths(ctx)
			for _, path := range paths {
				owners = appendOnce(owners, path.AccountID)
			}
			return err
		})
	if err != nil {
		return err
	}
	// A workspace with no owner left loses nobody to the change.
	if len(owners) == 0 {
		return nil
	}
	lacking := domain.LackingWayIn
	for _, owner := range owners {
		held, err := h.Ways.WaysOf(ctx, tenantID, owner)
		if err != nil {
			return err
		}
		switch held.Lacking() {
		case "":
			return nil
		case domain.LackingFactor:
			lacking = domain.LackingFactor
		}
	}
	return domain.RefuseLockout(false, lacking)
}

func appendOnce(ids []shared.ID, id shared.ID) []shared.ID {
	for _, held := range ids {
		if held == id {
			return ids
		}
	}
	return append(ids, id)
}

// record writes the trail and the journal. Hosts only: an address is the installation's, not content.
func (h ChangeTenantAddress) record(
	ctx context.Context, actor appshared.ActorContext, current adminrepo.TenantRecord,
	next domain.TenantAddress, from string, now time.Time,
) error {
	changes := []audit.Change{
		{Field: "host", Classification: audit.Open, From: from, To: next.Host},
	}
	if next.Slug != current.Slug {
		changes = append(changes,
			audit.Change{Field: "slug", Classification: audit.Open, From: current.Slug, To: next.Slug})
	}
	if err := h.Audit.Append(ctx, audit.Entry{
		TenantID: current.ID, OccurredAt: now, Action: TenantAddressChangedAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: tenantTarget, TargetID: current.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	}); err != nil {
		return err
	}
	return h.Journal.Record(ctx, adminrepo.InstanceEvent{
		ID: h.IDs.NewID(), OccurredAt: now, Action: journalAddressChanged,
		TenantID: current.ID, TenantSlug: next.Slug, ActorLabel: actor.AccountName,
		Details: map[string]any{"from": from, "to": next.Host},
	})
}

func (h ChangeTenantAddress) find(ctx context.Context) (adminrepo.TenantRecord, error) {
	record, err := h.Tenants.Find(ctx)
	if errors.Is(err, shared.ErrNotFound) {
		return adminrepo.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	return record, err
}

// Descriptor is the catalogue entry.
func (h ChangeTenantAddress) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ChangeTenantAddressName,
		Summary: "Changes a workspace's address: a new short name where workspaces answer beneath the " +
			"installation's host, a new host where the installation assigns hosts. The old address " +
			"answers nothing at once and is free; everybody signs in again and passkeys made at the " +
			"old address stop working. Refused while it would lock out the caller or every owner. " +
			"Confirmed by a password or provider step-up, never a passkey. With dry_run it checks and " +
			"answers what the change would be.",
		SideEffects: "Replaces the workspace's main host and short name, ends every person's session, " +
			"mails every member with an address and tells the others at their next sign-in; records " +
			"it in the workspace's trail and the instance journal.",
		TokenScope: adminTenantsScope,
		StepUp:     "always, by password or provider - a passkey belongs to the address that ends",
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
			{Name: "slug", Kind: usecase.KindString,
				Description: "The new short name, where workspaces answer beneath the installation's host."},
			{Name: "host", Kind: usecase.KindString,
				Description: "The new host, where the installation assigns hosts."},
			{Name: "dry_run", Kind: usecase.KindBool,
				Description: "Check and answer without changing anything, and without a step-up."},
			{Name: "step_up_token", Kind: usecase.KindString,
				Description: "The proof POST /auth/step-up answered, by password or provider."},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantAddressChangedAction, TargetType: tenantTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ChangeTenantAddress) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	change, err := h.Execute(ctx, actor, ChangeTenantAddressCommand{
		TenantID: tenantID, Slug: in.String("slug"), Host: in.String("host"),
		DryRun: in.Bool("dry_run"), StepUpToken: in.String("step_up_token"),
	})
	if err != nil {
		return nil, err
	}
	out := usecase.Output{
		"tenant": adminTenantOutput(change.Tenant), "from": change.From,
		"members_mailed": change.Mailed, "members_told_at_sign_in": change.ToldAtSignIn,
		"dry_run": change.DryRun,
	}
	if change.To != "" {
		out["to"] = change.To
	}
	return out, nil
}
