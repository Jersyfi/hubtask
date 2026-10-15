---
id: UC-BAK-04
title: Keep backups unreadable to the target, and readable to us after a loss
context: backup
actors: [PE-owner, PE-selfhoster, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-11]
state: built
tasks: [E-02, E-05, H-10, CP-01]
checked_by: [test/backup/Archive_test.go, core/application/service/backup/Perform_test.go, core/application/service/backup/ArchiveKeys_test.go, core/application/service/backup/Apply_test.go, test/backup/Restore_test.go, test/integration/restore_lost_database_test.go, test/integration/sealing_archives_test.go, infrastructure/crypto/Envelope_test.go]
---

# Keep backups unreadable to the target, and readable to us after a loss

## Goal

Whoever runs the backup target cannot read the workspace's backups — and the workspace itself can
still open them after the server it came from is gone, which is the moment a backup exists for.

## Story

A club keeps its backups in a bucket at a cloud provider. The archives there are encrypted with
AES-256; the provider sees file names and sizes, nothing else. The club's technical member keeps
the key material the installation's operator documentation tells them to keep, somewhere other than
the server. When the server's disk dies, they install Hubtask again, give it the same key material,
point it at the same bucket, list the archives and restore the newest one.

## How to check

1. By default every archive a target receives is encrypted with AES-256-GCM, and its manifest
   names the key it was written under but holds no key.
2. A target without encryption exists only with an explicit acknowledgement and is marked
   unencrypted wherever targets are listed.
3. An archive from one target cannot be opened with another target's key.
4. After the installation's database is lost, a new installation given the kept key material and
   the target's credentials lists the archives at the target and restores an encrypted one.
5. An archive whose key the installation no longer holds is refused with a message naming what is
   missing, and the message does not ask for something no door can accept.
6. Retiring an old installation key never makes an existing archive unreadable without saying so
   first.

## Where it ends

* No escrow of keys by Hubtask or anybody else, and no key recovery: key material that was not kept
  is lost, and the documentation says so in the place the owner sets up backups.
* No end-to-end encryption of the workspace itself ([NG-e2e-encryption](../../vision/non-goals.md)).
* The workspace export handed to somebody else is deliberately unencrypted (*Take the workspace to
  another installation*).

See [backup-restore.md](../../architecture/backup-restore.md) §4 and
[ADR-0019](../../adr/ADR-0019-backup-targets.md).
