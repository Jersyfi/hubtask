---
id: UC-BAK-01
title: Choose where the workspace's backups go
context: backup
actors: [PE-owner, PE-selfhoster, PE-admin, PE-operator, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-06, P-08, P-09, P-11, P-12]
state: partial
tasks: [E-03, E-04, F4-16]
checked_by: [core/application/service/backup/Target_test.go, core/application/service/backup/TargetLifecycle_test.go, test/integration/backup_target_test.go, test/backup/Conformance_test.go]
---

# Choose where the workspace's backups go

## Goal

The owner names the places the workspace's backups are written to — a folder on the server, an S3
bucket, an SFTP server, a WebDAV share — checks that each one works, and trusts that nothing else
can be sent there and that the credential is never shown again.

## Story

A self-hoster adds two targets in *Backup*: the server's own backup folder, and a bucket at a
hosting provider. For the bucket they give the endpoint, the bucket and the key pair; *Test* writes
a small file, reads it back and deletes it, and reports that the target is writable and how much
room is left. The credential fields stay empty from then on. A NAS on the home network is refused
until the operator has allowed private networks for the installation; an SFTP server is refused
until its host key or fingerprint is named, because a first connection that trusts whatever answers
is one an attacker only has to be present for once. In a provider's installation (`D5`/`D6`) a
workspace may add its own targets only if the operator has allowed it.

## How to check

1. Only the owner can add or remove a target; an administrator can test one; an administrator or
   an auditor can list them, with no credential in the answer.
2. A target of a kind this build cannot talk to is refused with `backup.kind_unsupported`, naming
   the kind.
3. An SFTP target that names neither the host key nor its fingerprint is refused with
   `backup.host_key_required`; there is no way to switch the check off.
4. A target on a private or loopback address is refused unless the operator has released private
   networks for the installation.
5. *Test* reports whether the target can be written, read and cleared, and the free space where the
   target says it; the result is recorded.
6. A target without encryption needs an explicit acknowledgement (`backup.insecure_acknowledgement_required`)
   and is marked as unencrypted from then on.
7. In a multi-workspace installation a workspace's own target is refused with
   `backup.tenant_targets_disabled` unless the operator has allowed workspace targets.
8. A target still used by a schedule cannot be removed (`backup.target_in_use`).
9. Adding, changing and removing a target writes `backup.target_changed` or
   `backup.target_removed` with where data may now go, and never the credential.
10. Every kind the installation can talk to — local, S3, SFTP and WebDAV — can be added in the web
    app, through the API, with `hubctl backup target add` and through MCP, with every field the
    kind needs.

## Where it ends

* FTPS, FTP, SMB, Azure Blob, Google Cloud Storage, rclone and plain HTTP PUT are named in
  [ADR-0019](../../adr/ADR-0019-backup-targets.md) and arrive only when each passes the same
  conformance suite; until then they are refused by name (check 2).
* A target's path on the server stays inside the installation's backup volume; configuring a target
  does not administer the machine.
* No trust on first use for SSH hosts, ever.
* The installation's own system backup is not a target here; see *Recover the whole installation*.

See [backup-restore.md](../../architecture/backup-restore.md) §2.

## Today

* Check 10: not met in the web app — the target form offers no WebDAV, and its SFTP variant has no host key field and sends the user name as a credential, so an SFTP target added there is always refused; the form offers no encryption choice either, tracked in #1073.
