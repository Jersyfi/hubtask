# Hubtask — regulatory, standards and architecture foundation 2026–2028

Research date: 2026-10-28. Web research plus background knowledge. Anything marked **(unverified)** was not confirmed against a primary source during this research; dates in the regulatory tables were checked where a source URL is given. This is not legal advice; the CRA role question and the AI Act classification in particular should get a lawyer's view before a commercial offering launches.

**Deployment shorthand used throughout**

| Code | Meaning | Who is "the party" legally |
|---|---|---|
| **OSS** | The Apache-2.0 project as published (source, images, Helm chart) | The owner/project — manufacturer or steward under the CRA |
| **SH** | Self-hosted by an individual or company for itself | The operator is controller (GDPR) and possibly a NIS2 entity; the project is a supplier |
| **SaaS** | The owner or a provider runs Hubtask for others (B2C/B2B) | Provider is processor (GDPR Art. 28), cloud/managed-service provider (NIS2, Data Act), ICT third party (DORA) |
| **MSP** | Managed service provider running instances for customers | As SaaS, plus NIS2 "managed service provider" category in its own right |

---

## Part 1 — EU and German law

### 1.1 Timeline at a glance

| Date | What becomes applicable | Hubtask relevance |
|---|---|---|
| 2025-02-02 | AI Act: prohibited practices, AI literacy (Art. 4) | Deployers and providers alike; AI literacy is an organisational duty |
| 2025-06-28 | EAA / BFSG applicable | B2C sales flows and consumer-facing e-commerce; public-sector buyers require EN 301 549 already via procurement |
| 2025-08-02 | AI Act GPAI obligations (model providers) | Not Hubtask (it uses models, does not provide GPAI) |
| 2025-09-12 | Data Act applicable (incl. Ch. VI switching, Art. 23–31) | SaaS providers of Hubtask are "data processing services" |
| 2025-12-06 | German NIS2UmsuCG (new BSIG) in force, no transition | Customers in scope; MSPs/cloud providers in scope themselves |
| 2026-03-06 | BSI registration deadline for NIS2 entities | SaaS/MSP operators that meet the size thresholds |
| 2026-08-02 | AI Act Art. 50 transparency (chatbots/AI interaction disclosure, deepfake labelling) | Applies to Hubtask's AI chat/agent features |
| 2026-09-11 | **CRA Art. 14 reporting**: actively exploited vulnerabilities and severe incidents → ENISA single reporting platform (24 h / 72 h / 14 days) | Applies to every product already on the market, including existing Hubtask releases |
| 2026-12-02 | AI Act Art. 50(2) machine-readable marking of synthetic content — sources conflict whether deferred to this date **(unverified)** | Only if Hubtask generates text/images marketed as AI output |
| 2026-12-09 | **New Product Liability Directive** applies (Germany: new ProdHaftG adopted Oct 2026); software incl. SaaS is a product | Commercial distribution/operation of Hubtask |
| 2026-12-16 | Expected OJ citation of EN 301 549 v4.1.1 (WCAG 2.2) as harmonised standard **(unverified)** | Accessibility target shifts from WCAG 2.1 to 2.2 AA |
| 2027-01-12 | Data Act: switching charges must be zero (reduced cost-based charges allowed until then) | SaaS providers |
| 2027-12-02 | AI Act Annex III high-risk obligations (deferred by Digital Omnibus) | Only if a Hubtask AI feature is used for an Annex III purpose (e.g. employment: task allocation, performance monitoring) |
| 2027-12-11 | **CRA fully applicable**: Annex I essential requirements, conformity assessment, CE marking, SBOM, support period, steward obligations | Every Hubtask release placed on the EU market after this date |
| 2028-08-02 | AI Act Annex I (embedded product) high-risk obligations | Not relevant |

Sources: CRA reporting date — https://www.matheson.com/insights/the-eu-cyber-resilience-act-recap-reporting-obligations/, https://pasqualepillitteri.it/en/news/15586/cyber-resilience-act-reporting-obligations-september-11 ; AI Act Omnibus — https://www.gibsondunn.com/eu-ai-act-omnibus-agreement-postponed-high-risk-deadlines-and-other-key-changes/ , https://labs.cloudsecurityalliance.org/research/csa-research-note-eu-ai-act-high-risk-deadline-omnibus-20260/ ; NIS2UmsuCG — https://www.usd.de/nis2umsucg-offiziell-in-kraft/ , https://www.secjur.com/blog/nis2-umsetzungsgesetz ; PLD Germany — https://www.mtrlegal.com/en/bundestag-adopts-new-product-liability-law/ ; EN 301 549 v4.1.1 — https://nda.ie/news/en301549-published , https://www.deque.com/blog/en-301-549-v4-1-1-is-final-what-changed-what-it-means-and-what-you-should-do/

### 1.2 GDPR (and BDSG)

| Aspect | Applicability | Product requirements | Process requirements |
|---|---|---|---|
| Art. 25 privacy by design/default | SH operator and SaaS provider are bound; the project is not directly, but buyers require it (and Art. 25 recital 78 asks producers to take it into account) | Data minimisation per field; private-by-default visibility of new hubs; configurable retention per data class with automatic deletion; pseudonymised analytics; no user content in logs (already rule 10); encryption in transit and at rest; per-tenant export | A privacy-impact checklist per use case; a data inventory generated from the schema (which columns hold personal data, their retention) |
| Art. 30 records of processing | Controller (SH) and processor (SaaS) must keep them | Ship a **template RoPA** and a data-flow description per module (tasks, comments, time tracking, CMDB, AI) so operators can copy it | Keep the template current with each new module — a docs gate |
| Art. 35 DPIA | Required for operators when processing is likely high-risk: employee monitoring via time tracking, AI-based assessment, large-scale ITSM with special data | Product must make a DPIA *possible*: documented purposes, a feature switch for each monitoring-capable feature (time tracking, activity feeds, presence, AI analysis), works-council-friendly defaults | Publish a DPIA support document ("DPIA kit"); in Germany §26 BDSG/§87(1) No. 6 BetrVG means any feature that can monitor employee performance triggers works-council co-determination — buyers will ask for a feature matrix |
| Art. 28 processors | SaaS/MSP is processor; it uses sub-processors (hosting, mail, AI model APIs) | Tenant-level configuration that shows which sub-processors are active (mail relay, AI provider, storage); ability to disable each; audit log export for the controller | A standard DPA (AVV) template, sub-processor list with change notification (typically 30 days), TOMs annex — mapping to ISO 27001 Annex A controls |
| Data subject rights (Art. 15–22) | All operators | Per-person export (machine-readable, Art. 20), erasure with documented exceptions (legal hold, GoBD retention), rectification, restriction flag; erasure must reach backups by expiry and search indexes/read models/CRDT histories immediately | Documented deletion concept (DIN 66398 "Löschkonzept" is the German reference) |
| Art. 33/34 breach notification (72 h) | Controller; processor must notify controller "without undue delay" | Security event log, tenant-scoped forensic export, alerting | Incident playbook that feeds both GDPR and NIS2/CRA timelines |
| Ch. V international transfers | Any use of non-EU AI models, mail, telemetry | **BYO model with EU endpoint option**; no telemetry/phone-home by default; all third-country calls configurable and listed; EU-US Data Privacy Framework is valid but under challenge (Latombe judgment upheld it in Sept 2025 **(unverified)**) — provide a "no third-country transfer" mode | Transfer impact assessment template for each optional integration |
| Digital Omnibus GDPR changes | Proposed (Nov 2025): narrower personal-data definition, AI training legitimate interest, cookie rules moved into GDPR; still in Council/Parliament as of Oct 2026 | Design for current law; do not rely on the relief | Track; https://ppc.land/digital-omnibus/ |

### 1.3 NIS2 / NIS2UmsuCG (BSIG new version)

**Who is in scope.** Size-cap rule: medium (≥50 employees or >€10 m turnover and balance sheet) or large companies in Annex I/II sectors. Annex I "ICT service management (B2B)" covers **managed service providers and managed security service providers**, and "digital infrastructure" covers **cloud computing service providers** — a provider running Hubtask as SaaS for businesses is likely a cloud computing service provider; an MSP running it for customers is an MSP. In Germany: "besonders wichtige Einrichtung" (≥250 employees or >€50 m turnover and >€43 m balance sheet) and "wichtige Einrichtung". Registration with BSI (via "Mein Unternehmenskonto"/ELSTER, then BSI portal) was due 6 March 2026. Pure self-hosting individuals and the open-source project itself are not in scope.

