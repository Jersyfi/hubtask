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
// When the only way into a workspace was a provider the installation offered, and that offer has
// ended - withdrawn on its date, withdrawn now, or removed - the password opens again: for the
// accounts that hold one, under the workspace's own password and second-factor rules. It is read,
// like the offer itself, wherever the ways in are resolved, so nothing has to run on the day. It
// ends the moment an administrator switches on another way, which is what the screen asks them to
// do. An account without a password is let back in through its mailbox: the reset mails it a link to
// set one (ADR-0077 §3, MintResetToken).

// PasswordFallbackAction is a password sign-in that the fallback let through, in the workspace's
// own trail (ADR-0076 §4, "the workspace's trail records it").
const PasswordFallbackAction audit.Action = "auth.password_fallback"

// fallbackOpens answers whether the password opens only as the fallback: the workspace has switched
// it off, no provider is a way in now, and one it had switched on is an offer that has ended.
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
	for _, taken := range settings.OfferedProviders {
		at := slices.IndexFunc(inForce, func(row domain.IdentityProvider) bool { return row.ID == taken })
		// Gone is ended too: an installation's provider removed outright leaves the workspace's
		// switch behind, and the workspace in exactly the place a withdrawal would.
		if at < 0 || !inForce[at].OfferedAt(now) {
			return true
		}
	}
	return false
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
// workspace, and nothing of the credential.
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
			Changes: audit.Changes(audit.Change{
				Field: "method", Classification: audit.Open, To: domain.MethodDirect,
			}),
		})
	})
}
