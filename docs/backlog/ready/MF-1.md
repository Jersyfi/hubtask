# MF-1 — A pull request merges without being brought up to date with `main`

**Task:** MF-1 · no issue: the owner's request of 2026-10-12 · no milestone (CI and the way of working)
**Checked against:** `origin/main` at `138ea75f` on 2026-10-12
**Verdict:** ready

The owner asked on 2026-10-12 to work out how branch protection's "branch must be up to date"
(`strict`) can be turned off correctly, with every risk described and proven on this repository's
history, and to implement it with the other merge-flow measures right after approval. The setting
itself is changed by the coordinator after the merge, in the order of § 4; this pull request brings
everything that must be on `main` before it.

## 1. Premise — what is true today

| Claim | True? | Evidence |
|---|---|---|
| Branch protection requires `CI required` and an up-to-date branch | true | `gh api repos/Jersyfi/hubtask/branches/main/protection`: `strict: true`, `contexts: ["CI required"]`, `enforce_admins: false` |
| ci-cd.md §4 describes that protection | **false** | it says "linear history, mandatory review"; the API says `required_linear_history: false`, `required_approving_review_count: 0`; `strict` is in no document → corrected in this pull request (A9) |
| ci-cd.md §4: "Allow select actions" with an allowlist | **false** | `gh api repos/Jersyfi/hubtask/actions/permissions`: `allowed_actions: "all"`. Which side is wrong is a security setting's question → a `finding`, not corrected here |
| A pull request's CI checks out the merge of its head into the base as it stood when the event created the merge ref | true | the `quick` job of run 38281946478 (#1295): `HEAD is now at 0da1c98 Merge 9b9531d7… into 4eb18bb3…`, `4eb18bb3` being `main` then. A re-run repeats that merge commit; only a push makes a new one |
| Nothing re-runs a green pull request's CI when `main` moves | true | `ci.yml` triggers on `opened, synchronize, reopened, ready_for_review` and `push: main` only |
| Every commit on `main` gets a complete pipeline run | **false** | `concurrency: ci-${{ github.ref }}`, `cancel-in-progress: true`; 7 of the last 100 `main` runs ended `cancelled` |
| A duplicate migration number is caught before the merge | only through `strict` | no check reads the numbers; goose refuses a duplicate version at run time (`integration`, `compose`, `e2e` red on the merged tree, known-traps.md); `cmd/migrate/main.go:194` uses `goose.WithAllowMissing()`, so gaps and out-of-order numbers are harmless. ADR-0079 itself was renumbered from 0078 after a collision |
| `GITHUB_TOKEN` can re-run a workflow run | true | `pr-description-rerun.yml` does it; run 37150228555 attempt 2 was triggered by `github-actions[bot]` |
| GitHub starts a `workflow_run` for the end of a run its own token re-ran | **not known** | events caused by `GITHUB_TOKEN` start no workflow, with documented exceptions that do not name a re-run; nothing here relies on it (D5: a schedule sweeps) |
| The event matrix's count sentence makes two clean merges drift from `make generate` | true | replay #1278 below; `tools/eventmatrix/main.go:210` |
| Auto-merge is allowed; a merge queue is not available | true | `allow_auto_merge: true`; the repository belongs to a `User`, and GitHub offers the merge queue to organisations' repositories |
| deploy.yml deploys "code the gates have seen" | **false after this** | it deploys every push to `main` while `main`'s run is still going (deploy.yml:3-6) → the comment is corrected, D12 |
| ADR-0079 rejected dropping "branches must be up to date" | true | ADR-0079, Options considered, last row — the replay measures exactly what it feared |

