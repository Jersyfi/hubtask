# CI/CD with GitHub Actions

The platform decision is [ADR-0022](../adr/ADR-0022-github-platform.md); when CI runs on a pull
request is [ADR-0079](../adr/ADR-0079-a-draft-is-checked-locally.md). The release gates the
pipeline serves are [versioning-release.md](./versioning-release.md) §6.

---

## 1. The principle: the pipeline calls nothing but `make`

Every step of a workflow is a `make` target; the workflow holds orchestration, not logic. So every
gate is reproducible locally — `make verify-pr` runs what the pull request pipeline runs for the
branch, `make verify` is its fast half (§3.4) — and switching platform touches only
`.github/workflows/`. The product (application, Helm chart, Compose files) depends on nothing
GitHub-specific; the pipeline may.

---

## 2. Workflows

| File | Trigger | Purpose |
|---|---|---|
| `ci.yml` | A pull request once it is ready (opened ready, leaving draft, or pushed to while ready); a push to `main` | The pull request gates (§3) |
| `codeql.yml` | A ready pull request, a push to `main`, weekly | Static security analysis. Not part of `CI required`; a red CodeQL is a finding to fix, not to suppress |
| `pr-description-rerun.yml` | A pull request's description or title is edited | If the head commit's latest `ci.yml` run failed, re-runs its description job and `CI required` (§3.1) |
| `nightly.yml` | Schedule (overnight) | Long runs (§9): `deep` (fuzzing, load, resilience), the `matrix-*` cells, the PITR drill (`make gate-pitr`), the image scan, the action pins; the arm64 cell also runs `make gate-privacy-full` and `make gate-selftest`. A failure files an issue |
| `release.yml` | Tag `v*` | After the `production` environment's approval: the gates again, image, SBOM, signature, provenance, chart, GitHub release ([deployment.md](./deployment.md) §7) |
| `deploy.yml` | Push to `main`, manual dispatch | Builds, signs and verifies the per-commit image and runs `helm upgrade` into `integration` ([deployment.md](./deployment.md) §3) |
| `website.yml` | Push to `main` touching `apps/website/`, `packages/design-system/` or the lockfile; manual dispatch | Builds `apps/website/dist` and mirrors it to the webspace (§10). A failure files an issue |
| `workbench.yml` | Push to `main` touching `packages/design-system/`; manual dispatch | Publishes the component workbench to `workbench.hubtask.eu` (§10) |
| `scorecard.yml` | Schedule | **Planned, not built.** The OpenSSF supply chain scorecard. The file does not exist; the row stays as the plan |

---

## 3. Jobs in `ci.yml`

Every job of `ci.yml`, in file order. `ci-required` waits for all of the others (§3.2).

| Job | Contents |
|---|---|
| `changes` | Classifies what a pull request touches (`dorny/paths-filter`, §3.1) |
| `secrets` | Gitleaks over the history (SG-7) |
| `dependencies` | Dependency review: no high-severity vulnerability, no GPL, LGPL, AGPL or SSPL licence ([licensing-editions.md](./licensing-editions.md) §1) |
| `licences` | `make gate-licenses`: no copyleft Go dependency, `THIRD-PARTY-LICENSES.md` current |
| `quick` | `make gate-quick` (gofmt, `golangci-lint` with `gosec` and `depguard`, `go vet`, `make generate` without a diff), `make gate-sdk` |
| `build` | `make build` for linux/amd64 and linux/arm64 |
| `unit` | `make gate-unit`: Go tests with `-race`, the gates' own tests under `tools/` among them, coverage `core/domain` ≥ 85 %, `core/application` ≥ 75 % |
| `architecture` | `make gate-architecture`: layer rules, the `go` ban outside `SafeGo`, mandatory authorisation, use case parity across REST, MCP and automation, RT-12, SG-13, the translation gate, `actionlint` |
| `resilience` | `make gate-resilience`: RT-1…RT-5, RT-7, RT-10, RT-12 (§3.2) |
| `observability` | `make gate-observability`: `promtool check rules` and `promtool test rules` over the four rule files, and `test/observability` |
| `chart` | `make gate-chart`: `helm lint` and `helm template` with every optional object on; the chart's copy of rules and dashboards current |
| `compose` | `make gate-compose`: builds the image from this commit and starts the self-hosting stack until `/readyz` answers |
| `e2e` | `make gate-e2e`: the `hubctl` session against the Compose stack (`scripts/hubctl-e2e.sh`), including `hubctl sync-conformance` |
| `engine-session` | `make gate-engine-conformance`: the first-party sync engine against its own Compose stack, answering [offline-sync.md](./offline-sync.md) §9's eight client requirements by number |
| `selftest` | `make gate-selftest`: one deliberate violation per configured rule, each expected to turn the build red |
| `integration` | PostgreSQL 16 (`pgvector/pgvector:pg16`), `make migrate`, `make gate-integration` (object storage from `test/s3test`, one pinned SeaweedFS — [ADR-0067](../adr/ADR-0067-s3-test-fixture.md)), then **`make gate-contract`**: responses, events, closed sets, the router and the SDK examples against the contract |
| `security` | `make gate-security` (`govulncheck`, `test/security`: cross-tenant, RLS, SSRF, uploads, authorisation, redaction) and `make gate-privacy` (PG-1, PG-3…PG-6, PG-8) |
| `data` | `make gate-data` (RE-1…RE-9, BK-1 for every target, SY-1…SY-12, audit) and `make gate-privacy-full` (PG-2, PG-7) |
| `docs` | `make gate-docs` (§3.1) |
| `pr-description` | `make gate-pr` over the description and the title read from the API; not for Dependabot's pull requests (§3.1) |
| `node` | Per affected workspace package: the workspace map lint, build, lint, typecheck, test; for the website `make website` first |
| `engines` | The built web app in Chromium, Firefox and WebKit through pinned Playwright ([ADR-0048](../adr/ADR-0048-browser-job-driver.md)), asserting [ADR-0044](../adr/ADR-0044-browser-support-row.md)'s feature table; the one job that runs a browser |
| `tokens-drift` | `make tokens`; the committed `LabelTokens.go` must not change |
| `api-client-drift` | `make api-client`, then the workspace built and typechecked against it; nothing generated may be committed |
| `ci-required` | The one required check (§3.2) |

