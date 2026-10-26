---
id: UC-JUM-09
title: Mail straight into a collection or an entry
context: jumble
actors: [PE-person, PE-member, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-11, P-12]
state: retired
tasks: []
checked_by: []
---

# Mail straight into a collection or an entry

> **Retired 2026-10-25:** the random per-place addresses this describes are superseded by one
> intake routed by handles and ticket keys ([ADR-0094](../../adr/ADR-0094-one-mail-intake-routed-by-handles-and-ticket-keys.md),
> decision #1756); what a person does now is [UC-JUM-12](./UC-JUM-12-receive-mail-into-a-place.md).

## Goal

A person forwards a mail to the place it belongs to — a collection gets a new entry, an entry gets
a comment with the files — without a mail server of the workspace's own and without writing a rule,
and whatever is not clearly allowed waits in the jumble instead of being lost or walking in.

## Story

The operator set a receiving domain once. Anna opens the collection *Büro*, copies its address from
the collection's menu and forwards the printer's offer to it. A minute later *Büro* has the entry
*Angebot Druckerei*, written by Anna, with the PDF attached. A mail from a stranger to the same
address waits in the jumble, marked *for Büro*, with *Sort in* one click away. The entry
*Startseite texten* has its own address under *Details*; Anna's colleague answers a client's mail to
it and the answer becomes a comment with the attachment.

Without a receiving domain, the same places say that an administrator can set one, and the
jumble's webhook door works as before.

## How to check

1. The jumble, a collection and an entry each show their address with *Copy* — the jumble in its
   head and on *Today* while nothing waits, a collection in its menu and in *Settings → General*,
   an entry as a *By mail* line in its details; the address has the form
   `<label>.<code>@<receiving domain>`, with a code of 16 random characters.
2. *New address* replaces it: a delivery to the old one stores nothing and is answered not-found.
3. A mail to a collection's address from the address of a person in the workspace who may create
   entries there becomes an entry by that person: subject as title, text as description with the
   quoted reply cut, files as attachments.
4. A mail to an entry's address from such a person becomes a comment by them, and its files are
   attached to the entry.
5. Under *Only members* (the default), a mail from an unknown sender, from a person without the
   right, or marked as spam by the inbound service lands in the jumble addressed to the place, with
   *Sort in* offered there; under *Anyone with the address*, an unknown sender's mail is written as
   the person who opened it to everyone, headed *From: name <address>*.
6. A collection or entry that is archived or in the trash stores nothing and answers not-found;
   restored, the same address works again.
7. A message whose `Auto-Submitted` is not `no`, or that carries Hubtask's own notification header,
   is accepted and stores nothing; more than 60 deliveries an hour to one address are answered 429.
8. The inbound route refuses a request without the installation's inbound key, and without a
   receiving domain the places show a hint for administrators instead of an address.

## Where it ends

* No mail server, no IMAP, no mailbox password ([ADR-0040](../../adr/ADR-0040-no-imap-intake.md),
  [ADR-0092](../../adr/ADR-0092-mail-addresses-for-every-place.md)).
* No sender lists per place beyond *members* and *anyone*.
* No mail client: no threads, no outgoing mail from an entry beyond the notifications.
