# Milestone F4 — Automation, administration, tenant

The goal: the workspace stops being something only a server operator can run. Work arrives by mail
and by webhook and is turned into a task or dismissed; a rule is written, read back, tried against a
sample event, switched on, and what it did afterwards is legible run by run; an external system
subscribes to the event stream, and a delivery that died is found and sent again. And the half that
has never had a screen at all gets one: a person signs in with a password rather than by pasting a
token, enrols a second factor, proves themselves again before the irreversible, sees the sessions
they hold and ends one; a workspace reads its own settings and its quota standing, invites somebody,
grants a role and can see what that role carries; backups are configured, run, verified and — with
three doors in front of it — restored; retention is written and previewed before it deletes
anything; the audit trail is queried, verified and exported; and a data subject request is recorded,
carried and closed against its statutory deadline.

F4 is the fourth milestone of the client track (`roadmap.md` phase 5). It opens with `0.6.0` and is
the widest of the track's backlogs, because three core milestones' worth of contract has never been
drawn (decision 1) and because the identity half underneath all of it has been a placeholder since
F1.

**F4 is not a version.** It is a planning milestone; nothing is released by it and the product
version stays the single line ADR-0035 decided. The client's maturity stage is `preview` since #381
and **stays `preview` through this milestone**: ADR-0035 §2 gives `stable` to convergence (`0.9.5`).

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone: AI in any form — the jumble's `:suggest`, the
decomposition, `AISuggestion`, semantic search — language switching, CLDR formats, the RTL audit and
WCAG 2.2 AA conformance as a body of work (`F5`; the individual accessibility rules still apply to
every component written here); offline behaviour, the local store, the sync cursor that survives a
session, the Tauri shells, the celebration kit and the onboarding tour (`F6`); `:pull` and `:push`,
which have no server until `0.8.5`; and the website's **content**, which the roadmap's website lane
keeps blocked on a brief that does not exist yet. Three things that *look* like this milestone's and
are not: everything under `/admin` (decision 6), the tenant export it holds, and the "lives on the
web" affordance ADR-0032 asks of the mobile shell — F4 tags the routes, F6 excludes them and builds
the affordance, which is what the roadmap's F6 row already says.

Twelve decisions taken while writing this backlog, so that nobody re-derives them later:

* **F4 builds the surface for three core milestones, not two.** The roadmap's table says `0.4.5`
  and `0.5.0`, and its own contents column for the same row names MFA, sessions and step-up, the
  OIDC connection and quotas — all of them `0.6.0`'s. The contents column governs and the table cell
  is corrected in the pull request that opens this milestone. The window rule exists so that the
  client works against a contract that has stopped moving (`roadmap.md` phase 5), and this one has:
  `0.6.0` is fifteen of sixteen closed, and its one open task (#267, PITR and the production
  decision) has no client surface at all.
* **Two core tasks, and both are gaps of the kind every client milestone has found.** **F4-01**: the
  workspace cannot read or change its own settings. `tenant.settings.require_admin_totp` is read by
  the sign-in path (`core/application/service/identity/TwoStep.go`) and written by nothing but SQL;
  the display name, the default locale and the default time zone are readable only through
  `/admin/tenants`, which is the installation operator's cross-tenant listing. The roadmap's "tenant
  settings" screen has, today, nothing to read and nothing to write. **F4-02**: three pieces of
  operational configuration can be created and never revised — a backup target has no delete, a
  backup schedule has no listing at all, and a retention policy has neither an update nor a delete.
  A screen that can only add rows is not an administration screen.
* **The token-paste sign-in dies here, and what replaces it is not what its own comment predicted.**
  `apps/webapp/CLAUDE.md` and `lib/platform/tokenStore.ts` both name `0.6.0` as the milestone that
  replaces them and both expect "a session the browser holds and script cannot read". The contract
  mints no such thing: `POST /auth/sessions` answers an access token of fifteen minutes and a
  refresh token of thirty days, `/auth/oidc:callback` answers the same pair, and every route takes a
  bearer. So the pair goes where the bearer already goes — `sessionStorage`, for F1-11's reason —
  and the refresh half gets that and no more. **The browser session is as long as the tab**; the
  refresh token's thirty days belong to clients that persist it, and this one does not. The two
  comments that promised otherwise are corrected in F4-03 rather than left standing.
* **Refresh is the seam's, it happens once, and two of them at a time would be theft.** A `401`
  today ends the session through the engine's one hook. It becomes: refresh once, retry the request
  once, and end the session only when the refresh itself is refused. It lives in the transport
  because a refresh in every store would race with itself — and reuse detection is not a soft
  failure: a second refresh presenting a retired token invalidates the whole family and signs the
  person out everywhere (`security.md` §5, T-01). Concurrent requests therefore share one refresh in
  flight, and that is a property with a test rather than a comment.
* **The QR code is a dependency decision, so F4-04 opens with a draft ADR rather than with code.**
  `POST /auth/mfa/totp:enroll` answers a provisioning URI and says in as many words that "the QR
  image is a client's job". Rendering one needs either a new dependency — a supply-chain decision no
  task takes on its own (CLAUDE.md) — or an encoder written into the design system. Enrolment does
  not depend on the answer: every authenticator accepts a secret typed in, so the enrolment screen
  ships with manual entry and the QR arrives when the owner has decided. The ADR should be opened
  early, because it waits on a person rather than on code.
* **What is under `/admin` is the installation operator's, and stays in `hubctl`.**
  `/admin/tenants`, `/admin/tenants/{id}:suspend|:resume|:delete|:export`, `/admin/encryption` and
  `/admin/tenants/{id}/quotas` are cross-tenant operations, and this bundle is served inside one
  tenant, from one tenant's origin. A workspace reads its own standing at `/quotas` and cannot raise
  its own ceiling; the screen says so rather than offering a control that would be refused. The
  tenant export is the operator's for the same reason, and the roadmap's F4 row names no tenant
  export.
* **An export leaves through a backup target, never through the browser.** The audit export
  (`/audit:export`) and a data subject request's archive are written to a backup target, and the
  `Job.result_url` that follows points at where the archive lies. Under `s3` or `sftp` that is an
  origin `connect-src 'self'` does not name and a place the browser holds no credential for.
  ADR-0047 added the **media** origin to the policy and exactly one origin; a backup target is not
  the media origin, and F4 does not widen the policy a second time. Every export screen therefore
  reports the target and the path it was written to, and offers no download.
* **The two routes the server already points browsers at are owed by this milestone.** The
  invitation mail links `<base>/redeem#token=…` — the token in a **fragment**, which no `Referer`
  and no server log ever sees — and the OIDC redirect URI the composition root derives is
  `<base>/auth/callback` (`cmd/server/main.go`). Both fall through `presentation/webui`'s handler to
  `index.html` today and resolve to nothing. F4-03 and F4-05 owe them, and neither may let the
  credential in the URL survive into the session history.
* **Administration is an area on a route, and the tagging is per route rather than a task.**
  ADR-0032 names the three areas, `lib/router.ts` already carries `area` on a route, and F3-17 tagged
  the first non-default one. Every administrative route added here is tagged `administration` where
  it is added — that is the "routes tagged by area" the roadmap asks of F4 — and F4-08 adds the one
  navigation entry the area needs. **Own security is not administration**: sessions, MFA, personal
  access tokens and the apps a person has allowed are that person's own settings, which ADR-0032
  puts in profile configuration and every client carries.
* **A destructive restore is the only screen in the product that can end the workspace, and it is
  built behind three doors.** `REPLACE_TENANT` replaces the tenant's rows, and the credentials that
  could sign in to it are among them. The client always runs the dry run first, demands the exact
  workspace name typed, carries a step-up token, leaves the safety backup on unless switching it off
  is confirmed by itself — and says, in the catalogue's words rather than a component's, that
  sign-in credentials do not survive the mode.
* **The jumble's AI half is F5's, and its absence is not a `CapabilityGate`.**
  `/jumble/entries/{id}:suggest` is an AI route and `AISuggestion` is F5's component. F4 builds
  arrival, conversion and dismissal; the suggestion control is **absent** rather than disabled with
  a reason, because a gate explains a capability this installation lacks and this one is a feature
  the milestone did not build.
