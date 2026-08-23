# ADR-0033 — One product UI, three targets: the shared client architecture

**Status:** accepted · **Date:** 2026-08-23

## Context

[ADR-0030](./ADR-0030-svelte-frontend-framework.md) settles the framework,
[ADR-0031](./ADR-0031-tauri-app-shell.md) the shells, and
[ADR-0032](./ADR-0032-client-capability-matrix.md) promises near-parity — a promise that is only
cheap if web, desktop and mobile genuinely build from the same code. How the codebase is cut is
therefore a structural decision, and it collides with two rules that already exist:

* [ADR-0027](./ADR-0027-monorepo-structure.md) rule 3: `apps/*` may depend on `packages/*`,
  never the reverse, and **never `apps/*` on `apps/*`**. So the product UI cannot live in one
  app and be imported by two shell apps.
* CLAUDE.md's dependency map currently reads `packages/* → nothing here`. Taken literally, a new
  shared package could not use `@hubtask/api-client` — which any client-side sync engine must.

The sync engine is the part with the sharpest requirements.
[ADR-0021](./ADR-0021-offline-sync.md) and [offline-sync.md](../architecture/offline-sync.md) §9
bind every client to the same protocol behaviour (UUIDv7 IDs, an `op_id` and HLC per mutation,
deletion on `ACCESS_REVOKED`, full resync on `sync.cursor_too_old`, encrypted local storage wiped
on sign-out), and the server owns all merging. That behaviour must be testable without a UI —
`hubctl sync-conformance` will check it against a running instance, and a sync engine that can
only be exercised by clicking through Svelte components cannot pass a conformance suite.

Open points SY-B (default sync scope) and SY-D (local-cache encryption per platform) were parked
"with the frontend decision". This is that decision, so they are answered here for first-party
clients.

## Options

**A. Three apps (webapp, desktop, mobile), shared code in packages.** The obvious cut, and wrong
here: the "shared code" would be the entire product UI, leaving three near-empty apps and one
giant package — a layout that maximises indirection to honour a rule the next option satisfies
directly.

**B. Shells import the webapp across `apps/`.** Forbidden by ADR-0027 rule 3, and rightly: it
would make one app's internals another app's contract.

**C. One application directory that builds all targets; framework-agnostic logic in packages
(chosen).**

## Decision

**1. `apps/webapp` is the one product UI, and every target builds from it.** The Vite build
produces the static bundle ADR-0028 embeds; the Tauri shells live inside the same application as
`apps/webapp/src-tauri/` (one Tauri project, desktop and mobile targets) and wrap the same build.
No second app directory, nothing shared across `apps/`, ADR-0027 rule 3 untouched. The name
`webapp` keeps its ADR-0027 meaning — the product application, as distinct from the website —
and the web remains its primary target; renaming was considered and rejected as churn without a
beneficiary. Platform differences are confined to one seam: `src/lib/platform/` defines the
platform interface, with a browser and a Tauri implementation chosen at build time — no
`isTauri` conditionals scattered through components.

**2. The sync engine is `packages/sync-engine`: framework-agnostic TypeScript, no Svelte
anywhere in it.** It implements the client half of ADR-0021 — the local store schema, the
mutation queue, `:pull`/`:push`/SSE orchestration, HLC generation, cursor and tombstone handling,
`ACCESS_REVOKED` and `cursor_too_old` behaviour — against three narrow interfaces it defines:

| Port | Implemented by |
|---|---|
| `Transport` | `@hubtask/api-client` in production; an in-memory fake in tests |
| `Storage` | IndexedDB (browser), SQLite via Tauri plugin (shells), in-memory (tests) |
| `Clock` | Real time in production; a controlled clock in tests |

The engine **never merges**. Merging is the server's (offline-sync.md §4); the engine queues,
pushes, applies what the server answers, and surfaces `conflict`/`rejected` states for the UI to
render. A merge rule appearing in this package is a bug against ADR-0021.

