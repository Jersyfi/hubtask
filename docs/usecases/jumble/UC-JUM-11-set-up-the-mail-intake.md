---
id: UC-JUM-11
title: Set up the mail intake
context: jumble
actors: [PE-admin, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-11, P-12]
state: built
tasks: []
checked_by: [core/application/service/mail/Settings_test.go, core/domain/model/mail/Settings_test.go, apps/webapp/e2e/mail.test.mjs]
---

# Set up the mail intake

## Goal

A workspace's administrator decides in one place whether mail comes in, where what names no place
goes, who may write and which senders' mail is filed where - and sees what arrived and what each
message became, so a missing mail is a look at the log rather than a support question.

## Story

Anna opens *Administration → E-mail*, switches *Take mail* on and names the workspace *familie* on
the operator's receiving domain; the page shows `familie@in.example.app`. She chooses *Members and
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

## Where it ends

* A connected mailbox (IMAP, JMAP), custom domains and OAuth sign-in are later steps on the same
  intake ([ADR-0094](../../adr/ADR-0094-one-mail-intake-routed-by-handles-and-ticket-keys.md)).
* No per-collection sender level: the level is the workspace's, the rules file by sender.
