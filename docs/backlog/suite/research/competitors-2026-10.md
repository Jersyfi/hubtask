# Competitor landscape for an enterprise work-management suite (state: October 2026)

Scope: what Jira/Atlassian, Asana, monday.com, ClickUp, Linear, Wrike, Smartsheet, Microsoft Planner/Project, Notion, Confluence and the open-source field offer in 2025–2026, as input to Hubtask's product strategy.

How reliable this is:
- Established features are stated from the vendors' documentation and long-standing product knowledge.
- 2025–2026 launches were checked by web search (October 2026). Many of those sources are partner or reseller blogs; claims that rest on them, or where sources disagreed, are marked **(unverified)**.
- Tier names and prices change often. Check them on the vendor's pricing page before quoting them.

---

## 1. Capability matrix

✓ = native and mature · ◐ = partial, basic, via add-on or a sister product, or a top tier only · – = absent

| Capability | Jira (+JSM, JPD) | Confluence | Asana | monday.com | ClickUp | Linear | Wrike | Smartsheet | MS Planner/Project | Notion | OpenProject | Plane |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Lists / boards / table views | ✓ | ◐ (databases) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Configurable workflows (statuses, transitions, conditions) | ✓ (deepest) | – | ◐ (rules, sections) | ◐ (status columns + automations) | ◐ (custom statuses) | ◐ (fixed categories) | ✓ (custom workflows, approvals) | ◐ | – | ◐ | ✓ (workflows per type × role) | ◐ |
| Custom fields / issue types | ✓ | ◐ | ✓ | ✓ | ✓ | ◐ | ✓ | ✓ | ◐ (Premium) | ✓ | ✓ | ✓ |
| No-code automation | ✓ | ✓ | ✓ | ✓ | ✓ | ◐ | ✓ | ✓ | ◐ (Power Automate) | ◐ (DB automations) | ◐ | ◐ |
| Sprints / scrum / velocity | ✓ | – | ◐ | ◐ (dev product) | ✓ | ✓ (cycles) | ◐ | – | ◐ (Premium sprints) | ◐ (templates) | ✓ | ✓ (cycles) |
| Roadmaps / timeline | ✓ (Plans) | ◐ | ✓ | ✓ | ✓ | ✓ (projects/initiatives) | ✓ | ✓ | ✓ | ◐ | ✓ | ✓ |
| Gantt, dependencies, critical path | ◐ (Plans; no CPM) | – | ◐ (timeline deps) | ✓ (Gantt, baseline) | ✓ (critical path) | – | ✓ (critical path) | ✓ (critical path) | ✓ (lead/lag, critical path in Project desktop) | – | ✓ | ◐ |
| Baselines | ◐ (unverified) | – | – | ✓ (Enterprise) | – | – | ✓ (snapshots) | ✓ | ✓ (Premium) | – | ✓ (baseline comparison) | – |
| Portfolios / programs | ✓ (Plans, Focus) | – | ✓ (Portfolios) | ✓ (portfolio solution) | ◐ | ✓ (initiatives) | ✓ (Strategic Portfolio Mgmt) | ✓ (Control Center) | ✓ (Portfolios in Planner) | – | ✓ | ◐ |
| Goals / OKRs | ✓ (Goals, Focus) | – | ✓ (Goals) | ◐ (goals template) | ✓ | ◐ (initiatives) | ◐ | ◐ | ✓ (Viva Goals retired → Planner Goals) | ◐ | – | – |
| Workload / resource mgmt | ✓ (Plans capacity) | – | ✓ (Workload, capacity) | ✓ | ✓ | – | ✓ (Resource, effort, bookings) | ✓ (Resource Management by Smartsheet) | ✓ (Project Online/desktop) | – | ◐ | – |
| Time tracking / timesheets | ◐ (log work; Tempo add-on) | – | ✓ (Advanced+) | ✓ | ✓ | – | ✓ (approval) | ◐ | ◐ | – | ✓ (cost reports) | ◐ |
| Budgets / cost tracking | – (add-ons) | – | ◐ (budget fields) | ◐ | ◐ | – | ✓ (Financials) | ✓ | ✓ (Project desktop/Online) | – | ✓ (budgets module) | – |
| Risk register | ◐ (template) | – | ◐ (template) | ◐ (template) | ◐ (template) | – | ◐ (AI risk agent) | ◐ (template) | ◐ | – | ◐ (types) | – |
| Approvals / proofing | ◐ (JSM approvals) | ◐ (page approvals, unverified) | ✓ (approvals, proofing) | ✓ | ✓ (proofing) | – | ✓ (best: proofing, multi-step) | ✓ (approval workflows) | ◐ (Power Automate) | – | ◐ | – |
| Forms / intake | ✓ (Jira forms, JSM portal) | ◐ | ✓ (branching forms) | ✓ (WorkForms) | ✓ | ✓ (Asks web forms, Enterprise) | ✓ (request forms) | ✓ | ✓ (MS Forms) | ✓ (Notion Forms) | ◐ | ✓ (intake) |
| Dashboards / reporting | ✓ (+Atlassian Analytics) | ◐ | ✓ (universal reporting) | ✓ | ✓ | ◐ (insights) | ✓ (BI, cross-tagging) | ✓ (dashboards, DataTable) | ✓ (Power BI) | ◐ (charts) | ◐ | ◐ |
| Service desk / ticketing / SLA | ✓ (JSM: ITSM, ITIL, SLA, CMDB/Assets) | – | – | ✓ (monday service) | ◐ | ◐ (triage, Asks) | ◐ | – | – | – | ◐ (BIM issue/request) | ◐ (intake) |
| Product discovery / ideas | ✓ (JPD) | – | – | ◐ | ◐ | ✓ (customer requests) | – | – | – | ◐ | – | – |
| CRM-lite | – (Customer Service Mgmt is adjacent) | – | – | ✓ (monday CRM) | ◐ | – | – | ◐ | – (Dynamics separately) | ◐ (templates) | – | – |
| Wiki / docs | ✓ (Confluence) | ✓ | ◐ (notes only) | ✓ (WorkDocs) | ✓ (Docs) | ✓ (project docs) | ◐ | – | ✓ (Loop) | ✓ | ✓ (wiki) | ✓ (Pages/wiki) |
| Whiteboards | ◐ (via Confluence) | ✓ (whiteboards) | – | ✓ (WorkCanvas) | ✓ | – | ◐ | – | ◐ (MS Whiteboard) | – | – | – |
| Chat / messaging | – (Loom, Slack) | ◐ | ◐ (messages) | ◐ (updates) | ✓ (Chat) | – | – | – | ✓ (Teams) | ◐ (comments) | ◐ | ◐ |
| Offline | – | – | – | – | ◐ | ✓ (local-first client) | – | – | ◐ (desktop) | ◐ (offline pages 2025) | – | – |
| AI assistant | ✓ (Rovo) | ✓ | ✓ (AI Studio) | ✓ (Sidekick) | ✓ (Brain) | ✓ | ✓ | ✓ | ✓ (Copilot) | ✓ | ◐ | ✓ |
| AI agents (assignable, autonomous) | ✓ (Rovo agents GA in Jira) | ✓ | ✓ (AI Teammates; GA unclear) | ✓ (monday agents) | ✓ (Super Agents) | ✓ (Linear Agent, 3rd-party agents) | ✓ (agents GA Feb 2026) | ◐ (Smart Agents early adopter) | ◐ (Project Manager agent, preview) | ✓ (Custom Agents) | ◐ (roadmap) | ✓ (unverified depth) |
| MCP server | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ◐ | ◐ | ✓ | ◐ | ✓ |
| Self-hosted option | – (Data Center EOL 2029) | – (same) | – | – | – | – | – | – | ◐ (Project Server) | – | ✓ | ✓ |
| Open source | – | – | – | – | – | – | – | – | – | – | ✓ (GPLv3) | ✓ (AGPL, open core) |

