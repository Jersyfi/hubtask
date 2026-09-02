# Implementation Plan

Versioning follows [SemVer](./architecture/versioning-release.md). Before `1.0.0`, minor releases
may contain breaks; with `1.0.0`, API v1 is promised stable. There is **one** version for the
product and every first-party client ([ADR-0035](./adr/ADR-0035-one-product-version.md)).

The order follows the requirement: **the core is built completely first**, then automation and
operations. The client track no longer waits for the end of that — it runs **alongside from
`0.4.0`**, one milestone window behind the core, so that every screen is built on a contract that
has already settled (phase 5). Both tracks meet in the convergence milestone `0.9.5`, and `1.0.0`
is released when the server, the clients and the website are finished together.

---

## Phase 0 — Foundation (`0.1.0`) · Walking skeleton

| Epic | Contents |
|---|---|
| Repo setup | Instantiate the template, work through the checklist in [project-structure.md](./architecture/project-structure.md) §5, `LICENSE`, `CLA`, `SECURITY.md`, `CONTRIBUTING.md` |
| Toolchain | `Makefile`, `golangci-lint`, `sqlc`, `goose`, `oapi-codegen`, architecture tests, GitHub Actions with every gate |
| Skeleton | `cmd/server` with roles, the environment port, the four health levels (`/healthz`, `/startupz`, `/readyz`, `/meta/health`), OpenTelemetry, structured logs with redaction, `SafeGo`, graceful shutdown |
| Security baseline | Gates SG-1…SG-12 in the pipeline (initially against the reference use case), `GuardedClient`, security headers, rate limits, Argon2id, token hashing, the secret type, `SECURITY.md`, secret scanning, SBOM + image signature |
| Resilience baseline | `infrastructure/resilience` (timeouts, breaker, retry, bulkhead, load shedding), panic middleware, the degradation registry, tests RT-1…RT-4 |
| Observability material | The basic metric catalogue, trace propagation across the outbox and jobs, `deploy/observability/` with a first dashboard, alert rules A-03/A-04/A-05/A-07/A-12 and their runbooks |
| Audit skeleton | `audit_log` with the hash chain and grants (no `UPDATE`/`DELETE` for the app role), the `AuditableAction` registry, gate SG-13 and test AT-1 on the reference use case |
| Data protection skeleton | The data catalogue created and checked in the gate, field classification, log and metric redaction, the `retention_policy` table with defaults |
| Sync foundation | `change_log`, `tombstone`, HLC generation, and position fractional indices in the data model — not introducible later without a break ([ADR-0021](./adr/ADR-0021-offline-sync.md)) |
| Persistence | The PostgreSQL connection, UnitOfWork, the tenant context (`SET LOCAL`), the RLS skeleton, the first migration |
| API skeleton | `openapi.yaml` with `/meta/*`, the generated router, problem details, middleware (auth stub, locale, request ID, rate limit) |
| Reference use case | `CreateContainer` end to end: REST + MCP + automation registration + tests + event |
| Deployment | Dockerfile (distroless, multi-arch), `compose.yaml`, the Helm chart adjusted, the migration job |

**The result:** a runnable, deployable, tested system with exactly one use case — every cross-cutting
concern driven through once, completely.

Security and reliability belong deliberately in **phase 0**, not in stabilisation: the gates must
bite while there is one use case and rework is cheap. A cross-tenant test introduced later finds
defects in 200 methods at once — one present from the start prevents them one at a time.

---

## Phase 1 — The business core (`0.2.0` – `0.4.5`)

### `0.2.0` Hierarchy and items
The identity base model (tenant, account, membership, roles, permission resolution), containers
(hub/collection), the generalised `WorkItem` with capability profiles, the hierarchy service,
buckets, labels, ordering (drag and drop), query DSL v1, trash/archive, activity history, and
`hubctl` as the first real client.

### `0.3.0` Collaboration and content
Comments, members and assignment, automatic assignment (every strategy), covers (colour/image
upload), media and attachments with presigned upload, custom fields, full-text search, notifications
(email), the SSE stream, and bulk plus duplicate — the two item operations the use case catalogue
has named since the beginning and no milestone owned. `POST /items:bulk` had been in the
specification since A-06 and answered `route.operation_not_available`; `:duplicate` was in no
document but the catalogue, and C-11 is where it entered the contract.

