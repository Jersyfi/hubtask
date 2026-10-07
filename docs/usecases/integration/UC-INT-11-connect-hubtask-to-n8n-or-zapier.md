---
id: UC-INT-11
title: Connect Hubtask to n8n or Zapier
context: integration
actors: [PE-integrator, PE-person, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-11]
state: partial
tasks: [P-04, P-05]
checked_by: [packages/n8n-nodes-hubtask/scripts/generate.test.mjs, packages/n8n-nodes-hubtask/scripts/trigger.test.mjs, packages/zapier-app/scripts/generate.test.mjs]
---

# Connect Hubtask to n8n or Zapier

## Goal

Somebody who builds their workflows in n8n or Zapier finds Hubtask there as a ready node or app,
with a trigger per event and an action per operation, and connects it to their own installation
without writing an HTTP request by hand.

## Story

The integrator installs the Hubtask community node in their n8n, enters their installation's
address and a personal access token, and builds a flow: *when an entry is completed in Hubtask,
post to the team chat*. In Zapier they search for Hubtask, connect their installation through its
consent screen, and pick *New entry* as a trigger. A new use case in a later Hubtask version appears as a new action after an update of the
node or app, without anybody adding it by hand.

## How to check

1. The n8n node and the Zapier app offer one action per API operation and one trigger per event
   type the contract declares; a test fails the build when either misses one.
2. A trigger subscribes a webhook when it is switched on and removes it when it is switched off.
3. Both connect to any installation by its address: the n8n node with a personal access token, the
   Zapier app through the installation's own OAuth with PKCE.
4. An n8n user installs the node from n8n's community node registry, and a Zapier user finds the
   app in Zapier's directory.

## Where it ends

* No Make (Integromat) module; Make and others use the API and webhooks directly.
* The node and the app hold no logic of their own beyond the contract.

## Today

* Check 4: not met — both packages are built and tested but marked private and not published; publishing needs accounts and reviews the owner holds.
