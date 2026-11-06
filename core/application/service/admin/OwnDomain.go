// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"slices"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// A workspace's own domain (ADR-0095 §4, UC-INS-15): its owner sets it up without the operator,
// where the installation allows it; a job proves it by DNS and re-proves it; the operator sees every
// one and may block one. Hosts only reach the trail and the journal - an address is the
// installation's, not content (rule 10); the operator's reason is the journal's alone.

const (
	SetUpOwnDomainName  = "SetUpOwnDomain"
	RemoveOwnDomainName = "RemoveOwnDomain"
	ListOwnDomainsName  = "ListOwnDomains"
	BlockOwnDomainName  = "BlockOwnDomain"

	// OwnDomainAddedAction and the others are the workspace's trail (UC-INS-15 check 11).
	OwnDomainAddedAction    audit.Action = "tenant.own_domain_added"
	OwnDomainRemovedAction  audit.Action = "tenant.own_domain_removed"
	OwnDomainVerifiedAction audit.Action = "tenant.own_domain_verified"
	OwnDomainBrokenAction   audit.Action = "tenant.own_domain_broken"
	OwnDomainBlockedAction  audit.Action = "tenant.own_domain_blocked"
	ownDomainsReadAction    audit.Action = "instance.own_domains_read"

	// journalOwnDomainBlocked is the instance journal's entry for the operator's block.
	journalOwnDomainBlocked = "tenant.own_domain_blocked"
)

// OwnDomains is what the own domain's use cases share.
type OwnDomains struct {
	Instance   InstanceWriter
	Domains    identityrepo.OwnDomains
	Doors      identityrepo.OwnDomainDoors
	Owners     identityrepo.Owners
	Settings   identityrepo.InstanceSettings
	Tenants    adminrepo.Tenants
	Journal    adminrepo.Journal
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Entropy    clock.Entropy
	Encoder    text.DomainEncoder
	Registry   text.DomainRegistry
	Tenancy    env.TenancyMode
	// InstallationHost is the installation's own host, which no own domain may be or lie beneath,
	// and whose `domains.` label every own domain's CNAME points at.
	InstallationHost string
}

// ownDomainMarkBytes is the entropy behind a mark: 80 bits, sixteen characters of base32.
const ownDomainMarkBytes = 10

// rule is what the installation decided: off in single mode and wherever it decided nothing.
func (d OwnDomains) rule(ctx context.Context) (domain.OwnDomainRule, error) {
	if d.Tenancy != env.TenancyMulti {
		return domain.OwnDomainRule{}, nil
	}
	level, err := installationLevel(ctx, d.UnitOfWork, d.Settings)
	if err != nil {
		return domain.OwnDomainRule{}, err
	}
	return domain.OwnDomainRule{Allowed: level.OwnersOwnDomains}, nil
}

// authorizeOwner is the owner's door: the role matrix's structure permission, then the owner's role
// itself - an administrator who is not an owner sees the panel and is refused.
func (d OwnDomains) authorizeOwner(
	ctx context.Context, authorizer WorkspaceAuthorizer, actor appshared.ActorContext, action audit.Action,
) error {
	if err := authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     action,
		TokenScope: workspaceManageScope,
		TargetType: tenantTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return err
	}
	var owner bool
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		paths, err := d.Owners.OwnerPaths(ctx)
		owner = slices.ContainsFunc(paths, func(path identityrepo.OwnerPath) bool { return path.AccountID == actor.AccountID })
		return err
	})
	if err != nil {
		return err
	}
	if !owner {
		return shared.ErrForbidden.WithDetail("access.owner_required")
	}
	return nil
}

