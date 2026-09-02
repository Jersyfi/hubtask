# CI/CD with GitHub Actions

Implements [ADR-0022](../adr/ADR-0022-github-platform.md) and makes concrete the gates from
[versioning-release.md](./versioning-release.md) §6.

---

## 1. The principle: the pipeline calls nothing but `make`

Every step of a workflow is a `make` target. The workflow contains no logic, only orchestration.
Which means:

* Every gate is reproducible locally (`make verify` = the complete PR pipeline).
* Switching platform touches only `.github/workflows/`.
* Contributors get the same result before pushing as after.

---

## 2. Workflows

| File | Trigger | Purpose |
|---|---|---|
| `ci.yml` | Pull request, push to `main` | The PR gates: format, lint, generation, build, tests, security, architecture, data, chart, Compose, documentation and licences |
| `nightly.yml` | Schedule (overnight) | Long runs: fuzzing, load and resilience tests, the support matrix cells ([support-matrix.md](./support-matrix.md)), the privacy gates that need a database — PG-2 and PG-7 (`make gate-privacy-full`) — and the whole of `make gate-selftest`, whose probes for those two are skipped where there is no PostgreSQL, the vulnerability scan of the published build, the action pins. A failure files an issue labelled `claude:task` |
| `release.yml` | Tag `v*` | Compute the version, build the multi-arch image, SBOM, signature, provenance, Helm chart, GitHub release |
| `deploy.yml` | Push to `main`, manual dispatch | `helm upgrade` into the `integration` environment ([deployment.md](./deployment.md) §3) |
| `codeql.yml` | PR, schedule | Static security analysis |
| `scorecard.yml` | Schedule | OpenSSF supply chain scorecard |
| `claude-review.yml` | Pull request, unless it is a draft or Dependabot's | **Switched off** — posts the review checklist as the record that no automated reviewer ran (§5) |
| `claude.yml` | `@claude` in an issue, comment or review; the label `claude:task` on an issue | **Switched off** — answers that delegation is off rather than dropping the request (§5) |

---

## 3. Jobs in `ci.yml`

Staggered by runtime: whatever fails fastest runs first.

| Job | Contents | Gate |
|---|---|---|
| `quick` | `gofmt`, `go vet`, `golangci-lint`, `make generate` with no diff | Format, lint, generation |
| `build` | `go build ./...` for linux/amd64 and linux/arm64 | Buildability |
| `unit` | Domain and application tests, coverage thresholds (85% / 75%) | Unit gate |
| `integration` | Service container PostgreSQL 16, `goose up`, repository and use case tests; object storage and the other backup targets come from Testcontainers | Integration |
| `contract` | Responses against `openapi.yaml`, events against JSON schemas, OpenAPI diff against the last tag | Compatibility |
| `architecture` | Import/layer rules, the `go` ban outside `SafeGo`, mandatory authorisation, use case parity across REST/MCP/automation, observability completeness (RT-12), audit declarations (SG-13) | Structure |
| `selftest` | One deliberate violation per configured rule, each expected to turn the build red (`make gate-selftest`) | The gates themselves |
| `security` | `govulncheck`, `gosec`, cross-tenant suite (SG-3), RLS/`BYPASSRLS` test (SG-4), SSRF suite (SG-6), secret scan (SG-7), upload matrix (SG-12), auth negative tests (SG-11) | SG gates |
| `data` | Migration check against the previous state, retention tests RE-1…RE-9, backup round trip BK-1 (local + MinIO), sync tests SY-1…SY-12 | Data guarantees |
| `security` (same job) | The privacy gates that need no database: PG-1, PG-3, PG-4, PG-5, PG-6, PG-8 (`make gate-privacy`, also part of `make verify`) | Data protection |

`integration`, `security`, and `data` run in parallel; `quick` is a prerequisite for all of them.

### 3.2 Where the resilience tests run, and why (O-4)

