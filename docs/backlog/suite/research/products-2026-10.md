# Hubtask suite: feature specification per product (research, 2026-10-28)

Scope: what an enterprise buyer expects of each planned Hubtask product, judged against the
best-in-class offerings of 2025–2026. Each feature is one line. **MUST** = table stakes (a buyer
rejects the product without it), **SHOULD** = competitive (expected in a shortlist comparison),
**LATER** = differentiators. Non-functional targets are proposals derived from the references,
not vendor guarantees. Items marked *(unverified)* rest on secondary sources or on knowledge that
could not be confirmed this session.

Conventions used throughout:

- "Live card" = an embedded, permission-checked, real-time rendering of a Hubtask object (task,
  work package, ticket, goal, record) that can be edited in place.
- "Live view" = an embedded saved query rendered as list, board, table, timeline or chart.
- All products inherit the platform: tenants, SSO/SCIM, roles, audit log, offline sync, MCP, REST,
  webhooks, i18n, accessibility (WCAG 2.2 AA), retention and export.

---

## 0. Cross-cutting expectations (apply to every product)

| Area | MUST | SHOULD | LATER |
|---|---|---|---|
| Identity | SSO (OIDC, SAML), SCIM provisioning, groups, guest users | Just-in-time provisioning, external-collaborator restrictions per space | Attribute-based access (department, clearance) |
| Permissions | Space/project roles, item-level restriction, inheritance shown in UI | Permission explainer ("why can X see this") | Classification labels (public/internal/confidential) driving policy |
| Audit | Admin and content audit log, export | Per-object activity history in UI | SIEM streaming (syslog/CEF/OTLP) |
| Data | Full tenant export (JSON + attachments), retention policies, legal hold | Per-space export, scheduled export | Customer-managed keys (BYOK) |
| Search | One global search across all products, permission-trimmed | Filters per product, saved searches | Semantic/hybrid search, cross-product answers (RAG) |
| Notifications | In-app, mail, digest, per-object watch | Chat delivery (Teams/Slack/Matrix), quiet hours | Smart batching, notification analytics |
| Automation | Rule engine: trigger → condition → action, across products | Scheduled rules, webhooks out, templates | Low-code flows with branches, AI actions |
| AI / agents | MCP tools for every product, AI off-switch per tenant, no training on tenant data | Summaries, drafting, Q&A grounded in permitted content | Autonomous agents with approval gates and audit |
| Accessibility | Keyboard navigation, screen-reader labels, contrast | Reduced motion, high-contrast theme | Accessible alternatives for canvases (outline view) |
| Mobile/offline | Responsive web, read offline | Edit offline with conflict resolution | Native apps |

---

## 1. Workflows & item types (Jira-grade)

References: Jira (company-/team-managed projects, workflow editor, issue types, screens, field
configurations), Linear (statuses, triage), OpenProject (types, workflows per role), monday
(status columns), ServiceNow (state models).

| Tier | Feature |
|---|---|
| MUST | Configurable item types per project (task, bug, story, epic, ticket, change …) with icon and colour token |
| MUST | Custom fields: text, rich text, number, currency, date, date-time, single/multi select, user, group, URL, checkbox, label, cascading select |
| MUST | Field contexts: which fields apply to which type/project, required/optional, default values |
| MUST | Visual workflow editor: statuses, status categories (to do / in progress / done), transitions |
| MUST | Transition rules: conditions (role, field value), validators (required fields), post-functions (set field, assign) |
| MUST | Transition screens (ask for fields on transition, e.g. resolution) |
| MUST | Workflow schemes shared across projects; migration of items when a workflow changes |
| MUST | Hierarchy configuration (epic → story → sub-task), mapped to Hubtask's five levels |
| MUST | Issue links with typed relations (blocks, relates, duplicates, clones, causes) |
| MUST | Boards (kanban/scrum) driven by workflow status, swimlanes, WIP limits |
| MUST | Query language with saved filters (JQL-equivalent), bulk edit, bulk transition |
| MUST | Item templates and default descriptions per type |
| MUST | Project templates (software, business, service, marketing) |
| SHOULD | Triage inbox for incoming items (Linear-style) |
| SHOULD | Calculated/formula fields, rollups from children |
| SHOULD | Per-status SLA timers usable outside service desk |
| SHOULD | Workflow simulation / validation ("which items would become stuck") |
| SHOULD | Field-level permissions (hide/readonly per role) |
| SHOULD | Components/areas with default assignee |
| SHOULD | Versioned workflow definitions with draft → publish |
| SHOULD | Import from Jira (CSV and REST), mapping of types/fields/statuses |
| LATER | Natural-language workflow creation by AI ("create an approval workflow for purchases") |
| LATER | Cross-project workflow analytics (time in status, bottleneck heat map) |
| LATER | Workflow marketplace / shareable workflow packages |

Non-functional: workflow transition < 300 ms p95 server side; bulk transition of 1,000 items in
< 60 s as a background job with progress; schema changes on a project with 100k items without
downtime (background migration); field count per tenant ≥ 1,000 without degrading list queries;
query language must compile to parameterised SQL (rule 9).

---

## 2. Planning: dependencies, Gantt, critical path, baselines, sprints, releases, roadmaps

References: Jira Plans (Advanced Roadmaps: hierarchy, scenarios, capacity, dependencies),
MS Project (scheduling engine, resource levelling, baselines), Smartsheet, Wrike (Gantt,
workload), OpenProject (15.4 added automatic scheduling alongside manual; modes can be mixed per
work package), Linear (cycles, projects with milestones, initiatives; team-led initiatives
Aug 2026), monday (timeline, dependencies).

### 2.1 Gantt and scheduling

| Tier | Feature |
|---|---|
| MUST | Gantt chart: bars, milestones, summary bars from children, today line, zoom day → year |
| MUST | Drag to move/resize, drag to create dependency |
| MUST | Dependency types FS, SS, FF, SF with lag/lead (positive and negative) |
| MUST | Manual and automatic scheduling per item (OpenProject model), mixed in one plan |
| MUST | Working calendars: workdays, holidays per tenant/project, non-working days skipped |
| MUST | Duration vs. dates (setting duration moves finish, setting finish changes duration) |
| MUST | Parent date roll-up from children |
| MUST | Cycle detection on dependencies with a clear error |
| MUST | Print/export Gantt to PDF and PNG; export plan to CSV/XLSX |
| SHOULD | Critical path calculation and highlighting; total and free float |
| SHOULD | Baselines: save ≥ 3 named baselines, show baseline vs. current bars, variance columns |
| SHOULD | Constraint types (start no earlier than, must finish on, as late as possible) |
| SHOULD | Cross-project dependencies, external dependency markers |
| SHOULD | Undo/redo of scheduling operations; "preview impact" before committing a move |
| SHOULD | Import/export MS Project XML (MSPDI), import from Smartsheet/Excel |
| LATER | Resource levelling (automatic delay to remove over-allocation) |
| LATER | Monte Carlo / probabilistic finish dates |
| LATER | Earned value metrics (PV, EV, AC, SPI, CPI) |

