---
id: UC-PRV-03
title: Erase a person on request
context: privacy
actors: [PE-owner, PE-admin, PE-scripter]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-03, P-04, P-08, P-11]
state: partial
tasks: [E-10, E-11, H-13, F4-20, PH-01, PH-09]
checked_by: [core/application/service/privacy/Erasure_test.go, core/application/service/privacy/Perform_test.go, test/integration/privacy_test.go, test/privacy/PG2_deletion_test.go, core/domain/model/privacy/Request_test.go, core/domain/model/privacy/Plan_test.go, core/application/service/privacy/Preview_test.go, core/application/service/privacy/Remainder_test.go, test/integration/privacy_hold_test.go, test/integration/privacy_remainder_test.go, test/retention/account_hold_test.go, test/contract/privacy_kept_test.go, apps/webapp/e2e/privacy.test.mjs]
---

# Erase a person on request

## Goal

A person who asks to be erased disappears from the workspace — their account, their address, what
identifies them — while the work that belongs to the workspace and to other people stays usable,
and the trail can still prove the erasure happened.

## Story

The owner records an erasure request and chooses how much to remove: *anonymise* (the default —
their account becomes *former user*, what they wrote stays, credited to nobody) or *delete in full*
(their own comments go too). The screen explains what each choice does to other people's work
before it is made. Starting it asks the owner to confirm, naming the person and what the chosen
mode removes. A job carries it out: sessions and tokens end, assignments are released,
notifications are discarded, media nobody else uses is removed. The case closes itself. In the
trail the person's old entries stay, and every read of them now shows a pseudonym.

## How to check

1. Only the owner can start an erasure; an administrator who tries is refused with
   `access.not_permitted`.
2. A case with no mode chosen is carried out as *anonymise*.
3. After *anonymise*: the account shows as *former user* with no address and no password; the
   tasks and comments the person wrote are still there; their sessions and tokens are ended and
   their assignments released.
4. After *delete in full*: additionally, every comment the person wrote is gone, and a restore of
   an older archive does not bring it back.
5. *Delete in full* for a person who still runs automation rules is refused with
   `privacy.erasure_blocked_by_rule`, naming what to do first.
6. The person's trail entries remain, every read and export of them shows the same pseudonym, and
   the chain still verifies.
7. The erasure writes one `dsr.erased` entry with counts per place data was removed from, and no
   name or address.
8. In the web app, starting an erasure asks for a confirmation that names the person and what the
   chosen mode removes, before anything happens.
9. After erasure, no storage location — rows, media, search index, outbox, rule runs, deliveries —
   still holds the person's personal data, apart from the trail's metadata **and what a legal hold
   keeps** (check 10).
10. Data a legal hold covers — a hold on the workspace, a hub, a collection, an entry, or on the
    person's account — is not erased or anonymised but restricted: out of automation and AI; it
    stays in backups, the workspace export and the person's own copy of their data (Art. 15).
    Everything no hold covers is erased as usual.
11. A case that kept something closes as partly completed and names how much was kept, under which
    hold, and the legal basis (Art. 17(3)(e)); the confirmation before starting says in advance
    what a hold will keep.
12. Releasing the hold carries out the rest of the erasure by itself, and the case records it.
13. A hold on an account covers the entries that person created, their comments, their attachments
    and the account itself, and stops the account's erasure and deletion — not its sign-in.

## Where it ends

* Backups keep the person until they expire — 35 days for the system backups; this is told to the
  person, not hidden.
* The trail is not rewritten; see *Keep the trail for its period, and no longer* in the audit
  context.
* No erasure of other people's content that mentions the person.
* No undo: once carried out, an erasure is final (P-04's grace is the step from *received* to
  *in progress*).
* A hold wins over an erasure only as far as it reaches — decided 2026-09-30
  ([data-protection.md](../../architecture/data-protection.md) §4.1, which also answers R-3).

See [data-protection.md](../../architecture/data-protection.md) §4–5.

## Today

* Check 10: not met — automation and AI read a restricted person's data, tracked in #1227.
