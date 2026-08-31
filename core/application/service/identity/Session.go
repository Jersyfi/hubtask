// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	SignInName         = "SignIn"
	RefreshSessionName = "RefreshSession"

	sessionTarget = "session"
)

// The audit codes of the sign-in flow. Opening a way into the workspace and detecting a stolen
// one are both the class of event a review looks for (audit.md §2, T-01).
const (
	SignedInAction         audit.Action = "auth.signed_in"
	SessionRefreshedAction audit.Action = "auth.session_refreshed"
	RefreshReuseAction     audit.Action = "auth.refresh_reuse_detected"
)

// The metric reasons of hubtask_auth_failures_total, a closed set (§3.2). refresh_reused is the
// one A-15's second half watches.
const (
	FailureWrongCredential = "wrong_credential" //nolint:gosec // G101: a metric label, not a credential
	FailureLocked          = "locked"
	FailureRefreshRefused  = "refresh_refused"
	FailureRefreshReused   = "refresh_reused"
	FailureRedemption      = "redemption_refused"
)

// AuthSignals is what the sign-in flow reports about itself: one counter, by reason. An
// implementation must never receive a subject - the reasons are the whole label set.
type AuthSignals interface {
	AuthFailure(ctx context.Context, reason string)
}

// SessionWriter is what the session use cases share, AccessTokenWriter's shape: one dependency
// set, because the rules about one credential pair belong in one place.
type SessionWriter struct {
	Accounts   repository.SignInAccounts
	Sessions   repository.Sessions
	Refresh    repository.RefreshTokens
	Attempts   repository.AuthAttempts
	Tenants    repository.TenantDirectory
	Passwords  cryptoport.PasswordHasher
	Signer     cryptoport.SessionTokenSigner
	Audit      audit.Sink
	Signals    AuthSignals
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Entropy    clock.Entropy
	// Multi is decision 3's mode switch: in multi mode the tenant comes from the subdomain or
	// the header, in single mode from the installation's only row - one code path, one special
	// case (multi-tenancy.md §1).
	Multi bool
}

// SessionPair is what a successful sign-in, refresh or redemption answers: the two tokens in
// their masking wrapper, their horizons, and the session both hang off.
type SessionPair struct {
	AccessToken      secret.Secret
	AccessExpiresAt  time.Time
	RefreshToken     secret.Secret
	RefreshExpiresAt time.Time
	Session          domain.Session
}

// SignInCommand carries what the sign-in route received. The client fields are hints recorded on
// the session (T-01); the tenant fields are decision 3's sources, resolved before the credential
// is checked.
type SignInCommand struct {
	Email    string
	Password secret.Secret
	// UserAgent and RemoteAddr are recorded on the session, coarsened by the domain.
	UserAgent  string
	RemoteAddr string
	// TenantSlug is the subdomain the request arrived under, when the adapter saw one.
	TenantSlug string
	// TenantHeader is the X-Hubtask-Tenant header, when the caller sent one. It may confirm the
	// resolution, never overrule it.
	TenantHeader string
}

// SignIn is the password sign-in (H-01): email and password in, an access/refresh pair and a
// session row out.
type SignIn struct{ Writer SessionWriter }