**The replay** (method in ADR-0083; scripts and raw results with the analysis handed to the owner):
the last 62 squash merges on `main` (#1135…#1300). For each, the head before its last catch-up with
`main` was merged onto `main` as it stood right before the actual merge (`git merge-tree`) — what a
squash with `strict: false` would have produced. Current process (a merge of `main` into the
branch): 52 pull requests, #1152…#1300.

* 44 produce **the identical tree** to the actual squash, so `main`'s own run on that tree is their
  proof (39 green, 3 cancelled by the next push, 1 red by an outage, 1 outside the listed runs). 10
  of them had no catch-up at all; in the other 34 the catch-up changed nothing; 12 had files
  changed on both sides.
* 6 **conflict** textually: GitHub refuses a conflicting pull request whatever `strict` says, so
  they need a catch-up in both worlds. 2 (#1261, #1287) conflict only in the event matrix's count
  sentence; without it they merge clean and equal to the actual tree.
* 2 produce a **different tree**: #1295 (a stale "N today" in COVERAGE; the fast gates green) and
  #1278 (a duplicate `isMultiTenant` getter: `svelte-check` red; the count sentence: `make
  generate` diff). The fast gates run on them: build, vet, generate without a diff, gate-docs,
  test/architecture, the touched packages' tests, the web app's typecheck and build.
* One break in 52 merges. No replay holds two migrations or two ADRs with one number. The 10
  pull requests of the rebase era are reported apart: the head before a rebase is not the work that
  merged.

## 2. Coverage and the matrix

**(a)** No use case: CI and the way of working. No use case's *Today* names the workflows.

**(b)** The states that matter are those of `main` and of a pull request; each row is a test in § 4.

| `main` | the pull request | what happens |
|---|---|---|
| green | CI green on a merge ref with an older `main` | merges; `main`'s own run checks the combination |
| red: the newest deciding run failed, timed out, or is re-running a failure | finishes its run | `CI required` fails at its last step, naming the red run |
| red | title `revert…`/`Revert "…"`, or a line `Mends main: <sha>` | passes: it is the fix; editing the description re-runs `CI required` (pr-description-rerun.yml) |
| turns red | already green, maybe hours ago | the after-CI script re-runs its `CI required` alone; the guard turns it red |
| red → green | failed at the guard alone | its failed jobs are re-run |
| any | the push to `main` itself | the guard does not run (a step condition): `main` must be able to turn green |
| any | adds a migration or ADR number another open pull request adds | `numbers` fails in the run that looks second |
| — | a run fails, attempt 1 | re-run once, except a pull request's run that failed at the guard alone; a red `main` is filed at once |
| — | a Dependabot or fork run | the token is read-only: a refused re-run or issue write is a warning; the schedule's sweep reads `main` with its own token |

**(c)** Definition of Ready: premise checked (§ 1); no contract, migration, query or dependency; no
use case; the owner's request is the scope; no personal data.

**(d)** Rules: 6 does not apply (no outbound HTTP from the product); 7 — every job has
`timeout-minutes`, `tools/numbers` calls git and gh with a context deadline, the scripts run inside
their job's limit; 13 (English). The hardening of ci-cd.md §4: pinned actions; least permissions
(`CI required` gains `actions: read` and `pull-requests: read`; the after-CI job `actions: write`
and `issues: write`); no `pull_request_target` — `workflow_run` runs with write rights, so it
checks out `main` (the default branch) only and reads ids, attempts and conclusions through the
environment, D6. known-traps.md: the migration and ADR number row leaves the list (a gate holds it;
its advice on `DIRTY` pull requests stays as a row of its own); "`grep -q`
after a pipe"; "a test's scratch repository inherits `GIT_DIR`".

## 3. Decisions

- [x] D1 Turn `strict` off, with the safeguards below in `main` first — decided by: owner,
  2026-10-12: the merge-flow concept and this pull request accepted ("das merge flow concept
  akzeptiere ich ... direkt umsetzen"), so ADR-0083 is accepted and `strict` goes off after the
  merge, by the coordinator, in the order of § 4; a review of the flow after two weeks, filed as an
  issue by the coordinator
- [x] D2 Every commit on `main` gets its own complete run: the concurrency group is the commit on
  `main`, the ref on a pull request — decided by: worker; without it the first red commit is not
  known. Cost: about 25 jobs per merge that were cancelled before; the runner queue was ~0 over 5,721
  jobs. Rejected: `cancel-in-progress: false` alone, which still replaces a pending run
- [x] D3 A guard as the last step of `CI required`: red while `main` is red (§ 2(b)) — decided by:
  worker; there because auto-merge acts seconds after `CI required` turns green, and a job of its own
  would have to sit in `CI required`'s `needs` and add nothing. Rejected: failing on a pending
  `main` (serialises the merges again); a branch-protection rule on the base's age (GitHub has
  none); the merge queue (not offered to this repository)
- [x] D4 Not decided here: a standing word for merging a revert of the commit that turned `main`
  red. "Merge only on the owner's word" stands; the issue says so. Whether reverts may merge without
  asking is the owner's question, put as a `decision` issue by the coordinator; nothing in this pull
  request depends on its answer
- [x] D5 A failed run is re-run once automatically, a red `main` opens one `finding` issue "CI: main
  is red" at once and holds the green pull requests, a green `main` closes it — decided by: worker,
  the flake rule of known-traps.md made automatic. A schedule (every 15 minutes) sweeps the same
  way, because nothing guarantees a `workflow_run` for a re-run's end. A flake a re-run absorbs stays
  visible as the run's second attempt
- [x] D6 `workflow_run` is safe here: the job checks out `main`'s scripts, reads only run ids,
  attempts and conclusions, passes them through the environment, and reads titles and descriptions
  from the API — decided by: worker; ci-cd.md §4 names it
- [x] D7 Migration and ADR numbers: a job fails a pull request that adds a number another open pull
  request adds (only added or renamed files, every page of the file list), or a tree with two files
  of one number — decided by: worker. Rejected: timestamp migration names (every tool, document and
  the rule-12 check reads the four-digit numbers; a check is enough)
- [x] D8 `selftest` starts with `quick` instead of after it (A4a) — decided by: worker; the
  variant that skips it for feature pull requests touches when a gate runs and stays with the owner
- [x] D9 The event matrix and the coverage report carry no counts (A3) — decided by: worker
- [x] D10 Not done: removing the use case index's State column (B2) — in the one replay it touched
  it was one of three conflicting files, so it saves no catch-up; the readiness record's tiers (B1)
  stay with the owner (ADR-0081)
- [x] D11 A new ADR, because ADR-0079 rejected this option with a reason; the rule goes to
  ci-cd.md §3.4 and §4 — decided by: AGENTS.md, "a rule lives in its subject document"
- [x] D12 deploy.yml keeps deploying every push: integration may run a combination for the minutes
  `main`'s run takes; a red run is filed and the next merge redeploys — decided by: worker; its
  comment is corrected. Rejected: deploying on `workflow_run` of a green run, which would stop
  integration at every flake and changes deployment.md §3

## 4. Proof and steps

* The replay above.
* `tools/numbers`: table tests, and a scratch repository run of the whole check (the branch's own
  pull request left out, GitHub failing is an error, on `main` only the tree); then against
  GitHub's open pull requests of the day: a branch adding `0137_…` fails naming #1297, one adding
  `0199_…` passes, a tree with two `0135` files fails with `-open=false`.
* The guard and the after-CI script: tests in `test/architecture` against a stand-in `gh` that logs
  every write — every row of § 2(b); each of fourteen deliberate mutations of the scripts turns them
  red. The guard against the real API: green today; red as of 2026-10-11T16:30Z (`AS_OF`), naming
  run 38152906050.
* `ci.yml`: tests for the concurrency expression (red against the old file) and for the guard's
  step condition; actionlint; `make verify-pr`.

Steps:

1. docs(backlog): this record
2. ci: every commit on `main` gets its own pipeline run
3. ci: the gate self-test starts with the quick checks, not after them
4. feat(tools): migration and ADR numbers no other open pull request holds
5. ci: `CI required` waits for a green `main`
6. ci: a failed run is re-run once, and a red `main` is filed
7. docs(audit): the event matrix and the coverage report without counts
8. docs: ADR-0083 and the subject documents for a merge without catching up

After the merge, the coordinator, in this order (the commands are in the pull request):

1. Watch the first `main` run under the per-commit group go green, the `numbers` job included.
2. Run the after-CI workflow's sweep once by hand (`workflow_dispatch`) and see it read `main` green.
3. Turn `strict` off.
4. For the next ten merges, read each `main` run; a red one follows the issue's steps.
5. Turn `strict` back on (one command) if `main` is red more than twice in a week from untested
   combinations, or if the guard or the after-CI workflow misbehaves.

## 5. Review

Attacked by a second agent with a fresh context, given this record, the task and the repository, on
2026-10-12 (at commit `5b1cd227`). Its findings and what changed:

1. A pull request green before `main` turned red still merges → the after-CI script re-runs `CI
   required` of every green ready pull request when `main` turns red (D5, § 2(b)).
2. The guard on a push to `main` would keep `main` red for ever → a step condition, and a test.
3. D4 decided an owner question by the merge → withdrawn; the rule stands, the question goes to the
   owner as a `decision` issue.
4. "Newest" undefined; a re-run in progress; cancelled/timed-out runs; API errors; a fork's branch
   named `main` → defined in the guard's header and § 2(b), each a test; the API failing fails closed.
5. `workflow_run` for a re-run's end is not proven → § 1 says so; a scheduled sweep and
   `workflow_dispatch` make recovery independent of it.
6. Forks, Dependabot, cancelled runs, refused writes, duplicate issues, stale payloads → refused
   writes are warnings; only `failure` is re-run; the issue is found through the issue list, not the
   search; titles and descriptions are read from the API.
7. deploy.yml → D12, its comment corrected.
8. A re-run hides flakes; a guard failure re-run at once → no immediate re-run of a guard-only
   failure; the attempt stays visible (D5).
9. `numbers`: added files only, pagination, the job's place in `ci-required` and `tools/cilocal` →
   in the code and step 4.
10. The replay's scripts outside the repository; miscounts → counts corrected; the method is in
    ADR-0083 and the scripts go to the owner with the analysis, no "re-run from its scripts" claim.
11. Order and undo → the list above.
12. ci-cd.md §4's allowlist; `enforce_admins`; the merge queue → § 1, D3; the allowlist is a
    `finding` for the coordinator to file.
13. The guard's permissions → § 2(d).
14. A re-run never refreshes the base → § 1 and the after-CI script's header.
15. The real-API proof → it reads the state as of a time, so it proves the selection too (`AS_OF`).
16. Scope and step size: the self-test and the counts stay, the owner asked for the measures
    together; one concern per commit. Labels: the issue's label `finding` exists.
17. D3's wording → corrected.
18. D2's cost → stated.

Checked and found correct by the reviewer: the protection and repository settings, the cancelled
runs, the triggers, the cited code and runs, ADR-0079's rejected option, every replay number but
the two corrected, today's free numbers, the architecture tests' constraints, D2's and D7's
rejected alternatives, D10.