Notes: Microsoft's portfolio is confusing. Project for the web was folded into Planner (redirect from August 2025). Premium Planner features (Goals, Sprints, baselines, lead/lag dependencies, Portfolios) need a Planner and Project Plan 3/5 licence. Project Online and Project desktop remain for classic scheduling. Basecamp has no matrix column. It offers to-dos, a message board, chat ("Campfire"), docs, schedule, Hill Charts, Card Table and timesheets (2024). It deliberately offers no custom fields, dependencies or workflows.

---

## 2. Best in class, by area

### 2.1 Workflow configuration — Jira Cloud
Jira's configuration model is what an "enterprise" buyer will benchmark against:
- **Issue types and issue type schemes.** Each project gets a set of types (Epic, Story, Bug, Task, Sub-task, custom types), with hierarchy levels above Epic on Premium (Plans).
- **Workflows and workflow schemes.** A workflow is a set of statuses, each in a category (To Do / In Progress / Done), plus transitions. A transition can carry:
  - **conditions**, deciding who can see it: a group, a project role, the assignee only, sub-tasks closed;
  - **validators**, checking the input: required fields, a regex, a permission;
  - **post-functions**: set a field, assign, fire an event, trigger a webhook;
  - **transition screens** and **properties**.
- A workflow scheme maps issue types to workflows. Team-managed projects use a simplified editor with rules instead.
- **Screens, screen schemes and issue type screen schemes** control which fields appear on create, edit and view.
- **Field configurations** decide whether a field is required or hidden, and which renderer it uses. **Custom field contexts** give per-project options and defaults.
- **Permission schemes** have about 40 project permissions, each granted to groups, roles, a user custom field or the reporter. **Issue security schemes** control row-level visibility. **Notification schemes** route events to recipients.
- **JQL**, a query language over issues, drives filters, boards, automation scopes, dashboards and SLAs.
- **Automation:** triggers, conditions and branches with smart values, global and multi-project rules. Execution limits per tier are a common pricing complaint.

