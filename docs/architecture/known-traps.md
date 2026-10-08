# Known traps

Mistakes that pass every gate once and are found later — by a walk, a real database, a second run.
Each is a class, not an incident: it happens again wherever the same shape is built. A readiness
record names the ones that apply to its task (section 2(d) of `docs/backlog/ready/TEMPLATE.md`).

A trap belongs here only while nothing better holds it. The day a gate, a test or a comment where it
bites can hold it, it moves there and leaves this list. So every entry says why no gate holds it; a
reason that no longer holds is the cue to build the gate. A new trap is added in the pull request
that fixes its first instance.

## Transactions and wiring

| Trap | Where it bites | How to avoid it | Why no gate |
|---|---|---|---|
| **A write outside `UnitOfWork.Within`** passes every service test — the fake runs any callback — and answers 500 (`postgres.no_transaction_in_context`) against PostgreSQL. | audit entries, repository calls added after the main transaction | an integration test that drives the path against the database | the fake unit of work cannot tell inside from outside, and a static check cannot see which call runs in a transaction |
| **A write on a refusal path is rolled back with the refusal.** A failure counter, a lockout or an audit entry written inside the transaction that then returns the error is never stored. | sign-in, second factor, step-up, any "count the failure and refuse" | write it in its own transaction, detached from the request's cancellation | whether a write must outlive the refusal is the design's intent, not a pattern a tool can find |
| **A nested scope may not switch tenant.** `InstallationScope()` inside a tenant transaction is refused (`postgres.tenant_switch_in_transaction`); a swallowed error looks like a policy decision. | helpers that open their own scope | use the ambient context inside somebody else's transaction; a table without row level security and a `SECURITY DEFINER` function read under any scope; log an error before swallowing it on purpose | the refusal already happens at run time; swallowing it is ordinary Go |
| **Writer copies miss a late field.** `SessionWriter` is a value; copies taken in `cmd/server/main.go` before a field is assigned never learn it. | any field set on a writer after others copied it | set it in the literal; a wiring test in `cmd/server` | copying a value before a field is set is ordinary Go; only a wiring test sees the result |
| **A port can be written and never called.** It compiles and is documented; nothing calls it. | ports added "for later" | a test that drives the use case, not the port | its implementations use the method, so a dead-code check sees it as used |
| **A read-then-write without a lock** loses an update when two requests interleave. | switches, counters, removals guarded by a count | check in the statement that writes (conditional `UPDATE`/`DELETE`), or lock the row | a race lives between two requests; a static check sees one |

## Contract, registry and adapters

| Trap | Where it bites | How to avoid it | Why no gate |
|---|---|---|---|
| **`Handler.Invoke` skips validation.** A test that calls the handler directly never meets the registry's input validation, so refusal codes the registry answers first look reachable. | refusal tests | go through `registry.Invoke` or the REST route | unit tests call handlers directly on purpose; only a refusal test is wrong to |
| **The registry refuses an undeclared input key** (`usecase.field_unknown`): a route that looks implemented answers 400. | REST controllers building the input map | a contract test through the router | the input map is built at run time from the request |
| **An explicit JSON `null` reads as absent** (`Input.Present`). | adapters decoding nullable fields | send `""` where "cleared" must be told from "omitted" | what `null` means is decided per field, in the contract |
| **A manifest declares what it does not answer** — a capability or field published before anything fills it. | `/meta/capabilities` | check the answer against what the feature does, not the schema | whether a feature fills a field is behaviour, not schema |
| **A capability test can ask itself.** Two code paths running the same query agree and prove nothing. | capability and availability tests | test against what the feature needs | a test's independence from the code under test is judgement |
| **A search must resolve what it parses** — a placeholder like `@me` resolved in one read path and not the other. | anything parsed once and read twice | one resolution step both paths call | only a reader sees two read paths as one feature |
| **A public route still verifies a presented bearer**, and answers 401 to a stale one. | clients calling public routes with a stored token | send no token where none is needed | the server is right to; the trap is in a client |

