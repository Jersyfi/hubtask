---
id: UC-WRK-34
title: See my day
context: work
actors: [PE-person, PE-member, PE-child]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-10, P-12]
state: verified
tasks: [UX-15]
checked_by: [apps/webapp/e2e/today.test.mjs, apps/webapp/src/lib/today.test.ts, apps/webapp/e2e/looks.test.mjs]
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
7. The page greets the person by the time of their day and says at a glance how much is due today, how much is coming up, what waits in the jumble and what is open on them, each figure a link to those entries.

## Where it ends

* The page is the person's own; nobody else's day is shown.
* The detailed lists stay [UC-WRK-20](./UC-WRK-20-see-what-is-on-me-when-i-arrive.md).
