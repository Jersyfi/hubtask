# Industry standards on Hubtask's building blocks

A standard is never a module of its own. It is a **template** on the building blocks of
[`model.md`](./model.md) §2, so a customer who follows ITIL and one who follows nothing use the same
product, and a customer who follows ITIL "mostly" changes the template instead of fighting it.
Evidence and sources: [`research-admin-2026-10.md`](../research/admin-2026-10.md) §3.

## 1. The primitives the standards need, and where Hubtask stands

| # | Primitive | Hubtask today | In the suite concept |
|---|---|---|---|
| P1 | Typed entries with type-specific fields | 3 fixed levels, 8 field kinds | types per level, computed and relation fields (M1) |
| P2 | Statuses with categories, steps with conditions | completion + columns; *Flow* gate | workflows (M1) |
| P3 | Typed links (blocks, caused by, duplicates, affects CI) | parent/child only | links and dependencies (M2) |
| P4 | Registers — records that are not work | — | record types (M8, decision D-07) |
| P5 | Time boxes — sprints, stages, PIs, quarters | — | sprints, stages, releases (M2), goal periods (M6) |
| P6 | Estimates and actuals with roll-up | amounts on lists | estimate field, time entries (M2, M5) |
| P7 | Clocks and SLAs with business calendars | — | calendars, SLAs (M4) |
| P8 | Approvals — single, quorum, sequential, a group (CAB) | — | approval steps (M4) |
| P9 | Rules | ✓ six triggers, CEL, dry run, replay | + trigger "step taken", SLA events |
| P10 | Flow metrics | *Flow* figure (throughput, time first → last column) | from status history (M6) |
| P11 | Baselines | — | M2 |
| P12 | Thresholds and tolerances raising an exception | WIP limit as advice | tolerances on a place, inherited and lockable like settings (new, M2) |
| P13 | Intake catalogue | jumble, mail intake, ticket keys | the type's portal channel (M4) — no separate form |
| P14 | Knowledge with review dates | — | pages (M3) |
| P15 | Audit trail and retention | ✓ beyond the market | ✓ for every block and every configuration version |
| P16 | Named accountabilities per place | roles at scopes | custom roles (M7) bound per hub — *Product Owner*, *Change manager*, *Risk owner* |

P12 is the one primitive the suite concept did not yet name; it is added to module 2.

## 2. The standards, as templates

| Standard | Template(s) | What the template sets up | Primitives |
|---|---|---|---|
| **ITIL 4 / ISO/IEC 20000-1** | *ITIL service desk* (incident, service request, problem, change, knowledge), *Service catalogue*, *CMDB* | impact × urgency → priority; response and resolution SLAs on business hours; major incident flag; problem linked to incidents; change types standard / normal / emergency with risk and **CAB approval**, change calendar and freeze windows; CIs with relations; knowledge lifecycle; continual-improvement register | P1–P4, P7, P8, P9, P13, P14, P16 |
| **Scrum Guide 2020** | *Scrum team* | accountabilities as roles (Product Owner orders the backlog); Product Goal on the backlog, Sprint with Sprint Goal, **Definition of Done** as the checks of the step into *Done*; planning, review and retrospective as page templates with action items; velocity and burndown | P1, P2, P5, P6, P10, P14, P16 |
| **Kanban (both guides)** | *Kanban team* | columns mapped to status categories; WIP limits per column and person; **explicit policies** shown on each column and enforced as step checks; classes of service; Service Level Expectation against work-item age; blocked flag with reason | P2, P7, P10, P12 |
| **SAFe 6 / LeSS / Scrum@Scale** | *Scaled agile* | a hub per train, PI as the time box above sprints, PI objectives with business value, ART board (features × teams × iterations with dependencies), ROAM risk register, WSJF as computed field; LeSS: one backlog collection for many teams | P3, P4, P5, P6, P10 |
| **PRINCE2 7 / PMBOK 7 / ISO 21502** | *Classic project*, *Hybrid project* | management stages, **tolerances** per level raising exceptions, issue / risk / quality registers and lessons log, change control with a change authority, product breakdown with descriptions, baselines, highlight and end-stage reports generated from data; WBS on the five levels | P3, P4, P5, P8, P11, P12 |
| **Portfolio (MoP), OKR** | *Portfolio*, *OKR* | portfolios over hubs with health and review gates; objectives with key results fed by views, confidence, quarters, alignment | P4, P5, P6, P8 |
| **RAID** | part of every project template | risks, assumptions, issues, dependencies as four record types with owner, score, response, review date | P4 |
| **ISO 9001 §10.2 (CAPA)** | *Nonconformity and corrective action* | containment → root cause → actions → **effectiveness check** after a delay (a rule with a `RELATIVE_DATE` trigger on the completion or a date field) → closure; controlled documents with approval | P1, P2, P3, P8, P9, P15 |
| **ISO/IEC 27001:2022** | *Risk and controls* | risk assessment and treatment plan; Statement of Applicability over the 93 Annex A controls as a register with evidence links; recurring internal audit and management review | P3, P4, P9, P15 |
| **GDPR** | *Records of processing*, *Data subject requests* | Art. 30 register; DPIA as a workflow; Art. 15–22 requests with a one-month clock (Hubtask's own privacy context already does this for its own data); breach within 72 hours | P4, P7 |
| **DORA metrics** | part of *Bug tracking* and *ITIL* with code links | deployments and changes from the forge, incident ↔ change links, the four keys as dashboard | P3, P10 |
| **Lean A3 / PDCA** | *A3 problem solving* | one structured page with child actions; PDCA as a status set | P2, P14 |

## 3. What "aligned with a standard" promises — and what not

* A template **implements the practices' records and flows**; it names the standard and the version
  it follows, and its guide page says what the organisation still has to decide itself.
* Hubtask never claims a certification or compliance of the customer (`NG`-spirit of
  `enterprise.md`'s control mapping): it supports the evidence, the organisation is audited.
* A template is updated when its standard is revised (ITIL and PMBOK are being revised as of 2026 —
  unverified in the research); the update reaches customers as an *Update available*, never
  silently (`templates.md` §3).
