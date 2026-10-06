# Known traps

Mistakes that pass every gate once and are found later — by a walk, a real database, a second run.
Each is a class, not an incident: it happens again wherever the same shape is built. A readiness
record names the ones that apply to its task (section 2(d) of `docs/backlog/ready/TEMPLATE.md`).

A trap belongs here only while nothing better holds it. The day a gate, a test or a comment where it
bites can hold it, it moves there and leaves this list. A new trap is added in the pull request that
fixes its first instance.

## Transactions and wiring

| Trap | Where it bites | How to avoid it |
|---|---|---|
| **A write outside `UnitOfWork.Within`** passes every service test — the fake runs any callback — and answers 500 (`postgres.no_transaction_in_context`) against PostgreSQL. | audit entries, repository calls added after the main transaction | an integration test that drives the path against the database |
| **A write on a refusal path is rolled back with the refusal.** A failure counter, a lockout or an audit entry written inside the transaction that then returns the error is never stored. | sign-in, second factor, step-up, any "count the failure and refuse" | write it in its own transaction, detached from the request's cancellation |
| **A nested scope may not switch tenant.** `InstallationScope()` inside a tenant transaction is refused (`postgres.tenant_switch_in_transaction`); a swallowed error looks like a policy decision. | helpers that open their own scope | use the ambient context inside somebody else's transaction |
| **Writer copies miss a late field.** `SessionWriter` is a value; copies taken in `cmd/server/main.go` before a field is assigned never learn it. | any field set on a writer after others copied it | set it in the literal; a wiring test in `cmd/server` |
| **A port can be written and never called.** It compiles and is documented; nothing calls it. | ports added "for later" | a test that drives the use case, not the port |
| **A read-then-write without a lock** loses an update when two requests interleave. | switches, counters, removals guarded by a count | check in the statement that writes (conditional `UPDATE`/`DELETE`), or lock the row |

## Contract, registry and adapters

| Trap | Where it bites | How to avoid it |
|---|---|---|
| **`Handler.Invoke` skips validation.** A test that calls the handler directly never meets the registry's input validation, so refusal codes the registry answers first look reachable. | refusal tests | go through `registry.Invoke` or the REST route |
| **The registry refuses an undeclared input key** (`usecase.field_unknown`): a route that looks implemented answers 400. | REST controllers building the input map | a contract test through the router |
| **An explicit JSON `null` reads as absent** (`Input.Present`). | adapters decoding nullable fields | send `""` where "cleared" must be told from "omitted" |
| **A manifest declares what it does not answer** — a capability or field published before anything fills it. | `/meta/capabilities` | check the answer against what the feature does, not the schema |
| **A capability test can ask itself.** Two code paths running the same query agree and prove nothing. | capability and availability tests | test against what the feature needs |
| **A search must resolve what it parses** — a placeholder like `@me` resolved in one read path and not the other. | anything parsed once and read twice | one resolution step both paths call |
| **A public route still verifies a presented bearer**, and answers 401 to a stale one. | clients calling public routes with a stored token | send no token where none is needed |

## Database and migrations

| Trap | Where it bites | How to avoid it |
|---|---|---|
| **A superuser hides migration defects.** Test environments migrate as one; a migration that needs one fails on an operator's database. | `CREATE EXTENSION`, `CREATE ROLE`, policies | run it as the migrator role; document what an operator grants |
| **`SECURITY DEFINER` functions bypass row level security** as their owner. | functions reading across workspaces | answer only what the caller may know (a number, never a list); guard the scope inside |
| **A row mapper that drops `tenant_id`** reads fine and breaks whatever rebuilds the aggregate from the row. | repository mappers | select the tenant; a round-trip test |
| **Migration and ADR numbers taken by unmerged branches collide.** | new migrations, new ADRs | take the number from all remote branches right before writing the file |
| **`CONCURRENTLY` cannot run in a `DO` block**, and `CREATE EXTENSION` needs a superuser. | conditional migrations | separate the conditional part; document the grant |
| **Rank keys need byte order** (`COLLATE "C"`); a glibc collation interleaves them, and CI's musl image hides it. | `order_key` queries | state the collation in the query |
| **`id <> $moving_id` with an empty id** empties the level or answers 500. | rank-neighbour queries | never pass an empty id |
| **The audit hash covers the stored shape**, not the one the caller built. | audit entries, renames | read back, then hash; never rewrite a stored row |
| **Restore and import treat credentials differently.** A backup keeps password hashes; an export strips them with every token hash, so accounts brought in from an export have no password. A destructive restore ends the sessions and tokens of the accounts it rewrites. | restore, import | say what a person must do after; nobody is locked out without a way back |
| **The integration tests share one database**: the hub level is tenant-wide, and `write()` opens a pool per call. | `test/integration` | own tenants and rank keys; one pool in a loop; run the whole package |

