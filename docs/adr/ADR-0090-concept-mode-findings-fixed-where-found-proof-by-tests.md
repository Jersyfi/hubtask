# ADR-0090 — Concept mode, findings fixed where found, proof by tests

**Status:** accepted · **Date:** 2026-10-23 · **Decided:** 2026-10-23, by the owner

**Rule lives in:** [AGENTS.md](../../AGENTS.md) (working rules, "Steps and commits", "Working with
the owner", the loop, "What you do not decide yourself", "Code comments"),
[docs/backlog/README.md](../backlog/README.md) ("A concept built whole", "Tasks", "Decisions",
"Findings"), [docs/usecases/README.md](../usecases/README.md),
[docs/evidence/README.md](../evidence/README.md),
[engineering-guidelines.md](../architecture/engineering-guidelines.md) §2, §3.16, §3.19

Builds on [ADR-0089](./ADR-0089-check-once-where-it-is-cheap.md); the bar it set stays.

## Context

After ADR-0089, work was still slow, and not because of CI. Measured with `make flowstats`:

* A `feat` pull request took a median 21.9 h from first commit to merge: 15.3 h from opened to
  *Ready*, 0.4 h from *Ready* to merged. CI on a pull request took a median 16 min. CI is not the
  bottleneck and stays as it is.
* A fifth of the agents' time was waiting.
* Most findings of milestone UX were small fixes, each filed as an issue and taken up again later
  by a fresh worker instead of being fixed where they were seen.
* Reading every required document completely cost a worker about 190k tokens before any code —
  most of a context that should hand over near 250k.
* Cut into many tasks, a concept pays that start-up cost once per task.

## Decision

The bar does not move: use cases with checks and proof, the DDD and hexagonal structure, arc42 and
the subject documents, contract first, the fifteen rules, every gate and CI stay exactly as strict.
What changes is how the work is cut and what is written on the way.

1. **Concept mode.** On the owner's word a worker builds a whole concept on one branch, in one pull
   request, without cutting it into tasks; the milestone file is optional. Its use cases are
   written or updated and proven by tests. When one context cannot hold it, it is split by screen or
   area, never by a cross-cutting theme.
2. **Findings are fixed where found** — on the branch, in their own commit, test first, named under
   *Premises*. A `finding` issue only for a fix clearly bigger than one step or one that touches an
   item the owner decides.
3. **The owner is asked only when the work is blocked.** Otherwise the worker takes the option that
   changes no rule, gate or use case, builds on, and names the choice in the pull request, where the
   owner may overrule it.
4. **Commits by concern.** The steps are the numbered list on the draft; a commit is one concern,
   pushed when `make check` is green — not one commit per step.
5. **Proof by tests, no files nobody reads again.** A use case check is proven by a test that
   asserts it — unit, integration or an engine walk — and `checked_by` names tests. A user-interface
   change is shown to the owner, never committed as a walk file or screenshots. `docs/evidence/`
   holds only runs no pipeline can make; the files already there stay as history.
6. **Read what the change touches.** The task, its use cases and the `AGENTS.md` of the touched
   directories completely; the domain model, the project structure, the subject document and the
   known traps by the sections the change touches. Comments are short and cite where the reasoning
   lives.

## Consequences

* A concept arrives on `main` as one reviewable change instead of a chain of tasks; its review is
  larger, and the owner reads it against the concept's use cases.
* A choice the owner would have made differently is found in the pull request and changed there,
  not before the build.
* A screen's behaviour is held by an engine walk that runs on every pull request, not by a file
  that describes one run.
* A worker who reads only the touched sections can miss a rule elsewhere; the gates and CI catch
  what they check, and the premise check stays.
* The review (on or after 2026-11-05, with ADR-0089's) compares the next window by `make flowstats`.
  If `fix` pull requests against fresh features rise, or `main` is red more often, a step is taken
  back.

## Alternatives

* **Faster CI**: rejected — CI is 16 minutes of a 21.9-hour median.
* **Fewer use cases or lighter checks**: rejected by the owner; the quality they bought is the point.
* **Concept mode without the milestone's cut checklist**: rejected; the checklist still says what is
  open before the build.
