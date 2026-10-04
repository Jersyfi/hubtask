# <TASK> — <title> · readiness record

**Task:** <TASK> · issue #<n> · milestone file `docs/backlog/milestone-<X>.md`
**Size:** L
**Verdict:** not ready
**Checked:** <YYYY-MM-DD> against `main` at `<sha>` · reviewer: <how the independent review ran>

<!-- Size is M or L (README § "Size"). The verdict is exactly one of: ready / waiting on the owner /
     not ready. `ready` only when every section below says something, § 8 has no `- [ ]`, and the
     reviewer's findings are answered. Everything inside these comments is guidance and is removed
     by the gate before it reads: a section that holds only its comment is empty, and refused.
     Write "none" in a section that truly has nothing, and say why. -->

## 1. Premise — what is true today

<!-- Every statement the task, its ADRs and the named use cases' *Today* make about the current
     system, checked against the code. One line each: the claim, true or false, the evidence
     (file:line, a query, a test that ran). A false one is corrected in its document in this
     task's pull request. Grep for "nothing does", "only", "never", every quoted number, every
     named function, action and check number. -->

## 2. Coverage — every check and promise this task touches

<!-- Every check of every named use case - not only the cited ones - and every documented promise
     the change touches. "Carried by": this task / <other task> / Where it ends / a later
     milestone that exists. Nothing is "probably elsewhere".

     | Source | Item | Carried by |
     |---|---|---|
     | UC-...-.. check n | ... | this task |

     Then the cross-cutting rows (engineering-guidelines.md §2), each answered or "none":
     API (operations, fields, error codes; versioning-release for a break) · events · permissions
     (role, scope) · message codes · tenancy (tenant_id path, RLS) · automation (action, trigger)
     · migration (expand/contract) · security (threats T-xx) · failure behaviour (dependency down)
     · personal data (catalogue, deletion path) · audit (action) · offline sync (merge rule) ·
     client matrix · backup, restore and import. -->

## 3. Doors and states

<!-- States the entity or account can be in, including the lifecycle (created, invited, active,
     disabled, deleted, restored, imported, pending deletion, suspended) and the feature's own
     (e.g. offered / withdrawing / ended). Doors that reach it: REST, MCP, hubctl, the web app,
     automation, the worker, in-flight steps (pending credentials, a second step), deprecated
     paths, the installation's and the workspace's level. Kinds of actor: password, provider-only,
     with and without a second factor, token, agent. Deployments D1-D7 where they differ (no mail
     server, one workspace, many). Failures: mail, provider, storage down.

     | Door x state x kind | What happens | Proven by |
     |---|---|---|

     An empty cell is a decision: § 8, decided by the cheapest decider (README rule 2). -->

## 4. Standing rules

<!-- One line each, how it holds for this change: the rules that do not bend (CLAUDE.md), the
     principles' "Broken when" lines (docs/vision/principles.md), the non-goals, and every entry
     of docs/backlog/standing-decisions.md that applies - among them that nobody is locked out
     (ADR-0077 §4): for every state x kind in § 3 the change could close, the way in that
     remains. -->

## 5. Concurrency and transactions

<!-- Every read-then-write, every check-then-act across two transactions, every write on a refusal
     path (does it survive the rollback?), every count two requests can move at once - and how
     each stays correct. -->

## 6. Known traps

<!-- The entries of docs/architecture/known-traps.md that apply, and how this task avoids each. -->

## 7. Acceptance — and how each is proven

<!-- | Criterion | Test or walk that proves it | Can it pass? |
     |---|---|---|
     Every "later" this task promises (a migration that drops something, a follow-up) is cut as a
     task now and named here. -->

## 8. Decisions

<!-- One line per decision, deciders in this order (README rule 2):
     - [x] D1 - the question - decided by <document § and sentence>
     - [x] D2 - the question - decided by precedent <file:line>
     - [x] D3 - the question - decided by the owner <date, standing-decisions.md or record>
     - [x] D4 - the question - worker decision: <choice>; because <reason>; rejected <alternative>
     - [ ] D5 - the question - owner (on the owner's list because <which item>) - proposal in the
           form of /decision; also listed under the milestone's "## Open decisions" -->

## 9. Independent review

<!-- How it ran, what the reviewer found, and how each finding was answered - by changing the
     record, not by arguing. -->

## 10. Escapes

<!-- Filled during and after the build: every finding this record should have caught.
     Classes D G C O P N R (README). -->

| # | Found by | Class | What | Row of § 1–7 that should have caught it | Handled |
|---|---|---|---|---|---|
