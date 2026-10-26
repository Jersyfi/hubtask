---
id: UC-JUM-11
title: Set up the mail intake
context: jumble
actors: [PE-admin, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-11, P-12]
state: built
tasks: []
checked_by: [core/application/service/mail/Settings_test.go, core/application/service/mail/Mailboxes_test.go, core/domain/model/mail/Settings_test.go, core/domain/model/mail/Mailbox_test.go, infrastructure/mailbox/IMAP_test.go, infrastructure/mailbox/JMAP_test.go, infrastructure/mailbox/OAuth_test.go, core/application/service/mail/OAuth_test.go, core/domain/model/mail/OAuth_test.go, test/integration/mail_intake_test.go, apps/webapp/e2e/mail.test.mjs]
---

# Set up the mail intake

## Goal

A workspace's administrator decides in one place whether mail comes in and through which door - an
existing mailbox the workspace already has, or the operator's receiving domain - where what names no
place goes, who may write and which senders' mail is filed where, and sees what arrived and what
each message became, so a missing mail is a look at the log rather than a support question.

## Story

Anna opens *Administration → E-mail*, switches *Take mail* on and connects the family's Gmail
mailbox with an app password; the page says it was read a minute ago, and every collection's
handle now has its plus address, `familie+einkauf@gmail.com`. At an operator's she would instead
name the workspace *familie* on the receiving domain; the page then shows
`familie@in.example.app`. She chooses *Members and
allowed senders*, allows `@steuerbuero.de`, blocks a newsletter's domain and files
`@lieferant.de` into *Einkauf*. Below, the page explains the routing order and lists every
collection's handle to copy. A week later a supplier asks why their mail never arrived; the log
says *In the jumble · Sender not allowed*.

## How to check

1. *Administration → E-mail* is the workspace administrators' (the automation permission): the
   intake on or off, the name on the receiving domain with the address it makes, the default
   collection, the sender level with its allow and block lists, and the sender rules - saved in
   one write, with an audit entry.
2. A name another workspace holds is refused; a list entry that is neither an address nor a domain
   is refused with the field named.
3. The page shows the routing order, every collection's handle, and the newest messages of the
   last 30 days with what each became and why.
4. A collection's *Settings → General* shows its subject word (`#einkauf`), its address where the
   receiving domain makes one, and changes its handle for whoever shapes the collection; a handle
   another collection holds is refused.
5. The log is kept 30 days by the retention run.
6. *Connect a mailbox* offers Gmail, GMX, web.de, IONOS and Fastmail with what each provider asks
   for first, and any IMAP or JMAP server; it signs in once and stores the mailbox only when that
   worked, the secret sealed and never shown again. A refused password, an unreachable server and
   a missing folder are said in the form.
7. A connected mailbox is read every minute: only mail that arrived after connecting is taken,
   each filed message is moved to the folder *Hubtask*, and a message the intake cannot take stays
   in the mailbox. *Check now* reads it at once; each mailbox shows when it was last read and what
   the last read failed with; *Disconnect* forgets it.
8. *Connect with Google* and *Connect with Microsoft* sign in at the provider instead of typing a
   password: one button where the operator configured an app, otherwise the form takes an app of
   the administrator's own and shows the redirect URI to register; *Other provider (OAuth)* takes
   any provider by its HTTPS endpoints, scopes and IMAP server. The provider's answer comes back to
   the page and connects the mailbox after one sign-in with the token; a refusal, an answer later
   than ten minutes, a used or someone else's state connect nothing and say why. The refresh token
   is sealed, and a grant the provider withdraws shows on the mailbox.

## Where it ends

* Custom domains are a later step on the same intake
  ([ADR-0094](../../adr/ADR-0094-one-mail-intake-routed-by-handles-and-ticket-keys.md)).
* At most five mailboxes per workspace; no IDLE yet - a minute is the delay.
* No per-collection sender level: the level is the workspace's, the rules file by sender.
