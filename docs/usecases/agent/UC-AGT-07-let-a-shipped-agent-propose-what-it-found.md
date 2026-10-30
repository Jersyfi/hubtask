---
id: UC-AGT-07
title: Let a shipped agent propose what it found, for a person to apply
context: agent
actors: [PE-admin, PE-member, PE-agent, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: built
tasks: []
checked_by: [core/domain/model/agent/Job_test.go, core/application/service/agent/Runner_test.go, test/integration/agent_repository_test.go, apps/webapp/src/lib/data/agents.test.ts]
---

# Let a shipped agent propose what it found, for a person to apply

## Goal

The person an agent of the product acts for, or an administrator, sets it to work on a ticket or on
a hub's inventory; the agent reads what it may, as itself, and leaves what it would change - labels,
the configuration items a ticket names, which of two duplicate CIs stays, a knowledge page drawn
from a resolved ticket - for a person to approve, and never changes anything itself.

## Story

Ana starts *Triage* on a new ticket about the printer on floor 3. A minute later the ticket carries
a suggestion of labels, and among the agent's requests waits a comment naming the CIs *Printer* and
*Router-2*. She approves the comment and dismisses nothing else. Once the ticket is resolved, the
knowledge agent proposes a page from it in the hub's space.

## How to check

1. A job of a shipped agent is started by its owner or an administrator, never by an agent, on an
   entry - ticket triage, CI suggestion, knowledge draft - or a hub - duplicate review; an agent
   built elsewhere runs none (`agents.runs_elsewhere`).
2. A job is refused at its start, and ends refused at its run, where the agent is stopped, not live
   or its model changed, where its tokens for the day are spent, or where it already runs as many
   jobs as its runs at once allow (`agents.budget_spent`); the tokens a job spent are the job's and
   a row of the ledger.
3. The job acts as the agent's own account for its person, through the gate: every write it attempts
   waits for a person among the agent's requests, whatever its policy would allow; nothing is
   applied by the job itself.
4. Ticket triage proposes labels by the workspace's classifier as a suggestion on the entry and the
   CIs of the entry's hub its title or notes name as a comment; CI suggestion proposes those CIs;
   duplicate review proposes, for each open duplicate, the CI a source saw last - else the oldest -
   as the one that stays; knowledge draft proposes a page in the entry's hub from a resolved entry's
   title and notes, with a reference back, and nothing from an open one (`agents.job_entry_open`).
5. A job keeps its target, how it ended, how many proposals it left and the tokens it spent - never
   what it read or what a model answered -, stays in its workspace and joins its archive.
6. REST, MCP and hubctl start a job and list an agent's jobs alike.
7. The web app's administration lists an agent's jobs and lets its owner or an administrator start
   one on an entry or a hub.
8. An automation rule starts a job on the ticket its run is about - its arrival (`item.created`) or
   its resolution - with the action `START_AGENT_JOB`, the run supplying the entry; a hub's job is
   not started from an entry (`agents.target_invalid`), and a start naming no target is refused
   (`agents.target_required`). The rule's account is held to the owner's or an administrator's right.

## Where it ends

* A job runs when somebody or a rule starts it; nothing starts one without a rule saying so.
* Routing a ticket to a queue by its offering is not proposed.
* No agent-to-agent protocol and no orchestration of several agents.