Wrike (custom item types, blueprints, multi-step approvals) and OpenProject (workflows per type × role matrix, roles with permissions) come closest. Linear deliberately offers less configuration, and that is part of its appeal.

### 2.2 Planning: Gantt, dependencies, critical path, baselines
- **Smartsheet, Wrike and MS Project desktop are the leaders.** All three schedule from dependencies (FS/SS/FF/SF with lead/lag), highlight the critical path, set baselines and show variance. MS Project adds resource levelling.
- **monday.com** has Gantt with baselines on Enterprise.
- **OpenProject** has Gantt with automatic or manual scheduling modes, baselines (since 13.x) and a "project phases/stages" model. This is unusual for open source.
- **Jira Plans** (Premium) offers cross-project timelines, capacity per team or sprint, scenarios and dependencies, but no critical-path calculation.

### 2.3 Portfolios, goals and OKRs
- **Asana:** Portfolios (status rollups, fields, workload across projects) plus Goals with automatic progress from sub-goals or projects.
- **Atlassian:** Goals and Projects (formerly Atlas) with weekly updates. "Focus" (from the Jira Align / Atlassian Focus line) does strategic portfolio planning and funding. Team '26 rebranded or extended the portfolio products; the exact naming is **(unverified)**.
- **Wrike:** "Strategic Portfolio Management" (Collaborate 2026).
- **Smartsheet Control Center:** provisions a standardised project from a blueprint at scale, with rollups.
- **Microsoft:** Viva Goals was retired at the end of 2025; Goals now live in Planner Premium.

### 2.4 Resource and workload management, time and budgets
- **Smartsheet's Resource Management** (formerly 10,000ft) is a dedicated module: scheduling by role and skill, utilisation, forecasting, time entry and budgets.
- **Wrike:** effort allocation, bookings, job roles, a timesheet approval workflow, and Wrike Financials (rate cards, cost and billing).
- **Asana:** Workload (effort fields) and capacity planning on Advanced/Enterprise, plus native time tracking.
- **ClickUp:** time tracking with billable rates, timesheets, workload view.
- **OpenProject:** time and cost reporting, a budgets module, unit costs.
- **Jira:** relies on the Tempo add-on (timesheets, planner, cost tracker).

