// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	CreateAccessTokenName = "CreateAccessToken"
	ListAccessTokensName  = "ListAccessTokens"
	RevokeAccessTokenName = "RevokeAccessToken"

	// accountsRead is the read counterpart of accountRead, which despite its name is
	// accounts:write. Listing one's own credentials is a read of one's own account.
	accountsRead = "accounts:read"

	// The audit codes. A credential is an asset in its own right, so both its minting and its
	// withdrawal are auditable acts (audit.md §2).
	TokenCreatedAction audit.Action = "access.token_created"
	TokenRevokedAction audit.Action = "access.token_revoked"
	// TokenReadAction is what a listing performs. Declared so that the three share one target
	// type rather than inventing one each.
	TokenReadAction audit.Action = "access.token_read"

	tokenTarget = "access_token"
)

// AccessTokenWriter is what the three credential use cases share.
//
// One dependency set rather than three, for the reason the calendar feeds have one: the rule that
// decides whose tokens somebody may touch is a single rule, and a second copy of a rule about
// credentials is how two answers to one security question get into a codebase.
type AccessTokenWriter struct {
	Tokens     repository.AccessTokens
	Accounts   repository.Accounts
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Entropy is where the secret half comes from. A port, so that production draws from
	// crypto/rand and a test can fix the credential it asserts on (rule 4).
	Entropy clock.Entropy
	// KnownScopes is every scope this build declares, which is the union of the descriptors'.
	// It is passed in rather than read, because the catalogue is assembled from these very use
	// cases and a package that imported it would close the circle (ADR-0001).
	KnownScopes []string
}

// CreateAccessTokenCommand is the input, typed.
type CreateAccessTokenCommand struct {
	// AccountID is whose credential. Zero means the caller's own.
	AccountID shared.ID
	Name      string
	Scopes    []string
	ExpiresAt time.Time
}

// MintedToken is what a mint answers: the row as it will be listed, and the credential that will
// never be readable again.
//
// The plaintext travels in secret.Secret rather than as a string, so that a struct printed whole
// - the shape that actually leaks - masks it (T-18, rule 10).
type MintedToken struct {
	Token  domain.AccessToken
	Secret secret.Secret
}

// CreateAccessToken mints a personal access token and answers it once (G-01).
type CreateAccessToken struct{ Writer AccessTokenWriter }

// Execute mints the credential.
//
// The scopes are a bound rather than a grant: a token can never do more than the role its holder
// carries, whatever it asks for, because both are checked on every call (ADR-0005). That is why
// asking for a scope one does not currently exercise is not an escalation and needs no permission
// of its own - and why asking for a scope this installation does not declare is refused, since a
// name nothing checks is a bound nothing applies.
func (h CreateAccessToken) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateAccessTokenCommand,
) (MintedToken, error) {
	w := h.Writer

	owner, err := w.resolveOwner(ctx, actor, cmd.AccountID, accountRead, TokenCreatedAction)
	if err != nil {
		return MintedToken{}, err
	}
	if err := w.checkScopes(cmd.Scopes); err != nil {
		return MintedToken{}, err
	}

	material, err := w.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return MintedToken{}, shared.ErrInternal.
			WithDetail("access.token_unmintable").
			WithCause(err)
	}
	presented, err := domain.NewToken(actor.TenantID, material)
	if err != nil {
		return MintedToken{}, err
	}

	var minted domain.AccessToken
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		token, err := domain.NewAccessToken(domain.NewAccessTokenInput{
			ID: w.IDs.NewID(), TenantID: actor.TenantID, AccountID: owner,
			Name: cmd.Name, Scopes: cmd.Scopes, ExpiresAt: cmd.ExpiresAt, Now: now,
		})
		if err != nil {
			return err
		}
		if err := w.Tokens.Insert(ctx, token, presented); err != nil {
			return err
		}
		minted = token
		return w.recordAudit(ctx, actor, token, TokenCreatedAction, now)
	})
	if err != nil {
		return MintedToken{}, err
	}
	return MintedToken{Token: minted, Secret: secret.New(presented.Secret())}, nil
}

