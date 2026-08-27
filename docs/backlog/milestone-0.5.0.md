# Milestone 0.5.0 — Automation and webhooks

The goal: the system becomes something other systems can build on. Every domain change leaves the
process as a CloudEvent other software can subscribe to, poll for, or react to; a person can write
a rule — trigger, conditions, actions — that does what they would have done by hand, under an
account whose rights bound it, with a log that says what it did and a dry run that says what it
would do; a webhook reaches an external system with a signature, retries, and a dead letter an
operator can read and replay; a script or an integration platform authenticates with a token
somebody minted, scoped, and can revoke; and mail, webhooks and quick captures land in the jumble,
where they become work items by hand or by rule. Phase 1 made the core complete and 0.4.5 made it
operable; this milestone is what makes it programmable.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone: the AI actions `AI_SUGGEST_FIELDS`, `AI_SUMMARIZE`,
`AI_CLASSIFY` and `SuggestFromJumbleEntry` — the AI port arrives at `0.7.0`, and a rule naming one
of them is refused the way a retention rule naming a missing notification category was; OAuth2 for
third-party apps, sessions, OIDC and MFA (`0.6.0` — the PAT stays the only credential, exactly as
`api-guidelines.md` §11 phases it); general resource quotas per tenant (`0.6.0` — the rule engine's
own throttle and dedupe land here, a tenant-wide budget does not); the NATS adapter (`0.6.0`); the
official n8n node and Zapier app (`0.9.0` — this milestone builds the endpoints they will be
generated against); the importers that share the jumble's ingestion path (`0.9.0`); IMAP polling as
a mail intake (see decision 6 — the intake lands webhook-first, and the IMAP adapter waits for its
dependency decision); and every screen that renders any of it — the client builds this surface in
**F4**, one window behind, exactly as `roadmap.md` says it should.

Seven decisions taken while writing this backlog, so that nobody re-derives them:

* **The contract is the empty half this time, and the schema is the full one.** `0001_init` has
  carried `jumble_entry`, `automation_rule`, `rule_run`, `webhook_subscription`, `webhook_delivery`,
  `outbox_event` and `access_token` since phase 0, complete with the `run_as` foreign key, the
  `on_error` check and the failure counter — and `openapi.yaml` carries **no** automation, webhook,
  jumble, trigger-polling or token path at all. Every task's specification-first step is real here,
  and the route shapes are already decided in `api-guidelines.md` §4: `/automation/rules` and
  `/automation/runs` with `:test`, `:trigger`, `:replay`; `/integrations/webhooks` with
  `deliveries`, `:replay`, `:rotate-secret`; `/integrations/triggers/{eventType}`;
  `/jumble/entries` with `:convert`, `:dismiss`; `/auth/tokens` and `/auth/service-accounts`.
* **`cel-go` is the milestone's one new direct dependency, and ADR-0009 chose it two years of
  documents ago.** The same shape as `rrule-go` in 0.4.0 (pre-decided by ADR-0008) and
  `golang.org/x/crypto` in 0.4.5: one task (G-06) promotes and pins it, and no other task
  introduces any. The sandbox limits are part of the decision, not an option: maximum expression
  length, the evaluator's cost limit, 50 ms per expression.
* **Actions are the catalogue, not a list.** `automation.md` §1.3 already rules it: the action
  kind is the use case name in `SCREAMING_SNAKE_CASE`, derived from
  `core/application/usecase.Descriptor` — so the parity gate's third channel, which every use case
  since A-03 has been registered into, is the action surface, and G-07's work is dispatch rather
  than adapters. A new use case becomes an action without anybody editing a list, which is the
  property the whole milestone is named after.
* **Events already have their vocabulary; what is missing is the publication.** `core/domain/event`
  carries the full type set (`de.hubtask.<context>.<entity>.<action>.v1`), the envelope carries
  actor, correlation, causation, `causation_depth` and `replay`, and the outbox with its dispatcher
  has run since phase 0. What no consumer outside this process has ever had is the CloudEvents 1.0
  mapping ADR-0007 promises and the JSON schemas under `api/events/` it says are "versioned like
  the API". G-02 publishes both, and the conformance test — every emitted event validates against
  its schema — is what keeps the promise from drifting.
* **`SCHEDULE` triggers are RRULE through the one engine, and cron waits.** `automation.md` §1.1
  says "Cron or RRULE"; this installation has exactly one schedule engine (D-04, reused by E-05,
  with the DST proof), and a second notation would be a second engine or a translator into the
  first. RRULE lands now; cron notation is recorded as sugar for later rather than scope for this
  milestone. The same task answers the same two questions E-05 had to: what anchors `DTSTART`, and
  the leader-versus-self-seeding shape — and it follows E-05's precedent, not a new one.