### 2.5 Approvals, proofing and forms
- **Wrike** is best in class for creative proofing (annotations on images, video and PDF; multi-step approvals with decision states) and dynamic request forms with branching that create items from blueprints.
- **Asana:** approval task type, proofing, branching forms.
- **monday.com:** WorkForms with conditional logic.
- **Linear Asks:** web forms (April 2026, Enterprise) for requesters without a Linear account, feeding triage.

### 2.6 Service desk and ticketing — Jira Service Management
JSM is the reference:
- request types with a portal, queues and SLAs with calendars;
- ITIL practices (incident, problem, change with CAB approvals, release);
- **Assets** (CMDB, with object schemas);
- on-call and alerting (Opsgenie folded into JSM);
- virtual service agent / Rovo agents;
- knowledge base via Confluence;
- customer organisations.

Atlassian added **Customer Service Management** as a separate product for external support (2025). monday service and Asana-adjacent tools are lighter. Among open-source tools, Zammad and Znuny are separate projects.

### 2.7 Product discovery
**Jira Product Discovery**: ideas with impact/effort fields, formula-based prioritisation scores, insights linked from Slack, Zendesk and browser, views (matrix, board, timeline), roadmap publishing and delivery linking to Jira epics. **Linear** has customer requests linked to issues, plus initiatives.

### 2.8 Knowledge, docs and wikis
- **Confluence:** spaces, a page tree, templates and blueprints, live docs, page permissions and restrictions, inline comments, analytics, whiteboards, databases (2024), Loom embeds and Rovo search across connected tools.
- **Notion:** blocks; relational databases (relations, rollups, formulas 2.0); synced blocks; Sites; Forms; Calendar and Mail; offline pages (2025); Custom Agents.
- **Coda:** docs-as-apps with formulas, Packs (integrations) and buttons. Coda joined Grammarly (Superhuman) in December 2024.
- **Microsoft Loop:** components synchronised across Teams and Outlook.
- **ClickUp Docs:** nested pages linked to tasks.
- **Outline:** open source (BSL 1.1 → Apache after a delay, **(unverified)** exact terms), fast, Slack-like UX, SSO, collections.
- **Obsidian:** local-first Markdown and plugins; personal knowledge rather than team work.

### 2.8a Whiteboards
Miro (diagramming, workshops, Miro AI, Intelligent Canvas), Confluence whiteboards (sticky notes convert to Jira issues), monday WorkCanvas, ClickUp Whiteboards. Open source: AFFiNE (edgeless mode), Excalidraw, tldraw (not OSS-licensed for commercial use).

### 2.9 Chat and messaging
ClickUp Chat (2024–25) and Microsoft Teams are the only "integrated" chats. Huly bundles chat and video. Everyone else integrates with Slack or Teams.

### 2.10 Dashboards and reporting
- **Wrike:** cross-tagging, reports, BI export (Wrike Analyze).
- **Smartsheet:** dashboards, DataMesh, DataTable for large data sets.
- **Atlassian Analytics:** SQL and visual over the Atlassian Data Lake (Enterprise).
- **Asana:** universal reporting across teams and goals.

### 2.11 CRM-lite
**monday CRM** (pipelines, email sync, sequences, campaigns since 2025) is the only work-management vendor with a full CRM product. The others offer CRM templates.

---

## 3. Enterprise must-haves (what procurement asks for)

