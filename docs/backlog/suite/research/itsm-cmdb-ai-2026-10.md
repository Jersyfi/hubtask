# CMDB/ITAM, ITSM and governed AI agents: research for Hubtask

Research date: 2026-10-28. Web research only; no repository changes. Claims a primary source did not
confirm are marked **(unverified)**. Vendor and competitor marketing is labelled as such. Where no
fetch confirmed a detail, it comes from general product knowledge and is marked **(background)**.
Check those before relying on them.

---

## 0. Executive summary

- **Why JSM Assets felt "reduced":** the tiers gate it. Standard gets 5,000 objects. Data Manager,
  Atlassian's reconciliation layer that sits closest to an IRE, is Premium/Enterprise only, Cloud
  only, and several of its adapters are still beta or EAP. Discovery is a separate on-premises
  scanner that writes XML files. The model is a flat object database, so classes carry no
  semantics, and API, AQL and custom fields all have hard caps (AQL `total` caps at 1,000, a
  custom field holds 200 objects, an object report shows 10,000). These are structural limits, not
  configuration mistakes.
- **What ServiceNow gets right:** it is not the table count. It is four things together:
  1. a **prescriptive service model** (CSDM)
  2. a **single write path with identification and per-attribute source authority** (IRE)
  3. **measured data quality** (CMDB Health, certification, Data Manager lifecycle)
  4. **connectors that write only through that path** (Service Graph Connectors)

  Hubtask can copy this design at small scale.
- **Where ServiceNow AI is heading:** it is becoming a governed, headless execution layer for
  *any* agent. Knowledge 2026 brought an MCP server ("Action Fabric", Claude as launch partner), an
  AI Control Tower that discovers, observes, governs, secures and measures (cost/ROI, kill
  switch, quotas), and in April 2026 a pricing change that folds AI into every tier, metered in
  "assists". ServiceNow says itself that a stale CMDB makes agents expensive and unreliable. For
  Hubtask the lesson is to build the CMDB quality loop and the governance layer before building
  many agents.
- **Hubtask's opening:** sovereign hosting, open data model, open connectors (GLPI, NetBox,
  i-doit, Intune, Zabbix/Checkmk, Wazuh/Greenbone, CycloneDX), BSI IT-Grundschutz and NIS2
  out of the box, and an agent governance layer on its existing MCP server and hash-chained
  audit trail. Every agent action becomes a reviewable, budgeted, attributable event.

---

## 1. Atlassian Assets (JSM) limitations 2025–2026

### 1.1 Tiering and object limits