### `0.3.5` The workspace
The repository becomes a monorepo: `apps/webapp` (the to-do application), `apps/website`
(hubtask.eu), `packages/design-system` and `packages/api-client`, plus the design tokens every
client draws from and a pipeline that runs only where a change actually landed. No use case is
added — the milestone is measured by what still works afterwards. It sits here because phase 5
starts in parallel from `0.4.0`, and the house has to exist before anybody moves in
([ADR-0027](./adr/ADR-0027-monorepo-structure.md),
[ADR-0028](./adr/ADR-0028-embedded-web-ui.md),
[ADR-0029](./adr/ADR-0029-design-system-tokens.md)). With
[ADR-0030](./adr/ADR-0030-svelte-frontend-framework.md)–[ADR-0033](./adr/ADR-0033-shared-client-architecture.md)
accepted, the milestone carries a second wave, W-06–W-09: the Svelte webapp scaffold, the token
wiring, the embedded pipeline proven against the real bundle, and the workspace dependency lint.

### `0.4.0` Time
Due dates (including all-day, time zones), reminders (predefined + custom), recurrence (RRULE, both
modes, DST tests), the scheduler role, the job queue, the retention job, the ICS calendar feed,
templates, and saved views including layout hints for list/kanban/timeline. The scheduler role, the
job queue, and the retention job were built ahead of schedule (A-08, B-10, C-09); what this
milestone adds to them is the first duty that turns a stored future timestamp into work.

**The result of phase 1:** the core is functionally complete per the product idea — without a
frontend.

---

### `0.4.5` Backup, retention, audit
Backup targets (`local`, `s3`, `sftp`, `webdav` first), the archive format with encryption, RRULE
schedules, generational retention, listing at the target, restore in every mode including the
deletion journal; retention rules with a preview, a grace period, and safeguards; audit query,
export, and `:verify`; legal hold; data subject requests (`data_subject_request`) with deadline
tracking and export. Further target adapters (`ftps`, `ftp`, `smb`, `azure`, `gcs`, `rclone`) follow
once they pass conformance test BK-1. Two things this list does not name are prerequisites rather
than additions: `/jobs/{id}`, which `0.4.0` deferred to "the first operation that genuinely cannot
be bounded" and which three backup responses have been promising since A-06, and the data
protection gates PG-1…PG-8, which four documents assert and which exist in no form.

---

## Phase 2 — Automation and integration (`0.5.0` – `0.6.0`)

### `0.5.0` Automation and webhooks
The outbox dispatcher production-ready, CloudEvents schemas published, the rule engine (triggers,
CEL conditions, every action), dry run, RuleRuns, loop and throttle protection, webhook
subscriptions with HMAC/retry/dead letter, the trigger polling endpoints, PATs and service accounts
with scopes, and the jumble (email intake, webhook intake, quick capture, conversion). The backlog
(`docs/backlog/milestone-0.5.0.md`) scopes the email intake webhook-first — the IMAP adapter waits
on a dependency decision recorded as open point AM-1 — and lands `SCHEDULE` triggers as RRULE
through the one schedule engine, with cron notation deferred as sugar.

The milestone also closes what earlier ones parked with a date on it: the retention advance warning
(R-1), the referential safeguard's direction (R-2), the `AUDITOR`'s configuration reads (A-4, a
read-only permission split out of `STRUCTURE`), and where the chaos tests run (O-4, decided by
measuring what they cost). And `hubctl` grows with it — `rule`, `webhook`, `jumble` and
`events poll` — so that the milestone's verbs are typed against a real stack in the scripted
session rather than described.