// ListAccessTokens answers the credentials of one account.
type ListAccessTokens struct{ Writer AccessTokenWriter }

// Execute reads them. One's own need no permission; a service account's need the one that manages
// members; another person's are refused whatever the role, because a list of somebody's
// credentials is a list of which of their automations to attack.
func (h ListAccessTokens) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
) ([]domain.AccessToken, error) {
	w := h.Writer

	owner, err := w.resolveOwner(ctx, actor, accountID, accountsRead, TokenReadAction)
	if err != nil {
		return nil, err
	}

	var tokens []domain.AccessToken
	err = w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Tokens.ListForAccount(ctx, owner)
		tokens = found
		return err
	})
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// RevokeAccessToken stops a credential.
type RevokeAccessToken struct{ Writer AccessTokenWriter }

// Execute revokes the token, and is idempotent: revoking twice is somebody making sure.
//
// It takes effect on the next call rather than eventually: the hash is checked against the row on
// every single request, so there is no cache whose life a withdrawn credential could outlive.
//
// Revocation is never harder than minting, deliberately. The moment somebody needs it is the
// moment a credential has gone somewhere it should not have, and a person who cannot stop their
// own leak because their role changed in the meantime is a security problem rather than a policy.
func (h RevokeAccessToken) Execute(
	ctx context.Context, actor appshared.ActorContext, tokenID shared.ID,
) error {
	w := h.Writer
	if err := actor.RequireScope(accountRead); err != nil {
		return err
	}
	if tokenID.IsZero() {
		return shared.ErrValidation.
			WithDetail("access.token_id_required").
			WithFields(shared.FieldError{Path: "/token_id", Code: "access.token_id_required"})
	}

	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		token, err := w.Tokens.Find(ctx, tokenID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return tokenNotFound(tokenID)
			}
			return err
		}
		if token.AccountID != actor.AccountID {
			// Somebody else's. A service account's is administered by whoever answers for access;
			// another person's is not found, for the reason every other read of somebody else's
			// thing is not found (T-04).
			if err := w.requireServiceAccountAdministration(
				ctx, actor, token.AccountID, accountRead, TokenRevokedAction,
			); err != nil {
				return err
			}
		}

		now := w.Clock.Now()
		changed, err := w.Tokens.Revoke(ctx, token.ID, now)
		if err != nil {
			return err
		}
		if !changed {
			// Already revoked. No second audit entry: nothing happened, and an entry saying it
			// did would be a false one.
			return nil
		}
		return w.recordAudit(ctx, actor, token.Revoked(now), TokenRevokedAction, now)
	})
}

// resolveOwner decides whose credentials the caller is touching, and whether they may.
//
// Three cases and no fourth: one's own are self-service through the account scope, exactly as
// changing one's own preferences is; a service account's need MANAGE_MEMBERS, because an account
// that is nothing but access is administered by whoever answers for access; and another person's
// are refused whatever the role, because nobody administers somebody else's credentials.
func (w AccessTokenWriter) resolveOwner(
	ctx context.Context, actor appshared.ActorContext, requested shared.ID,
	tokenScope string, action audit.Action,
) (shared.ID, error) {
	if requested.IsZero() || requested == actor.AccountID {
		if err := actor.RequireScope(tokenScope); err != nil {
			return "", err
		}
		if actor.AccountID.IsZero() {
			// A credential belongs to an account. The system itself has none, and a token minted
			// by nobody would be a credential nobody can revoke.
			return "", shared.ErrForbidden.WithDetail("access.token_owner_required")
		}
		return actor.AccountID, nil
	}

	if err := w.requireServiceAccountAdministration(ctx, actor, requested, tokenScope, action); err != nil {
		return "", err
	}
	return requested, nil
}

// requireServiceAccountAdministration is the one path to somebody else's credentials, and it
// leads only to a machine's.
func (w AccessTokenWriter) requireServiceAccountAdministration(
	ctx context.Context, actor appshared.ActorContext, owner shared.ID,
	tokenScope string, action audit.Action,
) error {
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     action,
		TokenScope: tokenScope,
		TargetType: tokenTarget,
		TargetID:   owner,
	}); err != nil {
		return err
	}

	var account domain.Account
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Accounts.Find(ctx, owner)
		account = found
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return accountNotFound(owner)
		}
		return err
	}
	if account.Kind != domain.AccountServiceAccount {
		// A person's credentials are theirs. An administrator may disable the account or revoke
		// its memberships - both of which stop it acting - and may not hold or enumerate what it
		// authenticates with.
		return shared.ErrForbidden.WithDetail("access.token_not_administrable")
	}
	return nil
}

