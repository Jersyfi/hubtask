---
id: UC-AI-05
title: Use the AI model our provider offers, without setting anything up
context: suggestion
actors: [PE-person, PE-owner, PE-admin]
deployments: [D5, D6]
serves: [P-10, P-12, P-14]
state: specified
tasks: [SC-11]
checked_by: []
---

# Use the AI model our provider offers, without setting anything up

## Goal

A customer of a hosted Hubtask — a single consumer or a company — switches on AI suggestions with
one decision: "Use AI from {provider}". No address, no model name, no key. They see who processes
their content and where before they agree, and they can switch it off again at any time.

## Story

Anna signs up for a provider's Hubtask. *Settings → AI* says: "Your provider offers AI suggestions
through *Mistral Large (EU)*. Your content is sent to Mistral AI in the EEA when you ask for a
suggestion. [Use it]". One click — the consent — and *Suggest* appears on her tasks. A company on
the same installation that is allowed to bring its own sees two choices: the provider's model, or
its own (UC-AI-02).

## How to check

1. When the installation offers a model and the workspace may use it, the AI screen shows it by its
   display name with processor and jurisdiction, and one switch to use it.
2. Switching it on is the workspace's consent; nothing is sent before it, and switching it off stops
   the next call.
3. The workspace never sees the provider's key, address or model configuration.
4. Suggestions, search by meaning and every other AI feature work with the offered model exactly as
   with an own one.
5. Use of the offered model counts against the workspace's AI budget, and the workspace sees its use
   and the budget.
6. Where the installation offers several models, the workspace picks one by name.
7. Where the workspace may use both, it switches between offered and own on the same screen; its own
   configuration is kept while the offered model is in use, and the other way round.

## Where it ends

* A provider cannot switch AI on for a workspace ([NG-ai-consent-by-default](../../vision/non-goals.md)).
* No per-person AI choice inside a workspace.
* Changing the model used for search by meaning re-indexes the workspace in the background; the
  screen says so and search by words keeps working meanwhile.

## Today

* Check 1: not met — an AI provider exists only per workspace; the installation offers no model, tracked in #1067.
* Check 2: not met — there is no offered model to switch on, tracked in #1067.
* Check 3: not met — there is no offered model whose configuration could be withheld, tracked in #1067.
* Check 4: not met — no AI feature runs with an offered model, tracked in #1067.
* Check 5: not met — no offered model's use is counted, tracked in #1067.
* Check 6: not met — the installation cannot offer several models, tracked in #1067.
* Check 7: not met — there is no offered model to switch to, tracked in #1067.
