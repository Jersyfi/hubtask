// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package stream

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Reauthenticate runs a request's authentication again, with the credential it was given.
type Reauthenticate func(ctx context.Context) (appshared.ActorContext, error)

type reauthenticateKey struct{}

// WithReauthentication leaves the request's own re-authentication in its context, for every held
// connection - a live document, the change stream, an agent's stream - to ask alike.
func WithReauthentication(ctx context.Context, again Reauthenticate) context.Context {
	return context.WithValue(ctx, reauthenticateKey{}, again)
}

// CredentialStill is the question a held connection asks while it stays open: does the credential
// it was admitted with still authenticate as the same actor? A revoked or expired session, a
// disabled account and a view as somebody that ended answer no. Nil where the request was not
// authenticated by the middleware that leaves the question.
func CredentialStill(ctx context.Context, actor appshared.ActorContext) func(context.Context) error {
	again, ok := ctx.Value(reauthenticateKey{}).(Reauthenticate)
	if !ok {
		return nil
	}
	return func(ctx context.Context) error {
		now, err := again(ctx)
		if err != nil {
			return err
		}
		if now.TenantID != actor.TenantID || now.AccountID != actor.AccountID || now.ViewedBy != actor.ViewedBy {
			return shared.ErrUnauthenticated.WithDetail("access.token_revoked")
		}
		return nil
	}
}

// Accounts are those whose revocation reaches a stream of the actor: its own, and the viewer's
// while somebody views the app as the actor. What Registry.Admit is given.
func Accounts(actor appshared.ActorContext) []string {
	accounts := []string{actor.AccountID.String()}
	if actor.ViewedBy != "" {
		accounts = append(accounts, actor.ViewedBy.String())
	}
	return accounts
}
