# Known traps

Defects that passed every gate once and were found later — by a walk, a real database, an operator.
Each is a class, not an incident: it will happen again wherever the same shape is built. A readiness
record (`docs/backlog/ready/`, § 6) names the ones that apply to its task and how it avoids each.

Until 2026-10-04 these lived only in the private notes of individual sessions. They are here so that
every session — and every person — reads the same list. A new class found by a walk, a check or an
escape (`docs/backlog/ready/README.md` § "Escape classes") is added here in the pull request that
fixes its first instance.

## Transactions and wiring

| Trap | Where it bites | How to avoid it |
|---|---|---|
| **A write outside `UnitOfWork.Within`** passes every service test — the fake runs any callback — and answers 500 (`postgres.no_transaction_in_context`) against PostgreSQL. | audit entries, repository calls added after the main transaction | an integration test that drives the path against the database |
| **A write on a refusal path is rolled back with the refusal.** A failure counter, a lockout or an audit entry written inside the transaction that then returns the error is never stored. | sign-in, second factor, step-up, any "count the failure and refuse" | write it in its own transaction, detached from the request's cancellation; the fake unit of work in identity tests rolls the attempt ledger back to show it |
| **A nested scope may not switch tenant.** `InstallationScope()` inside a tenant transaction is refused (`postgres.tenant_switch_in_transaction`); a swallowed error looks like a policy decision. | helpers that open their own scope | use the ambient context inside somebody else's transaction |
| **Writer copies miss a late field.** `SessionWriter` is a value; copies taken in `cmd/server/main.go` before a field is assigned never learn it. | any field set on `sessionWriter` after other writers copied it | set it in the literal; a wiring test in `cmd/server` |
| **A port can be written and never called.** Compiles, is documented in the contract, nothing calls it. | ports added "for later" | a test that drives the use case, not the port |
| **A read-then-write without a lock** loses an update when two requests interleave; a check in one transaction and the act in another lets the state change in between. | switches, counters, removals guarded by a count | do the check in the statement that writes (conditional `UPDATE`/`DELETE`), or lock the row |

## Contract, registry and adapters

| Trap | Where it bites | How to avoid it |
|---|---|---|
| **`Handler.Invoke` skips validation.** A test that calls the handler directly never meets the registry's input validation. | refusal tests | go through `registry.Invoke` or the REST route |
| **The registry refuses an undeclared input key** (`usecase.field_unknown`) — a route that looks implemented answers 400. | REST controllers building the input map | a contract test through the router |
| **An explicit JSON `null` reads as absent** (`Input.Present`). | adapters decoding nullable fields | write `""` for a JSON null where "cleared" must be told from "omitted" |
| **A contract enum value the descriptor does not accept** is refused generically before the domain's own refusal runs. | enums in `openapi.yaml` and descriptors | the gate comparing both; keep them in one place |
| **A manifest declares what it does not answer.** A capability or list field published before anything fills it. | `/meta/capabilities` | check the answer against what the feature does, not the schema |
| **A capability test can ask itself.** Two code paths running the same query agree with each other and prove nothing. | capability and availability tests | test against what the feature actually needs |
| **A search must resolve what it parses** — placeholders like `@me` resolved in one read path and not the other. | anything parsed by one grammar and read twice | one resolution step both paths call |
| **A public route still verifies a presented bearer**, and answers 401 to a stale one. | clients calling public routes with a stored token | do not send a token where none is needed |
| **No display text in an API answer** — a "reason" or "message" field written in English is rule 8 broken. | new manifest or error fields | message codes, or identifiers (an `operationId`, a header name) |

## Database and migrations

| Trap | Where it bites | How to avoid it |
|---|---|---|
| **A superuser hides migration defects.** Every test environment migrates as one; a migration that needs superuser fails on an operator's database. | `CREATE EXTENSION`, `CREATE ROLE`, policies | run it as the migrator role; document what an operator must grant |
| **`SECURITY DEFINER` functions rely on the owner bypassing row level security**, as `resolve_tenant` does. | functions reading across workspaces | answer only what the caller may know (a number, never a list); guard the scope inside the function too |
| **Row mappers that drop `tenant_id`** read fine and break everything that rebuilds the aggregate from the row. | repository mappers | select the tenant; a round-trip test |
| **`db/schema.sql` mirrors the migrations** and is compared against a migrated database. | new functions, indexes, policies | change both in the same commit |
| **Migration and ADR numbers taken by unmerged branches collide.** | new migrations, new ADRs | take the number from `origin/main` right before writing the file |
| **`CONCURRENTLY` cannot run in a `DO` block**, and `CREATE EXTENSION` needs a superuser. | conditional migrations | separate the conditional part; document the grant |
| **Rank keys need byte order** (`COLLATE "C"`); a glibc collation interleaves them, and CI's musl image hides it. | `order_key` queries | state the collation in the query |
| **`id <> $moving_id` with an empty id** empties the level or answers 500. | rank-neighbour queries | never pass an empty id |
| **The audit hash covers the stored shape**, not the shape the caller built. Never rewrite a stored row. | audit entries, renames | read back, then hash; renames are aliases on read |
| **A destructive restore ends credentials** — accounts are re-written without passwords or sessions. | restore, import | say what a person must do after; nobody is locked out without a way back |
| **The integration package shares one database**: hub level is tenant-wide, `write()` opens a pool per call. | integration tests | own tenants and ring keys; one pool in loops; run the whole package |

## Clients

| Trap | Where it bites | How to avoid it |
|---|---|---|
| **`engine.reset()` clears every subscription**; a module-level resource must re-subscribe. | long-lived client modules | subscribe again after a reset |
| **A live region created already filled is not announced.** | status messages | mount it empty, fill it after |
| **A drag is measured, not assumed**: a grid fills its container, a margin forgets the gap, `data-x=""` is falsy. | drag and layout code | a walk that actually drags |
| **The design-system conventions gate reads comments**; a tag name or pixel value in a comment fails it. | Svelte components | no markup or values in comments |
| **The Docker `ui` stage copies little**; an import reaching out of `apps/webapp` fails only in the image. | relative imports | import through packages |
| **`locales/*.json` is grouped, not sorted**; a re-dump buries the change. | adding message codes | insert by text next to the neighbours |
| **The catalogue's plural subset**: `{count, plural, one {…} other {…}}` — "1 workspaces" is a bug. | any message with a count | use the plural form |

## The process itself

| Trap | How to avoid it |
|---|---|
| **The premise of a task can be wrong** — "nothing does X" while the code has done it for months; a *Today* that claims more than the code does. | § 1 of the readiness record, checked with grep and a test |
| **A promise of "later"** (a migration that drops something, a follow-up) **without a task** is forgotten. | cut it as a task when it is promised |
| **A proposal that promises a capability that does not exist** — a lever, a switch — becomes a false sentence in an ADR. | check every capability a proposal names against the code before putting it to the owner |
| **`make gate | tail && git commit` commits on red**; `git push | tail` hides a rejected push. | test the command's own exit status |
| **`Closes #n` inside backticks does not close.** | the line starts with `Closes #` |
