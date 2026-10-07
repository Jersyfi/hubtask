# CI/CD with GitHub Actions

The platform decision is [ADR-0022](../adr/ADR-0022-github-platform.md); when CI runs on a pull
request is [ADR-0079](../adr/ADR-0079-a-draft-is-checked-locally.md). This document is the current
pipeline. The release gates it serves are listed in [versioning-release.md](./versioning-release.md) §6.

---

## 1. The principle: the pipeline calls nothing but `make`

Every step of a workflow is a `make` target. The workflow contains orchestration, not logic.
Therefore:

* Every gate is reproducible locally. `make verify-pr` runs what the pull request pipeline runs for
  the branch; `make verify` is its fast half (§3.4).
* Switching platform touches only `.github/workflows/`. The product — the application, the Helm
  chart, the Compose files — depends on nothing GitHub-specific; the pipeline may.
* A contributor gets the same result before pushing as after.

---

## 2. Workflows

| File | Trigger | Purpose |
|---|---|---|
| `ci.yml` | A pull request once it is ready — opened ready, leaving draft, or pushed to while ready; a push to `main` | The pull request gates (§3) |
| `codeql.yml` | A pull request once it is ready, a push to `main`, weekly schedule | Static security analysis. It reports new alerts in the code a pull request changed. It is not part of `CI required` and not a required check, but a red CodeQL is a finding to fix, not to suppress |
| `pr-description-rerun.yml` | A pull request's description is edited | Re-runs the failed jobs of the latest `ci.yml` run, so a fixed description turns `CI required` green (§3.1) |
| `nightly.yml` | Schedule (overnight) | Long runs (§9): fuzzing, load and resilience (`deep`); the support matrix cells (`matrix-*`, [support-matrix.md](./support-matrix.md)); the point-in-time recovery drill against a real operator and object store (`make gate-pitr`); the vulnerability scan of the published image; the action pins (`make gate-action-pins`). The arm64 matrix job also runs `make gate-privacy-full` and `make gate-selftest` on the other architecture. A failure files an issue (§9) |
| `release.yml` | Tag `v*` | After the `production` environment's approval: the gates again, the multi-arch image, SBOM, signature, provenance, the Helm chart, the GitHub release ([deployment.md](./deployment.md) §7) |
| `deploy.yml` | Push to `main`, manual dispatch | Builds and signs the per-commit image, verifies the signature, and runs `helm upgrade` into the `integration` environment ([deployment.md](./deployment.md) §3) |
| `website.yml` | Push to `main` touching `apps/website/`, `packages/design-system/` or the lockfile; manual dispatch | Builds `apps/website/dist` and mirrors it to the webspace over SFTP (§10). A failure files an issue |
| `workbench.yml` | Push to `main` touching `packages/design-system/`; manual dispatch | Publishes the component workbench to `workbench.hubtask.eu` (§10) |
| `scorecard.yml` | Schedule | **Planned, not built.** The OpenSSF supply chain scorecard. The file does not exist; the row stays as the plan |

---

## 3. Jobs in `ci.yml`

Every job of `ci.yml`, in file order. `ci-required` waits for all of the others (§3.2).

