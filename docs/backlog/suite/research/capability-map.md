# Capability map — Hubtask today, the market, the target

The market evidence is [`competitors-2026-10.md`](./competitors-2026-10.md)
(web research of 2026-10-17; items marked *unverified* there stay unverified here). This file reads
it against Hubtask.

Legend: ✓ built · ◐ partial · – absent · **M#** the module that closes it, numbered as in [`modules/`](../modules/README.md) — background since
[`products.md`](../products.md) renumbered the products M1 … M10 (workflows stay M1; the others moved).

| Capability | Market leader(s) | Hubtask today | Target | Wave |
|---|---|---|---|---|
| Lists, boards, timeline, saved views | everyone | ✓ | ✓ + Gantt, workload, calendar, table | 1 |
| Custom fields | everyone | ✓ (8 kinds) | ✓ + computed fields, field schemes | 1 |
| Item types | Jira, Wrike, OpenProject | ◐ (3 fixed levels) | **M1** types per level | 1 |
| Configurable workflows (statuses, transitions, conditions, validators, post-actions, screens) | Jira (deepest), Wrike, OpenProject | – (completion + buckets) | **M1** | 1 |
| No-code automation | everyone | ✓ (6 triggers, CEL, dry run, replay) | ✓ + SLA and status triggers | — |
| Templates | everyone | ✓ (item trees) | ✓ + project templates with dependencies | 2 |
| Dependencies, Gantt, critical path, baselines | Smartsheet, Wrike, MS Project, OpenProject | – (“not planned”) | **M2** | 1–2 |
| Sprints, velocity, burndown, releases | Jira, Linear, ClickUp, OpenProject | – | **M2** | 2 |
| Wiki, pages, notes, co-editing | Confluence, Notion, ClickUp | – (Markdown notes on entries) | **M3** | 2 |
| Forms, intake | everyone | ◐ (jumble: mail, webhook, capture) | **M4** forms | 2 |
| Service desk: portal, queues, SLA | Jira Service Management, monday service | – | **M4** | 2 |
| Approvals | Wrike, Asana, monday, Smartsheet | – | **M4** | 2 |
| Time tracking, timesheets | Wrike, ClickUp, Asana, OpenProject | – | **M5** | 3 |
| Workload, capacity, budgets | Smartsheet RM, Wrike, MS Project | ◐ (auto-assign least-loaded) | **M5** | 3 |
| Goals, OKRs | Asana, Atlassian, ClickUp | – | **M6** | 3 |
| Portfolios, status updates | Asana, Wrike, Smartsheet | – | **M6** | 3 |
| Dashboards, flow metrics | everyone | ◐ (saved views, counts) | **M6** | 2 |
| Query language | Jira (JQL) | ◐ (query DSL in the API) | **M6** typed text form | 2 |
| Custom roles, permission schemes | Jira, OpenProject | ◐ (7 fixed roles, scopes down to one entry, private hubs) | **M7** | 1 |
| SAML, SCIM | everyone (often a paid add-on) | ◐ (OIDC; SAML/SCIM after 1.0) | **M7** in 1.1 | 1 |
| Audit trail | Enterprise tiers | ✓ hash-chained, verifiable, anchored — **beyond the market** | + SIEM streaming | 1 |
| Backup / restore drill, retention, legal hold, GDPR requests | rarely in product | ✓ — **beyond the market** | ✓ for every module | — |
| Sandbox, configuration as code | Atlassian (Premium+) | – (instance file for the installation) | **M7** | 3 |
| Offline | Linear (local-first) | ✓ sync engine, conformance test | ✓ for every module | — |
| MCP / agents | everyone; agents as assignees in 2026 | ✓ MCP inside the installation, AI optional | **M8** agents as members | 2 |
| BYO / sovereign AI model | Notion, ClickUp (model choice; not regional) | ✓ per workspace, provider-offered, budgets | ✓ | — |
| Code-hosting link | Jira, Linear | – | **M8** | 2 |
| Importers that keep configuration | Jira Cloud Migration Assistant | ◐ (Trello, Google Tasks, To Do, CSV) | **Jira / Confluence / OpenProject import with workflows and history** | 1–2 |
| Records / custom entities | monday, Notion databases | – | **M8** | 4 |
| Whiteboards | Miro, Confluence, ClickUp | – | **M8** (late) | 4 |
| Chat, video | ClickUp, Teams | – | **non-goal**, integrate | — |
| CRM | monday CRM | – | **non-goal**, records cover lists | — |
| Self-hosted | OpenProject, Plane; Atlassian DC ends 2029-03-28 | ✓ one image + PostgreSQL | ✓ | — |
| Licence | OpenProject GPLv3, Plane AGPL open-core | Apache-2.0, no paid edition | unchanged | — |

## Where Hubtask already beats the market

The guarantees enterprise tiers charge for, or nobody ships: verifiable trail, restore drill,
retention preview and legal hold, data-subject requests in the product, tenant isolation in the
database, SSO without an "SSO tax", BYO model with consent, offline with a conformance test.

## The opening

Atlassian Data Center reaches end of life on 2029-03-28 (new licences stopped 2026-03-30). The
organisations that must stay on-premises — public sector, health, finance under DORA, defence — need
a home with **Jira-grade workflows, a wiki and a service desk**, and an importer that carries their
configuration. OpenProject is the only mature open-source candidate and is classic project
management. That is the window this concept is sequenced for: workflows, planning, SAML/SCIM and the
Jira importer in wave 1, knowledge and service desk in wave 2 — usable by 2028, when renewals end.