// checkScopes refuses a name the installation does not declare.
//
// A field error rather than a silent drop: a token minted with a scope nobody checks is a token
// whose holder believes it can do something it cannot, and they find out at the moment it matters.
func (w AccessTokenWriter) checkScopes(requested []string) error {
	var fields []shared.FieldError
	for index, scope := range requested {
		if scope == "" || slices.Contains(w.KnownScopes, scope) {
			continue
		}
		fields = append(fields, shared.FieldError{
			Path: "/scopes/" + strconv.Itoa(index),
			Code: "access.token_scope_unknown",
			// The scope is the caller's own input and a protocol identifier rather than content,
			// so naming it back is what makes the refusal actionable (api-guidelines.md §6).
			Params: map[string]string{"scope": scope},
		})
	}
	if len(fields) == 0 {
		return nil
	}
	return shared.ErrValidation.
		WithDetail("access.token_scope_unknown").
		WithParams(map[string]string{"scope": fields[0].Params["scope"]}).
		WithFields(fields...)
}

// recordAudit writes the evidence. Every field on it is an identifier or a scope name: the one
// value that would matter - the credential - is written nowhere, here least of all (rule 10).
func (w AccessTokenWriter) recordAudit(
	ctx context.Context, actor appshared.ActorContext,
	token domain.AccessToken, action audit.Action, at time.Time,
) error {
	changes := []audit.Change{
		{Field: "account_id", Classification: audit.Open, To: token.AccountID.String()},
		// The scopes and the expiry are what the credential can do and for how long, which is
		// exactly what an auditor asks of a minting. The name is the owner's free text and stays
		// out: it is theirs, and a trail does not need it to answer the question.
		{Field: "scopes", Classification: audit.Open, To: strings.Join(token.Scopes, " ")},
		{Field: "expires_at", Classification: audit.Open, To: token.ExpiresAt.UTC().Format(time.RFC3339)},
	}
	if action == TokenRevokedAction {
		changes = append(changes, audit.Change{
			Field: "revoked_at", Classification: audit.Open,
			To: token.RevokedAt.UTC().Format(time.RFC3339),
		})
	}

	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   token.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		// Notice rather than info, on InviteAccount's reasoning: somebody now has a way into this
		// workspace, or one has just been taken away. Both are the class of event a review looks
		// for, and a listing is not (audit.md §2).
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: tokenTarget,
		TargetID:   token.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// parseExpiry reads the one spelling the contract declares, RFC 3339. An unreadable value is a
// field error rather than a zero time: a zero time means "no expiry was given", and that is a
// different refusal from "what you gave is not a date".
func parseExpiry(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, shared.ErrValidation.
			WithDetail("access.token_expiry_malformed").
			WithParams(map[string]string{"value": raw}).
			WithFields(shared.FieldError{Path: "/expires_at", Code: "access.token_expiry_malformed"})
	}
	return at, nil
}

func tokenNotFound(id shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("access.token_not_found").
		WithParams(map[string]string{"token_id": id.String()})
}

func accountNotFound(id shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("identity.account_not_found").
		WithParams(map[string]string{"account_id": id.String()})
}

// tokenOutput is the projection every channel gets. The credential is not in it: it exists in the
// answer to the mint and nowhere else, which is what "shown once" means.
func tokenOutput(token domain.AccessToken) usecase.Output {
	scopes := make([]any, 0, len(token.Scopes))
	for _, scope := range token.Scopes {
		scopes = append(scopes, scope)
	}

	out := usecase.Output{
		"id":           token.ID.String(),
		"account_id":   token.AccountID.String(),
		"name":         token.Name,
		"scopes":       scopes,
		"expires_at":   token.ExpiresAt.UTC(),
		"created_at":   token.CreatedAt.UTC(),
		"last_used_at": nil,
		"revoked_at":   nil,
	}
	if !token.LastUsedAt.IsZero() {
		out["last_used_at"] = token.LastUsedAt.UTC()
	}
	if token.IsRevoked() {
		out["revoked_at"] = token.RevokedAt.UTC()
	}
	return out
}

