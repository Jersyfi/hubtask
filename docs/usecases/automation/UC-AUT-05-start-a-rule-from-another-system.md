---
id: UC-AUT-05
title: Start a rule from another system
context: automation
actors: [PE-integrator, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-08, P-11]
state: built
tasks: [G-08, F4-13]
checked_by: [core/application/service/automation/InboundTrigger_test.go, test/integration/automation_trigger_test.go]
---

# Start a rule from another system

## Goal

An outside system — a shop, a form, a monitoring tool — starts one specific rule by posting to that
rule's own secret address, and the rule can use what was posted, without the outside system ever
holding an account in the workspace.

## Story

The administrator writes a rule whose trigger is *an inbound webhook*, saves it, and makes its
address: shown once, to be copied into the shop's settings. The rule's condition reads
`payload.status == "paid"` and its action creates a task *Ship order* with the order number in the
title. Every paid order now appears as a task. When the address leaks, a new one is made and the old
one stops at once.

## How to check

1. The address is made per rule, shown once, and begins with `hbt_hook_` so secret scanners find a
   leaked one; no listing shows it again.
2. A POST with a JSON object to the address starts exactly one run of that rule; two posts are two
   runs.
3. What was posted is available to the conditions and actions as `payload`, as data: nothing in it
   is ever followed as an instruction.
4. A body that is not a JSON object is refused with `automation.inbound_payload_not_an_object`.
5. An unknown or replaced address, a deleted rule, a switched-off rule and a rule whose trigger has
   changed are all answered with the same not-found, and nothing runs.
6. Making a new address ends the old one in the same moment, and making one needs the automation
   permission at the rule's scope.
7. An address cannot be made for a rule whose trigger is not an inbound webhook
   (`automation.trigger_not_inbound`).
8. The run names no person as its cause; what it may do is decided per action as the rule's acting
   account.

## Where it ends

* No signature verification of the sender: the address is the credential.
* No answer to the sender beyond accepting the post; a rule cannot reply with data.
* No address that starts every rule; one address, one rule.
