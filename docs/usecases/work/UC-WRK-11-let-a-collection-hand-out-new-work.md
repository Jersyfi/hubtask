---
id: UC-WRK-11
title: Let a collection hand out new work automatically
context: work
actors: [PE-admin, PE-owner, PE-member, PE-integrator]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-06, P-08, P-11, P-12]
state: built
tasks: [C-02, F3-07, F10-13]
checked_by: [test/integration/auto_assign_policy_test.go, core/domain/model/work/AutoAssignPolicy_test.go, core/application/service/work/AutoAssignWorkItem_test.go]
---

# Let a collection hand out new work automatically

## Goal

Whoever runs a collection decides once how new work is shared out — always the same person, by
turns, at random, or to whoever has least on their plate — and from then on nobody has to think
about who takes the next one.

## Story

The club's support collection gets a policy: *round robin* over four volunteers, in an order the
administrator sets. Each new request lands on the next volunteer. In a family the chores collection
uses *random member* across the children's group. An entry created without a named person is
handed out by the policy; somebody who names a person themselves overrides it for that entry.

## How to check

1. The collection's policies dialog offers the strategies the server publishes: fixed, random
   member, random member of a random group, round robin, and least loaded — each with a sentence
   saying what it does — and *none*.
2. A fixed policy takes exactly one candidate; every other strategy at least one; random group
   member takes groups, the others take people. A wrong shape is refused with a code naming what
   is wrong (for example `containers.auto_assign_single_candidate_required`).
3. Round robin continues where it left off, entry after entry; the order of the candidates is set
   in the dialog.
4. With the policy enabled, an entry created without a responsible person is given one by the
   policy; its history records the assignment, and the event a rule or a webhook receives names
   the strategy that chose.
5. A candidate who cannot see the collection is never chosen; a policy with nobody eligible leaves
   the entry unassigned and says so in the answer.
6. An entry can be handed out on demand (*Auto-assign* on the entry), offered only where the
   collection has a policy — enabled or not.
7. Only a person who may change the collection's shape can set or change the policy; others see
   the dialog's reason rather than the controls.

## Where it ends

* No skill- or label-based routing; a rule in the automation context can assign by condition.
* No policy on a hub or the workspace that collections inherit — one policy per collection.
* No rebalancing of work already assigned.
