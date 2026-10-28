# Module 4 — Intake, service desk and approvals (`UC-FRM`, `UC-SVC`, `UC-APR`)

> **Background** — the concept of 2026-10-17; where it differs from [`../products.md`](../products.md),
> that file wins ([`README.md`](./README.md)).

## 1. Goal

People outside a team ask for something in a form that fits the request; the team works the
requests from queues against the response times it promised; and a step that needs somebody's
yes — a purchase, a leave, a change — waits for it, with the yes in the trail.

## 2. Parity

| Capability (market) | Hubtask today | Proposed |
|---|---|---|
| Form builder: field types, conditional sections, branching, file upload | Jumble (mail, webhook, quick capture, API) | **Forms**: fields from the target type's field scheme, conditions (CEL) show/hide sections, a submission becomes an entry (via the jumble or directly) |
| Public forms, guest forms, embedded forms | — | A form is internal, guest-only or public (public only if the workspace allows; rate-limited, bot-protected without third-party CAPTCHA — `NG-phone-home`) |
| Request types per service, portal grouping | — | `RequestType(form, itemType, queue, sla)` |
| Customer portal: my requests, status, reply | — | A portal route area of the same web app (`NG-second-app`): requesters see only their requests (`PE-requester`, `GUEST`-like role) |
| Email channel, reply by mail | Mail into the jumble | Mail threads bound to a request; replies by mail land as comments (signed reply addresses) |
| Queues with filters, assignment, skills | Saved views, auto-assign (round robin, least loaded) | A queue is a saved view plus an assignment policy; skills = groups |
| SLAs: time to first response, time to resolve, pause conditions, business hours, holidays | — | `Sla(goals[], startOn, pauseOn, stopOn [status categories / CEL], calendarId)`; `BusinessCalendar(hours, holidays, timeZone)` (shared with resources) |
| SLA breach warnings and escalations | Rules (RELATIVE_DATE) | SLA events (`…sla.at_risk.v1`, `…sla.breached.v1`) as rule triggers |
| Canned responses, macros | — | Reply templates; a macro is a manual rule |
| Knowledge base deflection | — | The portal suggests pages of a published space (knowledge module) before submitting |
| CSAT after resolution | — | A one-question survey on resolution, results per queue |
| Incident / problem / change (ITSM) | — | Shipped workflow templates for incident, problem, change (workflow module), with approvals for change |
| Assets / CMDB | — | Records module (`UC-REC`) linked to requests |
| Approvals: single, any-of, all-of, sequence; delegation; due dates | — | `ApprovalStep` as a transition guard: approvers (people, groups, the requester's manager field), rule (`ANY`, `ALL`, `N_OF`), deadline, delegate while absent |
| Approve by mail / mobile | — | Approve from the notification (signed link → the app, the approver's own sign-in; `NG-magic-link` holds: the link opens, it does not authenticate) |

## 3. Use cases (titles)

| ID | Title |
|---|---|
| FRM-01 | Build a form that creates the right kind of entry |
| FRM-02 | Let people outside the workspace submit a form |
| FRM-03 | Show only the questions that apply |
| SVC-01 | Offer our services in a portal where people ask and follow up |
| SVC-02 | Work requests from a queue |
| SVC-03 | Promise response times and see which promise is about to break |
| SVC-04 | Answer a requester by mail, and get their reply on the request |
| SVC-05 | Point a requester to an answer before they ask |
| SVC-06 | Learn whether the requester was satisfied |
| APR-01 | Make a step wait for somebody's yes |
| APR-02 | Approve or decline, with a reason, wherever I am |
| APR-03 | Hand my approvals to someone while I am away |

## 4. Principles, rules, where it ends

P-01 (a public form never reveals whether an address has an account), P-02 (approval links never
sign in), P-05, P-11 (an SLA clock the handler can see), P-16. Rule 10 (form answers are user
content), rule 6 (outbound mail through the existing guarded paths). Data catalogue: requesters
without an account are personal data — catalogue entry, retention, erasure path.

Ends: no telephony, no live chat widget (`NG-chat`), no full CMDB discovery.