// Descriptor is the catalogue entry.
func (h CreateAccessToken) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateAccessTokenName,
		Summary: "Mints a personal access token and answers it in clear, once. What is stored " +
			"is a hash keyed on the installation secret under its own purpose label, so nothing " +
			"can turn it back into the credential and a token that was lost is revoked and " +
			"minted again. The expiry is mandatory and at most a year out - there is no " +
			"default. The scopes are asked for explicitly rather than defaulted to everything, " +
			"and they bound the token rather than granting it anything: it can never do more " +
			"than its holder may.",
		SideEffects: "Writes the token and an audit entry, and answers a credential.",
		TokenScope:  accountRead,
		Input: []usecase.Field{
			{
				Name: "name", Kind: usecase.KindString, Required: true,
				Description: "What the token is for, in the owner's words. What they read in a " +
					"year when deciding whether it is still needed.",
			},
			{
				Name: "scopes", Kind: usecase.KindList, Required: true,
				Description: "The scopes it may exercise. Each has to be one this installation " +
					"declares; one it does not is refused as a field error naming it.",
			},
			{
				Name: "expires_at", Kind: usecase.KindString, Required: true,
				Description: "When it stops working. In the future and at most a year out.",
			},
			{
				Name: "account_id", Kind: usecase.KindID,
				Description: "Whose token. Omitted means the caller's own. A service account's " +
					"needs the member management permission; another person's is refused.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TokenCreatedAction, TargetType: tokenTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A credential is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateAccessToken) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	scopes, err := in.StringList("scopes")
	if err != nil {
		return nil, err
	}
	expiresAt, err := parseExpiry(in.String("expires_at"))
	if err != nil {
		return nil, err
	}

	minted, err := h.Execute(ctx, actor, CreateAccessTokenCommand{
		AccountID: accountID,
		Name:      in.String("name"),
		Scopes:    scopes,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, err
	}

	// The one place the credential is ever answered. Every channel gets it here and no channel
	// can get it again - the projection every other call uses does not carry it.
	out := tokenOutput(minted.Token)
	out["token"] = minted.Secret.Reveal()
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ListAccessTokens) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListAccessTokensName,
		Summary: "One account's personal access tokens, newest first, with the scopes each " +
			"carries, when it expires, when it was last used and whether it was revoked. The " +
			"credential itself is not among the fields: it is answered once, at the mint. " +
			"One's own need no permission, a service account's need the one that manages " +
			"members, and another person's are refused whatever the role.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID,
				Description: "Whose tokens. Omitted means the caller's own.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TokenReadAction, TargetType: tokenTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListAccessTokens) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}

	tokens, err := h.Execute(ctx, actor, accountID)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(tokens))
	for _, token := range tokens {
		rows = append(rows, tokenOutput(token))
	}
	return usecase.Output{"data": rows}, nil
}

// Descriptor is the catalogue entry.
func (h RevokeAccessToken) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RevokeAccessTokenName,
		Summary: "Revokes a personal access token. It takes effect on the next call, because " +
			"the hash is checked against the row on every request. The row stays, stamped with " +
			"the moment it was withdrawn - separate from the expiry, so that \"it ran out\" and " +
			"\"somebody pulled it\" stay distinguishable. Revoking twice is not an error; " +
			"somebody else's is not found rather than forbidden.",
		SideEffects: "Stamps the token and writes an audit entry. A repeat writes neither.",
		TokenScope:  accountRead,
		Destructive: true,
		Input: []usecase.Field{
			{
				Name: "token_id", Kind: usecase.KindID, Required: true,
				Description: "The token to revoke, from the owner's own list.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TokenRevokedAction, TargetType: tokenTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason minting one is exempt: a credential is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RevokeAccessToken) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tokenID, err := in.ID("token_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, tokenID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
