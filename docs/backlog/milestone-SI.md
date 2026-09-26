# Milestone SI — Signing in

The goal: **the way into Hubtask is as considered as what is inside it.** Today a person can be
invited and can sign in; they cannot change their password, cannot recover one they forgot, cannot
copy the ten recovery codes they are shown once, and learn what a password must be only after it
is refused. An installation cannot say what a password must be at all: the rule is a constant in
the domain, and there is nowhere to hold one that applies to every workspace. When this milestone
closes, the sign-in is a card that draws every step the server already names, a password has a
life — set, changed, forgotten, required anew — under a rule three levels decide, and an
installation has a layer above its workspaces to decide it from.

The concept was drawn on 2026-09-21 and walked by the owner in six passes; the client half was
built and walked against a local server on 2026-09-24/26 before this was cut. What the walk
settled is [ADR-0068](../adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md),
[ADR-0069](../adr/ADR-0069-third-party-brand-marks.md) and
[ADR-0070](../adr/ADR-0070-the-instance-layer.md); this backlog cuts them into tasks and adds only
what the cutting decided.

**Where this sits in the roadmap is the owner's to set.** Its centre is in the core — a policy, a
reset, an instance layer — while its visible half is the client, so it belongs to neither track
cleanly. The tasks are numbered `SI-xx` and carry no version.

**SI is not a release.** Nothing is published by it; the product version stays the single line
[ADR-0035](../adr/ADR-0035-one-product-version.md) decided.

Every task is one pull request. The order is binding where dependencies exist, and one order is
binding beyond them: **the specification before the code, and the core before the screen.**

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue. Both are **[L]** during the initial phase (`CLAUDE.md`).

What deliberately is **not** in this milestone:

* **Passkeys.** The next milestone, with its own ADR — the library-or-in-house question
  (`go-webauthn` against a minimal in-house verifier, the way [ADR-0053](../adr/ADR-0053-totp-qr-code.md)
  answered the QR encoder) is a supply-chain decision that deserves its own reading. What lands
  here is only what makes them cheap later: credentials in the plural, `session.signed_in_with`,
  the step machine as the contract's own shape, and `methods` as a list.
* **Plans.** The grouping of workspaces that carries limits, locks and feature entitlements is a
  milestone of its own. Three preparations land here so it needs no second model (SI-05).
* **Custom domains.** After the passkeys, because a passkey binds to the registrable domain and a
  workspace that has both has to be told which host its passkeys belong to. `tenant_host` lands
  here as a list of hosts with a canonical one (SI-12), which is the migration that would
  otherwise run through the whole sign-in path later.
* **An operator console beyond the four screens SI-17 names.** No billing, no contracts, no
  dunning: Hubtask holds a state and a consumption, and the platform in front of it holds the rest.
* **A second catalogue, a second frame, a second app.** The instance area is a route area of the
  web app, for the reason [ADR-0070](../adr/ADR-0070-the-instance-layer.md) §5 gives.

Nine decisions taken while cutting, beyond what the ADRs hold:

1. **The client is gated behind the manifest, so it can land first.** `features.sign_in_rules` in
   `/meta/capabilities` is what turns the new surface on. Until a server answers it, the sign-in
   screen is exactly the screen it was: no forgotten-password link, no rules list, and **no probe
   of a route that does not exist**. That is what makes SI-13 landable before SI-02.
2. **One use case sets every password.** Four doors — invitation, reset, pending credential,
   bearer with a step-up — one policy check, one audit action, one history write. Four use cases
   would be four places for the rule to drift.
3. **The enforcement never becomes a job.** It is a step of the sign-in, because that is where the
   plaintext already is. This is not an optimisation; it is the project's rule that nothing
   enumerates accounts or tenants, and it is what makes a policy change instant at any size.
4. **The check route demands the setting's own proof.** Without it `password:check` is an oracle
   for blocklists and histories. Its rate limit is its own, beside the auth bucket.
5. **Recovery codes are the account's, not the factor's.** They move to their own route, get a
   count that is answered, and become the fallback for whatever second factor exists — which is
   what makes them still right when a passkey is one.
6. **The eye replaces "repeat it" everywhere**, and the second field goes. SC 3.3.8 asks that
   authentication is not a memory test; two fields make everybody type twice to catch the typing
   mistakes of a few.
7. **A refused password gets no banner.** The field is `aria-invalid`, the list says which rule,
   and the one live region says the count once. A banner would be the same sentence twice on a
   card that has one slot for a sentence.