## Database and migrations

| Trap | Where it bites | How to avoid it | Why no gate |
|---|---|---|---|
| **A superuser hides migration defects.** Test environments migrate as one; a migration that needs one fails on an operator's database. | `CREATE EXTENSION`, `CREATE ROLE`, policies | run it as the migrator role; document what an operator grants | every test environment migrates as a superuser; none runs as the migrator role yet |
| **`SECURITY DEFINER` functions bypass row level security** as their owner. | functions reading across workspaces | answer only what the caller may know (a number, never a list); guard the scope inside | what a function may reveal is a judgement per function |
| **A row mapper that drops `tenant_id`** reads fine and breaks whatever rebuilds the aggregate from the row. | repository mappers | select the tenant; a round-trip test | the row reads fine; only a round trip through the aggregate shows the gap |
| **Migration and ADR numbers taken by unmerged branches collide.** A migration collision turns several container jobs red at once, and only the Go jobs print `goose: duplicate version`; two ADRs collide on the index rows of `docs/adr/README.md` too. A conflicting (`DIRTY`) pull request runs no workflow and shows "no checks" rather than red. | new migrations, new ADRs | take the number from all remote branches right before writing the file, and again before leaving draft; read `gh pr view <n> --json mergeStateStatus` before reading an empty check list; on a collision `main` keeps the number — the branch renumbers (a migration only if it was never applied anywhere, with no commit left holding two files at one version) and rewrites every reference | the competing numbers sit on unmerged branches no pull request's gate reads |
| **`CONCURRENTLY` cannot run in a `DO` block**, and `CREATE EXTENSION` needs a superuser. | conditional migrations | separate the conditional part; document the grant. An index on a table the same block just created is built without `CONCURRENTLY` (nobody reads it yet — say so in a comment); a migration without the right to create an extension treats `insufficient_privilege` as absence. A bare `exit 1` from `gate-compose` or `gate-e2e`: read `compose logs migrate` | PostgreSQL refuses it at run time; the trap is in designing the migration |
| **Rank keys need byte order** (`COLLATE "C"`); a glibc collation interleaves them, and CI's musl image hides it. | `order_key` queries | state the collation in the query | CI's musl image sorts like `C`, so no test there can fail |
| **`id <> $x` with an empty or null `$x`** compares against nothing: the level comes back empty, or the query answers 500. | rank-neighbour and "all but this one" queries | `id IS DISTINCT FROM sqlc.narg('x')::uuid`, as `Work.sql` and `Structure.sql` do | the SQL is valid; only an empty or null argument shows it |
| **The audit hash covers the stored shape**, not the one the caller built. | audit entries, renames | read back, then hash; never rewrite a stored row | the chain check finds the break only after the row is stored |
| **Restore and import treat credentials differently.** A backup keeps password hashes; an export strips them with every token hash, so accounts brought in from an export have no password. A destructive restore ends the sessions and tokens of the accounts it rewrites. | restore, import | say what a person must do after; nobody is locked out without a way back | a consequence to tell people, not a code pattern |
| **The integration tests share one database**: the hub level is tenant-wide, and `write()` opens a pool per call. | `test/integration` | own tenants and rank keys; one pool in a loop; run the whole package; drop any object a test creates (index, constraint, function) in `t.Cleanup`, or the schema-reference test reports it as drift in a full run | isolation is a property of each test's data, seen only when files run together |

## Clients

