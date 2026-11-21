# CI/CD with GitHub Actions

The platform decision is [ADR-0022](../adr/ADR-0022-github-platform.md); when CI runs on a pull
request is [ADR-0079](../adr/ADR-0079-a-draft-is-checked-locally.md), and what a pull request runs
is [ADR-0089](../adr/ADR-0089-check-once-where-it-is-cheap.md). The release gates the
pipeline serves are [versioning-release.md](./versioning-release.md) §6.

---

## 1. The principle: the pipeline calls nothing but `make`

Every step of a workflow is a `make` target; the workflow holds orchestration, not logic. So every
gate is reproducible locally — `make check` runs the checks of what a branch changed, `make verify`
every gate without containers, and each gate has its own target (§3.4) — and switching platform
touches only
`.github/workflows/`. The product (application, Helm chart, Compose files) depends on nothing
GitHub-specific; the pipeline may.

---

## 2. Workflows

| File | Trigger | Purpose |
|---|---|---|
| `ci.yml` | A pull request once it is ready (opened ready, leaving draft, or pushed to while ready); a push to `main`; manual dispatch on a pushed branch (§3.4) | The pull request gates (§3) |
| `codeql.yml` | A ready pull request, a push to `main`, weekly | Static security analysis. Not part of `CI required`; a red CodeQL is a finding to fix, not to suppress |
| `after-ci.yml` | A `ci.yml` run ends; every 15 minutes; manual dispatch | A failed run is re-run once; a red `main` opens the issue "CI: main is red" and holds the green pull requests; a green `main` closes it and re-runs the pull requests that waited (§3.5) |
| `pr-description-rerun.yml` | A pull request's description or title is edited | If the head commit's latest `ci.yml` run failed, re-runs its description job and `CI required` (§3.1) |
| `nightly.yml` | Schedule (overnight) | Long runs (§9): `deep` (fuzzing, load, resilience), the `matrix-*` cells, the PITR drill (`make gate-pitr`), the image scan, the action pins; the arm64 cell also runs `make gate-privacy-full` and `make gate-selftest`. A failure files an issue |
| `release.yml` | Tag `v*` | The installed apps built without a secret (`apps`: Windows, Linux, Android); after the `production` environment's approval: the gates again, image, SBOM, signature, provenance, the CSAF advisories and the release record, each signed, chart, the apps signed with the project's keys, one checksum file, GitHub release; then the n8n node to npm and the Zapier app, each only when the owner's account secret (`NPM_TOKEN`, `ZAPIER_DEPLOY_KEY`) is set (UC-INT-11) ([deployment.md](./deployment.md) §7, [security.md](./security.md) §14.2, [versioning-release.md](./versioning-release.md) §3, §9) |
| `deploy.yml` | Push to `main`, manual dispatch | Builds, signs and verifies the per-commit image and runs `helm upgrade` into `integration` ([deployment.md](./deployment.md) §3) |
| `website.yml` | Push to `main` touching `apps/website/`, `packages/design-system/`, `security/advisories/` or the lockfile; manual dispatch | Writes the CSAF advisories into the site (`make csaf`), builds `apps/website/dist` and mirrors it to the webspace (§10). A failure files an issue |
| `workbench.yml` | Push to `main` touching `packages/design-system/`; manual dispatch | Publishes the component workbench to `workbench.hubtask.eu` (§10) |
| `scorecard.yml` | Schedule | **Planned, not built.** The OpenSSF supply chain scorecard. The file does not exist; the row stays as the plan |

---

## 3. Jobs in `ci.yml`

Every job of `ci.yml`, in file order. `ci-required` waits for all of the others (§3.2).