// Execute checks the credential and opens the session.
//
// Every credential failure is one generic refusal, byte for byte (T-02): the ledger and the
// metric learn the difference, the caller never does. The tenant is resolved first, because
// accounts are per-tenant and the lookup cannot run without a context (decision 3).
func (h SignIn) Execute(ctx context.Context, cmd SignInCommand) (SessionPair, error) {
	w := h.Writer

	tenantID, err := w.resolveTenant(ctx, cmd.TenantSlug, cmd.TenantHeader)
	if err != nil {
		return SessionPair{}, err
	}

	subjects := attemptSubjects(cmd.Email, cmd.RemoteAddr)
	scope := persistence.Scope{TenantID: tenantID}

	// First transaction: the ledger's standing and the stored hash. The password work happens
	// outside it - Argon2id is deliberately slow, and a connection held through it would let a
	// burst of sign-ins drain the pool (T-02 without a self-inflicted T-17).
	var (
		found     repository.SignInAccount
		accountOK bool
	)
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()
		if err := w.checkLocked(ctx, subjects, now); err != nil {
			return err
		}

		read, err := w.Accounts.FindForSignIn(ctx, cmd.Email)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// Remembered, not answered: the decoy below makes this cost what a real check
				// costs, and the refusal is minted in exactly one place.
				return nil
			}
			return err
		}
		found, accountOK = read, true
		return nil
	})
	if err != nil {
		return SessionPair{}, err
	}

	verified := false
	if accountOK && !found.PasswordHash.IsEmpty() {
		ok, err := w.Passwords.Verify(found.PasswordHash.Reveal(), cmd.Password)
		if err != nil {
			return SessionPair{}, err
		}
		verified = ok
	} else {
		// No account, or an account that signs in some other way: burn the same work, answer
		// the same bytes (T-02).
		w.Passwords.VerifyDecoy(cmd.Password)
	}

	if !verified {
		if err := w.recordFailure(ctx, scope, subjects); err != nil {
			return SessionPair{}, err
		}
		w.failure(ctx, FailureWrongCredential)
		return SessionPair{}, domain.ErrSignInFailed()
	}

	// The password was right; from here the answers may talk about the account, because the
	// caller has proved they are its holder.
	if err := found.Account.Verify(); err != nil {
		return SessionPair{}, err
	}

	pair, err := w.openSession(ctx, scope, tenantID, found.Account, cmd.UserAgent, cmd.RemoteAddr,
		SignedInAction, subjects)
	if err != nil {
		return SessionPair{}, err
	}
	return pair, nil
}

// RefreshSessionCommand carries the exchange's input.
type RefreshSessionCommand struct {
	RefreshToken secret.Secret
	// TenantHeader may confirm the token's tenant, never overrule it.
	TenantHeader string
}

// RefreshSession exchanges a refresh token for the next pair (T-01).
type RefreshSession struct{ Writer SessionWriter }

// Execute rotates. A rotated token presented again means two holders: the family dies, the
// metric and A-15 fire, and the caller learns nothing a probe could use.
func (h RefreshSession) Execute(ctx context.Context, cmd RefreshSessionCommand) (SessionPair, error) {
	w := h.Writer

	token, err := domain.ParseRefreshToken(cmd.RefreshToken.Reveal())
	if err != nil {
		w.failure(ctx, FailureRefreshRefused)
		return SessionPair{}, refreshRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return SessionPair{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}

	material, err := w.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return SessionPair{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}

	var pair SessionPair
	scope := persistence.Scope{TenantID: token.TenantID()}
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		credential, err := w.Refresh.FindByToken(ctx, token)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				w.failure(ctx, FailureRefreshRefused)
				return refreshRefused()
			}
			return err
		}

		now := w.Clock.Now()
		if credential.Token.IsRotated() {
			return w.familyDies(ctx, credential, now)
		}
		if err := credential.Token.Verify(now); err != nil {
			w.failure(ctx, FailureRefreshRefused)
			return err
		}
		if err := credential.Session.Verify(now); err != nil {
			w.failure(ctx, FailureRefreshRefused)
			return err
		}
		if err := credential.Account.Verify(); err != nil {
			return err
		}

		rotated, err := w.Refresh.Rotate(ctx, credential.Token.ID, now)
		if err != nil {
			return err
		}
		if !rotated {
			// Somebody exchanged it between our read and our write. Two holders, same answer.
			return w.familyDies(ctx, credential, now)
		}

		next, presented, err := w.mintRefresh(credential.Session, material, now)
		if err != nil {
			return err
		}
		if err := w.Refresh.Insert(ctx, next, presented); err != nil {
			return err
		}
		session := credential.Session.Rotated(now)
		if err := w.Sessions.Extend(ctx, session.ID, session.ExpiresAt); err != nil {
			return err
		}
		if session.NeedsTouch(now, LastUsedInterval) {
			if err := w.Sessions.TouchLastSeen(ctx, session.ID, now); err != nil {
				return err
			}
			session.LastSeenAt = now
		}

		if err := w.recordSessionAudit(
			ctx, SessionRefreshedAction, audit.OutcomeSuccess, audit.SeverityInfo,
			session, credential.Account, now,
		); err != nil {
			return err
		}

		pair = w.pairFor(session, credential.Account, presented, next, now)
		return nil
	})
	if err != nil {
		return SessionPair{}, err
	}
	return pair, nil
}