### 2.2 Agile: sprints/cycles, releases

| Tier | Feature |
|---|---|
| MUST | Sprints/cycles with start/end, goal, commitment, carry-over of unfinished items |
| MUST | Backlog with ranking, estimation (story points, time, t-shirt) |
| MUST | Sprint report, burndown/burnup, velocity chart |
| MUST | Releases/versions: fix version, release date, release notes generation, status |
| SHOULD | Automatic cycles (Linear-style recurring cadence), cooldown periods |
| SHOULD | Capacity per sprint from team availability (links to product 6) |
| SHOULD | Cumulative flow diagram, cycle-time and lead-time control charts |
| SHOULD | Release readiness view (open items, linked code/deployments) |
| LATER | AI sprint planning suggestions based on velocity and capacity |
| LATER | Release trains / PI planning (SAFe) with program board |

### 2.3 Roadmaps and portfolio timeline

| Tier | Feature |
|---|---|
| MUST | Roadmap timeline of epics/projects across teams, grouped by team/initiative |
| MUST | Hierarchy above projects (initiative → project → item), progress rolled up |
| MUST | Shareable read-only roadmap (internal link; public link optional per tenant policy) |
| SHOULD | Scenarios / sandbox plans: change dates and scope without touching live data, then commit (Jira Plans) |
| SHOULD | Dependency map across teams, conflict markers (successor starts before predecessor ends) |
| SHOULD | Now/Next/Later roadmap view for product teams |
| SHOULD | Project updates with health (on track / at risk / off track) and history |
| LATER | Capacity-aware auto-scheduling of a portfolio |
| LATER | What-if comparison of two scenarios side by side |

Non-functional: Gantt renders 5,000 rows at 60 fps scroll with virtualisation; open a plan with
10,000 items in < 3 s; rescheduling cascade across 2,000 dependent items < 1 s server side;
scheduling engine deterministic (same inputs → same dates) and uses the Clock port; offline
edits to dates reconcile without silently breaking dependencies (conflict surfaced).

---

## 3. Knowledge: wiki, pages, notes, real-time co-editing

References: Confluence (page trees, live docs without "publish", databases, whiteboards, page
and bulk archiving, analytics, templates, macros), Notion (blocks, databases, synced blocks,
wikis with verified pages that expire and notify owners), Outline, Docmost (AGPL; real-time
editing; Draw.io, Excalidraw, Mermaid built in; spaces, groups, page history), BookStack
(shelves/books/chapters), Slab, Guru (verification workflows), GitBook (docs-as-code, Git sync),
Microsoft Loop (portable components), Nuclino, Coda (docs with formulas/packs), La Suite Docs
(DINUM/ZenDiS; BlockNote + Yjs + HocusPocus; some export packages under GPL/AGPL).

### 3.1 Editor block catalogue

| Tier | Blocks |
|---|---|
| MUST | Paragraph, headings H1–H3 (H4–H6 optional), bulleted, numbered, checklist |
| MUST | Quote, callout/panel (info, warning, success, error), divider |
| MUST | Code block with syntax highlighting and language picker; inline code |
| MUST | Table with header row/column, merge cells, resize, sort |
| MUST | Image (upload, paste, drag), file attachment, video/audio embed |
| MUST | Link, @mention (person, page, item), date, emoji, status lozenge |
| MUST | Toggle/expand, table of contents, child-pages list |
| MUST | Live task card and live view (list/board/table) of Hubtask items |
| MUST | Math (KaTeX inline and block) |
| MUST | Mermaid diagram; draw.io or Excalidraw-style embedded diagram (Docmost has all three) |
| SHOULD | Columns/layouts, synced/excerpt block (reuse content across pages, Notion synced block / Confluence excerpt-include) |
| SHOULD | Inline database/table with typed properties and views (Notion/Confluence databases) |
| SHOULD | Embedded whiteboard (product 4) and Gantt/roadmap snapshot (product 2) |
| SHOULD | Decision log, action items with assignee/due date that become tasks |
| SHOULD | Web embeds (oEmbed/iframe allow-list): video, maps, Figma-like, Grafana panels |
| SHOULD | Page properties/metadata block and page-properties report |
| SHOULD | Footnotes, glossary/definition, anchor links |
| LATER | Formula/computed blocks (Coda-style), buttons that run automations |
| LATER | Portable components that live in chat/mail (Loop-style) |
| LATER | Charts from inline data or queries |

### 3.2 Collaboration

| Tier | Feature |
|---|---|
| MUST | Real-time co-editing (CRDT, e.g. Yjs) with presence, cursors, selections |
| MUST | Inline comments on selections, page comments, threads, resolve, @mentions |
| MUST | Version history with diff, restore, named versions |
| MUST | Autosave; "live doc" model with optional draft/publish mode per space (Confluence offers both) |
| MUST | Offline editing with merge on reconnect (CRDT makes this natural) |
| SHOULD | Suggestion mode (tracked changes) |
| SHOULD | Reactions, page watchers, change notifications with diff |
| SHOULD | Page-level "following" digest |
| LATER | Async video/voice notes inline |

### 3.3 Structure and lifecycle

| Tier | Feature |
|---|---|
| MUST | Spaces (team, project, personal), page tree with drag-reorder, move, copy |
| MUST | Breadcrumbs, backlinks ("linked from"), labels/tags |
| MUST | Archive pages and spaces (hidden from default search, restorable); trash with retention |
| MUST | Page templates (meeting notes, decision, retrospective, how-to, runbook, requirements, ADR) and space templates |
| SHOULD | Verification: owner, verified-until date, expiry reminders, "outdated" banner (Notion, Guru) |
| SHOULD | Page review/approval workflow before publication (regulated content) |
| SHOULD | Bulk archive/move via content manager (Confluence Premium) |
| SHOULD | Space home/overview with featured pages, recently updated |
| SHOULD | Analytics: views, viewers, time on page, stale-content report |
| LATER | Content lifecycle automation (auto-archive after N months unviewed, with owner confirmation) |
| LATER | Docs-as-code sync with Git (GitBook) |

