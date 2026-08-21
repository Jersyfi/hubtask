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
| `ci.yml` | Pull request, push to `main` | The PR gates: format, lint, generation, build, tests, security, architecture |
| `nightly.yml` | Schedule (overnight) | Long runs: fuzzing, load test, resilience/chaos tests, rolling update simulation, memory leak test |
| `release.yml` | Tag `v*` | Compute the version, build the multi-arch image, SBOM, signature, provenance, Helm chart, GitHub release |
| `codeql.yml` | PR, schedule | Static security analysis |
| `scorecard.yml` | Schedule | OpenSSF supply chain scorecard |
| `ai-assist.yml` | PR (label `ai-review`), issue comment `/ai` | Advisory AI assistance (§5) |
| `docs.yml` | PR touching `docs/**` | Link and structure check of the architecture documentation, ADR index reconciliation |

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
| `architecture` | Import/layer rules, the `go` ban outside `SafeGo`, mandatory authorisation, use case parity across REST/MCP/automation, observability completeness (RT-12), audit registry (AU-1) | Structure |
| `selftest` | One deliberate violation per configured rule, each expected to turn the build red (`make gate-selftest`) | The gates themselves |
| `security` | `govulncheck`, `gosec`, cross-tenant suite (SG-3), RLS/`BYPASSRLS` test (SG-4), SSRF suite (SG-6), secret scan (SG-7), upload matrix (SG-12), auth negative tests (SG-11) | SG gates |
| `data` | Migration check against the previous state, deletion tests per storage location, retention tests RE-1…RE-9, backup round trip BK-1 (local + MinIO), sync tests SY-1…SY-12 | Data guarantees |

`integration`, `security`, and `data` run in parallel; `quick` is a prerequisite for all of them.

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
* No `pull_request_target`; contributions from forks run without secrets.
* Publishing only through a GitHub **environment** (`production`) with an approval rule.
* `cosign` keyless via the workflow's OIDC token — no private key in the repository.
* `GITHUB_TOKEN` with `id-token: write` only in the release job.
* Branch protection on `main`: linear history, mandatory reviews, all gates green, signed commits
  recommended.

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

---

## 6. Required repository secrets and variables

| Name | Kind | Purpose | Required |
|---|---|---|---|
| `ANTHROPIC_API_KEY` | Secret | AI assistance in `ai-assist.yml` | Only if AI is used |
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
