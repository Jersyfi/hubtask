---
id: UC-AI-04
title: Get suggestions and decide on each one
context: suggestion
actors: [PE-person, PE-member, PE-agent]
deployments: [D1, D3, D4, D5, D6]
serves: [P-03, P-11, P-14]
state: built
tasks: [J-03, J-04, J-05, J-06, J-07]
checked_by: [core/application/service/suggestion/Producing_test.go, core/application/service/suggestion/Actions_test.go, core/application/service/suggestion/Duplicates_test.go]
---

# Get suggestions and decide on each one

## Goal

A person asks for help — fields for a task, a breakdown into work packages, a template, what a jumble
entry should become, possible duplicates — and gets suggestions they accept, change or dismiss; a
suggestion never changes anything by itself.

## Story

On a task, *Suggest* proposes a due date and labels; *Break down* proposes work packages. The
suggestion appears beside the task, marked as a suggestion. *Apply* makes it real, *Dismiss*
removes it. If the task changed meanwhile, the suggestion says it is out of date.

## How to check

1. A suggestion is stored separately and changes nothing until a person applies it.
2. Applying goes through the ordinary operation with the person's own rights; what they may not do,
   the suggestion cannot do either.
3. A suggestion made for an entry that has since changed says it is stale and can still be
   dismissed.
4. Suggestions are queued; a slow or unavailable model never blocks the screen, and "unavailable"
   is said, not shown as loading.
5. The trail records that a suggestion was applied, as an AI-assisted change by the person.

## Where it ends

* No autonomous agent that acts without a person applying the result.
* No chat assistant inside the product; agents come through MCP.