**The chaos-shaped RT tests are a pull request gate; the load-shaped ones are nightly.** That is
what the pipeline does, and G-12 is where it stops being an accident and becomes a decision — made
by measuring what the job costs on the shared runners rather than by preference.

| Measured on the shared runner | Time |
|---|---|
| `resilience` on a pull request (RT-1…RT-5, RT-7, RT-10, RT-12) | 4m22s, 4m29s (PRs #243, #242) |
| The longest job in the same run (`selftest`) | 7m55s, 7m57s |
| `integration` beside it | 6m39s, 5m11s |
| The nightly `Fuzzing, load, resilience` job whole | 4m33s |

The resilience job takes about four and a half minutes and runs in parallel with jobs that take
six and eight, so it costs **no wall clock at all**: the pull request is not waiting for it, and
moving it to the nightly would save nothing a contributor would notice. What it buys is the thing
that decides the question — a dependency failure, a database outage, a process killed mid-job or a
rule looping is a defect that reaches `main` in the diff that caused it, and the run that finds it
has to be the run that reviews it. A nightly failure two days later is a bisect.

The three that stay nightly — **RT-6** (overload), **RT-8** (rolling update under load) and
**RT-11** (memory leak over an hour) — are nightly for a reason no measurement changes: they need
sustained load for an hour or more, and a shared runner cannot give it. Their home is
`nightly.yml`, and CI-1 (a self-hosted runner for load tests) is what would let them come closer.

The rule this states, for a resilience test written later: **a test that can run in minutes on a
shared runner is a pull request gate; a test that needs an hour of load is nightly.** Nothing in
between is a judgement call.

### 3.1 Where a job runs, and where it does not

One repository holds the Go core, two clients and the design system ([ADR-0027](../adr/ADR-0027-monorepo-structure.md)),
so most jobs have nothing to say about most pull requests. A `changes` job
(`dorny/paths-filter`) classifies what a pull request touches; every other job decides from its
outputs whether it has work to do.

| Changed | What runs |
|---|---|
| `go`, `openapi`, `db`, `deploy` | The whole Go pipeline, including all twelve security gates |
| `openapi` | Additionally: `packages/api-client` is regenerated and the workspace typechecks against it |
| `design_system` | Additionally: all token targets are regenerated and the committed `LabelTokens.go` must not move |
| `webapp`, `website`, `design_system`, `api_client` | Lint, typecheck, test and build — for the affected packages and the packages they consume |
| `webapp`, `design_system`, `api_client`, `go`, `deploy` | The container build, because the image contains both halves ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) |
| documentation only | The documentation gate, and nothing else |
| `.github/**` | Everything, no exceptions |

Three jobs are behind no filter at all — `secrets`, `dependencies` and `licences`. A key and a
copyleft dependency get in through any path, including a stylesheet and a README, so a filter that
could skip them is a filter that will.

On a push to `main` and on a tag every filter output is `true` and the whole pipeline runs. There
is no filtering on the branch that gets released.

### 3.2 `ci-required` is the only required status check

**This is the trap the design exists to avoid**, and it is worth stating plainly because getting
it wrong blocks every merge in the repository until somebody notices.

A required status check that never reports is not "skipped", it is **pending** — and a pull
request with a pending required check can never merge. So the filtering must never happen in a
workflow-level `paths:` or `paths-ignore:` trigger, because a workflow that does not trigger
produces no check at all. Every workflow here triggers on every pull request, and the filtering
happens inside jobs, in `if:` conditions. A job skipped by an `if:` *does* report.

`ci-required` is what turns that into a single answer:

```yaml
ci-required:
  name: CI required
  if: always()
  needs: [ …every other job… ]
```

It fails if any dependency ended as `failure` or `cancelled`, and passes when they all ended as
`success` or `skipped`. **Branch protection names `CI required` and nothing else.** Adding an
individual job to the required list re-introduces exactly the deadlock above, so a new job is
added to `ci-required`'s `needs` and nowhere else.

