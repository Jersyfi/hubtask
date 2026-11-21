# ADR-0106 — An independent review, a design brief, and the owner on the loop

**Status:** accepted · **Date:** 2026-11-21 · **Decided:** 2026-11-21, by the owner (the concept
[development-flow](../backlog/development-flow/README.md) §6, decisions D-1 to D-15 and D-8a)

**Rule lives in:** [AGENTS.md](../../AGENTS.md) ("Where knowledge lives", the working rules),
[docs/README.md](../README.md) ("How to write"), [docs/adr/README.md](./README.md) (when a decision
gets an ADR), [engineering-guidelines.md](../architecture/engineering-guidelines.md) §3,
[ci-cd.md](../architecture/ci-cd.md) §5, [docs/backlog/README.md](../backlog/README.md) ("A concept
built whole"), [CONTRIBUTING.md](../../CONTRIBUTING.md), and the skills under `.agents/skills/`

**Supersedes in part:** [ADR-0079](./ADR-0079-a-draft-is-checked-locally.md) (CI can be started by
hand on a branch; a draft's push still starts nothing),
[ADR-0089](./ADR-0089-check-once-where-it-is-cheap.md) (decision 4: the author is no longer the
only reviewer; a brief returns for one class of change; the dated review of the merge flow) and
[ADR-0090](./ADR-0090-concept-mode-findings-fixed-where-found-proof-by-tests.md) (a concept meets
CI at checkpoints; the dated review). Everything else in the three stands.

## Context

The owner writes concepts, takes the decisions on the owner's list and looks at results; agents
build. The owner no longer reads the diff. Measured on 2026-11-21 against `main`:

* The author was the only reviewer of a change. A concept build of 339 commits was discarded after
  its builder had judged its own result.
* A concept reached CI once, at its end: one pull request of 134,813 added lines, four failed runs
  before a green one, five fix pull requests in the two days after.
* A pull request's CI run went from a median 9.5 to 33.0 minutes and from 7 % to 33 % failed in
  four weeks. The review that would have seen it was a date somebody had to remember.
* `AGENTS.md` had 259 lines and held procedures beside rules; six standing rules existed only in
  private notes on one machine; nothing said how a text is written or what a change removes.
* The browser walks, about 1,135 of them, ran against hand-written answers under no written rule.

## Decision

The bar does not move: no gate is lowered, no use case check dropped, no reviewer made required,
no model added to CI.

| # | Decision |
|---|---|
| D-1 | A fresh agent that never saw the builder's conversation reviews every change before it leaves draft; the description reports it in a *Review* section. |
| D-2 | A structural change — a migration of an existing table, a port, the sync protocol or a merge rule, authorisation, a new door, a contract change to an existing operation, work across bounded contexts, any concept — starts with a design brief of one screen, challenged once. The class is the kind of change; the bar stays one. |
| D-3 | CI can be started by hand on a branch, so that a concept is checked after each family of screens without leaving draft. |
| D-4 | The browser walks get written rules, each with a gate or a count that only falls. |
| D-5 | A scheduled weekly flow report with thresholds the owner sets replaces the dated reviews. A breach opens a `finding`. |
| D-6 | No job leaves the pull request path. |
| D-7 | `CONTRIBUTING.md` says what is welcome, what happens to a pull request, that work done with an AI tool is welcome on the same terms, and who repairs a refused description. |
| D-8, D-8a | Procedures are skills in the open skill format, one folder each under `.agents/skills/`, linked from `.claude/skills/` and listed in `AGENTS.md`. The review's instructions are a skill; `.claude/agents/reviewer.md` only wraps it. |
| D-9 | Five writing rules, a table of what a change owes and one of what it removes. No length limit on documents. |
| D-10 | A change removes what it makes unused. What a tool proves unused is removed in a pull request of its own. Nothing is removed on a model's judgement. A deleted or weakened test is named in the description. |
| D-11 | Two tools report unused code: the Go team's `deadcode` at a pinned version, and Knip as a development dependency of the workspace. |
| D-12 | CodeRabbit comments on a pull request after the independent review, for a trial of four weeks. It is no required check and decides nothing. The owner installs the app. |
| D-13 | The README shows the CI state, the OpenSSF Scorecard, the proof state generated from the use cases, and how to verify a release. |
| D-14 | Coverage stays a gate at its thresholds and is reported, not badged; no coverage service. CodeQL also reads the clients' languages and the workflows. |
| D-15 | A worker obtains the OpenSSF Best Practices badge from start to end. Nothing shown may need the owner's upkeep; only a consent given under the owner's identity is the owner's act. |

## Consequences

* The owner reads the *Use cases* table, the *Premises* and the *Review* summary instead of the
  diff, and the weekly report instead of single runs.
* A reviewer is a model and misses things. Its section proves that a review ran, not that it was
  good; the report shows whether fixes after features fall.
* Each change costs one more agent run before *Ready*, and a structural one a brief before the
  code. The brief is one screen and one challenge so that it does not grow back into the readiness
  record ADR-0089 removed.
* A third party with write access to the repository comments on pull requests during the trial.
* The whole change is one pull request and migrates no data; reverting it takes it back.

## Alternatives

* **A required approving review or a model in CI:** rejected. A pull request from a fork gets no
  secrets, so no model can run on it in CI, and a required reviewer would be the owner reading
  diffs again.
* **A length limit on documents:** rejected. A limit makes a model compress, and compressed text is
  the kind a person cannot read.
* **Cleanup by a model's judgement:** rejected. A model cannot tell unused from not yet used, and
  deleting a test is the cheapest way to turn a red build green.
