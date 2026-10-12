# MF-1 — A pull request merges without being brought up to date with `main`

**Task:** MF-1 · no issue: the owner's request of 2026-10-12 · no milestone (CI and the way of working)
**Checked against:** `origin/main` at `138ea75f` on 2026-10-12
**Verdict:** not ready

The owner asked on 2026-10-12 to work out how branch protection's "branch must be up to date"
(`strict`) can be turned off correctly, with every risk described and proven on this repository's
history, and to implement it with the other merge-flow measures right after approval. The setting
itself is changed by the coordinator after the merge, in the order the description names; this
pull request brings everything that must be on `main` before it.

## 1. Premise — what is true today

| Claim | True? | Evidence |
|---|---|---|
| Branch protection requires `CI required` and an up-to-date branch | true | `gh api repos/Jersyfi/hubtask/branches/main/protection`: `strict: true`, `contexts: ["CI required"]` |
| ci-cd.md §4 describes that protection | **false** | it says "linear history, mandatory review"; the API says `required_linear_history: false`, `required_approving_review_count: 0`, and `strict` is in no document → corrected in this pull request (A9) |
| A pull request's CI checks out the merge of its head into the base as the base stood when the run started | true | the `quick` job of run 38281946478 (#1295): `HEAD is now at 0da1c98 Merge 9b9531d7… into 4eb18bb3…`, `4eb18bb3` being `main` then; `actions/checkout` without `ref:` on `pull_request` |
| Nothing re-runs a green pull request's CI when `main` moves | true | `ci.yml` triggers on `opened, synchronize, reopened, ready_for_review` and `push: main` only |
| Every commit on `main` gets a complete pipeline run | **false** | `concurrency: ci-${{ github.ref }}`, `cancel-in-progress: true`; 7 of the last 100 `main` runs ended `cancelled` (`gh api …/runs?branch=main&event=push`) |
| A duplicate migration number is caught before the merge | only through `strict` | no check reads the numbers (`git grep duplicate` in `test/architecture`, `tools/`); goose refuses a duplicate version at run time, so `integration`, `compose`, `e2e` turn red on the merged tree (known-traps.md); `cmd/migrate` uses `goose.WithAllowMissing()` (`cmd/migrate/main.go:194`), so gaps and out-of-order numbers are harmless |
| `GITHUB_TOKEN` can re-run a workflow run | true | `pr-description-rerun.yml` does it; run 37150228555 attempt 2 was triggered by `github-actions[bot]` |
| The event matrix's count sentence makes two clean merges drift from `make generate` | true | replay #1278 below; `tools/eventmatrix/main.go:210` |
| Auto-merge is allowed in the repository | true | `allow_auto_merge: true` |
| ADR-0079 rejected dropping "branches must be up to date" | true | ADR-0079, Options considered, last row: parallel sessions in one bounded context would merge untested combinations — the replay below measures exactly that |

