---
id: UC-JUM-03
title: Forward mail into the jumble
context: jumble
actors: [PE-person, PE-member, PE-selfhoster, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-11, P-12]
state: built
tasks: [G-11, H-15]
checked_by: [core/application/service/jumble/InboundMail_test.go, presentation/intake/MailParser_test.go]
---

# Forward mail into the jumble

## Goal

A mail somebody forwards — an invoice, a request from a client, a newsletter to read later — lands
in the workspace's jumble with its subject, its text and its files, and nothing that arrives is
lost, even a message nobody can parse.

## Story

The workspace has an intake address (the previous use case). Whoever runs the mail — the
self-hoster's own mail server, a provider's inbound route, a mail-to-webhook bridge — posts each
raw message to that address. A person forwards a mail to the mailbox that route watches; a minute
later it is in the jumble under *Undecided*, marked *Email*, with the sender, the subject, the text
and the attached PDF.

In `D1`/`D2` without any mail server, the household uses a provider's inbound route or a bridge;
Hubtask does not fetch from a mailbox itself.

## How to check

1. A raw message posted to the mail door with a valid address becomes one jumble entry with
   channel *Email*, the sender, the subject and the text body.
2. The message's files become the entry's attachments, each judged by its content rather than by
   the type the message claimed.
3. A message the parser cannot read is still stored: the entry carries the raw message as a file
   named `message.eml`, and the delivery is not refused.
4. A message beyond one of the parser's bounds — more than 100 parts, nesting deeper than 4, more
   than 20 files, a file over 10 MB — is refused with the code that names the bound
   (`mail.too_many_parts`, `mail.too_deeply_nested`, `mail.too_many_attachments`,
   `mail.attachment_too_large`).
5. A subject that is too long is shortened rather than refused.
6. A delivery to an unknown or replaced address stores nothing — no entry and no file — and is
   answered not-found.
7. The entry names no account as its author; the arrival is announced as the system's, and a rule
   on new jumble entries fires for it.

## Where it ends

* No IMAP or POP polling, and no stored mailbox password ([ADR-0040](../../adr/ADR-0040-no-imap-intake.md)).
  A transport beyond the webhook door, when one is wanted, is JMAP.
* No reply by mail, no thread tracking, no mail client.
* No spam filtering; whatever the forwarding route lets through arrives.