`quick` is the prerequisite of every Go job; a skipped `quick` skips them all. The Go jobs after it
run in parallel.

**Contract gate.** In CI, `make gate-contract` runs in one place, the last step of
`integration`; there is no separate `contract` job. It needs no database and runs in seconds, so
`make verify` runs it as well. The OpenAPI diff against the last tag is not built (§8, CI-6).

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
| `ci` (`.github/**`, `.gitleaks.toml`) | Everything |
| documentation only | The four jobs behind no filter — `secrets`, `dependencies`, `licences`, `docs` — and the description check |
| anything, on a draft | Nothing. Every job is skipped and `ci-required` reports as `CI not run (draft)` (§3.4) |

On a push to `main` every filter output is `true`: the branch that gets released is never filtered.

**Four jobs are behind no filter.** A key or a copyleft dependency gets in through any path, so
`secrets`, `dependencies` and `licences` never filter. `docs` takes seconds and reads files every
filter could skip: `checkdocs` reconciles the Go version across `go.mod`, the workflows, the
Dockerfile, the support matrix and the README; reconciles the support matrix with the nightly's
jobs; resolves ADR citations in every kind of source file and document; checks links and anchors;
and holds the newest `docs/evidence/COVERAGE-<date>.md` to `catalogue.Descriptors()` — one row per
served use case, none for an unserved one, and no "nobody built it" without an issue number.

**`pr-description` reads the description, not the tree.** On every pull request that is not Dependabot's,
`tools/checkpr` holds the description to `.github/PULL_REQUEST_TEMPLATE.md`: every section present,
in order, filled or marked n/a; `Closes #n` at the start of a line or `No issue: <why>`; use cases
that exist; one ADR answer ticked and named; the template's Definition of Done items, none deleted
or reworded, each ticked or n/a; no placeholder left in Impact; and the title a Conventional
Commit. With the branch's history it also holds: a readiness record ready before the first commit
outside `docs/` and at the head, with something under each of its five sections, for a branch that carries a task, names a use case or changes the
contract, a migration, a query, a dependency, an ADR or what a use case promises (otherwise
`Readiness: n/a — <why>`); no merged migration changed; no use case deleted and no ID reused; a
change to a use case's Goal, How to check or Where it ends named `correction` or `decision #<n>`;
no settled ADR changed beyond its status line, its `Rule lives in` line and link targets; and
no numbered section of a subject document (`docs/architecture/`, `docs/design/`) dropped or
renumbered, nor left as a bare heading where it had text — a retired section keeps its heading
with one sentence saying where its content went. The
rules added after the readiness rule hold pull requests opened from 2026-10-08 on, as it does. It
reads the description and the title from the API, so a re-run judges them as they stand.
`make gate-pr BODY=<file> TITLE="<title>"` runs it locally, `BASE=origin/main` with the history. `gh pr create --body` never shows the template;
start from a copy of it.

**The filters name trees, and `test/architecture` checks that they name all of them.** Every
tracked file is claimed by a filter or listed among the paths that deliberately trigger nothing,
and every pattern matches something that exists. A filter by file pattern (`**/*.go`) would leave
the fixtures a Go test reads outside it, and `ci-required` counts that skip as a pass.