| Job | Contents |
|---|---|
| `changes` | Classifies what a pull request touches (`dorny/paths-filter`, §3.1) |
| `actions-allowed` | Every `uses:` of every workflow as a step that never runs, so GitHub's job set-up checks each against the repository setting (§4); behind the `ci` filter, always on `main`. CI only: nothing local can ask the setting |
| `secrets` | Gitleaks over the history (SG-7) |
| `dependencies` | Dependency review: no high-severity vulnerability, no GPL, LGPL, AGPL or SSPL licence ([licensing-editions.md](./licensing-editions.md) §1) |
| `licences` | `make gate-licenses`: no copyleft Go dependency, `THIRD-PARTY-LICENSES.md` current, every brand mark with its row and its preset |
| `quick` | `make gate-quick` (gofmt, `golangci-lint` with `gosec` and `depguard`, `go vet`, `make generate` without a diff), `make gate-sdk` |
| `build` | `make build` for linux/amd64 and linux/arm64 |
| `unit` | `make gate-unit`: Go tests with `-race`, the gates' own tests under `tools/` among them, coverage `core/domain` ≥ 85 %, `core/application` ≥ 75 % |
| `architecture` | `make gate-architecture`: layer rules, the `go` ban outside `SafeGo`, mandatory authorisation, use case parity across REST, MCP and automation, RT-12, SG-13, the translation gate, `actionlint` |
| `resilience` | `make gate-resilience`: RT-1…RT-5, RT-7, RT-10, RT-12 (§3.2) |
| `observability` | `make gate-observability`: `promtool check rules` and `promtool test rules` over the four rule files, and `test/observability` |
| `chart` | `make gate-chart`: `helm lint` and `helm template` with every optional object on; the chart's copy of rules and dashboards current |
| `api-compat` | `make gate-api-compat`: `api/openapi.yaml` against the newest `v*` tag through the pinned `oasdiff` (below) |
| `compose` | `make gate-compose`: builds the image from this commit and starts the self-hosting stack until `/readyz` answers |
| `e2e` | `make gate-e2e`: the `hubctl` session against the Compose stack (`scripts/hubctl-e2e.sh`), including `hubctl sync-conformance` |
| `upgrade` | `make gate-upgrade`: the newest release's image, compose file and `hubctl` write; this build's migrator runs while that release serves; the old client reads and writes on the new schema, this build's reads it, and the release goes back on the new schema and writes again (`scripts/upgrade-session.sh`) |
| `engine-session` | `make gate-engine-conformance`: the first-party sync engine against its own Compose stack, answering [offline-sync.md](./offline-sync.md) §9's eight client requirements by number |
| `selftest` | `make gate-selftest`: one deliberate violation per configured rule, each expected to turn the build red |
| `integration` | PostgreSQL 16 (`pgvector/pgvector:pg16`), `make migrate`, `make gate-integration` (object storage from `test/s3test`, one pinned SeaweedFS — [ADR-0067](../adr/ADR-0067-s3-test-fixture.md)), then **`make gate-contract`**: responses, events, closed sets, the router and the SDK examples against the contract |
| `security` | `make gate-security` (`govulncheck`, `test/security`: cross-tenant, RLS, SSRF, uploads, authorisation, redaction) and `make gate-privacy` (PG-1, PG-3…PG-6, PG-8) |
| `data` | `make gate-data` (RE-1…RE-9, BK-1 for every target, SY-1…SY-12, audit) and `make gate-privacy-full` (PG-2, PG-7) |
| `docs` | `make gate-docs` (§3.1) |
| `numbers` | `make gate-numbers`: one file per migration and ADR number in the tree; on a pull request and on a manual run, no number the branch adds that another open pull request adds (§3.5) |
| `pr-description` | `make gate-pr` over the description and the title read from the API; not for Dependabot's pull requests (§3.1) |
| `node` | Per affected workspace package: the workspace map lint, build, lint, typecheck, test; for the website `make website` first |
| `engines` | The built web app in Chromium through pinned Playwright ([ADR-0048](../adr/ADR-0048-browser-job-driver.md)), asserting [ADR-0044](../adr/ADR-0044-browser-support-row.md)'s feature table, with Firefox and WebKit best effort by hand (`ENGINES=firefox,webkit`); the suites in three shards by file (`node --test --test-shard`), each on its own runner; the one job that runs a browser. The walks write down every answer they give for `/api/v1`, and `make gate-walks` then holds this shard's answers to `api/openapi.yaml`: an answer the contract does not allow fails unless `test/contract/testdata/walks-baseline.txt` lists it, and `make gate-pr` refuses a line added to that list. The rules for a walk, and the two counts the `node` job holds (fixed waits, suites that cite no use case check), are in [`apps/webapp/e2e/AGENTS.md`](../../apps/webapp/e2e/AGENTS.md) |
| `shell` | The installed apps' shell (`apps/webapp/src-tauri/`) on Linux, Windows and Android: the icons generated from the mark and checked, a debug build bundled as `.deb`, NSIS and an x86_64 APK, each with the `walk` feature; on Linux also `cargo-deny` (licences, sources), `rustfmt`, `clippy` and the Rust tests. Then each leg's smoke walk (UC-SYN-07 check 9, `apps/webapp/e2e/webview/smoke.mjs`): sign in, open a hub, create, edit and complete an entry, sign out, against a server built from the commit on the runner's PostgreSQL, through the engine's own WebDriver - WebKitWebDriver, the runner's msedgedriver, its chromedriver with an API 35 emulator. A failure in one engine fails the lane. Behind the `shell` filter: the web app, the packages it is built from, the workspace manifests |
| `tokens-drift` | `make tokens`; the committed `LabelTokens.go` must not change |
| `api-client-drift` | `make api-client`, then the workspace built and typechecked against it; nothing generated may be committed |
| `ci-required` | The one required check (§3.2); on a pull request its last step waits for a green `main` (§3.5) |

`quick` is the prerequisite of every Go job but `selftest`; a skipped `quick` skips them all. The Go
jobs after it run in parallel. `selftest` starts beside `quick`, on the same filters: it is the
longest Go job, and behind `quick` it set the pipeline's wall clock ([ADR-0083](../adr/ADR-0083-a-merge-without-catching-up.md)).

**Contract gate.** In CI, `make gate-contract` runs in one place, the last step of
`integration`; there is no separate `contract` job. It needs no database and runs in seconds, so
`make verify` runs it as well.