func (d OwnDomains) trail(
	ctx context.Context, tenantID shared.ID, actor appshared.ActorContext, action audit.Action,
	severity audit.Severity, change audit.Change, now time.Time, more ...audit.Change,
) error {
	kind, id, label := actor.Kind, actor.AccountID, actor.AccountName
	if kind == "" {
		kind = shared.ActorSystem
	}
	return d.Audit.Append(ctx, audit.Entry{
		TenantID: tenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: severity,
		ActorKind: kind, ActorID: id, ActorLabel: label,
		TargetType: tenantTarget, TargetID: tenantID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(append([]audit.Change{change}, more...)...),
	})
}

// SetUpOwnDomain is the owner's set-up.
type SetUpOwnDomain struct {
	OwnDomains
	Authorizer WorkspaceAuthorizer
}

// Execute sets up the caller's workspace's own domain and answers it, waiting for its records.
func (h SetUpOwnDomain) Execute(ctx context.Context, actor appshared.ActorContext, raw string) (usecase.Output, error) {
	if err := h.authorizeOwner(ctx, h.Authorizer, actor, OwnDomainAddedAction); err != nil {
		return nil, err
	}
	rule, err := h.rule(ctx)
	if err != nil {
		return nil, err
	}
	if err := rule.Refuse(); err != nil {
		return nil, err
	}
	host, err := domain.OwnDomainOf(raw, h.InstallationHost, h.Encoder, h.Registry)
	if err != nil {
		return nil, err
	}
	material, err := h.Entropy.Bytes(ownDomainMarkBytes)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("admin.own_domain_unmintable").WithCause(err)
	}
	now := h.Clock.Now()
	row, err := domain.NewOwnDomain(domain.NewOwnDomainInput{
		TenantID: actor.TenantID, Host: host, Mark: base32Mark(material), Now: now,
	})
	if err != nil {
		return nil, err
	}
	var answer usecase.Output
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Domains.Find(ctx); !errors.Is(err, shared.ErrNotFound) {
			if err != nil {
				return err
			}
			return shared.ErrConflict.WithDetail("admin.own_domain_exists")
		}
		if _, err := h.Domains.BlockInView(ctx); !errors.Is(err, shared.ErrNotFound) {
			if err != nil {
				return err
			}
			return shared.ErrConflict.WithDetail("admin.own_domain_exists")
		}
		blocked, err := h.Domains.Blocked(ctx, host)
		if err != nil {
			return err
		}
		if blocked {
			return domain.OwnDomainBlocked(host)
		}
		if err := h.Domains.Insert(ctx, row); err != nil {
			return err
		}
		if err := h.trail(ctx, actor.TenantID, actor, OwnDomainAddedAction, audit.SeverityWarning,
			audit.Change{Field: "own_domain", Classification: audit.Open, To: host}, now); err != nil {
			return err
		}
		view, err := h.Domains.Find(ctx)
		answer = identityservice.OwnDomainOutput(&view, nil, h.InstallationHost)
		return err
	})
	return answer, err
}

// Descriptor is the catalogue entry.
func (h SetUpOwnDomain) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetUpOwnDomainName,
		Summary: "The workspace's owner sets up an own domain, where the installation allows it, and " +
			"is answered the two DNS records that prove it. Verification runs by itself.",
		SideEffects: "Writes the domain, waiting for its records; records it in the workspace's trail.",
		TokenScope:  workspaceManageScope,
		Input: []usecase.Field{
			{Name: "domain", Kind: usecase.KindString, Required: true,
				Description: "The domain, such as tasks.example.com."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OwnDomainAddedAction, TargetType: tenantTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a workspace's address is not an entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			return h.Execute(ctx, actor, in.String("domain"))
		}),
	}
}

// RemoveOwnDomain is the owner's removal: the row goes, or a block leaves the owner's view.
type RemoveOwnDomain struct {
	OwnDomains
	Authorizer WorkspaceAuthorizer
}

