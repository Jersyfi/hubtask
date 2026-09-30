---
id: UC-PRV-01
title: Record a data subject request and answer it in time
context: privacy
actors: [PE-admin, PE-owner, PE-scripter, PE-operator]
deployments: [D3, D4, D5, D6, D7]
serves: [P-08, P-11, P-12]
state: partial
tasks: [E-10, E-12, H-13, F4-20, PH-02]
checked_by: [core/application/service/privacy/Requests_test.go, core/application/service/privacy/Deadlines_test.go, core/domain/model/privacy/Request_test.go, cmd/hubctl/Privacy_test.go, deploy/observability/alerts/tests/selfhosting.test.yaml]
---

# Record a data subject request and answer it in time

## Goal

When somebody exercises a data protection right — a copy of their data, erasure, restriction,
objection, rectification — the workspace records it as a case with a deadline, carries it to an
answer, and is warned before the deadline passes, so the right is not violated by forgetting it.

## Story

A former club member writes to the club asking for everything the club holds about them. The club's
administrator opens *Data subject requests* in the administration area, records the case — which
right, who it is about (an account or, for somebody without one, an address), a note — and sees its
deadline: thirty days from today. The register lists open cases soonest deadline first, and marks
those owed soon and those overdue. The administrator starts the case, which runs the work; when the
work is done the case closes itself. A case that cannot be answered is refused with a reason, which
is kept. In `D4`–`D6` the installation's monitoring raises an alert when a case nears its deadline,
so it does not depend on somebody opening the screen.

## How to check

1. An owner or administrator records a case with a kind (access, portability, erasure,
   restriction, objection, rectification), a subject (an account or an address) and an optional
   note, in the web app, through the API, with `hubctl dsr create` and through MCP; without a
   subject it is refused with `privacy.subject_required`.
2. A case without a stated deadline gets thirty days; a deadline in the past is refused with
   `privacy.deadline_in_past`.
3. A case moves only *received → in progress → completed* or to *rejected*; any other move is
   refused with `privacy.transition_refused`, and a rejection without a reason with
   `privacy.rejection_reason_required`.
4. The register lists open cases soonest deadline first and marks overdue cases; closed cases can be
   shown or hidden.
5. From seven days before a deadline, the register marks the case as owed soon — the same moment
   the installation's deadline watch starts warning.
6. A case can be given its own deadline when it is recorded, through every door that records one.
7. Recording, starting, completing and rejecting each write an entry (`dsr.recorded`,
   `dsr.started`, `dsr.completed`, `dsr.rejected`) with the right as its legal basis and no note
   text.
8. A member without the administrator's right neither sees the register nor records a case.
9. An open case whose original deadline has not passed can be extended **once**, to at most three
   months after receipt, with a reason — complexity or the number of requests — and the date the
   person was informed; without either it is refused. A second extension is refused.
10. After an extension the register shows the original and the extended date, and the watch and
    *owed soon* read the extended one; the extension writes `privacy.request_extended` with the
    reason and no note text. An installation-wide case is extended by the operator and every
    workspace it touches records it.

## Where it ends

* The person does not file the request in Hubtask themselves; it reaches the controller by any
  channel and the controller records it. Checking the requester's identity happens before
  recording and is not a feature.
* Hubtask does not inform the person of an extension; it records that the controller did
  (decided 2026-09-30, [data-protection.md](../../architecture/data-protection.md) §4.1).
* No correspondence with the requester from inside Hubtask.
* Each kind's work is its own use case: a copy of the data, erasure, restriction, objection.

See [data-protection.md](../../architecture/data-protection.md) §4.

## Today

* **Check 5 fails in the web app.** The register marks a case *owed soon* from two days before its
  deadline (`apps/webapp/src/lib/data/privacy.ts`), the deadline watch from seven
  (`core/application/service/privacy/Deadlines.go`).
* **Check 6 fails in the web app.** The record form has no deadline field; the API and
  `hubctl dsr create --due` take one.
* **Checks 9 and 10 are not built.** Decided 2026-09-30; milestone PH, task PH-02.
