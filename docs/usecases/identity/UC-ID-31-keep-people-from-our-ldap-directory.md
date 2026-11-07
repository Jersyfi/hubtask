---
id: UC-ID-31
title: Keep our people and groups from our LDAP directory
context: identity
actors: [PE-admin]
deployments: [D3, D4, D6, D7]
serves: [P-01, P-02, P-03, P-12, P-16]
state: verified
tasks: []
checked_by: [core/domain/model/identity/LdapDirectory_test.go, core/application/service/identity/LdapDirectory_test.go, infrastructure/ldap/Directory_test.go, test/integration/ldap_directory_test.go]
---

# Keep our people and groups from our LDAP directory

## Goal

A company whose people live in an LDAP directory or Active Directory - and which runs no SCIM
client in front of it - keeps the workspace's people and groups from that directory on a schedule:
a person who joins is invited, a person who leaves can no longer act, the groups follow the
directory's - without an administrator repeating any of it, without a password ever crossing the
network in the clear, and without a directory that answers wrong being able to switch the
workspace off.

## Story

The IT administrator creates a service account *Directory* (UC-ID-17) with a role that may manage
members, opens *Sign-in → Directory*, enters `ldaps://dc.example.com`, the account Hubtask reads
as, its password, the people's base and the groups' base, and chooses the service account. Hubtask
binds once and saves. Every hour it reads the directory: Lena, new in it, is invited and becomes
active at her first sign-in through the workspace's provider; Max, gone from it, is switched off -
his tasks and comments stay his. When somebody changes the people's base to an empty branch, the
next run answers nobody and is refused; nothing changes, and the screen says why.

## How to check

1. `PUT /directory/ldap` binds to the directory and searches the people's base before anything is
   stored, over `ldaps` or StartTLS only; a refused bind is `422` (`directory.ldap_refused`), an
   unreachable directory or one that refuses StartTLS `503` (`directory.ldap_unreachable`), and
   nothing is stored. The password is sealed to the workspace and never answered; omitting it keeps
   the sealed one. The run's account must be an active service account of the workspace
   (`directory.ldap_run_as_invalid`) that may manage members (`directory.ldap_run_as_refused`).
   The change needs a fresh step-up.
2. A run invites a person the directory has and the workspace does not, under the directory's
   address, holding `ldap:` and the directory's identifier; takes over an account already here
   under that address; renames a person whose name changed; switches on a person it had switched
   off; and switches off a person it holds who left the directory - their sessions end, nothing of
   theirs is erased.
3. A run never switches off an owner of the workspace or a person under a restriction of
   processing; they are counted as kept.
4. A run is refused, changing nothing, when the directory answers nobody
   (`directory.ldap_answer_empty`) or would switch off more than ten people and more than a fifth
   of those it holds (`directory.ldap_switch_off_too_many`, with the count); `confirm_switch_off`
   giving exactly that count back with a fresh step-up lets it through, and `dry_run` shows what a
   run would do without doing it.
5. The scheduled run records a directory that is down or refuses, and comes back after a backoff
   of up to a day; removing the directory ends it.
6. A group in the directory's groups' base is created and its members follow the directory's; a
   group the directory no longer has keeps its roles and loses its members; a run that changes
   nothing writes nothing.
7. The directory's configuration is neither read nor written from another workspace.

## Where it ends

* No password is read from the directory: a person signs in through the workspace's provider.
* No write to the directory; Hubtask only reads it.
* A group's members are its entries' names (`member`); nested groups and `memberUid` are not
  followed. A group holding `OWNER` is not changed by a run, as by SCIM.
* The identifier attribute must never change for an entry; a person whose identifier changes is
  switched off and the new entry skipped, since the address is already held.