// familyDies is reuse detected (T-01): the session - and with it every token of the family - is
// revoked, the event is audited as the security event it is, and the caller gets the same answer
// an unknown token gets.
func (w SessionWriter) familyDies(
	ctx context.Context, credential repository.RefreshCredential, now time.Time,
) error {
	if _, err := w.Sessions.Revoke(
		ctx, credential.Session.ID, credential.Session.AccountID, now,
	); err != nil {
		return err
	}
	if err := w.recordSessionAudit(
		ctx, RefreshReuseAction, audit.OutcomeDenied, audit.SeverityWarning,
		credential.Session.Revoked(now), credential.Account, now,
	); err != nil {
		return err
	}
	w.failure(ctx, FailureRefreshReused)
	return refreshRefused()
}

// openSession is the shared tail of sign-in and redemption: the ledger wiped, the session row,
// the first link of the chain, the audit entry, the pair.
func (w SessionWriter) openSession(
	ctx context.Context, scope persistence.Scope, tenantID shared.ID, account domain.Account,
	userAgent, remoteAddr string, action audit.Action, subjects []string,
) (SessionPair, error) {
	material, err := w.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return SessionPair{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}

	var pair SessionPair
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()

		for _, subject := range subjects {
			if err := w.Attempts.Clear(ctx, subject); err != nil {
				return err
			}
		}

		session, err := domain.NewSession(domain.NewSessionInput{
			ID: w.IDs.NewID(), TenantID: tenantID, AccountID: account.ID,
			UserAgent: userAgent, RemoteAddr: remoteAddr, Now: now,
		})
		if err != nil {
			return err
		}
		if err := w.Sessions.Insert(ctx, session); err != nil {
			return err
		}

		first, presented, err := w.mintRefresh(session, material, now)
		if err != nil {
			return err
		}
		if err := w.Refresh.Insert(ctx, first, presented); err != nil {
			return err
		}

		if err := w.recordSessionAudit(
			ctx, action, audit.OutcomeSuccess, audit.SeverityNotice, session, account, now,
		); err != nil {
			return err
		}

		pair = w.pairFor(session, account, presented, first, now)
		return nil
	})
	if err != nil {
		return SessionPair{}, err
	}
	return pair, nil
}

// mintRefresh builds one link of the chain from drawn material.
func (w SessionWriter) mintRefresh(
	session domain.Session, material []byte, now time.Time,
) (domain.RefreshToken, domain.Token, error) {
	presented, err := domain.NewRefreshToken(session.TenantID, material)
	if err != nil {
		return domain.RefreshToken{}, domain.Token{}, err
	}
	return domain.RefreshToken{
		ID:        w.IDs.NewID(),
		TenantID:  session.TenantID,
		SessionID: session.ID,
		CreatedAt: now.UTC(),
		ExpiresAt: now.Add(domain.RefreshTokenLifetime).UTC(),
	}, presented, nil
}

// pairFor signs the access half and wraps both tokens for the one answer they travel in.
func (w SessionWriter) pairFor(
	session domain.Session, account domain.Account,
	presented domain.Token, refresh domain.RefreshToken, now time.Time,
) SessionPair {
	accessExpiry := now.Add(domain.AccessTokenLifetime).UTC()
	access := w.Signer.Issue(cryptoport.SessionClaims{
		TenantID:  session.TenantID,
		SessionID: session.ID,
		AccountID: account.ID,
		ExpiresAt: accessExpiry,
	})
	return SessionPair{
		AccessToken:      secret.New(access),
		AccessExpiresAt:  accessExpiry,
		RefreshToken:     secret.New(presented.Secret()),
		RefreshExpiresAt: refresh.ExpiresAt,
		Session:          session,
	}
}

