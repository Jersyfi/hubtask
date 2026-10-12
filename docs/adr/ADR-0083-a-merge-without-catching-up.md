# ADR-0083 — A pull request merges without catching up with `main`

**Status:** accepted · **Date:** 2026-10-12 · **Decided:** 2026-10-12, by the owner (asked for; accepted by merging its implementation)

**Rule lives in:** [ci-cd.md](../architecture/ci-cd.md) §3.4, §3.5, §4

Amends [ADR-0079](./ADR-0079-a-draft-is-checked-locally.md), whose options rejected dropping
"branches must be up to date".

## Context

Branch protection on `main` requires `CI required` and that a branch be up to date with `main`
(`strict`). Every merge therefore forces the next pull request to merge `main` in, and, since the
hook asks `make verify-pr` for the commit that leaves draft, to run it again: on 2026-10-12, 31, 22
and 33 minutes of local runs sat between merges that CI then confirmed in ten. Merges ran one at a
time, at about one an hour. ADR-0079 kept `strict` because parallel sessions in one bounded context
would merge combinations nobody tested. That fear can be measured.

**The replay.** For the last 62 squash merges (#1135…#1300), the head each pull request had before
its last catch-up with `main` was merged with `git merge-tree` onto `main` as it stood right before
the actual merge — what a squash without `strict` would have produced — and compared with the tree
that actually landed. For the 52 of the current process (`main` merged into the branch, #1152 on):

| Outcome | Pull requests | What it means |
|---|---|---|
| The identical tree | 44 (10 had no catch-up, 12 had files changed on both sides) | `main`'s own run on that tree is the proof: 39 green, 3 cancelled by the next push, 1 red by an outage, 1 outside the listed runs |
| A textual conflict | 6 | GitHub refuses a conflicting merge with or without `strict`: a catch-up in both worlds. 2 of them only in the event matrix's count sentence |
| A different tree, the fast gates green | 1 (#1295) | a stale "N today" in the coverage report, which no gate reads |
| A different tree, red | 1 (#1278) | a getter added twice (`svelte-check`), and the count sentence (`make generate` differs) |

The fast gates — build, vet, `make generate` without a diff, `gate-docs`, `test/architecture`, the
touched packages' tests, the web app's typecheck and build — ran on the two different trees. No
replay held two migrations or two ADRs with one number. The ten pull requests of the earlier rebase
era are left out: the head before a rebase is not the work that merged.

So one merge in 52 would have turned `main` red, and two conflicts in six were a generated count.
Three things are not measured by the replay and are designed for instead: a number two open pull
requests both take (it happened to ADR-0078/0079), a `main` run cancelled by the next merge (7 of
the last 100 were), and what happens while `main` is red.

## Decision

1. **`strict` is off.** A pull request merges when `CI required` is green on its last run, however
   far `main` has moved since. That run checked the merge of the branch with `main` as it was when
   the run's event created the merge commit; a re-run repeats that commit. A pull request is brought
   up to date only when GitHub reports a conflict, or when its base was squash-merged.
2. **Every commit on `main` gets its own complete run**: the concurrency group is the commit on
   `main` and the ref on a pull request, and only a pull request's older run is cancelled.
3. **`CI required` waits for a green `main`.** Its last step, on a pull request only, fails while
   the newest deciding run on `main` is red — failed, timed out, or re-running a failure — unless
   the pull request is a revert or its description has a line `Mends main: <sha>`.
4. **After every CI run** (`after-ci.yml`): a failed run is re-run once (not a pull request's run
   that failed only at the guard); a red `main` opens the issue "CI: main is red" and re-runs `CI
   required` of every green ready pull request, which the guard then holds; a green `main` closes
   the issue and re-runs the pull requests that waited. A schedule sweeps the same way every 15
   minutes, because GitHub starts no workflow for what its own token caused.
5. **No number twice**: the `numbers` job fails a pull request that adds a migration or ADR number
   another open pull request adds, and any tree with two files of one number.
6. **No count in a generated or shared file**: the event matrix and the coverage report drop theirs.
7. **`selftest` starts beside `quick`**, which takes about three minutes off every run.

Merging stays the owner's word; whether a revert of the commit that turned `main` red may merge
without asking is the owner's question, outside this decision.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: `strict` off, with the six safeguards** | One break in 52 merges on the history; it is found by `main`'s run in about ten minutes, named by commit, and nothing merges on top until it is fixed. Merges no longer wait for each other. |
| Keep `strict`, mark ready after the first `verify-pr` and catch up without a second local run | Halves the cycle, keeps it serial: each merge still waits for the previous one's CI. |
| GitHub's merge queue | Tests each combination before it lands — offered only to an organisation's repository; this one belongs to a person. Moving it touches the module path of every Go file. |
| Fail `CI required` while a run on `main` is still going | Equal to `strict` with a queue of one: merges wait for each other again. |
| A rule on the age of the base a pull request was tested against | GitHub has no such setting; a check cannot change after it reported. |
| Timestamp migration names instead of the numbers check | Every tool, document and the rule-12 check reads the four-digit numbers; the check is enough. |
| Deploy integration only after a green run on `main` | Integration would stop at every flake, and deployment.md §3 promises every push; integration runs an untested combination for the minutes `main`'s run takes instead. |

## Consequences

**Positive**

* A pull request merges about ten minutes after it is ready (its own CI), whatever merged in
  between: several an hour, limited by CI and by the owner's word, no longer by each other.
* The local second run of `make verify-pr` after a catch-up is gone; a catch-up is needed in about
  one merge in nine on the history (6 of 52), one in thirteen without the count sentence (4 of 52).
* Every commit on `main` is proven or named red on its own.

**Negative / countermeasures**

* *An untested combination can turn `main` red* (one in 52 on the history). → `main`'s run names
  the commit; the issue says how to revert; the guard keeps everything else from landing on top
  until it is green.
* *Pull requests wait while `main` is red*, at best about twenty minutes: `main`'s run, its re-run,
  and the fix's own run. → The after-CI workflow releases them without a person.
* *`main` runs every commit*: about 25 jobs per merge that a cancelled run did not run before. The
  runner queue was about zero over 5,721 jobs; if it grows, the jobs' own filters are the lever.
* *Integration may run a combination no gate has seen*, for the minutes `main`'s run takes. → A red
  run is filed; the next merge deploys again.
* *A flake absorbed by the automatic re-run leaves no red cross.* → The run's second attempt shows
  it; known-traps.md keeps the flake table.
* *An administrator's merge bypasses `CI required`* (`enforce_admins` is off), as it did before.

## Proof

* The replay above.
* `tools/numbers`, against GitHub's open pull requests of 2026-10-12: a branch adding `0137_…`
  failed naming #1297, one adding `0199_…` passed, a tree with two `0135` files failed.
* The guard against the real API: green on 2026-10-12; as of 2026-10-11T16:30Z it named run
  38152906050 red.
* The guard and the after-CI script against a stand-in `gh` in `test/architecture`, every case of
  the decision; each of fourteen deliberate mutations of the two scripts turns those tests red.
* That GitHub starts `after-ci.yml` for a run's end is proven by its first run on `main`; the
  schedule does not depend on it.
