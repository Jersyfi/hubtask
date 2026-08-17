// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant  = shared.ID("018f2a1b-0000-7000-8000-0000000000ab")
	account = shared.ID("018f2a1b-0000-7000-8000-0000000000cd")
	tokenID = shared.ID("018f2a1b-0000-7000-8000-0000000000ef")
	now     = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
)

// unitOfWork is a fake that records the scope it was opened with - which is the assertion that
// matters here: authentication must run in the tenant the token names and in no other.
type unitOfWork struct {
	scopes []persistence.Scope
}

func (u *unitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, scope, fn)
}

type tokens struct {
	credential repository.Credential
	findErr    error

	touched   shared.ID
	touchedAt time.Time
	touchErr  error
}

func (f *tokens) FindByToken(context.Context, identity.Token) (repository.Credential, error) {
	return f.credential, f.findErr
}

func (f *tokens) TouchLastUsed(_ context.Context, id shared.ID, at time.Time) error {
	f.touched, f.touchedAt = id, at
	return f.touchErr
}

func mintCredential(t *testing.T) (string, repository.Credential) {
	t.Helper()
	secret := make([]byte, identity.TokenSecretBytes)
	for i := range secret {
		secret[i] = byte(i)
	}
	token, err := identity.NewToken(tenant, secret)
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}

	return token.Secret(), repository.Credential{
		Token: identity.AccessToken{
			ID:         tokenID,
			TenantID:   tenant,
			AccountID:  account,
			Scopes:     []string{"items:read", "items:write"},
			ExpiresAt:  now.Add(24 * time.Hour),
			LastUsedAt: now.Add(-time.Minute),
		},
		Account: identity.Account{
			ID: account, Kind: identity.AccountUser, Status: identity.AccountActive,
		},
		TenantLocale:   "de",
		TenantTimeZone: "Europe/Berlin",
	}
}

func handlerFor(store *tokens, uow *unitOfWork) AuthenticateToken {
	return AuthenticateToken{Tokens: store, UnitOfWork: uow, Clock: clock.Fixed(now)}
}

func TestAValidTokenBecomesAnActor(t *testing.T) {
	raw, credential := mintCredential(t)
	store := &tokens{credential: credential}
	uow := &unitOfWork{}

	actor, err := handlerFor(store, uow).Execute(t.Context(), AuthenticateTokenCommand{
		Credential: raw, FallbackLocale: "en", FallbackTimeZone: "UTC",
	})
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	if !actor.IsAuthenticated() {
		t.Error("the actor is not authenticated")
	}
	if actor.TenantID != tenant || actor.AccountID != account || actor.TokenID != tokenID {
		t.Errorf("actor = %+v", actor)
	}
	if actor.Kind != appshared.ActorUser {
		t.Errorf("kind = %q", actor.Kind)
	}
	if !actor.HasScope("items:write") {
		t.Errorf("scopes = %v", actor.Scopes)
	}
}

// The tenant of the transaction comes from the token, never from anywhere else
// (multi-tenancy.md §2.2).
func TestTheLookupRunsInTheTokensTenant(t *testing.T) {
	raw, credential := mintCredential(t)
	uow := &unitOfWork{}

	if _, err := handlerFor(&tokens{credential: credential}, uow).
		Execute(t.Context(), AuthenticateTokenCommand{Credential: raw}); err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	if len(uow.scopes) != 1 {
		t.Fatalf("%d transactions opened", len(uow.scopes))
	}
	if uow.scopes[0].TenantID != tenant {
		t.Errorf("scope = %+v, want tenant %q", uow.scopes[0], tenant)
	}
	if !uow.scopes[0].ActorID.IsZero() {
		t.Error("the lookup claimed an actor it had not yet identified")
	}
}

func TestTheLocaleChainPrefersTheRequest(t *testing.T) {
	raw, credential := mintCredential(t)
	credential.Account.Locale = "fr"
	credential.Account.TimeZone = "Europe/Paris"

	cases := []struct {
		name             string
		requested        string
		accountLocale    string
		wantLocale       string
		wantTimeZoneFrom string
	}{
		{"the request wins", "pt-BR", "fr", "pt-BR", "Europe/Paris"},
		{"then the account", "", "fr", "fr", "Europe/Paris"},
		{"then the tenant", "", "", "de", "Europe/Paris"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			credential.Account.Locale = c.accountLocale
			store := &tokens{credential: credential}

			actor, err := handlerFor(store, &unitOfWork{}).Execute(t.Context(), AuthenticateTokenCommand{
				Credential: raw, RequestedLocale: c.requested,
				FallbackLocale: "en", FallbackTimeZone: "UTC",
			})
			if err != nil {
				t.Fatalf("authentication failed: %v", err)
			}
			if actor.Locale != c.wantLocale {
				t.Errorf("locale = %q, want %q", actor.Locale, c.wantLocale)
			}
			if actor.TimeZone != c.wantTimeZoneFrom {
				t.Errorf("time zone = %q, want %q", actor.TimeZone, c.wantTimeZoneFrom)
			}
		})
	}
}

