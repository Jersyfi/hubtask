---
id: UC-AUD-01
title: Look up what happened in the workspace
context: audit
actors: [PE-owner, PE-admin, PE-auditor, PE-scripter, PE-agent]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-11, P-12]
state: partial
tasks: [E-09, E-12, G-12, F4-19, SC-29]
checked_by: [core/application/service/audit/List_test.go, test/integration/audit_read_test.go, presentation/rest/AuditController_test.go, cmd/hubctl/Audit_test.go, test/integration/audit_lookup_test.go, test/integration/privacy_test.go, test/security/authorization_test.go, apps/webapp/e2e/audittrail.test.mjs, docs/evidence/SC-15-D2-2026-10-13.md, docs/evidence/SC-15-D4-2026-10-13.md]
---

# Look up what happened in the workspace

## Goal

Somebody responsible for a workspace answers "who did what, when, to which thing, and was it
allowed?" from the workspace's own trail, without asking anybody — and the answer never shows the
content of anybody's work.

## Story

An administrator notices that a collection was moved last week and nobody remembers doing it. On
the audit screen they narrow the trail to the last seven days and the action *moved*, and find the
entry: who, when, the collection's identifier, and that it succeeded. A scripter asks the same
question with `hubctl audit query`, an agent through its MCP tool. Entries about somebody whose
account was erased name a pseudonym, not the person. Refusals are there too: somebody who tried to
change a setting they may not change left an entry marked *denied*.

In `D2` the parent who set up the household reads it; in `D4`–`D6` it is the company's
administrator or the customer's own; in `D5`/`D6` the provider's staff do **not** read it — the
workspace's trail is the workspace's.

## How to check

1. An owner, an administrator or an auditor can narrow the trail by period, action, actor, target
   and outcome, on the audit screen, through the API, with `hubctl audit query` and through MCP; the
   same filters give the same entries through every door.
2. An entry shows the moment, the action, the outcome (succeeded, failed, denied), who acted, and
   the target's type and identifier — and never a title, a note, a comment or a file's content; a
   changed sensitive field shows only *changed*.
3. An entry whose actor was erased names a stable pseudonym, the same one on every entry of that
   actor, never the former name or address.
4. A request for another tenant's entries answers nothing of that tenant; one workspace never sees
   another's trail.
5. A plain member who asks for the whole trail, or for a colleague's events, is refused with
   `access.not_permitted`, and the refusal itself appears in the trail as a *denied* entry.
6. Reading the trail adds no entry; the screen says so in words.
7. A period whose end is before its start is refused with `audit.period_invalid`; an unknown
   outcome with `audit.outcome_invalid`.
8. The list continues when the reader asks for more; there are no page numbers.

## Where it ends

* No full-text search over the trail: it holds no text to search.
* The trail is not an item's history. What happened to one task, told to everybody who can read
  that task, is the item's activity and belongs to the work context.
* The installation's own journal (provisioning, suspension, hard deletion of workspaces) is the
  operator's and is read in the instance area, not here; a workspace sees only the entries about
  itself that were written into its own trail.
* No operator access to a workspace's trail
  ([NG-operator-reads-content](../../vision/non-goals.md)); no pager
  ([NG-page-numbers](../../vision/non-goals.md)).
* A member reading their own events is a use case of its own, *See what the workspace recorded
  about me*.

See [audit.md](../../architecture/audit.md) §2, §5 and [ADR-0017](../../adr/ADR-0017-audit-trail.md).

## Today

* Check 5: not met as written - a plain member who asks for the whole trail is answered their own events rather than refused (`TestAMemberIsNarrowedToTheirOwnEvents`), as UC-AUD-06 and `audit.md` §5 promise; naming a colleague is refused and recorded as denied. Which one stands waits on the owner's decision.
