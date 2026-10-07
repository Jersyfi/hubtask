# <TASK> — <title>

**Task:** <TASK> · issue #<n> · `docs/backlog/milestone-<X>.md`
**Checked against:** `<base branch>` at `<sha>` on <YYYY-MM-DD>
**Verdict:** not ready <!-- exactly one of: ready / waiting on the owner / not ready -->

<!-- Write this record BEFORE the first line of code, as the branch's first commit. It is a snapshot:
     after the merge nobody updates it, and its durable results live elsewhere (see § 3).
     Keep it as short as the task allows. Comments are guidance and are stripped before the gate
     reads the file. "none — <why>" is a valid answer; an empty section is not. -->

## 1. Premise — what is true today

<!-- Every claim the task text, its ADRs and the named use cases' *Today* make about the current
     system, checked against the code: claim · true/false · evidence (file:line, a query, a run).
     A false claim is a CORRECTION: fix it in the task text or document in this same commit.
     A claim that is true but a worse design choice is a DECISION: § 3. -->

## 2. Coverage and the matrix

<!-- (a) EVERY check of every named use case — not only the numbers the task cites — and every
         use case whose *Today* names the code you touch → carried by: this task / <task> /
         "Where it ends" / nobody (then § 3 decides).
     (b) Only if the task touches access, a lifecycle or a state: the rows that matter of
         door (web app, REST, MCP, hubctl, automation, worker/job, installation level) ×
         state × actor kind × deployment D1–D7. Name the dimensions you collapsed and why, in one
         line. Each row: what happens · how it is proven.
     (c) engineering-guidelines.md § 2 (the Definition of Ready), each item in one word or line.
     (d) Standing rules: rules 1–15 and the principles' "Broken when" that this change could
         break; every entry of docs/architecture/known-traps.md that applies; two writers at once
         (which statement guards it, what the loser sees). -->

## 3. Decisions

<!-- Every open point from 1–2, each with its decider:
       - [x] D1 … — decided by: <document, file:§>
       - [x] D2 … — decided by: owner, <date> → written in <file:§>
       - [x] D3 … — decided by: worker — reason; rejected alternative
       - [ ] O1 … — waiting on the owner: decision issue #<n>
     Only items on the owner's list in AGENTS.md ("What you do not decide yourself") may stay open.
     Before opening a decision issue, search: docs/vision, docs/adr, docs/usecases, AGENTS.md, the
     milestones' "Decisions", and closed `decision` issues. A worker decision that the next reader of
     the code needs goes into a code comment or the document as well, not only here. -->

## 4. Proof and steps

<!-- For each use case check this task carries: the test or walk that proves it, and why it CAN:
     through the registry, not Handler.Invoke; a real transaction, not the fake unit of work; not as
     a superuser where RLS matters; the stored shape, not the value written; a mechanism that exists
     (does the package have the kind of test you name?). Edge rows (month ends, zones, empty lists).
     Then the steps, inside out, one commit each, each one building. A step that adds a use case
     lands it end to end - domain, use case, descriptor, registration, catalogue, REST handler and
     their tests in one commit - because the parity test fails a descriptor missing from any of
     them. -->

## 5. Review

<!-- Who attacked this record without having written it — a second agent run with a fresh context,
     or a person other than the owner — given only this record, the task and the repository. Every
     finding with what changed. "No findings" needs the list of what was checked. -->