| Trap | Where it bites | How to avoid it | Why no gate |
|---|---|---|---|
| **`engine.reset()` clears every subscription**; a module-level resource must subscribe again. | long-lived client modules | re-subscribe after a reset | whether a module outlives a reset is run-time structure |
| **A live region created already filled is not announced.** | status messages | mount it empty, fill it after | the accessibility checks see that the region exists, not when it was filled |
| **A drag is measured, not assumed**: a grid fills its container, a margin forgets the gap, `data-x=""` is falsy. | drag and layout code | a walk that actually drags | only a rendered layout shows it |
| **The design-system gates read comments**: a tag name or a pixel value in a comment fails them, and `#359` in a front-end comment is a colour to the literal lint. | Svelte and TS comments | no markup, values or hashed issue numbers in comments | teaching them to skip comments would let a value hide in one |
| **The Docker `ui` stage copies little**; an import reaching out of `apps/webapp` fails only in the image. | relative imports | import through packages | the container build catches it, late; nothing earlier sees the image's tree |
| **`locales/*.json` is grouped, not sorted**; a re-dump buries the change. | adding message codes | insert by hand next to the neighbours | a sort-order gate would force the very re-dump that buries the change |
| **A global class rule restyles every scoped class of that name.** Svelte scopes a component's own selectors only, and `.panel` or `.fields` mean different things in different views. | `apps/webapp/src/app.css` | element selectors only in `app.css`; a screen's rules stay in the screen | how far a selector should reach is intent |
| **Splitting a screen or removing a wrapper breaks what pointed into it**: a tour anchor (`data-tour`) points at nothing, message codes lose their use, a field layout sized for the old wrapper stretches. | refactoring screens | grep for `data-tour` and the codes the old screen rendered; look at every field layout at full width | anchors and codes are strings, found by search, not by a type |
| **A count needs the plural form**: `{count, plural, one {…} other {…}}` — "1 workspaces" is a bug. | any message with a number | use the plural form; both renderers parse it | whether a placeholder is a count is meaning, not syntax |

## Tests, gates and tooling

| Trap | How to avoid it | Why no gate |
|---|---|---|
| **A test's scratch repository inherits `GIT_DIR`.** Under a git hook or `git rebase --exec`, git exports `GIT_DIR` for the real repository; a scratch `git init` then re-initialises it, and `core.bare = true` stops the checkout and every worktree. | run git in a scratch repository with every `GIT_*` variable dropped (`scratchGitEnv` in tools/checkpr, `withoutGitVariables` in test/architecture); if it happened, `git config core.bare false` in the main checkout | a new helper can call git anywhere; the existing ones are held by a test that sets `GIT_DIR` |
| **The pull request hook reads a quoted `gh pr create` as a command**: a description written through a heredoc that quotes the template's comment is refused as a non-draft create. | write the description with a file tool, then pass `--body-file` | the hook cannot parse shell quoting; refusing too much is its safe side |
| **`make gate-x \| tail && git commit` commits on red**, and `git push \| tail` hides a rejected push. | test the command's own exit status | it is the shell's behaviour in the worker's own command |
| **`git add -A` while a gate regenerates files** stages deletions (the licence gate rewrites `third-party/licenses/`). | never stage while a gate runs; check `git show --stat` | a race between two of the worker's own commands |
| **`make gate-selftest` edits the working tree.** Overlapping runs, or a gate beside it, report each other's probes as regressions. | run it alone | it has to edit the tree to prove the gates go red |
| **Coverage is enforced per package**: new use cases (long `Descriptor()`/`invoke()`) push a service package under its threshold at the end of a task. | one registry-level test per use-case group | the gate exists; the trap is that it bites at the end of a task |
| **Secret scanning reads the whole commit range**: a flagged fixture stays red until it is gone from every commit. | fixtures that cannot look like a credential; fold the fix into the commit that introduced it | the scanner is the gate; the trap is getting past a true hit |
| **A generated file must not call out**: a field resolved over the network at generation time makes the no-diff gate flaky. | generate from local inputs only | the no-diff gate is what turns flaky; the call is found by reading the generator |
| **An alert test can agree with the bug**: `promtool` sees only the series the test invents; a selector that matches nothing is an empty vector, and the alert stays silent. | name series exactly as the code emits them; check the metric exists | `promtool` sees only what the test feeds it |
| **A hand-kept list goes stale**: a dependency list in a Makefile or a workflow drifts from what the build really reads. | derive it, or test it against the source | each list has its own source; the test is written where one bites |
| **A green local run proves nothing about a pull**: a cached image hides a registry that stopped serving it. | pull fresh in CI before trusting a container gate | the cache is on the machine, outside the repository |
| **In zsh, `$ID:complete` is a parameter modifier**, not a path; the request goes to the wrong route and answers 405. | brace it: `${ID}:complete` | it is the worker's shell, not the repository |
| **A message code runs the Go lane**: `locales/**` is in CI's `go` filter because the binary embeds the catalogue, so a client-only pull request adding a code runs every Go gate — and waits for a red `gate-security` on `main` to be fixed. | expect the Go gates; fix an open advisory first (take the named version, `make licenses`) | intended: the binary embeds the catalogue |
| **`grep -q` after a pipe under `set -o pipefail` reports a match as a failure**: `grep` exits on the first match and the writer dies of SIGPIPE. | match a variable or a here-string | it is shell semantics, and no linter in use flags it |
| **A local composite action hides its pins**: the pin check scans only `.github/workflows`. | no third-party `uses:` under `.github/actions/` until the scan covers it | the pin scan reads only `.github/workflows`; widening it is open |
| **A red CodeQL check shows no detail**, and `make verify` cannot see it (gosec carries other queries). The alert belongs to `refs/pull/<n>/merge` and may sit in a file the pull request only touched — or one alert open on `main` already. CodeQL is not a required check. | read it with `gh api "repos/<owner>/<repo>/code-scanning/alerts?ref=refs/pull/<n>/merge"`, and the flow behind it from the analysis (`…/code-scanning/analyses?ref=…`, fetched with `Accept: application/sarif+json`) before renaming anything — an identifier containing "password" is sensitive to CodeQL by its name alone; bind a bound to one variable, guard it, allocate from it, and drop the now unneeded `//nolint:gosec` | CodeQL is GitHub's check, outside `make` |
| **`Closes #n` closes one issue per bare line**; inside a code span it closes nothing, and `gate-pr` checks only that one bare line exists. | one bare `Closes #n` line per issue; confirm each issue's state after the merge | GitHub's parser decides what closes; `gate-pr` checks one bare line |
| **A decomposed fixture typed as text arrives composed**: editors normalise what they write, so a normalisation test proves nothing. | write decomposed fixtures as escapes and assert they differ from their composed twin | the editor normalises before any tool reads the file |
| **A premise in a task can be wrong** — "nothing does X" while the code has done it for months. | check every claim against the code (readiness record §1) | a claim in prose is checked against the code by reading |
| **A proposal can promise a lever that does not exist.** | check every capability a proposal names against the code before it reaches the owner | a claim in prose is checked against the code by reading |

