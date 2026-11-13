---
id: UC-WRK-34
title: See my day
context: work
actors: [PE-person, PE-member, PE-child]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-10, P-12]
state: built
tasks: [UX-15]
checked_by: [test/integration/query_group_queue_test.go, apps/webapp/e2e/today.test.mjs, apps/webapp/e2e/todaybands.test.mjs, apps/webapp/src/lib/today.test.ts, apps/webapp/e2e/looks.test.mjs]
---

# See my day

## Goal

The first page a person sees answers what is theirs today across every hub, and lets them act on it without going anywhere.

## Story

On *Today* the person sees two tasks due this morning, the shopping list as a tile with five items, the next step of the renovation and that the jumble holds two arrivals. They tick one task right there.

## How to check

1. *Today* is the page the app opens on; it lists entries due today or overdue that are on the person, across hubs, each with its hub's colour, and ticking one there completes it.
2. Collections whose figure is *open* are shown as one tile with the figure, not entry by entry.
3. Collections with progress show their next due step.
4. The jumble shows how many arrivals wait and the first three.
5. Recently opened places on this device are listed.
6. What is overdue is said as overdue, without blame: the glance counts it, its band is *Overdue* and its rows carry the overdue mark with the date; no "you missed" anywhere. (Changed 2026-10-24 by the product owner: the person wants to see at once what is late.)
7. The page greets the person by the time of their day and says at a glance, as a row of cards, how much is overdue and due today and, where their products are on, what needs their reply, their time today and what waits for their yes; each card's one act leads to those entries. (The glance became the work-surface concept's row of cards, §6.1.)
8. Where a product is on, *Today* shows its band (*Needs your answer*, *Waiting for your yes*, *Time today*, *Sprint*) fed by the product's own reads, and none where it is off. *Needs your answer* lists the open desk entries with an SLA target running that the reader holds, or that nobody holds yet in a collection whose auto-assign policy names a group the reader belongs to (by the owner's seventh answers, item 6). (Added 2026-10-31 by the product owner's decision to build the user-experience direction, `user-experience.md` §3.3 and §9.)

9. *Sections* lets the person hide and order *Today*'s cards (overdue, due today, needs your reply, time today, waits for your yes) and tiles (your collections, the inbox, recently opened); the arrangement is the account's and follows the person to another device, and a card or tile their roles or modules do not give is neither shown nor offered. (Added 2026-11-12 by the product owner's decision E-17 of the work-surface concept.)

## Where it ends

* The page is the person's own; nobody else's day is shown.
* The person may hide and order the cards and tiles, nothing more: *Today* does not become a free dashboard - no section of one's own, no section another person sees (work-surface concept E-17).
* The detailed lists stay [UC-WRK-20](./UC-WRK-20-see-what-is-on-me-when-i-arrive.md).
