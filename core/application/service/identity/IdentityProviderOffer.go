// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// Whether a provider is a way into *this* workspace (SI-10, the concept's §8).
//
// **One verb, two stores, one question.** For a workspace's own provider the answer lives on the
// row (`enabled`); for one the installation offers it lives in the workspace's settings, because
// the row is the installation's and not the workspace's to change. A caller asks the same thing
// either way — *is this a way in here* — and which store answers is inwards of here.
//
// **An installation's provider is off until a workspace switches it on.** "Für alle Arbeitsbereiche
// angeboten, nirgends an: jeder Owner schaltet selbst." A list of the ones taken rather than of the
// ones refused, so that a provider the installation adds tomorrow is not on everywhere tonight.

const (
	OfferIdentityProviderName = "OfferIdentityProvider"

	// IdentityProviderOfferedAction is its own code rather than the configuration's: switching a
	// way in on or off is the act a review looks for after somebody could not sign in, and it has
	// to be findable without reading every configuration entry.
	IdentityProviderOfferedAction audit.Action = "identity.provider_offered"
)

// OfferIdentityProvider switches a provider on or off as a way into this workspace.
type OfferIdentityProvider struct {
	Writer IdentityProviderWriter
	// Workspaces is where a workspace's own switches live, for the installation's rows.
	Workspaces repository.Workspaces
}

// Execute switches it.
func (h OfferIdentityProvider) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, offered bool, stepUpToken string,
) (domain.IdentityProvider, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     IdentityProviderOfferedAction,
		TokenScope: identityProviderManage,
		TargetType: identityProviderTarget,
	}); err != nil {
		return domain.IdentityProvider{}, err
	}
	// Switching a way in on is as much a change to who signs in here as configuring one.
	if err := w.proveChange(ctx, actor, stepUpToken); err != nil {
		return domain.IdentityProvider{}, err
	}
	if id.IsZero() {
		return domain.IdentityProvider{}, shared.ErrValidation.
			WithDetail("identity_provider.not_found")
	}

	var switched domain.IdentityProvider
	err := w.Session.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Providers.Find(ctx, id)
		if err != nil {
			return err
		}

		// The last way in cannot be switched off. Counted rather than assumed: a workspace whose
		// only method is a provider and who switches it off is a workspace nobody can reach, and
		// the refusal belongs here rather than in a screen that might not be the one asking.
		now := w.Session.Clock.Now()
		if !offered && !anotherWayIn(ctx, h.Writer.Providers, h.Workspaces, found.ID, now) {
			return lastWayIn()
		}
		// An offer the installation ended is not one to take (ADR-0076 §2): from its date it is a
		// way in nowhere, and a switch turned on for it would be a button that leads nowhere.
		if offered && found.Installation() && !found.OfferedAt(now) {
			return shared.ErrValidation.WithDetail("identity_provider.withdrawn")
		}

		if found.Installation() {
			switched, err = h.offerInstallationRow(ctx, found, offered)
		} else {
			switched, err = h.switchOwnRow(ctx, found, offered)
		}
		if err != nil {
			return err
		}
		return w.recordOffer(ctx, actor, switched, offered)
	})
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	return switched, nil
}

// switchOwnRow flips `enabled` on the workspace's own row.
func (h OfferIdentityProvider) switchOwnRow(
	ctx context.Context, found domain.IdentityProvider, offered bool,
) (domain.IdentityProvider, error) {
	found.Enabled = offered
	stored, written, err := h.Writer.Providers.Update(
		ctx, found, nil, h.Writer.Session.Clock.Now())
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	if !written {
		return domain.IdentityProvider{}, shared.ErrNotFound.
			WithDetail("identity_provider.not_found")
	}
	stored.OfferedHere = stored.Enabled
	return stored, nil
}

// offerInstallationRow writes the workspace's own switch. The row is untouched: it belongs to the
// installation, and this workspace is saying what it does with it, not what it is.
func (h OfferIdentityProvider) offerInstallationRow(
	ctx context.Context, found domain.IdentityProvider, offered bool,
) (domain.IdentityProvider, error) {
	if h.Workspaces == nil {
		return domain.IdentityProvider{}, shared.ErrInternal.
			WithDetail("identity_provider.incomplete")
	}
	workspace, err := h.Workspaces.Find(ctx)
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	was := workspace.Settings.Offers(found.ID)
	workspace.Settings = workspace.Settings.WithOffer(found.ID, offered)
	written, err := h.Workspaces.Update(
		ctx, workspace, workspace.Version, h.Writer.Session.Clock.Now())
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	if !written {
		return domain.IdentityProvider{}, shared.ErrConflict.WithDetail("workspace.version_stale")
	}
	// The installation's count of workspaces that use it (ADR-0076 §1), moved by this switch and
	// by nothing else, in the same transaction - and only when the switch changed something, so a
	// repeated "on" is not a second workspace. The write above is guarded on the version, which is
	// what keeps two administrators switching at once from moving it twice.
	if was != offered {
		step := 1
		if !offered {
			step = -1
		}
		if err := h.Writer.Providers.MoveOfferCount(ctx, found.ID, step); err != nil {
			return domain.IdentityProvider{}, err
		}
		if offered {
			found.OfferedWorkspaces++
		} else {
			found.OfferedWorkspaces = max(0, found.OfferedWorkspaces-1)
		}
	}
	found.OfferedHere = offered
	return found, nil
}