| Job | Contents | Gate |
|---|---|---|
| `changes` | Classifies what a pull request touches (`dorny/paths-filter`); every other job reads its outputs (§3.1) | — |
| `secrets` | Gitleaks over the history (SG-7). Behind no filter | Secret scan |
| `dependencies` | GitHub's dependency review on a pull request: no high-severity vulnerability, no GPL, LGPL, AGPL or SSPL licence ([licensing-editions.md](./licensing-editions.md) §1). Behind no filter | Supply chain |
| `licences` | `make gate-licenses`: no copyleft Go dependency, and `THIRD-PARTY-LICENSES.md` current. Behind no filter | Licences |
| `quick` | `make gate-quick` (gofmt, `golangci-lint` including `gosec` and `depguard`, `go vet`, `make generate` without a diff) and `make gate-sdk` (the generated Python SDK parses) | Format, lint, generation |
| `build` | `make build` for linux/amd64 and linux/arm64 | Buildability |
| `unit` | `make gate-unit`: the Go tests with `-race`, the gates' own tests under `tools/` among them, coverage `core/domain` ≥ 85 % and `core/application` ≥ 75 % | Unit |
| `architecture` | `make gate-architecture`: layer rules, the `go` ban outside `SafeGo`, mandatory authorisation, use case parity across REST, MCP and automation, observability completeness (RT-12), audit declarations (SG-13), the translation gate, and `actionlint` over the workflows | Structure |
| `resilience` | `make gate-resilience`: RT-1…RT-5, RT-7, RT-10, RT-12 (§3.2) | Reliability |
| `observability` | `make gate-observability`: `promtool check rules` and `promtool test rules` over the four rule files, and the runbook/alert/dashboard checks in `test/observability` | Alerts and runbooks |
| `chart` | `make gate-chart`: `helm lint` and `helm template` with every optional object on, and the chart's copy of the rules and dashboards current | Helm chart |
| `compose` | `make gate-compose`: builds the image from this commit and starts the self-hosting stack until `/readyz` answers. It is also the container build | Self-hosting |
| `e2e` | `make gate-e2e`: the `hubctl` session against the Compose stack (`scripts/hubctl-e2e.sh`), including `hubctl sync-conformance` | The client contract, end to end |
| `engine-session` | `make gate-engine-conformance`: the first-party sync engine driven through its own API against its own Compose stack, answering each of [offline-sync.md](./offline-sync.md) §9's eight client requirements by number | The client's half of offline synchronisation |
| `selftest` | `make gate-selftest`: one deliberate violation per configured rule, each expected to turn the build red | The gates themselves |
| `integration` | PostgreSQL 16 (`pgvector/pgvector:pg16`) as a service container, `make migrate`, `make gate-integration` (repository and use case tests; object storage from `test/s3test`, which starts SeaweedFS at the one pinned version — [ADR-0067](../adr/ADR-0067-s3-test-fixture.md)), then **`make gate-contract`**: responses against `api/openapi.yaml`, events against their JSON schemas, the closed sets a descriptor and the contract each declare, the router against the specification, the SDK examples | Integration, contract |
| `security` | `make gate-security` (`govulncheck` and the suites under `test/security`: cross-tenant, RLS, SSRF, uploads, authorisation, redaction) and `make gate-privacy` (PG-1, PG-3…PG-6, PG-8) | SG and PG gates |
| `data` | `make gate-data` (retention RE-1…RE-9, backup round trip BK-1 for every target, sync SY-1…SY-12, audit) and `make gate-privacy-full` (PG-2 and PG-7 against a migrated database) | Data guarantees |
| `docs` | `make gate-docs` (§3.1). Behind no filter | Documentation |
| `pr-description` | `make gate-pr` over the description and the title read from the API. Not for Dependabot's pull requests (§3.1) | The description |
| `node` | Per workspace package, when it or something it consumes changed: the workspace map lint, then build, lint, typecheck and test; for the website also `make website` before any build | Clients and packages |
| `engines` | The built web application in Chromium, Firefox and WebKit through Playwright, pinned ([ADR-0048](../adr/ADR-0048-browser-job-driver.md)), asserting [ADR-0044](../adr/ADR-0044-browser-support-row.md)'s feature table in each (`pnpm --filter @hubtask/webapp test:engines`). The one job that runs a browser | The browser row of the support matrix |
| `tokens-drift` | `make tokens`; the committed `LabelTokens.go` must not change | Design tokens |
| `api-client-drift` | `make api-client`, then the whole workspace built and typechecked against it; nothing generated may be committed | The TypeScript client |
| `ci-required` | Passes when every job it waits for ended `success` or `skipped` (§3.2) | The one required check |

`quick` is the prerequisite of every Go job; a skipped `quick` skips them all. The Go jobs after it
run in parallel.

**Contract gate.** In CI, `make gate-contract` runs in one place: the last step of `integration`.
There is no separate `contract` job. It needs no database and runs in seconds, so `make verify`
runs it as well.

**Not built.** The OpenAPI diff against the last tag, which [versioning-release.md](./versioning-release.md)
§6 lists as the compatibility check, does not exist yet. Nothing fails a breaking change to the
contract except review and the contract tests above.

**The hubctl session spends the rate limit.** Every API call `scripts/hubctl-e2e.sh` makes through
its request helpers uses `curl --retry`. The session runs as one client against the anonymous and
per-token budgets, so a section appended late spends a limit the earlier sections already drew on,
and a `429` there is the session's own budget answering. The sync engine's conformance run has a
stack of its own for the same reason.

### 3.2 Where the resilience tests run

**A test that can run in minutes on a shared runner is a pull request gate; a test that needs an
hour of load is nightly.**

