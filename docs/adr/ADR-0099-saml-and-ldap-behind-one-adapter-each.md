# ADR-0099 — SAML and LDAP behind one adapter each

**Status:** accepted · **Date:** 2026-10-30 · **Decided:** 2026-10-30, by the owner (new
dependencies for SAML and an LDAP client, [accepted.md](../backlog/suite/accepted.md) A)

**Rule lives in:** [identity.md](../architecture/identity.md) §26 (SAML),
[foundation.md](../backlog/suite/foundation.md) §3 (the dependencies)

## Context

A SAML 2.0 provider proves a person by an XML document whose signature covers one element of it.
Verifying that needs exclusive canonicalisation and XML signature processing, which the standard
library does not have, and getting it wrong by hand is how signature wrapping hands over somebody
else's assertion. An LDAP directory speaks BER over TLS, which the standard library does not have
either.

## Decision

1. **`github.com/russellhaering/goxmldsig` with `github.com/beevik/etree`** verify the signature, in
   `infrastructure/saml` and nowhere else. goxmldsig is Apache-2.0 (as go-oidc is), etree
   BSD-2-Clause; both pass the licence gate. The adapter reads only the element the library
   returns as verified, never the tree it arrived in.
2. **`github.com/go-ldap/ldap/v3`** (MIT) reads a directory, in `infrastructure/ldap` and nowhere
   else, dialled through the outbound guard as IMAP is.
3. **SAML is the identity-provider port's second implementation**, not a second sign-in: the start,
   the admission, the arrival and the session are those of OpenID Connect (§10), and only how the
   identity is proven differs. The answer is posted to one assertion consumer service per
   installation, kept on the flow and verified at the one callback.
4. The architecture gate holds each library to its adapter.

## Consequences

* Replacing either library is an edit to one directory.
* Step-up and connection at a provider stay OpenID Connect's: a SAML answer reaches only a sign-in's
  flow.
* Encrypted assertions and signed requests would need a service provider key; neither is served.
