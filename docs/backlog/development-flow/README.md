# Concept — the development flow: guidelines, proof, and the owner on the loop

**State:** 2026-11-22. The owner accepted D-1 to D-15 and D-8a on 2026-11-21 (§6).
It was built on the branch `concept/development-flow`, stage by stage, and is on `main` since
2026-11-22 (`a198ed9ce`); §7 says what the merge showed. §3 holds what was measured before the build, against `main` at
`77661b5a6` and the GitHub API; a figure read while building says so. Where something was not
checked, the text says so. What was not built is named where it is described, and in §9.

**What it answers.** How Hubtask stays fast to build with coding agents and safe to change, when the
owner no longer reads code: the owner writes concepts, takes strategic decisions and looks at
results; agents build; rules, gates, an independent review and a standing report hold the quality.
And what an outside contributor needs to take part.

---

## 1. The answer in one page

The repository is already built for agents: fifteen rules with gates, a pipeline that calls only
`make`, one required check, use cases with numbered checks, `decision` and `finding` issues. That
stays. Nine things are missing or have slipped, and each has a small, checkable remedy:

| # | Gap, measured | Remedy |
|---|---|---|
| 1 | **CI got three times slower and five times less reliable in four weeks** (a pull request's run: median 9.5 → 33.0 min, failed 7 % → 33 %). Nobody saw it, because the review that would have was a date somebody had to remember; its issue is still open. | A weekly **flow report** made by a scheduled job, with thresholds the owner sets once. A breach opens a `finding`. (§5.6) |
| 2 | **The browser walks have no written rules.** 212 suites, about 1,135 walks, 54,000 lines; every one runs against hand-written answers that nothing holds to the API contract; 355 fixed waits; 56 suites cite no use case check. | Rules for a walk in `apps/webapp/e2e/AGENTS.md`, each with a gate or a bound that only falls; the stubbed answers checked against the contract. (§5.4) |
| 3 | **The builder reviews its own work.** The lesson "the reviewer is never the builder" was paid for with a discarded build of 339 commits and lives in one build plan and in private notes. | An **independent review** by a fresh agent before every pull request leaves draft, reported in the description. (§5.5) |
| 4 | **A concept reaches CI only at the end.** The work surfaces arrived as one pull request of 134,813 added lines in 1,385 files; four of its CI runs failed before one passed; fix pull requests followed within two days. | **Checkpoints**: CI runs on the concept branch after each family of screens, without leaving draft. (§5.3) |
| 5 | **Nothing forces the full picture before a structural change.** The Definition of Ready lists seventeen points; nothing asks a worker to answer them. | A **design brief** of one screen for a named class of changes, written from a research pass, challenged once. (§5.3) |
| 6 | **Rules the next worker needs sit in the owner's private notes**, and `AGENTS.md` (259 lines) mixes what is always true with how-to procedures. | The notes move into the repository; procedures become **skills** loaded on demand; a table says where each kind of knowledge goes. (§5.2) |

| 7 | **Documentation grows ten lines for each one cut**, and no rule says how a text is written for a person, how much a change adds or what it removes. | Five writing rules, a table of what each kind of change owes, and a rule for removing what a change makes false — with no length limit, because a limit produces shorthand. (§5.9) |
| 8 | **Leftover code is found only inside a package.** Nothing reports an unreachable exported function, an unused file or a test whose subject is gone. | A change removes what it orphans; a tool's report drives removal in its own pull request; cleanup by a model's judgement is left out; a deleted test is named. (§5.10) |
| 9 | **The repository shows none of what it does.** Coverage thresholds, static analysis, vulnerability scans, signed releases with a bill of materials and pinned actions all exist; the README carries no badge and no line about them. | One short README section, two badges a machine fills and a third once the Best Practices project is registered, and the proof state printed from the use cases. No coverage service, no number to chase. (§5.12) |

A second reviewer from another vendor, CodeRabbit, is added as advice after the independent review
(§5.11). For contributors the repository lacks little: three policy sentences, a labelled entry point and one
command to run it locally (§5.8).

What this concept does **not** do: lower a gate, drop a use case check, add an AI workflow to CI,
add a required reviewer, or introduce a second quality class. CodeRabbit comments; it sets no
required check and decides nothing.

---

## 2. What was examined

* `AGENTS.md` and the eight nested ones, `CONTRIBUTING.md`, `docs/README.md`, `ci-cd.md`,
  `engineering-guidelines.md`, `known-traps.md`, `docs/backlog/README.md`,
  `docs/usecases/README.md`, `docs/evidence/README.md`, ADR-0079, -0083, -0089, -0090, the
  work-surface concept and its build plan, `ci.yml`'s triggers and filters, `scripts/check.sh`, the
  hook, the pull request template, the issue forms.
* GitHub: branch protection, repository settings, the community profile, labels, issues, the last
  200 `ci.yml` runs on `main`, pull request 1820 and its runs, issue 1309.
* `make flowstats` for 2026-10-23 … 2026-11-20, beside the baseline on issue 1309.
* The 212 suites under `apps/webapp/e2e/`, by counting patterns.
* The article the owner named and the tool documentation (§4, sources in §10).

**Not examined:** the product code's correctness, the security of the implementation, the content
of the 105 ADRs beyond the six named, performance, and whether the hot-file conflicts issue 1309
described were reduced. Nothing here is a claim about those.

---

## 3. Findings

### 3.1 What holds and stays

* **Rules a machine checks.** Of the fifteen rules, four are fully gated, ten partly and one not
  at all, and every rule says which gate holds it and what stays open. `make gate-selftest` plants a violation per rule
  and expects red — the gates themselves are tested.
* **`make` is the pipeline.** Local and CI run the same targets; one required check.
* **Use cases as the yardstick**: 294 files — 145 `verified`, 115 `built`, 32 `partial`, 1
  `specified`, 1 `retired`; a `verified` check must be cited by a test, and the uncited checks of
  `built` ones are bounded (313 today) by a number that only falls.
* **A draft costs nothing**, the merge needs no catch-up, a red `main` holds the queue.
* **The community files are complete**: GitHub's community profile reads 100 %.

### 3.2 CI slowed down and nobody was told

| | 2026-10-13 … 10-22 | 2026-10-23 … 11-20 |
|---|---|---|
| Pull request run, wall minutes, median / p75 | 9.5 / 14.8 | 33.0 / 56.2 |
| Pull request runs failed | 7 % | 33 % |
| `main` run, wall minutes, median / p75 | 10.6 / 18.2 | 27.9 / 41.4 |
| `main` runs failed | 13 % | 11 % |

Of the last 200 runs on `main` (2026-10-14 … 11-21), 51 needed more than one attempt. The per-job
figures are §5.7.1: the browser walks set the run's length and fail most.

ADR-0089 and ADR-0090 both end with "the review on or after 2026-11-05 compares the next window".
Issue 1309 carries that review — it is the review of ADR-0083's merge flow, which both ADRs
attach to — and is open with no report after 2026-10-22. The mechanism for
looking at the flow in aggregate exists as a tool (`make flowstats`) and as an intention, not as
something that happens.

### 3.3 The walks prove a screen against an imitation, by no written rule

Counted in `apps/webapp/e2e/*.test.mjs`:

| | |
|---|---|
| Suites / walks (`test(` calls) / lines | 212 / about 1,135 / 54,378 |
| Suites that drive a real server | 0 — every `/api/v1` request is answered by `fixture.mjs` or a stub in the suite |
| Anything that holds those answers to `api/openapi.yaml` | nothing found: the fixture is hand-written objects |
| Suites with route stubs of their own / calls | 172 / 382 |
| `waitForTimeout` (a fixed wait) | 353 uses in 93 suites, and 2 in two shared helpers |
| A timer (`setTimeout`), the other way to wait a fixed time | 35 uses in 23 suites, and 2 in two shared helpers |
| Suites that cite no `UC-…` | 50; another 6 name a use case and no check of it |
| Skipped or to-do walks | 0 |

The rows from the route stubs to the suites that cite no use case are those of the branch as
stage 2 found it (`main` at `5fa8a7c2e`), counted by `apps/webapp/build/walks.test.js`: a call is
`.route(` or `.waitForTimeout(` in a file's text, and a suite cites a check when its text holds
`UC-XX-nn check n`, `UC-XX-nn/n` or `UC-XX-nn checks …`. The first count of this document, by
`grep` on `main` at `77661b5a6`, was 171 suites with 378 route stubs, 351 fixed waits and 50
suites that cite no use case.

A walk is what makes a use case check about a screen `verified`. Today such a check is proven
against answers a worker typed. If the server answers another shape, the walk stays green and the
screen breaks; the real server meets the web app only in the installed apps' smoke walk and in
`hubctl`'s session, which use other clients. `docs/evidence/README.md` has rules for *exploring* by
script ("Scripting a walk") and says a stub belongs in the fixture, not in the suite — 172 suites
do otherwise. Rules for a walk *as a test* are written nowhere. This matches what the owner saw:
new end-to-end tests that do not follow the intended principles, because the principles are not
on paper where a worker reads them.

### 3.4 The author is the only reviewer

`engineering-guidelines.md` §3.19: the author reviews the change against every rule tagged
`[partial]`, `[unchecked]` or `[owner]`. The same agent, in the same context, with the same blind
spots, judges its work. The work-surface build plan records what that cost: "The builder judged its
own result; 'kept differences' piled up", and its remedy, "The reviewer is never the builder". That
remedy is in `docs/backlog/work-surface/build-plan.md` and in private notes, not in the rules.

### 3.5 A concept meets CI once, at the end

Pull request 1820: 100 commits, 134,813 lines added, 17,936 removed, 1,385 files, opened
2026-11-15, merged 2026-11-19. Its concept says "commits only, no CI". Its `ci.yml` runs: four
failed and five cancelled before the one green run. In the two days after the merge, five `fix`
pull requests landed — three for the release tooling, two for the product, one of those titled
"the owner's frontend findings". A change of that size cannot be
reviewed by a person, and its risk arrives in one piece.

### 3.6 The full picture is asked for, not produced

`engineering-guidelines.md` §2 (Definition of Ready) names what must be known before building:
context and aggregate, API impact, events, permissions, tenancy and RLS, automation, migration,
security, failure behaviour, data protection, audit, sync, client availability. ADR-0089 removed
the written readiness record because its bulk was matrices and a second agent run on every task —
while noting that 53 of 120 records had found a false premise. Since then nothing makes a worker
answer §2 before a structural change; the subject documents alone are 1.3 MB, so "read everything"
is not an option either (ADR-0090 measured about 190,000 tokens for the required reading).

### 3.7 Where knowledge sits

* `AGENTS.md` was 259 lines on `main`; the tool's documentation recommends under 200 per
  instruction file, because longer files are followed less well. On this branch it is 293 lines:
  the procedures left for the skills, and the table of where knowledge lives, the list of
  skills and the working rules this concept adds take more than they freed. The recommendation
  is not taken as a limit (§5.9): the tagged rule lists alone take about 166 lines. On `main` it
  held three kinds of text: what is true of every
  change (the map, the fifteen rules, the loop), how the owner's own workers run the flow
  (decisions, milestones, merging), and procedures (resuming, stacking, asking).
* Six standing rules exist only in the owner's private notes on one machine: mockups are the
  specification; a concept is cut by inventory, foundation and families with a separate reviewer;
  one branch and one pull request for a big concept; one worker at a time; report only what is
  checked or measured; the user-experience bar. `AGENTS.md` itself says such knowledge belongs in
  the repository. A contributor, or another tool, cannot follow rules it cannot read.
* `.claude/` holds one hook and no skills or agent definitions.

### 3.8 Contributors

One fork, no pull request from anyone but the owner and Dependabot (864 and 18 merged), no issue
labelled `good first issue` or `help wanted`. `CONTRIBUTING.md` is accurate and complete on set-up.
What a newcomer does not find: which contributions are wanted, what happens to a pull request and
how fast, whether work done with an AI tool is welcome, and who repairs a description the
description gate refuses. Running the app locally takes four manual steps.

### 3.9 Architecture, from the outside

The decisions examined fit a product built by agents under one owner: a hexagonal modular monolith
on one database keeps every rule checkable in one build; contract first gives three channels from
one catalogue. Two structural costs are visible without reading the code, and both are owner
choices, not defects:

* **Surface.** Eleven directories of inbound adapters under `presentation/`, two clients, installed apps on three systems, connectors,
  and about 300 use cases before a first release, with nobody but the owner using it yet (the
  build plan's words). Each surface is a standing cost per change — the Definition of Done has
  nineteen points because the product has that many edges.
* **Hot files.** In 57 commits since 2026-10-23: `locales/en.json` in 30, `api/openapi.yaml`
  (45,892 lines) in 24, `cmd/server/main.go` (7,086 lines) in 20, `apps/webapp/e2e/fixture.mjs` in
  16. They are where parallel work collides and what a worker must load to change anything.
  Splitting them is outside this concept; it is named in §9.

### 3.10 Documentation grows and is rarely cut; leftover code is found only in part

Counted on 2026-11-21:

| | |
|---|---|
| Markdown under `docs/` | 731 files, 107,925 lines; 46,752 of them in `archive/` |
| Lines added / removed in `docs/` outside `archive/`, 2026-10-23 … 11-21 | 27,381 / 2,623 — ten added for each one removed |
| Comment lines in non-test Go under `core/` | 54,181 of 304,847 lines (18 %) |
| Comment blocks of ten lines or more / twenty or more, in `core/`, `presentation/`, `infrastructure/`, `cmd/` | 660 / 47 — the rule says "a few lines" |
| Pull request description, last 30 merged | median 58 lines, longest 487 |

What the rules say today: one place per kind of knowledge; a comment says only what the code
cannot, in a few lines; an ADR is short; a wrong document found next to the work is fixed there.
`make gate-docs` holds structure — links, section numbers, citations, rule tags — and nothing about
how a text reads or how long it is. No rule says how a document is written for a person, how much
a change adds, or what a change removes. The owner's standing instruction — as much as needed, as
little as possible, understandable to a person — is typed into prompts and written nowhere.

Leftover code: the Go linter `unused` reports identifiers nothing in their package uses, and the
two apps compile with `noUnusedLocals` and `noUnusedParameters`. Nothing reports an exported Go
function no binary reaches, an unused file, export or dependency in the TypeScript workspace, an
unused translation key, or a test whose subject is gone. How much of each exists was **not
measured**: the one tool run that would count unreachable Go functions needs a download this
session was not allowed to make.

### 3.11 Quality and security are held, and shown nowhere

What exists, read from the `Makefile`, the workflows and the GitHub API on 2026-11-21:

| Held today | Where |
|---|---|
| Test coverage per package: at least 85 % in `core/domain`, 75 % in `core/application`; a planted violation proves the threshold bites | `make gate-unit`, `gate-selftest` |
| Static security analysis of the Go code (CodeQL, the extended security queries) | `codeql.yml`: ready pull request, `main`, weekly |
| Known vulnerabilities: reachable ones in Go, the published image, new dependencies on a pull request | `govulncheck`, Trivy nightly, dependency review |
| Secrets: scanning in CI, GitHub's scanning and push protection | `ci.yml`; repository settings |
| Releases: a bill of materials, a signature, a build provenance attestation | `release.yml` |
| Every one of the 161 action uses pinned to a commit; every workflow declares its permissions | the workflows |
| A security policy and private vulnerability reporting | `SECURITY.md`; enabled |
| Fuzzing, load, resilience, the support matrix, a recovery drill | `nightly.yml` |

Open alerts: code scanning 0, secrets 0, Dependabot 3.

What is missing:

* **Nothing is shown.** `README.md` (202 lines) has no badge and no section on quality or
  security; a visitor cannot tell this repository from one without any of the above.
* **The clients' code is outside two of the measures**: no coverage is measured for TypeScript and
  Svelte, and CodeQL is configured for Go only.
* **No outside measure**: no OpenSSF Scorecard, no Best Practices badge.
* **The project's own strongest measure is not published**: how many use case checks a test
  proves (§3.1).

The coverage figure the last `main` run reached was not read; only the thresholds are stated here.

---

## 4. What the research says, and what transfers

**The article** (engineering.fb.com, 2026-10-29, "Catching bugs before they land") describes tests
generated per code change that are meant to *fail* when the change has a bug. Overnight, for
changes a risk score marks as risky, two generators run: one treats the change itself as a
possible fault and writes tests that tell it from its parent; one infers the change's intent,
imagines ways it could go wrong, and writes tests against those. Failing tests are candidates. An
ensemble of models and a set of deterministic rules rank them, and only the best reach an
engineer, who answers one question: was this behaviour change intended?

*Human on the loop*, in its words: engineers "set criteria and review outcomes at an aggregate
level rather than supervising every output". Its closing list: define success criteria early, let
the AI execute, monitor outcomes in aggregate, intervene at the policy level.

Its numbers: 589 engineers engaged (64 % of those contacted), who reported 60 % of the findings as
true; 74 serious privacy bugs caught before landing in the first half of 2026. It does not say how
tests are kept, what a run costs, or how the ranking threshold is set, and two of its tables are
images that were not read.

**What transfers to a repository with one owner:**

| From the article | Here |
|---|---|
| Criteria first, aggregate monitoring, intervention at the policy level | The owner sets thresholds once; a weekly report shows the flow; the owner changes a rule, not a pull request (§5.6) |
| Target the risky changes | A named class of structural changes gets a design brief (§5.3) |
| The reviewer asks "was this change in behaviour intended?", not "is this code good?" | The independent review reports differences against the use cases and rules; the builder or the owner answers (§5.5) |
| Deterministic rules beside model judgement | Every new rule gets a gate or a falling bound where a tool can count it; the model reviews only what no tool can (§5.4, §5.5) |
| A test must be able to fail | *Red first*: a test cited as proof must fail on the code without the change (§5.4) |

**What does not transfer:** the generator pipeline itself. It needs a risk model trained on
history, an ensemble of judges and spare compute, and its 60 % is what it reaches *with* those.
Hubtask already has the cheaper half of the idea — `make gate-selftest` is mutation testing of the
gates. Generated catching tests are left as an experiment after everything else stands (§7, "Later").

**On documentation** (pages read 2026-11-21, §10):

* Google's documentation guide: a small set of fresh, accurate documents beats a large one in poor
  repair; documentation changes in the same change as the code; dead documentation misinforms and
  is deleted — slowly, first what is certainly wrong, the unclear later; do not duplicate, link.
* A study of repository context files for coding agents (Gloaguen et al., arXiv 2602.11988): such
  files did not generally raise task success and raised cost by over 20 % on average, for files a
  model wrote and files developers wrote alike. Instructions were followed; repository overviews
  did not help. The files are useful for practices that are not standard. So more text for the
  agent is a cost, not a safety margin.
* Diátaxis separates four kinds by what the reader is doing: learning (tutorial), getting a task
  done (how-to), looking something up (reference), understanding (explanation). Hubtask's
  `docs/README.md` already separates by kind; what it lacks is the reader's question per kind.
* The Go team's `deadcode` reports functions no `main` reaches. It is valid for one build
  configuration at a time, does not understand `//go:linkname`, and its own page says "some
  judgement is required". Knip reports unused files, exports and dependencies in a TypeScript
  workspace and lists a Svelte plugin; its page gives no false-positive rate, and support for a
  pnpm workspace was not confirmed from it.

**On showing quality** (sources read 2026-11-21, §10):

* **Coverage is a weak target.** Google's testing guidance: a high percentage does not guarantee
  good tests; there is no ideal number; pressure to hit the figure backfires; mutation testing is
  the better technique; gating the coverage of new code is an option. For code written by agents
  this weighs more: a model asked for a number produces the number.
* **OpenSSF Scorecard** runs twenty automated checks as a free weekly workflow and publishes a
  badge. Its own documents call the checks heuristics and say the aggregate "tells you nothing
  about what individual behaviors a repository is or is not doing". A repository with one
  maintainer cannot score on *Code-Review* (reviews by a bot or a model do not count),
  *Contributors*, or *Branch-Protection* beyond its first tier.
* **OpenSSF Best Practices badge**: free, self-certified by answering criteria with links. Tests
  and a vulnerability reporting process are required, a coverage figure only suggested. The site
  also carries the OpenSSF Baseline: level 1 fits any number of maintainers, level 2 needs two.
* **Mutation testing** (changing the code to see whether a test notices): for Go, the maintained
  tool says of itself that it is made for small modules and can take hours on large ones;
  StrykerJS is active and lists Svelte.
* **Coverage services** are free for open source; they need an app or a token on the repository,
  and what access the app takes was not found.
* **The EU Cyber Resilience Act** (Regulation 2024/2847) applies from 11 December 2027, its
  reporting duties from 11 September 2026. It covers products made available on the market in a
  commercial activity; open-source software its makers do not monetise is outside. Monetised — a
  price, paid support beyond cost, a platform that earns elsewhere — the maker is a manufacturer,
  and a natural person can be one: a machine-readable bill of materials, vulnerability handling
  and disclosure, security updates for a support period of at least five years, a 24-hour early
  warning, CE marking. The lighter "steward" role is open to legal persons only.

**Tool facts used below** (documentation read 2026-11-21): the coding tool in use reads a root
`AGENTS.md` natively when no `CLAUDE.md` exists, and a directory's `AGENTS.md` when it reads a file
there; a skill is a folder with a `SKILL.md` whose description is always in context and whose body
loads only when used, in an open format; a subagent starts with a fresh context and does not see
the conversation that started it; plan mode can be a project default but cannot be enforced; on a
public repository a pull request from a fork gets no secrets, so no model can run on it in CI.

---

## 5. The concept

### 5.1 Four instruments, one bar

Quality is the same for every change — a fix, a task, a concept, a contribution. What differs is
only how much each instrument has to do.

| Instrument | Holds | Who acts |
|---|---|---|
| **A rule** | What must be true, in the one place for its kind (§5.2) | The owner decides; a worker writes |
| **A gate** | Everything a tool can count or test | `make check`, CI |
| **The independent review** | What no tool judges: the use cases, the `[unchecked]` rules, the guidelines, the look | A fresh agent, before *Ready* |
| **The flow report** | Whether the three above still work, in aggregate | A scheduled job; the owner reads it |

The owner's places: before the build (concepts, the answers on the owner's list), at the result
(the running app, the review's summary), and weekly (the report). Not in the diff.

### 5.2 Where each kind of knowledge goes

This table goes into `AGENTS.md`, § "Where knowledge lives", and decides every future "where do I
write this":

| The knowledge | Its place | Test |
|---|---|---|
| True of every change, needed before any work | root `AGENTS.md` | Would a wrong change follow if a worker had not read it? Then here. It holds the map, the rules and the loop, and no step-by-step procedure. |
| True of one directory | that directory's `AGENTS.md` | Loaded when a file there is read |
| Anything a tool can check | a gate, a lint, a test — the rule names it | Can a tool check it? Then a tool does, and no sentence alone |
| How to do a kind of task, step by step | a **skill**, `.agents/skills/<name>/SKILL.md`, listed in `AGENTS.md` by name and path | Needed only when that task comes up |
| The current rule of a concern | its subject document, by numbered section | — |
| Why a decision was taken | an ADR | Only if a later worker, seeing the rule alone, would be tempted to undo it — or it is hard to reverse (data model, protocol, contract, licence, dependency) — or it is on the owner's list |
| A mistake that passed every gate | `known-traps.md`, until a gate holds it | — |
| What the owner decided once and wants always | the place above that fits; **never only a private note** | — |
| A machine's own facts (a slow Docker, an access path) | private notes | No other worker needs it |

**What changes.**

* The six private rules of §3.7 move: the concept ones into `docs/backlog/README.md` ("A concept
  built whole"), the two about the look into `docs/architecture/user-experience.md`, "one worker
  at a time" and "report only what is checked" into `AGENTS.md`'s working rules.
* `AGENTS.md` keeps the map, the fifteen rules, the tagged working rules, the loop, the owner's
  list and the command table. The how-to prose leaves it for skills. `make gate-docs` keeps
  holding every tagged rule, because the tagged rules do not move.
* Eight skills, each a page: `design-brief`, `build-concept`, `review-change`, `finish-pull-request`,
  `ask-the-owner`, `resume-work`, `triage-contribution`, `tidy`. A skill is plain Markdown at a
  known path, so a tool that knows nothing of skills reads it as a document.
* **Which tool reads what** (each tool's documentation, read 2026-11-21). `AGENTS.md` is read by
  every tool checked and is stewarded under the Linux Foundation. The skill format is an open
  specification; it does not fix a directory. `.agents/skills/` is read by Codex, Cursor, GitHub
  Copilot, Gemini CLI, opencode and Amp — not by Claude Code, which reads `.claude/skills/` only.
  So the skills live once, in `.agents/skills/`, and each `.claude/skills/<name>` is a link to its
  folder there, which Claude Code documents as supported. On a Windows checkout without symbolic
  links the link arrives as a text file; the worker then still finds the skill through the path
  `AGENTS.md` lists. A test in `test/architecture` holds that every skill has its link.
* **The reviewer has no shared format**: each tool defines a subagent in its own directory and
  syntax, and so does each tool's hook. The portable part is therefore the skill `review-change`,
  which holds everything the review does. `.claude/agents/reviewer.md` is a wrapper of a few lines
  — fresh context, tools for reading and for running tests, "follow `review-change`" — read by
  Claude Code and Cursor. It changes no file.
  With another tool, the review is a new session that is given the skill. The existing hook stays
  a convenience for one tool; what holds every tool is CI, as today.
* `docs/adr/README.md` takes the ADR test of the table. Process tuning is recorded where the rule
  lives, with its date; it needs no ADR.

### 5.3 Two lanes, and the design brief

**The task lane** — a fix, a small feature, a contribution — runs as today: draft, `make check`,
the independent review, *Ready*, CI, the owner's word.

**The concept lane** keeps "one branch, one pull request, the owner's word at the end" and gains
what the second work-surface build did by hand:

1. **Inventory first**: every view, menu and dialog in scope, each mapped to a mockup scene or a
   rule; "before" captures from `main`.
2. **Foundation, then shared server capabilities, then families of screens**, each family whole.
3. **A checkpoint after every family**: `make check`, the family's walks, the independent review
   of the family, then CI on the branch (below). A family is closed only when its checkpoint is
   green. Findings are fixed before the next family starts.
4. **A coherence pass** by one reviewer over the whole inventory, then the presentation.

*CI at a checkpoint without leaving draft:* `ci.yml` gains a manual trigger (`workflow_dispatch`).
The worker pushes the branch and starts one run on it; it runs the jobs `main` runs, on the
branch's head, and gates nothing: its summary job is named `CI by hand`, because a green
`CI required` on the branch's head would be the draft's. It is not a ready pull request's run:
`ci-cd.md` §3.4 lists every job that differs. GitHub starts it once `ci.yml` exists on `main` and
uses the branch's version of the file; the first one ran on this concept's branch on 2026-11-21.
A draft pull request still starts no run by itself. The
repository is public and its CI minutes are free (ADR-0089's measurement), so the cost is the
wait, which the worker spends on the next family. `test/architecture` holds the trigger, the push
to `main` alone, the summary's three names, that no job reads the event beyond "is this a pull
request" or reads the branch, and what `api-compat` and `numbers` compare with on a manual run
(`ci-cd.md` §3.4).

**The design brief.** Before a *structural* change, the worker writes one screen into the draft
pull request and builds only after it stands. Structural means any of: a migration that changes an
existing table; a new or changed port; the sync protocol or a merge rule; authorisation; a new
door; a contract change to an existing operation; work across two or more bounded contexts; any
concept. The brief is the Definition of Ready answered, nothing new to learn:

1. What changes for a person — the use case checks.
2. The impact map: for each point of `engineering-guidelines.md` §2, "touched: how" or "not
   touched: why".
3. The precedent: the existing code that does the nearest thing, by path.
4. One alternative, and why not.
5. What is unknown, and how each unknown gets resolved before it is built on.
6. The order of the build.

How it stays cheap: a read-only research agent gathers the map in its own context and returns a
page, so the builder's context is not spent on reading; the brief is then challenged **once** by a
fresh agent asked only "what makes this fail?". The owner sees a brief only when it touches the
owner's list. The independent review checks, from the finished diff, that a change in the class
had a brief; a missing one is a finding.

### 5.4 Proof that proves

**Rules for a walk**, in a new `apps/webapp/e2e/AGENTS.md` (loaded whenever a worker reads a file
there), each tagged like every other rule:

| Rule | Held by |
|---|---|
| A suite's header names the use case checks it proves; a suite that proves none says what it holds | gate: the suites without one, 56 today, are a list that only shrinks; a suite that is not listed has one |
| No fixed wait; wait for what the person would see | gate: the counts per file of `waitForTimeout`, 355 today in all, and of a timer (`setTimeout`), 37 today in 25 files, only fall; a file that is not listed has none |
| No answer the contract does not allow | gate: recorded stub answers validated against `api/openapi.yaml` (below) |
| An answer more than one suite needs lives in `fixture.mjs` | reviewed; the count of route stubs in suites, 382 today, is reported weekly |
| Find by role and name, as a person would; a `data-` hook only where no role exists | reviewed |
| A write is asserted twice: the request that left, and what the screen shows after | reviewed |
| The widths and the keyboard path the check names | reviewed |
| A check that crosses the server is proven by the walk **and** by a test of the same operation against the real server (integration or contract) | reviewed against `checked_by` |
| Red first: the walk fails on the build without the change | the independent review runs it |

The rules stand in `apps/webapp/e2e/AGENTS.md` as stage 1 worded them, derived from
`docs/usecases/README.md`, `docs/usecases/AGENTS.md` and `docs/evidence/README.md`, which state
them only in passing. The owner accepted them with D-4 and has not read the wording (§8).

**Stubs held to the contract, without a new dependency.** The walks' shared entry point
(`signedIn` in `fixture.mjs`, which already routes `**/api/v1/**`) writes every answer it fulfils —
method, path, status, body — to a file. A Go test beside `test/contract`, which already validates
responses against the contract, reads that file and fails on an answer the contract does not allow.
Suites that stub through their own `context.route` are wrapped by the same recorder. Built in
stage 2, the recorder patches Playwright's own `route` instead of `signedIn`, because 38 suites
launch a browser of their own and never pass through it. One run of every suite recorded
10,052 distinct answers in 209 suites; the other three suites leave no record, two because they
test a helper without a browser and one because `make e2e` does not walk it. A wrong answer is
counted once by kind, operation and status, not once per suite: 207 leave the contract — 190
bodies outside their schema, 15 statuses the operation does not declare, one media type the
response does not declare and one operation the contract does not have. They enter as a baseline that only shrinks (`test/contract/testdata/walks-baseline.txt`;
`make gate-pr` refuses an added line), so the gate is green on the day it lands and can only get
stricter. Two things were learned by running it. Counted per suite, and with the guess below, the wrong answers were 2,694,
and 13 of them differed between two runs on one machine, because which reads a walk still gets
answered before it ends depends on timing; a baseline per suite would have failed by chance. And
the answers that differ are the fixture's guess — one empty page for any request nothing
stubbed — so the guess is recorded and counted, 639 of the answers, and not judged. Nor are the
bodies of the 212 answers in `application/x-ndjson`: the contract declares that body a string and
gives its lines no schema. The key is coarse on purpose, and that has a price, named in §8.

**Red first, as a tool, is not built** (§9). `make proof BASE=origin/main` would take the tests
the branch adds or changes, run them against the product code of the merge base, and list each
that passes there — a test that passes without the change proves nothing about it. It would be
a report for the review, not a gate: a pure refactoring legitimately adds tests that pass before
and after. Until it exists the reviewer runs a test cited as proof against the code without the
change, in a worktree of its own (`review-change`).

### 5.5 The independent review

Before `gh pr ready` — and at every checkpoint of a concept — the builder starts the reviewer: a
subagent with a fresh context and tools for reading and for running tests — it changes no file —
given the diff, the description, the use cases
it names and the `AGENTS.md` of the touched directories. It never sees the builder's conversation.
The review is added to the author's own check against the rules no gate checks
(`engineering-guidelines.md` §3.19), which stays. The reviewer works through, in this order:

1. `docs/usecases/README.md`, "Checking work against its use cases": each check met, not met or
   not proven; work beyond *Where it ends*; principles broken.
2. Every rule tagged `[partial]`, `[unchecked]` or `[owner]` that the diff can touch.
3. The test rules: the levels of `engineering-guidelines.md` §1, the walk rules, red first.
4. Whether a structural change had its brief, and whether the build followed it.
5. For a screen with a mockup: the rendered mockup beside the running screen, region by region.
6. The text: every sentence that the diff makes false — in `docs/`, an `AGENTS.md`, a skill, a
   root Markdown file, a help comment of the `Makefile`, a workflow, `scripts/` or a comment in
   the code; every removed paragraph — false, or said elsewhere?; new text against the writing rules
   (§5.9).
7. What was left behind and what was taken away: code the change orphaned; every deleted, skipped
   or weakened test, with the reason the description gives (§5.10).

It reports a table — finding, where, severity, evidence — and nothing else. The builder fixes each
finding or answers it with a reason; the reviewer reads the answers once. The pull request's
description gains a section **Review**: who reviewed (model and date), findings found, fixed, and
left open with their reasons. `make gate-pr` holds the section to its form.

This is what the owner reads instead of the diff: the *Use cases* table, the *Premises*, and the
*Review* summary. It is not a workflow, sets no status and merges nothing, so `ci-cd.md` §5 and
§5.1 stand as written. For a contribution from a fork, the owner's worker runs the same review
locally on the fetched branch, since CI cannot give a fork's run a model.

**Relevant in practice.** A model reviewer reports what is wrong in theory as readily as what a
person meets, and a builder answers both with code. In this concept's close, reviewers raised
cases that occur nowhere in the repository, and a builder took a walk that failed now and then
for a timing fault of the walk, when the product was wrong. So a finding names how it is met in
practice or is marked theoretical (`review-change`), and nothing is built against a problem
nobody has shown (`AGENTS.md`, "Style"). It is the catching tests' question of §4 — was this
change intended — turned on the review: does this occur.

Limits, stated plainly: a reviewer is a model and misses things; it reads the diff, not the whole
product; its section proves that a review ran, not that it was good. The flow report (§5.6) is
what shows whether it works — fixes that follow features within a week should fall.

### 5.6 The flow report

A scheduled job, weekly, in `nightly.yml`'s manner (`flow.yml`: it comments on one issue, "Flow
report", files a `finding` per breach and starts no work): `flowstats` for the last seven days
beside the seven before, without the transcript part. It prints every row below; a row without a
source prints "not measured" and why:

| Measure | Threshold proposed (the owner sets it) |
|---|---|
| Pull request run, median wall minutes | ≤ 20 |
| Pull request runs failed | ≤ 15 % |
| `main` runs needing a second attempt | ≤ 10 % |
| Jobs by failures and by median minutes, the ten worst | reported |
| `fix` pull requests merged within seven days of a `feat` touching the same package | reported; a rise two weeks running is a breach |
| Hours `main` was red | ≤ 2 per week |
| Open `finding` and `decision` issues, and the oldest of each | reported |
| The falling bounds: uncited checks of built use cases, fixed waits, suites without a use case, stub answers outside the contract | none rises |
| Use cases by state | reported |
| Lines added and removed under `docs/` outside `archive/` | reported |
| Lines of the root `AGENTS.md` | reported |
| Comment blocks of twenty lines or more (47 today) | does not rise |
| Tests deleted or skipped in merged pull requests | reported, each with the reason its description gave |
| Unreachable Go functions; unused files, exports and types of the TypeScript workspace; unused translation keys (§5.10) | reported; a bound after the first removal |
| CodeRabbit: comments made, and the share that led to a change (§5.11) | reported |
| Open code scanning, secret scanning and Dependabot alerts, and the oldest of each (§5.12) | reported |
| The OpenSSF Best Practices badge's level (§5.13) | at least passing |

A breached threshold opens a `finding` naming the measure and the week, one per measure. The
thresholds live in `ci-cd.md` §11, which also says how each measure is read; the tool reads them
from `tools/flowstats/thresholds.json`, and a test holds the two equal. Changing one is the
owner's intervention "at the policy level". The report also counts the walks' timer waits.

Some rows are not measured, and each says why (`ci-cd.md` §11):

* unused translation keys, in every run: no tool in the repository counts them;
* the share of CodeRabbit's comments that led to a change, in every run: nothing records it. Its
  comments are counted once its app comments;
* unused TypeScript code, in the weekly job: the job does not install the workspace, so Knip's
  count is a local run's;
* secret scanning and Dependabot alerts, where the API refuses the job's token: no workflow
  permission covers them, and what the API answers is seen on the job's first run;
* the Best Practices badge, until its project is registered and its id stands in
  `thresholds.json`.

`deadcode` and Knip are pinned since D-11: on 2026-11-21 they list
175 unreachable Go functions and 409 unused files, exports and types of the TypeScript workspace,
neither list checked yet. "Uncited checks of built use cases" is the bound `make gate-docs`
holds, not a count of its own. The run on 2026-11-21, for 14 to 20 November, shows three
measures over the proposed thresholds: the failed pull request runs (53 % of 19), `main`'s
second attempts (83 % of 6) and the hours `main` was red (111.9, with 6 runs on `main`). The pull
request median is 15.9 minutes and within its threshold: it is the last attempt's time, and the
first reading of 38.5 minutes held the wait before a run was started again.
The session figures (waiting, tokens) stay a local command, since only the owner's machine has the
transcripts. With this, the dated reviews of ADR-0089 and ADR-0090 are replaced by something that
runs whether or not anybody remembers. Issue 1309, the dated review of ADR-0083's merge flow
(§3.2), is not the report's issue; a worker closes it when the first report stands.

### 5.7 CI

**5.7.1 The jobs, measured.** Every job attempt of 50 finished `ci.yml` runs from 2026-11-07 to
11-21 (pull requests and `main`; re-runs counted as attempts). Jobs under five minutes with no
failure are left out.

| Job | Attempts | Failed | Median min | p90 min |
|---|---|---|---|---|
| The client runs in Chromium, before it was split | 52 | 29 | 35.9 | 51.1 |
| The client runs in Chromium, shard 1 / 2 / 3 | 29 / 29 / 30 | 9 / 8 / 13 | 17.2 / 11.4 / 12.6 | 20.2 / 11.8 / 14.1 |
| The gates catch their own violations | 57 | 0 | 17.4 | 21.8 |
| Shell (Android) | 79 | 6 | 11.4 | 12.7 |
| Integration (PostgreSQL) | 62 | 13 | 10.9 | 12.8 |
| Unit and domain tests | 62 | 2 | 7.6 | 11.4 |
| Format, lint, generation | 63 | 0 | 6.5 | 8.8 |
| Shell (Windows) / Shell (Linux) | 80 / 79 | 4 / 6 | 4.8 / 4.3 | 5.8 / 5.1 |
| Workspace (design-system) | 80 | 8 | 0.7 | 0.8 |
| Dependency review | 65 | 8 | 0.2 | 0.2 |

What it says:

* **The walks are the wall clock and the flake.** Unsplit, the browser job took a median 36
  minutes and failed in 29 of 52 attempts; split in three it still fails in 30 of 88 shard
  attempts, and its slowest shard and `selftest` now set the run's length at about 17 minutes.
  Stage 3 read the logs of every failed attempt of the browser job from 2026-11-07 to 11-21: 83
  attempts, 39 before the split and 44 shards, of which 80 name the walks that failed.
  - `mail` failed in 25 of them, always the walk that connects a mailbox through its provider.
    It was a defect in the app: the mail page read its mailboxes and completed the connection
    side by side, and a read answered first and applied last took the new mailbox off the list.
    Fixed, with the walk giving that order every time.
  - `whiteboardframes` failed in 11, on a frame time; it left the job on 2026-11-16 (`ci-cd.md`
    §3.1, "Performance is measured, not gated").
  - `goalswalk`, `servicedeskwalk` and `timewalk` failed in 7, 6 and 6, mostly together: their
    record of errors (`productwalk.mjs`) held requests the browser had cancelled
    (`net::ERR_ABORTED` on a lazily loaded file, `/api/v1/stream` or a sync request). Three of
    those runs were read a second time, both attempts of each: 2026-11-09, 11-16 and 11-19. Each
    was a pull request of Dependabot's that raises Playwright from 1.64.1 to 1.65, and the three
    walks failed in both attempts — so there it is no flake. On 2026-11-22 Dependabot's pull
    request 1876, which raises Playwright to 1.65.1, failed the same way in the second attempt
    of run 45015522979, and there `workspacetemplates` failed too. Reproduced on a worker's
    machine: with 1.64.1 Chromium reports nothing for a request a page leaves under way; with
    1.65.1 it reports `net::ERR_ABORTED` between the next document's request and its commit.
    Fixed: the record (`scalewalk-errors.mjs`) leaves out exactly that, and keeps every failure
    of a document that stays.
  - `voice` and `productsband` failed in 5 each, on 2026-11-07 only; `instanceworkspaces` in 5,
    fixed on `main` on 2026-11-21. Twenty-one more suites failed in one to four attempts.
    `voice`, `productsband`, `celebrate`, `shell` and those others were counted and are unread:
    nobody looked for their cause.
  - `kompassfit` never failed in CI. Locally it failed under load: its walk left a hub before
    the hub's screen had noted the visit. Fixed in the walk.

  No failure traced to its cause so far was a fixed wait running out, so the counts of fixed
  waits stand where stage 2 put them, and the rule of §5.4 lowers them as suites are touched.
* **Integration failed in 16 of its attempts** from 2026-11-07 to 11-21, by stage 3's count; the
  table above counts 13, in its 50 runs. Which runs the 16 were counted over is not written
  down, and the two counts were not held against each other. Six were the cross-tenant tests under PostgreSQL 16.16, fixed on `main` on
  2026-11-16. All five after that fix, on 2026-11-20, were `go test`'s ten-minute default
  cutting a package that runs seven to ten minutes; `make gate-integration` has had
  `-timeout=30m` since 2026-11-21, and the nine attempts since then passed. That accounts for
  eleven; this document holds no cause for the other five.
* **The installed apps' jobs are not the problem**: 4 to 11 minutes, beside longer jobs. Nothing
  needs to leave the pull request path, and this concept moves no job.
* **`selftest` runs 17 minutes without ever failing**, on `main` and when a gate changes. It is the
  price of knowing the gates bite; it stays.
* **`make e2e SUITES=…` stays the local rule**: a worker walks the suites it touched; the whole
  set is CI's, as the Makefile already reads.

### 5.8 Contributors

Hubtask has no outside contributor yet, so the aim is to remove what would stop the first one, not
to build a community programme.

* **`CONTRIBUTING.md` gains four short paragraphs:** what is welcome without asking (bug fixes,
  translations, documentation, tests) and what starts as an issue (features, anything on the
  owner's list); what happens to a pull request (the owner's worker triages it, runs the review,
  the owner merges; no promised time, said honestly); that work done with an AI tool is welcome on
  the same terms — the contributor answers for it, and it passes the same gates; that a maintainer
  completes a description the description gate refuses, so a newcomer is never stopped by the
  template.
* **A labelled entry point:** at each cut, findings that are bounded and need no decision get
  `good first issue`. The labels exist and are unused.
* **One command to a running app:** `make dev` wraps the four steps of "Signing in locally"
  (application role, server as that role, a workspace, the web app) and prints the address. The
  steps exist as text and scripts today.
* **Governance in five lines** in `CONTRIBUTING.md`: one maintainer; decisions are `decision`
  issues, visible to all; merges on the owner's word; how that changes when a second maintainer
  arrives (`CODEOWNERS` already draws the boundaries).
* **Fork pull requests** already work as designed: no secrets, first-time contributors approved
  before CI runs, the same required check. Nothing to change.

Not proposed: a contributor agreement (ADR-0080 decided against), a chat server, a development
container — each can follow the first real contributor's request.

### 5.9 Writing: as much as needed, as little as possible, for a person

The owner's ideas, sorted into four questions: how a text is written, how much a change adds, what
a change removes, and how that is kept true over time. The rules go into `docs/README.md` as one
section, "How to write" (about a screen); `AGENTS.md` carries one tagged line that names it.

**How a text is written.**

1. **Write for the reader of that place.** Each row of `docs/README.md` gains the reader and the
   question the reader brings: a use case — a person checking the product; a subject document — a
   worker about to change that concern; an ADR — someone tempted to undo the decision;
   `CONTRIBUTING.md` — a newcomer. A sentence that does not help that reader act is left out.
2. **A sentence stands alone.** A person who has read only this section understands it. A citation
   (`rule 10`, `UC-ID-12/4`, `§9`) supports a sentence and never replaces its content. No coined
   label, no abbreviation the glossary (`arc42.md` §12) does not hold, no arrow or symbol
   chain in place of a sentence.
3. **One word per thing**, the domain model's word.
4. **The present only.** A document says what holds now. What it was before is the ADR's and the
   archive's; what is planned is the backlog's. No "previously", "new", "now", "updated".
5. **Short sentences in full words** — not shorthand. Shortness comes from leaving things out, not
   from compressing what stays.

Rule 5 is why this concept sets **no length limit** on documents. A limit makes a model compress,
and compressed text is the unreadable kind the owner described. The one limit that stays is the
existing one: a comment of a few lines. The root `AGENTS.md` has no line limit either. What is
held instead can be checked by reading: it holds the map, the rules and the loop, and no
step-by-step procedure — a procedure is a skill. Its length is reported weekly (§5.6).

**How much a change adds.**

| The change | Documentation it owes |
|---|---|
| A fix | Nothing. The test says what is true; the commit says why. A document that was wrong is corrected. |
| New or changed behaviour | The use case's *Today*, state and `checked_by`; the subject document's section **if** a rule changed |
| A new rule or structure | The subject document's section; an ADR only by the test of §5.2 |
| A new mistake that passed the gates | One entry in `known-traps.md` |
| Anything | A new file only when no existing file has a section the text belongs in. In the description, a section that does not apply reads "n/a". |

**What a change removes.** A change that makes a sentence false rewrites or deletes that sentence
in the same pull request. It does not append a correcting paragraph, an "update" note or a
struck-through line. Three cases, three different permissions:

| The text is … | In the change that causes it | Safeguard |
|---|---|---|
| **False now** | Rewritten or deleted — owed | Found by searching every tracked file for every name the change renames or removes (operation, setting, table, message code, section number): `docs/`, every `AGENTS.md`, the skills, the root Markdown files, the `Makefile`'s help comments, the workflows, `scripts/` and the comments in the code. The reviewer runs the same search independently. |
| **True, and said elsewhere too** | May become a link | Only after reading that the other place really says it |
| **True and said only here, but looks unnecessary** | **Not touched in passing** | Belongs to a tidy pass (§5.10) |

What keeps deleting safe: the reasoning for a rule lives in its ADR, which is never edited;
numbered sections are never renumbered and `make gate-docs` already refuses a citation to a section
that is gone; the diff shows every removed line, so the reviewer answers one question per removed
paragraph — false, or said elsewhere? — and reports any it cannot answer; git keeps the rest.

**Kept true over time.** The reviewer's list (§5.5) gains: sentences the diff makes false; text
that breaks rules 1–5. The flow report (§5.6) gains lines added and removed under `docs/` outside
`archive/`, and the count of comment blocks of twenty lines or more (47 today) as a bound that
only falls.

### 5.10 Cleanup: what a change leaves behind, and what a tool can prove

Judged critically, cleanup is three different things with three different risks. Two are worth
having; one is left out.

| Tier | What | Risk | Verdict |
|---|---|---|---|
| 1 | **What the change itself orphans**: the function whose last caller it removed, the test of behaviour it removed, the flag, the translation key, the stub answer | Low — the worker knows why it is unused, and the compiler, the linter and the tests confirm | **A rule, in every change** |
| 2 | **What a tool proves unused** across the repository: unreachable functions, unused files, exports, dependencies | Medium — a tool is valid for one build configuration and can be wrong about code reached by other means | **A report first, removal in its own pull request, never mixed with a change in behaviour** |
| 3 | **What a model judges unnecessary**: "this test is no longer needed", "this abstraction is not worth it", sweeps for tidiness | High — a model cannot tell unused from not yet used or used by an operator; a deleted test is the cheapest way to make a red build green | **Left out.** Never on a worker's own initiative; a concept or a `finding` if the owner wants it |

**Tier 1, the rule** (`engineering-guidelines.md` §3, one item): a change removes what it makes
unused, in the same pull request, in its own commit.

**Tests get a stricter rule**, because removing one removes proof. A test is deleted or weakened
only when the behaviour it asserts was removed or changed on purpose, or when another test asserts
the same thing — and the description names each such test and which of the two reasons applies.
`make gate-pr` can hold the naming: a diff that removes a test function or a walk, or adds a skip,
needs the test's name in the description. A test that a use case cites in `checked_by` leaves only
together with the change to that use case: `make gate-docs` refuses a `checked_by` that names a
file that does not exist. It reads no test's name, so a test deleted from a file that stays is
held by `make gate-pr` alone.

**Tier 2, the report.** `deadcode` over the binaries, once per build configuration, and a count of
translation keys no client uses, enter the weekly flow report as numbers. Removal follows in a
pull request of its own per package, where each removed function is one a tool reported and the
description says so; what is kept on purpose goes on a short list with its reason. The count
becomes a falling bound only after the first such pull request shows how many reports were right.
For the TypeScript workspace the matching tool is Knip, a development dependency of the
workspace root since the owner accepted D-11 (§6); `make deadcode` and `make unused` print the
two lists.

**The tidy pass for documents** is the same idea: at a cut, one document per pull request, no
change to any rule. The reviewer compares the rules before and after — every numbered section
still there, every rule sentence kept, moved with a link, or listed in the description as removed
and why. It is how the third row of the table in §5.9 gets handled without risk in passing.

**Never cleaned up by anyone but the owner's decision:** a contract field, a merged migration, a
database column, anything in `archive/` — `AGENTS.md` already lists these under what a worker
does not decide.

### 5.11 CodeRabbit as the last reviewer

Facts from CodeRabbit's own pages, read 2026-11-21 through a summarising fetch (§10); the wording
is to be confirmed on the page before anything is relied on.

| Fact | What it means here |
|---|---|
| Reviews of public repositories are free, on a personal account too | It costs no money |
| On a public repository with fewer than ten stars a review is started by hand | Hubtask has none: every review starts with the comment `@coderabbitai review` |
| Free-tier limits per developer and hour: 1–10 reviews and 100–300 files per review, by the repository's popularity; the thresholds are not published | All pull requests come from one identity, so a busy hour can hit the limit. A concept of 1,385 files is skipped whole |
| A skipped or rate-limited review **passes** its check | Its check proves nothing and is not made required |
| It reads `AGENTS.md` at every level as review guidelines by default | The rules stay in the repository; no second rule set |
| It remembers replies as "learnings" in its own database | A rule taught by a reply would live only at a vendor — against §5.2. Workers do not teach it; a rule goes into the repository |
| It runs about sixty linters of its own, each can be switched off | The ones `make` already runs are switched off, so one tool speaks per rule |
| Its servers are in the United States; its promise not to train on code excludes open-source projects | The code is public already. The owner should know it is used for training |
| The app asks for write access to code, pull requests, checks, issues and workflows | More than a reviewer needs, and it cannot be narrowed. `AGENTS.md` wants a steering tool to have no right to merge; nothing found says CodeRabbit merges, and nothing technical stops an app with that access |
| A pull request from a fork is reviewed with the base branch's configuration | A contribution gets a model's review, which CI cannot give it |

**Does it make sense?** Yes, as advice, for three reasons: it is a second model from another
vendor, so it does not share the builder's or the reviewer's blind spots; it covers fork pull
requests; it costs nothing. And with three limits: it reads a diff, not Hubtask's use cases, so it
does not replace the independent review; it does not cover a whole concept; it is noise if left on
its defaults.

**How it is used.**

1. The order: `make check`, the independent review, then `@coderabbitai review`, then *Ready*.
   Whether it reviews a draft on request was not confirmed; if it does not, the request goes out
   with *Ready* and runs beside CI.
2. The worker reads its comments (each carries a block written for an agent), fixes what is
   right, and answers the rest in one line. The *Review* section reports it in one row: comments,
   fixed, declined.
3. It is never a required check and never requests changes. `ci-cd.md` §5 — AI comments, gates
   decide — holds unchanged.
4. In the concept lane it is asked at a checkpoint only where the family's diff is under the file
   limit; otherwise the independent review stands alone, and the description says so.

**The configuration**, one file `.coderabbit.yaml` in the repository, which takes precedence over
the settings on its website: profile `chill`; no poem, no sequence diagrams; drafts not reviewed
by itself; `dependabot[bot]` ignored; generated code, lock files and `docs/archive/` filtered out;
the linters that duplicate a gate off (`golangci-lint`, `gitleaks`, `actionlint`), and three that
no gate runs (`markdownlint`, `yamllint`, `shellcheck`), because a rule the repository wants
becomes a gate; learnings scoped to this repository.

**A trial, not a fixture.** Four weeks; the flow report counts its comments and how many led to a
change. Proposed bar, the owner's to set: kept if at least one comment in three led to a change;
otherwise uninstalled, which removes one file and one row.

**The owner's part:** installing the app on the account — it grants the access above, so a worker
does not do it.

### 5.12 What the repository shows

**The principle.** A figure is shown only if a machine produces it and nobody is asked to raise
it. A badge a worker can turn green by writing weaker tests is worse than no badge. This is the
owner on the loop again: the figures are for looking at in aggregate, not targets for a change.

**Shown, in one README section of about ten lines, "Quality and security":**

| Shown | Source | Why this one |
|---|---|---|
| CI on `main` | GitHub's own badge | The single required check, as it stands now |
| **The proof state**: use cases by state, checks cited by a test or a document that proves them, the bounds that only fall | printed by `make proof-state`, written to the summary of each run on `main` and carried by the weekly flow report, which the README links to | Hubtask's own coverage: what a person can do, and whether something proves it. It cannot be raised without a `checked_by` file that cites a check |
| OpenSSF Scorecard | a weekly workflow on `main` | An outside, automated view of the supply chain. Pinned actions, declared permissions, static analysis, the security policy, signed releases and fuzzing are already there to be counted |
| OpenSSF Best Practices, passing level, and Baseline level 1 | a questionnaire answered once with links | Recognised by people who assess a project before running it; nearly every answer exists as a document |
| How to verify a release: the signature, the bill of materials, the attestation | one command and a link to `security.md` §11, which gains the commands | The artefacts exist; how to verify them stood in no document |

The proof state was first built as a committed page, generated by `make generate`, on the
reasoning that each figure stands on its own line and two pull requests that change one conflict.
The review of stage 6 showed the case that reasoning missed: two pull requests that make the
identical edit merge cleanly and leave the page stale, pull requests merge without catching up
with `main` (ADR-0083), and `main` then turns red on `quick` and `docs`. So `ci-cd.md` §3.5 keeps
its rule without an exception - generated files keep no counts - and the proof state became a
printed table: no file, no staleness check, the same figures from the same code.

No Scorecard result exists yet (§7). The README says in one sentence why the figure cannot reach
the top: three of its checks need a second person, and Hubtask has one maintainer. The badge links to the per-check
result, which is the part worth reading.

**Coverage.** The thresholds stay a gate as they are: per package, on the two layers where the
rules live, proven by a planted violation. The percentage is visible in one place that costs
nothing, the unit job's summary, and gets no badge and no service. Two things planned here were
not built (§9): the percentage in the weekly flow report, and a reported figure for the clients'
code.

**Three gaps closed along the way:**

* CodeQL also reads JavaScript, TypeScript and the workflows. Whether it reads `.svelte` files was
  not confirmed.
* The flow report (§5.6) gains open security alerts by kind and the age of the oldest.
* The three open Dependabot alerts — `qs`, `glob` and `minimatch` — are lifted to their patched
  versions by `overrides` in `pnpm-workspace.yaml` on this branch. GitHub closes the alerts when
  the branch merges; until then they are open.

**Whether the tests bite** is the honest question behind "coverage". The answer is mutation
testing, and it joins the experiments of §7: nightly, on `core/domain` only, where the tool's
limit on size does not hurt, with StrykerJS on one package of the workspace. Kept if what it
reports leads to better tests.

**Left out:** a coverage service, a row of badges for things `make` already holds, Baseline level
2 and the higher badge levels (they need a second maintainer), the REUSE badge (the licence
headers exist; the badge adds a registration and a directory for little).

**Under the Cyber Resilience Act the project is no manufacturer while it monetises nothing.**
Hubtask is open source and funded by donations only, which buy nothing in return
([licensing-editions.md](../../architecture/licensing-editions.md) §5,
[`.github/FUNDING.yml`](../../../.github/FUNDING.yml)). There is one edition and no paid one,
and §4 there says why ([ADR-0080](../../adr/ADR-0080-hubtask-is-apache-2-0.md)). The project's
position on the Act is [data-protection.md](../../architecture/data-protection.md) §7, and how it
reports is [security.md](../../architecture/security.md) §14.3; a provider who sells Hubtask or a
distribution of it is a manufacturer itself. ADR-0080 says the assessment is
made again before a paid offering appears. This is not legal advice.

### 5.13 Nothing shown needs the owner's hand

The owner is on the loop, so a signal that needs a person to register, fill in or renew is not
adopted in that form. Three rules:

1. **A worker does the whole thing**: prepares every answer with its link, registers the project,
   submits, and puts the badge into the README — through the browser, as any other
   administration (`AGENTS.md`, "Working with the owner").
2. **One act cannot be taken from the owner**: consent that lets a third party act under the
   owner's GitHub identity — signing in to the badge site with GitHub, installing the CodeRabbit
   app. A worker brings the page to the consent button and stops; the owner's part is that click,
   once per service, and all such clicks are collected into one list at the end of the build.
3. **Upkeep is a machine's**: the weekly flow job reads the Best Practices badge's level from
   the badge site's public JSON and opens a `finding` when it is below `passing`; a worker
   repairs it. The row reads "not measured" until the project is registered (§5.6). The job does
   not read single criteria, and it does not read the Scorecard's result: that one is uploaded
   as code scanning alerts, which the report counts.

If the consent click is one too many, the questionnaire badge is dropped and Scorecard, which
needs no account at all, stands alone. That is the fallback, not the plan.

---

## 6. What the owner decides

Each is on the owner's list or amends a decision the owner took. One ADR records them together
(the next free number), and each rule lands in its subject document.

| # | Decision | Amends |
|---|---|---|
| D-1 | An independent review by a fresh agent before every pull request leaves draft, reported in a *Review* section | `engineering-guidelines.md` §3.19; ADR-0089 item 4 in part |
| D-2 | A design brief for the structural class of §5.3, challenged once | ADR-0089, which rejected a record "by risk class" — here the class is the kind of change, and the bar stays one |
| D-3 | CI can be started by hand on a branch, for a concept's checkpoints | ADR-0079 ("a draft runs nothing" stays true of pushes) |
| D-4 | The walk rules of §5.4 and their falling bounds | new |
| D-5 | The flow report and its thresholds | replaces the dated reviews of ADR-0089, ADR-0090 |
| D-6 | Nothing: §5.7.1 shows no job that should leave the pull request path. The number is kept so the others do not move | — |
| D-7 | The contributor paragraphs of §5.8, the one on AI-assisted work among them | `CONTRIBUTING.md` |
| D-8 | Skills and the reviewer's definition under `.claude/`, in the open skill format, listed in `AGENTS.md` | `AGENTS.md`: "any machine and any tool" — kept, because the files are plain Markdown at named paths |

D-1 to D-8 were **accepted on 2026-11-21**. The owner's later questions added these, **accepted the same day**:

| # | Decision | Amends |
|---|---|---|
| D-8a | The skills live in `.agents/skills/`, linked from `.claude/skills/`; the reviewer's instructions are a skill, its `.claude/agents/` file a wrapper (§5.2) | D-8, which named `.claude/` only |
| D-9 | The five writing rules, what a change owes and what it removes; no length limit on documents (§5.9) | `docs/README.md`; `AGENTS.md`, one tagged line |
| D-10 | Cleanup in tiers: a change removes what it orphans; removal on a tool's report in its own pull request; none by judgement. A deleted or weakened test is named in the description, held by `make gate-pr` (§5.10) | `engineering-guidelines.md` §3; the pull request template |
| D-11 | Two tools for the report: the Go team's `deadcode`, run at a pinned version, and Knip as a development dependency of the workspace. Either can be declined; the Go half then has no count, the TypeScript half none either | The owner's list: a new third-party dependency |
| D-12 | CodeRabbit as an advisory last reviewer for a four-week trial, configured as in §5.11; the owner installs the app | `ci-cd.md` §5, unchanged in substance; the owner's list: a third party with write access |

From the owner's question about what the repository shows, **accepted on 2026-11-21**, D-15 in the form below after the owner refused a step that needed the owner to register and submit:

| # | Decision | Amends |
|---|---|---|
| D-13 | The README section of §5.12 with the CI and Scorecard badges, the proof state - printed and reported weekly, not a committed page (§5.12) - and the release verification; the Scorecard workflow, which adds one action to the allow-list | `README.md`; `ci-cd.md`; the owner's list: a new third-party action |
| D-14 | Coverage stays a gate at today's thresholds and is reported, not badged; no coverage service; CodeQL extended to the clients' languages and the workflows; security alerts in the flow report | `engineering-guidelines.md` §1; `security.md` §13 |
| D-15 | The Best Practices badge at passing level and Baseline level 1, done by a worker from start to end (§5.13); nothing shown may need the owner's upkeep | new |

---

## 7. How it is built

**One branch, one pull request, one orchestrating session** — the owner's word of 2026-11-21, and
concept mode as `AGENTS.md` defines it. The branch is `concept/development-flow`; this document is
its concept and lands with it. Most of it is rules, tools and tests; the only product code touched
is the walks and their fixture (stages 2 and 3).

**How the session runs.** The orchestrator builds nothing itself. It gives each stage to one fresh
worker — one at a time — with this document's sections for the stage and its "done when". When the
worker reports, the orchestrator runs the stage's checks itself and starts the reviewer on the
stage's diff; a stage is closed only when both are clean. It hands over (pushes, names the next
stage on the draft) before its own context passes the limit `AGENTS.md` sets.

| Stage | Content | Done when — checked by the orchestrator, not taken from the worker's report |
|---|---|---|
| 1 | **The rules.** The ADR; `AGENTS.md` (the placement table, the moved rules, procedures out, the lines for §5.9 and §5.10); "How to write" in `docs/README.md`; the eight skills in `.agents/skills/` with their links; `reviewer.md`; `apps/webapp/e2e/AGENTS.md`; the six private rules in their documents; `docs/adr/README.md`; `engineering-guidelines.md` §3; `CONTRIBUTING.md`; the template's *Review* section; `tools/checkpr` holding that section and the naming of a deleted test; `.coderabbit.yaml` if D-12 | `make gate-docs` green; `make gate-pr` green on a description with the section and red without; `AGENTS.md` holds no step-by-step procedure; a planted violation in `gate-selftest` for the missing *Review* section and for an unnamed deleted test; every skill has its link |
| 2 | **The walks' gates.** The counts as falling bounds; the recorder in `fixture.mjs`; the Go test that validates the record; a planted violation for each | The bounds equal today's counts; a planted fixed wait, a suite without a use case and an answer outside the contract each turn the gate red |
| 3 | **CI.** `workflow_dispatch` on `ci.yml`, with `test/architecture/drafts_test.go` updated; the failing browser shards read, and the suites that fail most fixed at their cause | `make gate-architecture` green; a draft's push still starts nothing (the test says so); the suites changed pass locally ten times in a row |
| 4 | **The flow report.** `flowstats` prints the measures of §5.6; the weekly job; the thresholds section in `ci-cd.md`; the reports of §5.10 if D-11 | A local run prints every row of §5.6; a threshold set below the current value produces the text of a `finding` in a test |
| 5 | **Contributors.** `make dev`; the label on the open findings that qualify | A fresh clone reaches a signed-in workspace with `make tools && make dev` |
| 6 | **What the repository shows.** The README section; the printed proof state; the Scorecard workflow; CodeQL's added languages; the coverage figure in the job summary; the answers for the Best Practices badge, each with its link, as a file the worker submits from at the close | `make proof-state` prints the table and its figures equal the use cases' states, `make generate` leaves no diff, and no committed file holds the counts; `make gate-architecture` green with the new action on the allow-list; the README section is under fifteen lines |
| Close | A coherence review by one fresh reviewer: this document's §5, item by item, against the branch — built, not built, or built differently. Then `make verify`, the description from the template, *Ready*, CI, the list of consent clicks (§5.13), CodeRabbit once installed, the owner's word | Every item of §5 is "built" or named in the description as left out and why; CI green |

**A target dropped while building.** Stage 1 first aimed at a root `AGENTS.md` under 200 lines.
The target was dropped on 2026-11-21: the tagged rule lists alone take about 166 lines, so it
could only be met by dropping rules, and no rule is dropped. The file's length is reported weekly
instead (§5.6).

**What the first manual run shows**, read off it by the orchestrator: that its `CI by hand`
leaves the pull request's required check alone, and that `api-compat` and `numbers` find their
base on a branch. The first run (2026-11-21, head `fafe6b1b2`) has 45 jobs: 41 pass, and two are
skipped by design, `Dependency review` and `Pull request description`, which need a pull request.
`The client runs in Chromium (1/3)` fails on one walk of `whiteboardwork`, so `CI by hand` fails.
The walk read the person on a task card before the person's name had arrived; it now waits for
the initials. `make gate-walks` passes in all three shards on the runner.

The second manual run (44937443059, 2026-11-21, head `7ee94f1e9`) failed before any job started.
The repository's setting of allowed actions lacks `ossf/scorecard-action@*`, and the job
`actions-allowed` names every action of every workflow, so GitHub refused the whole run. The
owner accepted the action with D-13; adding it to the setting is the owner's word (`ci-cd.md`
§4), and no run of `ci.yml` on this branch starts before it is there.

**What the merge showed** (2026-11-22, read by the orchestrator):

* The owner added `ossf/scorecard-action@*` to the allowed actions, and CI ran: green on the
  pull request and on `main`.
* The first Scorecard run and the first CodeQL analysis of JavaScript opened 37 code scanning
  alerts. Of CodeQL's 24, 17 were fixed in code and 7 dismissed with their reason. Of the
  Scorecard's 13, the five base images are pinned by digest; four installs that take no hash,
  the committed Gradle wrapper and the missing approving review are dismissed with their reason;
  three of nine advisories are closed by raising a dependency, and the six left are notices
  without a severity, named in `apps/webapp/src-tauri/osv-scanner.toml`; the badge's alert
  closes with the badge.
* The walk judge turned `main` red once, on an answer that depends on timing (§8).
* The weekly job ran once by hand: it wrote its report and opened a finding for each of three
  measures over their threshold.
* The Best Practices badge is at passing since 2026-11-22
  ([project 17000](https://www.bestpractices.dev/projects/17000)).

Not read yet: whether the Scorecard takes the six notices' file and closes its alert, which its
next run on `main` shows; whether the pull request median falls in the next reports; whether a
worker closes issue 1309 after them.

**Later, as experiments:** `make proof` (§5.4), which is not built, then generated catching tests on the structural class,
run nightly for four weeks — kept only if at least half of what it reports is real, counted on its
issue. And mutation testing as §5.12 describes it; its tools are new dependencies and are asked
for when it starts.

After four weekly reports the owner looks once at the trend and keeps, adjusts or takes back each
decision. The whole change is reversible by reverting one pull request; none of it migrates data.

---

## 8. Why this holds, and where it can fail

* **Every new rule is either counted or named as uncounted.** The bounds only fall; each new gate
  has a planted violation in `gate-selftest`; what stays judgement is tagged `[unchecked]` and goes
  to the reviewer. That is the repository's own method, extended, not a new one.
* **Nothing is trusted because an agent said so.** The reviewer's section is checked for form by
  a gate and for effect by the report.
* **It can fail in four ways**, each visible:
  1. The review becomes a ritual — seen as fixes after features not falling.
  2. The brief grows back into the readiness record — bounded to one screen and one challenge; the
     report counts time from opened to *Ready*, which ADR-0089 measured before and after.
  3. Checkpoint runs are skipped under time pressure — the final run fails as today, no worse.
  4. The stub validation finds so much that the bound stays high — then it has found the size of
     gap §3.3 describes, and each cut lowers it.
* **A known limit: the walks' baseline is coarse.** A line excuses every wrong body of its
  operation and status, not one body. 179 of the contract's 907 operations have such a line, and
  6,280 of the 9,413 answers judged in the run of 2026-11-21 — two in three — are for an operation
  and status the baseline lists. A new wrong body there passes until the line is removed, so the
  gate holds new operations and the corrected ones, and the rest only as each line goes.
  `make gate-walks` prints both numbers on every run.
* **The walk judge once failed on an answer that depends on timing.** A stub that answers every
  request with an error also answers a background request of the page, a pull or a push, and
  whether one is sent in that moment differs from run to run. While 34 of the contract's 907
  operations declared no `4XX` answer, `sync:pull` and `sync:push` among them, a `401` there was outside
  the contract; it turned `main` red once (2026-11-22), and the walk in `settings` is narrowed.
  Now 903 operations declare `4XX`, so the `401` that `manifest.test.mjs` gives every request
  presenting the ended credential is within the contract wherever it lands. The four WOPI operations declare their
  protocol's own bodyless codes alone; the document server calls them, never the web app.
* **Risks the additions bring**, and what bounds each: a worker deletes a true sentence it took
  for false — the reviewer answers for each removed paragraph, and the ADR and git keep the
  reasoning; a tool reports live code as dead — removal is its own pull request and must pass
  every test; the writing rules produce shorthand — rule 5 forbids it and no length is gated;
  CodeRabbit's comments are followed blindly — each is fixed or answered, and the gates decide.
* **Baseline level 1 is not reached at the close.** The answers file reads 22 Met and 2 Unmet
  (`docs/evidence/best-practices-answers.md`). The two unmet rows name the same file, the
  Android build's committed Gradle wrapper. It stays, on the owner's word of 2026-11-22:
  `tauri android build` runs it, and Tauri's template writes it. Its SHA-256 is the one Gradle
  publishes for 9.6.1. Two rows rest on the owner: `main`'s protection binds
  administrators, and the account has a second factor (2026-11-22). D-15 names level 1; it is not
  reached while the wrapper is committed.
* **The owner has not read the walk rules' wording.** D-4 accepted the rules of §5.4; the
  sentences in `apps/webapp/e2e/AGENTS.md` are stage 1's.
* **Still unverified:** how many of the functions and files `deadcode` and Knip list are unused,
  and how many translation keys are; whether CodeRabbit reviews a draft on request, what its
  limits are for a repository without stars, and whether its free tier includes every feature
  named; the cause of the browser job's failures that §5.7.1 counts as unread; how long
  `make proof` would take for a walk (it needs a second build of the web app); the reviewer's
  cost in tokens per pull request; a Scorecard result for this repository.

---

## 9. Left out on purpose

* **Splitting the hot files** (`api/openapi.yaml`, `cmd/server/main.go`, `locales/en.json`,
  `fixture.mjs`): it would cut conflicts and what a worker must load, and it is a change to the
  generators and the wiring — a concept of its own.
* **Narrowing the product's surface** before a first release: the owner's vision, not a process
  question; §3.9 only names its cost.
* **A model in CI**, a merge queue, a required approving review, a second quality class.
* **`make proof`**, red first as a tool (§5.4): the reviewer runs a cited test against the code
  without the change by hand, and the tool is the first of the experiments of §7.
* **The coverage percentage in the weekly flow report**: the `unit` job writes it to its run's
  summary, and a job cannot read another run's summary back. Carrying it would need the figures
  as an artifact or a committed file, and `ci-cd.md` §3.5 keeps counts out of committed files.
* **A coverage figure for the clients' code**: no package of the workspace runs its tests with
  Node's coverage flag, which Node still marks experimental. Nothing reports it.
* **Walks against a real server as a second suite**: the contract-held stubs and the paired
  server test close most of the gap for a fraction of the run time; reconsidered if the report
  shows screens breaking on answers the contract allows.

---

## 10. Sources

* The repository at `77661b5a6`, the GitHub API, `make flowstats`, read on 2026-11-21.
* https://engineering.fb.com/2026/10/29/security/catching-bugs-before-they-land-jittestings-human-on-the-loop-approach-at-full-scale/
  and the paper it cites, https://arxiv.org/pdf/2601.22832 (the paper itself was not read).
* Documentation: https://google.github.io/styleguide/docguide/best_practices.html;
  https://arxiv.org/abs/2602.11988 (abstract only); https://diataxis.fr/;
  https://pkg.go.dev/golang.org/x/tools/cmd/deadcode; https://knip.dev/.
* Other tools' reading of `AGENTS.md`, skills, subagents and hooks: the documentation of Codex,
  Cursor, GitHub Copilot, Gemini CLI, opencode and Amp; https://agents.md;
  https://agentskills.io. Not verified: the skill specification's list of clients.
* Showing quality: https://github.com/ossf/scorecard (README, `docs/checks.md`) and
  `ossf/scorecard-action`; https://www.bestpractices.dev and its `docs/criteria.md`;
  https://baseline.openssf.org/versions/2026-08-28;
  https://testing.googleblog.com/2020/08/code-coverage-best-practices.html;
  https://go.dev/doc/build-cover; https://nodejs.org/api/test.html;
  https://github.com/go-gremlins/gremlins; https://github.com/stryker-mutator/stryker-js;
  Regulation (EU) 2024/2847 on EUR-Lex (Articles 13 and 24 in excerpts only). Not verified: what
  access a coverage service's app takes; CodeQL on `.svelte` files; a Scorecard result for this
  repository.
* CodeRabbit: https://docs.coderabbit.ai (plans, configuration reference, code guidelines,
  learnings, status check, review commands, GitHub platform page, FAQ);
  https://www.coderabbit.ai/pricing and `/privacy-policy`.
* https://code.claude.com/docs/en/memory.md, `/skills.md`, `/sub-agents.md`, `/hooks.md`,
  `/github-actions.md`, `/permission-modes.md`; https://agents.md; https://agentskills.io.