| Obligation (BSIG new) | Who | Product requirement | Process requirement |
|---|---|---|---|
| §30 risk management measures (≈ NIS2 Art. 21(2) a–j) | In-scope SaaS/MSP operators; and every in-scope **customer** using Hubtask | MFA everywhere (incl. admin and API tokens), SSO (OIDC/SAML), encryption, access control with least privilege, audit logs, backup/restore, vulnerability handling, secure update channel | Operators must assess **supply chain security** — they will send Hubtask security questionnaires and ask for SBOMs, CVD policy, patch SLAs, pen test summaries |
| §32 incident reporting: early warning 24 h, notification 72 h, final report 1 month | In-scope operators | Detection signals: anomalous login, mass export, privilege change, tenant-boundary violation alerts; structured incident timeline export | Joint playbook with CRA/GDPR |
| §38 management accountability, training | Operators | — | Not a product matter |
| NIS2 Implementing Regulation (EU) 2024/2690 | Applies directly to cloud providers, MSPs, MSSPs, data centres regardless of size thresholds' national flavour (for those in scope) | Detailed technical measures: logging (retention, integrity, time sync), access control, crypto policy, MFA, configuration management, backup testing | This regulation is the most concrete checklist for a provider of Hubtask — map it in the operator guide |

Sources: https://www.advisori.de/blog/nis2-umsetzungsgesetz-nis2umsucg ; Implementing Regulation 2024/2690 https://eur-lex.europa.eu/eli/reg_impl/2024/2690/oj

### 1.4 Cyber Resilience Act (Regulation (EU) 2024/2847)

**Role question — the most important legal decision for the project.**

| Scenario | CRA role of the owner | Consequence |
|---|---|---|
| Purely non-commercial Apache-2.0 project, no monetisation | Not a manufacturer; possibly not in scope at all (Recital 18) | Nothing mandatory; good practice only |
| A legal entity (e.g. foundation, UG/GmbH) that systematically supports the OSS for commercial use by others, without itself monetising it | **Open-source software steward** (Art. 3(14), Art. 24) | Documented cybersecurity policy, CVD, cooperation with market surveillance, Art. 14 reporting to the extent involved (from 2026-09-11 for reporting; rest from 2027-12-11); no CE marking, no conformity assessment; no administrative fines (Art. 64(10)(b)) |
| Owner sells paid editions, paid hosted service built on the product, licences, or "support above cost"; or the same entity runs SaaS with the software | **Manufacturer** for the monetised product (Commission draft guidance 2026: paid enterprise editions, monetising services through software = commercial) | Full Annex I, technical documentation, conformity assessment, EU declaration of conformity, CE marking, support period ≥5 years (or expected use time), SBOM, reporting |
| Third party (provider/MSP) that takes the OSS and places its own branded distribution on the market | That party is manufacturer of its distribution | Provider will demand from upstream: SBOM, VEX, security advisories, signed artefacts |
| SaaS only (no software made available) | CRA does not cover pure SaaS (remote data processing is in scope only as part of a product); NIS2 covers SaaS | But the Tauri desktop/Android shells and self-hosted images **are** products with digital elements |

Hubtask's Tauri desktop and Android clients, its server images and Helm chart are all "products with digital elements". Category: a task/work-management platform is **default category** (self-assessment, module A) — it is not in Annex III "important" lists (identity management systems, password managers, VPNs, SIEM, etc. are). **Caveat:** if Hubtask's identity module is marketed as a standalone "identity management system" or its ITSM/CMDB as a security product, Class I might apply **(unverified — check the implementing act on technical descriptions, adopted Dec 2025 per FOSDEM 2026 slides)**.

**What the CRA requires from the software (Annex I Part I)** — mapped to Hubtask:

| Annex I requirement | Concrete Hubtask feature |
|---|---|
| No known exploitable vulnerabilities at release | Release gate: govulncheck, OSV scan of npm/Cargo/Go, container scan; block on reachable high/critical without VEX |
| Secure by default configuration, with reset to original state | Default install: TLS required, no default credentials (first-run setup token), registration closed, AI off, telemetry off; `hubtask admin reset-config` |
| Security updates, automatic where possible, with opt-out | Update notification in admin UI; signed images; Helm chart with documented upgrade; desktop auto-update via Tauri updater with signature check |
| Protection against unauthorised access (authN, identity, access management) | OIDC/SAML SSO, MFA/passkeys, RBAC, session management, brute-force protection, audit of auth events |
| Confidentiality (encryption at rest and in transit) | TLS 1.2+/1.3 (TR-02102-2), DB/storage encryption guidance, field-level encryption for secrets, BYOK option for SaaS |
| Integrity of data, commands, programs, configuration | Signed releases, signed webhooks (HMAC), audit-log hash chain, CSRF/replay protections, sync operation integrity |
| Data minimisation | Covered by GDPR design |
| Availability, resilience against DoS | Per-tenant rate limits, quotas, backpressure, graceful degradation |
| Limit attack surface, incl. external interfaces | Feature flags to disable MCP, webhooks, intake mail, calendar feeds; minimal container image (distroless), no shell |
| Logging and monitoring of security-relevant activity, with opt-out for user | Security event log (separate from audit trail), OpenTelemetry export |
| Secure deletion of all data and settings | Tenant hard-delete procedure covering DB, object storage, search index, caches, backups-by-expiry |

**Annex I Part II — vulnerability handling (the process side):**

| Requirement | Concrete process |
|---|---|
| Identify and document components: **SBOM** in a commonly used machine-readable format, at least top-level dependencies | Generate CycloneDX 1.6/1.7 (or SPDX 2.3/3.0) per release artefact (server image, desktop, Android, web bundle); attach to release and as OCI attestation |
| Address vulnerabilities without delay, incl. security updates | Published patch SLA (e.g. critical ≤7 days, high ≤30 days) |
| Regular tests and reviews | SAST, DAST, fuzzing, annual pen test |
| Public disclosure of fixed vulnerabilities with description, severity, remediation | GitHub Security Advisories → CVE (GitHub is a CNA), CSAF 2.0 feed, VEX |
| Coordinated vulnerability disclosure policy | `SECURITY.md`, `/.well-known/security.txt` (RFC 9116), ISO/IEC 29147/30111-aligned process |
| Contact address for reports | security@ mailbox + private advisory reporting |
| Secure distribution of updates | Sigstore/cosign signatures, SLSA provenance |
| Security updates free of charge, separate from feature updates where technically feasible | Patch releases on supported minor lines (LTS) |

**Further manufacturer obligations:** support period declared on product and at purchase (≥5 years unless shorter expected use; security updates must remain available 10 years after issue or for the support period, whichever is longer); technical documentation (Annex VII) incl. a cybersecurity risk assessment kept 10 years; EU declaration of conformity (Annex V) and CE marking (for software: on the declaration/website/accompanying docs); user information (Annex II): intended purpose, support end date, how to report vulnerabilities, secure decommissioning; Art. 14 reporting from **11 Sept 2026** for actively exploited vulnerabilities (24 h early warning / 72 h notification / final report 14 days after a fix) and severe incidents (24 h / 72 h / 1 month) via ENISA's Single Reporting Platform; informing users of the incident. Due diligence on integrated third-party components (Art. 13(5)) — including OSS dependencies — and reporting vulnerabilities found in them upstream.

**Harmonised standards:** CEN/CENELEC JTC 13 and ETSI are drafting ~41 standards (horizontal: secure design EN 40000-1-x, vulnerability handling); first deliverables due 2026–Q3 2026, none confirmed cited in the OJ yet **(unverified)**. Until then, ISO/IEC 27001 + IEC 62443-4-1 + ETSI EN 303 645 style mapping, and BSI TR-03183 (German CRA-oriented technical guideline incl. SBOM requirements, TR-03183-2) are the best reference. BSI TR-03183-2 requires CycloneDX ≥1.5 or SPDX ≥2.2.1 with specific fields (hash, licence, supplier) **(verify current version)**.

Sources: https://best.openssf.org/CRA-Brief-Guide-for-OSS-Developers ; https://www.opensourceforu.com/2026/07/eu-clarifies-cra-rules-for-non-commercial-open-source/ ; https://www.noze.it/en/insights/cra-commission-guidance/ ; https://fosdem.org/2026/events/attachments/EERURR-implementing_the_cyber_resilience_act_-_engaging_with_open_source/slides/267519/slides-cr_pyebtat.pdf ; BSI TR-03183 https://www.bsi.bund.de/dok/TR-03183

**Recommendation:** decide early which legal entity holds the project. A clean structure is "steward entity for the Apache-2.0 project; separate commercial entity as manufacturer of a supported distribution/hosted offering". Either way, implement the manufacturer process now — it is what NIS2 customers and providers demand anyway, and switching later is expensive. Hubtask's existing gates (architecture, security, SBOM-capable CI) are a good base.

### 1.5 EU AI Act (Regulation (EU) 2024/1689, amended by the Digital Omnibus on AI, 2026)

