// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package identity holds the use cases that turn a credential into an actor.
package identity

import (
	"context"
	"errors"
	"log/slog"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// LastUsedInterval is how often a token's last use is written back. Minutes of resolution are
// enough for the question it answers - "is anybody still using this token?" - and anything finer
// would turn a read path into a write path.
const LastUsedInterval = 5 * time.Minute

// AuthenticateToken turns a presented personal access token into the actor of the request.
//
// It authenticates and nothing more. Whether the actor may perform the operation is a separate
// question, answered by the use case that performs it (ADR-0005) - which is why this returns an
// ActorContext rather than a decision.
type AuthenticateToken struct {
	Tokens     repository.AccessTokens
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// AuthenticateTokenCommand carries the presented credential and the preferences resolved from the
// request, which win over the account's and the tenant's where the client stated them
// (i18n-l10n.md §2).
type AuthenticateTokenCommand struct {
	Credential string
	// RequestedLocale is empty when the client expressed no preference.
	RequestedLocale string
	// FallbackLocale and FallbackTimeZone are the installation defaults, the last link of the
	// chain.
	FallbackLocale   string
	FallbackTimeZone string
}

// Execute verifies the credential and builds the actor.
//
// The unit of work is opened for the tenant the token names. That is safe without checking the
// claim first: the hash is unique across the installation, so a token quoting a tenant it does
// not belong to finds nothing, and row level security makes finding nothing the only possible
// outcome (multi-tenancy.md §2.1).
func (a AuthenticateToken) Execute(
	ctx context.Context,
	cmd AuthenticateTokenCommand,
) (appshared.ActorContext, error) {
	token, err := identity.ParseToken(cmd.Credential)
	if err != nil {
		return appshared.ActorContext{}, err
	}

	var actor appshared.ActorContext
	scope := persistence.Scope{TenantID: token.TenantID()}

	// A read-write transaction although almost every request only reads: the last-use write
	// happens on the same row that was just read, and a second transaction for it would double
	// the round trips of the authentication path. An empty read-write transaction costs
	// PostgreSQL nothing - a transaction identifier is assigned on the first write, not on BEGIN.
	err = a.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		credential, err := a.Tokens.FindByToken(ctx, token)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// Deliberately the same answer as a malformed token. Whether a hash exists is
				// exactly what an attacker is trying to find out.
				return shared.ErrUnauthenticated.WithDetail("access.token_unknown")
			}
			return err
		}

		now := a.Clock.Now()
		if err := credential.Token.Verify(now); err != nil {
			return err
		}
		if err := credential.Account.Verify(); err != nil {
			return err
		}

		if credential.Token.NeedsTouch(now, LastUsedInterval) {
			if err := a.Tokens.TouchLastUsed(ctx, credential.Token.ID, now); err != nil {
				// The request is authenticated either way. Failing it because a bookkeeping
				// column could not be updated would turn a cosmetic problem into an outage
				// (ADR-0016, "an optional dependency does not block the path").
				slog.WarnContext(ctx, "recording the last use of a token failed",
					slog.String("error", err.Error()))
			}
		}

		actor = appshared.ActorContext{
			Kind:      actorKind(credential.Account.Kind),
			TenantID:  credential.Token.TenantID,
			AccountID: credential.Account.ID,
			TokenID:   credential.Token.ID,
			Scopes:    credential.Token.Scopes,
			Locale: firstNonEmpty(
				cmd.RequestedLocale, credential.Account.Locale,
				credential.TenantLocale, cmd.FallbackLocale),
			TimeZone: firstNonEmpty(
				credential.Account.TimeZone, credential.TenantTimeZone, cmd.FallbackTimeZone),
		}
		return nil
	})
	if err != nil {
		return appshared.ActorContext{}, err
	}
	return actor, nil
}

// actorKind maps the account kind to the actor kind of the audit trail. An unknown kind becomes a
// service account: the stricter of the two readings, since a machine actor is what a permission
// check treats most carefully.
func actorKind(kind identity.AccountKind) appshared.ActorKind {
	if kind == identity.AccountUser {
		return appshared.ActorUser
	}
	return appshared.ActorServiceAccount
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
