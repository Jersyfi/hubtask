# ADR-0060 — The check: a rule's references resolved before they fail

**Status:** accepted (2026-09-21) · **Date:** 2026-09-20

## Context

A rule is data that is executed later, with a service account's rights, without anybody
looking. [`automation.md`](../architecture/automation.md) §2.2 draws the line at the write: what
cannot be run is refused rather than stored, because "stored and ignored is worse than refused".
And §2 gives the engine one self-protection: five failed runs in a row switch a rule off and tell
its author.

Both speak *after* the moment they are about. A rule that was correct when it was written and
names a label that was deleted a week later is stored, enabled, and runs — into a `FAILED` run,
five times, and then off. A rule that names an action kind a later version of the product no
longer serves — a use case renamed, folded into another, retired — is stored, enabled, and every
run of it fails until the same counter reaches five. In both cases the first anybody hears of it
is the notification that the rule is gone, and the person reading it has to work out from the
run log what changed. That is the failure mode the owner named when the rule flow was designed
(milestone F8, decision 6): rules that stop working silently, found by their absence.

Three things constrain what can be built against it:

* **Nothing enumerates tenants** ([`multi-tenancy.md`](../architecture/multi-tenancy.md) §2.1).
  A pass "over every rule after an update" cannot be a job the leader runs, because the leader
  never sees a tenant's rules. What a tenant owes is seeded by a write in that tenant, or asked
  for by a request in that tenant.
* **A subscriber runs inside the dispatcher's transaction** and may not reach the use case
  registry (§2.0). A deletion event can say *that* something a rule may name is gone; the check
  that reads every rule of the workspace and resolves every reference is not work for the
  transaction that recorded the deletion.
* **The registry does not know what an identifier refers to.** A use case declares a field
  `label_id` of kind `id`; that it names a label is a convention of the name, not a fact the
  catalogue carries. The check needs a table from field name to the store that can answer
  "does this still exist" — and that table is the one place this decision can be wrong.

## Options

**A. Leave it to the run.** Keep §2.2 and the streak; improve the notification. Costs nothing,
and leaves the author learning about a deleted label from five failed runs.

**B. Refuse the deletion.** A label, bucket, template or subscription that a rule names cannot be
deleted while the rule exists. Correct in the small and wrong in the product: the person
deleting a label is usually not the person who wrote the rule, may not be allowed to see the
rule, and would be refused with a reason they cannot act on. It also says nothing about an
action kind that a *version* removed.

**C. A check that speaks before the run, with its findings on the rule.** A use case resolves
every reference a rule carries against what exists now and writes what it found on the rule
itself; it is asked for by the workspace (on demand, and by the screen that lists rules when it
opens) and seeded by the deletion of anything a rule may name. A finding that means the rule
cannot run switches it off, with the audit entry and the notification the streak already sends.

## Decision

**C.** The check is `CheckRules`, a use case in the catalogue like every other, and its
findings live on the rule.

**What the check resolves**, per rule, in the order of what is cheapest to refuse:

| Reference | Against | Level | Code |
|---|---|---|---|
| The trigger's event type | what this build emits (`event.Types()`) | `BROKEN` | `automation.finding.event_unknown` |
| Every action's kind | the catalogue (`ByAutomationAction`) | `BROKEN` | `automation.finding.action_unknown` |
| Every action's parameter keys | the descriptor's declared fields | `ATTENTION` | `automation.finding.parameter_unknown` |
| Every condition, and a branch's | the compiler (ADR-0009) | `BROKEN` | `automation.finding.condition_invalid` |
| The `run_as` account | the accounts and service accounts of the workspace | `BROKEN` | `automation.finding.account_gone` |
| Every parameter of kind `id` whose name the table below knows | the store of that kind | `ATTENTION` | `automation.finding.reference_gone` |

`ATTENTION` means the rule runs and one step would find nothing where it points; `BROKEN`
means the rule cannot run. The distinction decides two things: what the screen says, and
whether the check acts. A `BROKEN` rule that is enabled is disabled by the check — the audit
action `automation.rule_disabled` the streak uses, with `reason: check` where it writes
`consecutive_failures` — and its author is told through `Owners.RuleDisabled`, the same path.
An `ATTENTION` rule is left running: a rule that adds a label which no longer exists fails that
one step under `on_error`, and switching it off for a step it may not even reach would be the
check deciding more than it knows.

