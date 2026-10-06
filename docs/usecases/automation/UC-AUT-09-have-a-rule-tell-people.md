---
id: UC-AUT-09
title: Have a rule tell people something
context: automation
actors: [PE-admin, PE-owner, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-12]
state: specified
tasks: []
checked_by: []
---

# Have a rule tell people something

## Goal

A rule can tell a person or a group that something happened — *the invoice is overdue*, *a new
request arrived* — through the same notification path and the same preferences every other
message goes through.

## Story

The administrator adds a step *notify the group Accounting* to the rule that handles overdue
invoices. Each member of the group receives a mail naming the entry, with a link, in their own
language; a member who switched that kind of message off does not.

## How to check

1. A rule can name an account or a group as the recipient of a notification step; each member of
   a named group is one recipient.
2. The message goes through the notification path: the recipient's preferences apply, it is
   written in the recipient's language, and it carries a title and a link only.
3. A recipient who may not read the entry the run is about is not told about it.
4. The step is refused at the save when it names an account or group that does not exist, and the
   check flags it when one is removed later.

## Where it ends

* No free-form HTML mail, no attachments, no entry notes in the message.
* Whether a rule may mail an address that is no account in the workspace (`SEND_EMAIL` in
  automation.md) is not decided by this use case; it is a question for the owner.
* No message to a channel that is not built; mail is the only one today.

## Today

* Check 1: not met — no action names an account or a group as recipient; `NOTIFY_ACCOUNT` and `NOTIFY_GROUP` are refused with `automation.action_unknown`.
* Check 2: not met — no rule step reaches the notification path.
* Check 3: not met — there is no notification step whose recipients could be narrowed.
* Check 4: not met — there is no recipient for the save or the check to refuse or flag.
