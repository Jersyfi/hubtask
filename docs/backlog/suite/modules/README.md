# The modules — overview

> **Background.** These per-module concepts were written on 2026-10-17, before the products were
> settled. [`../products.md`](../products.md) supersedes their numbering and their scope (M1 … M10,
> with Assets & CMDB and governed agents as products of their own); the *MUST* rows of the research
> are the binding first-release scope. Use them for the reasoning and the market parity, and re-read
> each against milestone UX's presets and *Flow* (UC-WRK-23 … UC-WRK-31) at its pull request.

A module is a bounded context with its own use cases, switched on per workspace (and, where marked,
per hub) under P-17 ([principles.md](../../../vision/principles.md)). It reuses what Hubtask already
is — the `WorkItem` with capability profiles, the rule engine, the query DSL, the trail, the backup,
the sync engine, MCP — and adds only what is missing.

## How a module is switched on (shared mechanism, wave 0)

* `tenant.settings.modules` — a set of module keys, written through one use case
  (`SetWorkspaceModules`), audited, readable by `READ_CONFIGURATION`.
* Per hub where marked: `container.policies.modules` narrows (never widens) the workspace's set.
* Plans (milestone PL) gain a `modules` feature list: a plan may *offer* modules, a lock may fix one
  on or off (P-07, "the stricter level wins and says who decided"). No plan → every module
  available (P-09: no licence key switches anything off; self-hosted has everything).
* `/meta/capabilities` reports the active modules; the web app draws nothing of an inactive one
  (P-05).
* Switching a module off hides, never deletes (P-03): its data stays, exports and backups keep it,
  switching it on again shows it unchanged.

## The modules

| # | Module | Prefix | Core of it | Reuses | Wave |
|---|---|---|---|---|---|
| 0 | Module switch | `UC-INS` | modules per workspace, per hub, per plan | settings, plans, capabilities | 1 |
| 1 | [Workflows & types](./workflow.md) | `UC-FLW` | item types, statuses with categories, transitions with conditions, validators, post-actions, screens, schemes | capability profiles, CEL, automation actions | 1 |
| 2 | [Planning](./planning.md) | `UC-PLN` | dependencies, milestones, Gantt with critical path, baselines, sprints, releases, estimates, roadmap | timeline layout, `startAt`, saved views | 1–2 |
| 3 | [Knowledge](./knowledge.md) | `UC-DOC` | pages and spaces, live co-editing, page templates, backlinks, tasks in pages, notes, version history | hubs as spaces, memberships, sync engine, media | 2 |
| 4 | [Intake & service](./service.md) | `UC-FRM`, `UC-SVC`, `UC-APR` | forms, request portal, queues, SLAs with business calendars, approvals | jumble, automation, notification | 2 |
| 5 | [Time & resources](./resources.md) | `UC-TIM` | timers, timesheets with approval, estimates vs actuals, capacity, workload, absences, cost rates, budgets | assignments, groups, custom fields | 3 |
| 6 | [Goals, portfolios & insight](./strategy.md) | `UC-GOL`, `UC-RPT` | goals and key results, portfolios across hubs, status updates, risks, dashboards, flow metrics, scheduled reports | query DSL, events | 2–3 |
| 7 | [Enterprise administration](./enterprise.md) | `UC-ID`, `UC-INS` | custom roles and permission schemes, field-level access, SAML, SCIM, configuration as code, sandbox copy, SIEM streaming, IP rules | identity, audit, instance file | 1–3 |
| 8 | [Platform & ecosystem](./platform.md) | `UC-INT`, `UC-AI`, `UC-REC`, `UC-CNV` | code-hosting links, chat channels, app manifests and UI extension points, AI agents, records, whiteboards | webhooks, OAuth apps, MCP, suggestion | 2–4 |

## Why this order

1. **Workflows first** — it is the change that turns Hubtask from a task manager into a work
   platform: every later module (service desk, approvals, sprints) is a workflow with a view on it.
   It also touches the core model, so it must land before other modules build on the old shape.
2. **Planning and enterprise identity next** — they are what a company evaluating against the
   market leader checks in the first hour (dependencies, Gantt, sprints; SAML, SCIM, custom roles).
3. **Knowledge and service** — the two largest adjacent markets; both are mostly new code on top of
   a settled model.
4. **Time, goals, insight** — they read across everything above; built last, they measure real data
   instead of a guess.
5. **Ecosystem** continuously; records and whiteboards last.

## Each module, against the cut checklist (`docs/backlog/README.md`)

| Item | State in this concept |
|---|---|
| 1 What it delivers | Use cases named per module; full drafts for wave 1 in [`../usecases/`](../drafts/README.md) |
| 2 Principles | Named per module; "Broken when" read for P-03, P-05, P-06, P-08, P-10, P-17 |
| 3 The matrix | Open — per module at its cut |
| 4 The premise | Checked against `domain-model.md` and the use cases on 2026-10-17; not against code |
| 5 Decisions | Ten owner questions in [`../decisions.md`](../decisions.md), answered in [`../accepted.md`](../accepted.md) |
| 6 The rules | Subject documents named per module; not drafted |
| 7 Findings | Not yet swept |
| 8 Tasks | Not cut; concept mode, pull requests S-00 … S-23 in [`../README.md`](../README.md) |
| 9 Standing rules | Read: rule 1, 2, 3, 8, 9, 10, 11, 15 matter most; see each module |
| 10 A look before the build | The owner's presentation and prototype (kept outside the repository); per-module prototypes on request |
