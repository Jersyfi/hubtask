# Milestone CP — The open promises

The goal: **every promise a use case makes and a finding showed broken is kept** — backups that
open after the database is lost, a privacy request carried out across the installation and counted
in calendar months, erasures that a recovery does not undo, webhook subscriptions that can be
narrowed, retention periods that the data catalogue names and the product actually enforces, and the
sign-in notices and connections ADR-0078 decided.

CP is cut from the findings of GitHub milestone CP, each verified against `main` at c5586a40b on
2026-10-15: every one still holds. A finding enters as a task only where a use case check states
what it breaks; the others wait for the owner (§ "Waiting on the owner") or leave the milestone
(§ "Not in CP").

Every task names its use cases; a task is done when the named checks hold, with the evidence in the
pull request.

**Delivers:** UC-BAK-04 (4, 5, 6), UC-BAK-07 (1), UC-BAK-08 (3, 6), UC-ID-04 (9), UC-ID-05 (6), UC-ID-10 (8), UC-INT-01 (9), UC-PRV-01 (2), UC-PRV-06 (3, 4), UC-PRV-08 (2), UC-AUD-07 (3, 4)

## Decisions

Each measured against the deployments `D1`–`D7`:

1. **The backup archive's key is bound to the target by a salt the manifest carries** (the owner,
   2026-10-13, #1075): backup-restore.md §4. The passphrase stays refused; that is the rule, not a
   gap. CP-01 builds the rest.
2. **A case's deadline is one calendar month from the day of receipt** (the owner, #1177):
   UC-PRV-01 check 2, data-protection.md §4. CP-03 builds it; stored deadlines stay as they are.
3. **SC-36 and SC-37 carry the checks decided in #1172**: UC-ID-04 check 9, UC-ID-05 check 6,
   UC-ID-10 check 8 (ADR-0078 §1, §6).
