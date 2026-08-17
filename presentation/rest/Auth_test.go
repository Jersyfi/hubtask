// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	usecase "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

const (
	tenantID   = shared.ID("018f2a1b-0000-7000-8000-0000000000ab")
	credential = "hbt_pat_018f2a1b000070008000000000000ab0_" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
)

// authenticator is the use case as this middleware sees it.
type authenticator struct {
	actor appshared.ActorContext
	err   error

	command usecase.AuthenticateTokenCommand
	calls   int
}

func (a *authenticator) Execute(
	_ context.Context,
	cmd usecase.AuthenticateTokenCommand,
) (appshared.ActorContext, error) {
	a.calls++
	a.command = cmd
	return a.actor, a.err
}

func authenticatedActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID,
		Scopes: []string{"items:read"}, Locale: "de", TimeZone: "Europe/Berlin",
	}
}

// serveAuthenticated runs a request through the middleware against a router that knows one public
// and one private route, and reports what the handler behind it saw.
func serveAuthenticated(
	t *testing.T,
	auth *authenticator,
	request *http.Request,
) (*httptest.ResponseRecorder, appshared.ActorContext, bool) {
	t.Helper()

	routes := NewMux()
	routes.HandleFunc(http.MethodGet+" "+APIBasePath+"/meta/capabilities", func(http.ResponseWriter, *http.Request) {})
	routes.HandleFunc(http.MethodGet+" "+APIBasePath+"/containers", func(http.ResponseWriter, *http.Request) {})

	var seen appshared.ActorContext
	reached := false

	response := httptest.NewRecorder()
	Authenticated{
		Routes:        routes,
		Authenticator: auth,
		Locale:        env.LocaleConfig{DefaultLocale: "en", DefaultTimeZone: "UTC"},
		Next: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			reached = true
			seen, _ = appshared.ActorFrom(r.Context())
		}),
	}.ServeHTTP(response, request)

	return response, seen, reached
}

func request(t *testing.T, path string) *http.Request {
	t.Helper()
	return httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+path, nil)
}

func TestAPublicRouteIsServedWithoutACredential(t *testing.T) {
	auth := &authenticator{}

	response, actor, reached := serveAuthenticated(t, auth, request(t, "/meta/capabilities"))

	if !reached {
		t.Fatalf("the handler was not reached, status %d", response.Code)
	}
	if auth.calls != 0 {
		t.Error("a request without a credential reached the authentication use case")
	}
	if actor.IsAuthenticated() {
		t.Error("an anonymous request produced an authenticated actor")
	}
}

// Fail closed: everything the specification does not mark public needs a credential, and the
// refusal happens here rather than in a handler that has to remember.
func TestAPrivateRouteWithoutACredentialIs401(t *testing.T) {
	response, _, reached := serveAuthenticated(t, &authenticator{}, request(t, "/containers"))

	if reached {
		t.Error("the handler ran for an unauthenticated request")
	}
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", response.Code)
	}
	if got := response.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q - a 401 without it tells the client nothing", got)
	}

	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.DetailCode != "access.credential_required" {
		t.Errorf("detail code %q", problem.DetailCode)
	}
}

func TestAValidCredentialBecomesTheActor(t *testing.T) {
	auth := &authenticator{actor: authenticatedActor()}
	r := request(t, "/containers")
	r.Header.Set("Authorization", "Bearer "+credential)

	response, actor, reached := serveAuthenticated(t, auth, r)

	if !reached {
		t.Fatalf("the handler was not reached, status %d", response.Code)
	}
	if !actor.IsAuthenticated() || actor.TenantID != tenantID {
		t.Errorf("actor = %+v", actor)
	}
	if auth.command.Credential != credential {
		t.Errorf("the use case saw %q", auth.command.Credential)
	}
	if auth.command.FallbackLocale != "en" || auth.command.FallbackTimeZone != "UTC" {
		t.Errorf("installation defaults did not reach the use case: %+v", auth.command)
	}
}

// The scheme is compared case-insensitively (RFC 9110 §11.1), and anything else is refused rather
// than treated as anonymous.
func TestTheAuthorizationHeader(t *testing.T) {
	cases := []struct {
		name       string
		header     string
		wantStatus int
		wantDetail string
	}{
		{"lower case scheme", "bearer " + credential, http.StatusOK, ""},
		{"mixed case scheme", "BeArEr " + credential, http.StatusOK, ""},
		{"another scheme", "Basic dXNlcjpwYXNz", http.StatusUnauthorized, "access.scheme_unsupported"},
		{"no scheme", credential, http.StatusUnauthorized, "access.scheme_unsupported"},
		{"an empty credential", "Bearer ", http.StatusUnauthorized, "access.token_malformed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := request(t, "/containers")
			r.Header.Set("Authorization", c.header)

			response, _, _ := serveAuthenticated(t, &authenticator{actor: authenticatedActor()}, r)

			if response.Code != c.wantStatus {
				t.Fatalf("status %d, want %d", response.Code, c.wantStatus)
			}
			if c.wantDetail == "" {
				return
			}
			var problem Problem
			if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
				t.Fatalf("the body is not a problem document: %v", err)
			}
			if problem.DetailCode != c.wantDetail {
				t.Errorf("detail code %q, want %q", problem.DetailCode, c.wantDetail)
			}
		})
	}
}

