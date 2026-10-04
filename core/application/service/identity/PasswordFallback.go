// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The fail-safe of ADR-0076 §4: no workspace is left without a way in.
//
// When the ways in a workspace's rule resolves to leave none that works - the password is not among
// them and no provider is switched on here - the password opens again: for the accounts that hold
// one, under the workspace's own password and second-factor rules. **Whatever the cause** (the
// owner's decision of 2026-10-04, E2, #1138): an offer that ended, which is where ADR-0076 §4 began,
// but just as much an installation default or lock without the password, a rescue lock lifted after
// the provider went, a restore or an import that brought the settings without the providers, or two
// administrators switching off the last two ways at once. The guards at the doors (the last way in,
// SC-06) refuse what they can see; this is the net under all of them, so nothing else has to be
// perfect - no row lock for the race, no job on the day an offer ends. It is read wherever the ways
// in are resolved, and it ends the moment an administrator switches on a way in, which is what the
// screen asks them to do. An account without a password gains nothing from the sign-in: there is
// nothing to sign in with (ADR-0077 §3 is its way back).

// PasswordFallbackAction is a password sign-in that the fallback let through, in the workspace's
// own trail (ADR-0076 §4, "the workspace's trail records it").
const PasswordFallbackAction audit.Action = "auth.password_fallback"

// FallbackCauseNoWayIn is the `cause` the fallback's trail entry carries: the workspace's methods left
// no way in that works. One cause today, because the predicate no longer asks which (E2, #1138); the
// field is there so that an administrator reading the trail is not left to guess, and so that a
// second way the password can open - an operator's own lever, which SC-34 builds - is told apart
// from this one rather than recorded as the same thing.
const FallbackCauseNoWayIn = "NO_WAY_IN"

// fallbackOpens answers whether the password opens only as the fallback: the workspace's methods
// leave it out, and no provider is a way in here now. Nothing asks why (E2, #1138) - a cause the
// predicate had to recognise is a cause it could miss, and every miss is a lockout.
func fallbackOpens(
	methods []string, inForce []domain.IdentityProvider, settings domain.WorkspaceSettings,
	now time.Time,
) bool {
	if slices.Contains(methods, domain.MethodDirect) {
		return false
	}
	for _, configured := range inForce {
		if configured.Issuer != "" && offeredHere(configured, settings, now) {
			return false
		}
	}
	return true
}

// WaysIn reads what the fallback needs for a door that knows only the workspace: its ways in and
// its own switches for the installation's providers. Its zero value answers "no fallback", which is
// the shape before ADR-0076.
type WaysIn struct {
	Providers  repository.IdentityProviders
	Workspaces repository.Workspaces
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// PasswordFallback answers fallbackOpens for one workspace under the methods its rule resolved to.
func (w WaysIn) PasswordFallback(ctx context.Context, tenantID shared.ID, methods []string) (bool, error) {
	if w.Providers == nil || w.Workspaces == nil || w.UnitOfWork == nil || w.Clock == nil ||
		slices.Contains(methods, domain.MethodDirect) {
		return false, nil
	}
	var opens bool
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			inForce, err := w.Providers.List(ctx)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			workspace, err := w.Workspaces.Find(ctx)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			opens = fallbackOpens(methods, inForce, workspace.Settings, w.Clock.Now())
			return nil
		})
	return opens, err
}

// recordFallback writes the trail entry for a password the fallback let through: who, in which
// workspace, why the password was open, and nothing of the credential.
func (w SessionWriter) recordFallback(
	ctx context.Context, scope persistence.Scope, account domain.Account,
) error {
	return w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return w.Audit.Append(ctx, audit.Entry{
			TenantID:   scope.TenantID,
			OccurredAt: w.Clock.Now(),
			Action:     PasswordFallbackAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityWarning,
			ActorKind:  appshared.ActorUser,
			ActorID:    account.ID,
			ActorLabel: account.DisplayName,
			TargetType: workspaceTarget,
			TargetID:   scope.TenantID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "method", Classification: audit.Open, To: domain.MethodDirect},
				audit.Change{Field: "cause", Classification: audit.Open, To: FallbackCauseNoWayIn},
			),
		})
	})
}
