// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package identity declares what the identity use cases need from storage. The interfaces live
// here, with their callers; the implementations live in infrastructure/postgres
// (project-structure.md §2).
package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Credential is what one lookup yields: the token, the account behind it, and the tenant's
// defaults.
//
// All three in one result rather than three calls, because this runs on every single request. The
// tenant's preferences are two columns on a row the query has to reach anyway, and the difference
// between one round trip and three is the difference between an API that feels immediate and one
// that does not.
type Credential struct {
	Token   identity.AccessToken
	Account identity.Account
	// TenantLocale and TenantTimeZone are the third link of the resolution chain
	// (i18n-l10n.md §2). Never empty: the columns have defaults.
	TenantLocale   string
	TenantTimeZone string
}

// AccessTokens finds and maintains personal access tokens.
//
// The presented token is passed whole rather than pre-hashed. Hashing needs the server-side
// pepper, and the pepper has no business in the application layer - it is a secret of the
// persistence adapter, which is also the only place that can compare against what is stored
// (security.md §8).
type AccessTokens interface {
	// FindByToken returns the credential a presented token names, or an error wrapping
	// shared.ErrNotFound when the hash matches nothing.
	//
	// It reports what is stored and judges none of it: expiry, revocation and account status are
	// decided by the use case, so the rules stay where they can be tested without a database
	// (ADR-0001).
	FindByToken(ctx context.Context, token identity.Token) (Credential, error)

	// TouchLastUsed records that the token was used. Called at most once per interval, so an
	// owner can see a token nobody uses without every request costing a write.
	TouchLastUsed(ctx context.Context, tokenID shared.ID, at time.Time) error
}
