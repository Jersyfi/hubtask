---
id: UC-BAK-03
title: Back up now and check that the backup opens
context: backup
actors: [PE-owner, PE-admin, PE-selfhoster, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-08, P-11]
state: built
tasks: [E-05, E-06, E-12, F4-16]
checked_by: [core/application/service/backup/Run_test.go, core/application/service/backup/Perform_test.go, test/integration/backup_run_test.go, test/backup/Run_test.go, test/backup/Archive_test.go]
---

# Back up now and check that the backup opens

## Goal

Before something risky — an import, a big reorganisation, an upgrade — somebody responsible takes a
backup right now, watches it finish, and knows that it can actually be opened, not only that a file
was written.

## Story

Before importing three hundred tasks from another tool, the administrator presses *Back up now* on
the bucket target. The run shows its progress and finishes with its size. *Check that it opens*
reads the archive back — its checksums, its manifest, whether it decrypts — and the target shows
*last verified* with the moment. The list of what is at the target is read from the target itself,
so it shows the truth even if the database has forgotten a run.

## How to check

1. An administrator can start a backup to a target now, in the web app, through the API, with
   `hubctl backup run` and through MCP; the answer is a job whose progress can be followed.
2. A backup to a target that is already writing is refused with `backup.target_busy`; an
   incremental backup with nothing to build on with `backup.no_parent_archive`.
3. A finished run shows its size, its kind (full or incremental) and its end; a failed run says why.
4. *Check that it opens* reads the archive back and reports whether its checksums, manifest and
   encryption hold; the target shows when it was last verified.
5. The list of archives at a target is read from the target, shows for each whether it is complete,
   still being written or damaged, and never shows another workspace's archives.
6. A backup started by hand deletes no older archive.
7. Starting and verifying write `backup.started` and `backup.verified` in the trail.

## Where it ends

* No download of an archive through the browser; it is fetched at the target.
* No backup of the installation as a whole from here; see *Recover the whole installation*.
