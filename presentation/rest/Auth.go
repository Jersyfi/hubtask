// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net"
	"net/http"
	"strings"

	usecase "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/calendar"
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
	// Sign-in, refresh and redemption are how a credential is obtained, so there is none to
	// demand yet: the password, the refresh token and the redemption token travel in the body and
	// are the whole of what authenticates the call (H-01, security.md §5). Each route verifies
	// its own credential; the auth rate-limit bucket stands in front of all three.
	http.MethodPost + " " + APIBasePath + "/auth/sessions":           true,
	http.MethodPost + " " + APIBasePath + "/auth/sessions:refresh":   true,
	http.MethodPost + " " + APIBasePath + "/auth/invitations:redeem": true,
	// The second step and the enrolment routes are public for the same reason (H-02): the
	// pending credential in the body is the whole of what authenticates an enforcement flow,
	// and a signed-in caller's bearer is verified exactly as on any public route.
	http.MethodPost + " " + APIBasePath + "/auth/sessions:verify":  true,
	http.MethodPost + " " + APIBasePath + "/auth/mfa/totp:enroll":  true,
	http.MethodPost + " " + APIBasePath + "/auth/mfa/totp:confirm": true,
	// The relying-party flow is public for the same reason again (H-04): a person signing in
	// through their company's provider has no credential here yet, and the flow's own handle -
	// the single-use `state`, minted by :start and spent at :callback - is the whole of what
	// authenticates the second call. The auth bucket stands in front of both.
	http.MethodPost + " " + APIBasePath + "/auth/oidc:start":    true,
	http.MethodPost + " " + APIBasePath + "/auth/oidc:callback": true,
	// The token endpoint is public the way sign-in is (H-05): the single-use code, the PKCE
	// verifier and - for a confidential client - the secret travel in the body and are the
	// whole of what authenticates the exchange.
	http.MethodPost + " " + APIBasePath + "/oauth/token": true,
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
	// The mail door, on the same credential and the same trust model (G-11). What arrives here is
	// a message somebody else's bridge forwarded, and the token is the whole of what says it may.
	http.MethodPost + " " + APIBasePath + "/jumble/mail/{token}": true,
	// The CalDAV discovery address is outside the contract - it is the mount's own path, as
	// Mounted labels it - and public because RFC 6764 §5 has a client ask it before it has
	// presented anything: the answer is a redirect into the tree, which asks for the credential
	// itself (issue 719). The contract test cannot see this entry and does not need to.
	calendar.WellKnown: true,
}

// bearerScheme is compared case-insensitively, as RFC 9110 §11.1 requires of an auth scheme.
const bearerScheme = "bearer"

// basicScheme is accepted on BasicRoutes only: a CalDAV client sends the credential it was
// configured with as a Basic password and has nowhere to put a bearer (P-06). The password is
// the token - a personal access token, revocable where a password is not - and the user name is
// whatever the client shows; it is read and discarded.
const basicScheme = "basic"

// BasicRoutes are the mounted trees that take HTTP Basic beside the bearer. The route label is
// the mount's path (Mounted.Handler).
var BasicRoutes = map[string]bool{
	calendar.Prefix: true,
	// And the discovery address in front of it, for the client that sends its credential from
	// the first request on: refusing the scheme there would end the discovery before the tree
	// was ever asked.
	calendar.WellKnown: true,
}

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
	// BaseHost is the installation's own host name, for reading a tenant subdomain off the
	// request (multi-tenancy.md §3). Empty means no subdomain is ever read - single mode, or an
	// installation that did not state its base URL.
	BaseHost string
}

