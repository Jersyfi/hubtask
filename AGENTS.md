# Working on Hubtask

Instructions for everyone who changes this repository — a person or an AI coding agent of any make.
Agents load this file by themselves. A directory with its own `AGENTS.md` adds rules for that
directory. Binding.

## What Hubtask is

A task manager with five levels (Hub → Collection → Task → Work Package → Activity): multi-tenant,
offline-capable, Go and PostgreSQL behind a hexagonal core, two Svelte clients, Apache-2.0. The
architecture is decided and documented. It gets implemented, not redesigned.

## The map

```text
docs/vision/        why: principles, personas, deployments D1–D7, non-goals — the owner's
docs/usecases/      what must be true for a person — the yardstick for every change
docs/architecture/  the current rules, one subject document per concern
docs/design/        the design system and the product's voice
docs/adr/           why and when each decision was taken — a log, not the rules
docs/backlog/       milestones, tasks, readiness records, how work runs
docs/archive/       closed milestones and old run records — history only
core/               domain and application layer, technology-free
presentation/       inbound adapters: rest, mcp, stream, calendar, intake, worker, webui, openapi
infrastructure/     outbound adapters: postgres, storage, mail, httpclient, …
cmd/                the binaries; cmd/server/main.go is the composition root
api/                openapi.yaml, the source of the contract
db/                 migrations (forward only) and sqlc queries
apps/               webapp (the product UI), website (hubtask.eu, information only)
packages/           design-system, api-client, sync-engine, the n8n and Zapier connectors
sdk/                generated client libraries
```

Dependencies point inwards: `cmd → presentation, infrastructure → core`;
`core/application → core/domain, core/port`; `apps/* → packages/*`; never `apps/* → apps/*` or
`packages/* → apps/*`.

**Before you change a file under `core/`, `presentation/`, `apps/webapp/`, `apps/website/`,
`packages/design-system/`, `packages/api-client/`, `packages/sync-engine/` or `docs/usecases/`,
read that directory's `AGENTS.md`.** Some agents load it by themselves; read it anyway if yours
does not.

## Where knowledge lives

One place per kind of knowledge. Write it there; elsewhere, link to it.

| Knowledge | Place |
|---|---|
| Principles, personas, deployments, non-goals | `docs/vision/` |
| What must be true for a person | `docs/usecases/` |
| The current rule of a concern | its subject document in `docs/architecture/` or `docs/design/` |
| Why a rule is as it is | an ADR; its `Rule lives in` line points to the rule |
| What a milestone delivers, its tasks and decisions | `docs/backlog/milestone-<X>.md` |
| That a task is ready to build | `docs/backlog/ready/<TASK>.md` |
| How milestones, tasks, findings and decisions run | `docs/backlog/README.md` |
| Traps that have no home in code or a subject document | `docs/architecture/known-traps.md` |
| A question for the owner | an issue labelled `decision` |
| Something found outside the task | an issue labelled `finding` |
| How to work here | this file |

## Rules that do not bend

They take precedence over any task; if a task contradicts one, report it instead of breaking the
rule. The numbers are permanent — code cites them as "rule N". Migrations up to 0117 and older
documents cite them as "CLAUDE.md rule N", this file's name before 2026-10-07.

| # | Rule | Checked by |
|---|---|---|
| 1 | `core/domain` and `core/port` import no third-party library and nothing from `infrastructure/` or `presentation/`. Dependencies point inwards. | `[gate: gate-architecture]` |
| 2 | Authorisation happens only in the application layer — never in an adapter or a repository. | `[partial: gate-security; open: an adapter deciding a permission itself]` |
| 3 | Every database query runs through the transaction wrapper that sets `SET LOCAL app.tenant_id`; never `pgxpool` directly. | `[gate: gate-architecture]` |
| 4 | No `time.Now()`, `math/rand` or UUID generation in `core/domain` or `core/application` — only the `Clock`, `RandomSource` and `IDGenerator` ports. | `[gate: gate-architecture]` |
| 5 | No bare goroutines; concurrency only through `core/shared/concurrency.SafeGo`. | `[gate: gate-architecture]` |
| 6 | Every outbound HTTP call goes through `infrastructure/httpclient.GuardedClient`. | `[gate: gate-architecture]` |
| 7 | No call without a timeout or a context deadline. | `[partial: gate-quick; open: a context without a deadline further up]` |
| 8 | No display text in the backend — message codes and parameters only. | `[partial: gate-architecture; open: prose inside an error string]` |
| 9 | SQL only parameterised, through sqlc; no byte from a request becomes SQL text. The query DSL's one exception is bounded in `api-guidelines.md`. | `[partial: gate-quick; open: string building the linters miss]` |
| 10 | No user content (titles, notes, comments) in logs, metrics, traces or audit entries. | `[partial: gate-privacy; open: user content in a free-text field]` |
| 11 | `api/openapi.yaml` is the source: change it first, then `make generate`, then implement. Never hand-edit generated code. | `[partial: gate-quick; open: the order of the work]` |
| 12 | Migrations are forward-only and safe for rolling updates (expand/contract). A merged migration never changes. | `[gate: gate-pr]` |
| 13 | English everywhere: documents, code, identifiers, comments, commits. | `[unchecked: no tool judges language reliably]` |
| 14 | `core/` knows nothing about a frontend; no `.go` file under `apps/` or `packages/`. | `[gate: gate-architecture]` |
| 15 | No colour, spacing, radius or duration value outside `packages/design-system/tokens/tokens.json`; the generated `LabelTokens.go` is never hand-edited. | `[gate: ci:node]` |

