// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"log/slog"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// AccessibilityCommand is how an operator sets the statement from a terminal, which the hint names
// beside the screen. Not a sentence: the catalogue says what to do with it.
const AccessibilityCommand = "hubctl admin settings set legal.accessibility_url <address>"

// AccessibilityWarnings is the health report's hint that an installation serving other people
// names no accessibility statement (UC-ID-18).
//
// Only where the installation runs workspaces for others - multi mode, a provider for consumers or
// for companies - and never on a private installation, which owes nobody a statement (P-10). Only
// the installation's own link counts: a provider whose workspaces each set their own still offers
// the sign-in card of every workspace that set none. Advice rather than a fault, because a
// provider may lawfully owe none (UC-ID-18, *Where it ends*): a level that cannot be read gives
// no hint, and says so in the log.
type AccessibilityWarnings struct {
	Tenancy    env.TenancyMode
	Settings   identityrepo.InstanceSettings
	UnitOfWork persistence.UnitOfWork
}

// Warnings answers the hint where it holds.
func (a AccessibilityWarnings) Warnings(ctx context.Context) []health.Warning {
	if a.Tenancy != env.TenancyMulti || a.Settings == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, healthReadTimeout)
	defer cancel()

	var level identityrepo.InstanceLevel
	err := a.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		read, err := a.Settings.Read(ctx)
		level = read
		return err
	})
	if err != nil {
		slog.WarnContext(ctx, "the instance level could not be read for the accessibility hint",
			slog.String("error", err.Error()))
		return nil
	}
	if level.Legal.Links.Of(identity.LinkAccessibility) != "" {
		return nil
	}
	return []health.Warning{{
		Code: "config.accessibility_statement_missing", Severity: "info",
		Params: map[string]string{"command": AccessibilityCommand},
	}}
}