func (a Authenticated) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	_, route := a.Routes.Handler(r)
	credential, err := credentialOf(r, BasicRoutes[route])
	if err != nil {
		WriteUnauthenticated(w, err, requestID)
		return
	}

	if credential == "" {
		// No credential at all. A public route is served anonymously; everything else is refused
		// here rather than by the handler, because a handler that has to remember is one that
		// will forget.
		if PublicRoutes[route] {
			a.Next.ServeHTTP(w, r)
			return
		}
		if BasicRoutes[route] {
			// The challenge a CalDAV client acts on: it prompts for the account and the token
			// on a Basic challenge, and shows an error on a Bearer one.
			writeBasicChallenge(w, shared.ErrUnauthenticated.WithDetail("access.credential_required"), requestID)
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
		if BasicRoutes[route] {
			writeBasicChallenge(w, err, requestID)
			return
		}
		WriteUnauthenticated(w, err, requestID)
		return
	}

	// The subdomain and the header are §3's weaker sources of tenant resolution: either may
	// confirm the token's claim, never overrule it - a contradiction is answered with the code
	// the table names, not resolved in anybody's favour (multi-tenancy.md §3).
	if label := tenantLabel(r.Host, a.BaseHost); label != "" && label != actor.TenantSlug {
		WriteProblem(w, shared.ErrForbidden.WithDetail("access.tenant_mismatch"), requestID)
		return
	}
	if claimed := r.Header.Get(TenantHeader); claimed != "" &&
		claimed != actor.TenantID.String() && claimed != actor.TenantSlug {
		WriteProblem(w, shared.ErrForbidden.WithDetail("access.tenant_mismatch"), requestID)
		return
	}

	ctx := appshared.ContextWithActor(r.Context(), actor)
	// The tenant reaches the log lines and the metric label from here (§3.1). An identifier of an
	// installation, never user content (rule 10).
	ctx = correlation.ContextWithTenant(ctx, actor.TenantID.String())
	if !actor.APIClient.IsZero() {
		// The app behind the credential (H-05): every audit entry of the request records it.
		ctx = correlation.ContextWithAPIClient(ctx, actor.APIClient.String())
	}

	a.Next.ServeHTTP(w, r.WithContext(ctx))
}

// tenantLabel reads the subdomain off a request host, when this installation knows its own.
// One label and no more: a nested subdomain names nothing here.
func tenantLabel(host, baseHost string) string {
	if baseHost == "" {
		return ""
	}
	if split, _, err := net.SplitHostPort(host); err == nil {
		host = split
	}
	label, found := strings.CutSuffix(strings.ToLower(host), "."+baseHost)
	if !found || label == "" || strings.Contains(label, ".") {
		return ""
	}
	return label
}

// bearerCredential reads the Authorization header. An absent header is not an error - that is an
// anonymous request, and only the route decides whether it is allowed. A malformed one is.
func bearerCredential(r *http.Request) (string, error) {
	return credentialOf(r, false)
}

// credentialOf reads the credential a request presents: a bearer anywhere, and on a Basic
// route also a Basic password. The Basic user name is not a credential and is not read.
func credentialOf(r *http.Request, basicAllowed bool) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", nil
	}

	scheme, value, found := strings.Cut(header, " ")
	if found && basicAllowed && strings.EqualFold(scheme, basicScheme) {
		// The standard library's parser. What arrives in the password field is a token - a
		// personal access token, hashed at rest under its own purpose label like every other
		// credential - and never an account password, which the tree does not take.
		_, token, ok := r.BasicAuth()
		if !ok || strings.TrimSpace(token) == "" {
			return "", shared.ErrUnauthenticated.WithDetail("access.token_malformed")
		}
		return token, nil
	}
	if !found || !strings.EqualFold(scheme, bearerScheme) {
		return "", shared.ErrUnauthenticated.WithDetail("access.scheme_unsupported")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", shared.ErrUnauthenticated.WithDetail("access.token_malformed")
	}
	return value, nil
}

// writeBasicChallenge is the answer a Basic route gives to a missing or refused credential: the
// same problem document, under the challenge a CalDAV client prompts on. A Bearer challenge
// would make the client show an error instead of asking again.
func writeBasicChallenge(w http.ResponseWriter, err error, requestID string) {
	problem := ProblemFrom(err, requestID)
	if problem.Status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="Hubtask", charset="UTF-8"`)
	}
	writeProblem(w, problem)
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
