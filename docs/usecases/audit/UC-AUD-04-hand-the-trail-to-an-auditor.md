---
id: UC-AUD-04
title: Hand a period of the trail to somebody outside
context: audit
actors: [PE-auditor, PE-owner, PE-admin, PE-scripter]
deployments: [D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-11]
state: built
tasks: [E-09, E-12, F4-19]
checked_by: [core/application/service/audit/Export_test.go, core/application/service/audit/Archive_test.go, test/integration/audit_read_test.go, presentation/rest/AuditController_test.go, cmd/hubctl/Audit_test.go]
---

# Hand a period of the trail to somebody outside

## Goal

An auditor or a regulator receives the workspace's trail for a stated period as a file they can
open without Hubtask — and can tell that it came from this installation and was not altered on the
way.

## Story

The external auditor asks for the trail of the last financial year. The workspace's auditor opens
the export section of the audit screen, sets the period, chooses JSON Lines or CSV and the backup
target to write to, and starts it. The export runs as a job; when it is done the screen says where
the archive lies. There is no download button — the archive is fetched at the target, where the
browser holds no credential. The archive carries its entries, a manifest that states the period,
the number of entries and the stretch of the chain it covers, a checksum list and a seal. The export
itself is an entry in the trail.

## How to check

1. An owner, an administrator or an auditor can start an export for a period, in JSON Lines or CSV,
   to one of the workspace's backup targets, in the web app, through the API, with
   `hubctl audit export` and through MCP.
2. An export without a period is refused with `audit.period_required`, without a target with
   `audit.export_target_required`, in an unknown format with `audit.format_invalid`.
3. A member without the right to read the whole trail cannot start one; a token without the
   `audit:export` scope cannot either, even when its owner could.
4. The archive at the target holds the entries of exactly that period, a manifest naming the period,
   the entry count, the first and last sequence number and that it is not encrypted, a checksum
   list, and — where the installation holds a key — a seal over the manifest.
5. Entries of an erased actor carry the pseudonym in the export too.
6. Every export writes one `audit.exported` entry in the same workspace.
7. The web app offers no download and says where the archive is.

## Where it ends

* No streaming to a SIEM, syslog, CEF or a webhook: that waits for a demonstrated need after `1.0`
  (open point A-3 in [audit.md](../../architecture/audit.md) §9). Until then a SIEM pulls through
  the read API.
* No filtered export (one actor, one action): the export is a period of the whole trail.
* No encryption of the export: it is meant to be read where this installation's keys are not.
* The seal is under the installation's own key and is checked by asking this installation; a
  public-key signature is not part of this use case.
