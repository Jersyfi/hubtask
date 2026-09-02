# ADR-0036 — The OIDC token verification library

**Status:** accepted · **Date:** 2026-09-02

## Context

H-04 builds the relying-party half of [ADR-0005](ADR-0005-authn-authz.md): a tenant configures
its identity provider, and its people sign in through authorization code + PKCE, landing in the
session model H-01 built. That requires verifying an ID token: signature against the provider's
JWKS (fetched through `GuardedClient`, cached, surviving key rotation), `iss`, `aud`, `exp`,
`nonce`, no `alg: none`, clock skew ≤ 60 s — T-13 verbatim, with the tampered-JWT test cases the
threat row names.

Hand-rolling JOSE is excluded before any candidate is weighed:
[security.md](../architecture/security.md) §8 bans home-grown crypto, and JOSE is a format whose
history is a list of implementations that got `alg` confusion, key confusion or padding wrong.
Every cursor and token this product mints avoids JOSE for exactly that reason — but an ID token
arrives in the shape the provider chose, and RFC 7519 is the shape.

A verification library is a supply chain decision (0.6.0 decision 4, CLAUDE.md "what you do not
decide yourself"): it parses hostile input on the authentication path of every tenant that
enables SSO.

## What must be verified (the contract of the choice)

1. Signature over the provider's current JWKS, with rotation: a fetch through
   `infrastructure/httpclient.GuardedClient`, a cache, and a refetch on an unknown `kid`.
2. `iss` equals the configured issuer exactly; `aud` contains the client id; `exp`/`iat`/`nbf`
   with skew ≤ 60 s; the `nonce` this installation minted for the flow.
3. An `alg` allowlist that never contains `none` and never lets the token choose the family
   (RS/ES only, per the provider's JWKS).
4. Discovery (`/.well-known/openid-configuration`) parsed, with the issuer check RFC 8414 asks
   for.

## Options

1. **`github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2` (proposed).** The de-facto standard
   relying-party pair in Go. go-oidc does discovery, JWKS caching with rotation, and ID-token
   verification with an explicit algorithm allowlist; it is small (one package plus
   `gopkg.in/go-jose/go-jose.v2`'s successor `github.com/go-jose/go-jose/v4` underneath), widely
   deployed (Kubernetes, Dex, Vault ecosystems), and its API takes an `http.Client`, so
   `GuardedClient` slots in without a fork. `x/oauth2` is already the shape of the code+PKCE
   dance and is a `golang.org/x` module — the supply chain class this repository already accepts
   for `x/crypto`.
2. **`github.com/lestrrat-go/jwx/v2`.** A complete JOSE suite — JWS, JWE, JWK, JWT. Technically
   excellent and more configurable, but it is a much larger surface than the one thing H-04
   needs, and "complete JOSE" is precisely the attack surface §8's ban tries to keep small. More
   knobs on the verification path is not a feature here.
3. **`github.com/golang-jwt/jwt/v5` + hand-written JWKS handling.** The JWT half is fine, but
   discovery, JWKS caching and rotation would be ours to write and to get wrong - that is
   home-grown key management around a not-home-grown parser, the worst of both.
4. **Hand-rolled JOSE on the standard library.** Excluded by §8 outright; listed to record that
   it was not excluded by laziness but by policy.

## Decision

Option 1: `github.com/coreos/go-oidc/v3` (which brings `github.com/go-jose/go-jose/v4`) and
`golang.org/x/oauth2`, pinned, licence-gated (both Apache-2.0/BSD-class), imported by **exactly
one package** — `infrastructure/oidc` — with `gate-architecture` proving the confinement the way
it proves `cel-go`'s. The domain and application layers see only a port
(`core/port/identityprovider` or similar), which is what keeps a future library swap an adapter
change.

## Consequences

**Positive:** T-13's verification comes from the implementation the Go ecosystem has already
audited in anger; JWKS rotation and discovery are not ours to maintain; the PKCE dance reuses
`x/oauth2`'s well-worn shapes.
**Negative:** two new direct dependencies (three modules with go-jose) on the authentication
path; go-jose has had CVEs historically — which is an argument *for* a widely-watched library
over a private reimplementation, and `govulncheck` in SG-1 is the standing watch.
**Countermeasures:** confinement to one package; the T-13 tampered-JWT suite runs against our
adapter, not the library's own tests; Dependabot's security updates are ungrouped and immediate
(ADR-0022).

## The owner's decision

**Accepted on 2026-09-02: option 1, as proposed.** The three modules above are pinned, and H-04
is unparked — it is built against this decision. The alternatives stay recorded rather than
deleted: option 2 remains the swap that keeps the task's shape if go-oidc ever has to go, which
is precisely what confining the import to `infrastructure/oidc` buys.
