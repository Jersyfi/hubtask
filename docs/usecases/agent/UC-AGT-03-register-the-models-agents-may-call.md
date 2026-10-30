---
id: UC-AGT-03
title: Register the models agents may call, with the consent to send them material
context: agent
actors: [PE-admin, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: built
tasks: []
checked_by: [core/domain/model/agent/Agent_test.go, core/application/service/agent/Agent_test.go, test/integration/agent_repository_test.go, apps/webapp/src/lib/data/agents.test.ts]
---

# Register the models agents may call, with the consent to send them material

## Goal

An administrator registers the models the workspace's agents may call - the workspace's own
provider or a model the installation offers - with where each is hosted and how long its provider
keeps what it is sent; registering is the consent to send it material, and a change to any of that
stops every agent using it until a person approves it again.

## Story

The admin registers *local* on the workspace's self-hosted model. When the provider changes to a
model hosted elsewhere, the admin changes the entry; *Triage* stops, the admin approves it for the
new revision, and it acts again.

## How to check

1. A model is registered with a key, a name, its source, its name at the provider, its region and
   the provider's retention; the registering person and the moment are recorded as the consent.
2. A source the workspace's AI does not allow, a region below the installation's minimum
   jurisdiction or an offered model the installation does not offer is refused.
3. A change to its source, model, region or retention is a new revision with a new consent; a new
   name alone is not.
4. An agent whose approval does not cover its model's revision, or whose model is retired, is
   refused with `agents.model_changed`, and reads `needs_approval: true` until it is approved again.
5. Models stay in their workspace and join its archive.
6. REST, MCP and hubctl register, list, change and retire models alike.
7. The web app's administration lists the models with their region, retention and consent.

## Where it ends

* The model registry names models; calling one is the existing AI provider's (ai-first.md §2).
* No residency check of a model's actual address; the region is what the registrant states.
