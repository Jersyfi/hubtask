---
id: UC-AI-02
title: Connect our workspace's own AI model
context: suggestion
actors: [PE-owner, PE-admin]
deployments: [D1, D3, D4, D6]
serves: [P-06, P-09, P-14]
state: partial
tasks: [J-01, J-02]
checked_by: [core/application/service/integration/AiProviderConfig_test.go]
---

# Connect our workspace's own AI model

## Goal

A workspace connects the model it trusts — a local Ollama, an EU endpoint, a company's own
gateway — declares where it processes content, and keeps the key sealed.

## Story

*Administration → AI*: kind (OpenAI-compatible or Ollama), address, models for text and for search,
the key, and *Where does it process content?* (on our own machine, in the EEA, in a country with an
adequacy decision, elsewhere). The key is never shown again; the screen says only that one is
stored.

## How to check

1. The provider can be set, changed and removed from the screen, `hubctl` and the API.
2. The key is sealed on the way in and never answered again; the screen says "a key is stored".
3. A provider declared as processing outside the EEA without an adequacy decision is refused unless
   the operator allowed such transfers for the installation, and the refusal says so.
4. The declaration is shown wherever the workspace consents to processing (UC-AI-03).
5. Where the installation or plan allows only the provided model, this screen offers no own
   provider (UC-AI-06).

## Where it ends

* Hubtask does not verify the declaration; it records it ([ADR-0018](../../adr/ADR-0018-privacy-by-design.md)).
* No vendor-specific SDKs ([ADR-0049](../../adr/ADR-0049-ai-provider-surface.md)).

## Today

* Check 5: not met — nothing lets the installation or a plan allow only the provided model (UC-AI-06), so every workspace's screen offers its own provider, tracked in #1067.
