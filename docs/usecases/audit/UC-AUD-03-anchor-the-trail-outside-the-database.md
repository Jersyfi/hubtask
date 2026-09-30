---
id: UC-AUD-03
title: Anchor the trail outside the database
context: audit
actors: [PE-owner, PE-admin, PE-auditor, PE-scripter]
deployments: [D4, D5, D6, D7]
serves: [P-06, P-08, P-09, P-11]
state: partial
tasks: [P-13]
checked_by: [core/application/service/audit/Anchoring_test.go, test/integration/audit_read_test.go, cmd/hubctl/Audit_test.go]
---

# Anchor the trail outside the database

## Goal

A workspace that has to prove its trail even against somebody with full access to the database
keeps a daily seal of the chain somewhere that database cannot reach — and anyone checking the
trail can compare it against that seal.

## Story

A company's administrator switches anchoring on in the audit screen's *Anchoring outside this
database* section and names one of the workspace's backup targets — ideally one with object lock.
From then on, shortly after midnight UTC on every day the trail moved, a small file with the last
sequence number and its hash lands at that target. When the auditor runs the chain check with
*compare with the anchor*, the answer says up to which entry the trail is sealed and whether the
database still agrees with the seal. A database rewritten and recomputed whole passes the chain
check and fails here.

## How to check

1. An owner or administrator can switch anchoring on by naming one of the workspace's own backup
   targets, and off again, in the web app, through the API, with `hubctl audit anchor` and through
   MCP; the chosen target is read back on the workspace's settings.
2. Each change of the setting writes `audit.anchoring_configured` with the target before and after.
3. An auditor sees the setting and cannot change it; an operator of the installation has no control
   over it at all.
4. On a day the trail moved, one anchor file named for the workspace and the day appears at the
   target, holding the last sequence number and its hash and no content; on a day it did not move,
   none is written.
5. A check with the anchor comparison answers whether anchoring is configured, up to which sequence
   the trail is sealed and whether the database agrees; an unreadable anchor answers
   `audit.anchor_unreadable`, a file that no longer matches its recorded receipt
   `audit.anchor_receipt_mismatch`.
6. A chain rewritten and recomputed whole below the anchor is reported as disagreeing with the
   anchor.
7. The anchor comparison is available through every door that runs the chain check, including
   `hubctl audit verify`.
8. A workspace without anchoring is told so by the check, rather than shown a comparison with
   nothing.

## Where it ends

* No public transparency log, no blockchain, no third-party timestamping service
  ([NG-phone-home](../../vision/non-goals.md)): the anchor goes only to a target the workspace
  named.
* Hubtask does not set object lock on the target; the target's own protection is its operator's.
* Anchoring is off until somebody switches it on; the smallest deployments never need it
  (P-10).

## Today

* **Check 7 fails for `hubctl`.** `hubctl audit verify` has no flag for the anchor comparison
  (`cmd/hubctl/Audit.go`, the `verify` command); the comparison is reachable from the web app, the
  API and MCP only.