A gate whose subject does not exist yet (no migrations, no contract tests) reports that it is
skipping and stays green. It starts biting the moment the first package appears - which is what
lets the milestone build all gates up front and fill them in task by task. What must never happen
is the reverse: a gate that swallows a real failure. Whether the difference still holds is exactly
what the `selftest` job checks.

---

## 4. Hardening

```yaml
permissions:
  contents: read          # the default for every workflow, widened only where necessary
```

* Actions are referenced by **commit SHA**, not by tag (`actions/checkout@<sha> # v4.2.2`),
  and `make gate-action-pins` — a nightly job, because it needs the network — asks GitHub whether
  each pin is a commit that exists and whether the tag in the comment points at it. Half of the
  rule is invisible in a diff: a hex string looks equally correct whether it is the right commit,
  a stale one, or none at all, and a pin nothing can resolve is not a stricter pin but a step that
  never runs.
* Repository setting "Allow select actions" with an allowlist.
* Repository security settings: secret scanning with push protection, and Dependabot alerts.
* No `pull_request_target`; contributions from forks run without secrets.
* Publishing only through a GitHub **environment** (`production`) with an approval rule.
* `cosign` keyless via the workflow's OIDC token — no private key in the repository.
* `GITHUB_TOKEN` with `id-token: write` only in the release job.
* Branch protection on `main`: linear history, mandatory reviews, **`CI required` as the only
  required status check** (§3.2), signed commits recommended.

---

## 5. AI in the pipeline

It is used where it saves time without taking on responsibility. The guard rail is simple:
**AI comments, gates decide.**

| Use | What it does | What it must not do |
|---|---|---|
| PR review assistance | Check the diff against the architecture rules: layer violations, missing authorisation, missing cross-tenant tests, user content in logs, calls without a timeout — as a comment with locations | Set a status, or merge anything |
| Test suggestions | Suggest test cases for new use cases, particularly negative and edge cases | Commit tests automatically |
| ADR draft | When an architectural change without an ADR is detected, offer a draft as a PR comment | Create an ADR itself |
| Change log | Draft release notes from the Conventional Commits | Determine the version or the tag |
| Translation drafts | Suggest new message codes in further languages (`locales/*.json`) | Change `en.json`; translations need human approval |
| Triage | Categorise issues, suggest duplicates, ask for missing information | Close issues |

Rules:

* Runs only on explicit request (the `ai-review` label or an `/ai …` comment), not on every push —
  otherwise it costs money and creates noise.
* The diff is handed to the model; **no** secrets, no production data. Repositories with
  confidential content switch the workflow off.
* Every AI contribution is marked as such and carries the model name and the date.
* The workflow has `contents: read` and `pull-requests: write` — no write access to code.
* Costs are capped (a character limit per run, a counter per month); exceeding the cap aborts the
  run, not the gate.
* If the AI provider is down, that is **not** a pipeline failure (`continue-on-error: true`).

### 5.1 Both AI workflows are off during the initial development phase

Every task is worked on locally in a Claude Code session, so the architecture review happens there
— by the session that wrote the change, before the pull request leaves draft — and delegation to CI
would only add a second opinion from the same model on its own work. The `[G]` markers in the
backlog are a decision for the end of that phase; until then every task is `[L]`.

Neither workflow was deleted, and that is the point. `claude-review.yml` posts the checklist the
session applies, as the record that no automated reviewer ran; `claude.yml` answers a
`claude:task` label with a comment saying delegation is off. A trigger that did nothing and said
nothing about why is the failure this repository has already had: the review previously called the
action with input names the pinned version had renamed, and reported a green check in 29 seconds
without reviewing anything. **A green check that reviews nothing is worse than no check.**