* **A rule's conditions are supported, and the contract's prose says they are not — in three
  places.** G-06 landed the expression language, `automation.ValidConditionShape` accepts up to
  twenty conditions and `ValidThrottle` compiles a dedupe key like any other expression; yet
  `AutomationRule.conditions` still reads "Empty in this release", `RuleThrottle.dedupe_key_expr`
  still reads "Refused while it is non-empty", and `createRule`'s own description still lists a
  non-empty condition among what this build refuses. The neighbouring claim about the webhook
  `filter` is still **true** — `integration.RefuseFilter` refuses a non-empty one. F4-13 corrects
  the three that are wrong, in prose only, and the rule editor offers conditions because the server
  takes them; F4-15 leaves the filter field out because the server does not.

---

## F4-01 — The workspace can read and change its own settings **[L]**

*Depends on: nothing. The first of two core tasks, and the one the administration area's first
screen hangs from.*

`ReadWorkspaceSettings` and `UpdateWorkspaceSettings`, through all three channels, specification
first (ADR-0004). The `tenant` row has carried `display_name`, `default_locale`, `default_time_zone`
and a `settings` document since `0001_init`; H-02 put the TOTP enforcement switch inside that
document as `require_admin_totp`, and `identity.RequireAdminTotp` reads it on every two-step
sign-in. Nothing writes it. An installation that wants its administrators to hold a second factor
has, today, exactly one way to say so: `UPDATE tenant SET settings = …` in psql.

The same is true of the three columns beside it. `i18n-l10n.md` §2's resolution chain is request →
account → tenant → installation, so `default_locale` and `default_time_zone` are what every member
without a preference of their own falls back to — and the only route that answers them is
`GET /admin/tenants`, which lists every workspace of the installation and belongs to the operator
(decision 6). A workspace cannot read its own defaults.

