---
id: UC-MED-01
title: Attach a file to an entry
context: media
actors: [PE-person, PE-member, PE-guest, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-08, P-11]
state: built
tasks: [C-05, C-06, F3-09, F6-01]
checked_by: [core/application/service/media/Media_test.go, core/application/service/work/AttachMedia_test.go, test/integration/media_test.go, test/integration/media_acceptance_test.go, test/security/upload_test.go]
---

# Attach a file to an entry

## Goal

A person puts the file that belongs to a piece of work — the quote, the photo of the broken tap,
the signed form — on the entry, and everyone who may see the entry can download it; nobody else
can, and nothing uploaded can run in anybody's browser.

## Story

On an entry the person opens *Attachments*, chooses a PDF and waits while it uploads. It appears in
the list with its name, type and size. A colleague on the same entry presses it and the file
downloads. Over the API the same happens in three steps — ask for an upload, send the bytes to the
address answered, confirm — and then attach; `hubctl media upload` and `hubctl media attach` do it
from a terminal.

## How to check

1. An upload is asked for with a file name, a size and a purpose; the answer is a one-time address
   the bytes go to, which expires.
2. The upload is usable only after it is confirmed; confirming reads the bytes back, and a size
   that differs from the one declared is refused with `media.size_mismatch` and the bytes removed.
3. The stored type is what the bytes are, never what the uploader claimed; a claim contradicting
   the bytes on an image type (HTML named `image/png`) is refused with `media.type_mismatch`.
4. Only PNG, JPEG, GIF and WebP may be shown inside a page; every other file is only ever served as
   a download, from a separate origin.
5. Attaching needs the right to edit the entry and an upload that is confirmed and of this
   workspace; attaching to a type that carries no attachments — an activity by default — is
   refused.
6. Everyone who may read the entry sees its attachments, oldest first, and gets a short-lived
   download address for one on request; anybody else is told the file does not exist.
7. Attaching the same file twice changes nothing; one file may be attached to several entries
   without being copied.
8. Detaching takes the file off the entry and says only that; the file itself goes once nothing
   refers to it (the next use case).
9. Attachments of one workspace can never be read, attached or detached from another.

## Where it ends

* No preview or viewer inside the app for anything but the four image types.
* No virus scanning inside Hubtask.
* No capturing a file offline; an upload needs the server
  ([offline-sync.md](../../architecture/offline-sync.md) §1).
* No versions of a file; a new version is a new upload.
