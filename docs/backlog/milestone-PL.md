# Milestone PL — Plans

The goal: **a provider sells Hubtask in plans — "Private", "Family", "Business" — through its own
purchase or billing platform, and Hubtask is prepared for it**: the platform, with a credential of
its own, provisions a workspace on a plan, moves it, suspends and ends it, and learns what happened
and what was used — while Hubtask applies each plan's limits, locked values and features at once,
never destroys anything, and never knows a price. An installation that defines no plan, and every
self-hosted one, meets none of it. Milestone SI prepared the level without building it (ADR-0070
§3): the resolver's plan parameter, `tenant.plan_id`, and a lock that knows its origin. Read against
the code, less of it holds than the ADR says (§ "The cut checklist", 4).

The rules are drafted in [multi-tenancy.md](../architecture/multi-tenancy.md) §4.2 and §4.3,
[identity.md](../architecture/identity.md) §18 and [api-guidelines.md](../architecture/api-guidelines.md)
(the scope), the reasoning in
[ADR-0086](../adr/ADR-0086-plans-a-level-for-providers.md). Every task names its use cases; a task
is done when the named checks hold, with the evidence in the pull request.

**Delivers:** UC-INS-13 (1–11), UC-INS-14 (1–8), UC-INS-10 (1, 5), UC-INS-06 (6), UC-INS-08 (7), UC-ID-12 (4), UC-ID-16 (7), UC-ID-20 (8), UC-AI-06 (2), UC-AI-07 (1), UC-PRV-05 (11)
**Released:** 2026-10-16

## Decisions

Each measured against the deployments `D1`–`D7`:

1. **Plans prepare Hubtask for a provider's purchase or billing platform, and restrict nothing by
   themselves** (the owner, 2026-10-15, and #1550 on 2026-10-16). What the project already holds —
   P-15, `NG-billing` ("a platform pushes state in … and reads usage out"), `PE-platform`, D5 —
   becomes UC-INS-13's *Goal*. Hubtask ships no plan; a workspace without one has every feature
   and the installation's limits, as today; a self-hosted installation that defines none sees
   nothing new, and single mode offers plans nowhere (D1–D4).
2. **A plan leaves out exactly three features** — automation rules, webhooks, the workspace's own
   sign-in providers — and reaches everything else through values, locks and limits (the owner,
   2026-10-15, #1530): multi-tenancy.md §4.2, UC-INS-13 checks 3 and 4.
3. **A plan carries and locks its own legal links**, between the installation and the workspace
   (the owner, 2026-10-15, #1531): identity.md §18.
4. **UC-INS-13 reaches plans through every door** — checks 7 and 8, `D7`, check 6 read as moving
   a workspace (trail and journal) and changing a plan (journal) (the owner, 2026-10-15, #1532).
5. **A limit on people** — active and managed accounts and open invitations, never service accounts
   (the owner, 2026-10-15, #1533): multi-tenancy.md §4 and §4.2, UC-INS-10's *Goal* and check 5.
6. **A plan is sparse and referenced, never copied** (worker, at the cut): what it does not set
   falls through to the installation, so check 2 needs no job, and a workspace without a plan is
   the installation's (Decision 1).
7. **An installation lock binds the plan; a plan only tightens what is open, a limit goes higher
   only where the installation lets workspaces go higher; the operator's exception for one
   workspace stays the last word on a limit** (worker, at the cut): P-07 and ADR-0070's seventh
   settlement, applied to the new level. The sign-in resolver today does the opposite for a plan
   and is corrected in PL-02.
8. **The lock's origin reaches every reader** (worker, at the cut): `instance_setting` already
   stores `PLAN`, and the reads collapse it into a boolean; the workspace-facing fields that carry
   `lock`/`source` gain the value `PLAN` (additive), and the admin and quota documents gain an
   origin beside `locked` (additive, no field renamed or removed).
9. *Replaced by Decision 11*: plan changes are webhook events in PL after all.
10. **The platform's path is checked** (the owner, 2026-10-16, #1550): UC-INS-13 checks 9 and 10 —
    a short name that never changes and is never reused, a move bound to the workspace's version,
    the platform's own reference, reading one workspace and filtering the list.
11. **PL delivers UC-INS-14** (the owner, 2026-10-16, #1551), its checks 2 and 4 sharpened and 6, 7
    and 8 added: versioned, state-carrying events, pushed and readable forward, never switched off
    silently; use per day with its day basis.
12. **A platform requests a deletion without a step-up** (the owner, 2026-10-16, #1552): UC-INS-08
    check 7, multi-tenancy.md §5.
13. **`admin:provisioning`** (the owner, 2026-10-16, #1553): UC-INS-06 check 6, api-guidelines.md.
14. **An account link** (the owner, 2026-10-16, #1554): UC-INS-13 check 11; UC-INS-10's and
    UC-MED-04's *Where it ends* say it is not selling.
15. **Use for billing is opt-in and kept 13 months, also after the purge** (the owner, 2026-10-16,
    #1555): UC-INS-14 check 8, data-protection.md, the data catalogue.
16. **Every screen is shown to the owner before it is built** (the owner, 2026-10-16): a task that
    changes a screen starts — after its readiness record — with a preview the worker presents to the
    owner, and the screen's code follows the owner's word. Each such task says so in its
    *Acceptance*.
17. **The operator's exception stays the last word after a move** (worker, at the cut, from the
    research of 2026-10-16): a platform that sells an add-on re-sets it when it moves a workspace;
    the move's answer names the exceptions that stay, the limits now exceeded and the features that
    rest, so the platform never has to guess.
18. **Recording and offering use are two things** (worker, at the cut): the AI budget's ledger is
    written always, because a limit needs it; data-protection.md's "metering off" means the
    offering for billing, which is how its sentence is corrected.

---

## PL-01 — The plan, its control plane and its record

*Depends on: nothing.* · issue #1536 · multi-tenancy.md §4.2 · ADR-0086

**Use cases:** UC-INS-13 (1, 6, 7, 8, 9)

The contract first: `/admin/plans` (list, read, create, change with the version read, remove),
moving a workspace (`POST /admin/tenants/{id}:move-plan`, `null` to leave plans), a `plan` in
`TenantProvision` and `AdminTenant`. A migration creates `plan` without row-level policy — the
exception entered in all three boundary lists in the same commit — and makes `tenant.plan_id` a
reference that refuses removing a plan in use, and gives the workspace a version that grows with
every change the control plane makes. The plan in the domain: a short name that never changes and is
never reused, a display name, a version, sparse values per area with locks, the entitlements; the
values are refused where an installation lock lies or where they loosen what is open. Every write in
`instance_event`; moving a workspace in its trail as well. A move may name the workspace's version
and is refused with `version_conflict` when stale; repeated under its idempotency key it moves once
and writes once; its answer names the entitlements that rest and the exceptions that stay (Decision
17); the limits now exceeded join it in PL-03. Multi mode only: single mode answers
`admin.multi_mode_required`. `hubctl admin plan` and the instance file in both modes, through the
same use case.

**Acceptance:** domain table tests for the refusals (under an installation lock, looser than open);
a Testcontainers test for the repository, and the boundary test passing with `plan` named and
reasoned; application tests that a stale version is refused with `version_conflict`, a plan in use
is not removed, a short name is neither changed nor reused, provisioning on a plan is idempotent
under its key, a move writes journal and trail once, a stale move is refused and a late retry of an
older move cannot win; a `hubctl` test and an instance-file test with the same refusals; the restore
drill's list updated.

---

## PL-02 — The sign-in rule and the legal links resolve through the plan

*Depends on: PL-01.* · issue #1537 · identity.md §18 · multi-tenancy.md §4.2

**Use cases:** UC-INS-13 (2, 3, 4), UC-ID-12 (4), UC-ID-16 (7), UC-ID-20 (8)

`Effective` reads the workspace's plan and applies it under the installation's locks and the tighten
rule, the defect in today's plan branch corrected first with a failing test. `EffectiveLegal` gains
the plan level; moving to a plan with other terms, or changing a plan's terms, asks everybody once
more at their next sign-in, read, no job. Every caller passes the plan; the origin `PLAN` reaches
`/auth/sign-in-rules`, the workspace's sign-in screen, the private-hubs and managed-accounts
switches. A value the plan newly locks stays stored. A plan that locks the methods follows §18 and
§17.1 like the installation's lock.

**Acceptance:** table tests of `Effective` and `EffectiveLegal` across the four levels (open, locked
at installation, locked at plan, a stale workspace value), the plan branch's failing test first; an
application test that moving a workspace changes its rule at the next request with no job; a terms
test asking again after a move; an e2e walk showing "Set by your plan" on the sign-in screen and the
two switches; the preview shown to the owner first (Decision 16).

---

## PL-03 — Limits and the AI rule resolve through the plan

*Depends on: PL-01.* · issue #1538 · multi-tenancy.md §4, §4.2 · ADR-0072

**Use cases:** UC-INS-10 (1), UC-AI-06 (2), UC-AI-07 (1), UC-PRV-05 (11), UC-INS-13 (2, 3, 4)

`quota.Resolve` gains the plan between the installation and the operator's exception, which stays
the last word; `/quotas` says which level set each limit, `PLAN` among them, and the limits screen
names the plan; the move's answer gains the limits the workspace is now over. `EffectiveAi` receives
the workspace's plan instead of the empty layer, for `sources`, `min_jurisdiction` and
`person_opt_out`. A lowered limit refuses the next creation past it and removes nothing; a narrowed
AI rule rests the workspace's own model as UC-AI-06 check 5 already does.

**Acceptance:** table tests of `Resolve` across product, installation, plan and exception; an
integration test that a plan's change reaches the guard at the next request; tests for the three AI
keys with a plan lock; an e2e walk of *Administration → Limits* and the AI screen naming the plan;
the preview shown to the owner first (Decision 16).

---

## PL-04 — A limit on people

*Depends on: PL-03.* · issue #1539 · multi-tenancy.md §4, §4.2

**Use cases:** UC-INS-10 (5), UC-INS-13 (4)

The quota `accounts`, unlimited by default in both modes, at every level. Counted in one statement:
active and managed accounts and open invitations, not service accounts, not disabled or anonymised
ones. Inviting, creating a managed account and a provider's first sign-in that would create an
account check it in the transaction that writes, and refuse with `capacity.accounts`. Nothing
existing is touched below the count.

**Acceptance:** application tests for each of the three doors at the limit, and that service
accounts are not counted; a concurrency test that two invitations at the last place admit one (known
traps: a read-then-write without a lock); a test that a lower limit disables nobody; the limits
screen showing people; the preview shown to the owner first (Decision 16).

---

## PL-05 — What a plan leaves out

*Depends on: PL-01.* · issue #1540 · multi-tenancy.md §4.2

**Use cases:** UC-INS-13 (3, 4, 5)

The three entitlements as a closed type in the domain, read with the plan. Automation rules left out
run nowhere — event, schedule, the worker — and cannot be created or switched on; included again
they run from then on, a schedule without the missed occurrences. Webhooks left out deliver nothing
and take no subscription; included again they deliver events from then on. The workspace's own
providers left out sign nobody in, their connections and secrets kept, the fallback of §17.1 read
where they were the last way in. `/meta/capabilities` answers the entitlements caller-scoped, and
the screens show "Not included in your plan" in place of the control (P-05). Check 5 by
construction: the type has no entry for export, deletion, access requests, language, time zone,
accessibility or a door.

**Acceptance:** application tests per entitlement for left out and included again, including that
nothing is deleted; a worker test that no rule runs and no delivery leaves for a workspace without
the entitlement; a sign-in test that the password opens where the own provider was the last way in;
a test that the type cannot name the never-switchable features; e2e walks of the three screens; the
preview shown to the owner first (Decision 16).

---

## PL-06 — Plans on the installation's screens

*Depends on: PL-01; PL-05 for the entitlements.* · issue #1541 · ADR-0070 §5

**Use cases:** UC-INS-13 (7, 8)

*Installation → Plans*: the list with the number of workspaces on each, a plan's values drawn from
the same declarations as *Defaults* with the plan's lock column, the entitlements as switches, a
save that says somebody else saved first. *Installation → Workspaces*: the plan on each row, moving
a workspace with a confirmation that names what changes, and the plan in *New workspace*. Removing a
plan in use is not offered. In single mode none of it appears; in `enforce` mode the values show
with the file's path and no control.

**Acceptance:** e2e walks creating a plan, moving a workspace and provisioning on a plan; a walk in
single mode showing nothing; a keyboard and screen-reader walk; the preview shown to the owner first
(Decision 16)

---

## PL-07 — The installation's events, pushed and readable

*Depends on: PL-01, PL-09.* · issue #1561 · multi-tenancy.md §4.3 · UC-INT-01

**Use cases:** UC-INS-14 (1, 2, 3, 5, 6, 7)

The contract: installation subscriptions (`/admin/subscriptions`) and `GET /admin/events` with a
cursor and `410` past its window. The events of UC-INS-14's goal written in the transaction of the
change — created, moved to another plan, suspended, resumed, deletion requested and done, an owner
changed, a limit reached (once per crossing), the AI budget spent, an export finished — each with
the workspace's id, reference, status, plan, version and moment, never content; ordered by a
commit-ordered sequence, as the outbox is. Delivery reuses the workspace webhooks' signing, retries
and dead letters, without switching a subscription off: the operator is told (an alert) and dead
letters stay re-deliverable from the screen. `hubctl admin subscription` and the instance file.
Single mode: `admin.multi_mode_required`, and no screen. The feed and the subscriptions admit
`admin:provisioning` beside `admin:tenants` (PL-10, UC-INS-06 check 6), and the security gate's list
of the platform's verbs names them.

**Acceptance:** an application test per event that it is written once with the right state and
version; a test that "limit reached" is sent once per crossing; an integration test that the feed
returns every event of two concurrent transactions in order and none twice; a delivery test that
three dead letters leave the subscription on and the operator alerted; a test that no event carries
a title, a name or a note (rule 10); e2e walks of the subscription screens; the preview shown to the
owner first (Decision 16).

---

## PL-08 — Use the platform can bill by

*Depends on: PL-01, PL-04.* · issue #1562 · multi-tenancy.md §4.3 · data-protection.md

**Use cases:** UC-INS-14 (4, 7, 8)

People, storage and entries recorded per day as the day's highest count, written in the transaction
that changes the count (no job visits workspaces); the existing sums kept. `GET /admin/usage`
(range, metric, workspace) with a stable metric name, its day basis and `final_through`, offered
only while the operator switches metering on, off by default, and answering `admin.metering_off`
otherwise. At the purge the counts move to an installation-level ledger keyed by the bare id; a
sweep removes counts older than 13 months, inside and outside workspaces. `hubctl admin usage`; the
switch in the instance file. The data catalogue and data-protection.md say what is built. The read
admits `admin:provisioning` beside `admin:tenants` (PL-10, UC-INS-06 check 6), and the security
gate's list of the platform's verbs names it.

**Acceptance:** application tests of the highest-value counts under several changes a day and none
at all (carried forward by the reader); a read test across a day boundary in UTC and in a
workspace's zone; a test that the read is refused with metering off; a purge test that the counts
outlive the workspace without its name or reference, and a sweep test at 13 months; the boundary
test with the installation ledger named and reasoned.

---

## PL-09 — The platform finds its workspaces

*Depends on: PL-01.* · issue #1563 · multi-tenancy.md §4.3

**Use cases:** UC-INS-13 (10)

The contract: `external_reference` in provisioning and `PATCH /admin/tenants/{id}` (with
`If-Match`), unique on the installation; `GET /admin/tenants/{id}`; `POST /admin/tenants:query`
filtered by short name, reference, plan and status, paged with `size` and `cursor` and answering
`{data, page}` (a reference stays out of query strings and access logs; `GET /admin/tenants` stays as
it is). The reference is `PERSONAL_TECHNICAL` in the data catalogue, in no log, metric or journal
detail (rule 10), and removed at the purge. `hubctl admin tenant show`, `find --reference`.

**Acceptance:** application tests for a duplicate reference refused, a change, and the purge
removing it; a Testcontainers test for the filters and the cursor; a test that a provisioning call
replayed after its key expired can be reconciled by reference; a `hubctl` test.

---

## PL-10 — The platform's own credential

*Depends on: PL-01.* · issue #1564 · api-guidelines.md · multi-tenancy.md §4.3, §5

**Use cases:** UC-INS-06 (6), UC-INS-08 (7)

`admin:provisioning` in the catalogue, granted only with the operator register like `admin:tenants`,
on the verbs multi-tenancy.md §4.3 lists and on none else; offered when an operator creates a
platform's token on the screen and with `hubctl`. A deletion request from a service account in the
register takes the typed name and no step-up; a person's still takes one. Both are security gates:
the threat model and `gate-security` name them.

**Acceptance:** a test per control-plane operation that the narrow scope is admitted or refused as
listed; a test that the scope is refused once the account leaves the register; deletion tests for a
service account without a step-up and a person without one refused; the grace, cancel and legal-hold
tests run for the platform's request; the token screen walked, with the preview shown to the owner
first (Decision 16).

---

## PL-11 — The provider's account link

*Depends on: PL-05.* · issue #1565 · multi-tenancy.md §4.2 · identity.md §18

**Use cases:** UC-INS-13 (11)

One address at the installation and per plan, lockable, resolved like the legal links, answered to
owners and administrators with the entitlements and the limits. The web app shows "Your provider's
account page" beside "Not included in your plan", at a reached limit and on the suspended page; its
label is a message code, the address the operator's. No price, no plan names, no "upgrade".

**Acceptance:** resolution tests across installation and plan with a lock; a test that a member is
not answered the link; e2e walks of the three places, with and without a link; the preview shown to
the owner first (Decision 16).

---

## The matrix

| Door | What PL changes |
|---|---|
| Web app | The `/instance` area gains *Plans* and the plan on workspaces (PL-06); a workspace's screens say "Set by your plan" and "Not included in your plan" (PL-02, PL-03, PL-05). |
| REST | `/admin/plans`, a workspace's plan, the plan in provisioning (PL-01); the origin `PLAN` in the readers (PL-02, PL-03); one workspace and the filtered list (PL-09); `admin:provisioning` (PL-10); events, subscriptions and use (PL-07, PL-08). |
| `hubctl`, instance file | `hubctl admin plan`, `tenant show`, `subscription`, `usage`; plans, subscriptions and the metering switch in the file (PL-01, PL-07, PL-08, PL-09). |
| MCP, automation | Nothing to write — plans are the control plane's. A rule in a workspace without the entitlement does not run (PL-05). |
| Background worker | Reads the entitlements before running a rule or a delivery (PL-05); delivers the installation's events (PL-07); moves use to the installation ledger at the purge and sweeps it (PL-08). |
| Installation level | The plan level beneath it; its locks bind every plan. |

| Account | Plans |
|---|---|
| Operator | Define, change, assign plans; provision on one; subscribe the platform; set the account link. |
| Platform's service account | With `admin:provisioning`: provision, find, move, suspend, resume, request a deletion without a step-up, read use and events (PL-10). |
| Owner, administrator | See their plan, its values with their origin, the limits, what is not included and the provider's account link; change nothing a plan locks. |
| Member, guest, managed account | Meet the resolved rules; a managed account is counted by the people limit. |
| Service account | Not counted by the people limit. |

| State of a workspace | Plans |
|---|---|
| Active | As above. |
| Suspended, pending deletion | Can be moved; the plan applies as the workspace is used again. |
| No plan | The installation's values, every entitlement — today's behaviour. |

Deployments: D1–D3 single mode, nothing appears; D4 in multi mode can define plans and need not;
D5 and D6 are the use case; D7 brings plans up from the file.

## The cut checklist

1. **What it delivers** — UC-INS-13 is reworded by #1530, #1532, #1550 and #1554; UC-INS-14 enters
   PL by #1551; UC-INS-10 changes by #1533, UC-INS-06 by #1553, UC-INS-08 by #1552, UC-INS-14 check 8
   by #1555. The six use cases whose checks already name the plan (UC-ID-12, UC-ID-16, UC-ID-20,
   UC-AI-06, UC-AI-07, UC-PRV-05) hold today only because no plan exists, and PL makes them hold with
   one. Walked through D1–D7 above.
2. **Principles** — P-03 (a downgrade rests, never deletes; use outlives the purge as counts),
   P-04 (a platform's deletion keeps the grace and the cancellation), P-05 ("Not included" in place
   of a control; no plans in single mode), P-06 (one place per value, the origin shown), P-07 (the
   plan between, under the installation's locks), P-08 (every door, and no door switchable), P-11
   (the account link: a person learns whom to ask), P-15 (the platform pushes state in and reads
   use out; no price anywhere), P-16 (a left-out provider opens the fallback).
3. **The matrix** — above.
4. **The premise** — checked against `main` at 8343e20c9 and, for the platform's path, again on
   2026-10-16: `Effective(instance, plan, workspace)` takes a plan but applies it over an
   installation lock without the tighten rule; `EffectiveAi` takes one correctly; `quota.Resolve`
   and `EffectiveLegal` take none; `instance_setting` stores `PLAN` and the reads collapse it into
   `locked`; `tenant.plan_id` is unread, without a reference and without a `plan` table;
   provisioning and `AdminTenant` carry no plan and no reference; there is no
   `GET /admin/tenants/{id}` and the list is unpaged; a deletion demands a step-up a service account
   cannot give; `admin:tenants` is the control plane's only scope; no installation subscription or
   event feed exists, and a workspace subscription is switched off after three dead letters;
   `usage_record` records only AI tokens and automation runs, is always written although
   data-protection.md says metering is off, has no sweep, and goes with the workspace; no
   entitlement exists; there is no people quota.
5. **Decisions** — above; the owner's ten are answered (#1530–#1533, #1550–#1555), with
   Decision 1 and Decision 16 in the owner's words.
6. **The rules** — multi-tenancy.md §4.2 and §4.3 drafted, §4 gains the people row, §5 the
   platform's deletion; identity.md §18's legal links, account link and lock; api-guidelines.md the
   scope; data-protection.md and the data catalogue the metering and its 13 months;
   licensing-editions.md §4 says plans are the operator's, not the project's; ADR-0086 proposed,
   accepted when PL-01 merges; its context records where the code departs from ADR-0070 §3 and its
   seventh settlement, and how the research of 2026-10-16 found decisions 7 to 12.
7. **Findings** — none open belongs to PL. Three found at the cut are taken in rather than opened:
   metering always on against the documents (Decision 18, PL-08), no sweep for `usage_record`
   (PL-08), and a subscription switched off silently (PL-07, for installation subscriptions; a
   workspace's stays UC-INT-01's rule). #1303 stays in CP; PL-01 builds the version from the start.
   #1491 is about the AI budget's counting, not its level.
8. **Tasks** — each names its checks, dependencies and acceptance; together they carry `Delivers`.
9. **The standing rules** — rule 1 and rule 2 (plans in the domain, authorised in the application
   layer behind the register and the two scopes), rule 3 (`plan` and the installation's use ledger
   outside the tenant boundary by the three lists, `tenant.plan_id` read under the tenant's scope),
   rule 6 (deliveries to the platform through `GuardedClient`), rule 8 (the plan's display name and
   the account link are the operator's; the link's label and every refusal are codes), rule 10 (no
   plan name, reference or account link in a log, metric, event or journal detail beyond what
   UC-INS-14 check 3 allows), rule 11 and rule 12 (contract first, expand-only migrations); known
   traps: a read-then-write without a lock (the people limit, removing a plan in use, a stale move),
   a write on a refusal path (the "limit reached" event), a read policy that admits installation
   rows, writer copies in `cmd/server/main.go`.
10. **A look before the build** — the owner sees every screen before it is built (Decision 16):
    PL-02, PL-03, PL-04, PL-05, PL-06, PL-07, PL-10 and PL-11 each start with a preview the worker
    presents.