## Working rules

- A task starts with its readiness record (`docs/backlog/ready/TEMPLATE.md`) as the branch's first
  commit, attacked by a reviewer who did not write it; code follows only once it says `ready` or
  `waiting on the owner`, and the pull request leaves draft only when it says `ready`.
  `[partial: gate-pr; open: the quality of the record and of the review]`
- A pull request starts as a draft and leaves draft only after `make verify-pr` passed for the
  pushed `HEAD`. `[partial: ci:ci-required; open: a skipped local run — CI fails instead]`
- A pull request description is a copy of `.github/PULL_REQUEST_TEMPLATE.md` with every section; it
  closes its issue (`Closes #n`) or says `No issue:` and why. `[gate: gate-pr]`
- A task of a released milestone carries only checks from the milestone's `Delivers`.
  `[unchecked: not yet gated]`
- One concern per commit; each commit builds, carries a Conventional Commit title and a
  `Task: <ID>` trailer, and keeps its tests beside the code. History is rewritten only while the
  pull request is a draft. `[unchecked: what one concern is, is judgement; CI checks the head]`
- Something found outside the task: fix it on the branch if the rules allow, in its own commit; a
  document that states the current system wrongly is corrected in the same pull request; a question
  on the owner's list becomes a `decision` issue; anything else a `finding` issue. `[owner]`
- The owner is asked only about items on the list in § "What you do not decide yourself", only
  after searching, only as a `decision` issue in its template's form. `[owner]`
- A use case's *Goal*, *How to check* and *Where it ends*, and anything in `docs/vision/`, change
  only by the owner's decision. `[owner]`
- A rule lives in its subject document; an ADR records why and names that place. Numbered sections
  of subject documents are never renumbered. `[partial: gate-docs; open: a renumbered section]`
- No file named `CLAUDE.md`, `CLAUDE.local.md` or `AGENTS.override.md` is committed — it would hide
  this file from some agents. `[gate: gate-architecture]`