### 3.4 Permissions

| Tier | Feature |
|---|---|
| MUST | Space permissions by role/group (view, edit, admin, export) |
| MUST | Page restrictions (view/edit) inherited down the tree, shown visibly |
| MUST | Guest access to selected spaces |
| SHOULD | Public/anonymous publishing per space (off by default, admin-controlled) |
| SHOULD | Permission inspector per page |
| LATER | Classification labels with export/sharing policies |

### 3.5 Search

| Tier | Feature |
|---|---|
| MUST | Full-text search with stemming for EN/DE/FR at least, permission-trimmed |
| MUST | Filters: space, author, label, date, type; search attachments (PDF, DOCX text) |
| MUST | Quick-switcher (Ctrl/Cmd-K) for pages and items |
| SHOULD | Typo tolerance, highlighting, search inside page (Ctrl-F works across collapsed toggles) |
| LATER | Semantic search and AI answers with citations |

### 3.6 Import / export

| Format | Import | Export | Tier |
|---|---|---|---|
| Markdown (CommonMark/GFM, folders, front matter) | yes | yes | MUST |
| HTML | yes | yes | MUST |
| PDF | – | yes (page, tree) | MUST |
| DOCX | yes | yes | MUST |
| Confluence (Cloud/DC space export: XML/HTML zip, or REST API) | yes | – | MUST (migration path for most EU enterprises) |
| Notion (Markdown & CSV export zip, or API) | yes | – | SHOULD |
| Outline / BookStack / Docmost / XWiki | yes | – | LATER (XWiki matters for openDesk users) |
| ODT | yes | yes | SHOULD (public sector) |
| Full space archive (JSON + assets, round-trippable) | yes | yes | MUST |

### 3.7 Knowledge-base publishing for the service desk

| Tier | Feature |
|---|---|
| MUST | Mark a space as knowledge base; publish articles to the portal (product 5) per audience |
| MUST | Article suggestions while a customer types a request (deflection) |
| MUST | "Was this helpful?" feedback and view counts |
| SHOULD | Multilingual article variants with fallback language |
| SHOULD | Agent can create article from a resolved ticket; link article to ticket |
| SHOULD | Internal-only vs. customer-visible articles in the same space |
| LATER | Content gap report (searches without results, tickets without article) |

### 3.8 AI

| Tier | Feature |
|---|---|
| SHOULD | Summarise page/thread, draft from prompt, rewrite tone, translate |
| SHOULD | Ask-the-wiki answers with citations, permission-trimmed |
| SHOULD | Extract action items into tasks |
| LATER | Stale/contradictory content detection; auto-suggest verification owners |
| LATER | Agent can edit pages via MCP with review-before-apply |

Non-functional: page open (cached) < 1 s p95, cold < 2 s for a 20-page document; co-editing
propagation < 200 ms p95 within region, ≥ 50 concurrent editors per page, ≥ 500 concurrent
viewers; document size ≥ 1 MB text / 2,000 blocks without editor lag (keystroke < 50 ms);
version history retained ≥ 1 year by default, configurable to unlimited, with snapshot
compaction of CRDT updates; search index freshness < 5 s; attachments up to configurable limit
(default 100 MB); exports stream (no full in-memory build). Rule 10: page content never in logs.

---

## 4. Whiteboard

References: Miro (hard cap 100,000 objects per board, but performance degrades from about 1,000
and Miro recommends staying under 5,000), Confluence whiteboards (smart objects synced to Jira
issues, voting added late 2024, AI clustering and summaries Jan 2025), FigJam, Excalidraw
(PNG/SVG/clipboard export, `.excalidraw` JSON, `.excalidrawlib` libraries; no native PDF),
tldraw SDK (viewport culling; default per-page shape cap raised to 4,000 in 2024; R-tree spatial
index in SDK 4.4, Feb 2026), Mural, Lucidspark/Lucidchart, ClickUp whiteboards, AFFiNE edgeless.

### 4.1 Canvas and objects

| Tier | Feature |
|---|---|
| MUST | Infinite canvas, pan/zoom, minimap, grid/snap, alignment guides |
| MUST | Sticky notes (colours, sizes), text, shapes (basic + flowchart set), freehand pen, highlighter, eraser |
| MUST | Connectors: straight/elbow/curved, arrowheads, labels, attach to shapes and stay attached |
| MUST | Frames (sections) with names, used for navigation, export and presentation |
| MUST | Images, file attachments, embeds of web links |
| MUST | Group, lock, layer order, duplicate, copy/paste between boards |
| MUST | Comments pinned to objects or positions; @mentions |
| MUST | Real-time multi-cursor collaboration, presence, follow a user |
| MUST | Version history / restore |
| SHOULD | Tables and mind-map nodes with auto layout |
| SHOULD | Shape libraries (custom and imported, e.g. `.excalidrawlib`), icon sets |
| SHOULD | Tags on stickies, filter by tag/author |
| SHOULD | Auto-layout for diagrams (flowchart, tree) |
| LATER | Diagram-as-code (Mermaid/PlantUML → shapes and back) |

### 4.2 Facilitation

| Tier | Feature |
|---|---|
| MUST | Voting sessions (votes per person, anonymous option, results) |
| MUST | Timer visible to all participants |
| MUST | Sticky clustering: by colour/tag manually; select-and-group |
| SHOULD | AI clustering by theme and summary to a wiki page (Confluence, Miro) |
| SHOULD | Presentation mode stepping through frames; "bring everyone to me" |
| SHOULD | Private mode (hidden stickies until reveal) for retros |
| SHOULD | Visitor/anonymous participants with link (admin-controllable) |
| LATER | Breakout areas, music, reactions overlay |
| LATER | Live polls and icebreaker widgets |

### 4.3 Templates

