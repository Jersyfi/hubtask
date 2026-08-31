// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// SessionRepository maintains the sign-in rows and their refresh chains (H-01).
//
// It is the only place that knows how a refresh token becomes a hash, AccessTokenRepository's
// reasoning: the pepper is a secret of this layer, which is why the ports take the presented
// credential whole. No method takes a tenant - row level security bounds every statement to the
// tenant of the running transaction (ADR-0010).
type SessionRepository struct{}

func NewSessionRepository() SessionRepository { return SessionRepository{} }

var _ repository.Sessions = SessionRepository{}

// RefreshTokenRepository maintains the rotating chains. A type of its own because both ports
// insert and find, and one receiver cannot answer two spellings of the same verb.
type RefreshTokenRepository struct {
	hasher security.SessionRefreshHasher
}

func NewRefreshTokenRepository(hasher security.SessionRefreshHasher) RefreshTokenRepository {
	return RefreshTokenRepository{hasher: hasher}
}

var _ repository.RefreshTokens = RefreshTokenRepository{}

func (r SessionRepository) Insert(ctx context.Context, session identity.Session) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(session.ID)
	if err != nil {
		return err
	}
	accountID, err := uuidOf(session.AccountID)
	if err != nil {
		return err
	}
	if err := queries.InsertSession(ctx, sqlc.InsertSessionParams{
		ID:        id,
		AccountID: accountID,
		CreatedAt: pgtype.Timestamptz{Time: session.CreatedAt, Valid: true},
		UserAgent: nullableString(session.UserAgent),
		IpClass:   nullableString(session.IPClass),
		ExpiresAt: pgtype.Timestamptz{Time: session.ExpiresAt, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the session: %w", err))
	}
	return nil
}

func (r SessionRepository) FindForAuth(
	ctx context.Context, sessionID shared.ID,
) (repository.SessionCredential, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.SessionCredential{}, err
	}
	id, err := uuidOf(sessionID)
	if err != nil {
		return repository.SessionCredential{}, err
	}

	row, err := queries.FindSessionForAuth(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return repository.SessionCredential{}, shared.ErrNotFound.WithDetail("auth.session_not_found")
		}
		return repository.SessionCredential{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the session: %w", err))
	}

	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return repository.SessionCredential{}, err
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return repository.SessionCredential{}, err
	}

	return repository.SessionCredential{
		Session: identity.Session{
			ID: sessionID, TenantID: tenantID, AccountID: accountID,
			CreatedAt:  timeFrom(row.CreatedAt),
			LastSeenAt: timeFrom(row.LastSeenAt),
			ExpiresAt:  timeFrom(row.ExpiresAt),
			RevokedAt:  timeFrom(row.RevokedAt),
		},
		Account: identity.Account{
			ID:          accountID,
			TenantID:    tenantID,
			Kind:        identity.AccountKind(row.AccountKind),
			DisplayName: row.AccountDisplayName,
			Status:      identity.AccountStatus(row.AccountStatus),
			Locale:      stringFrom(row.AccountLocale),
			TimeZone:    stringFrom(row.AccountTimeZone),
		},
		TenantLocale:   row.DefaultLocale,
		TenantTimeZone: row.DefaultTimeZone,
	}, nil
}

func (r SessionRepository) ForAccount(
	ctx context.Context, accountID shared.ID, now time.Time,
) ([]identity.Session, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.SessionsForAccount(ctx, sqlc.SessionsForAccountParams{
		AccountID: id,
		Now:       pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the sessions: %w", err))
	}

	sessions := make([]identity.Session, 0, len(rows))
	for _, row := range rows {
		sessionID, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, identity.Session{
			ID: sessionID, AccountID: accountID,
			CreatedAt:  timeFrom(row.CreatedAt),
			LastSeenAt: timeFrom(row.LastSeenAt),
			UserAgent:  stringFrom(row.UserAgent),
			IPClass:    stringFrom(row.IpClass),
			ExpiresAt:  timeFrom(row.ExpiresAt),
			RevokedAt:  timeFrom(row.RevokedAt),
		})
	}
	return sessions, nil
}

