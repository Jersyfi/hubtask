# ADR-0005 — OIDC, local accounts, tokens, and RBAC

**Status:** accepted · **Date:** 2026-08-14

## Context
Private individuals need a login without an external identity provider. Companies demand SSO
(OIDC/SAML), group mapping, and later SCIM. Automation platforms and AI agents need
non-interactive, tightly scoped access. Permissions must take effect along the hierarchy
(tenant → hub → collection → item).

## Decision
* **AuthN:** OIDC (authorization code + PKCE) with just-in-time provisioning *and* local accounts (Argon2id) as a fallback. Short-lived access tokens, rotating refresh tokens.
* **Non-interactive:** personal access tokens (`hbt_pat_…`) and service accounts; only hashes are stored; scopes and an expiry are mandatory.
* **AuthZ:** RBAC with the roles `OWNER`, `ADMIN`, `MEMBER`, `CONTRIBUTOR`, `VIEWER`, `GUEST`, bound to scopes; the effective role is the highest role along the path. On top of that, token scopes (`items:write`, `automation:manage`, …) act as a second, independent bound.
* The check happens **in the application layer** (never in an adapter), with the tenant boundary additionally enforced by RLS.
* SAML and SCIM are later adapters, not part of 1.0.

## Options
1. **OIDC + local + PAT/service accounts + RBAC (chosen).**
2. An external IdP only — too heavy for self-hosting.
3. ABAC / a policy engine (OPA/Casbin) from the start — more powerful, but considerably more complex and harder to explain; RBAC covers the known requirements.
4. API keys only — no SSO, insufficient for companies.

## Consequences
**Positive:** both audiences are served; least privilege for automation and agents; the audit carries
an actor type; the permission logic is central and testable.
**Negative:** two authentication paths to maintain; the combination of roles and scopes needs
explaining; inheritance requires efficient resolution.
**Countermeasures:** permission resolution as its own domain service with a cache and comprehensive
table tests; `/meta/capabilities` documents the scopes machine-readably; the migration path to a
policy engine stays open, because the check lives in one place.
