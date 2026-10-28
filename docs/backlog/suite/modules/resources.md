# Module 5 — Time & resources (`UC-TIM`)

> **Background** — the concept of 2026-10-17; where it differs from [`../products.md`](../products.md),
> that file wins ([`README.md`](./README.md)).

## 1. Goal

People record the time they spend where the work is; a lead sees who has room before handing out
more; a project knows what it has cost against its budget — and none of it becomes an invoice.

## 2. Parity

| Capability (market) | Proposed |
|---|---|
| Timer on an entry, manual entries, timesheet week grid | `TimeEntry(itemId?, accountId, startedAt, duration, note, billable)`; a running timer per person, synced across devices |
| Timesheet submission and approval, locking periods | Weekly submission → approval (approval steps from module 4); an approved period is locked |
| Estimates vs actuals, remaining | Planning's `estimate` + summed time; roll-up through the levels |
| Capacity per person (hours per day, part time) | `Capacity(accountId, weeklyHours, from)` |
| Absences, public holidays | Absences per person; holidays from the shared `BusinessCalendar` |
| Workload view (heatmap per person per week) | A workload layout over assigned estimates, against capacity |
| Resource allocation / placeholders (roles not yet staffed) | Placeholder assignees ("a designer") on plan items |
| Cost rates, budgets (hours, money), burn | Rates per person or role (a number and a currency; no price list of Hubtask itself), budget per hub or collection, burn chart |
| Billable / non-billable, export to accounting | Flag + export (CSV, API); `NG-invoicing` |
| Time tracking from calendar / integrations | Through the API and CalDAV |

## 3. Use cases (titles)

TIM-01 Track my time on what I work on · TIM-02 Fill in and hand in my week ·
TIM-03 Approve a team's time · TIM-04 See who has room before handing out work ·
TIM-05 Record absences and holidays once for every plan · TIM-06 Keep a project within its
budget · TIM-07 Hand time and cost to accounting.

## 4. Principles, where it ends

Time entries are personal data about employees: data catalogue, purpose limitation, and in many
EU companies a works-council matter — the module must allow *aggregated only* views (a lead sees
team load, not a person's minutes) as a workspace setting. P-14 (no AI scoring of people). P-07.

Ends: no payroll, no attendance clocking (working-time law records are a different purpose), no
invoices.