* **Mail intake lands webhook-first, and IMAP waits for its dependency decision.** the arc42 context table
  allows both readings: "intake by IMAP poll or an inbound webhook". An IMAP client is a new
  third-party dependency — a supply chain decision this backlog does not take in passing — while
  the inbound webhook needs nothing new and works with every mail-to-webhook bridge an operator
  already runs. G-11 builds the mail *parsing* and the security posture behind a webhook intake;
  open point **AM-1** records that the IMAP adapter needs its dependency chosen by a
  draft ADR before it is scheduled.
* **Four parked halves come home in this milestone, and they are named rather than assumed.**
  R-1 (`data-retention.md` §9): the advance warning — a notification category, its template, and
  the resolution of `COLLECTION_ADMINS` and `TENANT_ADMINS`. The retention `condition`, refused
  since E-07 because the language was not there: honoured once CEL is. A-4 (`audit.md` §9): the
  `AUDITOR`'s configuration reads, which need a read-only configuration permission split out of
  `STRUCTURE` in the role matrix. O-4: whether chaos tests run in CI or nightly. All four are in
  G-12, except the retention condition, which G-06 unlocks and G-12 proves.

---

## G-01 — Tokens somebody can mint, scope, and revoke **[L]**

*Depends on: nothing. Everything that authenticates hangs off it, and it frees the e2e session
from seeding its credential by SQL.*

`CreateAccessToken`, `ListAccessTokens`, `RevokeAccessToken` behind `/auth/tokens`, and
`CreateServiceAccount`, `ListServiceAccounts` behind `/auth/service-accounts` — none of them in the
specification, so the routes are the specification-first step, `GET`/`POST`/`DELETE` per
`api-guidelines.md` §4. The `access_token` table, the `hbt_pat_` verification path and the scope
enforcement in every use case descriptor have all worked since phase 0; what has never existed is a
way to get a token without writing SQL, which is why `scripts/hubctl-e2e.sh` seeds its credential
by hand and why no external integration could authenticate at all.

The shape is `security.md` §3 and it is not negotiable per-field: the token is
`hbt_pat_<32 hex of the tenant>_<43 base64url characters>`, answered **once** at creation and
stored only as SHA-256 with the pepper; the expiry is mandatory and at most one year; the scopes
are the ones the descriptors already declare, requested explicitly rather than defaulted to
everything; last use is visible on the row. A service account is an `account` with
`kind = 'SERVICE_ACCOUNT'` — the enum has carried the value since phase 0 — and it is what G-05's
`run_as` points at, which is why this task comes first: a rule engine whose rules run as people is
a rule engine whose rules die with their author's departure.

Authorisation is deliberately split: one's own tokens are self-service through the account scope
(`UpdateAccountPreferences`'s precedent), a service account and its tokens are `MANAGE_MEMBERS` —
the administrator who answers for access answers for the accounts that are nothing but access.
Revocation is immediate (the hash is checked per request, so a revoked row refuses on the next
call), audited, and separate from expiry on the row so that "it ran out" and "somebody pulled it"
stay distinguishable. Creating and revoking a token are auditable actions with the token id and
never the token; the secret travels in `secret.Secret` from mint to response and appears in no log,
metric, trace or audit entry (rule 10, proved by the SG redaction tests extended to the new path).

**Acceptance:** a token minted through the API authenticates a request, appears in the list with
its scopes, expiry and last use, and refuses on the call after its revocation; the plaintext is in
the creation response and nowhere else — not in the row, not in any log, proved by the redaction
tests; a token request naming a scope the catalogue does not declare is refused with a field error;
the expiry is refused above one year and defaulted to nothing — a caller must choose; a service
account cannot sign in anywhere else (no password path accepts it) and can be `run_as`; the e2e
session mints its PAT through `hubctl token create` and the SQL seeding of the credential is gone;
the five use cases are registered with metric, span and audit declaration, and the parity gate is
green; the cross-tenant negative test proves a token of one tenant is invisible in another.

**Read:** `security.md` §3 (the token table), §4 (T-01); `api-guidelines.md` §4, §11;
`domain-model.md` §5 (Integration); ADR-0005

---

## G-02 — CloudEvents leave the process, and the dispatcher earns "production-ready" **[L]**

*Depends on: nothing. Runs alongside G-01; every consumer task hangs off it.*

The CloudEvents 1.0 mapping ADR-0007 promised — in the adapter, where the ADR put it: the envelope
becomes `id`, `source`, `type`, `time`, `subject` plus the extension attributes `tenantid`,
`actor`, `correlationid`, `causationid`, `causationdepth`, and `replay` present-only-when-true, so
a consumer written before restores existed reads an ordinary event exactly as it always did. The
JSON schemas land under `api/events/`, one per event type, versioned like the API — and they are
checked, not trusted: a conformance test renders every event type the domain can emit and validates
it against its schema, so the schemas cannot drift from the code that produces the events. Whether
the schemas are generated from the Go definitions or written and asserted is the task's own
decision; what is not optional is that `gate-docs` or `gate-contract` fails when they disagree.

The dispatcher half closes the gap between "runs since phase 0" and what ADR-0007's countermeasures
require of production: `LISTEN/NOTIFY` as the wake-up with the adaptive poll as the fallback, so
that delivery latency is not the polling interval; retention for `outbox_event` — the table grows
forever today, and the cleanup follows the retention engine's shape (a data kind, not a second
engine, keeping decision 4 of the 0.4.5 backlog intact); and the **outbox lag metric with its
alert**, which `observability-reliability.md` has been naming since phase 0 the way A-12 was named
before E-05 built it. Consumer-side deduplication becomes the library function the ADR promised
(`event_id` within a window), so G-03 and G-07 consume it rather than each writing their own.