**The replay** (scripts and raw results kept with the analysis, outside the repository): the last
62 squash merges on `main` (#1135…#1300). For each, the head before its last catch-up with `main`
was merged onto `main` as it stood right before the actual merge (`git merge-tree`), i.e. what a
squash with `strict: false` would have produced. Current process (a merge of `main` into the
branch, #1152…#1300): 52 pull requests.

* 44 produce **the identical tree** to the actual squash: the catch-up changed nothing; 14 of them
  had files changed on both sides, and `main` CI ran on that very tree (39 green, 3 cancelled by the
  next push, 1 infra red, 1 outside the listed runs).
* 6 **conflict** textually: GitHub refuses a conflicting pull request whatever `strict` says, so
  these need a catch-up in both worlds. 2 of them (#1261, #1287) conflict only in the event
  matrix's count sentence; with it gone they merge clean and equal to the actual tree.
* 2 produce a **different tree**: #1295 (a stale "N today" in COVERAGE; every fast gate green) and
  #1278 (a duplicate `isMultiTenant` getter: `svelte-check` red; the event matrix's count sentence:
  `make generate` diff). One break in 52 merges.
* No replay holds two migrations or two ADRs with one number; the rebase era (10 earlier pull
  requests) is reported separately, because the head before a rebase is not the work that merged.

## 2. Coverage and the matrix

**(a)** No use case: CI and the way of working. No use case's *Today* names the workflows.

**(b)** Not an access or state change of the product. The states that matter are those of `main`
and of a pull request, each row proven in § 4:

| `main` | the pull request | what happens |
|---|---|---|
| green | its CI green on a merge ref with an older `main` | merges (auto-merge or by hand); `main`'s own run checks the combination |
| red (newest completed run failed) | CI otherwise green | `CI required` fails at its last step, naming the red run; the merge waits |
| red | title `revert…`/`Revert "…"`, or a `Mends main: <sha>` line | passes: it is the fix |
| red → green | its `CI required` failed only at the guard | the after-CI workflow re-runs its failed jobs |
| any | adds a migration or ADR number another open pull request adds | `numbers` fails in the run that sees the other one |
| a run fails, attempt 1 | — | re-run once (known-traps.md, flaky tests); on `main` the "main is red" `finding` is opened at once |

**(c)** Definition of Ready: premise checked (§ 1); no contract, migration, query or dependency; no
use case; the owner's request is the scope; no personal data.

**(d)** Rules: 5, 6, 7 do not apply to scripts and workflows except 7 (every job and every call has
a time limit); 13 (English). AGENTS.md "Merge only on the owner's word" — D4. The workflow rules of
ci-cd.md §4: pinned actions, `timeout-minutes`, least permissions, no `pull_request_target`
(`workflow_run` runs with write rights: it checks out no pull request code and executes no text a
pull request supplies — D6). known-traps.md: "Migration and ADR numbers taken by unmerged branches
collide" (held by a gate after this, so it leaves the list); "`grep -q` after a pipe under
`pipefail`"; "A test's scratch repository inherits `GIT_DIR`" (the new tests use scratch repos).

## 3. Decisions

- [x] D1 Turn `strict` off, with the safeguards below in `main` first — decided by: owner, request
  of 2026-10-12; the setting changes after the merge, by the coordinator
- [x] D2 Every commit on `main` gets its own complete run: the concurrency group is the commit on
  `main`, the ref on a pull request — decided by: worker; without it the first red commit is not
  known and cancelled runs prove nothing. Rejected: `cancel-in-progress: false` alone, which still
  cancels a pending run when a third arrives
- [x] D3 A guard at the end of `CI required`: red while the newest completed `ci.yml` run on `main`
  failed — decided by: worker; at the end because auto-merge acts seconds after it. Rejected: a
  separate job (it would read `main` at the start of the run, minutes earlier), failing on a
  pending `main` (serialises the merges again), a branch-protection setting for the base's age
  (GitHub has none)
- [x] D4 A revert of the commit that turned `main` red may be merged without asking, by the owner's
  standing word written into AGENTS.md — decided by: owner, by merging this pull request, whose
  description names it as the owner's decision it asks for
- [x] D5 A failed run is re-run once automatically; a red `main` opens one `finding` issue "main is
  red", closed by the next green run — decided by: worker, the flake rule of known-traps.md made
  automatic; the nightly's issue pattern
- [x] D6 `workflow_run` is safe here: the job checks out `main`'s scripts (the default branch), reads
  only run ids, attempts and conclusions, and passes them through the environment — decided by:
  worker; ci-cd.md §4 names it
- [x] D7 Migration and ADR numbers: a gate fails a pull request that adds a number another open pull
  request adds, or a tree with two files of one number — decided by: worker. Rejected: timestamp
  migration names (every tool, document and the rule-12 check reads the four-digit numbers; the
  collision rate is low and a check is enough)
- [x] D8 `selftest` starts with `quick` instead of after it (A4a) — decided by: worker; the
  narrower variant that skips it for feature pull requests touches when a gate runs and stays with
  the owner, outside this pull request
- [x] D9 The event matrix and the coverage report carry no counts (A3) — decided by: worker; the
  numbers are derived, and they were the most frequent conflict and the one drift
- [x] D10 Not done: removing the use case index's State column (B2) — it was one of three
  conflicting files in the one case it touched, so it would have saved no catch-up; the readiness
  record's tiers (B1) stay with the owner (ADR-0081)
- [x] D11 A new ADR, because ADR-0079 rejected this option with a reason; the rule goes to
  ci-cd.md §3.4 and §4 and AGENTS.md — decided by: AGENTS.md, "A rule lives in its subject
  document; an ADR records why"

## 4. Proof and steps

* The replay above, re-run from its scripts; its numbers are in the ADR.
* `tools/numbers`: table tests over a scratch repository (duplicates in the tree, a number another
  open pull request adds, the same file in a stacked pull request, its own pull request excluded,
  the API failing) and a run against the open pull requests of the day.
* The guard and the after-CI script: tests in `test/architecture` that run the scripts against a
  stub `gh` serving recorded API answers — green `main`, red `main`, red `main` and a revert title,
  a `Mends main:` line, no completed run, a cancelled newest run; and for the after-CI script:
  attempt 1 re-run, the issue opened, commented, closed, and only guard-failed pull requests re-run.
  The guard is also run once against the real API with run 38152906050 (`main` red on 2026-10-11).
* `ci.yml`: the architecture tests for the concurrency expression and for `selftest`'s position;
  actionlint (gate-architecture); `make verify-pr`.

Steps:

1. docs(backlog): this record
2. ci: every commit on `main` gets its own pipeline run
3. ci: the gate self-test starts with the quick checks, not after them
4. feat(tools): migration and ADR numbers no other open pull request holds
5. ci: `CI required` waits for a green `main`
6. ci: a failed run is re-run once, and a red `main` is filed
7. docs(audit): the event matrix and the coverage report without counts
8. docs: ADR-0083, ci-cd.md, AGENTS.md and known-traps.md for a merge without catching up

## 5. Review

Pending.
