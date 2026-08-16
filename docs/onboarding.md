# Working Process: from here to the first running milestone

This guide covers how work flows in this project: the local environment, how to hand out
implementation tasks, what the pipeline does, and in which order to build things.

The setup steps — choosing the name, creating the repository, settling the licence — are done. What
was decided is recorded in [ADR-0013](./adr/ADR-0013-licensing.md) (licence),
[ADR-0022](./adr/ADR-0022-github-platform.md) (platform), and the
[licence and edition model](./architecture/licensing-editions.md).

---

## 1. Local working environment

```bash
# Prerequisites: Go 1.23+, Docker or Podman, make, git
make tools          # golangci-lint, oapi-codegen, sqlc, goose, govulncheck into .tools/
make db-up          # PostgreSQL for development
make migrate
make verify         # must be green before you commission anything
make run ROLES=api
```

Install Claude Code on your machine and start it **inside the repository directory** — `CLAUDE.md`
is then read automatically and contains the binding rules.

---

## 2. Repository configuration

The repository is one repository, not a monorepo with several products. Backend, documentation, the
Helm chart, deployment files, and tests belong together, because the CI gates do not work across
repositories: the use case parity check compares OpenAPI, the MCP manifest, and the automation
registry — if those live in different repositories, nobody checks anything any more. The frontend
gets its own repository later, because it has its own versioning and works through the published API
(`docs/roadmap.md`, phase 5).

The settings that matter, maintained through the GitHub UI:

* **Actions → General:** "Read and write permissions" **off**; the workflows request their own rights.
* **Branches:** a protection rule for `main` — a pull request required, CI green, no force pushes.
* **Environments:** `integration` without approval, `production` with a human approver. That way every release hangs on a deliberate confirmation.
* **Code security:** secret scanning, push protection, and Dependabot alerts enabled.

### Repository secrets

| Secret | For what | From where |
|---|---|---|
| `ANTHROPIC_API_KEY` | Claude Code in the pipeline (`@claude` in issues, architecture review on PRs) | console.anthropic.com → API keys |

Nothing more is needed initially. The push to `ghcr.io` uses the automatically provided
`GITHUB_TOKEN`, and signing goes through OIDC with no key management. Only when you want to roll out
to a Kubernetes cluster do `KUBE_CONFIG_INTEGRATION` and `KUBE_CONFIG_LIVE` join them — and only if
you stay with the push-based approach (see [deployment.md](./architecture/deployment.md)).

---

## 3. How to hand out implementation tasks

There are two routes. Both end in a pull request; neither writes directly to `main`.

### Route A: locally with Claude Code (recommended for the first milestones)

```bash
claude
> Read docs/backlog/milestone-0.1.0.md and implement task A-03.
```

The advantage: you see every step, you can intervene immediately, and `make verify` runs on your
machine. Work this way until the foundation stands — especially for the reference use case, which
sets the pattern for everything that follows.

### Route B: through GitHub issues

Create an issue using the "Implementation task" template, then either apply the `claude:task` label
or comment `@claude please implement`. The workflow works on a branch and opens a PR.

The advantage: asynchronous, traceable, several tasks in parallel. Worthwhile once the pattern
stands and tasks become more uniform.

### What makes a good task

The issue template asks exactly for it: use case names from the catalogue, the bounded context, the
documents to read, checkable acceptance criteria, and the cross-cutting impact (API, migration,
events, permissions, audit, data protection, the sync merge rule, i18n). A task that fills those
fields needs almost no follow-up questions. A task saying "do tasks somehow" creates rework.

**One task = one pull request.** If the scope grows beyond about eight files, split it.

---

## 4. What the pipeline does for you

| Trigger | What happens |
|---|---|
| Pull request | Format, lint, build (amd64/arm64), the unit, architecture, integration, contract, security, and data gates; plus an architecture review by Claude as a comment |
| Push to `main` | The same gates; then deployment to `integration` |
| Tag `v*` | The gates run again, a multi-arch image goes to `ghcr.io`, plus SBOM, keyless signature, provenance, the Helm chart as an OCI artefact, and a GitHub release — after manual approval |
| Overnight | Fuzzing, load test, resilience tests, a vulnerability scan of the published image |
| Weekly | Dependabot for Go modules, actions, and the base image |

The AI is deliberately **not** a gate: green or red is decided by tests, linters, `govulncheck`, and
the cross-tenant suite. Claude checks what a linter cannot see — agreement with the ADRs — and
leaves a comment. A review you can click away is more honest than one that blocks and therefore gets
circumvented.

---

## 5. The order of the work

Follow `docs/roadmap.md`. The first milestone is deliberately narrow and broad at once: **one** use
case, but through every layer and every cross-cutting concern. Only once `CreateContainer` is driven
all the way through — REST, MCP, automation registration, event, migration, metric, trace, audit,
cross-tenant test, sync entry — does every further use case have a pattern to copy.

The temptation to start with the data model for everything is strong. Resist it: one use case driven
end to end exposes defects in the cross-cutting concerns while they are still cheap to fix.

---

## Common pitfalls

| Pitfall | How to avoid it |
|---|---|
| Changing the module path afterwards | It is `github.com/Jersyfi/hubtask` and it is now load-bearing in every import and in the event type `de.hubtask.*` — changing it is a breaking change |
| Editing a migration afterwards | Migrations are immutable; a correction is a new migration |
| Hand-editing generated code | Change `openapi.yaml` or `db/queries`, then run `make generate` |
| Cutting tasks too large | One use case per task; split beyond about eight files |
| Switching a gate off "temporarily" | A gate that is switched off stays switched off. Shrink the task instead |
| Renaming a released event type | Event types are a public contract from the first release; add a `.v2` instead |