| Item | Standard | Premium | Enterprise | Source / note |
|---|---|---|---|---|
| Assets available | Yes (Service Collection Standard, rolled out 2025/26) | Yes | Yes | [Atlassian community: Assets now part of Service Collection Standard](https://community.atlassian.com/forums/discussion/3175576/assets-is-now-part-of-service-collection-standard) |
| Included objects | 5,000 | 50,000 | 500,000 | [Manage the usage limit for Assets](https://support.atlassian.com/assets/docs/increase-the-usage-limit-for-assets/), [Assets licensing](https://www.atlassian.com/licensing/assets) |
| Announced increase | n/a | 10M | 10M | Third-party blog plus a community snippet; official page still shows 50k/500k **(unverified)** — [assetmanagementforjira.com](https://assetmanagementforjira.com/blog/jira-assets-limitations) |
| Extra objects | Add-on, per object per month | same | same | $0.05/object/month (older Atlassian announcement) vs $0.02 (third party), **unverified**; [JSM pricing update](https://www.atlassian.com/blog/announcements/jsm-cloud-pricing-packaging-update) |
| Data Manager | **No** | Yes | Yes | "Premium and Enterprise… not available for Data Center" — [Assets Data Manager](https://www.atlassian.com/software/jira/service-management/features/assets-data-manager) |
| Pooling | Org-level across subscriptions | | | [Assets licensing](https://www.atlassian.com/licensing/assets) |

Usage counts objects across all non-system schemas. Data Manager's staging objects do not count.
A 500-person company passes 5,000 objects quickly with laptops, licences and history
(competitor's claim, plausible).

### 1.2 Structural and technical limits

| Area | Limit / behaviour | Source |
|---|---|---|
| AQL REST `/object/aql` | `total` capped at 1,000; `hasMoreResults` flag; separate `/object/aql/totalcount`; `/object/navlist/aql` deprecated | [Atlassian developer community](https://community.developer.atlassian.com/t/changes-to-assets-object-aql-and-object-navlist-aql-endpoints/79919) |
| Assets custom field on a work item | Up to 200 objects per field (raised from 20); 50 in queues, boards and dashboards | [Assets custom field scalability](https://support.atlassian.com/assets/docs/assets-custom-field-scalability/) |
| Object search report | Hard-coded 10,000-object ceiling | [KB: too much data](https://support.atlassian.com/jira/kb/assets-object-search-report-shows-sorry-but-there-is-too-much-data-to-be-displayed-please-reduce-the-scope-by-refining-your-parameters/), [JSDSERVER-15096](https://jira.atlassian.com/browse/JSDSERVER-15096) |
| Automation "lookup objects" | 100 objects per query | [Isos: is Jira automation for Assets limited](https://blog.isostech.com/is-jira-automation-for-assets-limited), competitor blog |
| Schema limits | 100 schemas, 120 attributes per object type, 2 unique constraints per type (third party) **(unverified)** | [assetmanagementforjira.com](https://assetmanagementforjira.com/blog/jira-assets-limitations) |
| Performance | Reports of slowdowns above ~40–50k objects, with 30–45 s queries (anecdotal, competitor source) | same |
| Data model | Generic object types with attributes and references. No class semantics (no "a Server is a Computer is a Hardware CI"), no built-in service model, no inherited identification rules **(background)** | — |
| Relationships | References are attributes with a free-text reference type. No typed relationship catalogue with direction or cardinality, and no impact propagation **(background)** | — |
| Permissions | Per schema (roles: admin, user, customer) plus object-type visibility. Fine-grained attribute or row permissions are limited **(background)** | — |

### 1.3 Discovery and imports

| Mechanism | What it does | Limitation | Source |
|---|---|---|---|
| Assets Discovery (agentless scanner, agent, collector) | Scans the local network (WMI, SSH, SNMP), writes XML | Agents export by file copy or SFTP. A **Collector** then pushes to Cloud every 30 min, and agents scan every 24 h by default. Direct agent-to-cloud upload is still a suggestion ticket (JSDCLOUD-11802) | [Export agent scan results to Cloud](https://confluence.atlassian.com/assetapps/export-agent-scan-results-to-cloud-1520369768.html), [KB](https://support.atlassian.com/jira/kb/how-discovery-agents-can-export-to-cloud-instance/), [What is Asset discovery](https://support.atlassian.com/jira-service-management-cloud/docs/what-is-asset-discovery/) |
| Imports | CSV, JSON, LDAP/Entra-style object imports, REST API. Mapping is object-type by object-type **(background, partly)** | No per-attribute source precedence. The last import wins | competitor blog plus background |
| Data Manager | Adapters (Intune, Jamf, SCCM, Defender, CrowdStrike Falcon, Qualys, Tenable, AWS, Azure, Puppet, …) pull into staging, then cleanse, normalise, reconcile, and give "golden record" output to Assets | Premium/Enterprise only, Cloud only. Several adapters marked **beta/EAP** (Falcon EAP, Jamf "open beta" on one page). Atlassian says it "is not a discovery tool" | [Learn about adapters](https://support.atlassian.com/jira-service-management-cloud/docs/learn-about-adapters), [Jamf adapter](https://support.atlassian.com/jira-service-management-cloud/docs/data-manager-jamf-adapter/) |
| Live sync | None native. Sync needs scripts or Marketplace apps | Competitor claim: "not designed for external, volatile, or live data" | [assetmanagementforjira.com](https://assetmanagementforjira.com/blog/jira-assets-limitations) |

### 1.4 Typical complaints (community and third-party synthesis)

1. Tier gating forces Premium for reconciliation and scale, which brings Premium per-agent cost
   (competitor estimate: about $18.8k/yr extra for 50 agents).
2. Weak out-of-the-box integration with inventory tools. MDM data means manual work or paid apps
   (this matches the owner's experience).
3. No prescriptive service model. Every customer invents its own schema, and nothing like CSDM
   ships.
4. Caps in AQL, custom fields, reports and automation break real-world queries ("all laptops of
   this department").
5. Performance with large schemas, and imports that overwrite without attribute-level authority.
6. Discovery is a Windows/Java tool on premises that ships files, which is awkward for cloud-first
   setups.

**Implication for Hubtask:** the gap is not "an object DB". It is *a service model, plus
reconciliation, plus connectors, plus quality metrics*, with no tier gating and no object cap
beyond hardware.

---

## 2. ServiceNow CMDB done right

### 2.1 CSDM 5 (white paper May 2025)

Sources: [CSDM 5 white paper post](https://www.servicenow.com/community/common-service-data-model/csdm-5-finally-get-the-csdm-5-white-paper-here/ta-p/3254967),
[Top features of CSDM 5](https://www.servicenow.com/community/common-service-data-model-forum/top-features-of-csdm-5-what-you-need-to-know/td-p/3294791),
[CSDM data domains (docs)](https://www.servicenow.com/docs/r/zS6yb8e~gR8BPjeviYdV9g/YO9~nS73A2CUXGgC7Mjz6g).
The domain names below follow community summaries. Check the white paper for exact names.

| Domain | Purpose | Core classes (examples) |
|---|---|---|
| Foundation | Shared reference data | Company, Location, Department, User/Group, Contract, Product Model, Business Capability |
| Ideation & Strategy (new in 5) | Ideas, goals, strategic priorities | Product Idea (`sn_align_core_product_idea`), Goal, Strategy **(table names per community, unverified)** |
| Design & Planning | Architecture and portfolio planning | Business Application, Information Object, Business Capability map |
| Build & Integration | SDLC artefacts | Digital Product/Software, SDLC Component, **SBOM** tracking (new in 5) |
| Service Delivery (formerly "Manage Technical Services") | What runs | **Application Service** (`cmdb_ci_service_auto`, mapped/discovered/tag-based/calculated), **Technical Service** and **Technical Service Offering**, Dynamic CI Group. CSDM 5 adds **Data, Network, Facility and Operational Process service instances** besides application services |
| Service Consumption (formerly "Sell/Consume") | What is offered | **Business Service**, **Business Service Offering**, Service Portfolio, Request Catalog |
| Manage Portfolios (new) | Portfolio management across the above | Portfolios, product lines |
| AI governance (new classes) | Inventory of AI systems | `dm_ai_system_digital_asset`, AI Model classes **(community, unverified)** |

Key CSDM ideas worth copying:
- **Business vs technical service split.** A Business Service Offering (what a consumer
  subscribes to, with an SLA) depends on Application Services, which are instances of a Business
  Application running on infrastructure CIs. A Technical Service Offering is what an IT team
  provides to other IT teams, such as "Linux hosting Gold".
- **Offerings carry support and approval groups.** Incidents and changes are routed by the
  offering, not by the CI.
- **Life cycle as two fields:** `life_cycle_stage` and `life_cycle_stage_status`, standardised
  across classes, for example Operational/In Use or End of Life/Retired (since CSDM 4)
  **(background)**.

### 2.2 CI class hierarchy and relationships (background, ServiceNow docs)

- Single-table-per-class inheritance from `cmdb_ci`. Main branches are Hardware → Computer → Server
  (→ Linux/Windows Server), Network Gear, Storage. Application, Database (instance/catalog), Cloud
  resources (`cmdb_ci_cloud_*`, VM instance, Kubernetes cluster/namespace/pod), Service, and
  "Logical datacenter". Roughly 1,000+ out-of-the-box classes.
- **Class Manager** defines per class: attributes, identification rules (inherited), reconciliation
  rules, dependent-class rules, health inclusion rules, and "principal class" flag.
- **Relationships** live in `cmdb_rel_ci` (parent, child, type). Types are directional pairs such as
  "Depends on::Used by", "Runs on::Runs", "Hosted on::Hosts", "Contains::Contained by",
  "Connects to::Connected by", "Members of::Member of", "Virtualized by::Virtualizes".
  **Suggested relationships** restrict which type is valid between which classes. **Dependent CIs**
  (for example a DB instance that only makes sense with its host) get identified through their
  host relationship.
- **Non-CI relationships** link CIs to users, groups and locations ("managed by", "supported by").

### 2.3 IRE (Identification and Reconciliation Engine)

Sources: [IRE docs](https://www.servicenow.com/docs/r/zS6yb8e~gR8BPjeviYdV9g/Q5J8vrX3tTDtGnnBzK3D5g),
[Components and process](https://www.servicenow.com/docs/r/GCPjp~ldPTNRN99oH1DYFg/eUXumCBbcZFJ~GfPH0xxsA),
[Flexera IRE best practices](https://docs.flexera.com/ServiceNowFlexeraOneApp/SNapp/v1.6/Content/helplibrary/integrationHUB.htm).

| Concept | How it works |
|---|---|
| Single entry point | Every source (Discovery, SGC, import set, API) sends a payload `{items:[{className, values, lookup, related}], relations:[…]}` to `IdentificationEngine.createOrUpdateCI(source, payload)`. Discovery, connectors and transform maps should not write CMDB tables directly |
| Identification rules | Per class, an ordered list of **identifier entries** tried by priority. Each entry is a set of attributes that together are unique, for example (1) serial number plus serial type, (2) name, (3) MAC address via the network adapter lookup table. Entries can search **lookup tables** (serial number table, network adapter, IP) instead of the main CI table. Rules are inherited down the class tree |
| Source native key | If a payload carries `source_name` plus `source_native_key` (the ID in the source system), IRE matches on that first ("Source" table, `sys_object_source`). This makes reruns idempotent |
| Reconciliation rules | Per class and attribute set: which **data sources are authoritative**. With **data precedence** (numeric priority per source), a lower number wins, and a lower-priority source cannot overwrite an attribute a higher-priority source last set. An optional **staleness period** lets a lower source take over when the authoritative one has gone silent ("dynamic reconciliation") |
| Data source rules | Which sources may create CIs of a class at all (create vs update-only) |
| De-duplication | Multiple matches create a **duplicate task** (`reconcile_duplicate_task`) and a de-duplication template (which CI survives, how to merge relations) |
| Partial payloads / insufficient data | Payloads without enough identifiers are held (partial payload table) rather than inserted, to avoid junk CIs |
| Logs | Every IRE run is logged with the source, the matched rule and the reason. Data Source History shows which source last wrote which attribute |
| Known pitfalls | Precedence and reconciliation rules interact confusingly. Dependent-CI identification differs via transform maps. Attribute "flapping" between sources ([community](https://www.servicenow.com/community/servicenow-ai-platform-forum/query-on-data-precedence-rules-and-reconciliation-rules/td-p/1158663)) |

### 2.4 CMDB Health, certification, Data Manager (background, ServiceNow docs)

| Capability | Metrics / function |
|---|---|
| **CMDB Health dashboard** | Three KPI groups, rolled up per class and per CI: **Completeness** (required and recommended attributes filled), **Correctness** (duplicates, orphans such as a CI missing a required relationship, **staleness** with a default of 60 days without update), **Compliance** (CIs pass audit against templates). Scheduled jobs compute scores; health inclusion rules scope which CIs count |
| **CMDB 360 / multisource CMDB** | Per attribute: which sources reported which value, for spotting conflicts |
| **Certification policies** (Data Certification) | Scheduled campaigns ask a CI's owner or support group to attest attributes ("is this server still owned by you?"). Results feed Compliance |
| **CMDB Data Manager** | Policy-driven CI lifecycle. **Retire**, **archive** and **delete** policies select CIs by filter (for example stale for 90 days, or `life_cycle_stage = End of Life`), create review tasks for owners, then act. Archive moves CIs to archive tables with a restore window. Delete requires archive first and approval |
| **CMDB Workspace / Query Builder** | Graph queries across classes and relations, saved queries, "CMDB AI insights" in recent releases |
| **Health remediation** | "Remediate" creates tasks for duplicates and stale CIs, routed to owner groups |

### 2.5 Discovery and Service Mapping (background)

| Feature | Notes |
|---|---|
| **Discovery (agentless)** | MID Server inside the customer network. Phases: scan (port probes), classify (SNMP/SSH/WMI/WinRM), identify (via IRE), explore. **Horizontal discovery** (devices and what runs on them) uses **patterns** (a DSL in Pattern Designer) or legacy probes/sensors. Credentials stored per credential type and tried by order. Schedules per IP range. Cloud discovery uses provider APIs (AWS, Azure, GCP) with service accounts |
| **Agent Client Collector (ACC)** | Lightweight agent (based on the Sensu project) for endpoints and servers. Does discovery (ACC-V), monitoring checks (ACC-M), log collection (ACC-L) and policy-driven checks |
| **Service Mapping** | Top-down: start from an entry point (URL, port), follow traffic and configuration to map the application service. Variants are **tag-based** (cloud tags), **pattern-based** (top-down), **traffic-based** (netstat connections), plus "calculated" and "dynamic CI group" services in CSDM |
| **Event Management / AIOps** | Alerts bound to CIs; impact trees propagate via service maps to business service offerings |

### 2.6 Service Graph Connectors (SGC) (background)

Store apps, each with a certified mapping to CSDM classes, built on IntegrationHub ETL and RTE
(robust transform engine), always writing through IRE with a registered data source name. The
catalogue includes **Microsoft Intune, Microsoft SCCM/MECM, Jamf Pro, AWS, Azure, GCP,
Tanium, Qualys, Rapid7, Tenable, CrowdStrike, SolarWinds, Infoblox, VMware vCenter, Workspace ONE,
Kubernetes (via Visibility), Datadog, Dynatrace, Splunk, Cisco Meraki, Armis, Lansweeper**
(exact catalogue not re-verified **(unverified)**). The value is in **certified mapping, IRE-only
writes and versioned updates**, not in the transport.

### 2.7 SAM and HAM basics (background)

| Module | Essentials |
|---|---|
| **SAM Pro / SAM** | Software **discovery models** (normalised publisher, product, version, edition, via the "Content Library"); **software installations** from Discovery, SCCM, Intune or ACC; **entitlements** (licence purchases with metric: per device, per user, per core, PVU, subscription); **reconciliation** (installations vs entitlements, giving compliance positions); publisher packs (Microsoft, Oracle, IBM, Adobe, SAP); **SaaS licence management** (SSO/usage data from Entra, Okta, Zoom, M365) and **reclamation** (uninstall or harvest unused licences, with user confirmation); removal candidates; software EOL dates |
| **HAM Pro / HAM** | Asset (financial/contractual) separate from CI (operational), linked by model. **Model** and **model category** define what becomes asset and/or CI. Asset lifecycle (order → receive → stock → deploy → in use → retire → dispose), stockrooms, contracts and warranties, depreciation, disposal certificates, normalised hardware models with EOL/EOS dates, hardware refresh campaigns |
| Asset–CI sync | Asset and CI share model, serial, state; a state change on one updates the other via mapping rules |

### 2.8 Workflow Data Fabric, RaptorDB, zero-copy (2024–2025)

- **Workflow Data Fabric** (October 2024) is a layer that gives workflows and agents real-time access
  to enterprise data without copying it. It runs on **RaptorDB Pro**, a high-performance
  (Postgres-derived, background) database replacing MariaDB for large instances.
  [Press release 2024](https://www.businesswire.com/news/home/20241023742891/en)
- **Knowledge 2025:** **Workflow Data Network**, an ecosystem with **zero-copy connectors** to
  Redshift, Databricks, BigQuery, SQL Server, Oracle, Snowflake, Cloudera and Teradata, plus 100+
  integrations. ServiceNow also acquired **data.world** (data catalogue and governance).
  [BusinessWire 2025](https://secure.businesswire.com/news/home/20250507149031/en/ServiceNow-Enhances-Its-Workflow-Data-Fabric-With-New-Ecosystem-to-Power-AI-agents-and-Workflows-With-Real-Time-Intelligence),
  [Constellation](https://www.constellationr.com/insights/news/servicenow-launches-workflow-data-network-acquires-dataworld)
- The "70% integration cost reduction" claim is a vendor customer story **(unverified)**.
- **Takeaway:** federation (query in place) works for analytics and agent context. Operational
  CMDB data (identity, relations, lifecycle) is still *materialised* through IRE. Hubtask should
  keep the CMDB materialised and treat federation as LATER.

---

## 3. ServiceNow AI platform direction 2024–2026

### 3.1 Timeline

| When | What | Source |
|---|---|---|
| 2023–24 | **Now Assist** (generative skills such as summaries, resolution notes, code), **Now LLM** (own domain models), **Now Assist Skill Kit** (custom skills). Knowledge 2024 was about model choice | [Constellation K24](https://www.constellationr.com/insights/news/servicenow-knowledge-2024-model-choices-genai-everywhere-automation-process) |
| Sep 2024 (Xanadu) – Jan 2025 (Yokohama) | **AI Agents** and **AI Agent Orchestrator** (multi-agent coordination toward a goal), **AI Agent Studio** (no/low-code agent building: role, instructions, tools such as flows, scripts, search and subflows, with triggers), prebuilt agents for ITSM, CSM and HR | [The New Stack](https://thenewstack.io/servicenow-launches-a-control-tower-for-agents/) |
| May 2025 (Knowledge 2025) | "**ServiceNow AI Platform**" rebrand ("any AI, any agent, any model"); **AI Control Tower** (central governance of native and third-party agents, models and workflows: monitoring, GRC integration, lifecycle); **AI Agent Fabric** (agent-to-agent communication across vendors, with partners such as Google Cloud, Microsoft, IBM, Adobe, Zoom; supports **MCP and Google A2A**, background); Workflow Data Network; data.world | [AI Control Tower press release](https://s205.q4cdn.com/537566246/files/doc_news/ServiceNow-Launches-AI-Control-Tower-a-Centralized-Command-Center-to-Govern-Manage-Secure-and-Realize-Value-From-Any-AI-Agent-Model-a-GZ2RB.pdf), [CX Today: 11 announcements](https://cxtoday.com/crm/servicenow-knowledge-2025-announcements), [Reworked](https://www.reworked.co/the-wire/servicenow-unveils-the-new-servicenow-ai-platform-to-put-any-ai-any-agent-any-model-to-work-across-the-enterprise/) |
| Zurich (H2 2025) | Model providers in Now Assist Admin: **BYOK or OEM** integration with Now LLM Service, Azure OpenAI GPT-4.1, Google Gemini 2.5, **Anthropic Claude via AWS**; per-skill or per-instance model selection approved in AI Control Tower; **regulated markets get Now LLM only** | [Zurich Now Assist release notes](https://www.servicenow.com/docs/r/zurich/release-notes/now-assist-rn.html), [Manage AI models](https://www.servicenow.com/docs/r/SRy58xeijUUqttfwLgfrHQ/HthEBS43hTzSOLxuj_BTlA) |
| 15 Dec 2025 | **Moveworks** acquisition completed (announced March 2025, about $2.85B), adding the employee-facing conversational and agentic front door and enterprise search | [BusinessWire](https://www.businesswire.com/news/home/20251215128980/en/ServiceNow-completes-acquisition-of-Moveworks) |
| 9 Apr 2026 | **New pricing:** five legacy packages (Standard/Pro/**Pro Plus**/Enterprise/Enterprise Plus) replaced by **Foundation / Advanced / Prime**. Now Assist, Moveworks layer, Workflow Data Fabric and AI Control Tower bundled in every tier. AI usage metered in **assists** from a tenant-level pool, with overage. Fully autonomous agents reportedly Prime-only. Legacy end of sale 1 July 2026 | [ServiceNow community licensing guide](https://www.servicenow.com/community/upgrades-and-patching-forum/servicenow-ai-native-licensing-in-2026-a-practical-guide-to/m-p/3565858), [Redress (advisory)](https://redresscompliance.com/servicenow-now-assist-ai-pricing.html) |
| May 2026 (Knowledge 2026) | **Action Fabric:** platform usable "headless" by any external agent through a **generally available MCP server**, giving agents workflows, SLAs, Knowledge Graph and Context Engine; **Claude** is the launch partner. **AI Control Tower expanded:** Discover (30 new integrations, discovers devices *and AI agents*), Observe (Traceloop acquisition), Govern (AI risk assessment), Secure (Veza acquisition: identity and permissions), Measure (cost, ROI), with an Otto conversational UI in Q3 2026. **Otto** (unified AI experience: Now Assist plus Moveworks). **Autonomous Workforce / AI specialists** (L1 IT service desk GA; infrastructure monitoring, SRE, AIOps, asset lifecycle). **Autonomous Security & Risk** (governs agents, identities and assets; Armis). Microsoft Agent 365 integration | [Constellation K26](https://www.constellationr.com/insights/news/servicenow-knowledge-2026-ai-control-tower-action-fabric-autonomous-workforce-and), [Reworked K26](https://www.reworked.co/digital-workplace/servicenow-launches-action-fabric-major-overhaul-of-ai-control-tower/), [EMA](https://www.enterprisemanagement.com/product/servicenow-expands-ai-control-tower-into-an-enterprise-governance-and-execution-layer/), [ERP Today](https://erp.today/servicenow-ai-security-governance-knowledge-2026/) |

Note: whether A2A carried over from "AI Agent Fabric" (2025) into "Action Fabric" (2026) was not
confirmed **(unverified)**.

### 3.2 What a smaller sovereign competitor should copy

| ServiceNow capability | Essence to copy | Hubtask fit |
|---|---|---|
| AI Control Tower inventory | **Registry of every agent, model and tool** (native and external), each with an owner, purpose, risk class, allowed models and data scopes | Natural on top of workspace consent and BYO models |
| Control Tower governance | Approval workflow before an agent goes live; risk assessment (EU AI Act classification); policy per workspace | Uses existing audit plus a decision workflow |
| Observe / measure | Traces per run (prompt, tool calls, results, tokens, latency, cost), success and escalation rates, cost per outcome | Must respect rule 10: traces of user content are tenant data, not logs |
| Kill switch and quotas | Per agent, per team and per workspace budgets (tokens, money, actions/hour); instant suspend | MUST |
| Action Fabric (MCP server) | The platform's workflows as governed tools for *external* agents, so the same guardrails apply to external and native agents | Hubtask already has an MCP server, so this is a head start |
| Model choice, regulated-market restriction | BYO model per workspace, with an allow-list; regulated tenants restricted to EU-hosted or self-hosted models | Already a Hubtask principle |
| AI specialists (L1 desk, asset lifecycle) | Narrow, measurable agents with defined handoff to humans | SHOULD, after the governance layer |
| Pricing in assists | Metering is a product feature: customers want predictable budgets | Hubtask can expose metering transparently without reselling AI |
| "Bad CMDB makes agents expensive" | CMDB quality directly drives agent accuracy and cost | Build CMDB Health first |

---

## 4. Inventory, discovery and monitoring tools to integrate

Pattern legend: **PULL** (scheduled connector calls the source API), **PUSH** (source sends a
webhook or the agent submits), **AGENT** (Hubtask-native or source agent), **FILE** (import
CSV/JSON/XML). The API style is from vendor documentation and background knowledge. Where only
the summary was checked, it is marked.

### 4.1 Open-source and EU inventory/CMDB tools

| Tool | Origin / licence | Data offered | API | Recommended pattern |
|---|---|---|---|---|
| **GLPI** (11.x) + **GLPI Agent** (FusionInventory fork) | Teclib (FR), GPLv3 | Hardware (CPU, RAM, disks, BIOS, serial), OS, software with versions, network ports and VLANs, printers, monitors, SNMP network discovery, remote inventory via ESX/SSH, antivirus status, users | Legacy REST API plus a new **High-level API (v2)** with **OAuth2** (GLPI 11 introduced OAuth clients; v2 docs filed under GLPI 12) — [RESTful API v2](https://help.glpi-project.org/documentation/12-en/setup/general/api/restful-api-v2), [OAuth clients](https://help.glpi-project.org/documentation/12-en/setup/oauth-clients). Native inventory endpoint takes the agent's JSON | (a) **PULL** from the GLPI API for customers who keep GLPI; (b) **PUSH/AGENT**: Hubtask can expose a GLPI-Agent-compatible inventory endpoint (JSON format documented in the open source agent), making the agent Hubtask's free discovery agent. High value |
| **OCS Inventory NG** | FR, GPLv2 | Hardware, software, network scan (IPDiscover), SNMP | REST API (read), SOAP legacy | **PULL**; lower priority (declining, often paired with GLPI) |
| **Snipe-IT** | Grokability, AGPLv3 | **HAM**: assets with tag, serial, model, status labels, check-in/out to users and locations, licences (seats), accessories, consumables, components, depreciation, maintenance, audits | REST JSON API with personal access token; webhooks for Slack-style notifications only **(background)** | **PULL** (assets, licences, users) incrementally via `updated_at`; possibly bidirectional for checkout state. Good HAM source for SMEs |
| **NetBox** (NetBox Labs) | Apache-2.0 | DCIM/IPAM source of truth: sites, racks, devices, interfaces, cables, IP prefixes and addresses, VLANs, VRFs, circuits, virtualization (clusters, VMs), tenants, contacts, custom fields, tags | REST (`/api/`) plus **GraphQL**; token auth; **webhooks / event rules** on object change; plugins | **PULL** for the initial load plus **PUSH** (event-rule webhook to a Hubtask intake) for changes. NetBox should stay authoritative for network and DC attributes (source precedence) |
| **Nautobot** | Network to Code, Apache-2.0 | NetBox fork: same domain plus Jobs, Git data sources, GraphQL | REST plus GraphQL, webhooks, Job hooks | As NetBox; one adapter with a dialect switch |
| **i-doit** (pro/open) | synetics (DE); open core AGPL, pro commercial | Full CMDB: 100+ object types, categories, relations, cabling, IT-Grundschutz/ISMS add-ons (VIVA/ISMS), document generation; imports from OCS, JDisc, h-inventory, LDAP | **JSON-RPC API** (API add-on; pro/commercial branch — [RT readme](https://erised.las.iastate.edu/CPAN/modules/by-module/RT/RT-Extension-ReferenceIDoitObjects-0.92.readme)); used by Zammad and Request Tracker integrations — [Zammad i-doit](https://admin-docs.zammad.org/en/latest/system/integrations/i-doit.html) | **PULL** objects plus relations; ticket↔object linking (as in Zammad). Important for the German Mittelstand and public sector; many will *keep* i-doit as CMDB, so Hubtask must interoperate rather than replace on day one |
| **JDisc Discovery** | DE, commercial | Agentless discovery of network, servers, software, dependencies | GraphQL API **(background)** | **PULL**; LATER |
| **Lansweeper** | BE, commercial SaaS | Agentless and agent discovery: hardware, software, users, OT/IoT, network, vulnerabilities (Risk Insights), cloud | **GraphQL Data API**, OAuth2 or personal token; read-only; webhooks limited; about 150 req/min reported — [Xurrent connector](https://www.xurrent.com/help/lansweeper-connector), [community](https://community.lansweeper.com/t5/api-integrations/lansweeper-api/ta-p/67091) **(limits unverified)** | **PULL**, paginated, incremental by `lastChanged`; respect rate limits |

### 4.2 Endpoint and identity management

| Tool | Data | API | Pattern |
|---|---|---|---|
| **Microsoft Intune** (Graph) | Managed devices (`/deviceManagement/managedDevices`): serial, model, manufacturer, OS, compliance state, primary user, last sync, encryption; detected apps; autopilot devices | Microsoft Graph REST, OAuth2 client credentials (app registration, `DeviceManagementManagedDevices.Read.All`), `$filter`/`$select`, paging, throttling (429 with Retry-After); delta query not available for all Intune entities **(background)** | **PULL** scheduled (hourly/daily). Requires the GuardedClient allow-list for `graph.microsoft.com` and `login.microsoftonline.com` |
| **Microsoft Entra ID** | Users, groups, devices, licences assigned (SaaS SAM), sign-in activity (licence reclamation) | Graph; **delta queries** for users, groups and devices | **PULL** with delta tokens; also SCIM inbound for users |
| **Jamf Pro** | macOS/iOS devices, hardware, apps, users, config profiles, security (FileVault) | Jamf Pro API (REST, OAuth client credentials) plus Classic API; webhooks (ComputerAdded, ComputerInventoryCompleted, …) | **PULL** plus optional **PUSH** webhooks |
| **SCCM / MECM** | Windows hardware and software inventory, collections, updates | AdminService REST (OData) on SMS Provider; otherwise SQL views (`v_R_System`, `v_GS_*`) | On-prem: **relay agent** in the customer network (like a MID server) running PULL, or **FILE** export. Cloud-hosted Hubtask cannot reach it directly |
| **Ansible / AWX / AAP** | `ansible_facts` (hardware, OS, network, packages), host inventories, job results | AWX REST API (`/api/v2/hosts/{id}/ansible_facts`), webhooks on job completion; fact cache (Redis/JSON files) | **PULL** facts, or **PUSH** via a callback plugin or task posting facts to Hubtask's intake. Also an *outbound* integration: changes trigger AWX job templates |

### 4.3 Monitoring and observability

| Tool | Data | API | Pattern |
|---|---|---|---|
| **Zabbix** (LV, AGPLv3 since 7.0) | Hosts, host groups, templates, **host inventory fields**, items, triggers, **problems/events**, maps | JSON-RPC API (`host.get`, `problem.get`), API tokens; **webhook media types** for alerts | **PULL** hosts (CI candidates, low precedence); **PUSH** problems via webhook, creating incidents bound to CIs (event→incident with dedup) |
| **Checkmk** (DE, GPL/commercial) | Hosts, services, HW/SW inventory (agent-based), BI aggregations (service models!) | REST API (OpenAPI) incl. HW/SW inventory endpoints; notification plugins / webhooks | **PULL** hosts plus inventory; **PUSH** notifications to incidents. Popular in DACH |
| **Icinga 2** (DE, GPL) | Hosts, services, states, downtimes | REST API (`/v1/objects`), event streams (`/v1/events`) | **PULL** objects; event stream or notification command for incidents |
| **Prometheus / Alertmanager** | Targets (service discovery), metrics, alerts | HTTP API; **Alertmanager webhook receiver** (JSON) | **PUSH** alerts via Alertmanager webhook to Hubtask (labels mapped to CI identifiers); targets as weak CI evidence |
| **Wazuh** (Apache/GPL) | Agents, **syscollector** (hardware, OS, packages, ports, processes, network), **vulnerability detection** (CVE per package), SCA (CIS benchmarks) | REST API (JWT), Indexer (OpenSearch) for alerts and vulns; integrations / webhooks | **PULL** agents, inventory and vulns (vuln findings linked to CI); **PUSH** high-severity alerts as security incidents |
| **Greenbone / OpenVAS** (DE) | Hosts, OS detection, open ports, **vulnerabilities (NVT, CVE, CVSS)**, reports | GMP (XML over TLS/socket, `python-gvm`); no REST | **PULL** reports via a relay agent (GMP is typically not internet-exposed); map host IP or hostname to CI through IRE lookup rules |

### 4.4 Software composition

| Tool | Data | API | Pattern |
|---|---|---|---|
| **OWASP Dependency-Track** | Projects, components (from SBOM), licences, **vulnerabilities**, policy violations, VEX | REST API (API key); **webhooks/notifications** (NEW_VULNERABILITY, BOM_PROCESSED) | **PULL** projects/components, linking projects to Application Service CIs; **PUSH** notifications as vulnerability items |
| **SBOM files** (CycloneDX 1.6, SPDX 2.3/3.0) | Component list, PURLs, hashes, licences, dependencies | Files (JSON/XML) | **FILE** import attached to an application CI (CSDM 5 SBOM tracking); store PURL as software identity |

### 4.5 Cloud providers

| Provider | Inventory source | API | Pattern |
|---|---|---|---|
| **AWS** | **AWS Config** (resource configuration items and relationships, history), Resource Explorer, Organizations | SDK/REST, SigV4; Config advanced queries (SQL-like); EventBridge for changes | **PULL** via assumed role (read-only), optional **PUSH** via EventBridge → HTTPS endpoint |
| **Azure** | **Azure Resource Graph** (KQL over all resources), Resource Changes | REST (`Microsoft.ResourceGraph/resources`), Entra service principal | **PULL** KQL queries per subscription/management group; Event Grid for push |
| **GCP** | **Cloud Asset Inventory** (resources, IAM policies, relationships, feeds) | REST/gRPC; **asset feeds** to Pub/Sub | **PULL** `searchAllResources`/export; feeds LATER |
| **Hetzner Cloud** (DE) | Servers, volumes, networks, load balancers, firewalls, labels; Robot API for dedicated servers | REST, bearer token per project; rate-limited (about 3,600 req/h per project, background) | **PULL**. EU sovereign, MUST for the target segment |
| **IONOS Cloud** (DE) | Datacenters (VDC), servers, volumes, LANs, NICs, K8s, DBaaS | Cloud API v6 REST, token | **PULL** |
| **OVHcloud** (FR) | Public Cloud (OpenStack), dedicated servers, vRack, IPs | OVH API (signed requests: app key/secret plus consumer key) plus OpenStack APIs | **PULL** |
| **STACKIT** (DE, Schwarz group) | Projects, servers, volumes, networks, SKE (K8s), DBaaS | REST APIs, service account keys (JWT) | **PULL** |
| **OpenStack** (generic, many EU clouds such as OTC/Open Telekom Cloud, Infomaniak, Cleura) | Nova, Cinder, Neutron, Keystone projects | REST (Keystone token) | **PULL**; one adapter covers several EU clouds |
| **Kubernetes** | Clusters, namespaces, workloads, pods, services, ingress, images | K8s API, watch | **PULL/watch** via an in-cluster agent; LATER |

### 4.6 Integration priority

| Priority | Connectors |
|---|---|
| **MUST (first wave)** | Generic **CSV/JSON file import** with mapping and dry run; **GLPI** (pull plus a GLPI-Agent-compatible intake); **NetBox/Nautobot**; **Microsoft Intune + Entra ID**; **Hetzner** plus one generic **OpenStack**; **Zabbix/Checkmk/Prometheus Alertmanager** inbound alert webhooks to incidents |
| **SHOULD** | **i-doit** (pull plus deep link); **Snipe-IT**; **Jamf**; **AWS Config / Azure Resource Graph / GCP CAI**; **Wazuh**; **Dependency-Track / CycloneDX**; **Ansible/AWX facts**; IONOS, STACKIT, OVH |
| **LATER** | SCCM (needs a relay agent), Greenbone (GMP via relay), Lansweeper, JDisc, OCS, Icinga, Kubernetes watch, Tanium, Qualys, Tenable, CrowdStrike |

---

## 5. Standards and data models

| Standard | What matters for the data model | Source |
|---|---|---|
| **ITIL 4: Service Configuration Management** | CI = any component that needs managing to deliver a service. Maintain accurate info on CIs *and their relationships*; configuration records, CMS, baselines; verification and audit. Value comes from use (change impact, incident diagnosis) | AXELOS ITIL 4 practice guides **(background)** |
| **ITIL 4: IT Asset Management** | Asset = financially valuable component; full lifecycle (plan → acquire → deploy → operate → retire → dispose); asset register; licence compliance; distinct from but linked to CIs | **(background)** |
| **ISO/IEC 19770** | -1: ITAM management system (processes, tiers); **-2: SWID tags** (software identification XML: tagId, name, version, entity/regid; NIST and the US government used them); -3: **entitlement schema (ENT)**; -4: resource utilisation (RUM); -5: overview and vocabulary. In practice SWID is less used than package managers and **PURL/CPE**; store SWID tagId, CPE and PURL as alternative software identifiers | ISO catalogue **(background)** |
| **DMTF CIM** | Object-oriented model (CIM_ManagedElement → CIM_LogicalDevice, CIM_System, CIM_ComputerSystem; associations as first-class classes). Basis for WBEM/WMI. Lesson: **class inheritance plus associations as typed classes with roles**; too heavy to adopt literally | dmtf.org **(background)** |
| **CSDM 5** | Prescriptive service model (see §2.1). The best "starter schema" to mirror conceptually, using Hubtask's own names, not ServiceNow table names (rule 17: no other product's name in the implementation) | §2.1 sources |
| **BSI IT-Grundschutz (BSI-Standard 200-2)** | **Strukturanalyse** (structure analysis): record business processes, applications, IT systems, rooms/buildings, networks and communication links and their dependencies; **group** similar objects (§8.1); network plan. **Schutzbedarfsfeststellung**: protection need (normal/high/very high) per *confidentiality, integrity, availability*, set on processes and **inherited** along dependencies via the maximum principle, cumulation and distribution effects. **Modellierung**: assign Kompendium **Bausteine** (modules such as SYS.1.1 General server, APP.3.2 Web server, INF.2 Data centre) to target objects, then IT-Grundschutz-Check (requirement fulfilment), risk analysis (200-3). Tools such as i-doit, verinice (open source, SerNet/DE), HiScout and GSTOOL successors implement this | [BSI 200-2 / NIS2 asset wiki](https://nisd2.eu/en/wiki/implementation/how-to-build-nis2-asset-inventory); BSI site **(background)** |
| **ISO/IEC 27001:2022 A.5.9** | "Inventory of information and other associated assets, including owners, shall be developed and maintained." Also A.5.10 (acceptable use), A.5.11 (return of assets), A.5.12 (classification), A.7.14 (secure disposal), A.8.1 (user endpoint devices) | ISO 27001:2022 Annex A **(background)** |
| **NIS2** (Directive (EU) 2022/2555, Art. 21(2)) plus **Implementing Regulation (EU) 2024/2690** | Art. 21(2) lists asset management among risk-management measures. CIR §12 (for digital-infrastructure and ICT-service entities in scope): classification, handling, removable media, **inventory (§12.4)** with a unique identifier, description, owner, protection level and location; **review at least annually and after significant change (§12.5)**; return of assets on leaving. German implementation (NIS2UmsuCG, BSI as authority) points to IT-Grundschutz. Article letter (i) vs (j) differs between sources **(verify)** | [CIR 2024/2690 wiki](https://nisd2.eu/en/wiki/fundamentals/cir-2024-2690), [NIS2 asset management](https://nisd2.eu/en/wiki/implementation/nis2-asset-management) (secondary source) |
| **EU AI Act** (Reg. (EU) 2024/1689) | AI system inventory, risk classification, human oversight (Art. 14), logging (Art. 12), transparency. Relevant to the agent registry: deployers of high-risk systems must keep logs and ensure oversight | **(background)** |
| **CycloneDX / SPDX, PURL, CPE** | Software identity and SBOM exchange; CycloneDX also models services, data flows and ML models (ML-BOM) | **(background)** |

**Consequence:** Hubtask's CMDB needs *owner*, *location*, *protection need (C/I/A)* with
inheritance along dependency relations, *review date / certification* and *grouping (quantity)* as
first-class fields. That covers ISO 27001 A.5.9, NIS2 CIR §12 and BSI 200-2 together, which is a
real differentiator against JSM and against ServiceNow, where it needs the IRM/GRC modules.

---

## 6. Recommendations for Hubtask

Priorities: **MUST** (needed for a credible first ITSM/CMDB release), **SHOULD** (the following
release), **LATER**. Every point must fit the existing rules: hexagonal core, tenant transaction
wrapper (rule 3), GuardedClient (rule 6), no user content in logs (rule 10), message codes only
(rule 8), contract first (rule 11), forward-only migrations (rule 12).

### 6.1 Data model

| # | Item | Prio | Notes |
|---|---|---|---|
| D1 | **CI class registry**: classes with single inheritance, a shipped base hierarchy (Hardware › Computer › Server/Workstation/Laptop; Network device; Virtual machine; Cloud resource; Container cluster; Database instance; Application/Software instance; Application service; Technical service and offering; Business service and offering; Location/Room/Building; Contract; Business process; Information object), extendable per workspace | MUST | Store attributes as typed definitions per class with a JSONB value column plus generated/indexed columns for identifiers. A table per class does not suit multi-tenant extensibility |
| D2 | **Typed relation catalogue**: relation types as directional pairs (depends on/used by, runs on/runs, hosted on/hosts, contains/contained in, connects to, member of, provides/provided by, owned by, located in) with an **allowed class pair matrix** and an **impact direction flag** | MUST | Relations are first-class records with a source, a first and last seen time, and their own lifecycle |
| D3 | **Service model like CSDM, in Hubtask's own names**: Business service › Offering (SLA, support group, approval group) › Application service › infrastructure CIs; Technical service and offering for IT-internal services | MUST | Incident/change routing by offering; the bridge to the five-level task model (a ticket is a Task in a service-desk Collection, background assumption) |
| D4 | **Lifecycle** as stage plus status, the same for every class (Planned, Ordered, In stock, In use, Maintenance, Retired, Disposed); separate **asset** (financial) and **CI** (operational) records linked 1:1 by model | MUST (CI lifecycle), SHOULD (asset) | |
| D5 | **Compliance fields first-class**: owner (person or group), location, **protection need C/I/A** (normal/high/very high) with **inheritance** along impact relations (maximum principle, manual override with justification), review due date, quantity/grouping | MUST | NIS2 CIR §12.4, ISO A.5.9, BSI 200-2. Differentiator |
| D6 | **Software identity**: normalised product (publisher, product, version, edition) with alternative keys (PURL, CPE, SWID tagId); installations as relations CI→software | SHOULD | |
| D7 | **Attribute provenance**: per attribute, the last writer (source, run, time). Needed for reconciliation and audit | MUST | Scale with care: store provenance per attribute group, or as a compact JSONB map |
| D8 | BSI **module assignment** (Kompendium module ↔ target object) and requirement status | LATER (or a partner tool such as verinice via export) | Licence of the Kompendium content: check BSI terms **(unverified)** |

### 6.2 Reconciliation engine (a Hubtask "IRE")

| # | Item | Prio |
|---|---|---|
| R1 | **One write path**: every source (UI, API, MCP agent, connector, file import) writes CIs through the reconciliation service in `core/application`. Adapters never write CI tables directly; authorisation is checked there (rule 2) | MUST |
| R2 | **Source registry** per workspace: source id, kind, trust rank, allowed classes, create-vs-update-only | MUST |
| R3 | **Identification rules per class**, inherited: ordered identifier sets (for example serial plus manufacturer → MAC address → (hostname, domain) → cloud resource id), with lookups into identifier side tables (serials, MACs, IPs, cloud IDs, source native keys) | MUST |
| R4 | **Source native key first**: (source, external id) mapping makes reruns idempotent | MUST |
| R5 | **Per-attribute-group precedence** with a **staleness takeover** (if the authoritative source has not reported for N days, the next source may write) | MUST (static precedence), SHOULD (staleness takeover) |
| R6 | **Insufficient-data hold**: payloads without a usable identifier go to a review queue, never get inserted | MUST |
| R7 | **Duplicate detection** with merge tasks (the survivor keeps its id and takes the relations and history); never an automatic hard delete (irrecoverable deletion is an owner decision) | MUST |
| R8 | **Explainability**: each reconciliation decision records which rule matched and why an attribute was or was not taken. It is stored as a tenant record, not a log line with values (rule 10 spirit: CI names may be user content) | MUST |
| R9 | **Dry run**: run a payload or a connector batch in simulate mode and get a diff (creates/updates/conflicts/holds) | MUST |
| R10 | Relationship reconciliation (relations also carry source and last-seen; stale relations expire) | SHOULD |

### 6.3 Connector framework

| # | Item | Prio |
|---|---|---|
| C1 | **Connector port** in `core/port` (list since cursor → payloads), adapters in `infrastructure/` per source; payloads normalised to the reconciliation payload | MUST |
| C2 | **Pull scheduling** through the existing worker (SafeGo, rule 5), with per-connector cron, **incremental cursors** (updated_at, delta tokens), full resync on demand | MUST |
| C3 | **Push intake**: signed webhooks (HMAC per connector), idempotency keys, replay protection, queued processing | MUST |
| C4 | **File import** (CSV/JSON, CycloneDX/SPDX) with a saved **mapping** (column → class/attribute, transforms, defaults) and dry run | MUST |
| C5 | **Credentials as sealed secrets** (encrypted at rest with a workspace key, write-only in API/UI, rotation, never logged); OAuth2 client credentials for Graph/Jamf/GLPI | MUST |
| C6 | **Outbound via GuardedClient** (rule 6) with a per-connector **host allow-list**, timeouts (rule 7), **rate limits and backoff** honouring 429/Retry-After, and circuit breaker | MUST |
| C7 | **Relay agent** (small Go binary run by the customer, outbound-only to Hubtask) for on-prem sources (SCCM, Greenbone, internal GLPI, network scans) | SHOULD |
| C8 | **Run history**: per run counts (fetched, created, updated, unchanged, held, conflicts, errors as message codes), duration, cursor; alert on failure or zero results | MUST |
| C9 | **Mapping packs** versioned per connector (certified mapping like SGC), shipped and upgradable | SHOULD |
| C10 | Event→incident: monitoring alerts open, update or resolve incidents with dedup keys and CI binding through identifier lookup | MUST |
| C11 | Outbound actions (AWX job, Intune remote action) as **governed tools**, only through approval | LATER |

### 6.4 Health metrics

| # | Metric | Prio |
|---|---|---|
| H1 | **Completeness**: required and recommended attributes per class (configurable) | MUST |
| H2 | **Staleness**: not confirmed by any source for N days (default 30/60, per class) | MUST |
| H3 | **Duplicates** (open merge tasks) and **orphans** (CI without a required relation, such as an application service without a host, or a CI without an owner) | MUST |
| H4 | **Compliance / certification campaigns**: owners attest CIs on a schedule (meets the NIS2 "review at least annually" duty); results as a score plus audit trail | SHOULD (MUST for the NIS2 pitch) |
| H5 | **Source conflict rate** (attributes flapping between sources) | SHOULD |
| H6 | **Lifecycle policies** (retire stale, archive retired after X, delete only after archive, with approval and restore window) | SHOULD; deletion parts need an owner decision |
| H7 | Health per service (a business service's score aggregates its CIs) | SHOULD |

### 6.5 Impact analysis and service maps

| # | Item | Prio |
|---|---|---|
| I1 | **Graph queries** (recursive CTE within the tenant transaction; depth-limited) for upstream/downstream impact along relations flagged for impact | MUST |
| I2 | **Impact on incident/change**: show affected offerings and services, affected users/customers, open incidents on related CIs; change collision detection (two changes on one CI or dependent CIs in the same window) | MUST (impact), SHOULD (collision) |
| I3 | **Service map view** in the webapp (layered graph: offering → app service → infra) | SHOULD |
| I4 | Service definition by **query/tag** (dynamic CI groups: "all VMs with label app=crm") | SHOULD |
| I5 | Traffic-based or pattern-based automatic service mapping | LATER |

### 6.6 SAM and HAM essentials

| # | Item | Prio |
|---|---|---|
| A1 | Asset register (model, serial, asset tag, cost, purchase date, supplier, warranty/contract, stockroom, assigned user/location, state) linked to its CI | SHOULD |
| A2 | Asset lifecycle with request→order→receive→deploy→retire→dispose as service-desk workflows; disposal certificate (ISO A.7.14) | SHOULD |
| A3 | Software normalisation (minimum: publisher/product/version from installations, with a manual alias table) | SHOULD |
| A4 | Entitlements (per device / per user / subscription seats) and a **compliance position** (installs or assignments vs entitlements) for the top publishers by hand | LATER |
| A5 | SaaS licence usage from Entra (assigned licences, last sign-in) with **reclamation** workflow (ask user → remove) | LATER |
| A6 | Hardware EOL/EOS dates per model; refresh reporting | LATER |
| A7 | Vulnerabilities (Wazuh, Greenbone, Dependency-Track) linked to CI and software; SLA by CVSS and protection need | SHOULD |

### 6.7 AI-agent governance layer

| # | Item | Prio | Notes |
|---|---|---|---|
| G1 | **Agent registry** per workspace: agent identity (a non-human principal with its own credentials), owner, purpose, model(s) allowed (from the workspace's BYO-model consent), tools/MCP scopes, data scopes (which Hubs/Collections/CI classes), risk class (EU AI Act), status (draft → approved → active → suspended → retired) | MUST | External agents on Hubtask's MCP server register too, with the same model as the Action Fabric: governed even when built elsewhere |
| G2 | **Permissions via the normal application-layer authorisation** (rule 2): an agent never has more rights than its owner/delegator; scopes are intersected | MUST |
| G3 | **Human-in-the-loop policies** per tool/action class: read = auto; reversible write = auto with notification or approval by threshold; irreversible or high-impact (close incident, approve change, retire CI, send external mail) = **approval required**; approvals as tasks with diff preview | MUST |
| G4 | **Audit**: every agent action and every approval goes into the hash-chained audit trail (actor = agent, on behalf of = person, tool, target ids, decision, without user content per rule 10) | MUST | Existing strength |
| G5 | **Run traces** (prompt and tool I/O) as *tenant data* with retention and access control, not as logs/metrics; opt-in per workspace; viewable by the agent owner and auditors | SHOULD |
| G6 | **Budgets and kill switch**: per agent, per workspace caps on tokens, cost, actions per hour and concurrent runs; immediate suspend; alerts at thresholds | MUST |
| G7 | **Evaluation**: test sets per agent (golden tickets → expected classification/routing/resolution), run before activation and after model change; track accuracy, escalation and override rates in production | SHOULD |
| G8 | **Guardrails**: tool allow-list, output schema validation, prompt-injection defences for content from tickets/emails (treat as data), PII redaction before external models, data residency check on the model endpoint (EU only for regulated workspaces) | MUST (allow-list, residency), SHOULD (others) |
| G9 | **Model registry**: models per workspace with provider, hosting region, data-retention terms, consent record; model change requires re-approval of dependent agents | MUST (ties into existing BYO consent) |
| G10 | **Metering dashboard**: usage per agent, skill and team; cost per resolved ticket; visible to workspace admins | SHOULD |
| G11 | First agents: **ticket triage** (classify, route by offering, link CI), **CI suggestion** for incidents, **duplicate CI review assistant**, **knowledge draft from resolved tickets**. Each narrow, measurable and approval-gated | SHOULD |
| G12 | Agent-to-agent (A2A) protocol, multi-agent orchestration, autonomous L1 resolution | LATER |

### 6.8 Proposed sequence (milestone-sized)

1. **CMDB core:** class registry, relation catalogue, service model, lifecycle, compliance fields
   (D1–D5, D7) plus the reconciliation engine (R1–R4, R6–R9) plus file import with dry run (C4).
2. **Connector framework plus first wave:** C1–C3, C5, C6, C8, C10; GLPI, NetBox, Intune/Entra,
   Hetzner/OpenStack, alert webhooks from Zabbix, Checkmk and Alertmanager.
3. **Health and impact:** H1–H3, I1–I2; certification campaigns H4 for the NIS2 pitch.
4. **Agent governance:** G1–G4, G6, G8 (allow-list/residency), G9 on the existing MCP server; then
   the first narrow agents (G11) with evaluation (G7).
5. **ITAM:** A1–A3, A7, then SAM entitlements and SaaS reclamation.

### 6.9 Owner decisions likely needed (per AGENTS.md "What you do not decide yourself")

- Any new third-party dependency (for example a graph-layout library for service maps, OAuth
  libraries, a GMP client).
- Lifecycle policies that **delete** CIs or assets (irrecoverable deletion).
- Whether to ship BSI Kompendium content (licence) and the EU AI Act risk classification as
  product features (vision scope).
- Retention of agent run traces (retention safeguards).

---

## 7. Source list

**Atlassian**
- https://support.atlassian.com/assets/docs/increase-the-usage-limit-for-assets/
- https://www.atlassian.com/licensing/assets
- https://www.atlassian.com/blog/announcements/jsm-cloud-pricing-packaging-update
- https://community.atlassian.com/forums/discussion/3175576/assets-is-now-part-of-service-collection-standard
- https://www.atlassian.com/collections/service/pricing
- https://www.atlassian.com/software/jira/service-management/features/assets-data-manager
- https://support.atlassian.com/jira-service-management-cloud/docs/learn-about-adapters
- https://support.atlassian.com/jira-service-management-cloud/docs/data-manager-jamf-adapter/
- https://community.developer.atlassian.com/t/changes-to-assets-object-aql-and-object-navlist-aql-endpoints/79919
- https://support.atlassian.com/assets/docs/assets-custom-field-scalability/
- https://support.atlassian.com/jira/kb/assets-object-search-report-shows-sorry-but-there-is-too-much-data-to-be-displayed-please-reduce-the-scope-by-refining-your-parameters/
- https://jira.atlassian.com/browse/JSDSERVER-15096
- https://confluence.atlassian.com/assetapps/export-agent-scan-results-to-cloud-1520369768.html
- https://support.atlassian.com/jira/kb/how-discovery-agents-can-export-to-cloud-instance/
- https://support.atlassian.com/jira-service-management-cloud/docs/what-is-asset-discovery/
- https://assetmanagementforjira.com/blog/jira-assets-limitations (competitor)
- https://blog.isostech.com/is-jira-automation-for-assets-limited

**ServiceNow**
- https://www.servicenow.com/community/common-service-data-model/csdm-5-finally-get-the-csdm-5-white-paper-here/ta-p/3254967
- https://www.servicenow.com/community/common-service-data-model-forum/top-features-of-csdm-5-what-you-need-to-know/td-p/3294791
- https://www.servicenow.com/docs/r/zS6yb8e~gR8BPjeviYdV9g/YO9~nS73A2CUXGgC7Mjz6g
- https://www.servicenow.com/docs/r/zS6yb8e~gR8BPjeviYdV9g/Q5J8vrX3tTDtGnnBzK3D5g
- https://www.servicenow.com/docs/r/GCPjp~ldPTNRN99oH1DYFg/eUXumCBbcZFJ~GfPH0xxsA
- https://www.servicenow.com/community/servicenow-ai-platform-forum/query-on-data-precedence-rules-and-reconciliation-rules/td-p/1158663
- https://docs.flexera.com/ServiceNowFlexeraOneApp/SNapp/v1.6/Content/helplibrary/integrationHUB.htm
- https://www.businesswire.com/news/home/20241023742891/en
- https://secure.businesswire.com/news/home/20250507149031/en/ServiceNow-Enhances-Its-Workflow-Data-Fabric-With-New-Ecosystem-to-Power-AI-agents-and-Workflows-With-Real-Time-Intelligence
- https://www.constellationr.com/insights/news/servicenow-launches-workflow-data-network-acquires-dataworld
- https://s205.q4cdn.com/537566246/files/doc_news/ServiceNow-Launches-AI-Control-Tower-a-Centralized-Command-Center-to-Govern-Manage-Secure-and-Realize-Value-From-Any-AI-Agent-Model-a-GZ2RB.pdf
- https://thenewstack.io/servicenow-launches-a-control-tower-for-agents/
- https://cxtoday.com/crm/servicenow-knowledge-2025-announcements
- https://www.reworked.co/the-wire/servicenow-unveils-the-new-servicenow-ai-platform-to-put-any-ai-any-agent-any-model-to-work-across-the-enterprise/
- https://www.servicenow.com/docs/r/zurich/release-notes/now-assist-rn.html
- https://www.servicenow.com/docs/r/SRy58xeijUUqttfwLgfrHQ/HthEBS43hTzSOLxuj_BTlA
- https://www.constellationr.com/insights/news/servicenow-knowledge-2024-model-choices-genai-everywhere-automation-process
- https://www.businesswire.com/news/home/20251215128980/en/ServiceNow-completes-acquisition-of-Moveworks
- https://www.servicenow.com/community/upgrades-and-patching-forum/servicenow-ai-native-licensing-in-2026-a-practical-guide-to/m-p/3565858
- https://redresscompliance.com/servicenow-now-assist-ai-pricing.html (advisory)
- https://www.constellationr.com/insights/news/servicenow-knowledge-2026-ai-control-tower-action-fabric-autonomous-workforce-and
- https://www.reworked.co/digital-workplace/servicenow-launches-action-fabric-major-overhaul-of-ai-control-tower/
- https://www.enterprisemanagement.com/product/servicenow-expands-ai-control-tower-into-an-enterprise-governance-and-execution-layer/
- https://erp.today/servicenow-ai-security-governance-knowledge-2026/

**Tools**
- https://help.glpi-project.org/documentation/12-en/setup/general/api/restful-api-v2
- https://help.glpi-project.org/documentation/12-en/setup/oauth-clients
- https://glpi-project.org/discover-native-glpi-inventory
- https://admin-docs.zammad.org/en/latest/system/integrations/i-doit.html
- https://erised.las.iastate.edu/CPAN/modules/by-module/RT/RT-Extension-ReferenceIDoitObjects-0.92.readme
- https://www.xurrent.com/help/lansweeper-connector
- https://community.lansweeper.com/t5/api-integrations/lansweeper-api/ta-p/67091

**Standards**
- https://nisd2.eu/en/wiki/fundamentals/cir-2024-2690
- https://nisd2.eu/en/wiki/implementation/nis2-asset-management
- https://nisd2.eu/en/wiki/implementation/how-to-build-nis2-asset-inventory