## Flaky tests

One red on a diff that does not touch the area: re-run the job once before reading code. Two in a
row is a real failure — except `make tools`: when the failing step in every red job is `make tools`
and the error names a `sum.golang.org` tile, re-run up to three times, then call it an outage. Two `make verify` runs in parallel worktrees, or a
walk's Docker stack beside the web app's engine tests, load the machine into timeouts: run them one
after another and read a wall of timeouts as load first. `gh run rerun --failed` is refused while
the run is still going.

Why no gate: a flake is a test that is right most of the time, so a gate would refuse good changes;
each one stays listed until its test is made deterministic.

| Test | Symptom |
|---|---|
| `make tools` in CI | `sum.golang.org` drops the connection; several unrelated jobs red at once |
| `TestSeveralHundredIdleStreamsDoNotPoll` | one poll tick inside the 300 ms window on a loaded runner |
| `TestTheSessionSweepStaysInsideTheTenantAndTakesOnlyTheOver` | wall-clock sensitive in the shared integration database |
| hubctl e2e, AI-stub section | "no suggestion arrived within 90s" while the stub image is still being pulled |
| engines, Firefox | `cursor` `undefined` on the first assertion; green on re-run |
| RT-1's container test | a Docker Hub token error pulling `nginx:alpine`; green on re-run |
| a Testcontainers gate run locally | `address already in use` on a random high port: another worktree's container on the shared Docker daemon took it — re-run the gate alone. A fixed port (18081, 19091) is a real collision |
