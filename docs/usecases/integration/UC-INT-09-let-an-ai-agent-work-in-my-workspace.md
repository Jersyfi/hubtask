---
id: UC-INT-09
title: Let an AI agent work in my workspace
context: integration
actors: [PE-agent, PE-admin, PE-owner, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-04, P-05, P-08, P-11, P-14]
state: built
tasks: [J-11, J-12, J-13, J-14]
checked_by: [presentation/mcp/McpServer_test.go, presentation/mcp/Resources_test.go, presentation/mcp/Prompts_test.go, test/security/agent_guardrails_test.go, test/integration/mcp_stream_test.go]
---

# Let an AI agent work in my workspace

## Goal

An AI assistant a person chose can read and change the workspace through MCP exactly as far as
the credential it was given allows — every operation a person has, clearly marked, with anything
destructive closed unless somebody opened it on purpose, and every act on the record as an agent's.

## Story

The administrator creates a service account for the assistant, grants it the member role on one
hub, and mints it a token with the scopes to read and write items. They point the assistant's MCP
client at `/mcp` with that token. The assistant lists its tools, reads the hub's collections as
resources, creates entries and completes them. When it tries to empty the trash, it is refused:
nobody gave its token the destructive scope.

## How to check

1. Every use case in the catalogue is an MCP tool, with its description, its inputs and a
   read-only or destructive marking taken from the same declaration the API uses.
2. A tool call is refused or allowed exactly as the same call over the API with the same token
   would be; the agent never has more rights than its account and its token's scopes.
3. Every call served through `/mcp` is recorded with the actor type *AI agent*, also when a person
   uses their own token there.
4. A tool marked destructive is refused unless the token carries `agent:destructive`.
5. An agent cannot write a rule whose actions are destructive, so a rule cannot be used to get
   around check 4.
6. Hubs, collections, entries and saved views can be read as resources by address; reading one
   asks the same permission as the corresponding tool.
7. Published prompts can be listed and fetched; an argument a prompt does not declare is refused
   by name.
8. The agent's requests count against the same rate limits and quotas as the same token over the
   API, and its open stream counts against the same stream caps.
9. `hubctl mcp tools`, `resources` and `prompts` show what an agent would see with the same token.

## Where it ends

* Hubtask ships no agent and chooses no model; the agent is the person's own
  ([NG-ai-required](../../vision/non-goals.md)).
* No agent account type: an agent acts through a service account or a person's token.
* The product's own AI suggestions are the AI context's, not this use case's.