The shape: `GET /tenant` answers the workspace the caller is in — id, slug, display name, status,
the two defaults, the settings the contract chooses to expose, and `created_at` — under `READ`, or
under `READ_CONFIGURATION` for the auditor who holds no `READ` (the split G-12 made). `PATCH /tenant`
is merge-patch like every other partial write (#430), takes `display_name`, `default_locale`,
`default_time_zone` and `require_admin_totp`, and needs `STRUCTURE`. The slug does not move: it is
the hostname in multi mode and renaming it would break every bookmark and every OIDC redirect at
once — that is the operator's operation, not the workspace's, and the refusal says so.

`require_admin_totp` is the field with a consequence, and the write states it rather than the
screen: switching it on cannot lock the last administrator out — H-02 routes an unenrolled `ADMIN`
into enrolment rather than into a refusal — but switching it on **is** what makes an ordinary
sign-in two-step for a whole class of people, so it is auditable with its before and after, like
every other configuration change. The settings document stays a document: a key this version does
not know is left where it is rather than dropped, because a partial write that rewrote the whole
JSON would silently discard what a later version put there.

**Acceptance:** `GET /tenant` answers the caller's own workspace and never another's, proved by the
cross-tenant suite; an auditor holding `READ_CONFIGURATION` and no `READ` may read it and may not
write it; `PATCH /tenant` with `application/merge-patch+json` sets the display name, either default
and the enforcement switch, and an absent key changes nothing; a slug in the body is refused by name
rather than ignored; switching `require_admin_totp` on makes an enrolled `ADMIN`'s next sign-in
two-step and leaves a `MEMBER` untouched, and the change is in the audit trail with its before and
after; a settings key this version does not model survives the write; the two use cases are in
`domain-model.md` §5 and registered in REST, MCP and automation with the parity test green; metric
and span per RT-12; `make generate` produces no diff and
`go test -tags contract ./test/contract/...` passes; the pull request names F4-08 as the client task
that consumes both.

**Read:** `domain-model.md` §5 (identity & tenancy); `api-guidelines.md` §2, §4; `multi-tenancy.md`
§3; H-02 in `milestone-0.6.0.md`; `core/application/service/identity/TwoStep.go` and
`infrastructure/postgres/MfaRepository.go` (`RequireAdminTotp`); `db/schema.sql` (`tenant`);
ADR-0004; ADR-0005; ADR-0010; ADR-0017; issue #430 (merge-patch)

---

## F4-02 — Operational configuration gets a lifecycle **[L]**

*Depends on: nothing. The second core task, and the one three administration screens wait for.*

Three pieces of configuration in this contract can be created and never revised, and one of them
cannot even be listed:

| Today | What it costs a screen |
|---|---|
| `POST /backup-schedules` and nothing else | A backup screen cannot show which schedules exist, cannot switch one off, cannot change the hour it runs at, and cannot delete one. The row is written and then invisible |
| `GET`, `POST` on `/backup-targets` | A target that was configured wrongly, or whose credentials were rotated away, stays in the list forever |
| `GET`, `POST` on `/retention-policies` | A rule that deletes data cannot be corrected or withdrawn from any client. `data-retention.md` §7's "notify-only first" only helps the rule that has not run yet |

So, specification first and every one of them additive: `GET /backup-schedules` (the schedules of the
workspace, with `next_run_at`), `PATCH /backup-schedules/{scheduleId}` (merge-patch; `enabled` is
the field that matters most and the reason a `DELETE` is not the only answer),
`DELETE /backup-schedules/{scheduleId}`, `DELETE /backup-targets/{targetId}`,
`PATCH /retention-policies/{policyId}` and `DELETE /retention-policies/{policyId}`.

Three refusals belong to the task rather than to a screen. A backup target that a schedule still
names is a `409` naming the schedules, not a cascade — deleting a target must never silently
disarm a backup. A target with archives at it is deleted from the workspace's configuration and
**nothing at the target is touched**: `backup-restore.md`'s rule that Hubtask never deletes a file
it did not write applies at least as strongly to the file it did write, and the answer says the
archives remain. And a retention policy under a legal hold's shadow behaves the way E-08 decided —
the hold wins, and it wins the same way for a deletion of the policy as for its execution.

`hubctl` grows the four verbs it lacks in the same pull request, because an operator who can create
a schedule and not list one is the person who found this gap.

**Acceptance:** `GET /backup-schedules` answers the workspace's schedules with their next run and
nothing of another workspace's, proved by the cross-tenant suite; `PATCH` on a schedule switches it
off and the scheduler stops waking for it, proved rather than asserted; deleting a target a schedule
names answers `409` with the naming schedules in the problem document, and deleting an unreferenced
one leaves every archive at the target untouched, proved by listing the target afterwards;
`PATCH` and `DELETE` on a retention policy round-trip, and a policy whose scope is under a legal
hold behaves as E-08 decided; all six operations are registered use cases with metric, span and —
because every one of them is configuration — an `AuditableAction` row (SG-13); `make generate`
produces no diff; the contract test passes; `hubctl` carries the four new verbs with the output
`hubctl`'s other listings have.

**Read:** `backup-restore.md` §4, §5, §6; `data-retention.md` §5, §7; E-03, E-05, E-07 and E-08 in
`milestone-0.4.5.md`; ADR-0019; ADR-0020; `api-guidelines.md` §2, §5; issue #430

---

## F4-03 — Sign-in becomes a session **[L]**

*Depends on: nothing. The task every other client task in this milestone stands on.*

The application stops asking for a token. `POST /auth/sessions` takes an email and a password and
answers the pair; the pair goes to the platform seam; `GET /accounts/me` still says who arrived. What
changes underneath is larger than the screen: the seam learns to refresh, and the two comments that
predicted a different `0.6.0` are corrected (decision 3).

Four pieces:

1. **The pair, and where it lives.** `platform/tokenStore.ts` holds an access token and a refresh
   token rather than one bearer, both in `sessionStorage`, both cleared together. The file's own
   reasoning is rewritten to say what was actually built: there is no cookie session in this
   contract, the session is as long as the tab, and the thirty days the refresh token could live are
   deliberately not used.
2. **Refresh, in the transport, once.** `FetchTransport` answers a `401` by calling
   `POST /auth/sessions:refresh` with the refresh token, replacing the pair, and retrying the
   original request exactly once. A second `401`, or a refused refresh, ends the session through the
   hook that ends it today. Concurrent requests share **one** refresh in flight: presenting a
   retired refresh token is theft as far as the server is concerned and costs the whole family
   (`security.md` §5), so two refreshes at once would sign the person out for being fast.
3. **The screen.** Email and password; the one generic refusal the server gives, rendered as the one
   sentence it is (T-02) — the client must not add a hint the server refused to give; the `202` that
   means a second factor is owed is handed to F4-04 and, until it lands, rendered as the message
   code it carries rather than as a failure. `redeemInvitation`'s screen at **`/redeem`** belongs
   here: the token arrives in the fragment, is read once, is replaced in the history entry before
   anything else happens, and is never put in a URL, a log or a message.
4. **The sessions listing.** `GET /auth/sessions` in the profile area: where each was opened, when it
   last acted, which one is answering — with "this device" marked, `DELETE /auth/sessions/{id}` on
   each and `DELETE /auth/sessions` as sign out everywhere, which ends the current session too and
   therefore says so before it is pressed.

Sign-out keeps what F1-11 built and adds the rest: `engine.reset()`, the pair released, and the
session ended at the server rather than only forgotten locally.

**Acceptance:** a person signs in with an email and a password and lands where they were going; a
wrong password and an unknown address produce the same sentence, and the client adds nothing to it;
an expired access token is refreshed and the request that met it succeeds, with **one** refresh
request for ten concurrent calls, proved by a test against a fake transport; a refused refresh signs
out, remembers the path and asks again; a `202` renders as the second-factor message rather than as
an error; `/redeem` sets a first password and lands signed in, and the token is gone from
`location.hash` and from the history entry before the first request leaves; the sessions list marks
the current session, ends one, ends all, and warns before the second; sign-out ends the session at
the server and leaves nothing in `sessionStorage`; `apps/webapp/CLAUDE.md` and `tokenStore.ts` no
longer promise a session the browser holds; `pnpm -r build lint typecheck test` green.

**Read:** `security.md` §5 and T-01, T-02; H-01 in `milestone-0.6.0.md`; the `SessionTokens`,
`SessionRefresh`, `Session`, `MfaChallenge` and `InvitationRedemption` schemas;
`apps/webapp/src/lib/session.svelte.ts`, `lib/platform/tokenStore.ts`,
`packages/sync-engine/src/FetchTransport.ts`;
`core/application/service/notification/DeliverNotification.go` (the invitation link); ADR-0032
(profile configuration)

---

## F4-04 — The second factor, and the proof before the irreversible **[L]**

*Depends on: F4-03. Opens with a draft ADR, which should be written first — it waits on the owner.*

**The ADR first (decision 5).** `POST /auth/mfa/totp:enroll` answers an `otpauth://` provisioning
URI and says the QR is the client's job. A QR encoder is either a dependency — a supply-chain
decision CLAUDE.md reserves for the owner — or roughly two hundred lines of Reed–Solomon and
masking in the design system. The draft ADR states both costs, and the enrolment screen ships
meanwhile with the secret shown for manual entry, which every authenticator accepts. Nothing else in
this task waits for the answer.

Then three flows the contract already serves:

* **Enrolment.** The secret and the ten recovery codes are shown **once** — `OneTimeSecret` from
  F4-06 is what shows them, and the screen refuses to advance until the reader confirms they have
  the codes. `:confirm` arms it with one valid code. Disabling asks for the password afresh, and
  under tenant enforcement an `OWNER` or `ADMIN` cannot disable at all: the refusal names the switch
  F4-01 made settable, which is how the two halves of MFA meet.
* **The two-step sign-in.** The `202` from F4-03 carries a pending credential that can do nothing
  but complete the sign-in. The screen takes a six-digit code or one recovery code, says how many
  recovery codes remain when one is spent, and — for an administrator under enforcement who is not
  enrolled — routes into enrolment rather than into a refusal, because that is what the server does
  and the client must not disagree with it.
* **Step-up.** `403` with `auth.step_up_required` stops being an error sentence and becomes a
  prompt: the accepted methods come from the refusal, the proof goes to `POST /auth/step-up`, and
  the grant travels back in the `X-Hubtask-Step-Up` header on the retried request. The seam carries
  it as a per-request header rather than as engine state, because a step-up token is consumed by one
  action and a second privileged action needs a second proof. `RestoreRequest.step_up_token` is the
  one place it travels in a body instead, and F4-17 uses it there.

The step-up prompt is a client-wide capability rather than a screen: any request may meet the
refusal, and the caller that met it is the one that retries.

**Acceptance:** the draft ADR exists, states the three options and their costs, and is linked from
the pull request; enrolment shows the URI and the codes once, arms only after a valid code, and a
reload loses them exactly as the server intends; a wrong code refuses without arming, and the same
code never verifies twice; disabling demands the password and is refused for an `ADMIN` under
enforcement with the switch named; a two-step sign-in completes with a TOTP code and with a recovery
code, and the remaining count is shown; an unenrolled administrator under enforcement lands in
enrolment; a refusal carrying `auth.step_up_required` produces the prompt, and the retried request
carries `X-Hubtask-Step-Up` and succeeds; no secret, code or grant reaches a log, a message, a URL
or the DOM beyond the field showing it; every sentence is a message code in `locales/en.json`;
`pnpm -r build lint typecheck test` green.

**Read:** `security.md` §5, §8; H-02 and H-03 in `milestone-0.6.0.md`; RFC 6238; the
`TotpEnrollment`, `TotpConfirmation`, `SignInCompletion`, `StepUpRequest` and `StepUpGrant` schemas;
`core/application/service/identity/Mfa.go`; CLAUDE.md ("What you do not decide yourself")

---

## F4-05 — OIDC: the company's provider signs people in **[L]**

*Depends on: F4-03. Independent of F4-04 — a provider's own second factor is the provider's.*

Two halves and one route this repository already promised. `POST /auth/oidc:start` answers where to
send the browser; the browser goes there; the provider sends it back to
**`<base>/auth/callback`** — the redirect URI `cmd/server/main.go` derives and the one no client has
ever implemented — with a code and the `state`; `POST /auth/oidc:callback` exchanges them for the
same pair a password sign-in mints.

What the client owes beyond the two calls:

* **The route.** `/auth/callback` in the table, tagged `end-user` because it is a sign-in, reading
  `code` and `state` from the query, replacing the history entry before anything else, and never
  keeping either. A callback reached without a flow in progress is a refusal in the words the server
  gives, not a blank screen.
* **The button, and when it is not there.** `/identity-provider` answers whether a provider is
  configured and switched on, but it needs a permission a signed-out visitor does not hold — so the
  sign-in screen cannot ask. It offers the provider sign-in and lets the server refuse: the refusal
  when no provider is configured is a clear code, and rendering it is cheaper and more honest than a
  second unauthenticated endpoint. Local sign-in never disappears, because
  `observability-reliability.md` §7's degradation is exactly that local accounts keep working when
  discovery cannot be reached.
* **The configuration screen**, in the administration area: the issuer, the client id and the
  allowed email domains, read from `/identity-provider` and written by the `PUT`, with the client
  secret write-only and never re-read — the contract does not answer it and the screen must not
  pretend it could. Removing the provider says what it costs: accounts provisioned through it keep
  their rows and lose the way back in.

No content security policy changes here, and the task says why where the next reader will look: a
top-level navigation to the provider is not a fetch, `connect-src` does not govern it, and both
calls this flow makes are to `'self'`.

**Acceptance:** a sign-in through a configured provider completes end to end against a test
provider and lands signed in with the same session the password flow produces; `state` and `code`
are gone from the URL and from the history entry before the exchange; a callback with an unknown or
spent `state` renders the server's one refusal; with no provider configured the button's refusal is
rendered and local sign-in still works; the administration screen reads, writes and removes the
configuration, never shows a secret, and states what removal costs; the policy constant in
`presentation/webui` is unchanged, asserted byte for byte; `pnpm -r build lint typecheck test` green.

**Read:** ADR-0036; ADR-0005; H-04 in `milestone-0.6.0.md`; `security.md` §4, T-13;
`observability-reliability.md` §7; the `OidcStart`, `OidcAuthorization`, `OidcCallback`,
`IdentityProvider` and `IdentityProviderConfiguration` schemas; `cmd/server/main.go` (the redirect
URL); ADR-0047 and ADR-0028 (why the policy does not move)

---

## F4-06 — Wave 3d: the administration marks **[L]**

*Depends on: nothing. Four components, and two of them join a wave the way F3's two did.*

`RoleBadge` and `PermissionMatrix` are the two the roadmap names and §4 has planned since the
inventory was written. Two more are added here, in this pull request, because the story gate reads
the inventory and a component in no wave fails the build — which is the point of the gate:

| Component | Wave | Why it exists |
|---|---|---|
| `RoleBadge` | 3 | Six roles, and the scope a role was granted at is half of what it means. A badge that said `ADMIN` without saying *where* would be the misreading the matrix exists to prevent |
| `PermissionMatrix` | 3 | The role matrix as **this installation** enforces it, read from `/meta/capabilities`'s `roles` and never compiled in. Two cells are qualifiers no permission name carries — `ASSIGNED` and the guest's comment — and the component renders `item_access` beside the permission columns rather than flattening it into a tick |
| `ProgressBar` | 1 | The second user of a `<progress>` element. `UploadField` hand-rolled one; a job that answers `progress: null` needs the indeterminate case beside it, and two hand-rolled bars is where a component belongs. `UploadField` is refactored onto it in the same change |
| `OneTimeSecret` | 3 | Five values in this milestone are "shown for the only time" — a minted token, a webhook signing secret, a TOTP secret with its recovery codes, an inbound trigger address, a jumble intake address. One component: reveal, copy, and an acknowledgement the caller can require before the value can be dismissed. It never renders a value it was not handed, and it holds none after unmount |

`PermissionMatrix` is the one with a rule worth stating: it is a **table of what the server says**,
not a control. Nothing in it is editable, because changing what a role carries is not an operation
this product has — a role is granted, and the matrix says what the grant means.

`OneTimeSecret` carries the discipline `apps/webapp/CLAUDE.md` already states for the bearer: the
value is in the DOM only while it is shown, `navigator.clipboard` is offered and its absence is not
an error, and nothing is written to any storage. The copy control announces success without the
value.

**Acceptance:** four components with a story each, keyboard-operable, in both themes and under RTL;
`design-system.md` §4 lists `ProgressBar` in wave 1 and `RoleBadge`, `PermissionMatrix` and
`OneTimeSecret` in wave 3, each with the sentence that says what it is for; `PermissionMatrix`
renders a manifest with an unknown role and an unknown permission without breaking, because
tolerance towards unknown fields is binding; `UploadField` renders through `ProgressBar` and its own
story is unchanged; `OneTimeSecret` holds no value after unmount, proved by a test; no literal
colour, spacing, radius or duration anywhere — `pnpm lint` proves it; `make workbench` shows all
four.

**Read:** `design-system.md` §4, §5, §9; ADR-0037; ADR-0029; the `RoleDescription`,
`RoleItemAccess` and `ItemAccess` schemas; `domain-model.md` §3.2; F3-05 and F3-06 in
`milestone-F3.md` (how a component joins a wave)

---

## F4-07 — Wave 3e: the automation marks **[L]**

*Depends on: nothing.*

The three the inventory named and nothing has drawn: `AutomationRuleCard`, `RunStatusBadge` and
`JumbleInboxItem`.

`RunStatusBadge` carries seven states, not four. `RuleRunStatus` is `RUNNING`, `WAITING`,
`SUCCEEDED`, `SKIPPED`, `FAILED`, `ABORTED_LOOP` and `THROTTLED`, and three of them are the ones
somebody actually asks about: `SKIPPED` is a condition that did not match and is not a failure,
`THROTTLED` is the rule protecting the workspace from itself, and `ABORTED_LOOP` is the causation
depth stopping a rule that triggered itself. A badge that mapped all three to "failed" would make
the runs list lie. The dry run is a **presentation** of a run rather than an eighth state — §4's
inventory row says "running / succeeded / failed / dry run" and the dry run is the `variant`, not
the `status`.

`AutomationRuleCard` shows a rule the way somebody scanning a list needs it: the name, the trigger
kind, how many actions, whether it is enabled, what it runs as, and the consecutive failure count
that is about to disable it. It renders no CEL and no action parameters — a card is not an editor.

`JumbleInboxItem` has three states and one rule that is not obvious: the raw subject and body are
**data that arrived from outside**, so the component renders them as text and never as markup, and
its story includes an entry whose subject is a script tag, because that is the test that matters.
`PROCESSED` links to what the conversion produced; `DISMISSED` is a state and not a deletion, and
the component says so rather than fading the row out.

**Acceptance:** three components with a story each, keyboard-operable, in both themes and under RTL;
all seven run statuses render with distinct marks and text, and the dry run is a variant of each
rather than a status of its own; a status the manifest does not know still renders, with the raw
value; `JumbleInboxItem` renders a subject containing markup as text, proved by a story and a test;
`design-system.md` §4's three rows carry the sentence each earned; no literal values; `make
workbench` shows all three.

**Read:** `design-system.md` §4; `automation.md` §1.1, §3; the `RuleRunStatus`, `AutomationRule`,
`RuleTrigger` and `JumbleEntry` schemas; G-10 in `milestone-0.5.0.md`; ADR-0009

---

## F4-08 — The administration area, the workspace's settings, and the quota standing **[L]**

*Depends on: F4-01, F4-06. The area's first screen, and the tagging the roadmap asks for.*

Three things at once, because they are one screen and the frame that holds it.

**The area.** `lib/router.ts` has carried `area` since F1 and F3-17 tagged the first non-default
route. Every route this milestone adds under administration is tagged `administration` as it is
added; this task adds the first of them and the navigation entry that reaches them, gated on what
the caller actually holds — `STRUCTURE` for most of it, `READ_CONFIGURATION` for the auditor's
read-only half, `AUDIT_READ` for the trail. A person who holds none of them sees no administration
entry at all, which is the one place in this client where hiding beats a `CapabilityGate`: the gate
explains a control a reader might want, and a whole area they will never hold is navigation noise.

**The workspace's settings**, from F4-01: the display name, the default locale and the default time
zone — each stated as what it is, the fallback for a member who has set nothing (`i18n-l10n.md` §2)
— and the TOTP enforcement switch, which says in the catalogue's words what switching it on does to
the next sign-in of every `OWNER` and `ADMIN`. The slug is shown and not editable, with the reason.

**The quota standing**, from `GET /quotas`: every §4 limit, the ceiling, what is used where a live
count exists, and the ratio — the same number `hubtask_tenant_quota_usage_ratio` reports, so an
operator reading Grafana and a member reading this screen see one fact. `limit: 0` is unlimited and
is rendered as that rather than as zero. `configured: false` means the mode's default rather than
this workspace's own ceiling, and the screen says which. **There is no control to raise one**:
`/admin/tenants/{id}/quotas` is the installation operator's (decision 6), and the screen names who
to ask instead of offering a button the server would refuse.

**Acceptance:** the administration entry appears for a caller holding `STRUCTURE` and for one
holding only `READ_CONFIGURATION` (read-only), and is absent for a `MEMBER`; every route added here
carries `area: 'administration'` and a test asserts that the area's route set is exactly the tagged
one; the settings screen reads and writes through F4-01's two operations, shows the slug as fixed
with its reason, and states what the enforcement switch costs; a failed write renders through
`lib/problem.ts` and never as a component's own sentence; the quota screen shows every limit the
server answers, renders `0` as unlimited, distinguishes a configured ceiling from the mode's
default, shows the ratio, and offers no way to change one; a quota the client does not know still
renders; every sentence is a code in `locales/en.json`; `pnpm -r build lint typecheck test` green.

**Read:** ADR-0032; `multi-tenancy.md` §4; H-08 in `milestone-0.6.0.md`; `i18n-l10n.md` §2; the
`QuotaStanding` schema; `apps/webapp/src/lib/router.ts` and `App.svelte`;
`apps/webapp/src/lib/data/capability.ts`

---

## F4-09 — People: an invitation, a role at any scope, and what the role means **[G]**

*Depends on: F4-06, F4-08. F3-07 built the container half; this is the workspace half.*

F3-07 built members and roles at container scope, because ADR-0032 puts "working with people in my
containers" on the end-user side. What is left is the tenant's own: inviting somebody who has no
account yet, granting a role at the workspace itself, the groups a membership can be granted to, and
the matrix that says what any of it means.

* **The invitation.** `POST /accounts:invite` creates an `INVITED` account, and the mail carries the
  redemption link F4-03's `/redeem` screen answers. The screen lists the workspace's accounts with
  their status, shows an invitation that has not been redeemed as exactly that, and offers the
  invite. Re-inviting is the idempotency key's job, not a second account.
* **Roles at tenant scope.** `GET /memberships?scope_type=TENANT` (F3-01's read), `POST
  /memberships` and `DELETE /memberships/{id}`, with `RoleBadge` saying which role at which scope.
  Granting or revoking `OWNER` demands a step-up, and F4-04 has made one possible — the control that
  F3-07 shipped switched off with `auth.step_up_required` as its reason is switched on here, and
  that is the sentence the pull request should carry.
* **Groups.** `GET /groups`, `POST`, `PATCH`, `DELETE` and `GET /groups/{id}` with its members: a
  membership granted to a group is shown as the people it reaches, which is what F3-01 built the
  read for.
* **The matrix.** `PermissionMatrix` from the manifest, reachable from the roles screen, so that
  "what does `CONTRIBUTOR` actually mean here" has an answer inside the product.

**One question this task records and does not answer.** Nothing in this contract disables or
removes an account. `account_status` has `DISABLED`, `POST /accounts/{id}:restrict` is Art. 18 and
means something else, and revoking every membership leaves an account that can still sign in and see
nothing. Offboarding — the person who left the company — has no route, and building one is a product
decision rather than a screen. The issue puts it to the owner with what it would need; the pull
request builds what exists.

**Acceptance:** a person is invited, appears as `INVITED`, redeems through F4-03's screen and becomes
`ACTIVE`; a role is granted at tenant scope and revoked, and `RoleBadge` says which role at which
scope everywhere it appears; granting `OWNER` asks for the step-up and succeeds with it, and is
refused without it in the server's own words; a group is created, renamed, filled, emptied and
deleted, and a membership granted to it shows the accounts it reaches; the permission matrix renders
from the manifest with no compiled-in role list; a caller without `MANAGE_MEMBERS` sees the screen
read-only rather than not at all, where the server permits the read; the offboarding question is in
the issue; `pnpm -r build lint typecheck test` green.

**Read:** ADR-0032; `domain-model.md` §3.2, §5; F3-01 and F3-07 in `milestone-F3.md`; `security.md`
§5 (privileged actions); the `AccountInvite`, `Membership`, `MembershipGrant`, `Group` and
`GroupDetail` schemas

---

## F4-10 — Credentials: personal access tokens, and the accounts that are nothing but access **[L]**

*Depends on: F4-04 (the step-up), F4-06 (`OneTimeSecret`).*

Two screens in two different areas, and the split is ADR-0032's rather than convenience.

**A person's own tokens are profile configuration.** `GET /auth/tokens` lists them — name, scopes,
expiry, last use, and whether somebody pulled it rather than it running out, which are two different
answers to "why did this stop working". `POST /auth/tokens` mints one and the credential is shown
once, through `OneTimeSecret`. Three rules the screen must not soften: the expiry is **mandatory**
and at most a year out, so the field has no default and the form cannot be submitted without it; the
scopes come from `/meta/capabilities` and are requested explicitly, never pre-ticked as everything;
and an **admin scope demands a step-up**, which is F4-04's prompt appearing in a place other than a
destructive operation.

**Service accounts are administration.** `GET` and `POST /auth/service-accounts` under
`MANAGE_MEMBERS`, and their tokens are minted through the same `POST /auth/tokens` with `account_id`
naming the service account — the one exception the contract carves into "a token is its holder's".
The screen says what a service account is for in the catalogue's words — the integration that must
outlive the person who wrote it — and shows the memberships it holds, because a service account with
no membership can do nothing and that is a common confusion rather than a rare one.

The client must not check a token's shape anywhere, for the reason `apps/webapp/CLAUDE.md` already
gives: the security scheme accepts three kinds of credential and a client enforcing one pattern
refuses the other two.

**Acceptance:** a token is minted with a name, explicit scopes and a chosen expiry, shown once, and
is gone from the DOM after the dialog closes; the form refuses to submit without an expiry and
refuses one more than a year out before the server does, with the server's own message code;
requesting an admin scope produces the step-up prompt and succeeds with the grant; the list
distinguishes expired from revoked; a service account is created, its tokens are listed and minted
by naming it, and its memberships are shown; a caller without `MANAGE_MEMBERS` reaches neither the
service accounts screen nor another person's tokens; no token value is logged, stored or put in a
URL; `pnpm -r build lint typecheck test` green.

**Read:** `security.md` §5 (PAT, service accounts, privileged actions); G-01 in `milestone-0.5.0.md`;
the `AccessToken`, `AccessTokenCreate`, `AccessTokenSecret` and `ServiceAccountCreate` schemas;
ADR-0032; `apps/webapp/CLAUDE.md` (the session section)

---

## F4-11 — Third-party apps: the consent screen this contract has been waiting for **[G]**

*Depends on: F4-03, F4-10.*

`POST /oauth/authorize` says it in its own description: "the consent *screen* arrives with the
client track and calls exactly this". Until it exists, an installation that registered a third-party
app has no way for a person to allow it — the authorization endpoint is headless and a person is
what it needs.

Three surfaces:

* **The consent screen** (`end-user`): reached with a client id, the exact redirect URI, the
  requested scopes and the PKCE challenge; it names the app, lists the scopes in the catalogue's
  words rather than as identifiers, states that a scope can never grant more than the person holds,
  and on approval hands the code back to the redirect URI. A public client without PKCE and anything
  but `S256` is refused by the server, and the screen renders that refusal rather than pre-empting it.
* **The grants** (`profile`): `GET /oauth/grants` beside the sessions from F4-03 — which app, which
  scopes, when it was allowed, when it last acted — and `DELETE` to withdraw one, which the screen
  says takes effect on the app's next request.
* **The clients** (`administration`): `GET`, `POST`, `DELETE /oauth/clients` — a name, the exact
  redirect URIs, whether it is confidential. A client secret, where there is one, is shown once
  through `OneTimeSecret`.

The redirect at the end of a consent is a **top-level navigation to a URI the server validated**,
not a fetch, so no policy directive is involved — and the screen navigates to what the server
answered rather than to anything it composed itself.

**Acceptance:** a registered client's authorization request renders the app's name and its scopes as
sentences, and approval produces a code the token exchange accepts, proved end to end; declining
produces no code and no grant; a request with a redirect URI the client did not register, or without
PKCE, renders the server's refusal; the grants list shows what was allowed and withdrawing one
refuses the app's next request; a client is registered and removed, and its secret is shown once;
every scope has a sentence in `locales/en.json` and an unknown scope renders its identifier rather
than nothing; `pnpm -r build lint typecheck test` green.

**Read:** H-05 in `milestone-0.6.0.md`; ADR-0005; the `OauthAuthorization`, `OauthCode`,
`OauthClient` and `OauthGrant` schemas; `core/application/service/identity/Oauth.go`; ADR-0032

---

## F4-12 — The jumble: things arrive, and become work or do not **[L]**

*Depends on: F4-07, F4-08.*

`GET /jumble/entries` newest first, `:convert` into a real entry, `:dismiss` against one, and
`POST /jumble/entries` as the quick capture from inside the product. `JumbleInboxItem` renders a
row; the screen is the inbox around it.

What the screen owes beyond the four calls:

* **An entry is decided about exactly once.** `NEW` → `PROCESSED` or `DISMISSED`, and `settled_at`
  says when. A dismissal is not a deletion: the row stays readable and ages out by retention rule,
  and the screen says that rather than removing it from view.
* **Conversion needs a destination**, and the destination is where the reader already is: the
  collection, the type, the parent. The result is a link to what was produced — `target_item_id` is
  the other half of the provenance pair, and following it is what makes the inbox trustworthy.
* **The intake address.** `POST /jumble/intake:rotate-token` mints the address the workspace accepts
  webhooks and mail on, shown once through `OneTimeSecret`. It is a credential in a URL, which is
  exactly why it is minted rather than displayed on demand, and the screen says what rotating one
  costs: the address in whatever system currently posts to it stops working.
* **The raw content is data from outside.** Rendered as text, never as markup, never as a link the
  reader can click by accident, and the `sender` is labelled as what the transport claimed rather
  than as an identity — a `From` header authenticates nothing, and the contract says so.

No AI (decision 11): `:suggest` is not called and no suggestion control exists.

**Acceptance:** entries arrive from a mail delivery and from a webhook delivery against a real
token and appear in the inbox with their channel; a conversion produces an entry in the chosen
collection, marks the jumble entry `PROCESSED`, and links to what it produced; a dismissal marks it
`DISMISSED`, leaves it readable, and says it will age out; quick capture from inside the product
lands as `QUICK_CAPTURE`; rotating the intake token shows the address once and states what it
breaks; a subject or body containing markup renders as text; an attachment on an entry is reachable
through the media surface F3-09 built; the list pages by cursor; `pnpm -r build lint typecheck test`
green.

**Read:** `automation.md` §4; G-10 and G-11 in `milestone-0.5.0.md`; ADR-0040; the `JumbleEntry`,
`JumbleEntrySubmit`, `JumbleEntryConvert` and `JumbleIntakeToken` schemas; F3-09 in
`milestone-F3.md` (the media surface)

---

## F4-13 — Automation rules: written, read back, switched on **[L]**

*Depends on: F4-07, F4-08, F4-09 (a rule runs as an account, and service accounts are what it
usually runs as).*

The rule editor, and the one contract correction this milestone makes to prose rather than to shape.

**The correction first.** G-06 landed the expression language and `automation.ValidConditionShape`
accepts up to twenty conditions, yet three places in `api/openapi.yaml` still say a condition is
refused: `AutomationRule.conditions` ("Empty in this release"), `RuleThrottle.dedupe_key_expr`
("Refused while it is non-empty") and `createRule`'s own description. All three are corrected here —
prose only, no field renamed, no field removed — because a client built from a contract that
disagrees with its server builds the wrong editor. The neighbouring claim about
`WebhookSubscription.filter` is **true** and stays: `integration.RefuseFilter` refuses a non-empty
one, and F4-15 leaves the field out for that reason.

**The editor.** A rule is a name, a scope (`TENANT`, `HUB` or `COLLECTION` — descendants included by
the ordinary rule, which the screen states because it is the question everybody asks), a `run_as`
account, one of six triggers with only the fields its kind takes, conditions, at least one action,
a throttle, and an error policy. Every list it offers is read rather than compiled in: the trigger's
event types and the action kinds come from `/meta/capabilities`'s `automation` and `event_types`, so
an installation that serves one more action gets one more option without a client release.
`WAIT`, `BRANCH` and `STOP` are the engine's control structures rather than use cases, and the
editor treats them as what they are — `BRANCH` nests, and the nesting is a nested list in the form
because it is a nested list in the contract.

**Two rules the editor must not soften.** A rule is **created switched off**, and `:enable` and
`:disable` are what move it, so the trail says which of the two somebody did — the form has no
"enabled" checkbox at creation. And writing a rule needs the automation permission *and* the rights
the rule's own actions need; the client cannot compute the second half, so it renders the server's
refusal rather than pre-empting it.

The list is `AutomationRuleCard` per rule, with the consecutive failure count shown where it is
climbing, because a rule about to disable itself is the one somebody wants to see.

**Acceptance:** the three stale descriptions are corrected and `make generate` produces no diff; a
rule is written with a trigger, a condition and two actions, appears switched off, is enabled,
disabled and deleted; the trigger and action lists come from the manifest and an action kind the
manifest does not declare is not offered; a `BRANCH` action's nested lists round-trip; a condition
the server refuses renders the server's field error under `/conditions/0/expr`; a rule whose actions
exceed the writer's own rights is refused and the refusal is rendered as the server phrased it; the
failure count is visible; a rule scoped to a hub says that its collections are included; `pnpm -r
build lint typecheck test` green.

**Read:** `automation.md` §1, §2, §3; ADR-0009; G-05, G-06 and G-08 in `milestone-0.5.0.md`; the
`AutomationRule`, `AutomationRuleCreate`, `RuleTrigger`, `RuleAction`, `RuleCondition`,
`RuleThrottle` and `RuleScope` schemas; `core/domain/model/automation/Rule.go`

---

## F4-14 — The dry run, the runs, and the replay **[L]**

*Depends on: F4-13.*

A rule that cannot be tried before it is let loose is a rule nobody dares switch on.
`POST /automation/rules:test` runs one against a sample event and answers what it *would* do;
`:trigger` runs it now for real; `GET /automation/runs` is what they did; `GET
/automation/runs/{id}` is one run with every condition and every action result; `:replay` completes
a failed one.

* **The dry run** is `RunStatusBadge`'s variant rather than a status. The sample event is composed
  in the screen from the trigger's own event type, and what comes back is rendered per action: what
  it would have done, and to what. Nothing is written, and the screen says so where the reader can
  see it beside the results rather than in a heading.
* **The runs list** is where the seven statuses earn their distinctness. `SKIPPED` shows *which*
  condition stopped the run — the contract puts the condition results in the order the rule declares
  them precisely so that this question has an answer — `THROTTLED` says the rule protected the
  workspace, and `ABORTED_LOOP` says the causation depth stopped a rule that triggered itself.
  `RUNNING` on an old row is a crash rather than a state, and the screen does not pretend it is
  progress.
* **The replay** completes a failed run rather than starting a new one, and the screen says which
  actions already succeeded and will not be repeated — that is the difference between a replay and a
  re-trigger, and confusing the two is how somebody sends the same mail twice.

**Acceptance:** a dry run against a sample event answers per-action results and writes nothing,
proved by reading the subject entry afterwards; a manual trigger produces a real run visible in the
list; the list filters by rule and by status, pages by cursor, and renders all seven statuses
distinctly; a skipped run names the condition that stopped it; a failed run is replayed, the
already-succeeded actions are not repeated, and the screen said so before the button was pressed; a
run detail shows the trigger kind, who pulled it where a person did, and the error code as a
sentence from the catalogue; `pnpm -r build lint typecheck test` green.

**Read:** `automation.md` §2, §3; G-07 and G-09 in `milestone-0.5.0.md`; the `RuleRun`,
`RuleRunStatus`, `RuleConditionResult`, `RuleActionResult`, `RuleTest` and `RuleTestResult` schemas

---

## F4-15 — Webhook subscriptions: signed, retried, dead-lettered, sent again **[G]**

*Depends on: F4-06, F4-08.*

An external system's standing request to be told what happens here. `GET` and `POST
/integrations/webhooks`, `PATCH` and `DELETE` on one, `GET .../deliveries`, `:replay` on a delivery,
`:send` one event by hand, and `:rotate-secret`.

* **The signing secret is answered once** — at creation and at each rotation — and shown through
  `OneTimeSecret`. The screen states what rotating costs: deliveries signed with the old secret stop
  verifying at the subscriber, and there is no overlap window in this contract.
* **The target URL is an egress channel.** A private range or the cloud metadata address is refused
  by the guarded client unless the installation deliberately released private networks (T-07), and
  the screen renders that refusal rather than validating URLs itself — what is reachable is the
  installation's answer.
* **The event types come from the manifest**, and a type this build does not emit is refused rather
  than stored, so the picker offers exactly `/meta/capabilities`'s `event_types`.
* **`filter` is left out**, because `integration.RefuseFilter` still refuses a non-empty one
  (decision 12). A field that is accepted and does nothing would be a subscriber receiving events
  they asked not to receive; a field that is shown and always refused would be worse.
* **The deliveries list** is the debugging surface: the attempt number carrying across a replay, the
  response status where the target answered, the error code as a sentence and never the target's
  response body, and when the next attempt is due. `DEAD_LETTER` is what `:replay` acts on, and the
  replay carries the same event id so a subscriber that deduplicates recognises the repeat — the
  screen says that, because it is what makes a replay safe to press.
* **`state`** is `ACTIVE`, `PAUSED` or `DISABLED`, and `DISABLED` is what sustained unreachability
  produced. Re-enabling is an ordinary write; the screen shows `failure_count` and `last_error`
  beside it so that the person re-enabling knows what they are re-enabling.

**Acceptance:** a subscription is created against a local receiver and receives a signed delivery;
the secret is shown once and rotation is stated and works; a target URL in a private range is
refused and the refusal is rendered; the event type picker offers exactly the manifest's types; the
deliveries list shows attempts, response statuses, error codes and the next attempt; a dead-lettered
delivery is replayed with the same event id and the list shows the continued attempt count; a
subscription auto-disabled by failures shows the count and the last error and can be re-enabled;
`filter` appears nowhere; `pnpm -r build lint typecheck test` green.

**Read:** `automation.md` §5; ADR-0007; ADR-0015 and T-07; G-02 and G-03 in `milestone-0.5.0.md`;
the `WebhookSubscription`, `WebhookSubscriptionCreate`, `WebhookSubscriptionUpdate` and
`WebhookDelivery` schemas; `core/domain/model/integration/WebhookSubscription.go`

---

## F4-16 — Backup: the targets, the schedules, the runs, and the verification **[L]**

*Depends on: F4-02, F4-06, F4-08.*

The first screen that watches a job, and the pattern the next three reuse. `POST /backups` answers a
`JobRef` rather than a result; `GET /jobs/{jobId}` is where the answer arrives; `ProgressBar`
renders `progress`, and `null` is the indeterminate case rather than zero.

* **The job watcher belongs to the data layer, once.** A store that polls `/jobs/{id}` with a
  backoff, stops on a terminal status, surfaces `error_code` as a message code, and offers
  `:cancel` where the job takes it. Four screens in this milestone need it and a fifth will; four
  copies of a polling loop is where a bug lives in three of them.
* **Targets**: list, create, `:test` the connection, and — new from F4-02 — delete, with the `409`
  naming the schedules that still use it and the statement that archives at the target are not
  touched. Credentials in a target's configuration are write-only, like the OIDC client secret.
* **Schedules**: the listing F4-02 added, the RRULE and its zone shown as what it means rather than
  as its text (the same reading `RecurrenceEditor` gives, and the same refusal to expand it in the
  browser — ADR-0008 put recurrence behind one library on the server), `next_run_at`, the generation
  plan with `min_keep` as a floor, and `enabled` as the switch that matters.
* **Runs**: what one backup did, and `:verify` at the target, which checks checksums and
  decryptability without restoring and records `verified_at`. A target that has never been verified
  says so, because "we have backups" and "we have backups that open" are different claims.

**Acceptance:** a target is created, tested, used and deleted, and the delete refuses while a
schedule names it; a schedule is created, listed with its next run, switched off and deleted; a
backup is started, the job is watched to `SUCCEEDED` with progress rendered and the indeterminate
case handled, and cancelling a running one works; a failed job shows its `error_code` as a sentence
from the catalogue; `:verify` reports success and failure distinctly and shows when a target was
last verified; the generation plan is shown with `min_keep` stated as a floor; no credential is ever
read back; the job watcher is one module with its own tests; `pnpm -r build lint typecheck test`
green.

**Read:** `backup-restore.md` §4, §5, §6, §10; ADR-0019; ADR-0008; E-01, E-03 and E-05 in
`milestone-0.4.5.md`; the `BackupTarget`, `BackupSchedule`, `BackupRetention`, `BackupRun`, `Job`
and `JobRef` schemas

---

## F4-17 — Restore: the dry run first, and three doors before the destructive one **[L]**

*Depends on: F4-04 (the step-up), F4-16 (the job watcher). The most dangerous screen in the
product.*

`POST /restores` in six modes — `INSPECT`, `SELECTIVE`, `MERGE`, `REPLACE_TENANT`, `NEW_TENANT`,
`INSTANCE` — with `GET /restores/{id}` for the report. The client's job is to make the safe path the
easy one and the destructive path deliberate.

* **`dry_run` is true by default in the contract and the screen never flips it silently.** A restore
  is always run as a dry run first, its report is shown — what would be created, overwritten,
  skipped — and only then is the same request offered without it. The second request is composed
  from the first, so that what was reviewed is what runs.
* **`REPLACE_TENANT` ends credentials, and the screen says so.** The mode replaces the workspace's
  rows and the accounts, sessions and tokens that could sign in to it are among them: a person who
  runs it may not be able to sign back in. That sentence is in the catalogue and on the screen
  before the confirmation, not in a tooltip.
* **Three doors, in this order.** The exact workspace name typed into `confirmation`; a step-up
  token from F4-04, carried in `step_up_token` because this is the one operation where the contract
  puts the proof in the body rather than the header; and `create_safety_backup`, which is on by
  default and whose switching off is its own confirmation naming what it gives up.
* **`INSTANCE` and `NEW_TENANT` are the operator's**, for decision 6's reason — they cross or create
  a tenant — and the screen offers neither. `INSPECT` and `SELECTIVE` are where most people belong,
  and the selection is composed from the archive listing rather than typed.

**Acceptance:** every offered mode runs as a dry run first and the report is rendered before any
non-dry request can be composed; `REPLACE_TENANT` cannot be started without the exact name, a valid
step-up token and an acknowledgement of what it costs, and the sentence about credentials is in
`locales/en.json` and on screen; switching off the safety backup takes its own confirmation;
`INSTANCE` and `NEW_TENANT` are not offered, with the reason stated; a restore without a step-up is
refused by the server and the refusal produces F4-04's prompt rather than a dead end; the job is
watched to its end and the report is shown; `pnpm -r build lint typecheck test` green.

**Read:** `backup-restore.md` §7, §8 (especially §8.3); E-06 in `milestone-0.4.5.md`; `security.md`
§5 (privileged actions); the `RestoreRequest` and `RestoreRun` schemas; ADR-0032 (why this is
administration, and why the phone does not carry it)

---

## F4-18 — Retention, the preview before it deletes anything, and the legal hold **[G]**

*Depends on: F4-02, F4-08.*

`data-retention.md`'s engine, made visible. `GET` and `POST /retention-policies`, `PATCH` and
`DELETE` from F4-02, `POST /retention-policies/{id}:preview`, `POST /items/{id}:retain`, and the
holds at `/legal-holds`.

* **The preview is the point.** A rule that deletes is written, previewed — what it would affect,
  how much of the workspace's holdings — and only then armed. `data-retention.md` §7's rule that a
  new policy starts in notify-only mode when its first run would affect more than 5% is the
  server's, and the screen shows the mode it landed in and why, rather than letting somebody
  discover it later.
* **Extending beyond a kind's upper bound needs a `justification`**, and the field is what it says:
  a sentence somebody will read in an audit, not a checkbox.
* **The effective view.** `?effective=true` answers the rules actually in force including
  inheritance, and that is the reading a container's screen needs: "what happens to what is in
  here" has one answer, and it is rarely the rule written at this level.
* **`:retain` takes one entry out of the running period**, and the screen states that it is not a
  hold — an exception on one object, versus an instruction that overrides everything.
* **Legal holds** are listed, placed with a reason that is mandatory and kept, and released. The
  hold is the first thing every deletion path checks and it overrides the workspace's own periods,
  so the screen puts a placed hold where the retention rules are read rather than on a page of its
  own.

`retention.svelte.ts` already reads `/retention-policies` for the one sentence the trash screen
shows and asks silently, because most members hold no `retention:read`. That module stays as it is
and this screen is the other reader; the pull request says which is which so nobody merges them.

**Acceptance:** a policy is written, previewed with counts, corrected, armed, and withdrawn; a
policy whose first run would exceed 5% lands in notify-only and the screen says so with the reason;
extending beyond the upper bound demands a justification and stores it; the effective listing for a
container shows the inherited rule and where it comes from; `:retain` takes an entry out and the
screen distinguishes it from a hold; a hold is placed with a reason, is visible where the rules are,
and blocks what the preview said it would; a caller with `READ_CONFIGURATION` reads everything and
writes nothing; `pnpm -r build lint typecheck test` green.

**Read:** `data-retention.md` §3, §5, §7, §9; ADR-0020; E-07 and E-08 in `milestone-0.4.5.md`; the
`RetentionPolicy` and `LegalHold` schemas and the inline answer `:preview` gives;
`apps/webapp/src/lib/data/retention.svelte.ts`

---

## F4-19 — The audit: queried, verified, and exported to where it went **[L]**

*Depends on: F4-06, F4-08, F4-16 (the job watcher).*

`GET /audit` with its filters, `POST /audit:verify` for the chain, `POST /audit:export` for the
period — behind `AUDIT_READ`, which an ordinary member does not hold and an auditor holds without
holding `READ`.

* **The query.** Period, actor, action, target, outcome, severity — paged by cursor like everything
  else. An entry carries no user content by design (ADR-0017), so the screen resolves an actor
  identifier to a name through the accounts store the way the activity feed does, and an erased
  account shows the marker its erasure left.
* **The verification.** `:verify` checks the hash chain, and the screen renders the two outcomes as
  the different facts they are: a chain that verifies, and a chain with a break at a named sequence
  number. The second is not an error message — it is the finding the audit trail exists to produce,
  and it is rendered as a finding.
* **The export goes to a backup target, and the browser never sees the archive** (decision 7). The
  form takes a period, a format (`JSONL` or `CSV`) and a target; the job is watched; and what the
  screen shows at the end is **where** the archive was written, to which target and with which
  checksum — not a download link. The export produces an audit entry of its own, which the screen
  says, because an auditor is owed the knowledge that their own export is in the trail.
* **There is no filter on an export**, deliberately: an export narrowed before it was signed would
  be evidence about somebody's selection. The screen states that where somebody would otherwise look
  for the filters they just used in the query.

**Acceptance:** the query filters by every parameter the contract declares and pages by cursor; an
actor resolves to a name, and an anonymised one to its marker; a member without `AUDIT_READ` cannot
reach the screen and an auditor without `READ` can; verification renders success and a break at a
named sequence distinctly; an export is started, watched to completion, and reports the target and
the path rather than offering a download; the screen states that an export is itself audited and
that it cannot be filtered; every sentence is a code in `locales/en.json`; `pnpm -r build lint
typecheck test` green.

**Read:** `audit.md` §3, §4, §5; ADR-0017; E-09 in `milestone-0.4.5.md`; the `AuditEntry` and
`AuditExport` schemas and the two inline answers `:verify` and `:export` give; ADR-0047 (why an
archive at a target is not fetchable); `apps/webapp/src/lib/data/accounts.svelte.ts`

---

## F4-20 — Data subject requests: the case, its deadline, and its close **[G]**

*Depends on: F4-08, F4-16 (the job watcher), F4-19 (the same reader, usually).*

The screen a controller answers a person's request from. `GET` and `POST /privacy/requests`, `PATCH`
on one, `POST /accounts/{id}:restrict`, `POST /privacy/consents:withdraw`.

* **The deadline is the column that matters.** `due_at` is thirty days from receipt unless the
  caller named another, and the list is ordered by what is closest to it, with the same colouring
  the product uses for a due date elsewhere — a statutory deadline is a due date, and inventing a
  second visual language for it would be a design decision nobody took.
* **The six kinds each mean something different**, and the screen says which: `RECTIFICATION` needs
  no special path because a correction is an ordinary write, and is a tracked case all the same
  because the deadline is somebody's responsibility either way.
* **`erasure_mode` is the controller's choice and the default is `ANONYMIZE`** — P-6's decision,
  because tenant data touches third parties' rights. The screen states what each mode does to other
  people's content before the choice is made, and never defaults to `FULL_DELETE` for a case that
  named nothing.
* **`INSTALLATION` scope is not offered.** It crosses the tenant boundary, needs the
  `admin:tenants` scope and is the provider's path rather than a tenant's (decision 6). The screen
  says the case can be raised with the installation's operator.
* **The result archive** goes to a backup target like every other export, so the case shows where it
  was written rather than a download.
* **Restriction and consent withdrawal** are the two Articles with their own routes: `:restrict`
  puts an account into `RESTRICTED`, which is readable and not processed, and the screen says what
  that means for the person's sessions and for the workspace's automations.

**Acceptance:** a case is recorded for an account and for an address with no account, gets its
deadline, and is moved through `IN_PROGRESS` to `COMPLETED` or `REJECTED` with a reason; the list is
ordered by deadline and marks what is overdue; an access or portability case produces an archive and
the screen reports where it went; `erasure_mode` defaults to `ANONYMIZE` and the screen states what
each mode costs; `INSTALLATION` scope is not offered and the alternative is named; restricting an
account states its effect; withdrawing a consent records it; a caller without the privacy permission
reaches none of it; `pnpm -r build lint typecheck test` green.

**Read:** `data-protection.md` §4, §9, §12; ADR-0018; E-10 in `milestone-0.4.5.md`; the
`DataSubjectRequest`, `DataSubjectRequestCreate`, `DataSubjectRequestKind`,
`DataSubjectRequestScope` and `ErasureMode` schemas

---

## F4-21 — The route, walked again — and the one nobody has walked **[L]**

*Depends on: everything. The milestone looking at itself.*

R-08 a third time, and the first walk of the administration area. The first two walks
([2026-09-04](../evidence/R-08-2026-09-04.md), [2026-09-08](../evidence/R-08-2026-09-08.md)) walked
the end-user route: create, organise, assign, comment, date, save a view. This one signs in with a
password, enrols a second factor, invites somebody, grants them a role, mints a token, writes a
rule and watches it run, subscribes a webhook and replays a dead letter, configures a backup target,
runs a backup, verifies it, restores it as a dry run, previews a retention rule, queries the audit
trail and verifies its chain, and records a data subject request — through the application, in a
browser, with `hubctl` used only to set up what a client cannot.

The rules of the walk are the two earlier ones': **no code change is smuggled into this pull
request**. Every gap found becomes an issue, labelled, and the evidence file records what was
attempted and what happened. The comparison section compares the three walks by kind — the first
found four missing surfaces, the second found none and six disagreements about a shape, and what
this one finds about an area that has never had a screen is the interesting number.

Two things this walk can check that the earlier ones could not. **The step-up actually stops
something**: an `OWNER` grant and a destructive restore are both refused without a proof and both
succeed with one, from the application. And **the area tagging is complete**: a test asserts that
every route the client declares carries an area and that the administration set is exactly the
screens this milestone built — the tagging is the deliverable ADR-0032's backlog impact asked for,
and a walk is where an untagged route is noticed.

**Acceptance:** the evidence file exists in the shape `docs/evidence/README.md` describes, dated,
with every step and its result; every gap has an issue with a label, and none is milestoned into a
milestone that is closing; the walk covers sign-in with a password, MFA enrolment and a two-step
sign-in, an invitation redeemed, a role granted with a step-up, a token minted, a rule written,
dry-run, enabled and run, a webhook delivered and a dead letter replayed, a backup run and verified,
a restore dry run, a retention preview, an audit query and a chain verification, and a data subject
request recorded; the route table's areas are asserted by a test; `roadmap.md`'s F4 paragraph is
written the way F1's, F2's and F3's are; no code change is in this pull request.

**Read:** `docs/evidence/README.md`; the two earlier R-08 files; `arc42.md` §11 (R-08); ADR-0032;
F2-16 and F3-20 in the earlier backlogs

---

## The order at a glance

```
F4-01 ──────────────┬── F4-08 ──┬── F4-09 ── F4-11
F4-02 ──────────────┤           ├── F4-12
F4-06 ──┬───────────┤           ├── F4-13 ── F4-14
F4-07 ──┘           │           ├── F4-15
F4-03 ──┬── F4-04 ──┴── F4-10   ├── F4-16 ──┬── F4-17 ─┐
        └── F4-05               ├── F4-18   ├── F4-19 ─┼── F4-21
                                └── F4-20 ──┴──────────┘
```

Five tasks depend on nothing and can start at once: the two core tasks **F4-01** and **F4-02**, the
two component waves **F4-06** and **F4-07**, and the session **F4-03**. F4-04 opens with a draft
ADR and should open early, because it waits on the owner rather than on code. F4-21 is last by
definition.

**Definition of Done for the milestone:** the workspace reads and changes its own settings and the
TOTP enforcement switch is settable from a client; the three pieces of operational configuration
that could only be created can be listed, changed and removed; a person signs in with a password,
refreshes without noticing, enrols a second factor, proves themselves again before the irreversible,
signs in through a company provider where one is configured, and sees and ends the sessions, tokens
and app grants they hold; the seven components the inventory named exist with their stories, and
`design-system.md` §4 lists the four this milestone added; the administration area exists, is tagged
on every route, and is invisible to somebody who holds none of it; the jumble, the rules, the runs
and the webhooks are operable from the application; backups are configured, run, verified and
restored with the destructive mode behind three doors; retention is previewed before it deletes;
the audit trail is queried, verified and exported to a target rather than to a browser; a data
subject request is carried to its deadline; every value still comes from `tokens.json`, the bundle
carries no inline script or style and contacts no origin the policy does not name — asserted byte
for byte, because this milestone was the one most likely to widen it; `go build ./...` and `go test
./...` still succeed with no Node.js installed; and R-08 is answered a third time by a walk through
the area that had never had a screen.
