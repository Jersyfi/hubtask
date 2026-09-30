---
id: UC-AI-01
title: Use all of Hubtask with AI switched off
context: suggestion
actors: [PE-person, PE-member, PE-owner, PE-selfhoster]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-09, P-14]
state: built
tasks: [J-01, J-17]
checked_by: [core/application/service/suggestion/Suggestions_test.go]
---

# Use all of Hubtask with AI switched off

## Goal

A person or a whole installation that never configures AI — or switches it off — has every feature
of Hubtask, and meets no AI control, no hint and no empty AI panel anywhere.

## Story

A fresh installation has no AI provider. Nobody sees "Suggest", nobody is asked to consent, search
finds words, and nothing tries to call a model.

## How to check

1. With no provider and no consent, no screen shows an AI control, an AI placeholder or a prompt to
   set one up — except the AI section of the administration, for those allowed to configure it.
2. Every feature outside AI works identically with AI off; search works by words.
3. No outbound call to any model is made.
4. Switching AI off for a workspace keeps its configuration, so switching it on again needs no
   re-entry.

## Where it ends

* AI suggestions are never required for a flow to complete ([NG-ai-required](../../vision/non-goals.md)).
