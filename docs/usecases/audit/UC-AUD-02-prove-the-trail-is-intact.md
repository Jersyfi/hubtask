---
id: UC-AUD-02
title: Prove the trail has not been changed
context: audit
actors: [PE-auditor, PE-owner, PE-admin, PE-scripter]
deployments: [D3, D4, D5, D6, D7]
serves: [P-08, P-11, P-12]
state: verified
tasks: [E-09, E-12, F4-19]
checked_by: [core/application/service/audit/Verify_test.go, test/integration/audit_read_test.go, presentation/rest/AuditController_test.go, cmd/hubctl/Audit_test.go, apps/webapp/src/lib/data/audit.test.ts]
---

# Prove the trail has not been changed

## Goal

An auditor can show — to themselves and to somebody outside — that nobody has edited, removed or
inserted an entry in the workspace's trail, and if somebody has, where the damage starts.

## Story

Before the annual review the data protection officer opens the audit screen and asks for a check.
The answer is plain: *the chain holds*, with how many entries were checked. On a day it does not
hold, the answer is not an error message but a finding — *the first entry that does not hold is
number 4 812* — and the finding is itself written into the trail as a critical entry, so that
whoever reads the trail months later sees that somebody noticed. A scripter runs the same check
with `hubctl audit verify` in a nightly job and gets a non-zero exit when it fails.

## How to check

1. A check needs a period; without one it is refused with `audit.period_required`, and a period
   ending before it starts with `audit.period_invalid`.
2. A sound trail answers *valid*, the number of entries checked, and no broken entry.
3. After one row is changed directly in the database, the check answers *not valid* and names the
   sequence number of the first entry that does not hold.
4. A missing sequence number is reported as a gap, by number.
5. A failed check writes one critical `audit.chain_broken` entry with the first broken number and
   the gaps; a successful check writes nothing.
6. `hubctl audit verify` exits with a non-zero status when the chain is broken, and zero when it
   holds.
7. The web app shows a broken chain as a finding with the entry number to start from, not as a
   failed request.
8. Concurrent writers never produce two entries with one sequence number, and an entry with
   nanosecond timestamps, a structured change or no change at all still verifies.

## Where it ends

* The chain proves tampering **inside** the database. Somebody with full database access who
  recomputes the whole chain is caught only by an anchor outside it — that is *Anchor the trail
  outside the database*.
* The check does not repair anything and does not say who changed the row; that is forensics at the
  database, not a feature.
* No public-key signature that a stranger can check without asking this installation.

See [audit.md](../../architecture/audit.md) §3.
