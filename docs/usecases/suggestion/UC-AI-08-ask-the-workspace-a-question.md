---
id: UC-AI-08
title: Ask the workspace a question
context: suggestion
actors: [PE-person, PE-member]
deployments: [D1, D3, D4, D5, D6]
serves: [P-05, P-12, P-14]
state: specified
tasks: [UX-22]
checked_by: []
---

# Ask the workspace a question

## Goal

A person asks in their own words — "What is open for Müller GmbH this week?" — and gets one answer with the entries it rests on, without building a filter.

## Story

In the palette the person types the question and chooses *Ask*. A proposed answer appears with three entries as its sources, the filter it implies as chips, and *Save as view*.

## How to check

1. With AI consented to, the palette offers *Ask* for a typed question; without it the action is not shown ([UC-AI-01](./UC-AI-01-work-fully-without-ai.md)).
2. The answer is headed as a proposal, lists the entries it rests on as links, and has its provenance behind one collapsed line.
3. Only entries the person may read are used; content of people who keep theirs out of AI is not.
4. Where the question is a filter, the filter is shown as chips that can be changed, and *Save as view* saves it.
5. Each question stands alone: there is no conversation history, no follow-up turn, and the answer performs no action.

## Where it ends

* No assistant that acts (decision #1628).
* No answers from outside the workspace.

## Today

* Check 1: not met — not built yet, tracked in milestone UX.
* Check 2: not met — not built yet, tracked in milestone UX.
* Check 3: not met — not built yet, tracked in milestone UX.
* Check 4: not met — not built yet, tracked in milestone UX.
* Check 5: not met — not built yet, tracked in milestone UX.
