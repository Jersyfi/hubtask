# Test evidence

Results of the runs that cannot happen in a pipeline — and the first run of any that newly can,
recorded once before the gate takes over. One file per run.

Mostly the reliability tests. Not only: a dogfooding pass is a run whose result is a file too, and
`R-08-*.md` is one — a route walked in the application, so the next walk can be compared with it.

The catalogue and the cadence each one is expected at are in [observability-reliability.md §12](../architecture/observability-reliability.md).

Older runs are history in [`docs/archive/evidence/`](../archive/evidence/). The newest coverage
report stays here, because `make gate-docs` holds the catalogue to it.

## How to walk

A walk is the evidence a reading test cannot give: the product, running, used the way a person uses
it. Two kinds, chosen by what the change can break.

**Looking — "does this look right".** For layout, copy and states of a screen, and for showing the
owner a change before it merges (AGENTS.md, "Working with the owner"):

```bash
pnpm --filter @hubtask/webapp build
node apps/webapp/e2e/preview.mjs          # http://localhost:5180, PORT= to change it
```

Any email and password sign in; every read is the fixture the engine walks use
(`apps/webapp/e2e/fixture.mjs`). Rebuild and reload after a change. A screen the fixture does not
cover is extended there, not in a private stub, so the next walk and the engine tests see it too.

**Using — behaviour.** Anything that writes, syncs, authorises, or crosses the server: walk against
a real server, because the fixture agrees with whatever the client sends.

1. A server and a database: `make db-up && make migrate && make run ROLES=api`, or the image through
   `deploy/docker/compose.yaml` (`make gate-compose` builds it).
2. A workspace somebody can sign in to: `scripts/dev-workspace.sh --bootstrap`, then
   `scripts/dev-workspace.sh` — through the product's own operations, password included.
3. Raise `HUBTASK_RATE_LIMIT_BURST` for the walk (600 is enough): a cold entry page is a burst of
   two dozen requests, and a scripted walk is many cold pages. `compose.yaml` does not forward it;
   pass it through an override file.
4. Sign in through the page or `POST /auth/sessions`. A personal access token is refused by the
   snapshot and the stream, so a walk on one never fills the local copy.

**Scripting a walk.** Playwright lives in `apps/webapp/node_modules`, with the three engines. A
probe is a temporary file under `apps/webapp/e2e/` that is deleted afterwards.

- Load each route in a fresh page and read `document.activeElement` before every key press; press
  only what the script named. A keystroke on an unknown focus is a write nobody asked for.
- Seed a throwaway account for the walk, never the owner's.
- One browser context per width, signed in per context: a credential holds a capped number of open
  streams, and closed contexts' streams linger, so `/stream` answers 503 to the thirtieth.
- Attach request listeners before `page.goto`, or the first burst is not counted.
- Measure a size before describing it, in a test or in prose: read `getBoundingClientRect`, not the
  screenshot.
- Two one-time codes in one 30-second window: the second is refused as a replay. Wait for the next
  window (`scripts/hubctl-e2e.sh` shows how).
- An editor's built-in browser pane may never run animation frames or turn Enter into a click;
  check motion, focus rings and key activation with Playwright.

**Recording it.** A walk that is evidence for a use case check is a file here — what was walked, at
which commit, what was pressed, what was seen, and what was found. A finding becomes a `finding`
issue; the walk file links it.