**Compatibility gate.** `api-compat` compares `api/openapi.yaml` with the newest `v*` tag behind the
commit through `oasdiff`, a pinned development tool that never ships, and classifies by
[versioning-release.md](./versioning-release.md) §2: a new value in a response enum is lowered to
compatible, the rest is `oasdiff`'s own judgement. Only the breaking changes the pull request adds
to those already on its base are judged — on a push to `main`, those of the pushed commit; on a
manual run, those since the branch left `main` — and
each one fails unless the change is marked: `!` in a commit's or the pull request's title, or a
`BREAKING CHANGE:` footer in a commit message or the description. A break already on the base was
judged when it landed. The job checks out the whole history; with no tag behind the commit it
skips with a notice. `make gate-api-compat API_BASE=<merge base>` judges only the branch's own
commits; without `API_BASE` it judges every break since the tag
against every commit since it.

**The hubctl session spends the rate limit.** `scripts/hubctl-e2e.sh` runs as one client against
the anonymous and per-token budgets, with `curl --retry` on every call, so a section appended late
spends a limit the earlier sections drew on; a `429` there is the session's own budget. The sync
engine's conformance run has its own stack for the same reason.

### 3.2 Where the resilience tests run

**A test that runs in minutes on a shared runner is a pull request gate; one that needs an hour of
load is nightly.**

* RT-1…RT-5, RT-7, RT-10 and RT-12 run in `resilience` on every pull request, beside longer jobs, so
  they cost no wall clock.
* RT-6 (overload), RT-8 (rolling update under load) and RT-11 (memory over an hour) run nightly
  through `make gate-load`. RT-6 asserts only what survives a noisy runner: shedding engaged, no
  interactive request refused, the interactive P95 inside its target, recovery. Percent-level
  questions belong to the release tier of the [load suite](../../test/load/README.md)
  ([observability-reliability.md](./observability-reliability.md) §13.2).

### 3.1 Where a job runs, and where it does not

Every job decides from the `changes` outputs whether it has work.

| Changed (filter) | What runs |
|---|---|
| `go`, `openapi`, `db`, `deploy` | The whole Go pipeline, all security gates included |
| `openapi` | Additionally: the API client is regenerated and the workspace typechecks against it; every package generated from the contract runs |
| `design_system` | Additionally: the tokens are regenerated and the committed `LabelTokens.go` must not move |
| `webapp`, `website`, `design_system`, `api_client`, `sync_engine`, `n8n_node`, `zapier_app`, `sdk_typescript` | The `node` job for the affected packages and their consumers |
| `webapp`, `design_system`, `api_client`, `go`, `openapi`, `db`, `deploy` | The container build (`compose`), because the image contains both halves ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) |
| `webapp`, `design_system`, `sync_engine`, `expression`, `editor`, `whiteboard`, `api_client`, `openapi`, `walk_judge` | The browser job (`engines`). `walk_judge` names what judges the walks' answers — `test/contract/walks*.go`, `test/contract/schema*.go` and the baseline — file by file, so that no other Go change walks every suite |
| `ci` (`.github/**`, `.gitleaks.toml`) | Everything |
| documentation only | The five jobs behind no filter — `secrets`, `dependencies`, `licences`, `docs`, `numbers` — and the description check |
| anything, on a draft | Nothing. Every job is skipped and `ci-required` reports as `CI not run (draft)` (§3.4) |

On a push to `main` every filter output is `true`: the branch that gets released is never filtered.
A manual run (§3.4) is no pull request either, so no filter applies to it. `dorny/paths-filter` runs
on a pull request only; on any other event it would compare with the default branch through git.

**Five jobs are behind no filter.** A key or a copyleft dependency gets in through any path, so
`secrets`, `dependencies` and `licences` never filter; a number is taken in `db/` as in `docs/adr/`,
so `numbers` does not either. `docs` takes seconds and reads files every
filter could skip: `checkdocs` reconciles the Go version across `go.mod`, the workflows, the
Dockerfile, the support matrix and the README; reconciles the support matrix with the nightly's
jobs; resolves ADR citations in every kind of source file and document; checks links and anchors;
holds the list of skills in the root `AGENTS.md` to the folders under `.agents/skills/`;
and holds the newest `docs/evidence/COVERAGE-<date>.md` to `catalogue.Descriptors()` — one row per
served use case, none for an unserved one, and no "nobody built it" without an issue number.