**Acceptance:** every event type the domain emits validates against its schema under `api/events/`,
in a gate, and a deliberately wrong field turns the gate red in `gate-selftest`; the CloudEvents
mapping carries the six extension attributes and `replay` is absent on an ordinary event, proved by
a golden rendering; a write in one tenant reaches a subscribed consumer within the notify latency
rather than the poll interval, proved with a timing assertion tolerant enough for CI; dispatched
outbox rows are cleaned up by a rule whose kind is in the data-kind catalogue, and a row not yet
consumed by every consumer is not; the lag metric is emitted, its alert has a runbook, and the
alert is exercised against a stalled consumer in a test; the dedupe helper is used by at least the
SSE consumer in this task, so it exists as a library rather than a document.

**Read:** ADR-0007; `core/domain/event` (the envelope and the type set);
`observability-reliability.md` §3, §8; `data-retention.md` §3 (the closed data-kind set)

---

## G-03 — Webhook subscriptions: signed, retried, dead-lettered, replayable **[L]**

*Depends on: G-01 (the secret is sealed the way a target credential is), G-02 (it consumes the
CloudEvents stream).*

`CreateWebhookSubscription`, `UpdateWebhookSubscription`, `DeleteWebhookSubscription`,
`ListWebhookSubscriptions`, `ListDeliveries`, `ReplayDelivery`, and `:rotate-secret` — the routes
`api-guidelines.md` §4 already shapes under `/integrations/webhooks`. The subscription carries
`target_url`, `event_types[]`, an optional CEL filter (accepted but inert until G-06 lands, and
**refused** rather than stored-and-ignored until then — E-08's `ACCOUNT` lesson), and a scope; the
payload is the CloudEvent of G-02, identical to the internal event, because "no feature available
only internally or only externally" is the document's first sentence.

The delivery discipline is `automation.md` §3.1 verbatim, and each clause is a test: the signature
`X-Hubtask-Signature: t=<ts>,v1=<hmac-sha256(secret, ts + "." + body)>` with the replay window; the
three headers (`X-Hubtask-Event-Id`, `X-Hubtask-Event-Type`, `X-Hubtask-Delivery-Attempt`); eight
attempts with backoff to 24 h and then the dead letter, visible under `deliveries` and replayable
by hand; auto-disable after sustained unreachability with a notification to the owner — the
notification category C-09 built, not a new channel. Zapier's REST-hooks pattern
(`subscribe`/`unsubscribe` through the API) falls out of the CRUD; the task proves it against
Zapier's documented flow rather than assuming it.

Security is the backup target's shape, deliberately: the secret is generated server-side, sealed
through E-02's `Encryptor`, answered once, rotated via `:rotate-secret` with a grace period in
which both secrets verify (a subscriber cannot deploy atomically), and masked everywhere; every
delivery goes through `GuardedClient` with private ranges blocked unless released (rule 6 — a
webhook target is an egress channel exactly as a backup target is, and T-07 covers both); and a
delivery carries the event, never more — no enrichment that would widen what a subscriber sees
beyond what the event already says.

**Acceptance:** a subscribed URL receives a signed CloudEvent whose signature verifies against the
documented formula, and a tampered body does not; attempt eight lands in the dead letter, the
deliveries listing shows every attempt with its status, and a replayed delivery carries a fresh
attempt header and the same event id; rotation keeps the old secret verifying for the grace period
and not after it, proved with a clock rather than a sleep; a subscription pointed at
`169.254.169.254` or a private range is refused by `GuardedClient` and released only by explicit
configuration (BK-9's sibling, and the test names it); auto-disable fires after the configured
failure run, notifies the owner through the existing notification path, and re-enabling is an
audited write; the CEL filter field is refused until G-06, then accepted, and the refusal test
flips to an acceptance test in G-06's pull request; no secret appears in any log, metric, trace,
audit entry or API response after creation, proved by the redaction tests; the use cases are
registered and the parity gate is green; the cross-tenant negative test proves subscriptions and
deliveries of one tenant are invisible in another.

**Read:** `automation.md` §3.1; ADR-0007; ADR-0015, `security.md` §4 (T-07); E-02 and E-03 in
`milestone-0.4.5.md` (the sealing and egress precedents); `api-guidelines.md` §4

---

## G-04 — Trigger polling: the cursor for platforms without a URL **[G]**

*Depends on: G-02. Small, and independent of everything after it.*

`GET /integrations/triggers/{eventType}?since=<cursor>&limit=100` per `automation.md` §3.2:
events of one type, ascending, with a stable cursor and `event_id` for deduplication — the pull
half of the same stream G-03 pushes, for n8n instances behind NAT and Zapier's polling triggers.
The events answered are the tenant's own, shaped exactly as the webhook payload (one schema, two
transports), and the cursor is opaque and survives a restart — which means it is derived from the
outbox's ordering rather than from anything a process invents.

Two boundaries the task must draw out loud. **Retention bounds the window**: G-02 gave dispatched
outbox rows a retention kind, so a cursor older than the retention answers a stable, documented
refusal (`410`-shaped, with a message code) rather than silently starting over — a poller that
missed more than the window must know it missed, or it invents consistency that is not there.
**Authorisation is the event's, not the endpoint's**: a token scoped to read items polls item
events and is refused audit events, which falls out of mapping event types to the scopes the
catalogue already declares rather than inventing a polling permission.

**Acceptance:** two polls with the returned cursor never overlap and never gap under concurrent
writes, proved with interleaved writers; the cursor survives a server restart; a cursor older than
the outbox retention answers the documented refusal with a message code, not an empty page; the
payload validates against the same `api/events/` schema the webhook delivery does; scope
enforcement is per event type with a test that tries the wrong scope; the route is in the
specification, registered, and the parity gate is green; the cross-tenant negative test holds.

**Read:** `automation.md` §3.2, §3.3; G-02's retention decision; `api-guidelines.md` §4, §6

---

## G-05 — The rule model and its writers **[L]**

*Depends on: G-01 — `run_as` points at a service account.*

`CreateRule`, `UpdateRule`, `EnableRule`, `DisableRule`, `DeleteRule`, `ListRules` over
`automation_rule` — the table that has carried the whole model since `0001_init`: scope with the
tenant/hub/collection precedence, `run_as` with its composite foreign key, `trigger`, `conditions`,
`actions`, `throttle`, `on_error` with its three-value check, the failure counter. The routes are
the specification-first step under `/automation/rules`.

Validation is the substance, because a rule is data that will later be executed with somebody's
rights: the trigger must be one of the six kinds and well-formed for its kind; every action must
name a use case that exists in the catalogue **and** that the `run_as` account's role could invoke
at the rule's scope — checked at write time as a courtesy *and* at run time as the boundary (a role
change between the two must narrow the rule, not widen a stale check); parameters are validated
against the descriptor's declared inputs, the way the registry already refuses undeclared keys.
Conditions are accepted as opaque text in this task and **refused as non-empty** until G-06 can
parse them — stored-and-ignored is not available (E-08's lesson), and the two tasks' tests trade a
refusal for an acceptance the way G-03's filter does. Actions naming `SEND_WEBHOOK`,
`HTTP_REQUEST`, `WAIT`, `BRANCH`, `STOP` or an AI kind are likewise refused until the task that
serves each lands, so the accepted vocabulary is exactly the executable vocabulary at every commit.

Authorisation follows the matrix's Automation column: writing a rule at a scope requires the
automation permission there, and additionally the writer must hold the rights the rule's actions
need — otherwise a member with automation rights and nothing else launders a privilege through a
generously-scoped service account. That composition rule is the task's sharpest decision and it is
recorded in `automation.md` §2 where the permission row already is, not only in code.

**Acceptance:** a rule round-trips through create, update, disable, enable, delete, with
optimistic concurrency on `version`; a rule naming an unknown action, an undeclared parameter, an
out-of-scope use case, or a `run_as` the writer may not use is refused with a field error naming
which; a non-empty condition is refused with a message code that says the language arrives with the
engine — and G-06's pull request flips exactly that test; enabling is separate from creating and
separately audited; the automation permission is enforced per scope with the existing matrix tests
extended; deleting a rule soft-deletes and its runs stay readable; the use cases are registered,
the parity gate is green, and the cross-tenant negative test holds; the new personal-data rows
(rule names can carry them — they are titles) are in the data catalogue with their deletion path,
and merge rules for the new fields are in `offline-sync.md` §4.2.

**Read:** `automation.md` §1, §2; ADR-0009; `domain-model.md` §3.2 (the Automation column), §5;
`descriptor-fields-are-validated` precedent (C-07)

---

## G-06 — CEL: the language, its limits, and the environment **[L]**

*Depends on: nothing technically; G-05 and G-03 hold refusals it will flip.*

The milestone's one new direct dependency lands here and nowhere else (decision 2): `cel-go`,
pinned, licence-gated, promoted exactly as `rrule-go` was in D-04. What this task builds is the
**evaluator as a port** — `core/port/expression` or its equivalent — so the domain describes what a
condition is and the adapter knows how CEL evaluates one, with the sandbox limits from
`automation.md` §1.2 enforced in the adapter and asserted in tests: maximum expression length, the
evaluator's cost limit, the 50 ms timeout per expression. No I/O, no loops, terminating — those are
CEL properties, and the tests prove the configuration keeps them (a regex bomb, a deeply nested
expression, a long string concatenation each hit a limit rather than a CPU).

The environment is the contract's half: `event`, `item`, `parent`, `collection`, `hub`, `actor`,
`now`, `payload`, `tenant.settings`, plus the date, set and string library functions §1.2 names.
`now` comes from the `Clock` port — an expression evaluated twice in one run sees one instant —
and the variables are built from the event and the read model lazily, so a condition touching only
`event` costs no reads. Templating in action parameters (`"Reminder: " + item.title`) is the same
environment applied to strings, built here, consumed by G-07.

Two consumers get their refusals flipped in this same pull request, because a language nobody
consumes is not proven: G-05's rule conditions validate at write time (parse errors become field
errors with the parser's position) — execution still waits for G-07 — and the retention rule
`condition` E-07 has refused since 0.4.5 is accepted, parsed, and honoured by the retention sweep,
with RE-tests extended to a conditioned rule. That second consumer is deliberate: it proves the
port serves an engine other than the rule engine, which is what makes it a port rather than a
private helper.

**Acceptance:** `cel-go` is a direct dependency, pinned, and the licence gate stays green; the
three limits each turn a hostile expression into a typed refusal, proved per limit; the environment
exposes exactly the documented variables — an expression naming anything else fails at parse — and
`now` is the `Clock`'s; a write-time parse error names its position in a field error; a retention
rule with a CEL condition sweeps only what matches, RE-1…RE-9 stay green, and the E-07 refusal test
is flipped, not deleted; `gate-architecture` proves `cel-go` appears in no layer inwards of the
adapter; templating renders against the same environment with the same limits.

**Read:** ADR-0009; `automation.md` §1.2; `data-retention.md` §2 (the condition field); D-04 in
`milestone-0.4.0.md` (how a dependency lands); ADR-0001

---

## G-07 — The engine: an event becomes a run becomes actions **[L]**

*Depends on: G-02, G-05, G-06. The milestone's centre.*

The automation engine as a consumer of the dispatcher: an `EVENT`-triggered rule matches an event,
its conditions evaluate, its actions dispatch into the use case registry as the `run_as` account,
and a `RuleRun` records all of it — condition results, per-action outcomes, timestamps, errors —
behind `ListRuleRuns` and `GetRuleRun` under `/automation/runs`. This is where `automation.md` §2's
table stops being documentation: at-least-once delivery with the `Idempotency-Key` derived from
`(rule_id, event_id, action_index)`, so a redelivered event re-runs into stored results rather than
double effects; the run's actor is the service account with its real rights, checked by the
authoriser exactly as a person's would be (rule 2 — the engine gets no bypass, which is the whole
point of `run_as`); loop protection aborts at `causation_depth` 5 with run status `ABORTED_LOOP`;
throttle and `dedupe_key_expr` bound a storm per rule; `on_error` does what its three values say;
and after n consecutive failures the rule disables itself and notifies its owner — the counter has
been in the table since phase 0.