| Requirement | Typical answer in the market |
|---|---|
| SAML SSO + SCIM provisioning | Universal at Enterprise tier. Atlassian requires **Atlassian Guard** (formerly Access), an extra subscription. Asana, monday and ClickUp include it at Enterprise. Notion SAML on Business, SCIM on Enterprise. |
| Audit logs + export / SIEM streaming | Enterprise tier everywhere. Atlassian Guard Premium adds anomaly detection; Asana has an audit log API. |
| Data residency | Atlassian (EU, DE among others; "realm" pinning, Enterprise/Premium). Notion EU (Frankfurt/Ireland) on Enterprise only, sales-gated, with AI subprocessors possibly outside the region. ClickUp EU (Ireland) on Enterprise. monday EU (Frankfurt) region. Asana EU. Linear: no EU region found **(unverified)**. |
| Admin hub | Atlassian Administration (org, multi-site, policies), monday Enterprise admin, Asana Admin Console, ClickUp Enterprise controls. |
| Sandbox / release tracks | Atlassian **sandboxes** and release tracks (Premium/Enterprise), monday sandbox accounts **(unverified)**, Salesforce-style staging rare elsewhere. |
| Encryption controls | BYOK/EKM: Atlassian (Enterprise), Asana (EKM add-on), monday (Guardian tier, BYOK), Slack-style. |
| Compliance | SOC 2 Type II, ISO 27001/27017/27018 and GDPR DPA are table stakes. HIPAA BAA is common at Enterprise. FedRAMP for Atlassian (Moderate; High in work), Smartsheet Gov, Asana FedRAMP Moderate **(unverified)**. **BSI C5** (Germany) only for a few vendors (Atlassian, Microsoft). |
| Data loss prevention | Classification levels and policies (Atlassian Guard data security policies), Notion and Asana DLP integrations. |
| Guest / external access controls | Domain allow-lists, guest roles, link expiry. |
| AI governance | Admin toggles per feature or app, AI credit budgets, "no training on customer data", choice of model (Notion, ClickUp Brain2 routes to Claude/GPT/Gemini), audit of agent actions (Rovo, ClickUp, Wrike). |
| Uptime SLA | 99.9–99.95 % at Enterprise. |
| Self-hosted / on-prem | Effectively gone among the leaders: Atlassian Data Center stopped new licence sales on 30 March 2026, renewals and expansions end 30 March 2028, and it reaches end of life (read-only) on 28 March 2029. Microsoft Project Server persists. |

**Tier gating, a pattern worth copying or avoiding.** The typical ladder is Free → Standard/Starter → Premium/Business/Advanced → Enterprise.
- Gated at Premium: advanced planning (Jira Plans, Asana Portfolios/Workload), approvals, time tracking, sandbox, automation volume, guest controls.
- Gated at Enterprise: SAML/SCIM (or a separate add-on), audit logs, data residency, EKM, analytics.
- AI increasingly sits on a **credits** layer on top of the seat price: Atlassian AI credits, monday AI credits, Notion Custom Agents metered (about $10 per 1,000 credits **(unverified)**), Asana AI Studio Plus/Pro, Copilot licence for Microsoft's agent.
- "SSO tax" and "audit-log tax" are well-known buyer grievances. For a self-hosted Apache-2.0 product that includes them, this is a ready-made differentiator.

---

## 4. Trends 2025–2026

