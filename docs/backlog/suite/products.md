# The products of the suite — scope before implementation

Accepted with the concept (2026-10-28). The **binding scope** of each product's first release is
the *MUST* rows of its section in [`../research/products-2026-10.md`](./research/products-2026-10.md)
(products 1–8 and the integrations catalogue) and [`../research/itsm-cmdb-ai-2026-10.md`](./research/itsm-cmdb-ai-2026-10.md)
§6 (CMDB/ITAM and agent governance); *SHOULD* is the following release, *LATER* is not scheduled.
Every product is a module (P-17), built from the building blocks of
[`../config/model.md`](./configuration/model.md) on the foundation of [`../foundation/`](./foundation.md),
connected by references (P-19).

| # | Product | First release in one sentence | Rests on |
|---|---|---|---|
| M1 | **Workflows & types** | Workspace-defined types with forms (layout + logic) and workflows (statuses with categories, steps with who / checks / asks / then), designer with draft, publish and migration preview | F-1, F-2, F-4 |
| M2 | **Planning** | Typed dependencies with lag, Gantt with mixed manual/automatic scheduling per entry, critical path, baselines, milestones, sprints with goal, velocity and burndown, releases, roadmap over hubs, tolerances raising exceptions | F-3, F-9, F-10 |
| M3 | **Knowledge** | Spaces per hub, page tree, block editor with live co-editing, comments inline, mentions, backlinks, embeds of live views and cards, templates, versions, verification dates, knowledge-base publishing for the portal, import/export (Confluence, Notion, Markdown, DOCX, HTML) | F-3, F-8, F-9 |
| M4 | **Whiteboard** | Infinite canvas with shapes, connectors, frames, stickies, voting, timer, templates (three retrospectives, Kanban, brainstorm, flowchart - the MUST rows of research §4.3; story map, PI board and mind map are SHOULD), live task and ticket cards, convert to entries; smooth at 5,000 objects | F-3, F-8 |
| M5 | **Service desk, intake, approvals** | Portal with request catalogue (type channels), mail and API channels, queues, SLAs/OLAs on business calendars with escalations, major incident, canned replies, knowledge deflection, CSAT, approvals single/quorum/sequential/CAB with delegation — ITIL 4 template | F-4, F-7, F-10 |
| M6 | **Assets & CMDB** (new) | CI class registry with a shipped hierarchy, typed relation catalogue with impact direction, a service model in the spirit of CSDM, lifecycle, compliance fields (owner, location, protection need C/I/A inherited along impact), one reconciliation write path for every source, connectors (file, GLPI, NetBox, Intune/Entra, Hetzner/OpenStack, alerts from Zabbix, Checkmk, Alertmanager), health (completeness, staleness, duplicates, orphans), impact analysis | F-1, F-3, F-11 |
| M7 | **Time & resources** | Timer and timesheets with approval and locked periods, estimates vs actuals, capacity, absences on shared calendars, workload (aggregated-only option), rates and budgets, export to accounting — no invoices | F-2, F-10 |
| M8 | **Goals, portfolios, insight** | Goals with key results fed by views, check-ins, portfolios with health and status updates, dashboards with aggregations and flow metrics, scheduled reports, the query language as a text line | F-9 |
| M9 | **Governed agents** (new, beside the existing AI) | Agent registry, scopes never above the delegating person, human-in-the-loop policies per action class, budgets and kill switch, model registry tied to consent, AI-Act marks, evaluation sets; first agents: ticket triage, CI suggestion, duplicate review, knowledge draft | F-12 |
| M10 | **Integrations** | The *Must* rows of the integrations catalogue: identity (SAML, SCIM, OIDC providers), chat notifications (Matrix first, Teams, Slack, Mattermost), mail (M365 Graph, Gmail, IMAP), calendar, files (Nextcloud, SharePoint, S3, WebDAV), code forges (GitLab, GitHub, Forgejo), monitoring alerts, automation (n8n, Zapier, Make), BI over a read API; openDesk and La Suite as first-class neighbours | F-7, F-11 |

## What this adds to the earlier suite concept

* **M6 Assets & CMDB is its own product**, not "records later": the owner's experience with a
  reduced asset database and incomplete inventory integrations is the gap to close. Its differentiator
  is the reconciliation engine plus compliance fields that cover ISO 27001 A.5.9, NIS2 and BSI
  IT-Grundschutz modelling at once.
* **M9 Governed agents** follows where the market leader is heading (governed, metered,
  approval-gated agents reachable over MCP) — on the customer's own installation and model.
* **Data quality before agents:** M6's health and reconciliation ship before the first ITSM agents.
* Products M1–M10 replace the module numbering of `../modules/` (which stays as background).

## Non-functional targets every product carries

From research §0 and the foundation: P95 read < 300 ms and write < 500 ms at 10⁶ entries per
workspace on the reference hardware; co-editing round trip < 150 ms in-region; offline for entries,
records and pages; WCAG 2.2 AA / EN 301 549 v4.1.1; every capability on every door (P-08, P-18);
export and import of everything (Data Act); no content in logs (rule 10); audit of every change.
