// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"strings"

	usecase "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// PublicRoutes are the operations api/openapi.yaml declares with `security: []`. Everything else
// needs a credential.
//
// The list is here rather than derived at run time because the generated code does not carry the
// security requirement - and a list nobody checks would rot, so the contract test compares it
// against the specification and fails when the two disagree. Fail closed either way: a route
// missing from this list is authenticated, never the reverse.
var PublicRoutes = map[string]bool{
	http.MethodGet + " " + APIBasePath + "/meta/capabilities": true,
	// The content routes carry their credential in the URL: a signed, expiring token minted by
	// requestMediaUpload and getMedia, validated by the route itself - the same trust model as a
	// presigned object-storage URL, which is what these stand in for on a local-storage
	// installation (C-06, T-11).
	http.MethodPut + " " + APIBasePath + "/media/{mediaId}:content": true,
	http.MethodGet + " " + APIBasePath + "/media/{mediaId}:content": true,
	// The calendar feed carries its credential in the URL for the same reason and with the same
	// trust model, and for one more: a calendar client is not a browser and has nowhere to put a
	// bearer header. The token is the whole of the authorisation, and the route validates it
	// itself (D-08, security.md §4 T-21).
	http.MethodGet + " " + APIBasePath + "/calendar/{token}.ics": true,
	// The inbound webhook carries its credential in the URL for the same reasons, and it
	// authenticates the *rule* rather than a person: there is no account behind the token, so
	// there is nothing for this middleware to resolve. The route validates it itself, and what
	// the run may then do is its `run_as` account's business (G-08, automation.md §1.1).
	http.MethodPost + " " + APIBasePath + "/automation/inbound/{token}": true,
	// The jumble's intake carries its credential in the URL with the same trust model, and it
	// authenticates the *tenant* rather than a person: there is no account behind the token, and
	// the entry it stores records no actor (G-10).
	http.MethodPost + " " + APIBasePath + "/jumble/inbound/{token}": true,
}

// bearerScheme is compared case-insensitively, as RFC 9110 §11.1 requires of an auth scheme.
const bearerScheme = "bearer"

// TokenAuthenticator is the slice of the authentication use case this middleware needs. An
// interface rather than the handler, so that the middleware can be tested without a database and
// the presentation layer keeps pointing inwards.
type TokenAuthenticator interface {
	Execute(context.Context, usecase.AuthenticateTokenCommand) (appshared.ActorContext, error)
}

// Authenticated turns a presented credential into the actor of the request.
//
// It authenticates and stops there. Whether the actor may perform the operation is decided by the
// use case behind the route, in the application layer and nowhere else (ADR-0005, CLAUDE.md
// rule 2) - this middleware never reads a scope and never denies an operation.
type Authenticated struct {
	Next http.Handler
	// Routes resolves the route template, which is what decides whether a credential is required.
	Routes Router
	// Authenticator is the use case. Nil is not a valid configuration; the composition root wires
	// it (cmd/server).
	Authenticator TokenAuthenticator
	// Locale carries the installation defaults, the last link of the resolution chain.
	Locale env.LocaleConfig
}

func (a Authenticated) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	credential, err := bearerCredential(r)
	if err != nil {
		WriteUnauthenticated(w, err, requestID)
		return
	}

	if credential == "" {
		// No credential at all. A public route is served anonymously; everything else is refused
		// here rather than by the handler, because a handler that has to remember is one that
		// will forget.
		if _, route := a.Routes.Handler(r); PublicRoutes[route] {
			a.Next.ServeHTTP(w, r)
			return
		}
		WriteUnauthenticated(w, shared.ErrUnauthenticated.WithDetail("access.credential_required"), requestID)
		return
	}

	// A credential that was presented is always verified, even on a public route: answering a
	// wrong token as though it were anonymous would hide a revoked or mistyped credential from
	// whoever is holding it.
	actor, err := a.Authenticator.Execute(r.Context(), usecase.AuthenticateTokenCommand{
		Credential:       credential,
		RequestedLocale:  requestedLocaleFrom(r.Context()),
		FallbackLocale:   a.Locale.DefaultLocale,
		FallbackTimeZone: a.Locale.DefaultTimeZone,
	})
	if err != nil {
		WriteUnauthenticated(w, err, requestID)
		return
	}

	// The header is the third source of tenant resolution and the weakest: it may confirm the
	// token, never overrule it (multi-tenancy.md §3).
	if claimed := r.Header.Get(TenantHeader); claimed != "" && claimed != actor.TenantID.String() {
		WriteProblem(w, shared.ErrForbidden.WithDetail("access.tenant_mismatch"), requestID)
		return
	}

	ctx := appshared.ContextWithActor(r.Context(), actor)
	// The tenant reaches the log lines and the metric label from here (§3.1). An identifier of an
	// installation, never user content (rule 10).
	ctx = correlation.ContextWithTenant(ctx, actor.TenantID.String())

	a.Next.ServeHTTP(w, r.WithContext(ctx))
}

// bearerCredential reads the Authorization header. An absent header is not an error - that is an
// anonymous request, and only the route decides whether it is allowed. A malformed one is.
func bearerCredential(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", nil
	}

	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, bearerScheme) {
		return "", shared.ErrUnauthenticated.WithDetail("access.scheme_unsupported")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", shared.ErrUnauthenticated.WithDetail("access.token_malformed")
	}
	return value, nil
}

// WriteUnauthenticated answers a refused credential. A 401 without WWW-Authenticate is
// incomplete (RFC 9110 §11.6.1): the client is told nothing about how to authenticate.
//
// A 403 travels through here too - a valid token belonging to a disabled account is not a reason
// to ask for the credential again - which is why the header is set from the mapped status rather
// than assumed.
func WriteUnauthenticated(w http.ResponseWriter, err error, requestID string) {
	problem := ProblemFrom(err, requestID)
	if problem.Status == http.StatusUnauthorized {
		// The scheme only. RFC 6750's error parameter would tell an attacker which of "unknown",
		// "expired" and "revoked" applies before they have a valid token; the body says it, and
		// the body reaches the holder of the token rather than a probe.
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	writeProblem(w, problem)
}
