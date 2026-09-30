---
id: UC-AI-03
title: Decide whether our content may be sent to a model, and take it back
context: suggestion
actors: [PE-owner, PE-admin, PE-person]
deployments: [D1, D2, D3, D4, D5, D6]
serves: [P-14, P-11, P-12]
state: built
tasks: [J-01, J-02]
checked_by: [core/application/service/integration/AiProviderConfig_test.go]
---

# Decide whether our content may be sent to a model, and take it back

## Goal

Nothing of a workspace's content is sent to any model — its own or the provider's — until the
workspace itself consented, knowing who processes it and where; taking consent back stops it at once.

## Story

Beside the provider: *Send this workspace's content to {provider} ({where})*, off. Switching it on
is the consent; switching it off stops every call before the next one.

## How to check

1. Consent is off until an owner or administrator of the workspace switches it on; it is never on
   because of a default, an installation setting or a plan.
2. The consent control names the processor and where it processes content, in words.
3. Every call to a model checks consent first; with consent off, suggestions are refused with the
   "AI unavailable" sentence and nothing is sent.
4. Giving and withdrawing consent are in the workspace's trail.

## Where it ends

* Consent is per workspace, not per person; a person who does not want AI suggestions simply does
  not ask for them.
* Consent is never given by a provider or a plan ([NG-ai-consent-by-default](../../vision/non-goals.md)).