### `0.6.0` Multi-tenancy and operations
Multi mode, tenant provisioning/suspension/deletion, export (GDPR), quotas and fairness, the OIDC
connection, an OAuth2 provider for third-party apps, TOTP MFA with enforcement per tenant, session
management, step-up authentication, envelope encryption with key rotation (open point S-2), role
separation in the Kubernetes deployment, HPA, PodDisruptionBudget, NetworkPolicy, load tests
(2 million items, an automation storm), partitioning, PITR backup with a verified restore (RT-9),
the complete alert catalogue A-01…A-18 with runbooks, SLO dashboards, and the optional NATS adapter.

The backlog (`docs/backlog/milestone-0.6.0.md`) scopes it: the Kubernetes half — role separation,
HPA, PodDisruptionBudget, NetworkPolicy — was built ahead with the chart and is proved under load
rather than added; sign-in, sessions, MFA, OIDC, OAuth2 and the admin surface start from an empty
schema *and* an empty contract, so every task is specification first and migration first; the
load figures are measured and recorded internally only, publication being a separate decision;
and the dependency candidates (the OIDC verification library, the NATS client) each land through
their own ADR rather than in passing — a third, an IMAP client, was declined by
[ADR-0040](./adr/ADR-0040-no-imap-intake.md) rather than chosen.

---

## Phase 3 — AI and ecosystem (`0.7.0` – `0.9.0`)

| Version | Contents |
|---|---|
| `0.7.0` | The MCP server complete (tools, resources, prompts), the AI port with Ollama and OpenAI-compatible adapters, AI suggestions for the jumble and for decomposition, semantic search (pgvector), provenance and audit |
| `0.8.0` | i18n complete: catalogue maintenance, the Weblate connection, CLDR formats, RTL metadata, language-dependent search, localised emails; accessibility and localisation requirements for the frontend documented |
| `0.8.5` | Offline synchronisation complete: `:pull`/`:push`, per-field merging, OR-sets, fractional indices, HLC bounding, device management, conflict preservation, the SSE stream, `hubctl sync-conformance` as the reference client check |
| `0.9.0` | Ecosystem: an official n8n node and Zapier app (generated), client SDKs (TypeScript, Go, Python), CalDAV, import from Trello/Microsoft To Do/Google Tasks, public API documentation |

---

## Requirements that arrive late

New requirements will arrive while this plan runs, and some of them will change the core and the
client at the same time. That is ordinary work rather than an exception, and it is handled like
this:

1. **The contract moves first.** `api/openapi.yaml`, then `make generate`, then `make api-client`,
   then the implementation ([ADR-0004](./adr/ADR-0004-api-first-openapi.md)); a change to the data
   model brings its migration in the same order, expand before contract.
2. **The pull request that changes the contract carries the client fix.** The generated client
   makes a break visible at typecheck time inside that very change. Fixing it there is what keeps
   `main` green, and what makes the real cost of a rename visible while reconsidering it is still
   cheap ([ADR-0035](./adr/ADR-0035-one-product-version.md) §4).
3. **Both sides get an issue, and the two are linked.** Where the change is additive they are
   separate pull requests and the client one lands in its own window. Where it removes or renames
   something a client already ships, they land together.
4. **The window closes when `0.9.5` opens.** Until that day a new requirement is scheduled into a
   milestone like any other. After it there are only defects: a new requirement waits for `1.1.0`,
   or it is an exception with its own ADR that says what it costs and why the freeze does not apply
   to it.

Rule 4 is what makes the freeze real. A stabilisation phase that still accepts features is not a
stabilisation phase, and every date after it is a guess.

---

## Phase 4 — Convergence and stabilisation (`0.9.5` – `1.0.0`)

A major is finished in three movements: parallel development, a **convergence milestone that
freezes the scope**, then stabilisation ([ADR-0035](./adr/ADR-0035-one-product-version.md) §5).
`1.0.0` is the first time the project runs them; `2.0` and every major after it follow the same
shape, which is why the rule lives in
[versioning-release.md](./architecture/versioning-release.md) rather than only here.

### `0.9.5` Convergence — where the two tracks arrive

Not a feature milestone. It holds exactly the work that can be done neither earlier nor later:

* **The coverage report.** Every use case of the catalogue, where it is reachable in each client,
  and every deliberate omission with its reason — written to `docs/evidence/` and reviewed, in the
  manner of the resilience evidence already there. The capability matrix
  ([ADR-0032](./adr/ADR-0032-client-capability-matrix.md)) is either met or amended by supersede;
  it is not quietly missed.
* **The maturity stage goes to `stable`.** The preview banner comes off, and from that moment a
  client regression blocks a release exactly as an API regression does (ADR-0035 §2).
* **The scope window closes**, per rule 4 above, on the day this milestone opens.
* **Everything with external lead time starts here**: the shells cut release candidates, installers
  are signed, store listings are submitted. Store review is a queue somebody else owns, which is
  precisely why it cannot be a week inside `1.0.0`.
* **The website switches to its 1.0 content**, and the arc42 client-architecture chapter is
  current rather than promised.
* **The two backlogs become one.** From here there is no core track and no client track, only a
  product being stabilised.

### `1.0.0` Stabilisation

Prerequisites for `1.0.0`:

1. API v1 frozen, the OpenAPI diff clean, the deprecation process exercised.
2. Event schemas v1 stable and documented.
3. Every quality scenario QS-01 to QS-27 demonstrated (test reports in the repository).
4. Load test results against the target figures published.
5. A security review including an external pentest or code audit of the tenant boundary and the webhook/SSRF paths (open point S-1); the threat model T-01…T-20 complete with test evidence; every gate SG-1…SG-12 permanently green.
6. The upgrade path from `0.x` documented and tested.
7. Trademark registration for the name and logo completed (the licence itself is settled, [ADR-0013](./adr/ADR-0013-licensing.md)).
8. Operating documentation complete: backup, restore (with a logged drill), monitoring, the alert catalogue with a runbook per alert, an SLO report over at least 30 days, resilience tests RT-1…RT-12 green, `hubtask_panics_recovered_total` at 0 over the period.
9. Reference deployments (Compose and Helm) tested reproducibly.
10. The data catalogue `docs/privacy/data-catalog.md` and the DPA template in place (open point S-3).
11. The audit demonstrably complete and immutable: AT-1…AT-7 green, `:verify` over a production period with no findings.
12. Backup and restore verified: BK-1…BK-10 green, a documented restore drill from every released target type, and the golden archives of every major version importable.
13. Retention rules exercised: RE-1…RE-9 green, and at least one complete run of a multi-stage chain in a production environment.
14. Offline synchronisation accepted: SY-1…SY-12 green, and the conformance test passed against at least one real client.
15. Client parity demonstrated: end-user features and profile configuration on web, desktop and mobile, administration on web and desktop; the mobile administration exclusion is the only restriction, and it behaves as [ADR-0032](./adr/ADR-0032-client-capability-matrix.md) describes — the capability is named and linked to the web app, never silently absent.
16. Accessibility demonstrated: WCAG 2.2 AA for the web app and the shells that render it, with the accessibility statement published (European Accessibility Act, [data-protection.md](./architecture/data-protection.md) §7).
17. Offline conformance from a first-party client: `hubctl sync-conformance` passed by `packages/sync-engine` against a real instance — criterion 14's "at least one real client" is named rather than hoped for.
18. The webview matrix green: the smoke suite passes on WebView2, WKWebView and WebKitGTK. One codebase across three engines is where a rendering defect hides.
19. The clients distributed: signed installers for Windows, macOS and Linux, live store listings for iOS and Android, and one updater run exercised end to end from a released version to its successor.
20. The website deployed from the release commit, carrying the 1.0 content, the licence notice and the download links.
21. The design system holds: no literal colour, spacing, radius or duration value anywhere (the lint proves it), contrast measured in CI rather than asserted, and waves 1 to 3 complete or the gap named with its reason.

---

## Phase 5 — The client track (in parallel from `0.4.0`)

The stack is decided: Svelte 5 with the webapp as a plain Vite SPA
([ADR-0030](./adr/ADR-0030-svelte-frontend-framework.md)), Tauri 2 shells for desktop and mobile
with the PWA path closed ([ADR-0031](./adr/ADR-0031-tauri-app-shell.md)), parity by default with
tenant administration reached via the web on mobile
([ADR-0032](./adr/ADR-0032-client-capability-matrix.md)), and one product UI plus a
framework-agnostic sync engine ([ADR-0033](./adr/ADR-0033-shared-client-architecture.md)). The
scaffolds live in milestone `0.3.5` (W-06–W-09). What was missing was the schedule.

