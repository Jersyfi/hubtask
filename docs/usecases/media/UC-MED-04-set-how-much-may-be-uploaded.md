---
id: UC-MED-04
title: Set how much may be uploaded
context: media
actors: [PE-selfhoster, PE-operator, PE-platform]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-07, P-10, P-11, P-15]
state: built
tasks: [C-05, H-08]
checked_by: [test/security/upload_test.go, test/integration/quota_test.go]
---

# Set how much may be uploaded

## Goal

Whoever runs the installation bounds how large one file may be and how much each workspace may
store in total, and a person who hits either bound is told which one before they wasted an upload.

## Story

The self-hoster leaves the default of 64 MB per file. A provider sets a storage quota of 5 GB per
workspace through its platform. A customer who reaches it is refused the next upload with a message
that names the storage limit, and sees their standing on the workspace's quota page.

## How to check

1. The installation's largest file is set once (64 MB by default); a larger upload is refused when
   it is asked for, with `media.too_large` naming the limit, and a stream that grows past it is cut
   off at the boundary rather than stored.
2. The web app reads the limit from the installation and refuses a larger file before sending it.
3. A workspace with a storage quota is refused an upload that would pass it, with
   `capacity.media_bytes`, before any bytes are sent.
4. A workspace's standing against its storage quota is shown on its quota page.
5. Without a quota a workspace is bounded only by the file limit; the default is no quota.
6. One workspace reaching its quota changes nothing for another.

## Where it ends

* No per-person or per-collection quota.
* No price per gigabyte and no upgrade button ([NG-billing](../../vision/non-goals.md)); the
  platform sets the quota.
* No list of which types may be uploaded; what a file may do is decided by its type, not refused
  by it.