4. **Integration runs its database in production's shape** (the owner, #1176, ADR-0082):
   deployment.md §3.1. A restore drill on integration is a drill against a real installation, so
   it carries UC-BAK-08 check 6; production's own first drill stays in
   `blocked-production-namespace.md` (worker, at the cut: integration is a running cluster with
   real backups, not `gate-pitr`'s kind cluster).
5. **Two built use cases are refuted by findings and move back to `partial`** (worker, at the cut,
   a correction): UC-BAK-07 check 1 fails in multi-tenant operation (#1158), and UC-PRV-08 check 2
   fails for finished jobs, webhook deliveries and the trail (#1389, #1155). Their *Today* says so.
6. **SC-30 leaves CP** (worker, at the cut, applying milestone SC's Decision 11): dropping the
   stored offer count is safe only in the first release after one that contains SC-26. The only
   tag that contains it is `v0.2.0-rc.2`, a pre-release, which is never for production
   (versioning-release.md), so no supported upgrade path has passed through SC-26 yet; and the task
   carries no unmet check. It stays on #1134 without a milestone, for the cut after `v0.2.0`.
7. **Retention comes last.** CP-09 and CP-10 touch the retention sweep's wiring and its
   queries, which the open fix of #1488 (draft #1496) rewrites; they start once it has merged.
   CP-09 refers to #1155 and CP-10 closes it.
8. **CP-07 comes last** (coordinator, 2026-10-15): its manifests, `bootstrap.sh` and documents are
   built in the repository; applying them to the integration server is done by whoever holds its
   access, and the task's drill record follows that.

---

## CP-01 — A backup opens after the database is lost

*Depends on: nothing.* · issue #1075 · backup-restore.md §4

**Use cases:** UC-BAK-04 (4, 5, 6)

A target gets a random salt when it is created; every archive's manifest carries it, and the
archive key is derived from the master key and that salt, read from the manifest on restore rather
than from the target's row. Archives written under the target's identifier keep opening (a legacy
derivation for a manifest without the salt). `backup.archive_key_required` no longer asks for
something no door accepts. The re-seal and the key census count backup archives, so retiring a key
says first what it would strand.

**Acceptance:** a test that a target re-created on a fresh installation with the kept key material
lists and restores an encrypted archive; a test that an archive written before the change still
opens; a census test naming archives under a retiring key.

---

## CP-02 — An installation-wide request is carried out by its own job

*Depends on: nothing.* · issue #1076

**Use cases:** UC-PRV-06 (3, 4)

The scope `admin:tenants` is checked when the case is started, by the person who starts it; the job
that carries it out does not ask its system actor for it again. In every other workspace the person
is found by their account there, not by the address alone, so the archive holds their entries and
comments too. Every workspace visited writes `dsr.collected`.

**Acceptance:** an integration test that a started installation-wide case writes one archive with
the person's data from two workspaces and a `dsr.collected` entry in each; the refusal without the
scope stays.

---

## CP-03 — A case counts one calendar month from the day of receipt

*Depends on: nothing.* · issue #1196 · Decision 2

**Use cases:** UC-PRV-01 (2)

A case records `received_on` (today unless stated, never later); without a stated deadline it gets
one calendar month from that day, clamped at the month's end, ending at the end of the day in the
workspace's time zone — the arithmetic PH-02's extension already uses. A case whose deadline has
passed is recorded and shown overdue; `privacy.deadline_in_past` is retired for it. Every door: the
web app's form, the API, `hubctl dsr create`, MCP. The API description's "thirty days" goes with the
contract change.

**Acceptance:** table tests over month ends (31 January, 29 February), zones and a day of receipt in
the past; a walk recording a late case and seeing it overdue.

---

## SC-36 — A person is told when a way into their account changes

*Depends on: nothing.* · issue #1145 · ADR-0078 §6 · Decision 3

**Use cases:** UC-ID-04 (9), UC-ID-05 (6)

A password set through a reset or changed, and a provider connected to or disconnected from the
account, are told to the person by mail — what changed and how to undo it — as well as audited;
without a mail server only audited. An operator's opening of the password keeps telling the
administrators only.

**Acceptance:** a test per change that one mail reaches the person and none without a mail server;
the mail names no secret.

---

## SC-37 — Connect a provider from a signed-in session

*Depends on: nothing.* · issue #1146 · ADR-0078 §1 · Decision 3

**Use cases:** UC-ID-10 (8)

From the account's settings a signed-in person connects a provider identity whose address differs
from the account's, after a step-up and a fresh sign-in at the provider; it appears among their ways
to sign in, and an identity connected to another account is refused. The first, unauthenticated
arrival keeps requiring the same address.

**Acceptance:** a test that a different address connects only from a session with a step-up and a
fresh provider sign-in; the refusal for an identity held by another account; a walk up to the
provider (no real provider account, as milestone SC's Decision 9).

---

## CP-04 — A webhook subscription can be narrowed

*Depends on: nothing.* · issue #1084 · automation.md §3.1

**Use cases:** UC-INT-01 (9)

A subscription takes a condition over the event in the automation's expression language — compiled
with the same bounds as a rule's — and a scope of one hub or collection; the dispatcher delivers
only what passes both. Subscribing to a scope needs the automation permission there. The contract
adds `scope` (the `filter` field exists and stops being refused); automation.md §3.1 says it is
built.

**Acceptance:** a test that a filtered and a scoped subscription receive only matching events; an
invalid or too deep condition refused at save; a cross-tenant negative test for the scope.

---

## CP-05 — A workspace in multi-tenant operation can be exported

*Depends on: nothing; see the note.* · issue #1158 · multi-tenancy.md §5

**Use cases:** UC-BAK-07 (1)

The operator's export may write to an installation-wide target in multi-tenant operation with tenant
targets off: the export reads that target as the installation's, through one narrow door, while the
workspace's own reads still see none. How that door is built is the task's readiness record; if it is
a `SECURITY DEFINER` function it depends on the answer to #1494 (managed PostgreSQL).

**Acceptance:** an integration test in multi mode with tenant targets off that an operator exports a
workspace to an installation-wide target, and that the workspace's own reads still list no
installation target.

---

## CP-06 — A recovery re-applies the erasures of the rewound period

*Depends on: nothing.* · issue #1271 · backup-restore.md §8.5

**Use cases:** UC-BAK-08 (3)

The erasure half of §8.5 step 5, in the shape of the holds half: `hubctl` reads the erasures
completed after the recovery point from the temporary cluster recovered to the end of the WAL
archive (or the archive fallback), and the server carries them out again per workspace after the
holds are placed and before traffic is admitted. Idempotent; each re-run is audited.

**Acceptance:** an integration test that an erasure completed after the recovery point is undone by
a rewind and carried out again by the command; a hold placed in the rewound period still keeps what
it covers; the §8.5 procedure names the command.

---

## CP-08 — Finished jobs go after seven days

*Depends on: nothing.* · issue #1389 · `docs/privacy/data-catalog.md`

**Use cases:** UC-PRV-08 (2)

A purge of finished jobs older than seven days, installation-wide (the job table has no workspace
boundary), in the leader's housekeeping; pending and running jobs stay.

**Acceptance:** an integration test that a finished job past the period goes and a pending one and a
recent finished one stay.

---

## CP-09 — Webhook deliveries go after thirty days

*Depends on: the fix of #1488 merged (Decision 7).* · issue #1155 (its first half)

**Use cases:** UC-PRV-08 (2)

A remover for `WEBHOOK_DELIVERY` in the retention run: deliveries older than their period are
deleted in batches; a legal hold is read as for every kind.

**Acceptance:** an integration test that a delivery past the period goes and a recent one stays,
inside one workspace only.

---

## CP-10 — The trail is kept for its period and no longer

*Depends on: CP-09.* · issue #1155 (its second half) · audit.md

**Use cases:** UC-AUD-07 (3, 4), UC-PRV-08 (2)

Whole monthly partitions of the trail older than the workspace's audit period are removed; a hold on
the workspace keeps them. Each removal writes an entry naming the period and the count. Verifying the
chain starts at the oldest remaining entry instead of reading its predecessor's absence as a break.

**Acceptance:** an integration test that a partition past the period goes, the removal is recorded,
and the remaining chain still verifies; a hold keeps the partition.

---

## CP-07 — Integration's database in production's shape

*Depends on: nothing; last, because only the repository's half can be built from a worker's
machine (Decision 8).* · issue #1197 · ADR-0082 · Decision 4

**Use cases:** UC-BAK-08 (6)

CloudNativePG with one instance on integration, the WAL archive to an object-store bucket (Object
Lock checked first), the restore drill as a CronJob, alert A-12 real; the database rebuilt and the
workspace bootstrapped again; RT-8's rolling-update probe rebuilt for CloudNativePG;
`deploy/integration/README.md` and deployment.md §3.1 say it is built.

**Acceptance:** one passing restore drill on integration, its dated record under `docs/evidence/`.

## Waiting on the owner

Findings of the GitHub milestone CP that no task above carries yet. Each enters with a task, and its
checks enter `Delivers`, once its decision is answered:

| Finding | Decision | What the answer settles |
|---|---|---|
| #1431 — an administrator issues a start password to an Owner | #1462 | whether a ceiling applies to issuing a way in; UC-ID-20 check 5 reworded |
| #1399 — a provider sign-in skips enrolment | #1428 | whether a provider sign-in counts for the second-factor rule; if not, identity.md §8 is corrected and #1399 closed |
| #1463 — an erasure of the last owner | #1478 | whether the erasure waits for another owner; UC-PRV-03 gains a check (draft #1476) |
| #1303, #1267, #1160, #1337 | #1498 | checks for four findings no check describes, or the CL bucket instead |

#1160 touches the retention sweep itself; whatever the answer, it comes after CP-09.

## Not in CP

* **SC-30** (#1134) — Decision 6.

## The cut checklist

1. **What it delivers** — the checks above, all existing; none changes wording.
2. **Principles** — P-03, P-04, P-08, P-09, P-11 are the ones whose "Broken when" the findings
   show; each task restores its own.
3. **The matrix** — per task in its readiness record: CP-02, CP-05, CP-06 and SC-37 touch access or a
   lifecycle and name their doors there.
4. **The premise** — every finding verified against the code on 2026-10-15 (see the head of this
   file).
5. **Decisions** — above; the owner's open points are #1462, #1428, #1478, #1498, and #1494 for
   CP-05 only if it needs a definer function.
6. **The rules** — backup-restore.md §4 and §8.5, data-protection.md §4 and deployment.md §3.1
   already say what is built; automation.md §3.1 and audit.md change in their tasks.
7. **Findings** — every finding of the GitHub milestone is a task, waits on a named decision, or
   leaves with a reason.
8. **Tasks** — each names its checks, dependencies and acceptance; together they carry `Delivers`.
9. **The standing rules** — rule 3 and rule 10 bind CP-06, CP-09 and CP-10 (no user content in
   what a re-run or a removal records); rule 11 binds CP-03, CP-04 and CP-06 (contract first);
   rule 12 binds every migration; known traps: "a read-then-write without a lock" (CP-06's re-run),
   "the audit hash covers the stored shape" (CP-10), "the integration tests share one database".
10. **A look before the build** — SC-37 and CP-03 change screens; the owner sees them on their
    draft pull requests.