func (r SessionRepository) TouchLastSeen(ctx context.Context, sessionID shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(sessionID)
	if err != nil {
		return err
	}
	if err := queries.TouchSession(ctx, sqlc.TouchSessionParams{
		ID:         id,
		LastSeenAt: pgtype.Timestamptz{Time: at, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the session's last use: %w", err))
	}
	return nil
}

func (r SessionRepository) Extend(ctx context.Context, sessionID shared.ID, expiresAt time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(sessionID)
	if err != nil {
		return err
	}
	if err := queries.ExtendSession(ctx, sqlc.ExtendSessionParams{
		ID:        id,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("sliding the session's horizon: %w", err))
	}
	return nil
}

func (r SessionRepository) Revoke(
	ctx context.Context, sessionID, accountID shared.ID, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(sessionID)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.RevokeSession(ctx, sqlc.RevokeSessionParams{
		RevokedAt: pgtype.Timestamptz{Time: at, Valid: true},
		ID:        id,
		AccountID: account,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("revoking the session: %w", err))
	}
	return changed > 0, nil
}

func (r SessionRepository) RevokeAll(
	ctx context.Context, accountID shared.ID, at time.Time,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return 0, err
	}
	changed, err := queries.RevokeAllSessionsForAccount(ctx, sqlc.RevokeAllSessionsForAccountParams{
		RevokedAt: pgtype.Timestamptz{Time: at, Valid: true},
		AccountID: account,
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("revoking the sessions: %w", err))
	}
	return int(changed), nil
}

// Insert writes a freshly minted link of a session's chain. The hash is computed here and
// nowhere else.
func (r RefreshTokenRepository) Insert(
	ctx context.Context, token identity.RefreshToken, presented identity.Token,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(token.ID)
	if err != nil {
		return err
	}
	sessionID, err := uuidOf(token.SessionID)
	if err != nil {
		return err
	}
	if err := queries.InsertRefreshToken(ctx, sqlc.InsertRefreshTokenParams{
		ID:        id,
		SessionID: sessionID,
		TokenHash: r.hasher.Hash(presented.Secret()),
		CreatedAt: pgtype.Timestamptz{Time: token.CreatedAt, Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true},
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the refresh token: %w", err))
	}
	return nil
}

func (r RefreshTokenRepository) FindByToken(
	ctx context.Context, token identity.Token,
) (repository.RefreshCredential, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.RefreshCredential{}, err
	}

	row, err := queries.FindRefreshTokenByHash(ctx, r.hasher.Hash(token.Secret()))
	if err != nil {
		if IsNoRows(err) {
			// Deliberately the same shape as a malformed token by the time it reaches the wire:
			// whether a hash exists is exactly what a probe is trying to learn.
			return repository.RefreshCredential{}, shared.ErrNotFound.WithDetail("auth.refresh_failed")
		}
		return repository.RefreshCredential{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the refresh token: %w", err))
	}

	tokenID, err := idFrom(row.ID)
	if err != nil {
		return repository.RefreshCredential{}, err
	}
	sessionID, err := idFrom(row.SessionID)
	if err != nil {
		return repository.RefreshCredential{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return repository.RefreshCredential{}, err
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return repository.RefreshCredential{}, err
	}

	return repository.RefreshCredential{
		Token: identity.RefreshToken{
			ID: tokenID, TenantID: tenantID, SessionID: sessionID,
			CreatedAt: timeFrom(row.CreatedAt),
			ExpiresAt: timeFrom(row.ExpiresAt),
			RotatedAt: timeFrom(row.RotatedAt),
		},
		Session: identity.Session{
			ID: sessionID, TenantID: tenantID, AccountID: accountID,
			CreatedAt:  timeFrom(row.SessionCreatedAt),
			LastSeenAt: timeFrom(row.SessionLastSeenAt),
			UserAgent:  stringFrom(row.SessionUserAgent),
			IPClass:    stringFrom(row.SessionIpClass),
			ExpiresAt:  timeFrom(row.SessionExpiresAt),
			RevokedAt:  timeFrom(row.SessionRevokedAt),
		},
		Account: identity.Account{
			ID:          accountID,
			TenantID:    tenantID,
			Kind:        identity.AccountKind(row.AccountKind),
			DisplayName: row.AccountDisplayName,
			Status:      identity.AccountStatus(row.AccountStatus),
			Locale:      stringFrom(row.AccountLocale),
			TimeZone:    stringFrom(row.AccountTimeZone),
		},
		TenantLocale:   row.DefaultLocale,
		TenantTimeZone: row.DefaultTimeZone,
	}, nil
}

func (r RefreshTokenRepository) Rotate(ctx context.Context, tokenID shared.ID, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(tokenID)
	if err != nil {
		return false, err
	}
	changed, err := queries.RotateRefreshToken(ctx, sqlc.RotateRefreshTokenParams{
		RotatedAt: pgtype.Timestamptz{Time: at, Valid: true},
		ID:        id,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("rotating the refresh token: %w", err))
	}
	return changed > 0, nil
}

// SignInRepository is the account surface of the sign-in flow: the credential check's read, the
// attempt ledger, the invitation redeemed, and decision 3's tenant resolution.
type SignInRepository struct {
	redemptionHasher security.RedemptionTokenHasher
	attemptHasher    security.AuthAttemptHasher
}

func NewSignInRepository(
	redemptionHasher security.RedemptionTokenHasher,
	attemptHasher security.AuthAttemptHasher,
) SignInRepository {
	return SignInRepository{redemptionHasher: redemptionHasher, attemptHasher: attemptHasher}
}

var (
	_ repository.SignInAccounts  = SignInRepository{}
	_ repository.AuthAttempts    = SignInRepository{}
	_ repository.TenantDirectory = SignInRepository{}
)

func (r SignInRepository) FindForSignIn(
	ctx context.Context, email string,
) (repository.SignInAccount, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.SignInAccount{}, err
	}

	row, err := queries.FindAccountForSignIn(ctx, email)
	if err != nil {
		if IsNoRows(err) {
			return repository.SignInAccount{}, shared.ErrNotFound.WithDetail("accounts.not_found")
		}
		return repository.SignInAccount{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the account: %w", err))
	}

	accountID, err := idFrom(row.ID)
	if err != nil {
		return repository.SignInAccount{}, err
	}

	return repository.SignInAccount{
		Account: identity.Account{
			ID:          accountID,
			Kind:        identity.AccountKind(row.Kind),
			Email:       stringFrom(row.Email),
			DisplayName: row.DisplayName,
			Status:      identity.AccountStatus(row.Status),
			Locale:      stringFrom(row.AccountLocale),
			TimeZone:    stringFrom(row.AccountTimeZone),
		},
		PasswordHash:   secret.New(stringFrom(row.PasswordHash)),
		TenantLocale:   row.DefaultLocale,
		TenantTimeZone: row.DefaultTimeZone,
	}, nil
}

func (r SignInRepository) SetRedemptionToken(
	ctx context.Context, accountID shared.ID, presented identity.Token,
	expiresAt, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.SetRedemptionToken(ctx, sqlc.SetRedemptionTokenParams{
		TokenHash: r.redemptionHasher.Hash(presented.Secret()),
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
		Now:       pgtype.Timestamptz{Time: now, Valid: true},
		ID:        id,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("storing the redemption token: %w", err))
	}
	return changed > 0, nil
}

func (r SignInRepository) FindByRedemptionToken(
	ctx context.Context, token identity.Token,
) (repository.RedemptionAccount, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.RedemptionAccount{}, err
	}

	row, err := queries.FindAccountByRedemptionHash(ctx, r.redemptionHasher.Hash(token.Secret()))
	if err != nil {
		if IsNoRows(err) {
			return repository.RedemptionAccount{}, shared.ErrNotFound.WithDetail("auth.redemption_failed")
		}
		return repository.RedemptionAccount{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the invited account: %w", err))
	}

	accountID, err := idFrom(row.ID)
	if err != nil {
		return repository.RedemptionAccount{}, err
	}

	return repository.RedemptionAccount{
		Account: identity.Account{
			ID:          accountID,
			Kind:        identity.AccountKind(row.Kind),
			Email:       stringFrom(row.Email),
			DisplayName: row.DisplayName,
			Status:      identity.AccountStatus(row.Status),
			Locale:      stringFrom(row.AccountLocale),
			TimeZone:    stringFrom(row.AccountTimeZone),
		},
		ExpiresAt:      timeFrom(row.RedemptionExpiresAt),
		TenantLocale:   row.DefaultLocale,
		TenantTimeZone: row.DefaultTimeZone,
	}, nil
}

func (r SignInRepository) Redeem(
	ctx context.Context, accountID shared.ID, passwordHash string, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	changed, err := queries.RedeemInvitation(ctx, sqlc.RedeemInvitationParams{
		PasswordHash: &passwordHash,
		Now:          pgtype.Timestamptz{Time: now, Valid: true},
		ID:           id,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("redeeming the invitation: %w", err))
	}
	return changed > 0, nil
}

func (r SignInRepository) Find(ctx context.Context, subject string) (repository.AuthAttempt, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.AuthAttempt{}, err
	}

	row, err := queries.FindAuthAttempt(ctx, r.attemptHasher.Hash(subject))
	if err != nil {
		if IsNoRows(err) {
			return repository.AuthAttempt{}, nil
		}
		return repository.AuthAttempt{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the attempt ledger: %w", err))
	}
	return repository.AuthAttempt{
		Failures:      int(row.Failures),
		LastFailureAt: timeFrom(row.LastFailureAt),
		LockedUntil:   timeFrom(row.LockedUntil),
	}, nil
}

func (r SignInRepository) Record(
	ctx context.Context, subject string, attempt repository.AuthAttempt,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	lockedUntil := pgtype.Timestamptz{}
	if !attempt.LockedUntil.IsZero() {
		lockedUntil = pgtype.Timestamptz{Time: attempt.LockedUntil, Valid: true}
	}
	if err := queries.UpsertAuthAttempt(ctx, sqlc.UpsertAuthAttemptParams{
		SubjectHash:   r.attemptHasher.Hash(subject),
		Failures:      int32(attempt.Failures), //nolint:gosec // G115: the curve caps far below the bound
		LastFailureAt: pgtype.Timestamptz{Time: attempt.LastFailureAt, Valid: true},
		LockedUntil:   lockedUntil,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the attempt ledger: %w", err))
	}
	return nil
}

func (r SignInRepository) Clear(ctx context.Context, subject string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if err := queries.ClearAuthAttempt(ctx, r.attemptHasher.Hash(subject)); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the attempt ledger: %w", err))
	}
	return nil
}

// Resolve maps a slug - or, empty, the single-mode installation's only row - to its tenant. It
// runs through the narrow SECURITY DEFINER function of migration 0063 inside an
// installation-scoped read-only transaction: the caller has no tenant yet, which is the whole
// point.
func (r SignInRepository) Resolve(ctx context.Context, slug string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}

	value, err := queries.ResolveTenant(ctx, nullableString(slug))
	if err != nil {
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("resolving the tenant: %w", err))
	}
	if !value.Valid {
		return "", shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
	}
	return idFrom(value)
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// DeleteExpired removes one batch of sessions that are over (H-01, the SESSION data kind). The
// query's own guard only ever matches a session that has run out or was revoked - ending
// sign-ins is revocation's job, and the sweep only forgets.
func (r SessionRepository) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	removed, err := queries.DeleteExpiredSessions(ctx, sqlc.DeleteExpiredSessionsParams{
		Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
		Batch:  int32(batch), //nolint:gosec // G115: the batch size is a small configuration value
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("sweeping the sessions: %w", err))
	}
	return int(removed), nil
}

// CountExpired reports what is due, bounded by the ceiling.
func (r SessionRepository) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	due, err := queries.CountExpiredSessions(ctx, sqlc.CountExpiredSessionsParams{
		Cutoff:  pgtype.Timestamptz{Time: cutoff, Valid: true},
		Ceiling: int32(ceiling), //nolint:gosec // G115: the ceiling is a small configuration value
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the due sessions: %w", err))
	}
	return int(due), nil
}