**The table from field name to store**, kept in one place (`core/application/service/automation`)
and held by a test to the catalogue - every name in the table is a field some use case declares
with kind `id`, so a rename in the catalogue that the table missed fails the build by name. The
reverse is deliberately not asserted: most `id` fields the catalogue declares are the run's to
supply (`item_id`, `comment_id`, the entry an event is about) and a rule almost never carries
them, so a field the table does not know is simply not resolved:

| Field | Kind |
|---|---|
| `label_id` | label |
| `bucket_id` | bucket |
| `container_id`, `parent_id`, `hub_id`, `collection_id` | container |
| `template_id` | template |
| `subscription_id` | webhook subscription |
| `group_id` | group |
| `account_id`, `assignee_id` | account |
| anything else | not resolved - the run supplies it (§2.2), or nothing of the workspace answers to it |

The store is asked through one port, `automation.References`, with one method,
`Exists(ctx, kind, id)`, implemented in `infrastructure/postgres` as one existence query per
kind under the tenant context — a reference is a fact of this workspace and the resolver can
answer nothing about another's (SG-3).

**Where the findings live: on the rule**, as `findings` (a JSON array of `{level, path, code,
params}`) and `checked_at`, two columns added by an expand migration. Reading the rules then
costs reading the rules: the listing screen, the card, the head of the editor and the card on
the canvas all read what the last check found, and none of them resolves anything. `path` names
what the finding is about in the rule's own address space — `actions/2/then/0`,
`conditions/1`, `trigger`, `run_as` — the same paths the run log and the write-time refusals
use, so an editor points at one thing for all three.

**When the check runs.**

1. **On demand**: `POST /automation/rules:check` checks every rule of the workspace the caller
   may read and answers them with their findings. The rules screen calls it when it opens. That
   is what makes "after an update, the rules that need attention are shown" true without
   anything enumerating tenants: the first person to open the screen after the update asks, and
   the answer is stored for everybody after them.
2. **On deletion**: a subscriber on `label.deleted`, `bucket.deleted` and `container.deleted`
   (the deletion events that exist for the kinds the table names; a template's and a
   subscription's join when their events do) enqueues one `automation.check` job for the
   tenant, deduplicated per tenant so that a bulk deletion is one check, and the job runs the
   same use case as the system. The subscriber writes nothing but the job.
3. **Never on a timer, and never across tenants.**

**What the check does not do.** It does not re-run §2.1's three rights checks — whether the
author may still delegate to the account, whether the account may still do what the actions
do. Those are asked when a rule is switched on and answered per action when it runs; a check
that repeated them would either need to act as the author, which it cannot honestly do, or
would answer a question the run is about to answer better. It does not resolve the values of
`string` parameters that happen to look like references — a strategy name, a message code. And
it does not delete, edit or repair a rule: a finding is information, and repairing is the
author's.

## Consequences

* A rule that a deletion or a version would leave useless is found by the next open of the
  rules screen or the next deletion, said where the rule is, and switched off only where it
  cannot run. Nobody learns about it from five failed runs.
* A deletion costs one job per tenant, not one check per event; a workspace that deletes
  nothing and opens no rules screen pays nothing.
* The field-to-store table is a convention, held to the catalogue by a test in one direction
  only. A field that is *misclassified* is a finding that is wrong, which no test can see - the
  table is short for that reason, and a use case that declares a new reference a rule would
  carry adds its name there.
* `automation.md` gains the check as its §2.3; the coverage report gains `CheckRules`; the
  data catalogue's rule row gains the two columns, which carry no personal data (a path, a
  code and identifiers of the workspace's own objects).
* The client's `AutomationRule` type gains `findings` and `checked_at`, additively. No merge
  rule: the columns are the server's alone and no client writes them.

## Notes

Written for milestone F8 (`docs/backlog/milestone-F8.md`, decision 6) from the prototype the
owner walked on 2026-09-20; built by F8-03 and drawn by F8-07. Put to the owner by name with
the pull request that builds it; accepted by the owner on 2026-09-21, after the F8-08 walk had
run the check's ATTENTION level exactly as written (`docs/evidence/F8-2026-09-20.md`).
