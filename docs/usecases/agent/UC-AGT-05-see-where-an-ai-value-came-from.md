---
id: UC-AGT-05
title: See where a value that came from AI came from
context: agent
actors: [PE-member, PE-auditor, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: partial
tasks: []
checked_by: [core/application/service/suggestion/Provenance_test.go, test/integration/agent_repository_test.go]
---

# See where a value that came from AI came from

## Goal

Whoever reads an entry can find out which of its values came from AI and a person accepted - which
fields, from which suggestion, model and prompt version, accepted by whom and when - long after the
suggestion itself is gone, and the workspace's archive carries it.

## Story

Ben accepts a better title the AI proposed and writes his own notes over the proposed ones. A year
later the auditor reads the entry's provenance: its title came from AI, the model and prompt
version, Ben accepted it; the notes are not marked.

## How to check

1. Accepting a suggestion marks each object it wrote with the names of the fields the model
   proposed, the suggestion, the model, the prompt and version, who accepted it and when - never a
   value; a field the person overrode is not marked.
2. An entry's provenance is read by whoever may read the entry; one they may not read answers not
   found.
3. The marks outlive the suggestion's retention and join the archive and the access export.
4. REST, MCP and hubctl read an entry's provenance alike.
5. The entry's page shows which of its values came from AI.

## Where it ends

* No visible label on an accepted value: a suggestion a person accepts needs none (ai-first.md
  §2.1, AI-2); the mark is machine-readable.
* Only an entry's provenance is read; a template's or a rule's marks are kept and read by no use
  case yet.

## Today

* Check 5: the entry's page does not show it (S-22).