**`pr-description` reads the description, not the tree.** On every pull request that is not Dependabot's,
`tools/checkpr` holds the description to `.github/PULL_REQUEST_TEMPLATE.md`: every section present,
in order, filled or marked n/a; `Closes #n` at the start of a line or `No issue: <why>`; use cases
that exist; *Premises* filled — what was found false and corrected, or what was checked; *Review*
with a reviewer and a date, the findings found, fixed and open as numbers, and the outside
reviewer's counts or `not run: <why>`; one ADR
answer ticked and named; no placeholder left; and the title a Conventional Commit. With the
branch's history it also holds: no merged migration changed; no use case deleted and no ID reused; a
change to a use case's Goal, How to check or Where it ends named `correction` or `decision #<n>`;
no settled ADR changed beyond its status line, its `Rule lives in` line and link targets; and
no numbered section of a subject document (`docs/architecture/`, `docs/design/`) dropped or
renumbered, nor left as a bare heading where it had text — a retired section keeps its heading
with one sentence saying where its content went; and a test the branch removes, a test file it
deletes and a test it adds a skip to are each named in the description — a Go `Test…` or `Fuzz…`
function, a `test(` or `it(` of a `*.test.ts`, `*.test.mjs` or `*.test.js`. A name that comes back
anywhere in the diff is a moved test; a declaration replaced within one run of changed lines is a
rename. It also refuses a line added to the baseline of the walks' answers outside the contract
(`test/contract/testdata/walks-baseline.txt`). It does not see a weakened assertion, a test deleted with another written in its place, a
test removed in one package while one of the same name is added in another, or whether the
*Review* and the reasons are true. The
history rules hold pull requests opened from 2026-10-08 on, *Review*, the removed tests and the
walks' baseline those opened from 2026-11-21 on. It
reads the description and the title from the API, so a re-run judges them as they stand.
`make gate-pr BODY=<file> TITLE="<title>"` runs it locally, `BASE=origin/main` with the history;
`DRAFT=1` lets a draft's *Review* say `not yet: <why>`, which CI — reading only a ready pull
request — refuses. `gh pr create --body` never shows the template;
start from a copy of it.

**The filters name trees, and `test/architecture` checks that they name all of them.** Every
tracked file is claimed by a filter or listed among the paths that deliberately trigger nothing,
and every pattern matches something that exists. A filter by file pattern (`**/*.go`) would leave
the fixtures a Go test reads outside it, and `ci-required` counts that skip as a pass. A file
`test/architecture` reads sets the `go` filter, which runs that package: the hook's settings, the
skills under `.agents/skills/`, their links under `.claude/skills/` and `.claude/agents/`. The
root `AGENTS.md` sets no filter: the documentation gate, which runs on every change, holds its
list of skills to the folders. The
outside reviewer's `.coderabbit.yaml` triggers nothing, since no job reads it.

**Performance is measured, not gated.** A test whose verdict is a time or a frame rate runs in no
job of the pull request's path and in no local gate (`make check`, `make e2e`): an end-to-end walk
proves that a thing works, and a shared runner or a laptop beside other work measures itself, not
the product. Such a test runs from time to time - today on demand
(`pnpm --filter @hubtask/webapp test:performance`, the board of 5 000 objects), in the long run on
the integration server. The owner's decision of 2026-11-16.

### 3.2 `ci-required` is the only required status check

(Both sections numbered 3.2 are cited, so neither is renumbered.)

A required check that never reports is **pending**, and blocks the merge for ever. So no pull
request workflow filters with a workflow-level `paths:` or `paths-ignore:`; filtering happens in
job `if:` conditions, and a job skipped by an `if:` does report.

```yaml
ci-required:
  name: CI required          # 'CI not run (draft)' on a draft, 'CI by hand' on a manual run
  if: always()
  needs: [ …every other job… ]
```

It fails if any dependency ended `failure` or `cancelled`, and passes when all ended `success` or
`skipped`. **Branch protection names `CI required` and nothing else**; a new job is added to
`ci-required`'s `needs`, never to a required-check list.

**On a draft it has another name.** Every job is skipped on a draft (§3.4), so a green
`CI required` there would satisfy branch protection without a gate having run. Its name is an
expression: `CI not run (draft)` on a draft, `CI by hand` on a manual run (§3.4), `CI required`
otherwise. A manual run checks the branch's head — the commit a draft's pull request shows —
without what reads a pull request and without the guard for a green `main` (§3.4 lists both), so
its green must not be that commit's `CI required` either. `test/architecture` holds the
expression, and `make gate-selftest` proves that test goes red without the draft's name and
without the manual run's.

A gate whose subject does not exist yet reports that it is skipping and stays green; it never
swallows a real failure, and the `selftest` job checks that the difference holds.

### 3.3 What a job no longer compiles again

The pinned tools are cached as binaries ([ADR-0062](../adr/ADR-0062-cached-tool-binaries.md)).
Jobs call `make tools-ensure`, which installs the whole set unless `.tools/.installed` names exactly
today's pins and the Go that built them and every binary is present; a restored cache that does not
match is thrown away. `release.yml` compiles its own set from the tag's pins, with no cache.

The project's Go build and module caches are not shared across jobs beyond `setup-go`'s cache.
Whether to share them is open, against the repository's 10 GB cache limit.

### 3.4 A draft runs nothing; the branch is checked once, where it is cheap

The rules of [ADR-0079](../adr/ADR-0079-a-draft-is-checked-locally.md) and
[ADR-0089](../adr/ADR-0089-check-once-where-it-is-cheap.md):

* **`ci.yml` and `codeql.yml` also trigger on `ready_for_review`.** Every job that needs no other
  job carries the draft condition, and so does `ci-required`; the others depend on one of them and
  are skipped with it. A draft's run starts no runner, and After CI ignores it. An edited draft's
  description re-runs nothing.
* **`make check` is the check while working and before leaving draft.** It runs what the branch
  changed against `origin/main`: format, vet, lint and tests of the changed Go packages and the
  architecture tests, `make generate` without a diff when the contract or a query changed, the
  changed workspace packages, the documentation gate when a document changed. No lock, about a
  minute. Everything else — containers, integration, security, the other packages — is CI's.