// resolveTenant is decision 3, in code: subdomain, then header, and in single mode the only row -
// with `tenant_mismatch` on contradiction and one refusal for "no workspace answers here".
func (w SessionWriter) resolveTenant(ctx context.Context, slug, header string) (shared.ID, error) {
	var tenantID shared.ID

	switch {
	case !w.Multi:
		// Single mode ignores the subdomain: there is one row, and localhost has no slug.
		id, err := w.lookupTenant(ctx, "")
		if err != nil {
			return "", err
		}
		tenantID = id
	case slug != "":
		id, err := w.lookupTenant(ctx, slug)
		if err != nil {
			return "", err
		}
		tenantID = id
	case header != "":
		id, err := shared.ParseID(header)
		if err != nil {
			return "", shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
		}
		tenantID = id
	default:
		return "", shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
	}

	if header != "" && header != tenantID.String() {
		return "", shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	return tenantID, nil
}

func (w SessionWriter) lookupTenant(ctx context.Context, slug string) (shared.ID, error) {
	var tenantID shared.ID
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			id, err := w.Tenants.Resolve(ctx, slug)
			tenantID = id
			return err
		})
	if err != nil {
		return "", err
	}
	return tenantID, nil
}

// checkLocked refuses while any subject's window is open. Nothing is written: the delay curve
// counts failures, not knocks on a closed door.
func (w SessionWriter) checkLocked(ctx context.Context, subjects []string, now time.Time) error {
	var until time.Time
	for _, subject := range subjects {
		attempt, err := w.Attempts.Find(ctx, subject)
		if err != nil {
			return err
		}
		if attempt.LockedUntil.After(until) {
			until = attempt.LockedUntil
		}
	}
	if until.After(now) {
		w.failure(ctx, FailureLocked)
		retry := int(until.Sub(now).Seconds()) + 1
		return shared.ErrRateLimited.
			WithDetail("auth.sign_in_locked").
			WithParams(map[string]string{"retry_after_seconds": strconv.Itoa(retry)})
	}
	return nil
}