| Role | When Hubtask is in it | Obligations |
|---|---|---|
| **Provider of an AI system** | The owner/manufacturer who integrates an LLM and places an "AI system" (suggestions, agents, summaries) on the market under its name — even with BYO models | For non-high-risk: Art. 50 transparency; Art. 4 AI literacy. For high-risk: Art. 9–17 (risk management, data governance, logging, human oversight, accuracy/robustness, QMS, conformity assessment, EU database registration) |
| **Deployer** | SH operators and SaaS providers using the AI features professionally | Art. 4 AI literacy; Art. 50(3)/(4) disclosures where applicable; for high-risk: use per instructions, human oversight, log retention ≥6 months, inform workers and works council (Art. 26(7)), FRIA for public bodies |
| **GPAI model provider** | Not Hubtask (it does not train/provide models). If it fine-tunes a model substantially and distributes it — possibly | Ch. V obligations (since Aug 2025) |
| Downstream provider | Hubtask building on GPAI models | Gets documentation from the GPAI provider (Art. 53(1)(b)); must pass relevant info on |

**High-risk trap:** Annex III point 4 (employment, workers management) covers AI used "to allocate tasks based on individual behaviour or personal traits or characteristics or to monitor and evaluate the performance and behaviour of persons in such relationships". A task manager whose AI **assigns work to people based on their behaviour/performance, or scores productivity**, would be high-risk (from 2 Dec 2027). Art. 6(3) exemption applies to narrow procedural tasks, improving the result of a human activity, preparatory tasks — but never when profiling natural persons.

**Product requirements derived:**

