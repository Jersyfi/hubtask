---
id: UC-JUM-01
title: Jot something down before deciding what it is
context: jumble
actors: [PE-person, PE-member, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-08, P-10, P-12]
state: partial
tasks: [G-10, F4-12, G-13]
checked_by: [core/application/service/jumble/Entries_test.go, test/integration/jumble_test.go]
---

# Jot something down before deciding what it is

## Goal

A thought gets out of a person's head and into the workspace in seconds, without first choosing a
collection, a type or a date. Deciding what it becomes comes later, in the jumble.

## Story

On the *Jumble* screen the person types a subject, a longer text if they want one, and presses
*Capture*. The entry appears at the top of the list as *Undecided*. From a terminal,
`hubctl jumble submit` does the same; a script does it over the API, an agent through MCP — and
either may carry files it has already uploaded. Nothing is a task yet.

## How to check

1. A person who may write in the workspace captures an entry with only a subject, only a body, or
   only an attachment; each appears in the jumble under *Undecided*, newest first, with the time it
   arrived.
2. An entry with no subject, no body and no attachment is refused with `jumble.entry_empty`.
3. A subject over 500 characters, a body over 64 KB or more than 20 attachments is refused with the
   code that names which (`jumble.subject_too_long`, `jumble.body_too_large`,
   `jumble.too_many_attachments`).
4. The same capture works from the web app, `hubctl jumble submit`, the REST API and the MCP tool.
   The entry records `QUICK_CAPTURE` or `API` as its channel; a request claiming `EMAIL` or
   `WEBHOOK` is refused with `jumble.channel_reserved`.
5. An attachment naming an upload that does not exist or is not confirmed is refused with
   `jumble.attachment_unknown`.
6. The body is stored exactly as typed, whitespace included — a pasted code block or quoted mail
   keeps its lines.
7. A person with no role on the workspace itself — a guest with one shared collection — sees
   neither the *Jumble* entry in the navigation nor the capture form, and a person who may only
   read the workspace sees the list but no capture form; the server refuses either one's capture
   as not permitted.
8. The audit trail holds `jumble.entry_submitted` naming the channel, and neither the subject nor
   the body.

## Where it ends

* Capturing decides nothing: no collection, no due date, no assignee. That is converting.
* Capturing works while the server answers. The jumble does not synchronise to devices and is not
  offered offline ([offline-sync.md](../../architecture/offline-sync.md) §4.2).
* No voice recording or transcription in the product; a transcript arrives as text like any other.
* No per-person jumble: the jumble is the workspace's, and everyone with a role on the workspace
  reads it.

## Today

* Check 7: not met in the web app — the navigation lists *Jumble* for everybody, and the capture form is drawn without asking whether the reader may write; only the server refuses, tracked in #1083.