// recordFailure advances every subject on the curve, in its own transaction: the refusal must
// land even though the sign-in did not.
func (w SessionWriter) recordFailure(
	ctx context.Context, scope persistence.Scope, subjects []string,
) error {
	return w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()
		for _, subject := range subjects {
			attempt, err := w.Attempts.Find(ctx, subject)
			if err != nil {
				return err
			}
			failures := attempt.Failures + 1
			if err := w.Attempts.Record(ctx, subject, repository.AuthAttempt{
				Failures:      failures,
				LastFailureAt: now.UTC(),
				LockedUntil:   domain.LockedUntil(failures, now),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// attemptSubjects is T-02's "per account and per IP": the address as presented, and the network
// it came from. Both count whether or not an account exists - the ledger stores only hashes, so
// counting a guessed address creates no record of it.
func attemptSubjects(email, remoteAddr string) []string {
	subjects := []string{"account:" + strings.ToLower(strings.TrimSpace(email))}
	if class := domain.IPClass(remoteAddr); class != "" {
		subjects = append(subjects, "ip:"+class)
	}
	return subjects
}

func (w SessionWriter) failure(ctx context.Context, reason string) {
	if w.Signals != nil {
		w.Signals.AuthFailure(ctx, reason)
	}
}

// refreshRefused is the exchange's one probe-facing refusal: unknown, reused and concurrent are
// the same bytes, because which of them applies is exactly what two holders of one token would
// like to know.
func refreshRefused() error {
	return shared.ErrUnauthenticated.WithDetail("auth.refresh_failed")
}

// recordSessionAudit writes the trail entry. Identifiers and moments only: the client hint stays
// on the row, and no token appears anywhere (rule 10).
func (w SessionWriter) recordSessionAudit(
	ctx context.Context, action audit.Action, outcome audit.Outcome, severity audit.Severity,
	session domain.Session, account domain.Account, at time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   session.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    outcome,
		Severity:   severity,
		ActorKind:  appshared.ActorUser,
		ActorID:    account.ID,
		ActorLabel: account.DisplayName,
		TargetType: sessionTarget,
		TargetID:   session.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(audit.Change{
			Field: "account_id", Classification: audit.Open, To: account.ID.String(),
		}),
	})
}

// sessionOutput is the projection every channel gets. `current` is the caller's question -
// whether this row is the one answering - and is stamped by the caller who knows.
func sessionOutput(session domain.Session, currentID shared.ID) usecase.Output {
	out := usecase.Output{
		"id":           session.ID.String(),
		"created_at":   session.CreatedAt.UTC(),
		"last_used_at": nil,
		"user_agent":   nil,
		"ip_class":     nil,
		"current":      !currentID.IsZero() && session.ID == currentID,
	}
	if !session.LastSeenAt.IsZero() {
		out["last_used_at"] = session.LastSeenAt.UTC()
	}
	if session.UserAgent != "" {
		out["user_agent"] = session.UserAgent
	}
	if session.IPClass != "" {
		out["ip_class"] = session.IPClass
	}
	return out
}

// pairOutput is the one answer a credential pair ever travels in.
func pairOutput(pair SessionPair) usecase.Output {
	return usecase.Output{
		"token_type":               "Bearer",
		"access_token":             pair.AccessToken.Reveal(),
		"access_token_expires_at":  pair.AccessExpiresAt.UTC(),
		"refresh_token":            pair.RefreshToken.Reveal(),
		"refresh_token_expires_at": pair.RefreshExpiresAt.UTC(),
		"session":                  sessionOutput(pair.Session, pair.Session.ID),
	}
}

// Descriptor is the catalogue entry.
func (h SignIn) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SignInName,
		Summary: "Signs a person in with email and password and answers an access/refresh pair " +
			"plus the session both hang off. The access token lives fifteen minutes and " +
			"verifies by its signature; the refresh token lives thirty days and rotates on " +
			"every exchange. Every credential failure is one generic refusal - whether an " +
			"account exists is not disclosed - and behind it sit a progressive delay and a " +
			"lockout per account and per source network.",
		SideEffects: "Opens a session, answers two credentials once, writes an audit entry, and " +
			"advances the attempt ledger on failure.",
		Input: []usecase.Field{
			{
				Name: "email", Kind: usecase.KindString, Required: true,
				Description: "The address the account was invited or created under.",
			},
			{
				Name: "password", Kind: usecase.KindString, Required: true,
				Description: "Checked against the stored Argon2id hash, in constant shape.",
			},
			{
				Name: "user_agent", Kind: usecase.KindString,
				Description: "The client as it introduced itself; recorded on the session as a " +
					"hint for recognising one's own devices.",
			},
			{
				Name: "remote_addr", Kind: usecase.KindString,
				Description: "The peer's address as the adapter saw it. Only its network class " +
					"is ever recorded - an IPv4 /24 or an IPv6 /48.",
			},
			{
				Name: "tenant_slug", Kind: usecase.KindString,
				Description: "The subdomain the request arrived under, in multi mode.",
			},
			{
				Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. It may confirm the " +
					"resolution, never overrule it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: SignedInAction, TargetType: sessionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SignIn) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	pair, err := h.Execute(ctx, SignInCommand{
		Email:        in.String("email"),
		Password:     secret.New(in.String("password")),
		UserAgent:    in.String("user_agent"),
		RemoteAddr:   in.String("remote_addr"),
		TenantSlug:   in.String("tenant_slug"),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return pairOutput(pair), nil
}

// Descriptor is the catalogue entry.
func (h RefreshSession) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RefreshSessionName,
		Summary: "Exchanges a refresh token for the next access/refresh pair. Rotation, not " +
			"renewal: the presented token is retired in the same moment, and presenting it " +
			"again afterwards means two holders - the whole family is invalidated, the sign-in " +
			"that opened it is over everywhere, and the reuse raises its own alarm.",
		SideEffects: "Retires the presented token, mints the next pair, slides the session's " +
			"horizon, and writes an audit entry. On reuse it revokes the whole session.",
		Input: []usecase.Field{
			{
				Name: "refresh_token", Kind: usecase.KindString, Required: true,
				Description: "The token being exchanged. It is retired by this very call.",
			},
			{
				Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. It may confirm the " +
					"token's tenant, never overrule it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: SessionRefreshedAction, TargetType: sessionTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A rotation is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RefreshSession) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	pair, err := h.Execute(ctx, RefreshSessionCommand{
		RefreshToken: secret.New(in.String("refresh_token")),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return pairOutput(pair), nil
}
