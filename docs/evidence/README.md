# Test evidence

Results of the runs no pipeline can make, one file per run: the reliability runs of the catalogue
in [observability-reliability.md §12](../architecture/observability-reliability.md) that run by
hand or once per release, the walk on the integration environment
([offline-sync.md §11](../architecture/offline-sync.md)), the restore drill there, and the two
accessibility walks before `1.0.0` ([design-system.md §10](../design/design-system.md)). A use case
check is not proven here: it is proven by a test that asserts it
([`../usecases/`](../usecases/README.md)).

The newest coverage report stays here, because `make gate-docs` holds the catalogue to it.

The other files here — walks and run records of tasks before 2026-10-20 — are history: they stay,
a `checked_by:` that names one keeps working, and none of them is read for current work. The walks
of milestone UX and of the tasks beside it from 2026-10-20 on are gone; each check they showed is
proven by the tests its use case names (the owner, 2026-10-23). Older runs are in
[`docs/archive/evidence/`](../archive/evidence/).

## How to walk

A walk is the product, running, used the way a person uses it: to show the owner a change, and to
see what a test should then pin down. Two kinds, chosen by what the change can break.

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
   `deploy/docker/compose.yaml` (`make gate-compose` builds it). `make run` is `go run`: stopping
   the parent leaves the server on its port, so a walk driven by a process manager runs a built
   binary (`go build -o`).
2. A workspace somebody can sign in to: the server prints a setup code on its console at a fresh
   start; `HUBTASK_SETUP_CODE=… scripts/dev-workspace.sh --bootstrap` spends it (or the *Set up
   Hubtask* card does), then `scripts/dev-workspace.sh` provisions more — through the product's own
   operations, password included.
3. Raise `HUBTASK_RATE_LIMIT_BURST` for the walk (600 is enough): a cold entry page is a burst of
   two dozen requests, and a scripted walk is many cold pages. `compose.yaml` does not forward it;
   pass it through an override file.
4. Sign in through the page or `POST /auth/sessions`. A personal access token is refused by the
   snapshot and the stream, so a walk on one never fills the local copy.
5. End the onboarding tour (`PATCH /accounts/{id}/preferences` with `onboarding_completed_at`), or it
   covers the first screen of every fresh account.

**Demo data.** `scripts/seed-demo.sh` fills a new workspace with realistic content in every
product, through the product's own operations (REST, never SQL): eight people with roles and three
groups; the Scrum, Kanban, classic project and ITIL templates taken; stories, cards, deliverables,
risks and incidents with types, fields, links, statuses and comments; sprints, releases, a Gantt
plan with dependencies, milestones and a baseline; portal requests from two organisations, two
portal articles, SLAs, an approved change and one waiting for the CAB, a macro's run; CIs with
relations and a service; wiki pages with versions and an image; whiteboards; time records and
timesheets; goals, portfolios and dashboards; two registered agents.
`--empty` makes a workspace with nothing in it beside, with its first start and tour. It needs a
multi-workspace installation (`HUBTASK_TENANCY_MODE=multi`), `HUBTASK_ADMIN_TOKEN` from
`scripts/dev-workspace.sh --bootstrap`, `HUBTASK_DEMO_PASSWORD` (every demo person's password, not
holding the slug) and, locally, a writable `HUBTASK_STORAGE_LOCAL_PATH` for the uploaded picture.
A slug is provisioned once; the script lists any write the server refused and exits non-zero.

```bash
scripts/seed-demo.sh --api http://localhost:8080 --slug nordlicht
scripts/seed-demo.sh --api http://localhost:8080 --slug leer --empty
make demo DEMO_SLUG=nordlicht        # the same, through make; DEMO_EMPTY=1 for --empty
```

**The whole demo on one machine**, from a checkout to a filled workspace in about a minute: a
database of its own beside the dev one (which collects e2e workspaces), the built web app inside
the server binary, and every secret generated into `$DEMO/env.sh`, outside the repository. In bash
or zsh, from the checkout's root:

