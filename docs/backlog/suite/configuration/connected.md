# A connected suite — every object reachable from every tool

The owner's addition (2026-10-28): the tools must work together wherever it makes sense — a task or
ticket inserted into a wiki page or onto a whiteboard, a board shown inside a page — a suite with
well-designed synergies, not modules side by side.

## 1. The rule (principle P-19)

> **P-19: Everything is an object you can point at, and every view of it is the same view.** Every
> entry, page, record, view, dashboard, goal, sprint and whiteboard has one address and one
> reference type. Any tool that holds text or space — a page, a whiteboard, a comment, a form's help,
> a status update — can mention it, embed it live, or turn its own content into it. An embedded view
> is the view itself, with the reader's permissions, not a copy.
>
> *Broken when* a tool shows a copy that goes stale; an embed shows the reader something their role
> would not; a tool can show entries but not the board everybody else uses; two tools have two ways
> of linking.

It follows from P-18: the *view* is one building block, so embedding a board in a page is the same
block drawn in a smaller frame; the *form* is one block, so creating a task from a whiteboard sticky
note opens the same form.

## 2. One reference, three strengths

| Strength | What it is | Example |
|---|---|---|
| **Mention** | an inline link that shows the object's live title, key, status and colour | `#SUP-128` in a comment, `@Website-Relaunch` in a page |
| **Card** | the object as a card with its key fields, actionable where the reader may act (tick, take a step, assign) | a task card on a whiteboard; a goal card in a status update |
| **Live view** | a saved view or dashboard widget drawn in place, with its layout (list, board, timeline, Gantt, chart) | the sprint board inside the sprint review page; the SLA chart in the weekly report |

All three are the same reference type in the data model, carry no copy of the object, and are
resolved for the reader (P-05, rule 2): what the reader may not see is shown as "not available to
you", never leaked by title.

## 3. Conversions — turning one thing into another, in both directions

| From | To | Keeps |
|---|---|---|
| a checklist line in a page | a task (the type's form) | a link both ways; ticking either ticks both |
| a sticky note on a whiteboard | a task, a ticket, a risk | the note becomes the card of the new object |
| a comment or a mail in the jumble | a ticket, a task, a page | provenance (`originJumbleEntryId` exists today) |
| a meeting-notes page | its action items as tasks, its decisions as a decision record | links back to the meeting |
| a selection of entries | a page (report), a whiteboard cluster | live cards, not text |
| a page | a template | its structure, placeholders |

## 4. Synergies the suite designs in, not leaves to integrations

| Synergy | Tools | What the person gets |
|---|---|---|
| Sprint review page | pages + sprints + views | the template drops the sprint's done board, burndown and goal into the review page |
| Retrospective | pages + whiteboard + tasks | a whiteboard with three columns; voting; action items become tasks in the next sprint |
| Incident → problem → knowledge | service + pages | resolving an incident offers "write the known error"; the article links back and is suggested in the portal |
| Change calendar | service + planning + calendar | changes, releases and freeze windows in one calendar view, also on the Gantt |
| Project status report | goals + Gantt + RAID + pages | a report page with live milestone, risk and budget cards; sent on a schedule |
| OKR check-in | goals + views | key results fed by views, the check-in page shows the work behind each number |
| Workshop to plan | whiteboard + planning | clustered notes become epics with tasks, placed on the roadmap |
| Onboarding | templates + approvals + pages | one template creates tasks for IT, HR and the team lead and a welcome page with live checklists |
| Search and palette | everything | one search finds entries, pages, records, views, settings |
| AI and agents | everything | one context: an agent reads the page, the linked tickets and the board through the same references and permissions |

## 5. What it asks of the building blocks

* **One reference type and one resolver** in the domain, used by pages, whiteboards, comments,
  notes, forms and rules; backlinks computed once.
* **Views render at any size**: the board, list, timeline, Gantt and chart components work in a
  page block, a whiteboard frame, a dashboard tile and full screen (design system).
* **Cards are the same component everywhere** — row, board card, whiteboard card, embed card — with
  the same anatomy (G8 of the approved system concept: every field visible and editable the same
  way everywhere).
* **Offline**: references and embeds resolve from the device's copy where it holds the object.
