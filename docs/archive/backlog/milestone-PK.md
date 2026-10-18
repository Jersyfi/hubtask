# Milestone PK — Passkeys

The goal: **a person signs in with a passkey alone, holds one for each device and a spare, and
uses a security key as the second factor** — at their workspace's own address, verified by a
verifier of Hubtask's own with no new dependency, and without a way in that is weaker than the
account's own proof. Milestone SI shaped the contract for it (`methods` a list, `signed_in_with` a
closed set, recovery codes on the account); the table for several credentials is PK's.

The rules are drafted in [identity.md](../../architecture/identity.md) §23 and the reasoning in
[ADR-0085](../../adr/ADR-0085-passkeys-verified-in-house.md). Every task names its use cases; a task
is done when the named checks hold, with the evidence in the pull request.

**Delivers:** UC-ID-03 (5), UC-ID-21 (1–9), UC-ID-22 (1–8)
**Released:** 2026-10-15
**Closed:** 2026-10-18

## Decisions

Each measured against the deployments `D1`–`D7`:

1. **A minimal in-house verifier, no new dependency** (the owner, 2026-10-15, #1520): ADR-0085 §1.
   `go-webauthn` would replace none of the sign-in Hubtask has; it was declined for the CBOR, JWT
   and TPM code it brings for attestation, which Hubtask does not ask for.
2. **A passkey with user verification opens the account alone** and satisfies the second-factor
   rule; a credential with presence only is the second factor after the password (the owner,
   2026-10-15, #1521): identity.md §23.1.
3. **Up to ten passkeys or security keys per account, beside at most one authenticator app**, and
   two use cases, UC-ID-21 and UC-ID-22; UC-ID-03's *Where it ends* and check 5 follow (the owner,
   2026-10-15, #1522).
4. **No workspace switch that demands passkeys** and refuses codes, for now (the owner,
   2026-10-15).
5. **A passkey belongs to the workspace's canonical host**, never to the registrable parent
   (worker, at the cut): one workspace's passkeys are never offered in another (P-01), and UC-INS-15
   check 5 already tells an administrator that a new domain ends them. identity.md §23.3.
6. **The `PASSKEY` switch is about the way in, not about factors** (worker, at the cut): switched
   off, a passkey opens nothing alone, and security keys still serve as the second factor and the
   step-up — otherwise a switch would lock out people whose only factor is a key (P-16).
   `PASSKEY` never counts as the last way in. identity.md §23.4.
7. **On by default where the password is on** (worker, at the cut): the migration adds `PASSKEY` to
   every stored method list holding `PASSWORD` and to the product's default; a workspace that
   turned the password off chose its ways in and switches passkeys on itself.
8. **The web app only** (worker, at the cut): a shell's webview origin is not the workspace's host;
   the shells' way to a passkey is F7's open point, added to its cut.
9. **#1337 stays in CP** (the owner's word that CP is untouched for now): until it is fixed, an
   invitation in multi mode is redeemed at the installation's host, so PK-04 offers a passkey at
   the setup during sign-in only where the page runs at the workspace's own host.
10. **The recovery codes stay while a key remains** (the owner, 2026-10-16, #1556): turning the
    authenticator app off keeps them while a passkey or security key remains; they go with the last
    factor of either kind. UC-ID-03 check 6 follows; identity.md §8 and §9 with PK-02.

---

## PK-01 — A person adds a passkey

*Depends on: nothing.* · issue #1523 · identity.md §23.2, §23.3, §23.5 · ADR-0085

**Use cases:** UC-ID-22 (1, 2, 3, 6)

The contract first: listing an account's WebAuthn credentials, beginning and finishing a
registration (the beginning behind a step-up, whose challenge carries it, bound to the session). A
migration creates the credential table under the tenant's row security and widens `auth_pending`'s
purpose for a registration challenge. The in-house verifier: the CBOR subset, the COSE key, the
three algorithms, the registration checks of WebAuthn Level 2 §7.1, an outbound adapter behind a
port, with the specification's examples as table tests and a fuzz target. Up to ten per account; the
first factor of an account without recovery codes answers ten. The profile's *Password and sign-in*
lists them with name, added and last used, offers *Add a passkey* only where the browser can and the
account holds fewer than ten. The data catalogue gains the table; backup keeps it, export strips it,
and an erasure removes it.

**Acceptance:** verifier table tests against the published examples and malformed inputs, a fuzz
target in `make gate-unit`'s reach; a Testcontainers test for the repository with a cross-tenant
negative test; a test that an eleventh registration is refused and a first one answers recovery
codes; an export test that strips the table; an e2e walk adding a passkey with the browser's
virtual authenticator; a walk with a real platform passkey and a FIDO2 key, recorded as evidence.

---

## PK-02 — A person renames and removes a passkey, and the factor rule counts it

*Depends on: PK-01.* · issue #1524 · identity.md §23.2

**Use cases:** UC-ID-22 (4, 5, 7), UC-ID-03 (5)

Renaming with the session alone; removing behind a step-up, at once. `FactorRule` reads an account
as holding a second factor when the authenticator app is armed or a WebAuthn credential exists, so
the profile does not offer removing the last factor — app or key — where the workspace requires one,
and says why. Adding and removing are audited without the name and tell the person through the
notice SC-36 built for every change to a way in. The recovery codes follow the same reading:
`has_second_factor`, `recovery_codes_remaining` and regenerating the codes count a key, and turning
the authenticator app off keeps the codes while a key remains (PK-01 answers codes with the first
key).

**Acceptance:** application tests that the last factor is refused for removal under each value of
`mfa_required_for`, and the app is removable while a key remains; an audit test that the name is in
no entry (rule 10); a notification test for adding and removing; an e2e walk renaming and removing.

---

## PK-03 — Sign in with a passkey

*Depends on: PK-01.* · issue #1525 · identity.md §23.3, §23.4

**Use cases:** UC-ID-21 (1, 2, 3, 4, 7, 8, 9)

The contract: a challenge for a sign-in without an identifier and a finish that opens a session
with `signed_in_with = PASSKEY`; `PASSKEY` in the method enums. The verifier's assertion checks
(WebAuthn Level 2 §7.2): origin and RP ID against the workspace's canonical host, UV required for
this door, the counter. The one refusal of §4.3 and the attempt ledger, written in its own
transaction (known traps: a write on a refusal path). The migration that adds `PASSKEY` to the
method lists that hold `PASSWORD`; the switch on the workspace's and the installation's sign-in
rules screens. The card: conditional mediation in the address field, the *Sign in with a passkey*
button where the browser can, a cancelled prompt that says nothing; the session list's label.

**Acceptance:** application tests for each refusal (unknown, removed, disabled, suspended, wrong
origin, wrong RP ID, no UV, counter regression) giving the same answer and counting; a multi-mode
test that a credential of another workspace is refused; a test that the switch off refuses and the
methods without `PASSWORD` or `OIDC` are refused as the last way in; an e2e walk with the virtual
authenticator signing in under `mfa_required_for = EVERYONE`; a keyboard and screen-reader walk.

---

## PK-04 — A security key as the second factor, the fresh proof and the setup

*Depends on: PK-01, PK-02.* · issue #1526 · identity.md §23.1, §23.4

**Use cases:** UC-ID-21 (5, 6, 9), UC-ID-22 (8)

The second-factor step (`TOTP` in `MfaChallenge.methods`, and the reset's kept factor) accepts an
assertion with presence only, beside the code and the recovery code, and the card offers *Use my
security key*. The step-up gains `PASSKEY` after `TOTP`. `RECOVERY` is offered where a key is held,
as where the app is. The `ENROLL` step offers adding a passkey or a security key beside the
authenticator app — only where the page runs at the workspace's own host (Decision 9).

**Acceptance:** application tests that a presence-only assertion completes the second-factor step
and never opens a session alone; a step-up test with `PASSKEY`, and that the elevation of an
operator accepts it; an `ENROLL` test completing with a key; e2e walks of the second-factor step
and of the setup during sign-in with the virtual authenticator.

---

## The matrix

| Door | What PK changes |
|---|---|
| Web app | Every check; the only place a ceremony runs. |
| REST | The ceremonies' routes, which the web app calls; a script gains nothing it can use without an authenticator. |
| MCP, `hubctl`, automation, background worker | Nothing: tokens, not passkeys. A token minted after a passkey step-up is the same token. |
| Installation level | The installation's sign-in rules carry the `PASSKEY` switch, default and lock (§18); an operator's elevation accepts a passkey step-up. |

| Account | Passkeys |
|---|---|
| Person, with password | Adds, signs in alone, uses a key as second factor and step-up. |
| Provider-only person | Adds after a provider step-up; signs in alone where `PASSKEY` is on. |
| Managed account (§13) | Adds; an issued start password leaves its passkeys. |
| Service account | None (§15.2). |
| Invited, not yet active | None until the invitation is redeemed; then the setup of PK-04. |
| Disabled, suspended workspace, restricted | Refused at the passkey door as at every door; a restricted account signs in and acts as today. |
| Anonymised | Its credentials go with the erasure (`CASCADE`). |

Deployments: D1–D7 alike; single mode binds to the installation's host, multi mode to the
workspace's subdomain (Decision 5); D6's own domains are CD's, after PL.

## The cut checklist

1. **What it delivers** — UC-ID-21 and UC-ID-22 are new (#1522), walked through D1–D7 above;
   UC-ID-03 check 5 is reworded so that it stays true once a key can be the factor that remains.
2. **Principles** — P-01 (the RP ID per workspace), P-02 (UV for a way in alone, presence only as a
   factor), P-05 (no *Add* at ten, no *Remove* on the last required factor, no button where the
   browser cannot), P-11 (a cancelled prompt is not a failure), P-13 (check 9 of UC-ID-21), P-16
   (the switch does not take factors away).
3. **The matrix** — above.
4. **The premise** — checked against `main` at b9e94728d: `account_mfa` is keyed by the account
   (one factor), `session.signed_in_with` has no check constraint, `MfaChallenge.methods`, the
   step-up method and `signed_in_with` are enums that gain a value (additive), `Tighten` already
   treats `PASSKEY` as loosening `methods`, the address field carries `autocomplete="username
   webauthn"`, and no credential table exists — SI's "credentials in the plural" was not built.
5. **Decisions** — above; the owner's three are answered (#1520, #1521, #1522).
6. **The rules** — identity.md §23 drafted; §2, §8 and §16.1 change in the tasks that build them;
   ADR-0085 proposed, accepted when PK-01 merges.
7. **Findings** — none open of the area belongs to PK: #1337, #1399 and #1431 stay in CP by the
   owner's word (Decision 9 bounds PK-04 around #1337); #1507 is about providers.
8. **Tasks** — each names its checks, dependencies and acceptance; together they carry `Delivers`.
9. **The standing rules** — rule 1 (the verifier is an adapter; the core sees a port), rule 3 (the
   credential table under row security), rule 4 (challenges from `RandomSource`, times from
   `Clock`), rule 10 (the name and the credential id in no log, metric or audit entry), rule 11 and
   rule 12 (contract first, an expand-only migration); known traps: a write on a refusal path,
   writer copies in `cmd/server/main.go`, credentials in restore and export.
10. **A look before the build** — PK-01, PK-03 and PK-04 change screens; the owner sees them on
    their draft pull requests.