func TestTheInstallationDefaultIsTheLastResort(t *testing.T) {
	raw, credential := mintCredential(t)
	credential.TenantLocale, credential.TenantTimeZone = "", ""
	store := &tokens{credential: credential}

	actor, err := handlerFor(store, &unitOfWork{}).Execute(t.Context(), AuthenticateTokenCommand{
		Credential: raw, FallbackLocale: "en", FallbackTimeZone: "UTC",
	})
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}
	if actor.Locale != "en" || actor.TimeZone != "UTC" {
		t.Errorf("locale %q, time zone %q", actor.Locale, actor.TimeZone)
	}
}

func TestARefusedCredential(t *testing.T) {
	raw, valid := mintCredential(t)

	cases := []struct {
		name       string
		credential string
		store      *tokens
		wantIs     error
		wantDetail string
	}{
		{
			name:       "misshapen",
			credential: "not-a-token",
			store:      &tokens{credential: valid},
			wantIs:     shared.ErrUnauthenticated,
			wantDetail: "access.token_malformed",
		},
		{
			name:       "unknown",
			credential: raw,
			store:      &tokens{findErr: shared.ErrNotFound},
			wantIs:     shared.ErrUnauthenticated,
			wantDetail: "access.token_unknown",
		},
		{
			name:       "expired",
			credential: raw,
			store:      withToken(valid, func(a *identity.AccessToken) { a.ExpiresAt = now.Add(-time.Hour) }),
			wantIs:     shared.ErrUnauthenticated,
			wantDetail: "access.token_expired",
		},
		{
			name:       "revoked",
			credential: raw,
			store:      withToken(valid, func(a *identity.AccessToken) { a.RevokedAt = now.Add(-time.Hour) }),
			wantIs:     shared.ErrUnauthenticated,
			wantDetail: "access.token_revoked",
		},
		{
			name:       "belonging to a disabled account",
			credential: raw,
			store:      withAccount(valid, func(a *identity.Account) { a.Status = identity.AccountDisabled }),
			wantIs:     shared.ErrForbidden,
			wantDetail: "access.account_not_active",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actor, err := handlerFor(c.store, &unitOfWork{}).
				Execute(t.Context(), AuthenticateTokenCommand{Credential: c.credential})

			if !errors.Is(err, c.wantIs) {
				t.Fatalf("error = %v, want %v", err, c.wantIs)
			}
			var domainErr *shared.Error
			if errors.As(err, &domainErr) && domainErr.DetailCode != c.wantDetail {
				t.Errorf("detail = %q, want %q", domainErr.DetailCode, c.wantDetail)
			}
			if actor.IsAuthenticated() {
				t.Error("a refused credential produced an authenticated actor")
			}
		})
	}
}

func TestTheLastUseIsWrittenBackAtMostOncePerInterval(t *testing.T) {
	raw, credential := mintCredential(t)

	t.Run("recently used, no write", func(t *testing.T) {
		store := &tokens{credential: credential}
		if _, err := handlerFor(store, &unitOfWork{}).
			Execute(t.Context(), AuthenticateTokenCommand{Credential: raw}); err != nil {
			t.Fatalf("authentication failed: %v", err)
		}
		if !store.touched.IsZero() {
			t.Error("a read path turned into a write path")
		}
	})

	t.Run("stale, written back", func(t *testing.T) {
		stale := credential
		stale.Token.LastUsedAt = now.Add(-time.Hour)
		store := &tokens{credential: stale}

		if _, err := handlerFor(store, &unitOfWork{}).
			Execute(t.Context(), AuthenticateTokenCommand{Credential: raw}); err != nil {
			t.Fatalf("authentication failed: %v", err)
		}
		if store.touched != tokenID || !store.touchedAt.Equal(now) {
			t.Errorf("touched %q at %v", store.touched, store.touchedAt)
		}
	})
}

// Bookkeeping must not cost the request. A failed last-use write is a warning, not a 500
// (ADR-0016).
func TestAFailedLastUseWriteDoesNotFailTheRequest(t *testing.T) {
	raw, credential := mintCredential(t)
	credential.Token.LastUsedAt = now.Add(-time.Hour)
	store := &tokens{credential: credential, touchErr: errors.New("write failed")}

	actor, err := handlerFor(store, &unitOfWork{}).
		Execute(t.Context(), AuthenticateTokenCommand{Credential: raw})
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}
	if !actor.IsAuthenticated() {
		t.Error("the actor is not authenticated")
	}
}

// A lookup that fails for a technical reason must not read as "no such token": one is a database
// that is unreachable, the other is a credential that is wrong.
func TestATechnicalFailureIsNotAnAuthenticationFailure(t *testing.T) {
	raw, _ := mintCredential(t)
	store := &tokens{findErr: shared.ErrUnavailable}

	_, err := handlerFor(store, &unitOfWork{}).
		Execute(t.Context(), AuthenticateTokenCommand{Credential: raw})

	if !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("error = %v, want the technical failure to survive", err)
	}
}

func withToken(base repository.Credential, apply func(*identity.AccessToken)) *tokens {
	apply(&base.Token)
	return &tokens{credential: base}
}

func withAccount(base repository.Credential, apply func(*identity.Account)) *tokens {
	apply(&base.Account)
	return &tokens{credential: base}
}
