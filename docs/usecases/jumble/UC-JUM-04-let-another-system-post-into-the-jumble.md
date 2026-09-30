---
id: UC-JUM-04
title: Let another system post into the jumble
context: jumble
actors: [PE-integrator, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-11]
state: built
tasks: [G-10]
checked_by: [core/application/service/jumble/Intake_test.go, test/integration/jumble_test.go]
---

# Let another system post into the jumble

## Goal

A form on a website, a monitoring alert or a phone shortcut can drop something into the jumble with
one HTTP request and no account — authenticated only by the workspace's intake address.

## Story

The integrator takes the intake address and configures their contact form to post the sender, a
subject and the message to it. Each submission appears in the jumble marked *Webhook*, waiting for
someone to decide what it becomes.

## How to check

1. A POST to the webhook door with a valid address, a subject and a body creates one entry with
   channel *Webhook*, visible in the jumble under *Undecided*.
2. The request needs no account and no token beyond the address; the entry names no account as its
   author.
3. The sender text the request carries is stored as provenance and grants nothing.
4. An unknown, replaced or malformed address stores nothing and is answered with the same
   not-found in every case.
5. The same bounds as a quick capture apply — 500 characters of subject, 64 KB of body — and an
   entry with neither is refused.
6. The arrival fires a rule whose trigger is a new jumble entry, exactly as a captured one does.

## Where it ends

* No files through the webhook door; a system that has files uses the API with a token and the
  media upload.
* No per-sender authentication, signature or allowlist: the address is the whole credential, and
  replacing it is the way to shut a sender out.
* No reply to the sender beyond the HTTP answer.