* **CI checks a pull request once it is ready**, in parallel jobs, about ten minutes; a red job is
  fixed by a push to the ready pull request. The hook in `.claude/settings.json` refuses
  `gh pr create` without `--draft`, and `gh pr ready` while `HEAD` is not pushed.
* **A manual run checks a pushed branch while its pull request stays a draft.** `ci.yml` triggers
  on `workflow_dispatch`: `gh workflow run ci.yml --ref <branch>` starts one run on the branch's
  head, with the jobs a push to `main` runs. It is for a concept's checkpoint (`build-concept`) and
  gates nothing: its summary job is named `CI by hand`, never `CI required` (§3.2), and leaving
  draft still starts the pull request's own run. GitHub starts it when `ci.yml` exists on the
  default branch, and runs the branch's version of the file — the trigger need not be on `main`.
  The first manual run was started on 2026-11-21, from the branch that added the trigger.

  It differs from a ready pull request's run in these ways, and in no other:
  - every job checks the branch's head, where a pull request's run checks the merge with `main`;
  - `changes` filters nothing, so the jobs a pull request skips for what it did not touch run too;
  - `dependencies` and `pr-description` do not run: they read a pull request;
  - `api-compat` judges the breaks added since the branch left `main`, where a pull request's run
    judges those added to its base, and reads no title and no description, so a break is marked
    in a commit;
  - `numbers` compares what the branch adds over `main` with the open pull requests, its own
    among them — the same file in both is no collision — and holds the branch's tree, not the
    merge, so a number `main` took since the branch left it shows only at *Ready*;
  - `CI required`'s guard for a green `main` does not run.

  After CI starts when a manual run ends and acts on nothing: it neither re-runs a failed one nor
  files it as a red `main`; the guard reads pushes to `main` only. No job of `ci.yml`
  publishes, deploys or pushes an image; a run on a branch writes caches in that branch's scope,
  which `main` never restores, though they count towards the repository's 10 GB. A push to a
  draft's branch still starts nothing. `test/architecture` holds the trigger, the push to `main`
  alone, that no job reads the event beyond "is this a pull request" or reads the branch, and the
  bases of `api-compat` and `numbers`; `make gate-selftest` takes the trigger away and plants a
  job that reads the branch.
* **A pull request runs what it can break.** `selftest` proves the gates and runs when one changes
  (the `gates` filter); the connectors (`n8n-nodes-hubtask`, `zapier-app`) run when they change;
  the browser job runs Chromium, with Firefox and WebKit best effort (support-matrix.md §5). `main`
  runs everything on every commit.

**Once per pull request.** `make check` passes; the independent review runs, and its findings are
fixed or answered (engineering-guidelines.md §3.19); `make check` passes on the last fix; the
commit is pushed; `gh pr ready`; and the pull request merges when `CI required` is green — on the owner's word, by hand or by auto-merge (§3.5).
It is not brought up to date with `main` to merge. Only a conflict GitHub reports, or a base that
was squash-merged, needs `main` merged in; that stays on the ready pull request (AGENTS.md), runs
`make generate` and `make check` for what the merge touched, and leaves the rest to its CI run.

### 3.5 Merging without catching up

The rule of [ADR-0083](../adr/ADR-0083-a-merge-without-catching-up.md). Branch protection does not
require a branch to be up to date (§4). A pull request's run checks the merge of its head with
`main` as `main` stood when the run's event made the merge commit — a re-run repeats that commit —
so what merged meanwhile is first checked by `main`'s own run. Four things keep that safe:

* **Every commit on `main` runs to the end.** `ci.yml`'s concurrency group is the commit on a push
  to `main` and the ref on a pull request and on a manual run; a pull request's or a manual run's
  older run on the same ref is cancelled, a push's never. The first red commit is therefore always
  known.