8. **The brand marks live in the application** until a second client draws one
   ([ADR-0069](../adr/ADR-0069-third-party-brand-marks.md) §5), and every colour carries the lint
   exemption with its reason.
9. **`instance_setting` carries no row-level policy**, like `job`, and its exception is entered in
   all three lists in the same commit as the table.

---

## SI-01 — The three records, and the documents they change **[L]**

*Depends on: nothing.*

[ADR-0068](../adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md),
[ADR-0069](../adr/ADR-0069-third-party-brand-marks.md) and
[ADR-0070](../adr/ADR-0070-the-instance-layer.md), the index rows in `docs/adr/README.md` and
`arc42.md` §9, this backlog, `security.md` §5 (the rule instead of the constant, the four doors,
the reset, rotation as an event and not a calendar) and `multi-tenancy.md` §4.1 (the instance
layer). The deviation this milestone makes from a subject document is recorded where the document
is, which is what makes it a decision rather than a drift.

**Acceptance:** `make gate-docs` green; every ADR reachable from both indexes; `security.md` §5 no
longer says "no forced rotation" without qualification.

**Read:** `security.md` §5, `multi-tenancy.md` §2, §4; the concept's §5 and §7

---

## SI-02 — The policy in the domain, and the rules a screen may read **[L]**

*Depends on: SI-01.*

