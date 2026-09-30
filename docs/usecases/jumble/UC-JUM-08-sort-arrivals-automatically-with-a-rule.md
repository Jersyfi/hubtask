---
id: UC-JUM-08
title: Sort arrivals automatically with a rule
context: jumble
actors: [PE-admin, PE-owner, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-11]
state: built
tasks: [G-10, G-07, F8-04]
checked_by: [test/integration/automation_trigger_test.go, core/application/service/automation/RunRule_test.go]
---

# Sort arrivals automatically with a rule

## Goal

Arrivals that always go to the same place — every invoice mail, every alert from monitoring — are
turned into work or dismissed by a rule, with no person pressing a button, and never with more
rights than the account the rule runs as.

## Story

The administrator writes a rule: when a new jumble entry arrives, if its sender ends in the
accounting provider's domain, convert it into the *Invoices* collection; otherwise leave it. They
switch it on. The next invoice mail appears in *Invoices* as a task; the jumble shows it as *Made
into an entry*.

## How to check

1. A rule whose trigger is a new jumble entry runs once for each arrival, whatever door it came
   through — capture, API, mail or webhook.
2. The rule's condition can read the entry's channel, sender, subject and body, as data.
3. A rule whose action converts the entry converts the entry the run is about without the rule
   naming it; one whose action dismisses it dismisses that entry.
4. The conversion runs as the rule's acting account and is refused where that account may not
   create in the destination; the run is recorded as failed and the entry stays *Undecided*.
5. An entry a person settled before the run reached it is not converted twice: the run's action
   is refused with `jumble.entry_settled`.
6. A rule may ask for an AI suggestion about the entry; the entry still becomes work only when a
   person accepts.

## Where it ends

* Writing, testing and switching on the rule are the automation use cases; this one is only what
  the jumble promises a rule.
* No rules that act on entries that arrived before the rule was switched on.