| Requirement | Hubtask design |
|---|---|
| Art. 50(1): people must be informed they interact with an AI system | Visible AI badge on agent chat, AI-authored comments, agent actors in activity streams ("Agent X on behalf of Y") |
| Art. 50(2): machine-readable marking of AI-generated content (text, images) | Provenance metadata on AI-generated content (field `generated_by`, model id); C2PA for images if image generation is added |
| Stay out of Annex III(4) by design | No AI-based assignment to individuals by behaviour/performance, no productivity scoring; AI may suggest *task* properties (labels, dates, summaries), not rank *people*. Make it a written design rule. If ever wanted: separate, explicitly high-risk module |
| Human oversight (good practice anyway) | Agent actions proposal → approval; reversible actions; per-tool permissions; kill switch per tenant |
| Logging | Per-agent action log with tool inputs/outputs metadata (no content in ops logs, rule 10; content in the tenant's audit trail only where allowed) |
| Deployer enablement | Instructions for use, model card for each shipped prompt/agent, list of AI features with a switch each, AI literacy material |

Sources: https://www.gibsondunn.com/eu-ai-act-omnibus-agreement-postponed-high-risk-deadlines-and-other-key-changes/ ; https://www.processexcellencenetwork.com/ai/articles/eu-ai-act-2026-what-actually-changed-and-what-deployers-still-must-do ; AI Act text https://eur-lex.europa.eu/eli/reg/2024/1689/oj

### 1.6 Data Act (Regulation (EU) 2023/2854), Ch. VI switching, Art. 23–31

Applies since 12 Sept 2025 to **providers of data processing services** — i.e. anyone offering Hubtask as SaaS/managed service (B2B and B2C). Not to the self-hoster for itself.

| Obligation | Product requirement |
|---|---|
| Art. 23/25: contract allows switching with ≤2 months notice, transition ≤30 days (extendable to 7 months if technically infeasible), data retrieval ≥30 days after | Full tenant export: all exportable data (tasks, history, comments, attachments, custom field definitions, workflows, users/roles, audit log) in documented open formats (JSON/NDJSON + files + schema), **and an import of the same format** into another Hubtask instance |
| Art. 26: information on exportable data and on procedures | Export documentation; a "data catalogue" page in the admin UI |
| Art. 29: switching charges — reduced (cost-based) until 12 Jan 2027, then **zero** | No paywalled export; export self-service |
| Art. 30: functional equivalence for IaaS; for SaaS: open interfaces, compatibility with open interoperability specifications/standards | Public OpenAPI, stable export schema, versioned |
| Art. 32: safeguards against unlawful third-country government access to non-personal data | Data residency; transparency on jurisdiction; encryption |
| Art. 35: interoperability specifications (Commission may adopt) | Track |

Digital Omnibus proposes narrowing switching obligations (e.g. for custom-built services and SMEs, contracts before 12 Sept 2025) — not adopted as of Oct 2026 (https://eaccny.com/?p=57747). Self-hosting + open source is a strong Data Act story: "the exit format is the import format".

### 1.7 European Accessibility Act / German BFSG

| Aspect | Applicability | Requirement |
|---|---|---|
| Scope | EAA covers specific consumer products/services: e-commerce services, banking, e-books, communications, transport. A generic B2B task manager is **not** listed. In scope: the **online sales/sign-up flow** of a B2C SaaS (e-commerce service: concluding a consumer contract online); a Hubtask web shop. Microenterprises (<10 staff, ≤€2 m) providing services are exempt | Consumer-facing website (hubtask.eu sign-up, checkout) and web app parts reached by consumers should meet EN 301 549 |
| Public sector | Directive (EU) 2016/2102 / BITV 2.0 (Germany): public bodies must procure and run accessible software; procurement Directive 2014/24 Art. 42 makes accessibility a technical specification | Any public-sector or public-benefit buyer will require EN 301 549 conformance and an accessibility statement |
| Large enterprise buyers | Not legal, but procurement routinely asks for a VPAT/ACR (EN 301 549 edition) | Publish an Accessibility Conformance Report |
| Standard | EN 301 549 v3.2.1 (WCAG 2.1 AA) is the cited harmonised standard; **v4.1.1** published 2 Sept 2026, aligned with **WCAG 2.2 AA** plus EAA annexes; OJ citation expected ~Dec 2026 **(unverified)** | Target WCAG 2.2 AA now (new criteria: focus not obscured, dragging alternatives, target size 24×24, consistent help, redundant entry, accessible authentication). Drag-and-drop boards, whiteboard and Gantt need keyboard/single-pointer alternatives |
| BFSG specifics | Accessibility statement (Annex 3 BFSG) in general terms and conditions, market surveillance by MLBF (Magdeburg) | Accessibility statement page, feedback mechanism |

Sources: https://nda.ie/news/en301549-published ; https://www.dwt.com/insights/2026/09/european-accessibility-act-ict-standards-update

### 1.8 DORA (Regulation (EU) 2022/2554) — financial-sector customers

Applies since 17 Jan 2025 to financial entities; providers of Hubtask to banks, insurers, investment firms become **ICT third-party service providers**. Task/ITSM/CMDB tools often support "critical or important functions" (ITSM incident management is explicitly part of DORA operations — a Hubtask service desk used for ICT incident handling at a bank is likely supporting an important function).

| What financial customers must get contractually (Art. 30) | Product/provider requirement |
|---|---|
| Clear description of services, locations of data processing (regions) | Data residency selection; documented sub-processing chain |
| Availability, integrity, confidentiality guarantees; SLAs with quantitative targets (for critical functions) | Published SLOs, status page, RPO/RTO |
| Access, recovery and return of data on insolvency/termination | Data Act export doubles as DORA exit |
| Incident assistance at no extra cost or pre-agreed cost | Incident notification to customer within agreed hours, forensic data export |
| Cooperation with competent authorities; **unrestricted audit, access and inspection rights** for the entity and authorities | Pooled audits, SOC 2/ISO 27001/C5 reports as primary evidence; on-site audit clause |
| Participation in customer's ICT security training and TLPT (threat-led pen testing) | Willingness to join red-team exercises |
| Termination rights and **exit strategies** with mandatory transition period | Documented exit plan; self-hosting the same OSS is the ultimate exit |
| Subcontracting rules (RTS on subcontracting, 2025) | Sub-processor transparency and notice |
| Register of information (Art. 28(3)) | Provide LEI, service identifiers, data locations so customers can fill their register |

The self-hosted OSS route avoids most of this — the bank runs it itself; then DORA asks the bank for software supply chain diligence (SBOM, patch SLA, vendor support contract). Sources: https://eur-lex.europa.eu/eli/reg/2022/2554/oj ; RTS on contractual policy (EU) 2024/1773.

### 1.9 eIDAS 2 (Regulation (EU) 2024/1183)

Only relevant if Hubtask adds **approvals with legal effect** (contract sign-off, ITSM change approvals with signature). Simple electronic signature (click-approve with audit trail) needs nothing; advanced/qualified signatures require integration with a QTSP (remote signing via CSC API) — Hubtask should not implement a signature itself. EUDI Wallets must be offered by member states by end of 2026 and accepted by very large platforms and regulated sectors (by ~2027) — for Hubtask, wallet-based login (OpenID4VP) is an optional future identity provider, not an obligation **(unverified for Hubtask specifically)**. Recommendation: model approvals as signed, hash-chained audit events; offer a QTSP connector later.

### 1.10 GoBD (German principles for books and records in electronic form, BMF letter 2019, amended 2024/2025)

Applies to the **business user** (taxpayer), not to the vendor, but buyers need the software to support compliance when it holds records relevant to taxation: time tracking used for invoicing, cost/expense data, contracts, invoice-relevant ITSM tickets.

| GoBD principle | Product requirement |
|---|---|
| Unalterability (Unveränderbarkeit), traceability | Change history for every booking-relevant record (time entries, costs); no silent edits; locked periods ("closed months") |
| Retention 6/8/10 years (§147 AO; 10 → 8 years for accounting vouchers since 2025 via BEG IV) | Retention class per record type that **overrides user deletion**; legal hold; GDPR erasure must respect it (Art. 17(3)(b)) |
| Machine evaluability, data access for tax auditors (Z1–Z3) | Export of time/cost data in structured format (CSV + description, GDPdU/"Beschreibungsstandard") |
| Verfahrensdokumentation | Ship documentation the operator can use for its own procedure documentation |
| Time tracking law context | EuGH C-55/18 and BAG 1 ABR 22/21: employers must record working time — a time-tracking module will be used for it; German draft Arbeitszeitgesetz amendment pending **(unverified status)** |

### 1.11 TDDDG (formerly TTDSG) §25 — cookies and device access

Consent is required for storing/reading information on the end device unless **strictly necessary** for a service the user explicitly requested. Session cookies, CSRF tokens, local offline storage (IndexedDB for sync) are strictly necessary for a logged-in task app. Product analytics, marketing pixels, third-party fonts/CDNs (transfer of IP) are not. Requirement: **no non-essential storage or third-party requests by default** in the web app and website; self-host fonts; if a provider adds analytics, a consent mechanism hook. Recognised consent-management services (PIMS) under the Einwilligungsverwaltungsverordnung (2025) are optional. This also simplifies the website: no banner needed if nothing non-essential is used.

### 1.12 Product Liability Directive (EU) 2024/2853

Applies to products placed on the market after **9 Dec 2026**; Germany adopted the new ProdHaftG in Oct 2026. Software (including SaaS and AI systems) is a product; manufacturer liable without fault for damage to persons, private property, and **destruction or corruption of data not used for professional purposes**. Defectiveness is judged incl. cybersecurity requirements (CRA non-compliance = presumption aid), failure to provide security updates under the manufacturer's control, and substantial modifications via updates. **Free and open-source software developed or supplied outside a commercial activity is excluded**; a commercial distributor/integrator is liable. Liability cannot be excluded by contract or licence (Apache-2.0 disclaimer does not help commercial offerings vis-à-vis consumers). Disclosure obligations in court mean **documentation of design decisions, tests and security updates is evidence** — the existing ADR/test discipline helps. Product requirement: data-loss protection for B2C users (backups, undo, soft-delete with recovery window).

Sources: https://www.mtrlegal.com/en/new-product-liability-law-applies-from-december-2026/ ; https://www.aoshearman.com/en/insights/update-on-product-liability-law

### 1.13 Other items worth noting

| Item | Relevance |
|---|---|
| **EUCS** (EU cloud certification scheme) | Still not adopted (sovereignty requirements contested) **(unverified as of Oct 2026)**; C5:2025 aligns with EUCS "substantial" |
| **Health sector §393 SGB V** | Cloud processing of health/social data requires a **C5 Type 2** attestation (since 1 July 2025) — relevant if a provider sells Hubtask to hospitals/insurers |
| **KRITIS-Dachgesetz** (physical resilience) | Operators only |
| **Whistleblower Act (HinSchG)** | Only if an intake/service desk is used as a reporting channel: confidentiality, 3-year retention, access restriction |
| **Works Constitution Act §87(1) No. 6** | Any feature able to monitor employees → co-determination; feature switches and a documented "monitoring-capable features" list are a sales necessity in Germany |
| **NIS2 + CRA for MSPs** | MSPs reselling Hubtask must handle both sides; give them a "provider kit" |

---

## Part 2 — Security and quality standards

### 2.1 Management system and attestation standards

| Standard | Applicability | Requirement for Hubtask | Notes / deadline |
|---|---|---|---|
| **ISO/IEC 27001:2022** | SaaS/MSP operators; the OSS project only if the commercial entity certifies | Product must supply evidence for Annex A controls: 8.15 logging, 8.16 monitoring, 8.24 crypto, 8.25–8.29 secure development (secure SDLC, secure coding, security testing), 8.9 config management, 8.12 DLP, 8.28 secure coding; 5.23 cloud services | Transition from 2013 ended 31 Oct 2025; Amendment 1:2024 (climate) |
| **ISO/IEC 27017:2015** | Cloud provider | Shared-responsibility matrix; tenant segregation (CLD.9.5.1), admin operations, monitoring of cloud services | Revision in progress **(unverified)** |
| **ISO/IEC 27018:2025** | Cloud processor of PII | Purpose limitation, no use for marketing without consent, sub-processor disclosure, return/deletion, breach notice, location transparency | Revised 2025 edition **(unverified)** |
| **ISO/IEC 27701:2025** | Privacy information management | Now standalone management system standard (2025 edition) **(unverified)** | Pairs with GDPR |
| **ISO/IEC 42001:2023** | AI management system | Increasingly asked for by buyers of AI features; maps to AI Act QMS | Optional |
| **BSI C5:2020 → C5:2025** | German public sector, health (§393 SGB V), banks, many enterprises demand C5 Type 2 for cloud | Basic criteria + **corresponding criteria (customer responsibilities)** — product documentation must state what the customer must configure | C5:2025 community draft 2025, aligned to EUCS "substantial", adds supply chain, AI, product security, environment separation, vulnerability management; final publication and transition period **(unverified — check bsi.bund.de)**. https://www.roedl.com/en/insights/bsi-c5-establishing-itself-as-a-cross-industry-standard-for-cloud-security/ |
| **BSI IT-Grundschutz** (Kompendium 2023; modernised "Grundschutz++" in progress) | Public sector and KRITIS customers model their systems with Bausteine: APP.3.1 web applications, APP.4.3 relational DBs, SYS.1.6 containers, APP.4.4 Kubernetes, CON.8 software development, OPS.2.2 cloud usage | Provide a Grundschutz mapping for the Hubtask deployment (which requirements are satisfied by the product, which by the operator) | Grundschutz++ as machine-readable OSCAL catalogue announced for 2026 **(unverified)** |
| **SOC 2 Type II** | Non-German/international enterprise buyers | Trust Services Criteria (security + availability + confidentiality); evidence of change management, access reviews, monitoring | AICPA; pairs with ISO 27001 |
| **TISAX** | Automotive customers | Only if sold to automotive | Optional |

**Recommendation:** design one control mapping (OSCAL or a YAML table) where each product capability references the ISO 27001 Annex A / C5 / NIS2 IR 2024/2690 / CRA Annex I items it supports. Providers then reuse it for their attestations ("compliance kit").

### 2.2 Application security standards

| Standard | What it requires / how Hubtask uses it |
|---|---|
| **OWASP ASVS 5.0** (May 2025; ~350 requirements, 17 chapters, levels 1–3) | Adopt **Level 2** as the baseline, Level 3 for authentication, session, access control, crypto and API chapters. Track each requirement as a test or a documented N/A — a natural fit for a gate. https://owasp.org/www-project-application-security-verification-standard/ |
| **OWASP Top 10:2025** | A01 Broken Access Control, A02 Security Misconfiguration, **A03 Software Supply Chain Failures** (new), A04 Crypto Failures, A05 Injection, A06 Insecure Design, A07 Authentication Failures, A08 Software/Data Integrity Failures, A09 Logging & Alerting Failures, **A10 Mishandling of Exceptional Conditions** (new: fail-open, error handling). For Hubtask: cross-tenant tests (A01), "fail closed" on authorisation errors (A10) — https://owasp.org/Top10/2025/0x00_2025-Introduction/ |
| **OWASP API Security Top 10 (2023)** | API1 BOLA (object-level authZ on every ID), API3 broken object property-level authZ (mass assignment, field-level visibility), API4 unrestricted resource consumption (pagination limits, query DSL cost limits), API5 function-level authZ, API6 unrestricted access to sensitive business flows (invite spam, export abuse), API10 unsafe consumption of APIs (webhooks, connectors) |
| **OWASP Top 10 for LLM Applications 2025** | LLM01 prompt injection, LLM02 sensitive info disclosure, LLM05 improper output handling, LLM06 excessive agency, LLM07 system prompt leakage, LLM08 vector/embedding weaknesses (cross-tenant leakage in RAG indexes), LLM10 unbounded consumption |
| **OWASP Top 10 for Agentic Applications 2026** (Dec 2025) | ASI01 agent goal hijack, ASI02 tool misuse, ASI03 identity & privilege abuse, ASI04 agentic supply chain (MCP servers/tools), ASI05 unexpected code execution, ASI06 memory & context poisoning, ASI07 insecure inter-agent comms, ASI08 cascading failures, ASI09 human-agent trust exploitation, ASI10 rogue agents. https://www.giskard.ai/knowledge/owasp-top-10-for-agentic-application-2026 |
| **OWASP SAMM 2.0** | Maturity model for the development process — useful for self-assessment and for answering buyer questionnaires |
| **BSI TR-02102-1..4** (crypto, annually updated, 2026 edition) | Approved algorithms and key lengths: AES-GCM, SHA-2/3, RSA ≥3000 bit (from 2023+), ECDSA/ECDH ≥250 bit, TLS 1.2 with listed suites / TLS 1.3; **post-quantum**: hybrid ML-KEM recommended for long-term confidentiality; passwords: Argon2id. Follow TR-02102 rather than FIPS for German buyers; FIPS 140-3 only for US federal (Go 1.24+ has a native FIPS 140-3 module — useful if ever needed). https://www.bsi.bund.de/dok/TR-02102 |
| **Post-quantum** | Go 1.24 enables X25519MLKEM768 hybrid key exchange in TLS by default; EU PQC roadmap (June 2025) asks to start migration by end 2026 and finish high-risk by 2030 **(unverified details)**. Keep crypto agile: algorithm identifiers stored with ciphertext/keys |

### 2.3 Supply-chain and vulnerability disclosure standards

| Standard | Requirement for Hubtask |
|---|---|
| **SLSA v1.2** (Nov 2025, adds Source Track) | Build L3: hosted, isolated build (GitHub Actions reusable workflow), non-forgeable provenance (in-toto attestation via `slsa-github-generator` or `actions/attest-build-provenance`). Source track: branch protection, two-party review, retained history — https://slsa.dev/blog/2025/11/announce-slsa-v1.2 |
| **SBOM formats** | CycloneDX 1.7 (Oct 2025, ECMA-424; adds citations, CBOM) or SPDX 3.0.1 (ISO/IEC 5962 covers 2.2.1). Produce **CycloneDX** for each artefact (Go: `cyclonedx-gomod`, npm: `@cyclonedx/cyclonedx-npm`, Cargo: `cargo-cyclonedx`, container: Syft); attach as cosign attestation; meet BSI TR-03183-2 fields. https://cyclonedx.org/news/cyclonedx-v1.7-released |
| **Sigstore/cosign** | Keyless signing of container images, Helm chart (OCI), release archives; verify in docs (`cosign verify --certificate-identity …`); Kubernetes admission policies (Kyverno/Sigstore policy-controller) for providers |
| **Reproducible builds** | Go builds are reproducible with `-trimpath`, fixed `-buildvcs`, `SOURCE_DATE_EPOCH`; container via `ko` or `apko` reproducible; frontend bundles reproducible with pinned lockfile. Verify by a second independent build in CI |
| **OpenSSF Scorecard** | Run the Scorecard action; target ≥8: pinned dependencies (actions by SHA), token permissions, branch protection, code review, SAST, fuzzing, signed releases, security policy, dependency update tool |
| **OpenSSF Best Practices badge** (passing → silver → gold) and **OSPS Baseline** (2025, levels 1–3) | Achievable at passing/silver quickly; OSPS Baseline maps to CRA — https://baseline.openssf.org **(verify URL)** |
| **CVE / CNA** | Publish advisories via GitHub Security Advisories (GitHub is a CNA) |
| **CSAF 2.0 (OASIS)** + **VEX** | BSI promotes CSAF; publish a CSAF provider directory (`/.well-known/csaf/provider-metadata.json`) and VEX documents (CSAF VEX profile or OpenVEX/CycloneDX VEX) stating "not affected" for dependency CVEs that are not reachable (`govulncheck` reachability makes this cheap) |
| **security.txt (RFC 9116)** | On hubtask.eu and in every instance (operator-configurable contact) |
| **ISO/IEC 29147:2018 / 30111:2019** | Disclosure process: intake, triage within x days, embargo, credit, advisory; internal handling process: root cause, variant analysis, fix, regression test |
| **Bug bounty** | Optional; start with a hall of fame + intake; later Intigriti/YesWeHack (EU platforms) — note "no other product names in the implementation" applies to code, not to process docs |
| **Pen tests** | Annual external pen test (white-box incl. multi-tenancy), publish summary letter; BSI-qualified "IS-Penetrationstest" providers favoured by public buyers |

### 2.4 Quality model and accessibility

| Standard | Use |
|---|---|
| **ISO/IEC 25010:2023** | Product quality model now has 9 characteristics: functional suitability, performance efficiency, compatibility, **interaction capability** (replaces usability), reliability, security, maintainability, flexibility (replaces portability), **safety** (new). Use it to structure quality requirements in arc42 §10 (quality scenarios) |
| **ISO/IEC 25019/25012** | Quality in use / data quality — optional |
| **WCAG 2.2 AA / EN 301 549** | See 1.7. Process: automated axe-core in component tests and e2e, manual screen-reader audits (NVDA, VoiceOver, TalkBack) per release, ACR per major version. WCAG 3.0 is a working draft — not a target before 2028 |

---

## Part 3 — Platform engineering and operations expectations

Buyers' security questionnaires (e.g. CAIQ v4 from CSA, BSI C5 questionnaires, VDA ISA) and enterprise RFPs repeatedly ask for the items below. "SH" = must be possible for self-hosters; "SaaS" = the provider must operate it; "OSS" = the project must ship it.

| Area | Expectation | Concrete Hubtask requirement | Who |
|---|---|---|---|
| **SLOs / error budgets** | Published availability SLO (99.9 % typical, 99.95 % premium), latency SLOs, error budget policy (Google SRE book) | Ship SLI definitions and Prometheus recording rules + alert rules (multi-window burn rate) in the Helm chart; product exposes RED metrics per endpoint class | OSS ships, SaaS operates |
| **OpenTelemetry** | OTLP traces, metrics, logs; semantic conventions (HTTP, DB stable) | OTel SDK in Go server with tenant id as *pseudonymous* attribute only, no content (rule 10); W3C trace context propagation through web app → API → jobs → webhooks | OSS |
| **Twelve-factor** | Config via env, stateless processes, logs to stdout, disposability, dev/prod parity | Already largely the Go server pattern; keep migrations as a separate job, no local disk state except caches | OSS |
| **Kubernetes/Helm** | Helm chart with values schema, probes (liveness/readiness/startup), PodDisruptionBudget, HPA, NetworkPolicies, securityContext (non-root, read-only rootfs, seccomp RuntimeDefault), resource requests; Pod Security Standards "restricted"; operator optional | Helm chart in OCI registry, signed; `values.schema.json`; documented upgrade path; Docker Compose for small SH | OSS |
| **Zero-downtime upgrades** | Rolling updates with N/N-1 compatibility between app versions and schema | Expand/contract migrations (rule 12 already), migration job before rollout, feature flags for contract phase, sync protocol versioning for offline clients (clients may be weeks old) | OSS |
| **Backup/restore, RPO/RTO** | Typical SaaS: RPO ≤15 min (WAL archiving/PITR), RTO ≤4 h; tested restores quarterly | Document pgBackRest/WAL-G or CloudNativePG backups; object storage versioning; **per-tenant restore** (logical export/import of one tenant) — the most-asked-for and hardest feature in shared-schema multi-tenancy | OSS docs + tool; SaaS operates |
| **Multi-region / data residency** | EU-only by default; some buyers want country-level (DE) residency; "sovereign cloud" claims | Tenant → region pinning (cell architecture: each cell = one DB cluster + app pool in one region); global directory service maps tenant/host to cell (ADR-0095 host-names-workspace fits) | SaaS |
| **Capacity/performance testing** | Load tests per release with defined workload model; documented sizing guide | k6 or Go-based load scenarios; "large tenant" fixtures (1 M tasks, 10 k users); performance budgets in CI (p95 of key queries via EXPLAIN-based regression or benchmark) | OSS |
| **Chaos testing** | Failure injection (DB failover, pod kill, network partition, object store outage) | Chaos Mesh/Litmus scenarios in staging; offline client resilience tests (sync after partition) | SaaS + OSS test suite |
| **Incident management** | On-call, severity scheme, status page, post-incident reviews (blameless), customer comms within SLA | Status-page integration; incident timeline export; runbooks per alert in the repo | SaaS |
| **Change management** | ITIL-ish: classified changes, approval for normal changes, rollback plans, audit trail (ISO 27001 8.32) | GitOps deployment (Argo CD/Flux), PR-based changes, release notes with security relevance tags | SaaS + OSS release process |
| **Secret management** | No secrets in env dumps/logs; rotation; external secret stores | Support file-mounted secrets and External Secrets/Vault; key rotation without downtime (multiple active signing keys, JWKS) | OSS |
| **Key management / BYOK / HYOK** | Envelope encryption: per-tenant data-encryption keys wrapped by a KEK in KMS/HSM; customer-managed keys for premium tenants; crypto-shredding for tenant deletion | Pluggable KMS port (Vault Transit, cloud KMS, PKCS#11 HSM); encrypt sensitive columns/attachments with per-tenant DEK; note: RLS + full-DB encryption is not BYOK — BYOK needs app-level envelope encryption for the content that matters | OSS port, SaaS operates |
| **Tenant isolation verification** | Evidence that tenant A cannot read B: automated tests, pen test focus, RLS policy tests | Cross-tenant negative test per repository method (already a project rule); a **generic isolation fuzzer** that replays every API operation with a foreign tenant's IDs; pgTAP/SQL tests asserting every table has `FORCE ROW LEVEL SECURITY` and a policy; DB role used by the app is not table owner and not `BYPASSRLS` | OSS |
| **Release cadence & LTS** | Predictable: e.g. minor every 4–6 weeks, LTS yearly with ≥2 years security support (CRA support period ≥5 years for the commercial product) | Semantic versioning; LTS branches with backported security fixes; published end-of-life table (endoflife.date format) | OSS |
| **API versioning/deprecation** | Stable major versions, deprecation notice ≥6–12 months, `Deprecation` and `Sunset` HTTP headers (RFC 9745 / RFC 8594), changelog | OpenAPI-driven diff check in CI (oasdiff) to detect breaking changes; webhooks versioned by payload schema | OSS |
| **Documentation** | arc42 + C4 for architecture, ADRs (MADR), Diátaxis (tutorials/how-to/reference/explanation) for user/admin docs, OpenAPI reference | Project already uses arc42/ADRs; add Diátaxis structure for operator docs and a security whitepaper | OSS |
| **Test pyramid** | Unit (table tests) > integration (Testcontainers) > contract > e2e (Playwright) | Already practiced; add **contract tests** for webhooks/connectors (Pact or schema-based via OpenAPI) and for the sync protocol between client versions | OSS |
| **Mutation testing** | Measures test strength on the domain core | Go: `go-mutesting`/Gremlins on `core/domain` nightly, trend not gate | OSS |
| **Fuzzing (Go native `go test -fuzz`)** | Parsers and decoders: query DSL, CEL rules, import formats, mail intake (MIME), markdown/rich text, sync operation decoding, iCal | OSS-Fuzz/ClusterFuzzLite in CI; corpus in repo; counts for Scorecard | OSS |
| **Dependency policy** | Allow-list licences, minimum age for new versions (supply-chain attacks via fresh releases), Renovate/Dependabot with grouping, pinned actions by SHA, `go mod verify`, npm `--ignore-scripts`/pnpm `onlyBuiltDependencies` | A written policy (new third-party dependency is already an owner decision) plus automated checks | OSS |
| **Licence compliance** | REUSE spec 3.3 (SPDX headers `SPDX-License-Identifier: Apache-2.0`, `SPDX-FileCopyrightText`), `reuse lint` in CI; OSS licence scanning of deps (ScanCode, `go-licenses`, ORT); NOTICE file aggregation as Apache-2.0 requires; DCO or CLA decision for contributions (a CLA matters if dual licensing/commercial editions are planned) | OSS |

---

## Part 4 — Architecture patterns for an extensible, performant multi-tenant work platform

### 4.1 Metadata-driven extensibility (custom types, fields, forms)

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **EAV** (entity-attribute-value rows) | Flexible, typed via value columns | Joins explode, no constraints, poor planner estimates, painful queries | Avoid for hot paths |
| **JSONB column per entity** (`custom jsonb`) + metadata tables describing the fields | One row per entity, fast reads, schema evolution without DDL, GIN indexing, partial updates with `jsonb_set` | No FK constraints inside JSON; type safety must be enforced by app/CHECK/JSON Schema; statistics on JSON keys are weak | **Recommended default** |
| **Per-tenant tables/DDL** (Salesforce-style "flex columns" or real ALTER TABLE) | Real columns and indexes | DDL in a multi-tenant DB = lock risk, migration explosion, catalogue bloat; incompatible with expand/contract discipline | Avoid |
| **Hybrid**: JSONB + promoted columns | Frequently filtered/sorted fields promoted to typed generated columns or a side table | Complexity | Use selectively |

Indexing recipe on PostgreSQL 17/18:
- `GIN (custom jsonb_path_ops)` for containment queries (`custom @> '{"f_123":"red"}'`); smaller and faster than default `jsonb_ops` but does not support key-existence operators.
- Field keys are **stable field IDs**, never display names (renames are free, rule 8 friendly).
- Sort/range queries on a custom field cannot use GIN: use an **expression index** `((custom->>'f_123')::date)` created on demand for heavy fields — but index DDL per tenant field is DDL again; mitigate with a **typed side table** `custom_values(tenant_id, entity_id, field_id, v_text, v_num, v_date, v_ref)` with composite B-tree indexes `(tenant_id, field_id, v_num)` — an "indexed EAV projection" maintained from JSONB as a read model. This gives sortable/filterable custom fields without DDL and keeps the JSONB as source of truth.
- **Generated columns** (`GENERATED ALWAYS AS … STORED`; PG 18 adds *virtual* generated columns) for system-defined derived fields.
- **Partial indexes** for hot subsets: `WHERE archived_at IS NULL`, `WHERE status_category <> 'done'`.
- All indexes lead with `tenant_id` (composite), matching RLS predicates.
- Validate custom values against a JSON Schema / CEL constraint compiled from field metadata in the application layer.

### 4.2 PostgreSQL at scale with RLS

| Pitfall | Mitigation |
|---|---|
| RLS policy functions not inlined / volatile `current_setting()` evaluated per row | Use `current_setting('app.tenant_id')::uuid` wrapped so it's evaluated once (e.g. `(SELECT current_setting(...))` sub-select pattern makes it an InitPlan); mark helper functions `STABLE`/`LEAKPROOF` where safe |
| Policies with joins/subqueries (e.g. membership checks in RLS) kill performance | Keep RLS for **tenant isolation only** (simple equality on `tenant_id`); do object-level authorisation in the application layer (rule 2 already says so) |
| Non-leakproof operators prevent index use under RLS (`security_barrier` semantics) | Test plans with RLS active (`SET ROLE app`), not as superuser; common operators on uuid/text are leakproof, but `LIKE`/full-text functions on some types are not — benchmark search under RLS |
| Table owner/superuser bypasses RLS | `ALTER TABLE … FORCE ROW LEVEL SECURITY`; app role not owner, no `BYPASSRLS`; CI test enumerates `pg_class` |
| Connection pooling with `SET` | `SET LOCAL` inside a transaction (already rule 3) is safe with PgBouncer transaction mode; never session-level `SET` |
| Noisy neighbour tenants | Statement timeouts per request class, per-tenant concurrency limits in app, `pg_stat_statements` with tenant tagging via query comments (sqlcommenter) |
| Very large tables | **Hash partitioning by tenant_id** (e.g. 16–64 partitions) for the biggest tables (activities/events, audit log, history) — or range partitioning by time for append-only logs (audit, events) with retention via `DROP PARTITION` (cheap deletion, GoBD retention per partition) |
| Scaling beyond one primary | Order: vertical + read replicas for read models/search → **cell architecture** (many independent Postgres clusters, tenant pinned to a cell, global directory) → Citus (distributed by tenant_id, co-located tables; supports RLS since Citus 12 for some cases **(unverified)**). Cells are simpler, fit data residency, and limit blast radius; prefer cells over Citus for a self-hostable product |
| Large tenant migration between cells | Needs per-tenant export/import (same mechanism as Data Act export) — design it once |

### 4.3 Events, outbox, CQRS

- **Transactional outbox**: write domain events into an `outbox` table in the same transaction; a relay publishes them (to webhooks, search indexer, notification, stream). In Postgres, the relay can be a River job or a `LISTEN/NOTIFY`-woken poller with `FOR UPDATE SKIP LOCKED`. Logical decoding (pgoutput/wal2json, Debezium) is an alternative when external brokers are needed.
- Keep the outbox as the **single integration seam**; no dual writes to a broker. External brokers (NATS JetStream, Kafka) only when providers need fan-out across services; NATS is a light, Go-native option **(owner decision: new dependency)**.
- **CQRS read models** where read shape differs strongly: board/list views with aggregated counts, dashboards, the custom-field projection (4.1), search index, CMDB relationship graph, timeline/Gantt. Read models in the same Postgres (materialised tables updated by event handlers) first; separate stores only for search.
- Events carry **IDs and change metadata, not content** where they leave the core (logs/metrics rule 10), and are versioned (schema registry = OpenAPI/AsyncAPI components). Webhook payloads described in OpenAPI 3.1 `webhooks` or AsyncAPI 3.0.

### 4.4 Search

| Option | Multilingual | Ops cost | Multi-tenancy | Verdict |
|---|---|---|---|---|
| Postgres FTS (`tsvector`, per-language config, GIN) + `pg_trgm` for fuzzy/prefix | Language-specific stemming; need per-document language detection; no CJK segmentation without extensions | None extra | RLS applies | **Default for SH and small SaaS** |
| ParadeDB `pg_search` (BM25 in Postgres, Tantivy) | Good tokenizers, BM25 ranking | Extension (AGPL) — licence check needed for distribution | In-DB | Watch; licence issue for Apache-2.0 bundling |
| Meilisearch | Good typo tolerance, multilingual tokenization (charabia) | Separate service, simple | Tenant tokens with filters | Good for SaaS UX |
| OpenSearch | Full analyzers (ICU, kuromoji), aggregations, vector search | Heavy ops | Index-per-tenant or routing + DLS | For large providers |
| Vector search (pgvector, HNSW) for AI/RAG | Embedding-based, language-agnostic | In-DB | **Must** filter by tenant before/inside ANN query (pgvector 0.8 iterative scans help filtered queries) | Use pgvector first |

Recommendation: a `SearchIndex` port with a Postgres FTS + trigram + pgvector default adapter (hybrid: reciprocal rank fusion of BM25-ish and vector scores) and an optional external adapter fed by the outbox. Index permissions: index documents carry visibility scope IDs; post-filter results through the application authorisation (rule 2) to avoid leaking titles.

### 4.5 Real-time collaboration (wiki, whiteboard, rich text)

- **Yjs** is the de facto CRDT for collaborative editors (ProseMirror/Tiptap, Lexical, tldraw-like whiteboards via y-* bindings). **Automerge 2/3** (Rust core, JS/WASM) is strong for JSON-document sync and history; Automerge 3 reduced memory dramatically **(unverified detail)**.
- Go server options: Yjs has no first-class Go implementation (y-crdt has Rust `yrs`, usable via WASM/FFI); the pragmatic pattern is a **"dumb relay + persistence" server in Go**: it authenticates, authorises per document, relays Yjs update messages (y-protocols sync/awareness), appends updates to Postgres (`doc_updates` table), and periodically compacts by merging updates — compaction can be done in a small sidecar using `yrs` or Node `yjs`, or client-driven snapshotting. Hocuspocus (Node) and y-sweet (Rust) are reference designs.
- Persist **update log + periodic snapshot**; derive plain-text/markdown projection for search and for MCP agents (agents should edit via structured operations converted to CRDT updates server-side, not raw text overwrite).
- GDPR: CRDT history retains deleted text — erasure requires snapshot rewrite (garbage-collected doc state) and dropping old updates.
- Keep CRDTs to free-form documents (wiki, whiteboard, long descriptions). Structured entities (tasks, fields) use the existing **operation-based offline sync with server authority** (last-writer-wins per field + conflict surfacing), which is easier to authorise and audit.

### 4.6 Job queues, workflows, rules

| Concern | Pattern / tool | Notes |
|---|---|---|
| Background jobs | **River** (Go, Postgres, `SKIP LOCKED`, transactional enqueue — enqueue in the same tx as the domain change, MPL-2.0) — River Pro features are commercial | Fits rule 3 if its client runs inside the tenant-setting transaction wrapper; check River's own connection use against "never pgxpool directly" (wrap it in the infrastructure adapter). Alternatives: own small `SKIP LOCKED` queue, or Riverqueue's competitors (`neoq`, `gue`) |
| Per-tenant fairness | Weighted fair queuing by tenant: queue partitions or per-tenant concurrency caps; River supports per-queue concurrency and (Pro) partitioned concurrency limits **(verify)** |
| Workflow/state machines | Domain-level **declarative state machines** (statuses, transitions, guards, actions) stored as tenant metadata, executed in the application layer; long-running orchestrations (approvals, SLAs, escalations in ITSM) as durable timers on the job queue | A full durable-execution engine (Temporal, Restate, DBOS-Go) only if workflows include long human waits plus external calls at scale; DBOS-Transact Go (Postgres-backed) is the lightest **(unverified maturity)** |
| BPMN | Enterprise buyers ask; offer import/export of a simple BPMN subset later, not as engine | Avoid embedding a Java engine |
| Rules engine | **CEL** (`cel-go`, Google; non-Turing complete, linear-time, cost estimation, sandboxed) for conditions in automations, validation, SLA policies, field visibility, ABAC | Cost limits per evaluation, compile/check at save time, types from field metadata; store compiled program cache |
| Scheduling (SLA timers, recurring tasks) | Job queue with scheduled_at; time zones and business calendars as domain objects | Clock port (rule 4) |

### 4.7 Plugins and extension isolation

| Level | Mechanism | Isolation | Use for |
|---|---|---|---|
| Configuration | Custom fields, workflows, CEL rules, templates | Full (data only) | Most customisation; prefer this |
| Out-of-process apps | Webhooks + OAuth-scoped REST/MCP APIs, app manifests declaring scopes, events, UI extension points | Network boundary, OAuth scopes, rate limits | Third-party integrations (the "marketplace app" pattern) |
| In-process sandboxed code | **WebAssembly** via `wazero` (pure-Go, no CGO) — Extism has a Go host SDK on wazero; resource limits (fuel/timeout via context, memory pages), capability-based host functions only | Strong if host functions are minimal; no network/FS unless granted | Custom functions in automations, data transforms, connectors run by operators |
| UI extensions | Sandboxed **iframes** (`sandbox` attribute, separate origin, CSP, postMessage bridge with typed RPC) rendering inside defined slots; or declarative UI schemas rendered by the host using the design system | Browser origin isolation | Panels, custom views; declarative preferred for design-system consistency and accessibility |

All extension calls go through the application layer for authorisation; extensions get their own principal type (like agents) and audit attribution.

### 4.8 File storage, caching, rate limiting

- **Files**: S3-compatible object storage port (MinIO is now in maintenance/licence flux — Garage, SeaweedFS, Ceph RGW as self-host alternatives **(unverified MinIO status)**); keys prefixed by tenant; presigned URLs with short TTL; malware scanning hook (ClamAV/ICAP) for intake mail and uploads; content-type sniffing and `Content-Disposition: attachment` for untrusted types; per-tenant quotas; envelope encryption for BYOK.
- **Caching**: in-process LRU for metadata (field definitions, workflows, permissions) with outbox-driven invalidation; HTTP caching with ETags; optional Valkey (the BSD-licensed Redis fork) for shared rate limits/presence — keep it optional for SH.
- **Rate limiting / fair multi-tenancy**: token bucket per (tenant, principal, route class) and global per-tenant concurrency; cost-based limits for the query DSL and GraphQL-like endpoints; `RateLimit`/`RateLimit-Policy` headers (IETF draft) and `429 Retry-After`; separate budgets for agents/MCP (agents can loop).

### 4.9 Modular monolith vs services

Keep a **modular monolith** with hard module boundaries (hexagonal core already). Each suite area (work management, wiki, whiteboard, ITSM/CMDB, planning, time, goals, dashboards) is a module with its own domain package, ports, tables and published events; cross-module calls via application-layer interfaces or events, never via another module's tables. Enforce with architecture gates (import rules per module). Split into separate deployables only when: (1) different scaling profile (real-time collaboration relay, search indexer, AI workers, mail intake) — these can be separate **processes from the same binary** (`hubtask serve --role=collab`); (2) different failure/security domain (sandboxed plugin runner, AI tool execution); (3) separate team ownership. Self-hosters must still be able to run one binary + Postgres.

### 4.10 API design

| Surface | Recommendation |
|---|---|
| REST (OpenAPI 3.1, contract-first) | Primary; resource-oriented, cursor pagination, sparse fieldsets, `If-Match` optimistic concurrency, idempotency keys (`Idempotency-Key` IETF draft) on POST, RFC 9457 problem details with message codes (rule 8) |
| Webhooks | Standard Webhooks spec (signed with HMAC/Ed25519, `webhook-id`, timestamp, retries with backoff, replay protection), delivery log UI, per-subscription secrets rotation |
| Streaming | SSE or WebSocket for live updates; sync endpoint for offline clients |
| GraphQL | Not needed; buyers rarely require it; it complicates authorisation, rate limiting (query cost) and caching. If desired later, a read-only GraphQL facade over the same application layer |
| Bulk/export | Async export jobs (NDJSON), bulk import with dry-run — doubles as Data Act and DORA exit |
| Standards for interop | iCalendar (RFC 5545) feeds, CalDAV optional; SCIM 2.0 for user provisioning (enterprise must-have); OIDC/SAML; for ITSM: email intake, possibly ITIL-aligned data model; CMDB import (CSV/JSON) |
| Deprecation | `Deprecation`/`Sunset` headers, oasdiff breaking-change gate |

### 4.11 AI agent integration

| Topic | Recommendation |
|---|---|
| **MCP** | Current spec revision **2026-07-28** (stateless HTTP, per-request authorisation) succeeded 2025-11-25 (OIDC discovery, incremental scope consent, client ID metadata documents) — https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization **(details unverified)**. Hubtask's MCP server is an OAuth 2.1 resource server (RFC 9728 protected resource metadata, audience-bound tokens per RFC 8707 resource indicators); no token passthrough to downstream APIs |
| **A2A** (Agent2Agent, Linux Foundation project since 2025) | Optional: expose Hubtask agents as A2A agents (Agent Card) for cross-vendor delegation; lower priority than MCP |
| **Agent identity** | Agents are first-class principals: "agent acting on behalf of user U with scopes S, created by U, limited to hubs H, expires T". Every action audited with both identities. OAuth token exchange (RFC 8693) for on-behalf-of |
| **Tool permissioning** | Per-tool scopes, read vs write vs destructive classification; destructive tools require human confirmation (elicitation) or a policy (CEL) allowing it; per-agent rate and spend limits; dry-run mode |
| **Guardrails against ASI01/LLM01** | Treat all tenant content as untrusted data: content returned by tools is wrapped/marked; no tool can expand its own permissions; agent cannot send outbound messages/webhooks to new destinations without approval; output encoding before rendering (LLM05); egress allow-list for AI provider calls via GuardedClient (rule 6) |
| **RAG isolation** | Embeddings stored per tenant with RLS; retrieval filtered by the requesting user's permissions, not the agent's maximal scope |
| **Evals** | Versioned eval suites per AI feature (golden tasks, adversarial prompt-injection corpus, cross-tenant leakage probes); run on model change (BYO models make this essential: publish a "model qualification" test the operator can run against its own model endpoint) |
| **Observability** | OTel GenAI semantic conventions for spans (model, tokens, latency) — without prompt/response content by default (rule 10), content capture only opt-in into the tenant's own store |
| **Cost/abuse** | Token budgets per tenant/user/agent, circuit breakers for agent loops (ASI08) |
| **AI Act fit** | Transparency badges (Art. 50), no people-ranking (Annex III(4)), human oversight UI, logs → see 1.5 |

---

## Part 5 — Consolidated requirement backlog (priority for 2026–2028)

| Priority | Item | Driven by | Deadline pressure |
|---|---|---|---|
| P0 | Decide CRA role/legal entity structure; publish CVD policy, security.txt, SECURITY.md, advisory process; ENISA reporting readiness | CRA Art. 14 | **Now** (reporting applies since 2026-09-11) |
| P0 | Signed releases (cosign), SLSA L3 provenance, CycloneDX SBOM per artefact, VEX/CSAF feed | CRA Annex I Part II, NIS2 supply chain, Scorecard | Before 2027-12-11, buyers ask now |
| P0 | Secure-by-default install (no default creds, AI/telemetry off, TLS), documented support period and LTS policy | CRA Annex I/II | 2027-12-11 |
| P0 | AI transparency (agent badges, provenance metadata), AI feature switches, "no people ranking" design rule | AI Act Art. 50, Annex III(4) | Art. 50 already applicable |
| P1 | Full tenant export + import, documented formats, self-service | Data Act, DORA exit, GDPR Art. 20 | Applicable now; zero charges 2027-01-12 |
| P1 | WCAG 2.2 AA conformance programme + ACR; keyboard alternatives for drag (boards, whiteboard, Gantt) | EAA/BFSG, public procurement, EN 301 549 v4.1.1 | v4 citation ~Dec 2026 |
| P1 | Retention classes + legal hold + locked periods for time/cost data; deletion concept consistent with GDPR | GoBD, GDPR | Before time-tracking ships |
| P1 | Control mapping (ISO 27001 / C5:2025 / NIS2 IR 2024/2690 / CRA) and operator "compliance kit" (RoPA, DPA/TOMs, DPIA kit, shared-responsibility matrix) | Providers' certifications | Continuous |
| P1 | SSO (OIDC/SAML), SCIM, MFA/passkeys, audit log export, security event log | NIS2 Art. 21, ISO 27001, enterprise RFPs | Continuous |
| P2 | Envelope encryption with KMS port, BYOK, crypto-shredding; crypto per TR-02102 incl. hybrid PQ TLS | C5, DORA, enterprise | 2027 |
| P2 | Cell architecture and data residency; per-tenant restore and move | SaaS scale, DORA, sovereignty | When first provider scales |
| P2 | ASVS 5.0 L2 traceability, annual pen test, fuzzing in CI, isolation fuzzer | ASVS, OWASP, buyers | Continuous |
| P2 | Extension model: webhooks (Standard Webhooks), OAuth apps, wazero sandbox, iframe UI slots | Suite extensibility | With marketplace |
| P3 | QTSP signature connector, EUDI wallet login | eIDAS 2 | Only on demand |

## Sources (selection)

- CRA: https://eur-lex.europa.eu/eli/reg/2024/2847/oj ; https://www.matheson.com/insights/the-eu-cyber-resilience-act-recap-reporting-obligations/ ; https://www.noze.it/en/insights/cra-commission-guidance/ ; https://www.noze.it/en/insights/cyber-resilience-act-sbom/ ; https://best.openssf.org/CRA-Brief-Guide-for-OSS-Developers ; https://www.opensourceforu.com/2026/07/eu-clarifies-cra-rules-for-non-commercial-open-source/ ; https://ospo-alliance.org/faq/security/12-cyber-resilience-act-open-source-impact/ ; https://www.greenbone.net/en/blog/cra-open-source-software/
- AI Act / Omnibus: https://www.gibsondunn.com/eu-ai-act-omnibus-agreement-postponed-high-risk-deadlines-and-other-key-changes/ ; https://labs.cloudsecurityalliance.org/research/csa-research-note-eu-ai-act-high-risk-deadline-omnibus-20260/ ; https://secureprivacy.ai/blog/eu-ai-act-digital-omnibus-the-new-high-risk-ai-deadlines-after-council-approval
- NIS2UmsuCG: https://www.usd.de/nis2umsucg-offiziell-in-kraft/ ; https://www.secjur.com/blog/nis2-umsetzungsgesetz ; https://www.advisori.de/blog/nis2-umsetzungsgesetz-nis2umsucg
- Digital Omnibus (data): https://ppc.land/digital-omnibus/ ; https://eaccny.com/?p=57747
- PLD: https://www.mtrlegal.com/en/bundestag-adopts-new-product-liability-law/ ; https://www.aoshearman.com/en/insights/update-on-product-liability-law ; https://www.insideprivacy.com/uncategorized/eu-member-states-begin-rolling-out-new-product-liability-rules/
- Accessibility: https://nda.ie/news/en301549-published ; https://www.deque.com/blog/en-301-549-v4-1-1-is-final-what-changed-what-it-means-and-what-you-should-do/ ; https://www.dwt.com/insights/2026/09/european-accessibility-act-ict-standards-update
- C5: https://www.roedl.com/en/insights/bsi-c5-establishing-itself-as-a-cross-industry-standard-for-cloud-security/ ; https://www.bsi.bund.de/C5 (verify)
- OWASP: https://owasp.org/Top10/2025/0x00_2025-Introduction/ ; https://blog.qualys.com/qualys-insights/2026/06/15/what-changed-in-owasp-top-10-2025-and-recommendations-for-each-category ; https://www.giskard.ai/knowledge/owasp-top-10-for-agentic-application-2026 ; https://owasp.org/www-project-application-security-verification-standard/
- Supply chain: https://slsa.dev/blog/2025/11/announce-slsa-v1.2 ; https://cyclonedx.org/news/cyclonedx-v1.7-released ; https://www.bsi.bund.de/dok/TR-03183 ; https://www.bsi.bund.de/dok/TR-02102
- MCP: https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization ; https://modelcontextprotocol.io/specification/2025-11-25/changelog
- DORA: https://eur-lex.europa.eu/eli/reg/2022/2554/oj ; NIS2 IR: https://eur-lex.europa.eu/eli/reg_impl/2024/2690/oj ; Data Act: https://eur-lex.europa.eu/eli/reg/2023/2854/oj ; AI Act: https://eur-lex.europa.eu/eli/reg/2024/1689/oj
