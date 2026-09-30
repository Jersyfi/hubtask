---
id: UC-INT-10
title: Script my workspace with hubctl or the API
context: integration
actors: [PE-scripter, PE-integrator, PE-admin, PE-platform]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-09, P-11]
state: built
tasks: [G-01, B-13, F4-10, P-01, P-02, P-03]
checked_by: [core/application/service/identity/AccessToken_test.go, test/integration/access_token_test.go, test/architecture/parity_test.go, test/integration/channel_parity_test.go, test/integration/idempotency_test.go]
---

# Script my workspace with hubctl or the API

## Goal

Anything a person can do on a screen, a script can do in one line — with a credential that is
narrower than its owner, expires, and can be withdrawn on its own without touching the owner's
password.

## Story

The person opens *Profile → Tokens*, names a token *nightly export*, gives it the scopes it needs
and an expiry six months out, and copies it — it is shown once. Their cron job sets
`HUBTASK_TOKEN` and runs `hubctl item ls`, `hubctl jumble submit`, `hubctl rule trigger`. For a
script that must not depend on one person, the administrator creates a service account, grants it
a role, and mints its token instead. When the laptop is lost, the token is
revoked and the next request with it fails.

## How to check

1. A token is minted with a name, explicit scopes and an expiry, and shown once, beginning with
   `hbt_pat_`; a token without an expiry, or with one more than a year out, is refused.
2. A scope the installation does not declare is refused as a field error naming it; asking for an
   administration scope needs a fresh proof of identity first.
3. A token can never do more than its holder may: a request inside its scopes but outside the
   holder's rights is refused, and a request outside its scopes is refused whatever the holder's
   rights.
4. A person mints tokens only for themselves and for service accounts, and only with the
   permission that manages members for the latter; another person's token is refused.
5. The token list shows name, scopes, expiry and last use, never the token; revoking one makes
   the next request with it fail.
6. Every use case the product offers is available over the REST API, as an MCP tool and as an
   automation action, and a test fails the build when one is missing from any of the three.
7. `hubctl` offers a command for the work, jumble, automation, webhook, calendar, media, sync,
   import, backup and audit areas, with machine-readable output.
8. A create repeated with the same `Idempotency-Key` answers the first result and creates nothing
   twice.
9. The public API reference and the Go, TypeScript and Python clients are generated from the same
   contract the server is built from.

## Where it ends

* No tokens without an expiry and no tokens that see more than their holder.
* No scripting language inside Hubtask; rules are the internal automation.
* The account password never works as an API credential.
