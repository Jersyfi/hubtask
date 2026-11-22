---
name: design-brief
description: Write the one-screen design brief that a structural change needs before any code. Use it before a migration that changes an existing table, a new or changed port, a change to the sync protocol or a merge rule, a change to authorisation, a new door, a contract change to an existing operation, work across two or more bounded contexts, or any concept.
---

# Design brief

A structural change is built only after its brief stands in the draft pull request. The brief is
the Definition of Ready (`engineering-guidelines.md` §2) answered for this change. It is one
screen long.

## When a change is structural

Any one of these makes it structural:

- a migration that changes an existing table;
- a new or changed port;
- the sync protocol or a merge rule;
- authorisation;
- a new door (web app, REST, MCP, `hubctl`, automation, background worker, installation level);
- a contract change to an existing operation;
- work across two or more bounded contexts;
- a concept built whole.

A fix or a small feature outside this list needs no brief.

## What the brief holds

Write these six parts into the description of the draft pull request, under the steps:

1. **What changes for a person.** The use case checks the change makes true or alters.
2. **The impact map.** One line for each of the seventeen points of `engineering-guidelines.md` §2:
   "touched:" and how, or "not touched:" and why.
3. **The precedent.** The existing code that does the nearest thing, by path.
4. **One alternative**, and why it was not taken.
5. **What is unknown**, and how each unknown is resolved before anything is built on it.
6. **The order of the build.**

## How to write it without spending the builder's context

1. Start a read-only research agent. Give it the task, the use cases and the list above. It reads
   the code and the subject documents in its own context and returns one page: the impact map and
   the precedent.
2. Write the brief from that page.
3. Start a fresh agent that has seen neither conversation. Give it the brief and one question:
   "What makes this fail?" It answers once.
4. Change the brief where the answer is right, and say in one line where it is not. There is no
   second round.

## Who sees it

The owner sees a brief only when it touches an item of `AGENTS.md`, "What you do not decide
yourself"; then follow `.agents/skills/ask-the-owner/SKILL.md`. Every other brief is read by the
independent review, which reports a structural change without a brief, and a build that left its
brief, as findings.