Replays are the promise E-06 wrote into the envelope: the dispatcher hands a replay to no
subscriber that has not asked, the engine never asks, and the test proves a restored change fires
nothing — BK-5's automation half, now against the real engine rather than a spy. Events the engine
itself causes carry the incremented depth through `Cause`, which D-track events already do.

The scheduling shape follows the queue's grain: one event may match several rules, and each
matching rule is one `automation.run` job — failure isolation per rule, the queue's backoff per
rule, and the dead letter naming which rule rather than which batch. The engine is the third
channel the parity gate has always asserted; what this task adds is the *asynchronous* path into
it, and the gate's existing synchronous parity (REST, MCP, automation-as-channel) stays untouched.

**Acceptance:** an event matching an enabled rule produces a run whose condition results and action
outcomes are readable through the API; a redelivered event produces no second effect, proved by
idempotency; a rule chain event→rule→event→rule aborts at depth 5 with `ABORTED_LOOP` on the run;
a `run_as` account lacking a right fails that action with the authoriser's refusal on the run — and
the run's audit entries name the service account as actor; a restored change (`replay`) fires
nothing; the throttle holds a mass change to its bound and the dedupe key collapses a storm, proved
with a bulk update; `on_error` `STOP` stops, `CONTINUE` continues, `RETRY` retries with backoff and
lands in the dead letter after the budget; the failure counter disables the rule at its threshold
and the owner is notified; `automation.rule_run_started`/`finished`/`failed` events are emitted;
the new use cases are registered with metric, span and audit declaration, and the parity gate is
green.