// Execute removes the caller's workspace's own domain.
func (h RemoveOwnDomain) Execute(ctx context.Context, actor appshared.ActorContext) error {
	if err := h.authorizeOwner(ctx, h.Authorizer, actor, OwnDomainRemovedAction); err != nil {
		return err
	}
	now := h.Clock.Now()
	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		block, err := h.Domains.BlockInView(ctx)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		if _, err := h.Domains.DismissBlocks(ctx, now); err != nil {
			return err
		}
		removed, err := h.Domains.Remove(ctx)
		if errors.Is(err, shared.ErrNotFound) {
			if block.Host == "" {
				return shared.ErrNotFound.WithDetail("admin.own_domain_none")
			}
			return h.trail(ctx, actor.TenantID, actor, OwnDomainRemovedAction, audit.SeverityWarning,
				audit.Change{Field: "own_domain", Classification: audit.Open, From: block.Host}, now)
		}
		if err != nil {
			return err
		}
		if removed.Main {
			// A main own domain is given up by the way back first (UC-INS-15 check 8).
			return shared.ErrConflict.WithDetail("admin.own_domain_is_main")
		}
		return h.trail(ctx, actor.TenantID, actor, OwnDomainRemovedAction, audit.SeverityWarning,
			audit.Change{Field: "own_domain", Classification: audit.Open, From: removed.Host}, now)
	})
}

// Descriptor is the catalogue entry.
func (h RemoveOwnDomain) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        RemoveOwnDomainName,
		Summary:     "The workspace's owner removes its own domain; a blocked one leaves the owner's view and stays blocked.",
		SideEffects: "Deletes the domain, which stops answering at once; records it in the workspace's trail.",
		TokenScope:  workspaceManageScope,
		Audit: usecase.AuditDeclaration{
			Action: OwnDomainRemovedAction, TargetType: tenantTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a workspace's address is not an entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			return usecase.Output{}, h.Execute(ctx, actor)
		}),
	}
}

// ListOwnDomains is the operator's list.
type ListOwnDomains struct {
	OwnDomains
}

// Execute answers every own domain and every block.
func (h ListOwnDomains) Execute(ctx context.Context, actor appshared.ActorContext) ([]identityrepo.ListedOwnDomain, error) {
	if err := h.Instance.authorize(ctx, actor); err != nil {
		return nil, err
	}
	var listed []identityrepo.ListedOwnDomain
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		all, err := h.Doors.All(ctx)
		listed = all
		return err
	})
	return listed, err
}

// Descriptor is the catalogue entry.
func (h ListOwnDomains) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ListOwnDomainsName,
		Summary:     "Answers every own domain the workspaces' owners set up, with its workspace and state, and every block.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: ownDomainsReadAction, TargetType: instanceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "the control plane acts on workspaces, not on items."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			listed, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			items := make([]usecase.Output, 0, len(listed))
			for _, row := range listed {
				items = append(items, ListedOwnDomainOutput(row))
			}
			return usecase.Output{"items": items}, nil
		}),
	}
}

// ListedOwnDomainOutput is the contract's AdminOwnDomain.
func ListedOwnDomainOutput(row identityrepo.ListedOwnDomain) usecase.Output {
	out := usecase.Output{
		"domain": row.Host, "tenant_name": row.TenantName, "state": string(row.State),
		"since": row.Since, "is_main": row.Main,
	}
	if !row.TenantID.IsZero() {
		out["tenant_id"] = row.TenantID.String()
	}
	if row.BlockedReason != "" {
		out["blocked_reason"] = row.BlockedReason
	}
	if !row.FellBackAt.IsZero() {
		out["fell_back_at"] = row.FellBackAt
	}
	if row.AddressHost != "" {
		out["address_host"] = row.AddressHost
	}
	return out
}

// BlockOwnDomain is the operator's block (UC-INS-15 check 10).
type BlockOwnDomain struct {
	OwnDomains
}