**The engine is UI-independently testable, as a hard requirement:** its test suite runs headless
against the fakes, covers the client obligations of offline-sync.md §9 point by point, and is the
first-party counterpart to `hubctl sync-conformance`. Svelte binds to the engine through a thin
adapter layer in `apps/webapp` (runes wrapping the engine's subscription API); components never
talk to storage or transport directly.

**3. The dependency map gains one refinement: `packages/*` may depend on `packages/*`,
acyclically.** Concretely: `sync-engine → api-client`, and nothing else new. `design-system` and
`api-client` remain leaves; `apps/* ↛ apps/*` and `packages/* ↛ apps/*` stand unchanged. This
refines ADR-0027's rule 3 (which constrained the apps/packages directions and is untouched) and
narrows only CLAUDE.md's summary line `packages/* → nothing here`, which is updated on
acceptance. `packages/api-client` stays free of everything but generated output (ADR-0027's
deferred SDK extraction is unaffected: `sync-engine` depends on it, not the reverse).

**4. Local persistence per platform — answering SY-D for first-party clients.**

| Platform | Store | At rest |
|---|---|---|
| Browser | IndexedDB | Best-effort cache only: resilience across brief disconnects, never the offline promise ([ADR-0031](./ADR-0031-tauri-app-shell.md)); no browser-side encryption theatre — what must not live unencrypted on a shared machine does not go into browser storage, which is why the promise lives in the shells |
| Desktop / mobile | SQLite via the Tauri SQL plugin | Encrypted at rest; the key is generated per device and held in the platform keystore (Keychain, Windows Credential Store, libsecret, Android Keystore), never on disk beside the data |

Sign-out deletes the store completely on every platform, per §9.6. The concrete plugin set
(SQL, keychain/stronghold) is confirmed as a dependency decision in the shell work packages,
under CLAUDE.md's dependency rule. SY-B (default sync scope: everything vs. subscribed
containers) is engine configuration by design — the scope parameter is already in the `:pull`
contract — and the product default (mobile syncs subscribed containers, desktop and web sync the
workspace) is set in the sync-engine work package, where real payload sizes exist to measure.

**5. Design-system consumption is uniform and already decided** —
[ADR-0029](./ADR-0029-design-system-tokens.md) governs; this ADR only fixes the mechanics: the
app imports the generated `tokens.css` and `fonts.css` once at its root, components reference
custom properties (via `tokens.ts` where a value is needed in script), and the shells inject
nothing of their own. One bundle means one theming surface on all five platforms.

## Consequences

* Parity (ADR-0032) is the build's natural output: a feature is written once in `apps/webapp`
  and exists everywhere the next time each target builds. The mobile admin exclusion is a
  route-level gate, not a second codebase.
* The sync engine can be developed and tested now-ish, long before shells exist — against the
  fakes and against a real server — and third-party client authors get a reference
  implementation to read next to offline-sync.md.
* Rust source lives under `apps/webapp/src-tauri/`. ADR-0027's "no `.go` under `apps/`" is
  untouched; the pnpm workspace ignores the directory, Go ignores it, and only the Tauri CI lane
  builds it.
* One codebase serving five platforms means platform-specific bugs surface in shared code. The
  `platform/` seam bounds that: anything conditioned on platform lives behind it, so the blast
  radius of a webview quirk is an adapter, not a component tree.
* The dependency-map refinement is a real loosening, taken knowingly and kept minimal: acyclic,
  one new edge, and the architecture lint for the workspace (the `pnpm` counterpart of
  `gate-architecture`) enforces the allowed edges so the map stays a checked fact rather than a
  drawing.
* `CLAUDE.md`'s map and `project-structure.md` §2 are updated on acceptance; nested `CLAUDE.md`
  files for `packages/sync-engine` follow with its scaffold.

### Backlog impact

| Work package | Target |
|---|---|
| `packages/sync-engine` skeleton: ports, store schema, queue, HLC, §9 test harness against fakes | frontend track (roadmap phase 5) — before the shells |
| Workspace dependency lint: enforce the refined map across `apps/*` and `packages/*` | current workspace milestone |
| Platform seam in the webapp scaffold (`src/lib/platform/`, browser implementation) | current workspace milestone, with the ADR-0030 scaffold |
| Tauri persistence: SQLite + keystore integration behind the `Storage` port | frontend track, with the desktop shell (ADR-0031) |

## Notes

Related: [ADR-0021](./ADR-0021-offline-sync.md) and
[offline-sync.md](../architecture/offline-sync.md) §4/§9/SY-B/SY-D (the contract the engine
implements and the open points this closes for first-party clients),
[ADR-0027](./ADR-0027-monorepo-structure.md) (the rules this cut honours and the one summary line
it refines), [ADR-0030](./ADR-0030-svelte-frontend-framework.md) (the framework),
[ADR-0031](./ADR-0031-tauri-app-shell.md) (the shells and the browser/installed split of the
offline promise), [ADR-0032](./ADR-0032-client-capability-matrix.md) (the parity this makes
cheap), [ADR-0029](./ADR-0029-design-system-tokens.md) (token consumption),
[ADR-0004](./ADR-0004-api-first-openapi.md) (the generated client the engine builds on).