- Merge only on the owner's word. `[unchecked: the owner's own agents work under the owner's GitHub
  identity; nobody else can merge, by GitHub permissions]`
- Knowledge another worker needs goes into the repository, never only into a tool's private notes;
  work is possible from any machine and with any tool. `[unchecked: private notes are outside the
  repository]`

## Working with the owner

- The owner works through coding sessions only: sets the direction, decides, looks at results. The
  session does the rest — development, administration, releases. `[owner]`
- User-interface changes are shown in the running app and refined on the same draft pull request
  until the owner is content; owner feedback on a task in progress is part of that task. How to
  show it: `docs/evidence/README.md`, "How to walk". `[owner]`
- When the owner works out a concept, the session applies the cut checklist of
  `docs/backlog/README.md` and says what is still open before the concept lands. `[owner]`
- Each new task starts in a fresh session, unless the owner asks for a series. `[unchecked: a
  preference about sessions]`
- An unattended run (scheduled, nightly) opens an issue and stops; it never starts the work.
  `[unchecked: depends on the run's own instructions]`
- Every new ADR is named to the owner. An ADR is short — context, decision, consequences, its
  `Rule lives in` line — and the rule itself changes in the subject document in the same pull
  request. `[owner]`

## The loop for every task

1. **Understand and settle.** Read the task and its issue, the use cases it names — completely —
   and the principles they serve, then the subject documents of the concern (§ "Reading"). Write
   the readiness record and have it attacked; commit it first.
2. **Plan in steps.** The steps are in the record. Open a draft pull request that closes the issue.
3. **Specification first.** API in `api/openapi.yaml`, data model as a migration and sqlc queries,
   then `make generate`.
4. **Implement from the inside out:** domain → application → ports → adapters → presentation. One
   step, one commit, pushed at once.
5. **Test.** Domain logic with table tests and no infrastructure; repositories with Testcontainers
   and a cross-tenant negative test for every new repository method; each use case check by a test
   that can prove it (`known-traps.md`).
6. **Check.** Every check the task carries is met with evidence (`docs/usecases/README.md` §
   "Checking work"); the change is reviewed against the rules no gate checks, findings named under
   *Definition of Done*; `make verify-pr` is green.
7. **Finish.** Fill in the template completely, move the use cases' `state:`, `checked_by:` and
   *Today*, then `gh pr ready`.

**Resuming:** compare `git log --oneline main..HEAD` with the steps in the record, run
`make verify`, continue with the first missing step. Work in your own worktree when another session
uses the checkout.

### Reading

Read selectively, but read completely what you read.

1. The use cases the task names, and the principles they serve.
2. `docs/architecture/domain-model.md` and `project-structure.md`.
3. `docs/architecture/api-guidelines.md`, before you touch an endpoint.
4. The subject document of the concern — `identity`, `security`, `multi-tenancy`, `audit`,
   `data-protection`, `data-retention`, `backup-restore`, `tenant-export`, `offline-sync`,
   `automation`, `ai-first`, `i18n-l10n`, `observability-reliability`, `deployment`, `ci-cd`,
   `versioning-release` — or `docs/design/design-system.md`.
5. `docs/architecture/known-traps.md`.

An ADR only when you need the reasoning behind a rule, or before you change one.

## What you do not decide yourself

Bring these to the owner with a worked-out proposal:

- A deviation from a rule in a subject document. Correcting a wrong statement is not one.
- A new third-party dependency.
- A change to `api/openapi.yaml` that renames or removes an existing field.
- A change to the licence, the security gates or the retention safeguards.
- Anything that could irrecoverably delete user data.
- A change to what a person, an administrator or an auditor observes in a use case's *Goal*,
  *How to check* or *Where it ends*, and anything in `docs/vision/`. A wrong reference or check
  number is a correction.
- A change to what a released milestone delivers.

Everything else you decide and write down why — in the readiness record, and where the next reader
of the code needs it.

**Asking.** Search `docs/vision`, `docs/architecture`, `docs/usecases`, the milestones' `Decisions`
and closed `decision` issues first; an answered question is cited, not asked again. Then open a
`decision` issue from its template — what it is about in plain words, the proposal walked through
D1–D7, the alternatives and why not, what waits until the answer — and tell the owner once, with
the list. A follow-up needs a fact the first question did not contain. The answer goes to `main` in
a small documentation pull request, into the place it governs, and that pull request closes the
issue.

## Which command checks what

| Changed | Run |
|---|---|
| Anything, while working | `make verify` |
| Before `gh pr ready` | `make verify-pr` — every gate CI runs for the branch, locally |
| One concern | `make gate-quick`, `gate-unit`, `gate-architecture`, `gate-security`, `gate-docs` |
| `api/openapi.yaml`, `db/queries/` | `make generate` (no diff may remain), `make api-client` |
| `packages/design-system/tokens/tokens.json` | `make tokens`, commit the regenerated `LabelTokens.go` |
| A translation in `locales/` | `make gate-architecture`; `make locales` for completeness |
| Anything in `apps/` or `packages/` | `pnpm -r build && pnpm -r lint && pnpm -r typecheck && pnpm -r test` |
| `deploy/docker/` | `make gate-compose` |
| A pull request description | `make gate-pr BODY=<file>` |

`go build ./...`, `go test ./...` and `make generate` work without Node.js. Run each gate on its own
line: `make gate-x | tail` hides a red gate from `&&`, and `git push | tail` a rejected push.
`make gate-selftest` edits the working tree — never beside another gate.

## When CI runs

A draft is checked in the session; CI runs once the pull request is ready (`ci-cd.md`,
[ADR-0079](docs/adr/ADR-0079-a-draft-is-checked-locally.md)). `gh pr create --draft`; leave draft
with `gh pr ready` after `make verify-pr`; rework on a ready pull request goes back to draft first
(`gh pr ready --undo`), bringing it up to date with `main` does not. Claude Code sessions are held
to this by the hook in `.claude/settings.json`; other agents keep it by themselves.

## Code comments

- Comment only what the code cannot say: why this and not the obvious alternative, an invariant, a
  trap — never what the next line does, never history, never plans. `[unchecked: judgement]`
- Cite only stable references: a rule (`rule 10`), a principle (`P-05`), a use case check
  (`UC-ID-12/4`), an existing identifier (`SG-3`, `RT-12`, `T-07`), a subject-document section
  (`security.md §9`), an ADR for the reasoning. Never a task ID, an issue or pull request number, or
  an instruction file. `[partial: gate-docs; open: single-letter task ids, which collide with alert and principle ids]`
- Text an API client or an end user reads — `api/openapi.yaml` descriptions and summaries, metric
  help — contains no internal reference; operator dashboards may link the operating documents. `[partial: gate-docs; open: metric help]`
- Every Go package has a package comment saying what it is responsible for.
  `[gate: gate-architecture]`

## Style

Small, self-contained changes — one pull request per use case or clearly bounded building block.
Errors are typed values, wrapped with `%w`. No speculative abstraction: the generalisation is
already in the domain model.