// A wrong token on a public route must not pass silently as anonymous: the holder of a revoked or
// mistyped credential has to be told.
func TestAPresentedCredentialIsVerifiedEvenOnAPublicRoute(t *testing.T) {
	auth := &authenticator{err: shared.ErrUnauthenticated.WithDetail("access.token_revoked")}
	r := request(t, "/meta/capabilities")
	r.Header.Set("Authorization", "Bearer "+credential)

	response, _, reached := serveAuthenticated(t, auth, r)

	if reached {
		t.Error("a revoked token was served as an anonymous request")
	}
	if response.Code != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", response.Code)
	}
	if auth.calls != 1 {
		t.Errorf("the use case was called %d times", auth.calls)
	}
}

// A valid credential belonging to an account that may not act is 403, and a 403 must not carry
// WWW-Authenticate - authenticating again would send the client round a loop it cannot leave.
func TestADisabledAccountIs403WithoutAChallenge(t *testing.T) {
	auth := &authenticator{err: shared.ErrForbidden.WithDetail("access.account_not_active")}
	r := request(t, "/containers")
	r.Header.Set("Authorization", "Bearer "+credential)

	response, _, _ := serveAuthenticated(t, auth, r)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", response.Code)
	}
	if got := response.Header().Get("WWW-Authenticate"); got != "" {
		t.Errorf("WWW-Authenticate = %q on a 403", got)
	}
}

// The header may confirm the tenant, never overrule it (multi-tenancy.md §3).
func TestATenantHeaderContradictingTheTokenIsRefused(t *testing.T) {
	auth := &authenticator{actor: authenticatedActor()}
	r := request(t, "/containers")
	r.Header.Set("Authorization", "Bearer "+credential)
	r.Header.Set(TenantHeader, "018f2a1b-0000-7000-8000-0000000000ff")

	response, _, reached := serveAuthenticated(t, auth, r)

	if reached {
		t.Error("the handler ran with a contradicted tenant")
	}
	if response.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", response.Code)
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.DetailCode != "access.tenant_mismatch" {
		t.Errorf("detail code %q", problem.DetailCode)
	}
}

func TestATenantHeaderAgreeingWithTheTokenPassesThrough(t *testing.T) {
	auth := &authenticator{actor: authenticatedActor()}
	r := request(t, "/containers")
	r.Header.Set("Authorization", "Bearer "+credential)
	r.Header.Set(TenantHeader, tenantID.String())

	response, _, reached := serveAuthenticated(t, auth, r)

	if !reached {
		t.Errorf("the handler was not reached, status %d", response.Code)
	}
}

// The chain in full: what the client asked for reaches the use case, so it can win over the
// account's and the tenant's preference (i18n-l10n.md §2).
func TestTheRequestedLocaleReachesTheUseCase(t *testing.T) {
	auth := &authenticator{actor: authenticatedActor()}

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/containers", nil)
	r.Header.Set("Authorization", "Bearer "+credential)
	r.Header.Set("Accept-Language", "pt-BR")

	routes := NewMux()
	routes.HandleFunc(http.MethodGet+" "+APIBasePath+"/containers", func(http.ResponseWriter, *http.Request) {})

	Localised{
		Locale: env.LocaleConfig{DefaultLocale: "en", DefaultTimeZone: "UTC"},
		Next: Authenticated{
			Routes:        routes,
			Authenticator: auth,
			Locale:        env.LocaleConfig{DefaultLocale: "en", DefaultTimeZone: "UTC"},
			Next:          http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		},
	}.ServeHTTP(httptest.NewRecorder(), r)

	if auth.command.RequestedLocale != "pt-BR" {
		t.Errorf("requested locale = %q", auth.command.RequestedLocale)
	}
}

// Without an Accept-Language header the use case must see no request preference at all -
// otherwise the installation default would silently outrank the account's own language.
func TestNoAcceptLanguageMeansNoRequestPreference(t *testing.T) {
	auth := &authenticator{actor: authenticatedActor()}

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/containers", nil)
	r.Header.Set("Authorization", "Bearer "+credential)

	routes := NewMux()
	routes.HandleFunc(http.MethodGet+" "+APIBasePath+"/containers", func(http.ResponseWriter, *http.Request) {})

	Localised{
		Locale: env.LocaleConfig{DefaultLocale: "en", DefaultTimeZone: "UTC"},
		Next: Authenticated{
			Routes:        routes,
			Authenticator: auth,
			Locale:        env.LocaleConfig{DefaultLocale: "en", DefaultTimeZone: "UTC"},
			Next:          http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		},
	}.ServeHTTP(httptest.NewRecorder(), r)

	if auth.command.RequestedLocale != "" {
		t.Errorf("requested locale = %q, want empty", auth.command.RequestedLocale)
	}
}