* RT-1…RT-5, RT-7, RT-10 and RT-12 run in the `resilience` job on every pull request. The job takes
  about four and a half minutes beside jobs that take six to eight, so it costs no wall clock, and a
  defect it finds reaches the run that reviews the diff that caused it.
* RT-6 (overload), RT-8 (rolling update under load) and RT-11 (memory over an hour) need sustained
  load and run in `nightly.yml` through `make gate-load`. What RT-6 asserts survives a noisy runner:
  shedding engaged, no interactive request was refused, the interactive P95 stayed inside its
  target, and the process recovered. A percent-level question is not asked of a shared runner — that
  is the release tier of the [load suite](../../test/load/README.md) ([observability-reliability.md](./observability-reliability.md) §13.2).

### 3.1 Where a job runs, and where it does not

The `changes` job classifies the files a pull request touches; every other job decides from its
outputs whether it has work.

| Changed (filter) | What runs |
|---|---|
| `go`, `openapi`, `db`, `deploy` | The whole Go pipeline, all security gates included |
| `openapi` | Additionally: the API client is regenerated and the workspace typechecks against it; every package generated from the contract runs |
| `design_system` | Additionally: the tokens are regenerated and the committed `LabelTokens.go` must not move |
| `webapp`, `website`, `design_system`, `api_client`, `sync_engine`, `n8n_node`, `zapier_app`, `sdk_typescript` | The `node` job for the affected packages and the packages that consume them |
| `webapp`, `design_system`, `api_client`, `go`, `openapi`, `db`, `deploy` | The container build (`compose`), because the image contains both halves ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) |
| `ci` (`.github/**`, `.gitleaks.toml`) | Everything |
| documentation only | The four jobs behind no filter — `secrets`, `dependencies`, `licences`, `docs` — and the description check |
| anything, on a draft | Nothing. Every job is skipped and `ci-required` reports as `CI not run (draft)` (§3.4) |

On a push to `main` every filter output is `true` and the whole pipeline runs. There is no filtering
on the branch that gets released.

**Four jobs are behind no filter.** A key and a copyleft dependency get in through any path, a
stylesheet or a README included, so `secrets`, `dependencies` and `licences` never filter. `docs`
takes seconds and reads files every filter could skip: `checkdocs` reconciles the Go version across
`go.mod`, the workflows, the Dockerfile, the support matrix and the README; reconciles the support
matrix with the nightly's jobs; resolves ADR citations in `.go`, `.md`, `.sql`, `.yaml` and `.tpl`;
checks links and anchors; and holds the newest `docs/evidence/COVERAGE-<date>.md` to
`catalogue.Descriptors()` — one row per use case the catalogue serves, none for one it does not
serve, and no omission "nobody built it" without an issue number.

**`pr-description` reads the description, not the tree.** On every pull request that is not Dependabot's,
`tools/checkpr` holds the description to `.github/PULL_REQUEST_TEMPLATE.md`: every section present,
in order, filled or marked n/a; `Closes #n` at the start of a line or `No issue: <why>`; use cases
that exist; one ADR answer ticked and named; every Definition of Done item ticked or n/a; no
placeholder left in Impact. It reads the description from the API, so a re-run judges it as it
stands. `make gate-pr BODY=<file>` runs it locally. `gh pr create --body` never shows the template;
start from a copy of it.

**The filters name trees, and `test/architecture` checks that they name all of them.** Every
tracked file is claimed by some filter or listed among the paths that deliberately trigger nothing,
and every pattern a filter names matches something that exists. A filter by file pattern (such as
`**/*.go`) leaves the fixtures a Go test reads outside it, and `ci-required` counts the resulting
skip as a pass.

### 3.2 `ci-required` is the only required status check

Two sections carry the number 3.2. Both are cited from code and other documents, so neither is
renumbered: the one above is the resilience split, this one the required check.

A required status check that never reports is not skipped, it is **pending**, and a pull request
with a pending required check can never merge. So no workflow that runs on pull requests filters in
a workflow-level `paths:` or `paths-ignore:` trigger — a workflow that does not trigger produces no
check at all. The filtering happens inside jobs, in `if:` conditions; a job skipped by an `if:`
does report.

```yaml
ci-required:
  name: CI required          # 'CI not run (draft)' on a draft
  if: always()
  needs: [ …every other job… ]
```

It fails if any dependency ended as `failure` or `cancelled`, and passes when all ended as
`success` or `skipped`. **Branch protection names `CI required` and nothing else.** A new job is
added to `ci-required`'s `needs` and to no required-check list; adding a job there reintroduces the
deadlock above.

