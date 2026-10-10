# Milestone PH — Privacy and the household

The goal: **a household can share one Hubtask and still keep things to itself, a child or a
grandparent can have an account without a mailbox, and a data protection officer can answer every
right the law gives — including where the law and a legal hold pull in different directions.**

**One defect comes first.** The erasure reads no legal hold at all today — not even one on the whole
workspace — so a hold placed for a lawsuit does not protect what it names from an erasure request.
PH-01 fixes it before anything is added.

Every task names its use cases; a task is done when the named checks hold, with the evidence in the
pull request.

**Delivers:** UC-ID-16 (1–8), UC-ID-20 (1–8), UC-LIF-06 (1–8), UC-PRV-01 (9, 10), UC-PRV-03 (9–13), UC-PRV-05 (4, 5, 7–12)
**Released:** 2026-09-30

## Decisions

Each measured against the deployments `D1`–`D7`:

1. **The data protection rights** ([data-protection.md](../architecture/data-protection.md) §4.1):
   a deadline extended once, a legal hold that wins over an erasure as far as it reaches (which also
   answers R-3), and AI objection per person with the workspace's choice to make AI part of the work
   for everybody.
2. **Private hubs** with a transparent emergency access ([ADR-0073](../adr/ADR-0073-private-hubs.md)).
3. **Managed accounts** without a mail address ([ADR-0074](../adr/ADR-0074-managed-accounts.md)).
4. **Only the workspace can name the legal basis `NOT_OFFERED` needs**, so the installation's and the
   plan's lock can hold a workspace at `OFFERED` only (PH-03).
5. **The extension's audit action is `dsr.extended`**, not `privacy.request_extended`: every action
   of the privacy context is `dsr.*`, and `privacy.*` names its message codes (the owner,
   2026-10-07).
