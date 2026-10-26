---
id: UC-JUM-12
title: Receive mail into a place
context: jumble
actors: [PE-person, PE-member, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-11, P-12]
state: built
tasks: []
checked_by: [core/domain/model/mail/Route_test.go, core/domain/model/mail/Settings_test.go, core/application/service/jumble/MailDelivery_test.go, presentation/intake/AddressedMailIntake_test.go, presentation/rest/MailIntakeController_test.go, apps/webapp/e2e/mail.test.mjs]
---

# Receive mail into a place

## Goal

A person sends or forwards a mail to the workspace's one intake, and it lands where it belongs -
a new entry in a collection, a comment on an entry - by readable names in the subject or the
address rather than secret codes, and whatever no rule may file waits in the jumble with the
reason, instead of being lost or walking in.

## Story

Anna forwards the printer's offer to the family intake with *#büro* in the subject. A minute later
*Büro* has the entry *Angebot Druckerei*, written by Anna, with the PDF attached. Ben answers a
client about *[WEB-42]*; the answer is a comment on that entry. A supplier's invoice from
*@lieferant.de* goes to *Einkauf* by a sender rule. A stranger's mail waits in the jumble, marked
*Sender not allowed* and *for Büro*, with *Sort in* one click away.

The same works through every door: `familie+buero@gmail.com` on a connected mailbox,
`buero.familie@in.example.app` or `familie@in.example.app` with *#büro* in the subject on the
operator's receiving domain.

## How to check

1. Every collection has a handle, derived from its name and unique in the workspace, and every
   entry a number in its collection; the entry shows its ticket key (`BUERO-42`) above its title.
2. A ticket key - `[KEY-42]` or `KEY-42` in the subject, `+key-42` or `key-42.` in the address -
   makes the mail a comment on that entry; a handle - `#handle` in the subject, `+handle` or
   `handle.` in the address - makes it an entry in that collection, the subject without the
   routing words as its title.
3. Without either, a sender rule, then the workspace's default collection decide; otherwise the
   mail waits in the jumble with the reason *No handle found* or *Handle or ticket key unknown*.
4. Under *Members only* (the default), a member who may create entries there writes as themselves;
   any other sender waits in the jumble with *Sender not allowed*. Under *Members and allowed
   senders* and *Anyone*, an admitted sender's mail is written as the administrator who opened the
   intake, headed *From: name <address>*.
5. The block list drops a sender's mail before every level; a provider's spam mark and a failed
   SPF, DKIM or DMARC send it to the jumble; an automatic reply or Hubtask's own notification, and
   more than 30 mails an hour from one sender, are dropped.
6. A message received again - the same Message-ID, or the same raw message without one - does
   nothing a second time.
7. The operator's inbound route refuses a request without the installation's inbound key, and an
   address that names no workspace with its intake on is answered not-found.

## Where it ends

* No SMTP receiver of Hubtask's own ([ADR-0094](../../adr/ADR-0094-one-mail-intake-routed-by-handles-and-ticket-keys.md)).
* No mail client: no threads, no outgoing mail from an entry beyond the notifications.
* Custom domains and OAuth sign-in for mailboxes are later steps on the same intake.