**On a draft it has another name.** Every job is skipped on a draft (§3.4), so `ci-required` would
pass, and a green `CI required` on a draft's commit would satisfy branch protection without a gate
having run. Its name is therefore an expression: `CI not run (draft)` on a draft, `CI required` on
every other run. `test/architecture` holds the expression, and `make gate-selftest` proves that test
goes red without it.

A gate whose subject does not exist yet reports that it is skipping and stays green. A gate must
never swallow a real failure; the `selftest` job checks that the difference still holds.

### 3.3 What a job no longer compiles again

The pinned tools are cached as binaries ([ADR-0062](../adr/ADR-0062-cached-tool-binaries.md)).
Jobs call `make tools-ensure`, which installs the whole set unless `.tools/.installed` names exactly
today's pins and the Go that built them, and every binary those pins name is present. The cache
decides nothing: a restored directory that does not match is thrown away and installed again.
`release.yml` compiles its own set from the pins of the tagged commit, with no cache.

The project's own Go build and module caches are not shared across jobs beyond `setup-go`'s cache.
Whether to do so is open: thirteen jobs each keeping a build cache is a real question against the
repository's 10 GB cache limit.

### 3.4 A draft is checked locally; CI runs when it is ready

The rule of [ADR-0079](../adr/ADR-0079-a-draft-is-checked-locally.md):

* **`ci.yml` and `codeql.yml` trigger on `ready_for_review` as well.** Every job that needs no
  other job carries `github.event_name != 'pull_request' || !github.event.pull_request.draft`; all
  others depend on one of them and are skipped with it. The review notice is posted when a pull
  request opens ready or leaves draft. An edited draft's description re-runs nothing.
* **`make verify-pr` is the check before leaving draft.** It runs `make verify`, then every gate
  this pipeline would run for the branch's diff against `origin/main`, selected through
  `tools/cilocal`: the `changes` filters and a table of the jobs, which `test/architecture` holds to
  `ci.yml` — the same jobs `ci-required` waits for, the same filters per job and per package. Three
  jobs are CI only and say why: `secrets`, `dependencies`, and the description, which `verify-pr`
  checks with `make gate-pr` itself.
* **The container gates take turns.** Their compose projects and host ports are fixed, and every
  worktree on a machine shares one Docker daemon, so `make verify-pr` runs them under a lock in the
  common git directory. When Docker does not answer, it reports them as not run, and the pipeline
  runs them after *Ready*.
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

* Actions are referenced by **commit SHA** with the tag as a comment
  (`actions/checkout@<sha> # v7.0.1`). All actions of one repository share one commit
  (`test/architecture`). `make gate-action-pins`, nightly because it needs the network, asks GitHub
  whether each pin is a commit that exists and whether the tag in the comment points at it.
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

**AI comments, gates decide.** AI assistance in the pipeline is advisory only
([ADR-0022](../adr/ADR-0022-github-platform.md)): it may comment, suggest and draft; it never sets
a status, merges, publishes, or replaces a gate. The gate that decides is `CI required` (§3.2).

No workflow calls a model today, and none reviews or works on its own (§5.1). The table and the
rules below are the outer limit for an AI workflow should one be added; adding one is the owner's
decision, recorded in an ADR.

| Use | What it may do | What it must not do |
|---|---|---|
| Review assistance | Check the diff against the architecture rules and comment with locations | Set a status, or merge anything |
| Test suggestions | Suggest test cases, particularly negative and edge cases | Commit tests |
| ADR draft | Offer a draft as a comment when an architectural change has no ADR | Create an ADR |
| Change log | Draft release notes | Determine the version or the tag |
| Translation drafts | Suggest message translations in `locales/*.json` | Change `en.json`; a translation needs a human ([i18n-l10n.md](./i18n-l10n.md)) |
| Triage | Categorise issues, suggest duplicates, ask for missing information | Close issues |

Rules for any AI workflow:

* It runs only on explicit request (a label or a comment), not on every push.
* The model receives the diff — no secrets, no production data.
* Every AI contribution is marked as such, with the model name and the date.
* The workflow has `contents: read` and `pull-requests: write` — no write access to code.
* Costs are capped per run and per month; exceeding the cap aborts the run, not the gate.
* An unavailable AI provider is not a pipeline failure (`continue-on-error: true`).

### 5.1 No workflow reviews or works on its own