1. **Agents as assignees.** Every leader now lets you assign a task to an AI agent, @mention it in comments and embed it in workflows, with action logs:
   - Atlassian Rovo agents (GA in Jira, Team '26, May 2026);
   - ClickUp Super Agents (February 2026, ClickUp 4.0 only);
   - Asana AI Teammates (beta; GA around FY27 Q1, conflicting sources);
   - monday agents (Elevate 2025);
   - Wrike AI Agents (GA 5 February 2026: Risk Status Reporter, Triage, Intake, Agent Builder, agent chaining);
   - Linear Agent (March 2026) and "Loops" (scheduled/event agents, Business+);
   - Notion Custom Agents (GA around May 2026, scheduled and triggered, e.g. by meeting notes);
   - Microsoft Planner Project Manager agent (preview, Copilot licence).
2. **No-code agent and app builders.** Rovo Studio (agents, automations and Forge apps from prompts, GA 2026), Asana AI Studio (AI steps inside rules), Wrike Agent Builder, monday **Vibe** (prompt-to-app on mondayDB; vendor claims more than 17,000 apps), Smartsheet Smart Hub and Smart Flows.
3. **Third-party agents as teammates and MCP everywhere.** Linear lists Claude Code, Cursor, Devin and Copilot as workspace members. Confluence lets you @mention external agents (Lovable, Replit and others; open beta). The Rovo MCP server and Teamwork Graph CLI open context to outside AI tools. ClickUp agents can call external MCP servers (4.07). MCP is now a checkbox item, and Hubtask already ships it.
4. **A "work graph" as context moat.** Atlassian Teamwork Graph, Wrike Work Intelligence Graph, ClickUp Brain2 (enterprise search across tools), Notion AI connectors. The pitch: structured work data plus permissions equals reliable AI.
5. **Model choice and BYO models.** Notion and ClickUp let users pick between model vendors. Enterprises ask for AI processing in their region. Notion admits LLM subprocessors may sit outside the residency region. Hubtask's BYO model is aligned with this, and a strong argument for EU buyers.
6. **Convergence into "everything apps".** ClickUp 4.0 (tasks, docs, chat, AI in one sidebar). Atlassian merged Jira Work Management into Jira (2025) and bundles collections ("Teamwork Collection", "Service Collection", **(unverified naming)**). Mattermost is retiring Boards for native task management inside chat.
7. **Consolidation and cloud-only.** Atlassian DC EOL, Project for the web folded into Planner, Viva Goals retired, Coda into Grammarly/Superhuman. Customers who must stay on-prem are left with fewer options.
8. **AI pricing via credits.** Moving from per-seat AI add-ons to consumption credits, with admin budget controls.
9. **European digital sovereignty push.** openDesk (Germany, ZenDiS; uses OpenProject for project management), La Suite numérique (France, DINUM; "Docs" co-developed with Germany, MIT), MijnBureau (Netherlands). The ICC moved to openDesk in 2025. OpenProject's 2026 roadmap positions it as a "sovereign alternative to Jira".

---

## 5. Self-hosted and EU landscape

| Product | Licence / model | Strengths | Weaknesses / notes |
|---|---|---|---|
| **OpenProject** (Bonn, DE) | GPLv3 core; Enterprise add-ons (SSO extras, boards, BIM, team planner) | Classic PM depth: Gantt, baselines, workflows per type × role, time and cost, budgets, meetings, wiki, Nextcloud integration, BIM edition. Part of openDesk; ISO 27001 for cloud. 28 releases in 2025; 2026 focus on a Jira alternative, AI and enterprise PM. | Rails, heavy UI; agile and modern UX weaker than Linear/Jira; no offline. |
| **Plane** (India/US) | AGPL community; commercial self-hosted tiers ("Plane One", Business/Enterprise) | Linear-like UX, cycles, modules, intake, pages/wiki, AI agents, air-gapped deployment. | Open core with key features paid; young enterprise feature set. |
| **Taiga** (Kaleidos, ES) | MPL-2.0 | Scrum/kanban, epics, user stories. | Slow development; dated. |
| **Focalboard / Mattermost Boards** | MIT/AGPL | — | Effectively discontinued: removed from the marketplace; Mattermost builds native tasks in chat. |
| **Vikunja** (DE) | AGPL | Lightweight personal/team to-dos: list, kanban, Gantt, table; CalDAV; Go backend. | Not enterprise (thin RBAC, audit, SSO). Closest technically to Hubtask's lower tier. |
| **Leantime** (US) | AGPL (+ plugins) | Neurodiversity-friendly PM: goals, ideas, timesheets, strategy canvases. | Small team; enterprise features thin. Not freshly verified. |
| **Huly** | EPL-2.0 | All-in-one: issues, docs, chat, video, HR; GitHub sync. | Heavy (CockroachDB/Postgres, many services; 8–16 GB RAM); hosted cloud reportedly shut down **(unverified)**; sustainability question. |
| **AppFlowy** | AGPL backend, open core | Notion alternative; local AI / on-prem LLM. | Self-hosted multi-user needs a paid licence. |
| **AFFiNE** | MIT client; server licence disputed **(unverified)** | Docs + whiteboard (edgeless) + databases, local-first. | Reportedly no SAML/SCIM/LDAP. |
| **Outline** | BSL → Apache (delayed) **(unverified)** | Best open team wiki UX. | Wiki only. |
| **Nextcloud Deck** | AGPL | Kanban inside Nextcloud (files, Talk, Calendar). | Basic; good as an integration target. |
| **Zammad / Znuny** | AGPL / GPL | Open-source ticketing. | Separate products — a service-desk gap in PM suites. |
| **EU SaaS** (not OSS) | proprietary | Stackfield (DE, end-to-end encryption), MeisterTask (AT), Zenkit (DE), Factorial etc. | EU hosting, but closed and mostly SMB. |

### Gaps a sovereign, self-hosted, Apache-2.0 suite could fill

1. **A post-Atlassian-DC home.** Thousands of regulated organisations (public sector, defence, health, banks under DORA) must leave Jira/Confluence DC by 2029 and cannot or will not use US cloud (CLOUD Act). OpenProject is the only mature OSS candidate, and it is classic-PM in style. There is room for a modern, fast, offline-capable, *permissive-licence* alternative with real **workflow configuration**: statuses, transitions, conditions, validators, field requirements, permission sets. Data Center admins need these to migrate.
2. **Jira/Confluence importers that preserve configuration** (workflows, permissions, custom fields, history, attachments). Migration cost is the main switching barrier.
3. **Enterprise features without paywalls.** SSO/SCIM, audit log, retention/legal hold, backups and GDPR tooling are already in Hubtask and sold as Enterprise add-ons elsewhere (Atlassian Guard, Notion Enterprise). Plane and AppFlowy are open core with these paid.
4. **Sovereign AI.** BYO model (on-prem or EU-hosted LLMs), agent actions under the same authorisation and audit trail, MCP for agents, no data leaving the tenant. Competitors either route to US LLM providers or lock AI to credits.
5. **Service desk inside the work tool.** No OSS suite integrates a JSM-like service desk (portal, request types, SLAs, approvals) with project work. Hubtask's jumble inbox with mail intake is a starting point.
6. **Portfolio, resource and time for the public sector.** Capacity, timesheets with approval, budgets and baselines are thin outside OpenProject and absent in Plane, Vikunja and Huly.
7. **Interoperability with openDesk / La Suite / Nextcloud.** CalDAV (already present), OIDC with Keycloak, file links to Nextcloud, docs links to "Docs". Being a good citizen of the sovereign stack is a procurement advantage, e.g. to get onto openDesk-like stacks.
8. **Certifications and attestations.** For OSS self-hosted, that means BSI IT-Grundschutz/C5 hints, published threat models, SBOM, signed releases and the EU Cyber Resilience Act (applies from 2027): an "enterprise-ready by documentation" story.

What *not* to chase (low differentiation, high cost): integrated chat/video (Huly-style), a CRM, generic whiteboards and prompt-to-app builders. Integrate (Matrix/Element, Nextcloud Talk, an OSS whiteboard) rather than build.

---

## 6. Sources

Vendor and primary sources:
- Atlassian Data Center end of life: https://www.atlassian.com/fr/licensing/data-center-end-of-life
- Atlassian SEC exhibit (cloud, data residency, FedRAMP): https://www.sec.gov/Archives/edgar/data/0001650372/000165037225000045/ex991blogpostdatedseptembe.htm
- Jira Work Management merger: https://www.atlassian.com/licensing/jira-work-management ; https://www.atlassian.com/blog/jira/how-we-merged-jira-work-management-into-jira-using-jira
- Asana AI Teammates overview (DE): https://asana.com/de/resources/ai-teammates-overview
- monday AI overview 2026: https://monday.com/blog/ai-agents/everything-monday-ai-can-do/
- monday Elevate 2025 press release: https://monday.com/p/press-release/monday-com-expands-ai-powered-agents-crm-suite-and-enterprise-grade-capabilities/
- Notion 3.0 release: https://notion.com/releases/2025-09-18 ; Notion data residency: https://www.notion.com/ja-jp/help/data-residency
- ClickUp Super Agents: https://help.clickup.com/hc/en-us/articles/31010910371991-What-are-Super-Agents ; changelog: https://clickup.com/learn/topic/project-management/tools/clickup/news/
- Linear Asks web forms: https://linear.app/changelog/2026-04-02-web-forms-for-linear-asks ; changelog: https://linear.app/changelog/timeline
- Wrike AI Agents GA: https://www.businesswire.com/news/home/20260205150291/en/Wrike-Launches-AI-Agents-Delivering-Six-Days-of-Output-in-a-Five-Day-Work-Week ; agentic update: https://www.wrike.com/newsroom/wrike-expands-work-delivery-capabilities-with-agentic-ai-integration/
- Smartsheet Intelligent Work Management: https://www.businesswire.com/news/home/20251105764524/en/Smartsheet-Debuts-Intelligent-Work-Management-Unifying-AI-Data-and-People-to-Accelerate-Enterprise-Execution
- Microsoft Planner FAQ (Premium, Project Manager agent): https://support.microsoft.com/en-us/Planner/frequently-asked-questions-about-microsoft-planner
- OpenProject 2025 review: https://www.openproject.org/blog/highlights-2025 ; roadmap 2026: https://www.openproject.org/pt/blog/roadmap-2026/ ; openDesk: https://openproject.org/opendesk
- Plane vs OpenProject (vendor): https://plane.so/blog/plane-vs-openproject-which-should-you-choose-in-2026
- Mattermost Boards future: https://forum.mattermost.com/t/whats-next-for-mattermost-boards/22446
- AFFiNE self-host: https://affine.pro/self-host ; AppFlowy vs AFFiNE (competitor-authored): https://appflowy.com/compare/appflowy-vs-affine
- Docs (La Suite / openDesk): https://en.wikipedia.org/wiki/Docs_(software)

Secondary (partner blogs, reviews — treat with caution):
- Team '26 summary: https://www.schneider.im/atlassian-team-26-changes-across-jira-confluence-rovo-focus-talent-more/ ; https://buzzclan.com/ai/atlassian-rovo/
- DC EOL timeline: https://www.eficode.com/de/blog/atlassian-stellt-data-center-ein?hsLang=en ; https://www.stagil.com/stagil-apps/data-center-end-of-life-what-does-it-mean-for-marketplace-apps
- Asana Teammates: https://www.computerworld.com/article/4063082/asana-puts-ai-teammate-agents-to-work.html ; https://forum.asana.com/t/the-guide-to-asana-ai-integrations-2026/1139500
- Notion agents: https://www.eesel.ai/es/blog/notion-ai-review ; https://matthiasfrank.de/en/notion-updates/store-your-companys-data-in-the-eu/
- ClickUp: https://techcrunch.com/2025/11/04/clickup-adds-new-ai-assistant-to-better-compete-with-slack-and-notion ; ClickUp vs Notion (EU residency): https://hackceleration.com/labs/compare/clickup-vs-notion
- Linear agents: https://blog.buildbetter.ai/linear-ai-agents-2026-guide-5-alternatives-for-engineering-teams/
- Project for the web → Planner: https://www.theprojectgroup.com/blog/en/project-for-the-web-becoming-planner/
- Huly: https://blog.elest.io/self-host-huly-one-open-source-app-to-replace-jira-linear-and-notion/ ; https://toolradar.com/guides/best-open-source-project-management-tools
- Focalboard alternatives: https://selfhosting.sh/compare/planka-vs-focalboard/
- Sovereign suites FR/DE/NL: https://academy.conduction.nl/academy/blog/la-frankendesk
