# The owner's acceptance — 2026-10-28

**The owner's word (2026-10-28, translated):** "Regarding the decisions — you can take them out,
because I am working out the concept together with you and therefore also accept it." Together
with: "I want this session to implement the complete concept at the end and to steer its
subagents for it."

Recorded as: the concept now in `docs/backlog/suite/` (suite, configuration model, products,
foundation) is accepted as a whole and is to be built in concept mode (`AGENTS.md`, "Working with
the owner"). Each question below ([`decisions.md`](./decisions.md)) is answered by the option the concept
proposes.

| # | Decided as proposed |
|---|---|
| D-01 | Hubtask becomes a work suite; vision sentence, promise 6, P-17 *Depth is switched on and starts from a default that works* |
| D-02 | In a workflow collection, completion follows the status category |
| D-03 | Modules per workspace and hub; plans offer and lock modules; without a plan everything is available |
| D-04 | A Yjs-compatible CRDT for live editing (dependencies listed in `foundation/README.md` §3) |
| D-05 | Custom roles from a permission catalogue; nobody grants a permission they do not hold |
| D-06 | New non-goals: chat, meetings, invoicing, in-process plugins, spreadsheet engine, sales CRM |
| D-07 | Types without completion (records) exist — realised as the *record* behaviour of F-1 |
| D-08 | Dependencies become planned (`UC-WRK-03` *Where it ends* corrected) |
| D-09 | Personas `PE-planner`, `PE-requester`, `PE-handler` added |
| D-10 | Migration from issue trackers and wikis (configuration, history, attachments) in the first wave |
| D-11 | P-18 *One feature, one building block, every door* |
| D-12 | P-19 *Everything is an object you can point at* |
| D-13 | A type's form binds every door, capture and the API included |
| D-14 | One settings catalogue for every level |
| D-15 | The configuration document with plan and apply for installation, workspace, hub |
| D-16 | An agent's plan waits for a person's approval by default |
| D-17 | Templates are copied with provenance (`template@version`) and upgraded by a three-way comparison — the owner's copy semantics of #1619–#1622 kept |
| D-18 | The administration regrouped into six groups |

**Still the owner's at the moment it arises** (`AGENTS.md`, "What you do not decide yourself"),
because no concept can settle them in advance: a lifecycle policy that would irrecoverably delete
records, the retention of agent run traces, a change to the security gates, a dependency that fails
the licence gate, and merging each pull request.

**Cyber Resilience Act role:** the project stays without a paid edition (ADR-0080), so it acts as an
open-source steward; providers who sell it are manufacturers themselves and receive from the
project what they need (SBOM, advisories, support period, CVD) — F-13.