No workflow reviews a pull request or starts work by itself, and none offers a trigger that would.
The author reviews a change against the rules no gate checks (Definition of Done item 19); a
person or an AI agent of any make works on a task. A tool that steers coding agents later works
through the same issues, labels and pull requests ([backlog README](../backlog/README.md)
§ "For tools that steer the work"), under its own GitHub App identity, without the right to merge.

### 5.2 An image scan describes the published image

The nightly vulnerability scan reads the newest published image (`:latest`, else `:rc`), not
`main`. An alert can therefore be true of that image and already fixed in `go.mod` on `main`. Check
`main` first; if it carries the fix, dismiss the alert as **won't fix** (never *false positive* —
it is true of that image), with a comment linking the pull request that fixed it. Only the
govulncheck half of the job is actionable from a branch.

---

## 6. Repository secrets and variables

| Name | Kind | Used by | Purpose |
|---|---|---|---|
| `GITHUB_TOKEN` | Provided | Every workflow | GHCR pushes, the release, issues |
| `KUBE_CONFIG` | Secret, environment `integration` | `deploy.yml` | The deploy identity for the integration cluster, limited to its namespace |
| `WEBSITE_SFTP_USER`, `WEBSITE_SFTP_PASSWORD` | Secret | `website.yml` | The website's SFTP account |
| `WORKBENCH_SFTP_USER`, `WORKBENCH_SFTP_PASSWORD` | Secret | `workbench.yml` | The workbench's own, scoped SFTP account |
| `WEBSITE_SFTP_HOST`, `WEBSITE_SFTP_HOST_KEY` | Variable | `website.yml`, `workbench.yml` | The webspace and its pinned host key (one webspace) |
| `WEBSITE_REMOTE_DIR`, `WORKBENCH_REMOTE_DIR` | Variable | `website.yml`, `workbench.yml` | Target directories |

**There is no credential for production in this repository.** Production pulls a published chart
and image through Argo CD ([deployment.md](./deployment.md) §4); no workflow can reach it.

---

## 7. Runner selection

Every job runs on GitHub's free hosted runners. `arm64` images are built with
`docker/build-push-action` under QEMU; the arm64 matrix cells run on native `arm64` runners.

Load is measured in two tiers, and no runner is bought for it: a **relative regression guard** in
the nightly on `ubuntu-latest`, comparing a run against a stored baseline with an explicit noise
band, and a **full capacity ramp per release on the integration server**, the named hardware. A
shared runner varies 10–30 % between runs and is not asked a percent-level question
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

The `matrix-*` jobs in `nightly.yml` are the evidence behind [support-matrix.md](./support-matrix.md):
the full suite natively on arm64, the integration suite against every PostgreSQL major the table
claims, the Compose stack under Podman, `hubctl` on each native platform, and the chart installed
into a throwaway kind cluster. Their names are load-bearing: `make gate-docs` reconciles them with
the table in both directions, and fails a matrix job that the nightly's `report` job does not wait
on.

A failing nightly job files an issue labelled `finding`: one issue per job, reopened rather than
duplicated, and closed again by the first run that passes. It starts no work by itself.

The nightly image scan targets `ghcr.io/<repo>:latest`, which exists only after `release.yml` has
run on a `v*` tag. Before the first release the scan is skipped with a notice in the run summary;
`govulncheck` scans the source every night regardless.

---

## 10. Publishing the website and the workbench

Both run after the merge and gate nothing, so their failures would land where nobody looks.

* **`website.yml`** builds `apps/website/dist`, proves it is plain static files, and mirrors it to
  the domain owner's webspace over SFTP with the host key pinned (`WEBSITE_SFTP_HOST_KEY`). Until
  the variables are set it builds, checks, and skips the upload with a notice. A failure files an
  issue under the nightly's rule, and the first publish that succeeds closes it.
* **The website's build half is a pull request gate.** The `node` job runs `make website` for the
  website before it builds anything else, in a checkout with no `dist/`, so the deploy command is
  exercised in the condition CI has and a developer's machine never has.
* **`workbench.yml`** publishes the workbench to `workbench.hubtask.eu`, the same webspace in its
  own directory ([ADR-0038](../adr/ADR-0038-workbench-published.md)). It uses its own scoped SFTP
  account and never falls back to the website's; host and host key are shared. `WORKBENCH_REMOTE_DIR`
  defaults to that account's home, and the publish step refuses a target that is neither empty nor
  a previous workbench deploy, because `mirror --delete` on the wrong directory would replace
  `hubtask.eu`.