Re-enabling either means fixing the inputs first — `direct_prompt` became `prompt`,
`allowed_tools` and `disallowed_tools` became `claude_args` — and then proving the gate can go red,
the way `make gate-selftest` proves it for the deterministic ones.

---

## 6. Required repository secrets and variables

| Name | Kind | Purpose | Required |
|---|---|---|---|
| `ANTHROPIC_API_KEY` | Secret | AI assistance in `claude-review.yml` and `claude.yml` | Only if AI is used |
| `COSIGN_EXPERIMENTAL` | Variable (`1`) | Keyless signing | Yes (release) |
| `HELM_REPO_TOKEN` | Secret | Publishing the chart (if external) | Optional |
| `SLACK_WEBHOOK` / `MATRIX_WEBHOOK` | Secret | Notification on release or failure | Optional |

`GITHUB_TOKEN` is provided automatically; GHCR needs no additional credentials. **There is no
secret for production systems in the repository** — deployment into your own environment happens
through a pull-based path (Argo CD/Flux) or manually, not through the public pipeline.

---

## 7. Runner selection

Public repositories use the free GitHub runners. `arm64` builds happen through
`docker/build-push-action` with QEMU, or on native `arm64` runners where available. Load tests in
the nightly run need more resources than the free runners provide — a self-hosted runner is planned
for that (open point CI-1).

---

## 8. Open points

| # | Point | Needed by |
|---|---|---|
| CI-1 | Self-hosted runner for load tests | `0.6.0` |
| CI-2 | Whether to enable the merge queue (worthwhile once several contributors work in parallel) | As needed |
| CI-3 | Enforce image signature verification at deployment (Kyverno/Sigstore policy) | `0.9.0` |
| CI-4 | ~~Where `hubtask.eu` is served from, and which workflow deploys it~~ — answered with F1-12: the domain owner's IONOS webspace, published by `.github/workflows/website.yml` over SFTP with the host key pinned. What remains with the owner: setting `WEBSITE_SFTP_HOST`, `WEBSITE_SFTP_HOST_KEY`, `WEBSITE_REMOTE_DIR` and the two secrets in the repository settings, and pointing the domain at the webspace — the workflow builds, checks and skips politely until then | Closed (F1-12), configuration pending |
| CI-5 | Where the component workbench is served from — answered with ADR-0038: `workbench.hubtask.eu`, the same IONOS webspace in its own directory, published by `.github/workflows/workbench.yml` on every change to `packages/design-system/`. It publishes with its own scoped SFTP account (`WORKBENCH_SFTP_USER`/`WORKBENCH_SFTP_PASSWORD`) and never falls back to the website's; the host and host key are shared because it is one webspace. `WORKBENCH_REMOTE_DIR` defaults to that account's home, and the publish step refuses a target that is neither empty nor a previous workbench deploy — `mirror --delete` on an unscoped account would otherwise replace `hubtask.eu` silently. What remains with the owner: the DNS record and the webspace mapping | Closed (ADR-0038), configuration pending |

---

## The support matrix

The `matrix-*` jobs in `nightly.yml` are the evidence behind
[support-matrix.md](./support-matrix.md): the full suite natively on arm64, the integration suite
against every PostgreSQL major the table claims, the Compose stack under Podman, and the chart
installed into a throwaway kind cluster. Their names are load-bearing — `make gate-docs`
reconciles them with the table in both directions, so renaming a job without its row turns the
build red, and deleting one does too.

A failing nightly job files an issue with the `claude:task` label rather than staying a red run in
a tab nobody opens; one issue per job, reopened rather than duplicated, so a platform that has been
broken for a week is one thread instead of seven.

The nightly image scan targets `ghcr.io/<repo>:latest`, which only exists once `release.yml` has
run on a `v*` tag. Before the first release the scan is skipped rather than failed, with a notice
in the run summary — `govulncheck` scans the source every night regardless. A red run whose only
message is "no release yet" trains people to stop reading the nightly, which costs more than the
scan it stands in for.