### 3.2 `ci-required` is the only required status check

(Both sections numbered 3.2 are cited, so neither is renumbered.)

A required check that never reports is **pending**, and blocks the merge for ever. So no pull
request workflow filters with a workflow-level `paths:` or `paths-ignore:`; filtering happens in
job `if:` conditions, and a job skipped by an `if:` does report.

```yaml
ci-required:
  name: CI required          # 'CI not run (draft)' on a draft
  if: always()
  needs: [ …every other job… ]
```

It fails if any dependency ended `failure` or `cancelled`, and passes when all ended `success` or
`skipped`. **Branch protection names `CI required` and nothing else**; a new job is added to
`ci-required`'s `needs`, never to a required-check list.

**On a draft it has another name.** Every job is skipped on a draft (§3.4), so a green
`CI required` there would satisfy branch protection without a gate having run. Its name is an
expression: `CI not run (draft)` on a draft, `CI required` otherwise. `test/architecture` holds the
expression, and `make gate-selftest` proves that test goes red without it.

A gate whose subject does not exist yet reports that it is skipping and stays green; it never
swallows a real failure, and the `selftest` job checks that the difference holds.

### 3.3 What a job no longer compiles again

The pinned tools are cached as binaries ([ADR-0062](../adr/ADR-0062-cached-tool-binaries.md)).
Jobs call `make tools-ensure`, which installs the whole set unless `.tools/.installed` names exactly
today's pins and the Go that built them and every binary is present; a restored cache that does not
match is thrown away. `release.yml` compiles its own set from the tag's pins, with no cache.

The project's Go build and module caches are not shared across jobs beyond `setup-go`'s cache.
Whether to share them is open, against the repository's 10 GB cache limit.

### 3.4 A draft is checked locally; CI runs when it is ready

The rule of [ADR-0079](../adr/ADR-0079-a-draft-is-checked-locally.md):

* **`ci.yml` and `codeql.yml` also trigger on `ready_for_review`.** Every job that needs no other
  job carries the draft condition; the others depend on one of them and are skipped with it. An
  edited draft's description re-runs nothing.
* **`make verify-pr` is the check before leaving draft.** It runs `make verify`, then every gate the
  pipeline would run for the branch's diff against `origin/main`, selected through `tools/cilocal`
  (the `changes` filters and a table of the jobs, which `test/architecture` holds to `ci.yml`).
  Three jobs are CI only: `secrets`, `dependencies`, and the description, which `verify-pr` checks
  with `make gate-pr` itself.
* **The container gates take turns** under a lock in the common git directory, because their
  projects and host ports are fixed and every worktree shares one Docker daemon. When Docker does
  not answer, they are reported as not run, and CI runs them after *Ready*.
* **A stamp and a hook.** On success `make verify-pr` writes the checked commit into the checkout's
  git directory. The hook in `.claude/settings.json` refuses `gh pr ready` unless that commit is
  `HEAD` and pushed, and refuses `gh pr create` without `--draft`.

For a change that touches everything, `make verify-pr` takes about fifteen minutes on a laptop.

---

## 4. Hardening

```yaml
permissions:
  contents: read          # the default for every workflow, widened only where necessary
```

* Actions are referenced by **commit SHA** with the tag as a comment; all actions of one
  repository share one commit (`test/architecture`). `make gate-action-pins` (nightly) asks GitHub
  whether each pin exists and the commented tag points at it.
* Repository setting "Allow select actions" with an allowlist.
* Secret scanning with push protection, and Dependabot alerts.
* No `pull_request_target`; contributions from forks run without secrets.
* Publishing only through the GitHub **environment** `production`, which has an approval rule.
* `cosign` keyless through the workflow's OIDC token — no private key in the repository.
  `id-token: write` only in the jobs that sign (`release.yml`, `deploy.yml`).
* Branch protection on `main`: linear history, mandatory review, **`CI required` as the only
  required status check** (§3.2), signed commits recommended.

---

## 5. AI in the pipeline

**AI comments, gates decide** ([ADR-0022](../adr/ADR-0022-github-platform.md)): AI assistance may
comment, suggest and draft; it never sets a status, merges, publishes, or replaces a gate.

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
The author reviews a change against the rules no gate checks (Definition of Done item 19); a
person or an AI agent of any make works on a task. A tool that steers coding agents later works
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

---

## 8. Open points

| # | Point | Needed by |
|---|---|---|
| CI-2 | Whether to enable the merge queue (worthwhile once several contributors work in parallel) | As needed |
| CI-6 | The OpenAPI compatibility check against the last tag (§3) | Before `1.0.0` |
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