* **`CI required` waits for a green `main`.** Its last step, on a pull request only
  (`scripts/ci/main-green.sh`, locally `make gate-main-green`), reads `ci.yml`'s runs of pushes to
  `main`, newest commit first: a finished `success` is green; `failure`, `timed_out` or
  `startup_failure` is red; a re-run still going is red, a first attempt still going and a cancelled
  run are passed over. While `main` is red it fails, unless the pull request is a revert (`revert:
  …`, `Revert "…"`) or its description has a line `Mends main: <sha>` — editing the description
  re-runs it (`pr-description-rerun.yml`). A failing API call fails it. A pure revert — `git revert`
  of exactly the commit the issue "CI: main is red" names, nothing else changed — merges as soon as
  its CI is green, without asking (the owner, 2026-10-13, #1311); the reverted change returns to its
  task as a new pull request. A fix forward (`Mends main:`) merges on the owner's word.
* **`after-ci.yml` acts after every run** (`scripts/ci/after-ci.sh`): a failed run is re-run once,
  known-traps.md's rule for flakes — not a pull request's run that failed only at the guard; a red
  `main` opens the issue "CI: main is red" (`finding`) with the commit, its pull request, the failed
  jobs and the revert, and re-runs `CI required` alone on every green ready pull request so that the
  guard holds it; a green `main` closes the issue and re-runs the failed jobs of every pull request
  that failed only at the guard. GitHub starts no workflow for what its own token caused, a re-run's
  end included, so the same workflow sweeps every 15 minutes and on dispatch. A write GitHub refuses
  (a Dependabot run's read-only token, a run already re-running) is a warning.
* **No number twice.** The `numbers` job fails a pull request that adds a migration or ADR number
  another open pull request adds, drafts included, under another file name, and any tree with two
  files of one number. Each run looks after its own push, so of two such pull requests the second
  to look is always told; the first may already be green. The one told renumbers — a migration only
  if it was never applied anywhere.

Generated and shared files keep no counts (the event matrix, the coverage report): two clean merges
would add up wrong. Integration deploys every push to `main` while `main`'s run is still going, so it
may run an untested combination for those minutes ([deployment.md](./deployment.md) §3).

---

## 4. Hardening

```yaml
permissions:
  contents: read          # the default for every workflow, widened only where necessary
```

* Actions are referenced by **commit SHA** with the tag as a comment; all actions of one
  repository share one commit (`test/architecture`). `make gate-action-pins` (nightly) asks GitHub
  whether each pin exists and the commented tag points at it.
* Every job of every workflow carries `timeout-minutes` (`test/architecture`); without it a hung
  step runs for GitHub's six hours and holds `CI required`. The limit is about three times the
  slowest successful run of the job, rounded up to five minutes and no less than fifteen;
  `deploy.yml`'s lies above its inner timeouts together, so it never cuts `helm upgrade` short. A
  step that waits on something outside the runner (apt in `engines`) gets a shorter limit of its
  own, so the log names it. A job that hits its limit is re-run once, like a flake.
* Only listed actions run. The repository setting "Allow select actions" allows actions owned by
  GitHub (`actions/*`, `github/*`) and the third-party repositories in
  `.github/actions-allowlist.txt`, each as `owner/repo@*` — the version is fixed by the SHA in the
  workflow — with verified creators off and **SHA pinning required**, so a tag- or branch-pinned
  `uses:` is refused. `test/architecture` holds the file to the workflows: every third-party
  repository used is listed, every entry is used, and an action that only a listed composite
  action runs inside itself is marked `# nested in <owner/repo>`. GitHub resolves nested actions at
  job set-up too (`aquasecurity/trivy-action` runs `aquasecurity/setup-trivy`). The `ci.yml` job
  `actions-allowed` (§3) names every `uses:` of every workflow as a step that never runs, so an
  action the setting lacks — a new nested one a Dependabot bump brings — fails the pull request
  that brings it, not the release or the nightly that would use it first; `test/architecture`
  holds its steps equal to the other workflows' `uses:`, pins included. Neither test reads the
  setting itself (that needs the administration right); a nested entry that a bump made unneeded
  is not found and stays until somebody removes it.
  A new third-party action is a new dependency and the owner's word: add the `uses:`, the entry in
  the file, and the same `owner/repo@*` in the setting
  (`gh api -X PUT repos/<owner>/<repo>/actions/permissions/selected-actions`, the whole list);
  `actions-allowed` stays red until the setting has it.
* `after-ci.yml` runs on `workflow_run`, which has write rights for a fork's run too: it checks out
  the default branch only and takes the run's values through the environment, never into `run:`
  (`test/architecture`).
* Secret scanning with push protection, and Dependabot alerts.
* No `pull_request_target`; contributions from forks run without secrets.
* Publishing only through the GitHub **environment** `production`, which has an approval rule.
* `cosign` keyless through the workflow's OIDC token — no private key in the repository.
  `id-token: write` only in the jobs that sign (`release.yml`, `deploy.yml`) and the npm provenance of `release.yml`'s `integrations` job.
* Branch protection on `main`: **`CI required` as the only required status check** (§3.2), and
  a branch need not be up to date with `main` (§3.5); conversations resolved before a merge; no
  force push, no deletion. No approving review is required (the owner's word to merge is the
  review), merges are squash merges only (a repository setting, so the history is linear without
  the rule), and administrators are not held to the protection.

---

## 5. AI in the pipeline

**AI comments, gates decide** ([ADR-0022](../adr/ADR-0022-github-platform.md)): AI assistance may
comment, suggest and draft; it never sets a required status, merges, publishes, or replaces a
gate. A check that an outside reviewer's app posts on a pull request is never a required check and
decides nothing: `ci-required` stays the only required status check (§3.2).

No workflow calls a model today, and none reviews or works on its own (§5.1). The limits and the
rules below are the outer limit for an AI workflow should one be added; adding one is the owner's
decision, recorded in an ADR.

What it may do, and the line it must not cross: review assistance comments with locations; test
suggestions are never committed; an ADR draft is offered as a comment, never created; release notes
are drafted, the version and tag never chosen; translation drafts never change `en.json` and need a
human ([i18n-l10n.md](./i18n-l10n.md)); triage categorises and asks, never closes an issue.

Any AI workflow runs only on explicit request (a label or a comment), receives the diff and no
secrets or production data, marks its contributions with model and date, has `contents: read` and
`pull-requests: write` only, is cost-capped per run and month (the cap aborts the run, not the
gate), and treats an unavailable provider as no failure (`continue-on-error: true`).

### 5.1 No workflow reviews or works on its own

No workflow reviews a pull request or starts work by itself, and none offers a trigger that would.
The author reviews a change against the rules no gate checks, a fresh agent the author starts
reviews it before it leaves draft, and an outside
reviewer may comment on the pull request (engineering-guidelines.md §3.19); both comment and decide
nothing (§5), and neither is a workflow. A person or an AI agent of any make works on a task. A tool that steers coding agents later works
through the same issues, labels and pull requests ([backlog README](../backlog/README.md)
§ "For tools that steer the work"), under its own GitHub App identity, without the right to merge.

### 5.2 An image scan describes the published image

The nightly vulnerability scan reads the newest published image (`:latest`, else `:rc`), not
`main`, so an alert can be already fixed on `main`. Check `main` first; if it carries the fix,
dismiss the alert as **won't fix** (never *false positive* — it is true of that image), linking the
fixing pull request. Only the govulncheck half of the job is actionable from a branch.

---

## 6. Repository secrets and variables

| Name | Kind | Used by | Purpose |
|---|---|---|---|
| `GITHUB_TOKEN` | Provided | Every workflow | GHCR pushes, the release, issues |
| `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD` | Secret, environment `production` | `release.yml` | The APK signing key, a PKCS #12 store with the alias `hubtask`, and its password ([versioning-release.md](./versioning-release.md) §9) |
| `TAURI_SIGNING_PRIVATE_KEY`, `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` | Secret, environment `production` | `release.yml` | The update key that signs the apps and `latest.json`, and its password ([versioning-release.md](./versioning-release.md) §9) |
| `KUBE_CONFIG` | Secret, environment `integration` | `deploy.yml` | The deploy identity for the integration cluster, limited to its namespace |
| `WEBSITE_SFTP_USER`, `WEBSITE_SFTP_PASSWORD` | Secret | `website.yml` | The website's SFTP account |
| `WORKBENCH_SFTP_USER`, `WORKBENCH_SFTP_PASSWORD` | Secret | `workbench.yml` | The workbench's own, scoped SFTP account |
| `WEBSITE_SFTP_HOST`, `WEBSITE_SFTP_HOST_KEY` | Variable | `website.yml`, `workbench.yml` | The webspace and its pinned host key |
| `WEBSITE_REMOTE_DIR`, `WORKBENCH_REMOTE_DIR` | Variable | `website.yml`, `workbench.yml` | Target directories |

**There is no credential for production in this repository.** Production pulls a published chart
and image through Argo CD ([deployment.md](./deployment.md) §4); no workflow can reach it.

---

## 7. Runner selection

Every job runs on GitHub's free hosted runners. `arm64` images are built under QEMU; the arm64
matrix cells run on native `arm64` runners. No runner is bought for load: the nightly is a relative
regression guard, and the capacity ramp runs per release on the integration server
([observability-reliability.md](./observability-reliability.md) §13.2).

A timed check of a use case that a pull request proves on a shared runner — the frames of a board
of 5,000 objects (UC-CNV-01 check 10) — keeps the use case's bounds and measures each gesture up to
three times, judged by its best: a slow build fails all three, one stall of the runner only one.
A bound that falls on a frame boundary allows half a millisecond for the timestamps' jitter, since
six frames at 60 Hz read either side of 100 ms (decision #1805).

---

## 8. Open points

| # | Point | Needed by |
|---|---|---|
| CI-2 | Whether to enable the merge queue, which GitHub offers only to an organisation's repository; until then §3.5 | When the repository moves to an organisation |
| CI-7 | `scorecard.yml` (§2) | Open |

---

## 9. The nightly and the support matrix

The `matrix-*` jobs in `nightly.yml` are the evidence behind [support-matrix.md](./support-matrix.md);
`make gate-docs` reconciles their names with the table and fails one the `report` job does not
wait on.

A failing nightly job files an issue labelled `finding`: one per job, reopened rather than
duplicated, closed by the first run that passes. It starts no work by itself.

The image scan targets `ghcr.io/<repo>:latest`, which exists only after a `v*` release; before
that the scan is skipped with a notice, and `govulncheck` scans the source every night regardless.

---

## 10. Publishing the website and the workbench

Both run after the merge and gate nothing, so their failures would land where nobody looks.

* **`website.yml`** builds `apps/website/dist`, proves it is plain static files, and mirrors it to
  the domain owner's webspace over SFTP with the host key pinned; until the variables are set it
  skips the upload with a notice. A failure files an issue under the nightly's rule. Its build half
  is a pull request gate: `node` runs `make website` first, in a checkout with no `dist/`.
* **`workbench.yml`** publishes the workbench to `workbench.hubtask.eu`, the same webspace in its
  own directory ([ADR-0038](../adr/ADR-0038-workbench-published.md)), with its own scoped SFTP
  account, never the website's. `WORKBENCH_REMOTE_DIR` defaults to that account's home, and the
  step refuses a target that is neither empty nor a previous workbench deploy: `mirror --delete` on
  the wrong directory would replace `hubtask.eu`.

---

## 11. The flow report and its thresholds

`flow.yml` runs `make flowstats` every Monday for the seven days that ended the day before, beside
the seven before those, and without the session figures: the transcripts are on the owner's
machine only, where `make flowstats SINCE=… UNTIL=…` prints them too. The report is one comment
per week on the issue "Flow report", which the job finds by its title and opens if it is missing.
The job starts no work.

**The thresholds are the owner's to set.** The values below are the first setting, proposed with
the report on 2026-11-21. Changing one is the owner's decision; it changes this table and
`tools/flowstats/thresholds.json`, which is what the tool reads, in the same pull request. The
file stands beside the tool because the tool embeds it, so the report cannot run with thresholds
other than the ones it was built with; a test of the tool holds the table and the file equal.

A measure over its threshold opens an issue labelled `finding`, titled "Flow report: <measure> is
over its threshold", with the measure, the value, the threshold and the window. There is one per
measure: an open one is left as it is, a closed one is opened again. A measure that was not
measured breaches nothing.

| Measure | Id | Threshold |
|---|---|---|
| Pull request run, median wall minutes | `pr_run_median_minutes` | ≤ 20 |
| Pull request runs failed, percent | `pr_runs_failed_percent` | ≤ 15 |
| `main` runs needing a second attempt, percent | `main_second_attempt_percent` | ≤ 10 |
| Hours `main` was red | `main_red_hours` | ≤ 2 |
| `fix` pull requests merged within seven days of a `feat` touching the same package | `fix_after_feat` | does not rise two windows running |
| Open `finding` issues, and the oldest | `open_findings` | reported |
| Open `decision` issues, and the oldest | `open_decisions` | reported |
| Uncited checks of built use cases | `uncited_checks_built` | does not rise |
| Fixed waits in the walks (`waitForTimeout`) | `fixed_waits` | does not rise |
| Timer waits in the walks (`setTimeout`) | `timer_waits` | does not rise |
| Suites without a use case | `suites_without_use_case` | does not rise |
| Stub answers outside the contract | `answers_outside_contract` | does not rise |
| Use cases by state | `use_cases_by_state` | reported |
| Lines added and removed under `docs/` outside `archive/` | `docs_lines` | reported |
| Lines of the root `AGENTS.md` | `agents_lines` | reported |
| Comment blocks of twenty lines or more | `comment_blocks` | does not rise |
| Tests deleted or skipped in merged pull requests | `lost_tests` | reported |
| Unreachable Go functions | `unreachable_go_functions` | reported |
| Unused translation keys | `unused_translation_keys` | reported |
| CodeRabbit: comments made | `coderabbit_comments` | reported |
| CodeRabbit: share of comments that led to a change | `coderabbit_share` | reported |
| Open code scanning alerts, and the oldest | `alerts_code_scanning` | reported |
| Open secret scanning alerts, and the oldest | `alerts_secret_scanning` | reported |
| Open Dependabot alerts, and the oldest | `alerts_dependabot` | reported |

Beside the table the report prints the merged pull requests by kind, the CI runs, the ten worst
jobs by failures and by median minutes, and each test a merged pull request deleted or skipped
with the line of its description that names it.

How each measure is read:

* **Runs** are `ci.yml`'s finished runs that were not cancelled. A pull request's run that took
  less than two minutes checked nothing and is left out. A run started by hand counts in no row.
  A run's wall time ends with its last attempt.
* **A second attempt** is a run on `main` whose newest attempt is not its first.
* **Red hours** run from the end of a failed run on `main` to the end of the next one that
  passed, inside the window. A run that failed and passed when run again counts as passed.
* **Jobs** are read from every attempt of the window's runs, one request per run.
* **A package** is a Go file's directory, or the workspace package of a file under `apps/` or
  `packages/`; a pull request's first hundred changed paths are read.
* **The tree's measures** are read from the checkout's commit, and beside it from the commit the
  window started with: the totals of `apps/webapp/e2e/bounds.json`, the lines of
  `test/contract/testdata/walks-baseline.txt`, the bound `uncitedBuiltAtMost` that
  `make gate-docs` holds, each use case's `state:`, and the runs of twenty or more `//` lines in
  the Go files under `core/`, `presentation/`, `infrastructure/` and `cmd/`.
* **A test deleted or skipped** is read from a pull request's merge commit, by the same reading
  as `make gate-pr` and with its limits; a skip is named by its file.
* **Issues and alerts** are counted as they are when the report runs, so they have no value for
  the window before.

**Not measured** is printed with its reason, never as a zero:

* Unreachable Go functions: `deadcode` is not among the tools `make tools` pins.
* Unused translation keys: no tool in the repository counts them.
* CodeRabbit's comments while no review comment of its login is in the window, and the share
  that led to a change, which nothing records.
* An alert kind the job's token may not read. Workflow permissions offer `security-events: read`
  for code scanning and nothing for secret scanning or Dependabot alerts; what the API answers
  the job's token is seen on its first run.