**Read:** `automation.md` §1.3, §2; ADR-0009; ADR-0007; ADR-0005 (rule 2); `security.md` §4 (T-xx
egress rows); E-06's replay decision in `milestone-0.4.5.md`

---

## G-08 — The other four triggers **[G]**

*Depends on: G-07. Each trigger is a producer into the same engine.*

`SCHEDULE`, `RELATIVE_DATE`, `MANUAL` (`TriggerRuleManually` behind `:trigger`), and
`INBOUND_WEBHOOK` — four ways a run starts that are not a domain event, each producing into the
engine G-07 built rather than a second execution path.

`SCHEDULE` is RRULE through the one engine (decision 5), with D-04's golden DST expectations
extended to a rule firing at 03:00 through both transitions, and the leader/self-seeding question
answered the way E-05 answered it: a tenant-scoped schedule is seeded by the write that creates the
rule, the due index is read the way `backup_schedule_due_idx` is, and nothing enumerates tenants.
`RELATIVE_DATE` ("24 h before due") produces occurrence jobs the way reminders do — D-02's shape,
not a new one — and recomputes when the anchor moves, which is the test that matters.
`MANUAL` is the smallest: a registered use case, the automation permission at the rule's scope, and
the run records who pulled the trigger. `INBOUND_WEBHOOK` mints a token-protected URL per rule —
the calendar feed's credential discipline (D-08): hashed, shown once, revocable by rotating — and
the payload enters the CEL environment as `payload`, size-capped and parsed defensively; the
inbound path authenticates the *rule*, never a person, and can do nothing but start that rule's
run.

