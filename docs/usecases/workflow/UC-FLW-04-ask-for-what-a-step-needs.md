---
id: UC-FLW-04
title: Ask for what a step needs, and act when it is taken
context: workflow
actors: [PE-admin, PE-member, PE-agent]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-08, P-11, P-12]
state: specified
tasks: []
checked_by: []
---

# Ask for what a step needs, and act when it is taken

## Goal

A team writes its rules into the steps of its process: who may take a step, what must be true
before, what must be filled in on the way, and what happens afterwards — without a script.

## Story

The purchasing team says: only the requester's manager takes "Approve"; "Order" needs a supplier
and an amount above zero; taking "Order" assigns the entry to the buyers' group by round robin and
notifies the requester. The administrator sets each in the step's panel from lists, tries the step
against a sample entry, and sees which condition would fail and why.

## How to check

1. A step can be limited to people by role, group, a person field of the entry, or a condition over
   the entry; a person who does not meet it is not offered the step.
2. A step can require fields and conditions (validators) with a message the team writes per
   language or picks from a list; the refusal shows that message.
3. A step can ask for fields in a small form when it is taken (a screen).
4. A step can run actions afterwards, chosen from the same list of actions as automation rules,
   in the same change as the step where they only change the entry.
5. A step can be tried against a chosen entry before it is published, showing each condition's
   outcome and the actions that would run, changing nothing.
6. Conditions use the same language and editor as rule conditions; no second language exists.

## Where it ends

* No scripts or code in steps.
* An approval by several people is APR-01.

## Today

* Check 1: not met — the suite plan, S-07.
* Check 2: not met — the suite plan, S-07.
* Check 3: not met — the suite plan, S-07.
* Check 4: not met — the suite plan, S-07.
* Check 5: not met — the suite plan, S-07.
* Check 6: not met — the suite plan, S-07.
