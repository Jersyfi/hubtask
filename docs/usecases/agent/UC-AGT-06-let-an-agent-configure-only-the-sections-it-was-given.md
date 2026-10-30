---
id: UC-AGT-06
title: Let an agent configure only the sections it was given
context: agent
actors: [PE-admin, PE-agent, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: built
tasks: []
checked_by: [core/application/service/config/Scopes_test.go, core/application/service/config/Config_test.go]
---

# Let an agent configure only the sections it was given

## Goal

An administrator gives a configuring agent a token for the sections of the configuration document
it may apply - types and workflows, say - without the scopes of every area those sections write,
and an agent that may read and plan sees the full reach of what it plans.

## Story

The admin mints a token for the setup agent with `config:read`, `config:plan`, `config:apply` and
`config:apply:types`. Its plan shows which entries a changed workflow would move; it applies a new
incident type, and the plan that also changes a role is refused at the role.

## How to check

1. A token holding `config:apply:<section>` beside `config:apply` applies that section's blocks
   without each block's own scope; a block of another section still needs its own, and the
   person's permission is asked by every block's use case as before.
2. A token holding `config:plan` reads the previews a plan's reach asks for; it admits no write.
3. The per-section scopes are minted like any scope and named on the consent screen.

## Where it ends

* The scopes bound a credential; they grant no permission a person does not hold.