Four rules govern the track. They are what make a client built alongside a moving core affordable
rather than a source of permanent rework.

**One version, and no second one.** [ADR-0035](./adr/ADR-0035-one-product-version.md): the client
is not versioned separately — the web app is embedded in the binary and released with it, the
shells carry the product version plus a platform build counter, the website is unversioned. What a
second number would have expressed is expressed by a maturity stage instead: `experimental`, then
`preview`, then `stable` at convergence.

**The client track runs one milestone window behind the core.** A client milestone builds the
surface for a core milestone that has already shipped. The client therefore works against a
contract that has just settled rather than one still moving, and the cost of the parallelism stays
bounded to the window it occurs in.

**Incomplete is normal; broken is a defect.** A use case without a screen is the expected state for
most of the `0.x` phase, and the maturity stage says so. A red client lane is not the frontend
catching up: build, lint, typecheck and test are green at every commit, on the same terms as the Go
gates. And because `packages/api-client` is generated from the specification, a contract change
turns the client red in the pull request that makes it — which is why that pull request carries the
client fix (ADR-0035 §4).

**`F1`…`F6` are milestones, not versions.** They are planning buckets holding the client issues;
nothing is released by them. Tasks are numbered `F1-01`, `F2-01` and so on, and each milestone is
cut into issues when it opens, from the then-current state of this file and the accepted ADRs.