**Acceptance:** a scheduled rule fires at 03:00 local through both DST transitions against the
golden expectations; a relative-date rule fires at the offset, refires correctly when the due date
moves, and does not fire for an anchor that was cleared; `:trigger` runs the rule now, records the
actor, and respects the same throttle; an inbound webhook URL starts its rule and only its rule,
refuses a wrong token, survives rotation, and a 2 MB payload is refused at the boundary; all four
carry the run log, the loop depth and the throttle exactly as `EVENT` runs do, proved by shared
tests over the trigger kinds; the use cases are registered and the parity gate is green.

**Read:** `automation.md` §1.1; D-02, D-04 in `milestone-0.4.0.md`; E-05's scheduling decision in
`milestone-0.4.5.md`; D-08's token discipline; `multi-tenancy.md` §2.1

---

## G-09 — Flow and outbound actions, the dry run, and the replay **[L]**

*Depends on: G-07; G-03 for `SEND_WEBHOOK`.*

The action vocabulary G-05 refused, served: `WAIT` (a delay as a job — the run suspends and
resumes, it does not sleep on a worker), `BRANCH` (a nested condition through G-06), `STOP`,
`SEND_WEBHOOK` (a delivery through G-03's machinery, to a named subscription), and `HTTP_REQUEST`
(method, headers, body template, optional signature) — the last one the riskiest surface in the
milestone, and it gets the full backup-target treatment: `GuardedClient` with private ranges
blocked unless released, header secrets sealed through E-02 and masked everywhere, response size
and time bounded, and the response available to nothing (a rule cannot read an answer in 0.5.0 —
that would be a data source in conditions, which ADR-0009 excluded; the refusal is documented in
`automation.md` rather than silently true).

`TestRule` behind `:test` is the dry run `automation.md` §2 promises: a sample event in, which
conditions matched and which actions *would* run out, no side effects — the restore's dry-run
discipline (nothing below opens a writing transaction) applied to rules. `ReplayRuleRun` behind
`:replay` re-executes a failed run's remaining actions under the same idempotency keys, which is
what makes it a completion rather than a duplication, and it is audited with the replayer.

**Acceptance:** a `WAIT` of a day holds no worker, survives a restart, and resumes on time (the
queue's `run_at`, proved with the fixed clock); `BRANCH` takes the branch its condition says and
the run log shows both the condition and the path; `SEND_WEBHOOK` delivers through the same
pipeline as a subscription (one delivery table, one retry discipline); `HTTP_REQUEST` to a private
range is refused unless released, its header secret never appears anywhere after creation, and a
response is discarded unread past the size bound; `:test` changes nothing, proved by the checksum
discipline, and reports condition results for the sample event; `:replay` completes a
half-finished run without repeating its finished actions; every refusal G-05 held for these kinds
is flipped in this pull request; parity gate green, audit declarations present.

**Read:** `automation.md` §1.3, §2; ADR-0009 (no external data in conditions); ADR-0015/T-07;
E-06's dry-run discipline in `milestone-0.4.5.md`

---

## G-10 — The jumble: entries arrive and become work **[L]**

*Depends on: G-07 for the `JUMBLE_ENTRY` trigger; the entity and the near intakes need nothing
before them.*

`SubmitJumbleEntry`, `ListJumbleEntries`, `ConvertJumbleEntry`, `DismissJumbleEntry` over
`jumble_entry` — the table with its four channels (`EMAIL`, `WEBHOOK`, `QUICK_CAPTURE`, `API`),
raw subject and body, attachments, sender and status, untouched since phase 0. Routes per
`api-guidelines.md` §4: `/jumble/entries` with `GET`, `POST`, `:convert`, `:dismiss`. The near
intakes land here: quick capture and API are `SubmitJumbleEntry` with a channel, and the **webhook
intake** is `presentation/intake/WebhookIntake.go` — a token-protected URL per tenant, the same
credential discipline as G-08's inbound webhook, accepting a small JSON shape and storing it as an
entry. Conversion produces a `WorkItem` at a named destination, records the provenance
(`origin_jumble_id`, which the archive already carries), sets `PROCESSED`, and emits
`jumble.entry_converted`; dismissal is a state, not a deletion, and the retention engine gets a
`JUMBLE_ENTRY` data kind so dismissed entries age out by rule rather than by hand — the closed-set
change D-06 predicted ("obvious candidate for 0.4.5", arriving one milestone later with its
feature).

Jumble content is the least trusted text in the system and the task treats it that way: raw
subject and body are `PERSONAL_CONTENT` in the data catalogue with the deletion path, never in
logs (rule 10), and never rendered as instructions to anything — `ai-first.md` already rules that
for the AI that arrives in 0.7.0, and the CEL environment exposes entry fields as *data* under
`payload` with the same discipline. The `JUMBLE_ENTRY` trigger fires the engine on arrival, which
is what "the basis for automatic conversion" means: a rule calls `CONVERT_JUMBLE_ENTRY` with a
destination, and the conversion runs as the rule's `run_as` with its real rights.

**Acceptance:** an entry submitted through each near channel (API, quick capture, webhook) lands
with its channel, is listable, and fires `jumble.entry_received`; conversion creates the item at
the named destination with provenance set and the entry `PROCESSED`, and a second conversion of
the same entry is refused; dismissal keeps the entry readable and the new retention kind ages it
out, proved through the retention engine's existing tests extended by the kind; the webhook intake
refuses a wrong token, caps payload size, and its token is shown once and rotatable; a rule on
`JUMBLE_ENTRY` converts an arriving entry end to end; raw content appears in no log, metric,
trace or audit entry, proved by the redaction tests; catalogue rows and merge rules exist for
every new field; use cases registered, parity green, cross-tenant negative test holds.

**Read:** `domain-model.md` §2 (JumbleEntry), §5 (Jumble); `automation.md` §1.1;
`api-guidelines.md` §4; `data-retention.md` §3; `ai-first.md` (content is data)

---

## G-11 — Mail becomes jumble: the parser and the webhook-first intake **[G]**

*Depends on: G-10. The `EMAIL` channel's producer.*

`presentation/intake/MailIntake.go`: an inbound mail, however it arrives, becomes a jumble entry
with subject, body, sender and attachments — the attachments through C-05's media pipeline with
its size and type discipline, never a second storage path. What arrives in 0.5.0 is the
**webhook-first** transport (decision 6): an operator points a mail-to-webhook bridge — their MTA,
their mail provider's push, any of the services that do exactly this — at the mail intake URL, which
authenticates per tenant the way G-10's webhook intake does. The parser is the durable half and it
is transport-independent: MIME walked defensively with stdlib, HTML stored as text alongside,
encodings normalised, attachment count and sizes bounded, and a mail that defeats the parser lands
as an entry with the raw payload attached rather than vanishing — a jumble exists to catch, and
"unparseable" is a thing to catch.

Security posture, stated because mail is hostile input: no HTML is ever rendered server-side;
sender addresses are data, not identities (a From header authenticates nothing — the intake token
does); the entry records the envelope enough for a person to judge provenance; and the parser runs
under the size bounds before allocation, so a crafted mail costs a refusal rather than memory.
Open point **AM-1** stays open: IMAP polling needs a client library, a new dependency this
milestone does not take in passing — a draft ADR proposes it when the webhook path has shown what
the parser needs, and the port cut here is what makes that an adapter rather than a rewrite.

**Acceptance:** a MIME mail with text, HTML and two attachments becomes one entry with both
attachments in the media store under their checks; a 30 MB attachment and a 200-part MIME bomb are
each refused at their bound with a message code; an unparseable payload still lands as an entry
carrying the raw bytes; the sender is stored and never trusted, and the intake refuses a wrong
token; a rule on `JUMBLE_ENTRY` converts an arriving mail end to end — the milestone's
mail-to-task demo, asserted in the e2e session; AM-1 is recorded in `automation.md` §5 (new
section, open points) with what the ADR must decide; the parser is fuzzed
(`go test -fuzz` corpus committed) because this is the one boundary that eats arbitrary bytes.

**Read:** `automation.md` §1.1; arc42 §3 (the email context row); C-05 in `milestone-0.3.0.md` (the
media discipline); `security.md` §4; G-10's token discipline

---

## G-12 — The parked halves come home **[G]**

*Depends on: G-06 (CEL for nothing here — the retention condition landed there), G-07 (the
notification path is exercised); independent of the jumble track.*

Four things earlier milestones parked with a date, due now, none of them large, each one owed:

* **R-1** (`data-retention.md` §9): the retention advance warning — a notification category, its
  template in `locales/en.json`, and the resolution of `COLLECTION_ADMINS` and `TENANT_ADMINS` to
  accounts through the membership model. A rule with `notify` set stops being refused; the refusal
  test flips; the warning fires in phase one with the days remaining, through C-09's notification
  machinery, and RE-4's retain-after-warning path is proved against a real notification rather
  than a refusal.
* **R-2**: the referential safeguard's direction — decided, written into `data-retention.md` §4.6,
  and if the answer changes the conservative default, the sweep tests change with it in the same
  pull request. A decision recorded is the deliverable; the backlog does not pre-take it.
* **A-4** (`audit.md` §9): the `AUDITOR`'s configuration reads. The read-only configuration
  permission is split out of `STRUCTURE` in the role matrix (`domain-model.md` §3.2), the ladder
  roles keep their exact effective rights (proved by the matrix tests asserting no widening and no
  narrowing for every existing role), `AUDITOR` gains the new permission, and the configuration
  reads — targets, schedules, retention rules, holds, automation rules, webhook subscriptions,
  never a secret — accept it. The audit access-model tests from E-09 grow the configuration half.
* **O-4** (`observability-reliability.md` §9): chaos tests in CI or nightly — decided by measuring
  what they cost on the shared runners, written into `ci-cd.md` with the reasoning, and wired
  where the decision says. A decision with a measurement behind it, not a preference.

**Acceptance:** each of the four ends with its document updated, its refusal (where one existed)
flipped rather than deleted, and its tests extended as named above; the role matrix change is
proved non-widening for every pre-existing role; `versioning-release.md`'s release table rows that
reference these stop pointing at open points.

**Read:** `data-retention.md` §9; `audit.md` §5, §9; `domain-model.md` §3.2;
`observability-reliability.md` §9; C-09 in `milestone-0.3.0.md`

---

## G-13 — hubctl grows with the milestone **[G]**

*Depends on: G-01 … G-12. The last task.*

B-13 built the CLI as the dogfooding client and every milestone since has grown it; the same here:
`hubctl token create/ls/revoke`, `hubctl service-account create/ls`, `hubctl rule
add/ls/enable/disable/test/trigger`, `hubctl rule runs/run show/replay`, `hubctl webhook
add/ls/rm/deliveries/replay/rotate-secret`, `hubctl jumble ls/submit/convert/dismiss`, and
`hubctl events poll` over G-04's cursor — the client types generated from the contract, a file per
group and a line in `groups()`.

Two things earn more than convenience. The session's own credential is finally honest: it mints
its PAT through the API it tests (G-01), so the first thing the session proves is the auth surface
itself. And the milestone's demo is scripted end to end: a rule that watches the jumble, a mail
arriving through the webhook intake, and the work item existing afterwards with provenance — the
automation counterpart of E-12's restore drill, asserted by reading the item back rather than by
eyeballing a log.

**Acceptance:** the scripted session grows the milestone's verbs — mint a token and use it, create
a service account, write a rule, dry-run it, watch a webhook delivery arrive signed and verify the
signature in the script, poll the trigger endpoint with a cursor, submit a jumble entry by webhook,
see the rule convert it, read the run log, replay a failed delivery — and stays green against the
Compose stack; `--json` stays pipeable; errors render through the message-code catalogue; secrets
print once with the warning on stderr (D-09's `calendar mint` discipline); the support matrix rows
still pass on every platform B-15 declared.

**Read:** `api-guidelines.md`; B-13, C-13, D-09, E-12 precedents; `support-matrix.md`

---

## The order at a glance

```
G-01 ─┬─────────────── G-05 ─┐
      │                      ├─ G-07 ─┬─ G-08 ──┐
G-02 ─┼─ G-03 ─┐  G-06 ──────┘        ├─ G-09 ──┤
      └─ G-04  └──────────────────────┤         ├─ G-13
                                      ├─ G-10 ── G-11
                                      └─ G-12 ──┘
```

G-01, G-02 and G-06 depend on nothing and start at once; G-03 and G-04 hang off G-02; G-05 needs
only G-01. G-07 is where the three meet, and everything after it fans out: the four other triggers,
the flow and outbound actions, the jumble with its mail, and the parked halves. G-13 comes last: it
consumes every channel the others opened.

**Definition of Done for the milestone:** the Automation, Integration and Jumble sections of the
use case catalogue are implemented for the 0.5.0 scope, each through REST, MCP and automation with
the full gate suite green; the CloudEvents schemas under `api/events/` validate every emitted
event and `gate-selftest` proves the check can go red; a webhook subscriber receives signed
events, survives its own outage through the retry and the dead letter, and can be replayed; a
platform without a URL polls the same stream with a stable cursor; a rule with a CEL condition and
catalogue actions runs under a service account's bounded rights, aborts loops, honours throttle,
logs every run, and can be dry-run and replayed; a mail arriving through the webhook intake
becomes a jumble entry and, by rule, a work item with provenance; a token can be minted, scoped,
listed and revoked without SQL, and the e2e session authenticates through it; R-1, R-2, A-4 and
O-4 are closed with their documents updated; `cel-go` is the milestone's only new dependency; and
the scripted session demonstrates the mail-to-task chain end to end against the Compose stack.