```bash
DEMO=$HOME/.cache/hubtask-demo; mkdir -p "$DEMO"/{bin,storage,backups,state}
corepack pnpm --filter @hubtask/webapp build && cp -R apps/webapp/dist/. presentation/webui/dist/
for b in server migrate hubctl; do go build -o "$DEMO/bin/$b" "./cmd/$b"; done
rm -rf presentation/webui/dist/assets presentation/webui/dist/favicon.svg
git checkout presentation/webui/dist/index.html          # the committed placeholder back
printf 'services:\n  postgres:\n    ports: !override\n      - "127.0.0.1:55432:5432"\n' > "$DEMO/port.yaml"
docker compose -p hubtask-demo -f deploy/docker/compose.dev.yaml -f "$DEMO/port.yaml" up -d --wait postgres
cat > "$DEMO/env.sh" <<EOF
export HUBTASK_DB_APP_PASSWORD=$(openssl rand -hex 16) HUBTASK_SECRET_KEY=$(openssl rand -base64 32)
export HUBTASK_ENCRYPTION_KEYS=k1 HUBTASK_ENCRYPTION_KEY_K1=$(openssl rand -base64 32)
export HUBTASK_DB_DSN="postgres://hubtask_app:\$HUBTASK_DB_APP_PASSWORD@127.0.0.1:55432/hubtask?sslmode=disable"
export HUBTASK_BASE_URL=http://localhost:8080 HUBTASK_HTTP_ADDR=127.0.0.1:8080 HUBTASK_OPS_ADDR=127.0.0.1:9090
export HUBTASK_TENANCY_MODE=multi HUBTASK_ROLES=api,worker,scheduler,automation HUBTASK_LOG_FORMAT=json
export HUBTASK_STORAGE_KIND=local HUBTASK_STORAGE_LOCAL_PATH=$DEMO/storage HUBTASK_BACKUP_LOCAL_PATH=$DEMO/backups
export HUBTASK_RESTORE_DRILL_RECORD_FILE=$DEMO/state/last_success_unix
export HUBTASK_RATE_LIMIT_BURST=600 HUBTASK_RATE_LIMIT_TENANT_BURST=2000
export HUBTASK_DEMO_PASSWORD=$(openssl rand -base64 30 | tr -dc 'A-Za-z0-9' | cut -c1-18)
EOF
. "$DEMO/env.sh"
HUBTASK_DB_DSN="postgres://hubtask:hubtask-dev@127.0.0.1:55432/hubtask?sslmode=disable" "$DEMO/bin/migrate" up
nohup "$DEMO/bin/server" > "$DEMO/server.log" 2>&1 & echo $! > "$DEMO/server.pid"
curl -sf --retry 30 --retry-connrefused -o /dev/null http://localhost:8080/api/v1/meta/capabilities
code=$(LC_ALL=C grep -ao '[A-Z0-9]\{4\}-[A-Z0-9]\{4\}-[A-Z0-9]\{4\}' "$DEMO/server.log" | head -1)
HUBTASK_SETUP_CODE=$code scripts/dev-workspace.sh --bootstrap | grep -o 'HUBTASK_ADMIN_TOKEN=.*' \
  | sed 's/^/export /' >> "$DEMO/env.sh"
. "$DEMO/env.sh"; HUBTASK_HUBCTL="$DEMO/bin/hubctl" scripts/seed-demo.sh --slug nordlicht
```

Open **http://nordlicht.localhost:8080/** (in multi mode a workspace is its subdomain) and sign in
with the generated `HUBTASK_DEMO_PASSWORD` (`grep DEMO_PASSWORD "$DEMO/env.sh"`), one address per
role:

| Role | Sign-in |
|---|---|
| Owner: every product, the administration included | `owner@nordlicht.example.org` |
| Administrator; a change waits for him and Priya in the CAB's approvals | `lukas.brandt@nordlicht.example.org` |
| Member | `amelia.hughes@nordlicht.example.org` (also `mehmet.yilmaz@`, `sofia.rossi@`, `priya.natarajan@`) |
| Contributor, viewer, auditor | `jonas.weber@`, `clara.neumann@`, `thomas.becker@nordlicht.example.org` |
| Portal requester | `hannah.albers@stadtwerke-lueneburg.example` (also `felix.hartmann@` there, `grace.oneill@nordwind-logistics.example`) |

The installation's operator is `owner@example.org` of the workspace `demo`
(http://demo.localhost:8080/), with the same password. Stop it all with
`kill $(cat "$DEMO/server.pid")` and
`docker compose -p hubtask-demo -f deploy/docker/compose.dev.yaml down` (`-v` for a fresh
database), and wait until the process is gone before starting the next one (below).

One server per database: two both run the worker, and a job landing on the one whose local storage
lacks the bytes fails with `media.object_missing`. Stop the old server before starting a rebuilt
one — it keeps the port, the new one dies on `ops: bind`, and the page still answers from the old
binary.

**Scripting a walk.** Playwright lives in `apps/webapp/node_modules`, with the three engines. A
probe is a temporary file under `apps/webapp/e2e/` that is deleted afterwards.

- Load each route in a fresh page and read `document.activeElement` before every key press; press
  only what the script named. A keystroke on an unknown focus is a write nobody asked for.
- Seed a throwaway account for the walk, never the owner's.
- One browser context per width, signed in per context: a credential holds a capped number of open
  streams, and closed contexts' streams linger, so `/stream` answers 503 to the thirtieth.
- Attach request listeners before `page.goto`, or the first burst is not counted.
- After signing in, wait for `nav[aria-label="Workspace"]`, not `header`: the header is drawn signed
  out too, and a `goto` before the session has landed throws it away.
- A faithful "before" screenshot is the current build with the old declaration or markup injected
  (read the element's scoped class first), measured rather than eyeballed.
- Measure a size before describing it, in a test or in prose: read `getBoundingClientRect`, not the
  screenshot.
- Two one-time codes in one 30-second window: the second is refused as a replay. Wait for the next
  window (`scripts/hubctl-e2e.sh` shows how).
- An editor's built-in browser pane may never run animation frames or turn Enter into a click;
  check motion, focus rings and key activation with Playwright.

**Screenshots are in English**, the repository's language; another language is proven by a test,
not a picture (the owner, 2026-10-12).

**Recording it.** A walk is not committed — neither a file nor its screenshots. It is shown to the
owner in the running app, or by screenshots in the conversation or the pull request; what it found
is fixed on the branch (AGENTS.md, "Working rules"), and what a use case check needs is asserted by
a test, an engine walk (`apps/webapp/e2e/*.test.mjs`) for a screen. A file here is written only for
a run no pipeline can make (above): what was run, at which commit, what was seen, what was found.