| Milestone | Opens with | Builds the surface for | Contents |
|---|---|---|---|
| **F1 — Foundations** | `0.4.0` | up to `0.3.0` | The component workbench decision and design-system wave 1; the three §9 gaps that block it (iconography, contrast verification in CI, voice and tone) and the wordmark the website needs; the application frame — layout, navigation, `data-theme`, the message-code renderer, problem-details rendering, `HealthBanner` from `/meta/health`, the capability manifest, the maturity banner; sign-in and session; and the data seam: `packages/sync-engine` with its three ports, an online-only pass-through and the Svelte binding, so that no component ever talks to `@hubtask/api-client` directly. And the pre-release website, whose own requirement follows this table |
| **F2 — The working surface** | `0.4.5` | `0.2.0` | Wave 2; hubs, collections and the five levels, buckets, labels, ordering and drag and drop, trash and archive, the activity history; the query language made visible — `SearchField`, `QueryBuilder`, `ViewSwitcher` for list and kanban, `TaskRow`, `WorkItemCard`, `BucketColumn`, `LabelChip` and `LabelPicker`, `CapabilityGate`. This is where the tool becomes usable for its own development: daily work moves out of `hubctl` and into the app, which is what risk R-08 was waiting for |
| **F3 — Collaboration, content, time** | `0.5.0` | `0.3.0` and `0.4.0` | Comments, members and assignment, covers, attachments with presigned upload, custom fields, notifications, the SSE stream, bulk and duplicate — `CommentThread`, `AssigneeControl`, `CustomFieldRenderer`, `ActivityFeed`; and the time surfaces `DueDateControl`, `ReminderEditor`, `RecurrenceEditor`, templates, saved views with their `layout` hint, the timeline, and calendar feed management |
| **F4 — Automation, administration, tenant** | `0.6.0` | `0.4.5` and `0.5.0` | The jumble inbox, `AutomationRuleCard` and `RunStatusBadge`, dry run, webhook subscriptions, personal access tokens and service accounts; the administration area — tenant settings, `RoleBadge` and `PermissionMatrix`, quotas, the OIDC connection, MFA, sessions and step-up, backup and restore, retention with its preview, audit query, export and `:verify`, data subject requests. Administration is the one area the mobile client does not carry, so its routes are tagged by area here (ADR-0032), long before there is a mobile build to exclude them from |
| **F5 — AI, i18n, accessibility** | `0.7.0` | `0.7.0` and `0.8.0` | `AISuggestion` — visually separable, and gone without residue when AI is switched off — semantic search, and the AI paths of the jumble and of decomposition; language switching, CLDR formats and the RTL audit against design-system rule 3; and accessibility: WCAG 2.2 AA, keyboard operability, `focus-visible`, a screen-reader pass, and the accessibility statement the European Accessibility Act expects |
| **F6 — Offline, the shells, the moments** | `0.8.5` | `0.8.5` | The sync engine becomes what its name says: local store, mutation queue, HLC, cursor and tombstone handling, `ACCESS_REVOKED` and `sync.cursor_too_old` behaviour, `SyncStatus` and `ConflictResolver`, the `offline-sync.md` §9 harness against fakes, and `hubctl sync-conformance` passed against a first-party client. Then the Tauri desktop shell — SQLite and keystore behind the `Storage` port, updater, signed distribution, the webview smoke matrix — and after it the mobile shell with signing, the store pipeline, platform adaptation (§9's last gap) and the capability matrix made real — the admin routes F4 tagged are excluded from this build, and each appears as the affordance ADR-0032 asks for: named, and linked to the web app of the server the client is signed into. The celebration kit and the onboarding tour close the milestone: the tour's last step is the first celebration, so they arrive together |

Each open point in [`design-system.md`](./design/design-system.md) §9 therefore has an owner: the
wordmark in F1, platform adaptation with the mobile shell in F6. The list is not fixed — F1-02,
F1-03, F1-04 and F1-05 closed contrast verification, iconography, voice and tone and the border
scale, and F1-13 closed the layering scale and opened two more, which is what a gap list does while
a milestone runs.

**F1 is open**, and its backlog is [`backlog/milestone-F1.md`](./backlog/milestone-F1.md) —
thirteen tasks, F1-01…F1-13. Cutting it found one thing this table could not: the client has no way
to learn which account it is signed in as, because nothing reads an `Account` and
`/accounts/{accountId}/preferences` needs an id it never receives. Since "locale and time zone
through the account preference" is a binding requirement above, F1 carries **one core task**
(`GET /accounts/me`, additive and specification first) — the first worked example of the rule for
a requirement that touches both sides, and a reminder that a client milestone may find a gap in
the contract rather than only build on it.

### The website: a pre-release site from the `0.4.0` window

> **`hubtask.eu` carries a pre-release site from the `0.4.0` window onwards.** It shows what the
> project intends to be and advertises it — before there is a product to sign into. It is the
> public face of the whole `0.x` phase, not a placeholder that goes up shortly before the launch.

The website is the one surface with an audience before the product has users, which is why it comes
first in the track rather than last. It has two stops: the **pre-release site** in F1, and the
**1.0 site** at convergence (documentation, the licence notice, downloads for the shells, the
accessibility statement). Because it is unversioned and continuously deployed
([ADR-0035](./adr/ADR-0035-one-product-version.md)), its content can move as often as the message
does without touching a release.

**Decided, and needing no brief:** SvelteKit with `adapter-static`, fully prerendered
([ADR-0030](./adr/ADR-0030-svelte-frontend-framework.md)); every value from the design system
([ADR-0029](./adr/ADR-0029-design-system-tokens.md)), with wave 4 as its component budget; never
embedded in the binary and never a second product surface — information only, no sign-in, no task
management ([ADR-0027](./adr/ADR-0027-monorepo-structure.md),
[ADR-0028](./adr/ADR-0028-embedded-web-ui.md)); and the client requirement above about cookies
applies to it before it applies to anything else.

**Open, and awaited from the owner** — named here so it is visible what waits on what:

* positioning and messaging: what the site claims the product is, and for whom;
* the page structure and how much of the roadmap is shown in public;
* visual direction beyond the tokens, and the wordmark F1 produces;
* what may be promised about dates, editions and price
  ([licensing-editions.md](./architecture/licensing-editions.md) describes the model; what is
  advertised from it is a separate decision);
* whether there is a waiting list, a newsletter or an early-access signup — each collects personal
  data and therefore needs a data-catalogue entry with a legal basis and a deletion path
  ([data-protection.md](./architecture/data-protection.md)), and consent for anything
  non-essential;
* the launch moment.

**The split that lets work start before the brief exists.** The scaffold, the deployment lane, the
design-system wiring and a minimal holding page are buildable now and are one work package. The
content wave is a **second** work package that is explicitly allowed to sit unstarted without
blocking F1; it begins when the brief does, in whichever milestone that turns out to be. What must
not happen is content invented in the absence of a brief and then defended because it is already
live.

Where the built files are actually served from is undecided: there is a `website` job that lints,
type-checks and builds, and none that deploys. It is open point **CI-4** in
[ci-cd.md](./architecture/ci-cd.md) §8.

A dedicated arc42 client-architecture document is written with the sync-engine package, once
there is a first implementation to describe. What was settled in preparation and stays binding:

* The contract: an OpenAPI-generated SDK, `/meta/capabilities` for configuration, saved views with a
  `layout` hint, SSE for live updates.
* Binding requirements on every frontend: locale and time zone handling through the account
  preference, RTL support, message codes instead of server text, tolerant behaviour towards unknown
  fields, and offline-tolerant writing with an `Idempotency-Key`.
* **Accessibility: WCAG 2.2 AA**, with an accessibility statement published for the released
  clients. [`data-protection.md`](./architecture/data-protection.md) §7 names this list as where
  the European Accessibility Act lands, and it is demonstrated for `1.0.0` rather than asserted.
* **No non-essential cookies without consent** — the second client requirement `data-protection.md`
  §7 places here. The backend uses bearer tokens rather than tracking cookies; nothing a client adds
  may quietly reintroduce them, the website included.
* **Offline conformance** per [offline-sync.md](./architecture/offline-sync.md) §9: client-assigned
  UUIDv7, an `op_id` per mutation, an HLC per field change, local deletion on `ACCESS_REVOKED` and
  `sync.gone`, a full resynchronisation on `sync.cursor_too_old`, and encrypted local storage with
  complete deletion on sign-out. Verifiable through `hubctl sync-conformance` — which applies to
  third-party implementations too.
* The offline promise is carried by the installed clients; the browser app holds a best-effort
  cache only, and no client merges — merging belongs on the server
  ([ADR-0031](./adr/ADR-0031-tauri-app-shell.md), [ADR-0033](./adr/ADR-0033-shared-client-architecture.md)).
* As an interim solution, `hubctl` (the CLI) plus a minimal reference client serve for dogfooding —
  not a product decision, just a tool (risk R-08). F2 is where that interim ends.

---

## What is immediately implementable

| Ready | Reason |
|---|---|
| The complete core (phases 0 and 1) | The domain model, invariants, use case catalogue, API contract, and persistence sketch are settled |
| The automation service (phase 2) | The rule model, trigger/action catalogue, execution semantics, and protective mechanisms are decided |
| The integration layer | The event catalogue, webhook mechanics, and auth methods are in place |
| Deployment and CI | The role model, image strategy, and pipeline gates are in place |
| The security baseline | The threat model T-01…T-20 with countermeasures and gates SG-1…SG-12 is decided ([security.md](./architecture/security.md)) |
| Audit, data protection, retention | The log model, data subject rights, deletion paths, and the retention model are decided and represented in the schema |
| Backup and restore | The target abstraction, archive format, schedules, retention, and restore modes are decided ([backup-restore.md](./architecture/backup-restore.md)) |
| Offline synchronisation | The protocol, conflict handling, and data model prerequisites are decided; the client requirements are settled ([offline-sync.md](./architecture/offline-sync.md)) |
| Observability and resilience | The health model, metric and alert catalogue, resilience patterns, and the test series RT-1…RT-12 are decided ([observability-reliability.md](./architecture/observability-reliability.md)) |
| **Not ready** | The concrete billing model, SAML/SCIM details, master key management in provider operation (S-2), and the capacity model from real load data (O-2). The frontend stack is decided (ADR-0030…ADR-0033); its visual design beyond the design system emerges with the component layer |
