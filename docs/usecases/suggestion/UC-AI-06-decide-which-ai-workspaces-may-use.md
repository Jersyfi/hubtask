---
id: UC-AI-06
title: Decide as a provider which AI our workspaces may use
context: suggestion
actors: [PE-operator, PE-platform]
deployments: [D4, D5, D6, D7]
serves: [P-06, P-07, P-08, P-14, P-15]
state: specified
tasks: [SC-11, SC-12]
checked_by: []
---

# Decide as a provider which AI our workspaces may use

## Goal

An operator offers one or more AI models to every workspace, and decides once — for the whole
installation, and later per plan — whether workspaces may use only the offered model, only their
own, either, or none; and the minimum a workspace's own model must promise about where it processes
content.

## Story

*Installation → AI → Offer a model*: display name, kind, address, models, key, where it processes
content. Under *Defaults → AI*, one choice: *Workspaces may use* — "the offered model", "their own
model", "either", "no AI" — with *Workspaces may* "narrow it" or "not change it". And *Their own
model must process content* — "anywhere allowed", "in the EEA or with adequacy", "in the EEA",
"on their own machine". A B2C provider offers Mistral (EU), chooses "the offered model" and locks
it. A B2B provider offers the same model, chooses "either" and requires "in the EEA" for own models.

## How to check

1. The operator can offer, change and withdraw installation models from the screen, `hubctl admin ai`
   and the instance file; the key is sealed and re-sealed with the installation's keys.
2. *Workspaces may use* resolves installation → plan → workspace like every other default: a
   workspace may narrow it ("either" → "only our own", or → "no AI") where it is not locked, and can
   always choose "no AI".
3. A workspace's AI screen offers exactly the sources the rule in force allows; a source it may not
   use is not shown ([P-05](../../vision/principles.md)).
4. An own model whose declared processing location is weaker than the minimum is refused with a
   sentence naming the minimum and who set it.
5. Narrowing the rule never deletes a workspace's own configuration: a source that is no longer
   allowed stops being called, its settings stay sealed, and it works again when the rule allows it.
6. The per-workspace AI budget applies to the offered model; an own model is limited only by the
   workspace's own optional budget.
7. No setting at any level switches consent on; consent stays UC-AI-03.
8. In a private installation (D1) none of this appears: there is no "offered model", only the
   workspace's own, as today.

## Where it ends

* Hubtask does not bill AI use; it counts it and makes the count readable
  ([NG-billing](../../vision/non-goals.md)).
* No per-model price or routing by cost.
* Third-country transfer stays the operator's environment decision
  (`HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER`).

## Today

* Check 1: not met — the operator cannot offer installation models from any door, tracked in #1067.
* Check 2: not met — there is no *Workspaces may use* rule at any level, tracked in #1067.
* Check 3: not met — a workspace's AI screen offers its own provider regardless of any rule, tracked in #1067.
* Check 4: not met — there is no minimum processing location to refuse an own model against, tracked in #1067.
* Check 5: not met — there is no rule whose narrowing could stop a source, tracked in #1067.
* Check 6: not met — there is no offered model the budget could apply to, and no own optional budget, tracked in #1068.