`SignInPolicy` as a value in `core/domain/model/identity`: the eighteen switches, `Tighten` per
switch, `Effective(product, instance, plan, workspace)` — with `plan` empty, because the parameter
is decision 3 of ADR-0070 and the column is SI-05's. `PasswordRules.Check(password, context,
history)` beside it: NFKC before anything is counted, Unicode categories for the four classes, the
leet fold for the list lookups, the context words from the address, the display name, the workspace
name and its slug. `CheckPassword` keeps its name and calls the default rule, so every caller that
has one today keeps working.

Ports: `PasswordBlocklist` (the operator's file, offline) and the embedded common-password list —
**which list, and under which licence, is decided in this task and recorded in
`THIRD-PARTY-LICENSES.md`**, or the switch ships reading only the operator's file.
`account.password_set_at` as a column. The rehash at sign-in, from the parameters in the hash
string.

`GET /auth/sign-in-rules` — public, workspace from the host, answering the methods, the providers,
the password rules and the legal links, and **nothing a guesser can use**. The specification first,
then `make generate`, then the handler; the contract gate compares the router with the document and
will not let one exist without the other.

`api/fixtures/password-rules.json` — rules, passwords, expected violations — read by the Go test
here and generated into `@hubtask/api-client` for the client's (SI-14). One corpus, two
implementations, no drift.

**Acceptance:** table tests over the eighteen switches including the Unicode cases (`Straße` is six
characters, `Ω` is an uppercase letter, an emoji is one "other"); the fixture read by the Go test
and by `pnpm --filter @hubtask/webapp test`; `make verify` green; `make generate` no diff; a
cross-tenant negative test for every new repository method.

**Read:** `core/domain/model/identity/Session.go`, `api-guidelines.md`, ADR-0068 §1–§3, §7

---

## SI-03 — `SetPassword`, and the change on the profile **[L]**

*Depends on: SI-02.*

One use case with four doors, in `core/application/service/identity`. `POST /auth/password`
(bearer + step-up) and `POST /auth/password:check` (the same proof the setting demands, its own
rate-limit bucket, an answer of rule ids and never a sentence). Changing ends every other session
of the account and says so; personal access tokens keep working, because they are their own
credentials with their own expiry and their own list. Audited as `account.password_changed`,
without the password and without a hint about its shape.

**Acceptance:** a refusal carries one `field_errors[]` entry per violated rule, each with the
`auth.password_rule.*` code; the other sessions are gone and this one is not; `password:check`
without a proof is refused; the action is in the `AuditableAction` registry (gate SG-13); the use
case is in the registry and reachable through REST, MCP and automation (parity test).

**Read:** `StepUp.go`, `Sessions.go`, `audit.md` §5, ADR-0068 §5

---

## SI-04 — Forgetting a password **[L]**

*Depends on: SI-03.*

`auth_pending.purpose` gains `RESET` (a forward-only migration of the check constraint).
`POST /auth/password:forgot` answers `202` for every address and queues
`notification.password_reset` on the queue the invitation already uses; an account that signs in
only through a provider gets a different mail and the same answer.
`POST /auth/password:reset` spends the token, sets the password, ends every session — and answers
the `202` that asks for the second factor where one is armed, rather than a session.

**Acceptance:** the two answers are indistinguishable for an address with and without an account,
asserted byte for byte; a spent token is refused; an expired one is refused the same way; the mail
is rendered from message codes (no display text in the backend); the reset of an account with TOTP
answers `202` and the pair only after the code.

**Read:** `InviteAccount.go`, `DeliverNotification.go`, `security.md` T-02, ADR-0068 §6

---

## SI-05 — The instance layer, part one: the register and the settings **[L]**

*Depends on: SI-02.*

`operator` (an account **or a service account**, since when, by whom), seeded from
`HUBTASK_OPERATORS`, checked **when `admin:tenants` is minted and when it is exercised** — the two
places, because either alone is a hole. `instance_setting` (key, value, lock, who, when) with no
row-level policy and **the exception entered in all three lists in this commit**: the policy block
in `db/schema.sql`, the reasoned map in `test/integration/tenant_boundary_test.go`, and
`cmd/restore-drill/checks.go`.

`GET/PUT /admin/settings`, `GET/POST/DELETE /admin/operators`, both behind the scope and the
register, both written into `instance_event`. `HUBTASK_INSTANCE_FILE` with `seed` and `enforce`,
and the health report saying which is in force. `hubctl admin settings|operator`.

The three preparations of ADR-0070 §3: `Effective` already takes the plan, `tenant.plan_id` exists
as a nullable column, the lock carries `INSTANCE`/`PLAN`.

**Acceptance:** an account outside the register cannot mint the scope **and** cannot use a token
that already carries it; the last operator cannot remove themselves; in single mode the register
is empty and the owner is the operator; `enforce` refuses the write with a named code; the boundary
test passes with the new table named and reasoned; `make gate-security` green.

**Read:** `Provision.go`, `AccessToken.go`, `multi-tenancy.md` §2.1, §4.1, ADR-0070 §1, §2, §5

---

## SI-06 — The instance layer, part two: the elevated session **[L]**

*Depends on: SI-05.*

`POST /auth/sessions:elevate` — a registered operator, a fresh step-up, one hour, not renewable,
bound to this session, ending with it, both ends in `instance_event`, the remaining time in the
answer. `catalogue.SessionScopes` gains the exception with the reasoning beside it.

**Acceptance:** an elevation that is not renewed expires and is refused; a second elevation needs a
second step-up; the elevation dies with the session and with a sign-out everywhere; an account not
in the register is refused; both ends are in the journal.

**Read:** `StepUp.go`, `catalogue/Catalogue.go`, ADR-0070 §4

---

## SI-07 — The workspace's rule, and the step that enforces it **[L]**

*Depends on: SI-05.*

`tenant.settings.sign_in_policy` written through `PATCH /tenant` by an owner behind a step-up, a
locked switch refused against its field, audited with the value before and after.
`MfaChallenge.methods` gains `PASSWORD_CHANGE`, `auth_pending.purpose` gains `PASSWORD`, and
`POST /auth/sessions:set-password` completes it. `account_password_history` (hash, set at, at most
ten, gone with the account) and the `min_age_hours` bound that a reset ignores.
"Require a new password from everyone" writes `rotation_from`.

**Acceptance:** a tightened rule routes the next sign-in of a non-conforming password into the
step and lets a conforming one straight through; a locked switch is refused with the field named;
the rotation is one write and no job; the history refuses the last *n* and the *n+1*-th is
accepted; `account.password_rotation_required` is in the audit registry.

**Read:** `Workspace.go`, `TwoStep.go`, ADR-0068 §2, §3

---

## SI-08 — The session's own bounds **[L]**

*Depends on: SI-07.*

`session.signed_in_with` (`PASSWORD`, `PASSWORD_TOTP`, `PASSWORD_RECOVERY`, `OIDC`, `INVITATION`,
`RESET` — and `PASSKEY` when there is one), answered in the session list. Three comparisons in
`AuthenticateToken`: the rotation cutoff, the maximum age and the idle bound.
`mfa_required_for = EVERYONE` routes a member without a factor into `ENROLL`, which the machine
already does for administrators.

**Acceptance:** a session opened before `rotation_from` is refused on its next request with the
code that says so; an idle session past its bound is refused; the list shows how each session was
opened; `EVERYONE` routes a member and not a service account.

**Read:** `AuthenticateToken.go`, `Sessions.go`, ADR-0068 §3

---

## SI-09 — Recovery codes as the account's **[L]**

*Depends on: SI-02.*

`POST /auth/mfa/recovery:regenerate` behind the step-up: ten new, the old ten burned in the same
transaction, audited as `mfa.recovery_regenerated`. `recovery_codes_remaining` on
`GET /accounts/me`, so the count the contract has answered at sign-in since H-02 is also readable
where somebody can act on it.

**Acceptance:** the old codes stop working the moment the new ones are answered; the count is
answered to its holder and to nobody else; zero is answered as zero rather than omitted.

**Read:** `Mfa.go`, `security.md` §5, the concept's §4

---

## SI-10 — Providers in the plural **[L]**

*Depends on: SI-05.*

`identity_provider` gains an `id` as its key, a nullable `tenant_id` (NULL = the installation's),
`display_name`, `kind`, `provisioning` and `position`; the row-level policy lets every workspace
read the NULL rows and write none. `account_identity` (tenant, account, provider, subject),
unique per provider and subject; `external_subject` stays and is copied in.
`provisioning` per provider: `INVITED_ONLY` (the subject must meet an invited account with the same
**verified** address), `DOMAINS` (today's behaviour), `ANY`. **A public provider is
`INVITED_ONLY` and cannot be changed** — which is the answer to the finding that today every
unknown subject is provisioned an account.

`/identity-providers` as a collection, `/admin/identity-providers` for the installation's,
`OidcStart.provider_id`. Presets as data: `GENERIC`, `GOOGLE`, `MICROSOFT` — issuer or issuer
pattern, the scopes, whether addresses are verified, the particulars (`hd`, the per-tenant issuer
behind `common`), and the registration instructions as message codes carrying this installation's
redirect URI.

**Acceptance:** two providers on one workspace both sign in; an installation provider is readable
by every workspace and writable by none; `INVITED_ONLY` refuses a subject with no invited account
and links one that has it, audited; a provider with unverified addresses cannot be `INVITED_ONLY`;
the tenant-boundary test names the new NULL-row rule.

**Read:** `OidcSignIn.go`, `IdentityProviderConfig.go`, ADR-0036, the concept's §8

---

## SI-11 — The marks, and the button that carries them **[L]**

*Depends on: SI-01, SI-10.*

[ADR-0069](../adr/ADR-0069-third-party-brand-marks.md) made real: the marks that ship, each with
its row in `THIRD-PARTY-LICENSES.md` naming the source and the guideline, each colour with its
lint exemption and reason; the letter tile for everything else; `Button`'s `lead` and `isFull`
(built with the client half and reviewed here against the ADR).

**Acceptance:** `pnpm -r lint` finds no unexplained colour; every mark that ships has a licences
row; a provider with no preset draws the tile; the column's marks are in one line and its labels
on one axis at 375 px and at 200 % zoom.

**Read:** ADR-0041, ADR-0069, `build/lint-no-literals.js`

---

## SI-12 — Legal links, and the hosts a workspace answers at **[L]**

*Depends on: SI-05.*

The four links as instance settings with locks and as `tenant.settings.legal`, resolved
`workspace → instance → nothing` and answered by `/auth/sign-in-rules`;
`/meta/capabilities.legal` for the case with no workspace. `tenant_host`: the canonical host
derived from the slug today, a state and a verification mark per row, and the canonical flag — the
model custom domains need, without the feature.

**Acceptance:** B2C (locked at the instance) and B2B (open, set per workspace) both resolve
correctly on the sign-in card; a workspace with nothing set shows the instance's; an installation
with nothing set shows no line at all, because a private installation owes nobody an imprint.

**Read:** `data-protection.md` §6, the concept's §10

---

## SI-13 — The card, and the steps the server names **[L]** · *built, in review*

*Depends on: nothing, behind `features.sign_in_rules`.*

The signed-out screens without the shell: the card with the wordmark, the host, the two ambient
washes and the operator's links in the footer; the step machine over `201`, `202` and the one
refusal — the code, the recovery code, the enrolment, the new password; the identity line with
"Not you?"; the expiry from `expires_at`; four tones for four causes; `CodeField`; the eye on every
password field; a provider button per provider; the last method used, remembered on the device.

**Acceptance:** a server that does not answer `features.sign_in_rules` gets the screen exactly as
it was; the card is one `<main>` and one `<h1>`; nothing scrolls sideways at 375 px; the walk of
SI-18 passes.

**Read:** ADR-0061, ADR-0068 §7, the concept's §2 and §3

---

## SI-14 — The rules under a password field **[L]** · *built, in review*

*Depends on: SI-13.*

`PasswordRules` and `passwordRules.ts`: one line per active rule, from the server's data through
message codes; the local rules at every keystroke, the server's once the local ones hold, after
400 ms of quiet, once per value, cached by value, cancelled when the value changes; the field
`aria-invalid` when a rule refuses and the list saying which; the count announced once when a send
is refused. At all four places a password is set.

**Acceptance:** the fixture from SI-02 passes in the client's test; the server is not asked while a
local rule is open; a value already answered is not asked again; the states are carried by a
character and a word as well as a colour.

**Read:** `capability.ts` (the two-implementations problem), ADR-0068 §7

---

## SI-15 — The password on the profile, and the codes **[L]** · *built, in review*

*Depends on: SI-03, SI-09.*

The security screen becomes what it now holds: the password with its rules, the recovery codes in
`OneTimeSecret` with copy and acknowledgement, the count and the button that makes new ones, the
second factor. The row is renamed to what the screen is.

**Acceptance:** the codes can be copied where the browser has a clipboard and the panel says so
where it has not; the panel cannot be dismissed before the acknowledgement; the count reads zero
as the number to act on.

**Read:** `OneTimeSecret.svelte`, ADR-0065 decision 3

---

## SI-16 — The workspace's sign-in settings **[L]** · *built, in review*

*Depends on: SI-07, SI-12.*

Administration → Sign-in: the eighteen switches in their groups, each with what the instance set
beside it, a locked one shown with its value and its reason and switched off, the legal links, and
"require a new password from everyone" behind a confirmation that says what it does.

**Acceptance:** a locked switch cannot be changed and says by whom; a value below the instance's is
a field error naming it; the rotation asks before it acts; every change is audited.

**Read:** ADR-0068 §2, ADR-0070 §3

---

## SI-17 — The instance dashboard **[L]**

*Depends on: SI-06, SI-12.*

`/instance` as a route area of the web app, excluded from the shells' areas like administration:
the overview (workspaces by state, accounts, health), the workspaces with their lifecycle, the
instance values with their locks, the operators, the journal. The elevation with its remaining
time on the screen, and the area invisible without it.

**Acceptance:** `routes.test.ts` asserts the area and the prefix agree; nothing in the area shows
the contents of any workspace; the elevation's remaining time is visible and its end returns the
reader to the application.

**Read:** ADR-0032, ADR-0070 §5

---

## SI-18 — The walk, and the documents current **[L]**

*Depends on: everything.*

The Playwright walk over every screen (keyboard including `Enter`, the eye, the refusal, the
second step, the rule timing and the value cache, 375 px, both modes, the provider column);
`hubctl-e2e` over the reset and the rotation; the data-catalogue rows for the reset token and the
session's user agent, IP class and method, each with a deletion path; the metrics
(`auth_password_reset_requested_total`, `auth_sign_in_step_total{step}`,
`auth_password_rehashed_total`) checked against a real scrape rather than invented in a promtool
test; the walk's evidence in `docs/evidence/`.

**Acceptance:** `make verify` green; the browser job green in all three engines; every new personal
data field in the catalogue with a deletion path; every metric read from a scrape.

**Read:** `docs/evidence/`, `observability-reliability.md` §4, `data-protection.md` §5

---

## The order at a glance

```
SI-01  the records
  ├── SI-02  the policy, the rules route, the fixture
  │     ├── SI-03  SetPassword + the check
  │     │     └── SI-04  forgot and reset
  │     ├── SI-05  the register and the instance settings
  │     │     ├── SI-06  the elevated session ── SI-17  the dashboard
  │     │     ├── SI-07  the workspace rule + PASSWORD_CHANGE ── SI-08  the session bounds
  │     │     ├── SI-10  providers in the plural ── SI-11  the marks
  │     │     └── SI-12  the legal links and tenant_host
  │     └── SI-09  the recovery codes
  └── SI-13  the card ── SI-14  the rules under the field
                      ├── SI-15  the profile
                      └── SI-16  the workspace settings
                                   └── SI-18  the walk
```

SI-13 to SI-16 are built and in review ahead of the core, which is only safe because of decision 1:
behind `features.sign_in_rules` they are the screen that exists today. Everything else waits for
the specification it is written against.
