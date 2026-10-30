---
id: UC-INT-13
title: See the commits, branches and pull requests that name an entry
context: integration
actors: [PE-admin, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-12]
state: partial
tasks: []
checked_by: [core/domain/model/forge/Forge_test.go, core/application/service/forge/Forge_test.go, presentation/intake/CodeIntake_test.go, infrastructure/security/CodeHookSecrets_test.go, core/application/service/sync/InitialSyncCode_test.go, apps/webapp/src/lib/data/code.test.ts, apps/webapp/src/lib/data/replica.test.ts, test/integration/code_forge_test.go]
---

# See the commits, branches and pull requests that name an entry

## Goal

A team whose code lives in GitHub, GitLab, Forgejo or Gitea sees on an entry the work done for it
there - the branch, the commits, the pull request and whether it was merged - because the code
names the entry's key, without anybody copying links and without the workspace calling the forge.

## Story

On *Website relaunch → Settings → Code* the hub's lead adds a GitHub forge called *website*,
copies the address and the secret shown once into the repository's webhook, and chooses the push
and pull request events. Ada names her branch `feature/WEB-42-login`, writes "WEB-42 the form" as
a commit message and opens a pull request from the branch. The entry *WEB-42 Login* shows the
branch, the commit and the pull request as open; when Bert merges it, the entry shows it merged.

## How to check

1. A person with STRUCTURE on a hub adds a GitHub, GitLab or Forgejo forge on the hub's
   *Settings → Code*: the secret is shown once beside the address the forge posts to, no read of
   the hub's forges answers it, a new secret can be drawn, and a person without STRUCTURE is
   refused.
2. A push or a pull request whose branch's name, commit message, or pull request's title or
   description names the key of an entry of the forge's hub (`WEB-42` in `feature/WEB-42-login`)
   is kept on that entry; a key of another hub's entry, or of none, links nothing.
3. On the entry, *Code* lists its pull requests, branches and commits with repository, title,
   author and state - pushed, open, merged or closed - opening each in the forge; the state follows
   the forge's own moment, so a late or repeated delivery never turns a merged pull request back.
4. A delivery with a wrong signature or token, to a forge switched off, or delivered a second time
   is answered `404` with `forge.hook_not_found` and links nothing.
5. A forge is switched off, never removed: switched off it links nothing, and what it linked stays
   on the entries.
6. An entry's code reaches a device that holds the entry, and is read there while the server is
   away; it never reaches one that may not read the entry, nor the workspace next door.

## Where it ends

* The forge is never called: no commits, branches or pull requests are pulled, no status is posted
  back to the forge, and no forge app is installed through its own consent; the forge posts.
* A merge does not move the entry: no step or completion is taken from a commit message or a merge
  (smart commits); a workflow rule on the entry is the place for that.
* No builds or deployments on an entry, and no Bitbucket or Azure DevOps.
* A forge's links are removed with their entry; removing a forge with its links is not offered.

## Today

* Check 1: not met — no engine walk (`apps/webapp/e2e`) proves adding a forge on *Settings → Code*.
* Check 3: not met — no engine walk proves reading *Code* on an entry.