6. **A legal hold stops a workspace's deletion and a destructive restore** (the owner, 2026-10-09,
   #1233): the deletion request is refused while a hold is in force, a workspace pending deletion
   stays pending until the last hold is lifted, and `REPLACE_TENANT` is refused under a hold and
   keeps the workspace's holds when it runs — data-protection.md §5, UC-LIF-06 check 3. Built by
   PH-10 (#1228).
7. **What a hold keeps stays in backups and the workspace export** (the owner, 2026-10-09, #1234):
   it is kept out of automation and AI only, and stays in backups, the workspace export and the
   person's own Art. 15 copy — data-protection.md §4.1, UC-PRV-03 check 10. PH-09 (#1227) filters
   no export.
8. **A deleted comment keeps its text under a hold** (the owner, 2026-10-10, #1246): it reads as
   deleted to everybody, and the sweep clears the text once the last hold covering it is lifted;
   editing stays free — data-retention.md §4, UC-LIF-06 check 9. Built by the fix of #1232.
9. **A point-in-time recovery places the legal holds of the rewound period again** (the owner,
   2026-10-10, #1247): from the audit export, before the erasures are re-applied and before traffic
   is admitted; a hold released in that period stays in force until its owner releases it again —
   backup-restore.md §8.5, data-protection.md §5, UC-BAK-08 check 3. Built by the fix of #1239.

---

## PH-01 — A legal hold wins over an erasure, as far as it reaches

*Depends on: nothing. First, because it is a defect.*

**Use cases:** UC-PRV-03 (9, 10, 11, 12, 13), UC-LIF-06 (1–8)

The erasure asks the holds before it removes anything: whatever a hold on the workspace, a hub, a
collection, an entry or the person's account covers is kept rather than erased; the rest is erased.
The account itself is kept, and restricted, only under a hold on it or on the workspace; otherwise it
is anonymised as today and what is kept stays attributed to the former user. The case closes as
partly completed with the count, the hold and the legal basis, and records the remainder; releasing
the hold is the write that seeds the remainder's erasure. `ACCOUNT` holds are accepted and cover the
account and the entries, comments and attachments the person contributed — for the erasure, deleting
for good, emptying the trash and retention. Starting an erasure gets the confirmation it lacks
(UC-PRV-03 check 8), and the confirmation says what a hold will keep. What the restriction keeps the
data out of is PH-09; the workspace's deletion and a destructive restore are PH-10.

**Acceptance:** a test per hold scope proving what is kept and what goes; a test that the release
completes the rest; the integration suite green.

---

## PH-02 — Extending a deadline once, with a reason

*Depends on: nothing.*

**Use cases:** UC-PRV-01 (9, 10)

`data_subject_request` keeps the original date beside the extended one, the reason (`COMPLEXITY`,
`NUMBER_OF_REQUESTS`) and the date the person was informed; one extension, at most three months
after receipt, before the original deadline; the watch and the register read the extended date;
`dsr.extended` (decision 5). Every door: web app, API, `hubctl dsr extend`, MCP, automation. An
installation-wide case is extended by an operator through the API, `hubctl` or MCP — the web app
offers none (UC-PRV-06) — and every workspace it touches records `dsr.extended` in its own trail.

**Acceptance:** tests for the bounds (twice, too late, too long — month ends included —, no reason,
no informed date); the register shows both dates; an installation-wide extension leaves one entry in
every workspace it touches.

---

## PH-03 — Keeping one's own content out of AI, and the workspace's choice

*Depends on: ADR-0072's installation level (SC-11) for the lock; the workspace half stands alone.*

**Use cases:** UC-PRV-05 (4, 5, 7, 8, 9, 10, 11, 12)

The workspace value `ai.person_opt_out` — `OFFERED` (default) or `NOT_OFFERED` with a required legal
basis — with the installation's and the plan's lock, which can hold a workspace at `OFFERED` only
(decision 4). The profile's *Keep my content out of AI*, shown only where AI is on and the workspace has more than one person. The prompt builder
reads the objections: authored content excluded, names replaced, AI actions not offered to the
person. `NOT_OFFERED` shows the basis instead of the switch and tells each person whose earlier
withdrawal stops taking effect. A person may withdraw their own consent in the web app.

**Acceptance:** a test that an objecting person's comment never reaches a prompt the adapter
receives; a walk in both positions.

---

## PH-04 — Private hubs: the rule and every reader

*Depends on: nothing.*

**Use cases:** UC-ID-16 (1, 2, 3, 5, 6, 8)

`container.private` on hubs; `EffectiveRole` ignores memberships above a private hub; every reader
that narrows by readable scopes honours it — navigation, hub list, search, overview, calendar feed,
saved views, automation and AI run for somebody else — and a gate test lists them. Any person may
create a private hub for themselves where the workspace allows it. Administrators see owner and size
without the name. The last member leaving sends the hub to the trash and tells the owner. Export and
backup mark it; restore keeps the mark. The offline replica of a non-member never receives it.

**Acceptance:** a table test over roles × scopes with a private hub on the path; a cross-tenant and a
cross-member negative test for every reader; a walk as two parents and a child.

---

## PH-05 — Private hubs: the emergency access and the setting

*Depends on: PH-04.*

**Use cases:** UC-ID-16 (4, 7)

The owner's emergency access — step-up, reason, one hour, not renewable — with an immediate
notification to every member (a new notification category) and an audit action. The workspace
setting *Private hubs allowed* with the installation's and the plan's default and lock.

**Acceptance:** a test that the access notifies before it answers any read; a walk of the access and
of the setting switched off.

---

## PH-06 — Managed accounts: creating one and signing in

*Depends on: nothing.*

**Use cases:** UC-ID-20 (1, 2, 3, 4, 6, 8)

`account.sign_in_name` (unique per workspace, nullable) and its catalogue row; *Add someone without an
address* with the start password drawn by Hubtask and shown once through `OneTimeSecret`; sign-in by
address or sign-in name with the same refusal and ledgers; the first sign-in routed into
`PASSWORD_CHANGE`; "not possible without a mailbox" where mail is needed; the workspace setting with
the installation's and the plan's default and lock.

**Acceptance:** a walk creating a child's account and signing in as the child; tests that a sign-in
name and an address are indistinguishable in every refusal.

---

## PH-07 — Managed accounts: a new start password, and an address later

*Depends on: PH-06.*

**Use cases:** UC-ID-20 (5, 7)

The administrator's *New start password* for managed accounts only — step-up, trail, every session
ended, second factor still demanded — and adding an address with its confirmation, after which the
account is an ordinary one and keeps its sign-in name.

**Acceptance:** a test that the action is refused for any account with an address; a walk of the
renewal and the upgrade.

---

## PH-08 — The walk, by use case and deployment

*Depends on: all.*

**Use cases:** every check in `Delivers` — the walk confirms them and adds none.

The use case checklist (`docs/usecases/README.md`, "Checking work against its use cases") over PH, then a walk per deployment: `D2` a household with two parents, a teenager with a
private hub and a grandparent with a managed account; `D4` a company with a legal hold, an erasure
request and AI made part of the work; `D5` a consumer alone in a workspace, where none of it appears.
The evidence under `docs/evidence/`; the use cases move to `built` or `verified`.

**Acceptance:** the milestone's use cases have no *Today* entry that names a PH task.

---

## PH-09 — What a hold keeps stays out of automation and AI

*Depends on: PH-01 (the restriction an erasure leaves); PH-03 (the prompt builder's exclusion).*

**Use cases:** UC-PRV-03 (10)

What a hold keeps from an erasure is restricted: the account carries `RESTRICTED`, the one state of
Art. 18. Today only automatic assignment reads it. The automation engine stops acting as or
assigning to a restricted person, and the suggestion service stops asking about their entries
(UC-PRV-04 checks 4 and 5, findings that block UC-PRV-03 check 10); the prompt builder treats a
restricted person's content as it treats an objecting person's (PH-03). Backups, the workspace
export and the person's own Art. 15 copy keep the kept data (Decision 7); no export is filtered.

**Acceptance:** a test that a rule neither acts as nor assigns to a restricted person; a test that a
restricted person's comment never reaches a prompt the adapter receives.

---

## PH-10 — A hold stops a workspace's deletion and a destructive restore

*Depends on: PH-01; the owner's answer on how a hold meets the 65 days of data-protection.md §5
(decision 6, #1233).*

**Use cases:** UC-LIF-06 (3)

Two deletion paths read no legal hold: the workspace's hard deletion after its grace, not even under
a `TENANT` hold, and a destructive restore, which clears `legal_hold` and imports the archive's —
dropping a hold placed after the archive, reviving a released one, replacing held rows and resetting
a kept account's status. Both honour the holds in force as the owner decided (decision 6): the
deletion request is refused under a hold, a pending workspace stays pending until the last hold is
lifted, and a replace is refused under a hold and keeps the workspace's holds when it runs.

**Acceptance:** a test that a workspace under a hold is not hard-deleted and goes after the last
release; a test that a destructive restore is refused under a hold and keeps the workspace's holds
and a restricted account when it runs.
