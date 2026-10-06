# ADR-0079 — A draft is checked locally; CI runs when a pull request is ready

**Status:** accepted · **Date:** 2026-10-04 · **Accepted:** 2026-10-06

**Rule lives in:** [ci-cd.md](../architecture/ci-cd.md) §3.4, §3.1, §3.2

Approved as ADR-0078; renumbered before merging, because #1147 brought its own ADR-0078 (the ways
back in) to `main` first.

Amends [`ci-cd.md`](../architecture/ci-cd.md) §1, §2, §3.1 and §3.2, `CLAUDE.md` ("Which command
checks what", "The loop for every task", "Steps and commits") and `CONTRIBUTING.md`. Replaces the
owner's decision of 2026-08-24 to run the full pipeline on every push to a pull request, drafts
included.

## Context

Every task is worked on by a Claude Code session in a draft pull request, one pushed commit per
step (`CLAUDE.md`, "Steps and commits"). `ci.yml` triggers on `pull_request` with the default
event types, so every one of those pushes starts the whole pipeline — and leaving draft starts
nothing at all.

Measured over the ten days 2026-09-24…10-03: 220 runs of `ci.yml`, 38 pull requests, and the draft
state of each pull request at the moment each run was created, taken from its
`ReadyForReviewEvent`/`ConvertToDraftEvent` timeline.

| | Runs | Job-minutes |
|---|---|---|
| Pull request in draft | **140 (64 %)** | **4,828 (63 %)** |
| Pull request ready | 54 (25 %) | 1,770 (23 %) |
| Push to `main` | 26 (12 %) | 1,121 (15 %) |

* Of the 140 draft runs, 66 were cancelled by the next push and 42 failed. The job that failed
  most often was *Pull request description* — 59 times, always on a draft, whose description is
  unfinished by design. A red cross that means nothing teaches everyone to stop reading red.
* `codeql.yml` ran 98 more times on drafts.
* 15 of the 38 pull requests were opened ready (3 of them Dependabot's). None was ever converted
  back to draft; 14 received further pushes while ready (38 runs).
* *Ready for review* triggered no run: #1129 left draft at 19:25, and its next run was a push at
  20:07.
* At the peak, 38 jobs ran at once — drafts competing for runners with pull requests waiting to
  merge.

The minutes cost nothing; the repository is public. What the draft runs cost is wall clock for the
pull requests that are ready, and a signal nobody reads.

They also caught real defects, which is the half of the question a rule has to answer. On drafts,
*Integration* failed 17 times, *The client runs in three engines* 21, *Self-hosting stack* 10 and
*The hubctl session* 10 — none of which `make verify` runs. `ci-cd.md` §1 calls `make verify` "the
complete PR pipeline"; it is the fast half of it.

### What a probe showed about the required check

Skipping every job on a draft makes `ci-required` — `if: always()`, a skip counts as a pass —
report a green `CI required` on the draft's commit. A probe pull request (#1137, closed unmerged)
measured what that green does when the pull request leaves draft.

* **Probe A — every job skipped on a draft.** The draft run reported 24 skipped jobs and `CI
  required: success`. The merge state read immediately after *Ready for review* was `UNSTABLE`,
  which is mergeable; two seconds later, once the new run's check suite existed, it was `BLOCKED`,
  with the old green still listed as the required check. Reviews, conversations and an outdated
  branch were ruled out as the cause. GitHub supersedes a workflow's checks with its latest run,
  which closes most of the gap — not all of it, and by no rule GitHub documents.
* **Probe B — `ci-required` named by an expression.** On a draft the job reported as `CI not run
  (draft)`; the commit carried no check called `CI required`, and the pull request was `BLOCKED`
  from the first reading after *Ready for review* until the real result existed. On the ready run
  the same job reported as `CI required`.
* A cancelled run still produces `CI required`, as a failure: `always()` runs on cancellation.

### What a local run costs

Every gate CI runs for a Go change, run one after another in a worktree on 2026-10-04, against
CI's median job time over the same ten days:

| Gate | Local | CI job (median) |
|---|---|---|
| `make verify` (the fast half) | 287 s | several jobs in parallel |
| `gate-integration` + `gate-contract` | 111 s | 3.8 min |
| `gate-data` + `gate-privacy-full` | 33 s | 1.6 min |
| `gate-resilience` | 64 s | 2.4 min |
| `gate-selftest` | 185 s | 4.7 min |
| arm64 build | 8 s | 1.0 min |
| image build, `gate-compose`, `gate-e2e`, `gate-engine-conformance` | 38 + 70 + 93 + 19 s | 3.3 / 3.6 / 2.5 min |
| `pnpm` build, lint, typecheck, test; the three engines | 20 + 20 s | 0.5–0.8 / 2.4 min |

About **15 minutes** for a change that touches everything, against about 8 minutes of wall clock
for a CI run that runs the same in parallel — paid once, before *Ready*, instead of on every push.

The measurement found two things that make a local run unreliable today:

* **`gate-e2e` leaves its stack running.** `scripts/hubctl-e2e.sh` installs `cleanup` as its EXIT
  trap; the AI stub replaces that trap with its own (line 1246) and `trap - EXIT` (line 1384) then
  removes every trap. The stack `hubtask-e2e` keeps port 18081, and the temporary directory with
  the session's generated credentials stays behind. On a runner nobody notices; on a laptop the
  next `gate-compose` fails every time — `Bind for 0.0.0.0:18081 failed: port is already
  allocated` — because its multi-mode stack uses the same port. Introduced in #498.
* **The container gates are not built for neighbours.** Their compose project names and host ports
  are fixed, and every session on the machine shares one Docker daemon (34 worktrees on the day of
  the measurement). Two sessions running them at once collide, and one session's `down -v` removes
  the other's stack.

## Decision

**A draft is checked in the session that writes it. The pipeline runs when a pull request is
ready, once, and again only when what is ready changes.**

### On GitHub

1. **`ci.yml` triggers on `opened`, `synchronize`, `reopened` and `ready_for_review`.** The jobs
   that need no other job — `changes`, `secrets`, `dependencies`, `licences`, `docs`,
   `pr-description` — carry `github.event_name != 'pull_request' || !github.event.pull_request.draft`;
   every other job depends on one of them and is skipped with it (probe A: all 24).
2. **`ci-required` is called `CI required` only on a run that is not a draft's:**
   `name: ${{ (github.event_name == 'pull_request' && github.event.pull_request.draft) && 'CI not
   run (draft)' || 'CI required' }}`. A draft never satisfies the required check; the check that
   does can only come from a run that checked something. Branch protection is unchanged: it names
   `CI required` and nothing else (§3.2).
3. **`codeql.yml`** gets the same trigger types and the draft condition. It is not a required
   check, so it needs no name.
4. **`claude-review.yml`** posts its notice on `opened` and `ready_for_review` only — the text never
   changes, so every push rewrote the same comment. **`pr-description-rerun.yml`** skips drafts.
5. **Unchanged:** the push to `main` (the unfiltered full run, the caches pull requests restore
   from, the CodeQL baseline, the deploy), the nightly, the release, and Dependabot's pull
   requests — never drafts, checked by nobody locally, so CI is their only check. Secret scanning
   push protection is enabled and stops a key before any workflow would.

### In the session

6. **`make verify-pr` is the local equivalent of the pull request check.** (Named `make ready` when this record was
   approved; renamed before the first push, because the readiness record of #1136 already gives
   *ready* a meaning — settled before code — and one word for two gates is one too many.) It runs `make verify`, then
   the gates CI would run for this branch's diff against `origin/main`, classified by **the
   filters in `ci.yml` itself** — read through the parser `test/architecture` already uses, moved
   to where a tool can import it — so the local and the remote selection cannot drift:
   * Go, contract, database, deploy or workflow changes: `gate-integration`, `gate-contract`,
     `gate-data`, `gate-privacy-full`, `gate-resilience`, `gate-selftest`, the arm64 build.
   * A workspace package: `pnpm` build, lint, typecheck and test; the engines when the web
     application or anything it consumes changed; `make tokens` and `make api-client` without a
     diff when their inputs changed; `make website` for the website.
   * Whatever reaches the image: the image build, `gate-compose`, `gate-e2e`,
     `gate-engine-conformance` — **under a machine-wide lock** in the repository's common git
     directory, so sessions in different worktrees take turns. When Docker does not answer within
     seconds, they are reported as *not run locally — CI will*, not as a failure.
   * Always: `make gate-pr` against the pull request's description.

   It refuses a dirty tree, puts `.tools` on the `PATH` the way CI does, prints what it ran and
   what it could not, and on success writes a stamp naming the checked commit into the checkout's
   git directory. A test in `test/architecture` holds every job in `ci-required`'s `needs` to one
   of two lists: run by `make verify-pr` for the same filters, or *CI only* with the reason
   (`secrets`: gitleaks is not among the pinned tools, and push protection stops a key at the
   push; `dependencies`: the review needs the pull request; the description, which `make verify-pr`
   checks with `make gate-pr` itself). CodeQL is a workflow of its own and runs after *Ready*.
7. **A Claude Code hook enforces the two transitions.** `.claude/settings.json`, committed, runs a
   `PreToolUse` hook on `Bash` that refuses
   * `gh pr ready` (not `--undo`) unless the stamp names `HEAD` and `HEAD` is pushed — a pull
     request marked ready with unpushed commits starts CI on the wrong commit;
   * `gh pr create` without `--draft`.

   Exit code 2 blocks the call and hands the hook's message to the session, which names the command
   to run instead. The hook binds the sessions, which is where the rules were broken; a person can
   still use the web interface, and Dependabot is untouched.
8. **`scripts/hubctl-e2e.sh` keeps its cleanup.** The AI stub's trap calls `cleanup` as well, and
   where the stub is removed the trap is set back to `cleanup` instead of cleared.
9. **The rules** (`CLAUDE.md`, `CONTRIBUTING.md`, `ci-cd.md`):
   * Every pull request a session opens starts as a draft, documentation included.
   * Every step is still pushed at once — the reason the work is split into commits is unchanged —
     and a push to a draft starts no CI.
   * `make gate-quick` at every commit, `make verify-pr` before `gh pr ready`.
   * **Rework on a ready pull request goes back to draft first:** `gh pr ready --undo`, the
     commits, `make verify-pr`, `gh pr ready`. That applies to findings of the owner's review, of a
     use case check and of a failed CI run alike.
   * **Except bringing the branch up to date with `main`**, which branch protection requires and
     which changes nothing that was reviewed. The pull request stays ready; that run is the one
     that has to happen.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: drafts skip, the summary job is renamed on a draft, `make verify-pr`, a hook** | Removes 64 % of the runs and the red drafts. The required check can only be met by a run that checked something — proven, not assumed (probe B). |
| Skip on drafts, name unchanged | What most repositories do. Leaves a green `CI required` on every draft's commit and relies on GitHub superseding it in time; probe A read the pull request as mergeable right after the transition. The rename costs one line. |
| Keep CI on every push (2026-08-24) | The numbers above. With parallel sessions the cost became a queue, not a number of minutes. |
| A cheap lane on drafts (documentation, secrets, licences) | A green-looking status on unchecked work; secrets are already stopped by push protection. |
| CI on a draft by label | A second way in for the same run — the transition to ready already is that label. |
| Fail `CI required` on a draft | Equally safe, and every draft carries a red cross again: the noise this decision exists to remove. |
| Drop "branches must be up to date" | Fewer runs, and parallel sessions in one bounded context (SC-16…SC-29 all touched identity) would merge combinations nobody tested. |

## Consequences

**Positive**

* On the measured ten days: 140 of 220 `ci.yml` runs and about 4,800 of 7,700 job-minutes fewer,
  98 fewer CodeQL runs, no red drafts. A check appears when a pull request is ready, and red then
  means red.
* `make verify-pr` makes `ci-cd.md` §1 true again — every gate reproducible locally — and says per gate
  when it is not.
* The rules are enforced where they were broken, in the session, rather than written down again.

**Negative / countermeasures**

* *A session waits up to about 15 minutes before it may leave draft.* → Once per transition, not
  per push. The groups that share nothing may run in parallel; the container group may not.
* *A defect only a container or a browser finds surfaces after Ready when Docker is unavailable
  locally.* → `make verify-pr` says so; CI finds it; the pull request goes back to draft — one round
  more instead of a run on every push.
* *The hook binds Claude Code sessions only.* → They open the pull requests. A person marking ready
  in the browser is a deliberate act.
* *GitHub could change how a job name expression is evaluated.* → `test/architecture` asserts the
  expression and the draft condition on every root job, and `gate-selftest` proves that test
  fails when either is removed.

## Proof

* **The hook, live (2026-10-06).** A freshly started Claude Code session in a repository holding
  the hook and its script was asked to run `gh pr ready --help`: without a stamp it was refused
  with the hook's message and the command never ran; with a stamp naming `HEAD` it ran.
* The implementing pull request is its own probe: as a draft its commit shows `CI not run (draft)`
  and no `CI required`; after *Ready for review* it is `BLOCKED` until its run ends, which then
  shows `CI required`.
* The hook script refuses and allows the cases it is written for, in a test that runs with the
  architecture gate.
* `make gate-e2e` followed by `make gate-compose` is green locally, with no stack left running.
