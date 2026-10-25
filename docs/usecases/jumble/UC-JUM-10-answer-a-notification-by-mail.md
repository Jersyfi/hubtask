---
id: UC-JUM-10
title: Answer a notification by mail
context: jumble
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-11]
state: built
tasks: []
checked_by: [core/application/service/jumble/MailAddresses_test.go, core/application/service/jumble/MailDelivery_test.go, presentation/intake/AddressedMailIntake_test.go]
---

# Answer a notification by mail

## Goal

A person who gets a notification about an entry by mail answers it from their mail program, and
the answer stands on the entry as their comment.

## Story

Anna gets *Ben commented on Startseite texten*. She presses *Reply*, writes two lines and sends.
The comment is on the entry, by Anna, without the quoted notification below it.

## How to check

1. Where a receiving domain is set, a notification about an entry carries the entry's address as
   `Reply-To`, and Hubtask's own notification header.
2. A reply from the recipient becomes a comment by them on that entry, with the quoted text cut.
3. A notification that comes back by itself — an out-of-office answer, a bounce — stores nothing.

## Where it ends

* No threads and no mail client; a reply is one comment.
* Without a receiving domain, notifications carry no `Reply-To`.