// anotherWayIn answers whether this workspace would still have a way in without the provider named.
//
// A password method counts, and so does any other provider that is on here. What it is guarding is
// the workspace with `methods: [OIDC]` and one provider — the arrangement in which one switch locks
// everybody out. Every door that can take a provider away asks it: the switch, the provider's own
// form and its removal (UC-ID-12 check 6, UC-ID-11 check 8).
func anotherWayIn(
	ctx context.Context, providers repository.IdentityProviders, workspaces repository.Workspaces,
	excluding shared.ID, now time.Time,
) bool {
	var workspace domain.Workspace
	if workspaces != nil {
		found, err := workspaces.Find(ctx)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			// Unreadable is not "there is another": a guard that fails open is not a guard.
			return false
		}
		workspace = found
	}
	on, err := providerOnHere(ctx, providers, workspace.Settings, excluding, now)
	if err != nil {
		return false
	}
	if on {
		return true
	}
	// No other provider. The workspace still has a way in when a password opens one, which is what
	// the effective policy's methods say — and a workspace with no policy above it signs in with a
	// password, because that is the product's own default.
	return !onlyProviderMethods(workspace)
}

// providerOnHere answers whether any provider other than the one named is a way into this workspace.
func providerOnHere(
	ctx context.Context, providers repository.IdentityProviders, settings domain.WorkspaceSettings,
	excluding shared.ID, now time.Time,
) (bool, error) {
	if providers == nil {
		return false, nil
	}
	inForce, err := providers.List(ctx)
	if err != nil {
		return false, err
	}
	for _, candidate := range inForce {
		if candidate.ID != excluding && offeredHere(candidate, settings, now) {
			return true, nil
		}
	}
	return false, nil
}

// lastWayIn is the one refusal every door gives.
func lastWayIn() error {
	return shared.ErrValidation.WithDetail("identity_provider.last_way_in")
}

// onlyProviderMethods reports whether this workspace has switched password sign-in off.
func onlyProviderMethods(workspace domain.Workspace) bool {
	methods := workspace.Settings.SignIn.Methods
	if methods == nil {
		return false
	}
	for _, method := range *methods {
		if method == domain.MethodDirect {
			return false
		}
	}
	return true
}

// offeredHere is the one reading of "is this a way in here", shared by the listing, the guard, the
// sign-in card and the sign-in itself. At a moment, because an installation's offer can end on an
// announced date (ADR-0076 §2) and the date is honoured here, where the offer is read, rather than
// by a job that would have to walk every workspace.
func offeredHere(
	configured domain.IdentityProvider, settings domain.WorkspaceSettings, now time.Time,
) bool {
	if !configured.OfferedAt(now) {
		return false
	}
	if configured.Installation() {
		return settings.Offers(configured.ID)
	}
	return true
}

// recordOffer writes the trail entry. The provider and the direction, never a secret.
func (w IdentityProviderWriter) recordOffer(
	ctx context.Context, actor appshared.ActorContext,
	switched domain.IdentityProvider, offered bool,
) error {
	return w.Session.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: w.Session.Clock.Now(),
		Action:     IdentityProviderOfferedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: identityProviderTarget,
		TargetID:   switched.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "offered_here", Classification: audit.Open,
				To: strconv.FormatBool(offered)},
			audit.Change{Field: "scope", Classification: audit.Open,
				To: providerScope(switched)}),
	})
}

func (h OfferIdentityProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: OfferIdentityProviderName,
		Summary: "Switches a provider on or off as a way into this workspace. For the workspace's " +
			"own provider that is the row's switch; for one the installation offers, it is this " +
			"workspace's own - the row belongs to the installation and is not the workspace's to " +
			"change, but whether it is offered here is. An installation's provider is off until " +
			"somebody switches it on. The last way in cannot be switched off.",
		SideEffects: "Writes the switch and an audit entry.",
		TokenScope:  identityProviderManage,
		Input: []usecase.Field{
			{Name: "id", Kind: usecase.KindString, Required: true,
				Description: "The provider to switch."},
			{Name: "offered", Kind: usecase.KindBool, Required: true,
				Description: "Whether it is a way into this workspace."},
			ProviderStepUpField,
		},
		StepUp: providerStepUp,
		Audit: usecase.AuditDeclaration{
			Action: IdentityProviderOfferedAction, TargetType: identityProviderTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's sign-in configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h OfferIdentityProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("id")
	if err != nil {
		return nil, err
	}
	switched, err := h.Execute(ctx, actor, id, in.Bool("offered"), in.String("step_up_token"))
	if err != nil {
		return nil, err
	}
	return ProviderOutput(switched), nil
}