// Execute blocks an own domain: it stops answering at once and no workspace sets it up again.
func (h BlockOwnDomain) Execute(
	ctx context.Context, actor appshared.ActorContext, rawHost, rawReason string,
) (identityrepo.ListedOwnDomain, error) {
	if err := h.Instance.authorize(ctx, actor); err != nil {
		return identityrepo.ListedOwnDomain{}, err
	}
	reason, err := domain.BlockReason(rawReason)
	if err != nil {
		return identityrepo.ListedOwnDomain{}, err
	}
	host := domain.RequestHost(rawHost)
	var found *identityrepo.ListedOwnDomain
	err = h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		all, err := h.Doors.All(ctx)
		for i := range all {
			if all[i].Host == host && (found == nil || all[i].State == identityrepo.HostStateBlocked) {
				found = &all[i]
			}
		}
		return err
	})
	if err != nil {
		return identityrepo.ListedOwnDomain{}, err
	}
	if found == nil || found.TenantID.IsZero() && found.State != identityrepo.HostStateBlocked {
		return identityrepo.ListedOwnDomain{}, shared.ErrNotFound.WithDetail("admin.own_domain_not_found")
	}
	if found.State == identityrepo.HostStateBlocked {
		return identityrepo.ListedOwnDomain{}, shared.ErrConflict.WithDetail("admin.own_domain_already_blocked")
	}

	now := h.Clock.Now()
	answer := *found
	scope := persistence.Scope{TenantID: found.TenantID, ActorID: actor.AccountID}
	err = h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		removed, err := h.Domains.Remove(ctx)
		if errors.Is(err, shared.ErrNotFound) || (err == nil && removed.Host != host) {
			return shared.ErrNotFound.WithDetail("admin.own_domain_not_found")
		}
		if err != nil {
			return err
		}
		// A main domain hands the main address back to the address row in this same step
		// (UC-INS-15 check 9); the block remembers it was main, for the owner's warning.
		var more []audit.Change
		if removed.Main {
			to, err := h.Domains.MainToAddress(ctx, time.Time{})
			if err != nil {
				return err
			}
			more = append(more, audit.Change{Field: "host", Classification: audit.Open, From: host, To: to})
		}
		if err := h.Domains.Block(ctx, domain.DomainBlock{
			Host: host, Reason: reason, BlockedAt: now, WasMain: removed.Main,
		}); err != nil {
			return err
		}
		if err := h.trail(ctx, found.TenantID, actor, OwnDomainBlockedAction, audit.SeverityWarning,
			audit.Change{Field: "own_domain", Classification: audit.Open, From: host}, now, more...); err != nil {
			return err
		}
		record, err := h.Tenants.Find(ctx)
		if err != nil {
			return err
		}
		return h.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: h.IDs.NewID(), OccurredAt: now, Action: journalOwnDomainBlocked,
			TenantID: found.TenantID, TenantSlug: record.Slug, ActorLabel: actor.AccountName,
			Details: map[string]any{"domain": host, "reason": reason},
		})
	})
	if err != nil {
		return identityrepo.ListedOwnDomain{}, err
	}
	answer.State, answer.Since, answer.BlockedReason = identityrepo.HostStateBlocked, now, reason
	return answer, nil
}

// Descriptor is the catalogue entry.
func (h BlockOwnDomain) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: BlockOwnDomainName,
		Summary: "Blocks an own domain with a reason: it stops answering at once, the ingress is refused " +
			"its certificate, and no workspace sets it up again. Its owner sees the reason.",
		SideEffects: "Deletes the domain's row and keeps the block; a main domain hands the main address " +
			"back to the workspace's subdomain in the same step; records it in the workspace's trail " +
			"and, with the reason, in the instance journal.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{Name: "domain", Kind: usecase.KindString, Required: true},
			{Name: "reason", Kind: usecase.KindString, Required: true,
				Description: "Why; shown to the workspace's owner word for word."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OwnDomainBlockedAction, TargetType: tenantTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "the control plane acts on workspaces, not on items."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			blocked, err := h.Execute(ctx, actor, in.String("domain"), in.String("reason"))
			if err != nil {
				return nil, err
			}
			return ListedOwnDomainOutput(blocked), nil
		}),
	}
}