## Clients

| Trap | Where it bites | How to avoid it |
|---|---|---|
| **`engine.reset()` clears every subscription**; a module-level resource must subscribe again. | long-lived client modules | re-subscribe after a reset |
| **A live region created already filled is not announced.** | status messages | mount it empty, fill it after |
| **A drag is measured, not assumed**: a grid fills its container, a margin forgets the gap, `data-x=""` is falsy. | drag and layout code | a walk that actually drags |
| **The design-system gates read comments**: a tag name or a pixel value in a comment fails them, and `#359` in a front-end comment is a colour to the literal lint. | Svelte and TS comments | no markup, values or hashed issue numbers in comments |
| **The Docker `ui` stage copies little**; an import reaching out of `apps/webapp` fails only in the image. | relative imports | import through packages |
| **`locales/*.json` is grouped, not sorted**; a re-dump buries the change. | adding message codes | insert by hand next to the neighbours |
| **A count needs the plural form**: `{count, plural, one {…} other {…}}` — "1 workspaces" is a bug. | any message with a number | use the plural form; both renderers parse it |

## Tests, gates and tooling

| Trap | How to avoid it |
|---|---|
| **`make gate-x \| tail && git commit` commits on red**, and `git push \| tail` hides a rejected push. | test the command's own exit status |
| **`git add -A` while a gate regenerates files** stages deletions (the licence gate rewrites `third-party/licenses/`). | never stage while a gate runs; check `git show --stat` |
| **`make gate-selftest` edits the working tree.** Overlapping runs, or a gate beside it, report each other's probes as regressions. | run it alone |
| **Coverage is enforced per package**: new use cases (long `Descriptor()`/`invoke()`) push a service package under its threshold at the end of a task. | one registry-level test per use-case group |
| **Secret scanning reads the whole commit range**: a flagged fixture stays red until it is gone from every commit. | fixtures that cannot look like a credential; fold the fix into the commit that introduced it |
| **A generated file must not call out**: a field resolved over the network at generation time makes the no-diff gate flaky. | generate from local inputs only |
| **An alert test can agree with the bug**: `promtool` sees only the series the test invents; a selector that matches nothing is an empty vector, and the alert stays silent. | name series exactly as the code emits them; check the metric exists |
| **A hand-kept list goes stale**: a dependency list in a Makefile or a workflow drifts from what the build really reads. | derive it, or test it against the source |
| **A green local run proves nothing about a pull**: a cached image hides a registry that stopped serving it. | pull fresh in CI before trusting a container gate |
| **In zsh, `$ID:complete` is a parameter modifier**, not a path; the request goes to the wrong route and answers 405. | brace it: `${ID}:complete` |
| **A premise in a task can be wrong** — "nothing does X" while the code has done it for months. | check every claim against the code (readiness record §1) |
| **A proposal can promise a lever that does not exist.** | check every capability a proposal names against the code before it reaches the owner |

## Flaky tests

One red on a diff that does not touch the area: re-run the job once before reading code. Two in a
row is a real failure.

| Test | Symptom |
|---|---|
| `make tools` in CI | `sum.golang.org` drops the connection; several unrelated jobs red at once |
| `TestSeveralHundredIdleStreamsDoNotPoll` | one poll tick inside the 300 ms window on a loaded runner |
| `TestTheSessionSweepStaysInsideTheTenantAndTakesOnlyTheOver` | wall-clock sensitive in the shared integration database |
| hubctl e2e, AI-stub section | "no suggestion arrived within 90s" while the stub image is still being pulled |
| engines, Firefox | `cursor` `undefined` on the first assertion; green on re-run |