| Tier | Template |
|---|---|
| MUST | Retrospective (start/stop/continue, 4Ls, mad/sad/glad) |
| MUST | Kanban board (columns of live cards) |
| MUST | Brainstorm / affinity map |
| MUST | Flowchart |
| SHOULD | User story map (backbone of activities → stories → releases) |
| SHOULD | Mind map |
| SHOULD | PI planning / program board with teams × iterations and dependency strings |
| SHOULD | Lean canvas, SWOT, impact/effort matrix, RACI |
| SHOULD | BPMN-lite (events, tasks, gateways, pools/lanes) |
| SHOULD | Customer journey map, service blueprint |
| LATER | Tenant template gallery with governance (approved templates) |

### 4.4 Diagram types

| Tier | Type |
|---|---|
| MUST | Flowchart |
| SHOULD | ERD (entities, attributes, crow's-foot notation) |
| SHOULD | UML: class, sequence, activity (subset) |
| SHOULD | Network/infrastructure diagrams with IT icon set (servers, cloud, firewall); objects linkable to asset records (product 8) |
| SHOULD | Org chart |
| LATER | Full BPMN 2.0 with executable mapping to workflows (product 1) |
| LATER | C4 / architecture diagrams with live links to records |

### 4.5 Connected work (whiteboard → suite)

| Tier | Feature |
|---|---|
| MUST | Convert sticky/shape to task (choose project, type); becomes live card |
| MUST | Paste/insert live task cards; status/assignee edits sync both ways |
| SHOULD | Bulk convert a frame of stickies to items; create from a column with status mapping |
| SHOULD | Insert a live board/list view (query) |
| SHOULD | Embed whiteboard (or a frame) in a wiki page, kept live |
| LATER | Dependency strings on canvas create real dependencies (product 2) |

### 4.6 Import / export

| Tier | Feature |
|---|---|
| MUST | Export PNG, SVG, PDF (board, frame, selection) |
| MUST | Native JSON export/import (round-trip) |
| SHOULD | Import Excalidraw (`.excalidraw`), tldraw (`.tldr`) |
| SHOULD | Import draw.io/diagrams.net XML; export draw.io (for wiki embedding) |
| SHOULD | CSV → stickies (and back) |
| LATER | Import Miro board (via REST API, OAuth) *(unverified how complete Miro's API export is)* |
| LATER | Import/export Visio (VSDX) *(unverified effort; Lucid supports it)* |

### 4.7 Accessibility of canvases

| Tier | Feature |
|---|---|
| MUST | Keyboard: tab through objects, arrow-key move, shortcut to create sticky/connector |
| MUST | Screen-reader announcements for selection and object text; alt text for images |
| SHOULD | Outline/list view of the board (frames → objects) as accessible alternative |
| SHOULD | Respect reduced motion; zoom without loss of focus |

Non-functional: smooth interaction (≥ 50 fps pan/zoom) at 5,000 objects on a mid-range laptop;
usable up to ≥ 20,000 objects with culling and spatial index (tldraw approach); hard limit
documented (proposal 50,000); open a 5,000-object board in < 3 s; collaboration latency < 150 ms
p95; ≥ 50 simultaneous active editors, ≥ 300 viewers (workshop size); offline edits merge via
CRDT; export of a 5,000-object board to PDF < 30 s as background job. Note the in-app browser
pane does not tick rAF; verify motion with Playwright.

---

## 5. Intake forms, Service desk, Approvals

References: Jira Service Management (portal, queues, SLAs, Assets objects (50,000 included in
Premium/Enterprise), virtual service agent in Slack/Teams, incident/problem/change), ServiceNow
ITSM/CSM, Zendesk, Freshservice, TOPdesk (EU; SaaS and on-premise; self-service portal with
custom forms, service catalogue, knowledge, events-and-actions automation), Zammad (open source;
SLAs with business calendars and escalations; channels incl. mail, web form, phone, chat,
Telegram, WhatsApp, Microsoft 365; public/internal multilingual KB), GLPI, iTop, OTRS/Znuny,
HaloITSM, Ivanti.

### 5.1 Intake forms

| Tier | Feature |
|---|---|
| MUST | Form builder: field types from product 1, sections, help text, required, defaults |
| MUST | Conditional logic (show/hide/require by answer) |
| MUST | Form → item mapping (type, project, fields, assignee rules) |
| MUST | Public (anonymous, rate-limited, CAPTCHA-free bot protection e.g. proof-of-work) and authenticated forms |
| MUST | Attachments with size/type limits and malware scan hook |
| SHOULD | Multi-page forms, prefill via URL (non-personal data only), embed snippet |
| SHOULD | Multilingual forms |
| SHOULD | Form analytics (submissions, abandonment) |
| LATER | AI form generation from a description; AI field extraction from mail |

### 5.2 Channels (omnichannel)

| Tier | Channel |
|---|---|
| MUST | E-mail in/out (IMAP/SMTP, Microsoft Graph, Gmail API), threading, reply parsing, multiple addresses per desk |
| MUST | Customer portal (submit, track, comment, approve) |
| MUST | Agent web UI and REST/API |
| SHOULD | Chat widget on websites (async, with handover) |
| SHOULD | Teams / Slack / Matrix intake (message → ticket, notify) |
| SHOULD | Monitoring alerts as tickets (Alertmanager, Zabbix, Checkmk webhooks) |
| LATER | Telephony/CTI: call → ticket, screen pop, click-to-call (SIP/CTI via e.g. Asterisk/3CX/Teams Phone) *(unverified buyer demand outside large IT)* |
| LATER | WhatsApp Business, Telegram, SMS (Zammad offers WhatsApp/Telegram) |

### 5.3 Agent workspace and queues

| Tier | Feature |
|---|---|
| MUST | Queues as saved filters, counts, sort by SLA breach |
| MUST | Assignment: manual, round-robin, load-based, skill/group |
| MUST | Internal notes vs. public replies, clear visual separation |
| MUST | Canned responses with variables; macros (multi-action: set fields + reply + transition) |
| MUST | Collision detection (another agent is viewing/replying) |
| MUST | Merge/split tickets, link to problem/change/task |
| MUST | Customer and organisation sidebar (history, open tickets, assets) |
| SHOULD | Ticket views per agent, keyboard-first triage |
| SHOULD | Shared mailbox signature and per-desk branding |
| SHOULD | Agent AI: summary, suggested reply from KB, sentiment, auto-categorisation |

### 5.4 SLAs, OLAs, UCs, calendars, escalations

| Tier | Feature |
|---|---|
| MUST | SLA policies: first response, next response, resolution; per priority/customer/type |
| MUST | Business calendars (hours, time zones, holidays) per desk/customer |
| MUST | Pause conditions (waiting for customer), resume |
| MUST | Escalations: notify/reassign/raise priority at % of target and at breach |
| MUST | SLA reporting: met/breached, by team/customer |
| SHOULD | OLAs for internal teams and UCs for suppliers, tracked separately on the same ticket |
| SHOULD | Multiple SLA clocks per ticket, SLA history audit |
| LATER | Predicted breach (ML on queue state) |

### 5.5 ITSM processes

| Tier | Feature |
|---|---|
| MUST | Incident, service request, problem, change as item types with templates |
| MUST | Service catalogue with request items and fulfilment tasks |
| SHOULD | Major incident: declare, war-room channel, stakeholder updates, status page, post-incident review page (product 3) |
| SHOULD | Change management: risk assessment, change calendar, CAB approvals, freeze windows |
| SHOULD | Link to assets/CIs (product 8), impact analysis |
| SHOULD | On-call schedules, rotations, overrides; alert routing and escalation policies |
| LATER | Status page (public/internal) with subscriber notifications |
| LATER | Full CMDB with discovery (agentless network discovery) |

### 5.6 Customers and portal

| Tier | Feature |
|---|---|
| MUST | Customer accounts (portal-only, not licensed agents), customer organisations |
| MUST | Organisation-wide visibility of tickets (configurable) |
| MUST | Multilingual portal (UI + content), branding per desk |
| MUST | Knowledge deflection on portal (product 3, §3.7) |
| SHOULD | CSAT survey after resolution (1–5 or thumbs), comment; reports |
| SHOULD | Customer self-service: close, reopen, add participants |
| SHOULD | Custom portal domain per tenant (fits ADR-0095 host-names-workspace) |
| LATER | Virtual agent: conversational intake in portal/chat, KB answers, intent flows, handover |
| LATER | CES/NPS surveys |

### 5.7 Approvals

| Tier | Feature |
|---|---|
| MUST | Approval step in workflow: approvers by user, group, field (manager), role |
| MUST | Rules: any one, all, N of M, majority |
| MUST | Multi-level / sequential approvals (manager → budget owner → CAB) |
| MUST | Approve/reject with comment from mail link, portal, chat, mobile |
| MUST | Audit trail of decisions (who, when, what version of the request) |
| SHOULD | Delegation / out-of-office substitute, reminders and escalation on timeout |
| SHOULD | Conditional approvers (amount > X → director) |
| SHOULD | CAB: meeting agenda from pending changes, batch decisions |
| SHOULD | Generic approvals outside service desk (documents, purchase, time sheets, budgets) |
| LATER | E-signature-backed approvals (qualified signature for regulated cases) |

Non-functional: inbound mail to ticket < 60 s p95; SLA timers accurate to the minute and
computed from the Clock port with business calendar; 100k open tickets per tenant, queue load
< 1 s; portal WCAG 2.2 AA (public sector, EAA); mail loop and auto-reply detection; DKIM/SPF/
DMARC-aligned outbound; portal survives 10× normal load during major incidents; data retention
per desk (GDPR deletion of customer data with anonymisation, no irrecoverable deletion without
the owner's safeguards).

---

## 6. Time tracking, resources, capacity, budgets

References: Tempo (Timesheets, Capacity Planner, Financial Manager: budgets, cost rates,
CAPEX/OPEX, plan vs. actual), Harvest (time + invoicing), Clockify, Toggl (timers, reports),
Float and Resource Guru (resource scheduling, leave), Smartsheet Resource Management, Wrike
(workload), OpenProject (time and cost reports, budgets).

### 6.1 Time tracking

| Tier | Feature |
|---|---|
| MUST | Log time on any item (duration or start/end), comment, date, billable flag |
| MUST | Timer (start/stop, one running), from item, from browser and mobile |
| MUST | Weekly timesheet grid; copy last week |
| MUST | Timesheet submission and approval (by manager/project lead), lock periods |
| MUST | Work types/activities (development, meeting, support) |
| MUST | Reports by user, project, item, period, billable; export CSV/XLSX |
| SHOULD | Calendar-based logging (drag from meetings in M365/Google/CalDAV) |
| SHOULD | Reminders for missing time; required hours per workload scheme |
| SHOULD | Time on tickets with SLA/contract consumption (product 5) |
| SHOULD | Offline timer |
| LATER | AI time suggestions from activity (commits, edits, meetings) — must be privacy-reviewed (works councils) |
| LATER | Overtime and flexitime accounts |

### 6.2 Resources and capacity

| Tier | Feature |
|---|---|
| MUST | Availability per person: working hours, part-time, holidays, leave |
| MUST | Workload view (people × time) with allocation from assigned items and estimates |
| MUST | Over/under-allocation highlighting |
| SHOULD | Resource scheduling (Float-style): allocate people or roles to projects by hours/% |
| SHOULD | Placeholder/role-based (generic) resources, later replaced by people |
| SHOULD | Skills on people, find by skill |
| SHOULD | Team capacity feeding sprints (product 2) and portfolios (product 7) |
| SHOULD | Leave import from HR systems (CSV/API) |
| LATER | Scenario capacity planning (hiring plan) — Atlassian "Talent" territory |
| LATER | Automatic levelling suggestions |

### 6.3 Budgets and costs

| Tier | Feature |
|---|---|
| MUST | Project budget (hours and money), consumption from logged time × rate |
| MUST | Cost rates per person/role with validity dates; billing rates per client/project |
| SHOULD | Material/other costs entries |
| SHOULD | Plan vs. actual (planned allocation vs. logged), forecast at completion |
| SHOULD | CAPEX/OPEX classification for capitalisation reports |
| SHOULD | Export to accounting (DATEV for DE, CSV, ERP API) |
| LATER | Invoicing (Harvest-style) or handover to ERP |
| LATER | Multi-currency with exchange-rate table |

Non-functional: rate and cost data restricted to a "finance" permission (rule 2, application
layer); time entries immutable after approval except via correction entry (audit); reports over
1M entries < 5 s; works-council compatibility (no individual performance analytics by default,
configurable visibility of others' time) — important for DE/AT; timer state survives offline and
device switch.

---

## 7. Goals/OKR, portfolios, dashboards, reporting

References: Asana Goals and Portfolios, Atlassian Goals and Focus (Strategy Collection, Team '25:
Focus for strategic planning with funding, Talent for workforce planning, Align for enterprise
planning), Viva Goals (retired 31 Dec 2025; no feature investment since Dec 2024), Perdoo,
Weekdone, Jira dashboards, Power BI-style reporting.

Lessons from Viva Goals' retirement *(interpretation, not Microsoft's statement)*: a stand-alone
OKR tool struggles when goals are not wired to the work; export (API, Excel, PowerPoint) mattered
to customers at shutdown; goals belong inside the work platform with progress from real data.

### 7.1 Goals / OKR

| Tier | Feature |
|---|---|
| MUST | Goals with owner, period (quarter/year), parent goal, status, description |
| MUST | Key results: numeric (start/target/current), percentage, binary, milestone |
| MUST | Progress from linked work (items, projects) or from KR values, rolled up the tree |
| MUST | Check-ins / updates with confidence and comment, history |
| MUST | Alignment tree / map view, filter by team/period |
| MUST | Periods/cycles management, close period with grading |
| SHOULD | Automatic KR values from a query (count of items) or from an integration/metric |
| SHOULD | Update reminders and digest to stakeholders |
| SHOULD | Goal templates and OKR quality hints |
| LATER | AI drafting of OKRs and summarising check-ins |
| LATER | KPIs (continuous metrics) separate from OKRs (Perdoo-style) |

### 7.2 Portfolios

| Tier | Feature |
|---|---|
| MUST | Portfolio = collection of projects/hubs with status, owner, dates, progress |
| MUST | Portfolio table with custom fields, health, roll-up of progress and budget |
| MUST | Nested portfolios |
| SHOULD | Portfolio timeline (product 2) and workload (product 6) |
| SHOULD | Status reports on schedule, published to stakeholders |
| SHOULD | Intake / demand management: proposals scored and approved into projects (product 5 approvals) |
| LATER | Funding allocation and prioritisation scoring (Focus-style) |
| LATER | Strategy-to-execution map (goal → initiative → project → items) with investment view |

### 7.3 Dashboards and reporting

| Tier | Feature |
|---|---|
| MUST | Dashboards with gadgets: filter results, pie/bar/line, created vs. resolved, burndown, two-dimensional stats, SLA, goals, time |
| MUST | Share dashboards with groups; permission-trimmed data per viewer |
| MUST | Scheduled report mail (PDF/CSV) |
| MUST | Export of any report/table to CSV/XLSX |
| SHOULD | Report builder: pick entity, group by, aggregate, filter, chart type |
| SHOULD | Cross-product reports (tickets + time + goals) |
| SHOULD | Wallboard/TV mode |
| SHOULD | BI access: read-only SQL views or OData feed per tenant, permission-respecting (Power BI, Metabase, Superset) |
| LATER | Natural-language questions to charts (AI) |
| LATER | Historical snapshots / time-travel reports (state at date X) |

Non-functional: dashboard with 10 gadgets loads < 2 s p95 on 1M items (pre-aggregation where
needed); BI feed must enforce tenant isolation (rule 3) and permission filtering; all charts use
design-system tokens (rule 15), accessible colours and a table alternative; OKR progress updates
visible in < 5 s after source change.

---

## 8. Records / registers (assets, risks, contacts)

References: JSM Assets (object schemas, references, AQL), ServiceNow CMDB, iTop and GLPI (CMDB,
inventory), Notion/Confluence databases, Airtable-style bases *(Airtable not in brief; reference
only)*, GRC tools for risk registers.

| Tier | Feature |
|---|---|
| MUST | Register = typed schema (object types, attributes, references to other records) |
| MUST | Attribute types: those of product 1 plus reference, IP address, status, file |
| MUST | Table, card and detail views; filters; import CSV/XLSX with mapping; export |
| MUST | Link records to items/tickets/pages (bidirectional), show linked work on record |
| MUST | Per-register permissions and audit history per record |
| MUST | Templates: IT assets/CIs, risks (likelihood × impact, owner, mitigation, review date), contacts/organisations, vendors/contracts |
| SHOULD | Relationship graph view (CI dependencies), impact query |
| SHOULD | Risk matrix heat map, review reminders, treatment tasks |
| SHOULD | Contract renewal reminders (dates trigger automation) |
| SHOULD | Query language over records (AQL-equivalent) usable in forms and automation |
| SHOULD | Sync from external sources (LDAP users, Intune/Entra devices, CSV schedule) |
| LATER | Discovery agents / network scan |
| LATER | Compliance frameworks (ISO 27001 controls, NIS2) mapped to risks and evidence |

Non-functional: 1M records per tenant, attribute search < 1 s; reference integrity on delete
(no silent cascade; GDPR erasure of contact data with anonymisation); schema changes online.

---

## 9. Integrations & apps, AI agents

### 9.1 Platform capabilities

| Tier | Feature |
|---|---|
| MUST | Public REST API (OpenAPI), API tokens and OAuth 2.0 apps, scopes, rate limits |
| MUST | Webhooks (signed, retried, filtered by event), delivery log |
| MUST | MCP server exposing all products, permission-trimmed, with per-tool scopes |
| MUST | Integration settings per tenant, admin approval for apps |
| SHOULD | App framework: UI extension points (panels, item actions), sandboxed |
| SHOULD | Connector SDK and marketplace (internal first) |
| SHOULD | AI agents as first-class actors: agent identity, assignable to items, audit of every action, human approval gates, spending/run limits |
| LATER | Bring-your-own-model (OpenAI-compatible endpoints, EU-hosted models, local via Ollama-like runtimes) |
| LATER | Agent marketplace, multi-agent workflows triggered by automation |

### 9.2 Integration catalogue

Must = a typical EU enterprise RFP requires it; Nice = differentiating or segment-specific.

| Group | Integration | Must/Nice | Protocol |
|---|---|---|---|
| Identity | Microsoft Entra ID | Must | OIDC/SAML SSO, SCIM 2.0 provisioning |
| Identity | Google Workspace | Must | OIDC, SCIM via Google *(Google's SCIM push is limited to supported apps; unverified for custom apps)* |
| Identity | Keycloak | Must (sovereign) | OIDC/SAML; SCIM via extension or LDAP federation |
| Identity | Okta | Must | OIDC/SAML, SCIM 2.0 |
| Identity | LDAP / Active Directory | Must (on-prem) | LDAP(S) sync or via IdP; SCIM where bridged |
| Identity | Authentik, Zitadel, Univention (UCS, openDesk IAM) | Nice | OIDC/SAML, SCIM |
| Communication | Microsoft Teams | Must | Bot Framework / Graph, adaptive cards, tabs, message extension |
| Communication | Slack | Must | Slack app (OAuth), Events API, Block Kit, slash commands, unfurls |
| Communication | Matrix / Element | Must (sovereign, openDesk) | Matrix client-server API bot, widgets |
| Communication | Mattermost | Nice | Incoming/outgoing webhooks, plugin/bot API |
| Communication | Rocket.Chat | Nice | Webhooks, REST bot |
| Communication | Nextcloud Talk | Nice | Talk bot API (webhook) |
| Mail | Exchange Online / M365 | Must | Microsoft Graph (mail, OAuth2); legacy EWS not for new work |
| Mail | Gmail | Must | Gmail API / IMAP with OAuth2 (XOAUTH2) |
| Mail | Any IMAP/SMTP | Must | IMAP IDLE, SMTP submission, DKIM signing |
| Mail | Outlook add-in (mail → task/ticket) | Nice | Office add-in (JS) |
| Calendar | CalDAV / iCal feeds | Must | CalDAV, iCalendar (RFC 5545) subscription |
| Calendar | M365 Calendar | Must | Graph |
| Calendar | Google Calendar | Must | Google Calendar API |
| Files | Nextcloud | Must (sovereign) | WebDAV, OCS share API, Nextcloud integration app (OpenProject has one) |
| Files | SharePoint / OneDrive | Must | Graph, file picker |
| Files | Google Drive | Should | Drive API, picker |
| Files | S3-compatible storage (attachments backend) | Must | S3 API |
| Files | WebDAV generic | Nice | WebDAV |
| Files | Collabora / OnlyOffice editing of attachments | Should (sovereign) | WOPI |
| Code | GitHub | Must | GitHub App, webhooks, smart commits/branch names, PR status on item |
| Code | GitLab | Must | Webhooks, OAuth app, REST/GraphQL |
| Code | Gitea / Forgejo | Should | Webhooks, REST |
| Code | Bitbucket | Nice | Webhooks, OAuth |
| Code | Azure DevOps | Nice | Service hooks, REST, OAuth/PAT |
| CI/CD | Deployments and builds on items (GitHub Actions, GitLab CI, Jenkins, Argo CD) | Should | Webhooks / deployment API (generic) |
| Monitoring | Prometheus Alertmanager | Must (IT) | Webhook receiver |
| Monitoring | Grafana (alerts, panel embeds) | Should | Webhook contact point, iframe/render API |
| Monitoring | Zabbix | Should (EU IT) | Media type webhook |
| Monitoring | Checkmk | Should (DE IT) | Notification plugin/webhook |
| Monitoring | PagerDuty / Opsgenie-like / Alerta | Nice | Events API v2, webhooks (or built-in on-call, §5.5) |
| CRM/ERP | SAP (S/4HANA, Business One) | Nice (large DE) | OData, REST, IDoc via middleware |
| CRM/ERP | Salesforce | Nice | REST, OAuth, platform events |
| CRM/ERP | Odoo | Nice | JSON-RPC/XML-RPC, REST |
| CRM/ERP | HubSpot | Nice | REST, OAuth, webhooks |
| CRM/ERP | DATEV | Should (DE market, time/costs) | DATEV export formats (CSV/ASCII) or DATEV Connect APIs *(unverified which API tier applies)* |
| BI | Power BI, Excel | Must | OData v4 feed or Power BI connector; CSV/XLSX |
| BI | Metabase, Superset, Grafana | Should | Read-only SQL views (tenant-scoped role) or REST |
| Automation | n8n | Must (sovereign, self-hostable) | Node using REST + webhooks |
| Automation | Zapier, Make | Should | Their app platforms (REST + webhooks, OAuth) |
| Automation | Power Automate | Should | Custom connector (OpenAPI) |
| E-signature | DocuSign, Adobe Sign | Nice | REST + webhooks |
| E-signature | EU QES providers (e.g. Swisscom, D-Trust, Yousign, Skribble) | Nice | REST, eIDAS-compliant *(unverified provider APIs)* |
| Telephony | Teams Phone, 3CX, Asterisk/FreePBX, Placetel/sipgate (DE) | Nice | CTI webhooks, TAPI, SIP events |
| Sovereign suites | openDesk (Nextcloud, OX App Suite, Element, Jitsi, OpenProject, XWiki, Collabora, CryptPad, Notes/Docs; IAM via UCS/Keycloak) | Must for DE public sector | OIDC via suite IAM, Matrix widgets, Nextcloud files, CalDAV/IMAP to OX, portal tile |
| Sovereign suites | La Suite numérique (Docs, Visio, Tchap/Matrix, ProConnect) | Should (FR public sector) | OIDC (ProConnect), Matrix, Docs embed/link |
| Sovereign suites | Nextcloud Hub | Must | Files, Talk, Deck/Calendar, dashboard widget, unified search provider |
| Video | Jitsi, BigBlueButton, Teams, Zoom meeting links on items/events | Should | Link generation APIs |

---

## 10. Cross-product connections (what makes the suite feel connected)

A suite feels connected when every object can appear, live and permission-checked, in every
other product, and when a casual artefact (sticky, sentence, mail, chat message) can become
structured work in one step without losing the link back. Proposal of the connection matrix:

| From → To | Embed (live) | Link (backlink both sides) | Convert / create |
|---|---|---|---|
| Wiki page → Tasks | Live task card, live list/board/table/Gantt view of a query | @mention item; item shows "mentioned in pages" | Select text → create task; action-item block → task; checklist → work packages |
| Wiki page → Whiteboard | Embedded board or frame, editable in place | Board lists pages it is embedded in | "Open as whiteboard" from a page outline (headings → frames) |
| Whiteboard → Tasks | Live cards on canvas, two-way edits | Card shows "on board X" | Sticky/frame → items; dependency arrow → dependency |
| Whiteboard → Wiki | – | – | Cluster summary (AI) → page; retro board → retro page with action items |
| Task → Wiki | Page preview in task side panel | Linked pages section | "Create spec page" from item (template), "post-incident review" from incident |
| Meeting notes ↔ Calendar | Agenda from calendar event | Event links page | Notes action items → tasks with due dates |
| Ticket → Knowledge | Suggested articles in agent view and portal | Article ↔ tickets that used it | Resolved ticket → draft article |
| Ticket → Work | Linked engineering items shown on ticket (status to customer) | Bidirectional | Escalate ticket → bug/task in product project |
| Form → Anything | Form embed in page/portal | Submission links to item | Form creates item, record, approval, or page |
| Approval → Anything | Approval widget in page/ticket/time sheet | Decision record linked | Approved proposal → project/portfolio entry |
| Goal → Work | Goal card in page; progress gadget | Item shows goals it contributes to | Create project from goal; KR from query |
| Planning → Wiki | Roadmap/Gantt snapshot or live view in page | Release ↔ release notes page | Release → generated release notes page |
| Time → Work/Budget | Time panel on items, budget gadget on dashboards/pages | Time entries link items | Calendar event → time entry; timer from any card |
| Records → Work | Record card in page/ticket/whiteboard (asset on network diagram) | Record shows linked tickets, risks, changes | Risk → mitigation tasks; contract → renewal task |
| Chat ↔ Everything | Unfurl of any Hubtask link as live card in Teams/Slack/Matrix | Message permalink stored on item | Message → task/ticket; thread → page |
| Mail ↔ Everything | – | Mail thread on item/ticket | Mail → ticket/task; forward to a project address |
| AI/MCP | Agents act on all of the above via one tool surface | Every agent action has provenance | "Turn this page into a plan" (page → items + dependencies + Gantt) |

Platform prerequisites for this: one universal object reference (URI/ID scheme for every object
type), one embed/card renderer used by all clients and by chat unfurls, one permission check for
embeds (an embed never leaks the title of an item the viewer cannot see — show "restricted
item"), one backlink index, one search index, one activity stream, and one automation engine that
sees events from every product.

---

## 11. Notes on uncertain points

- Confluence live docs: the "no publish" behaviour is confirmed by secondary sources; their GA
  status in 2026 *(unverified)*. Confluence databases: described in support docs; feature depth
  *(unverified)*.
- tldraw default `maxShapesPerPage` after 2024 *(unverified beyond the 4,000 raise)*.
- Excalidraw has no native PDF export per secondary sources *(unverified against current
  release)*.
- Zammad channels list from third-party listings *(unverified against Zammad docs)*.
- TOPdesk SLM details from listings *(unverified)*.
- JSM business-hours/calendar configuration not confirmed in this session's sources, but is
  well-known product behaviour *(unverified in this session)*.
- La Suite Docs licensing: MIT core, but PDF/DOCX export depends on BlockNote XL packages under
  GPL/AGPL — relevant if Hubtask considers BlockNote (new dependency → owner decision).
- Non-functional numbers are proposals derived from references, not measured vendor values,
  except Miro's documented 100k cap and 1k/5k performance guidance.

---

## Sources

- Confluence what's new: https://www.atlassian.com/software/confluence/features/whats-new
- Confluence whiteboards (collaborate, advanced tools): https://support.atlassian.com/confluence-cloud/docs/collaborate-on-whiteboards/ , https://support.atlassian.com/confluence-cloud/docs/advanced-tools-and-workflows-in-whiteboards/
- Confluence explainer (live docs, databases, secondary): https://www.kolekti.com/resources/blog/what-is-confluence
- Confluence page and bulk archiving: https://www.atlassian.com/blog/confluence/introducing-page-and-bulk-archiving-in-confluence-cloud
- Confluence space archiving: https://confluence.atlassian.com/spaces/CONF58/pages/771891911/Archive+a+Space
- Notion wikis and verified pages: https://notion.com/help/wikis-and-verified-pages
- Docmost docs: https://docmost.com/docs/
- La Suite Docs README: https://cdn.jsdelivr.net/gh/suitenumerique/docs@main/README.md ; BlockNote: https://blocknotejs.org/ ; https://news.itsfoss.com/la-suite-numerique-docs
- Miro board performance: https://help.miro.com/hc/articles/360013588560
- Miro community on board size: https://community.miro.com/ideas/board-size-and-performance-27329
- tldraw performance: https://tldraw.dev/sdk-features/performance ; options: https://tldraw.dev/sdk-features/options ; SDK 4.4: https://tldraw.dev/blog/tldraw-sdk-4.4 ; 2024 release notes: https://tldraw.dev/blog/release-notes/release-notes-20240620
- Excalidraw package: https://classic.yarnpkg.com/en/package/@excalidraw/excalidraw ; PDF workaround: https://www.pdnob.com/pdf-tips/excalidraw-export-to-pdf.html
- JSM ITSM features: https://www.atlassian.com/software/jira/service-management/features/itsm ; Premium: https://www.atlassian.com/software/jira/service-desk/premium ; packaging update: https://www.atlassian.com/blog/jira-service-management/jsm-cloud-pricing-packaging-update
- TOPdesk: https://www.topdesk.com/en/?p=37593 ; G-Cloud listing: https://www.applytosupply.digitalmarketplace.service.gov.uk/g-cloud-14/services/230924019529259 ; https://www.getapp.com/it-management-software/a/topdesk/
- Zammad: https://railway.com/deploy/zammad--zammad.md ; https://www.capterra.com/p/207587/Zammad/
- OpenProject scheduling modes: https://www.openproject.org/docs/user-guide/gantt-chart/scheduling/ ; 15.4 release notes: https://www.openproject.org/docs/release-notes/15-4-0
- Linear: https://linear.app/docs/making-the-most-of-linear ; project planning: https://linear.app/docs/beta-project-planning ; team initiatives: https://linear.app/changelog/2026-08-13-team-initiatives
- Atlassian Strategy Collection (Focus, Talent, Align): https://www.atlassian.com/blog/announcements/strategy-collection ; https://www.eficode.com/blog/the-strategy-collection-bringing-work-and-talent-together
- Viva Goals retirement: https://learn.microsoft.com/viva/goals/goals-retirement ; FAQ: https://learn.microsoft.com/en-us/viva/goals/goals-retirement-faq
- Tempo shared features: https://help.tempo.io/kb/latest/shared-features-in-tempo-apps ; plan vs actual: https://tempo.io/product-news/compare-plan-vs-actual-in-financial-manager
- openDesk: https://www.opendesk.eu/en/product ; operations docs: https://docs.opendesk.eu/operations/introduction ; https://en.wikipedia.org/wiki/OpenDesk ; Element for openDesk: https://element.io/en/solutions/opendesk
