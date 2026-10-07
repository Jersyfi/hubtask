# Hubtask

Task management with five levels — **Hub → Collection → Task → Work Package → Activity** —
for private individuals and for service providers. Backend first, API first, AI first.
Go, PostgreSQL, hexagonal architecture.

* **Self-hosting** with Docker/Podman: two containers, the full feature set, no limitations.
* **Platform operation** with Kubernetes: the same image, multi-tenant, horizontally scalable.
* **Automatable** through a REST API, webhooks, n8n/Zapier, and an internal rule engine.
* **Multilingual** without backend changes; any language, time zone, and text direction.
* **Agent-ready** through an MCP server; AI features are optional and can be switched off.
* **Secure by default** — the tenant boundary is enforced in the database, twelve security gates run in the pipeline.
* **Self-diagnosing** — degrades in a controlled way instead of crashing, and reports what it is missing through `/meta/health`.
* **Auditable and GDPR-ready** — a chained, content-free audit log; data subject rights as use cases with deadline tracking.
* **Freely backed up** — choose the schedule and the target yourself (S3, SFTP, FTP, WebDAV, cloud, local), encrypted, with retention and restore down to item level.
* **Usable offline** — clients keep working without a network; concurrent changes by others are not lost when merging.

---

## Documentation

The architecture is fully documented. [`docs/README.md`](./docs/README.md) says where each kind of
knowledge lives; start with the [vision](./docs/vision/README.md) (why), the
[use cases](./docs/usecases/README.md) (what must be true for a person) and
[arc42](./docs/architecture/arc42.md) (how it is built). [`AGENTS.md`](./AGENTS.md) is how to work
here, for a person or a coding agent.

---

## The architecture in one paragraph

One Go module, one container image, several process roles (`api`, `worker`, `scheduler`,
`automation`). The core (`core/`) is technology-free and knows only ports; REST, MCP, calendar,
mail, and webhooks are adapters. PostgreSQL is the only mandatory dependency and handles storage,
the job queue, the event outbox, full-text search, and tenant isolation (row level security).
Instead of four specialised level entities there is one generalised `WorkItem` aggregate root with
capability profiles — new levels and fields are configuration, not a migration.

The same repository also holds both first-party clients and the design system
([ADR-0027](./docs/adr/ADR-0027-monorepo-structure.md)): the web UI under `apps/webapp` is an
inbound adapter like REST or MCP and ships inside the binary
([ADR-0028](./docs/adr/ADR-0028-embedded-web-ui.md)), and the project website lives under
`apps/website`.

---

## Quick start

```bash
git clone https://github.com/Jersyfi/hubtask.git && cd hubtask
cp deploy/docker/.env.example .env      # set the secrets
docker compose -f deploy/docker/compose.yaml up -d
# API:  http://localhost:8080/api/v1
# Meta: http://localhost:8080/api/v1/meta/capabilities
```

Kubernetes:

```bash
helm upgrade --install hubtask ./k8s -f ./k8s/values.yaml
```

---

## The command line client

`hubctl` signs in with a personal access token and speaks the published contract — its types are
generated from `api/openapi.yaml`. Every command prints a table for a person, or, with `--json`,
the API's own payload for a pipe: exactly one document on standard output and every diagnostic on
standard error.

```bash
make build                                   # bin/hubctl
echo "$TOKEN" | bin/hubctl auth login --url http://localhost:8080
HUB=$(bin/hubctl container create --type HUB --name "Personal" | awk 'NR==2 {print $1}')
bin/hubctl container create --type COLLECTION --parent "$HUB" --name "Errands"
bin/hubctl item create --collection "$COLLECTION" --type TASK --title "Buy milk"
bin/hubctl item complete "$ITEM"
bin/hubctl comment add "$ITEM" --body "Done on the way home"
bin/hubctl search milk
bin/hubctl trash ls
bin/hubctl watch                             # follow the change stream; Ctrl-C ends it
bin/hubctl sync pull --all > state.jsonl     # the offline synchronisation, as a device: the cursor last
bin/hubctl sync push --file queue.jsonl      # one mutation per line; one result per line back
bin/hubctl sync devices ls                   # and `sync devices forget <id>`
bin/hubctl sync snapshot --out state.ndjson --apply   # the initial synchronisation as one stream; the cursor is the last line
bin/hubctl import trello board.json --hub "$HUB" --wait 5m   # also csv, google-tasks, microsoft-todo; the report on the run

bin/hubctl due set "$ITEM" --at 2026-09-10   # a day; a timestamp is a moment instead
bin/hubctl remind add "$ITEM" --at -PT30M    # half an hour before it is due
bin/hubctl recur set "$ITEM" --rule "FREQ=WEEKLY;BYDAY=MO" --zone Europe/Berlin
bin/hubctl template instantiate "$TEMPLATE" --collection "$COLLECTION" --anchor 2026-09-07
bin/hubctl view export "$VIEW" --format ICS --out week.ics
bin/hubctl calendar mint --view "$VIEW"      # prints the feed URL once; it is the credential
```

The automation surface, and the inbox it can act on:

```bash
RULE=$(bin/hubctl rule add --name "mail becomes a task" --trigger JUMBLE_ENTRY \
  --run-as "$ACCOUNT" \
  --action 'CONVERT_JUMBLE_ENTRY:{"collection_id":"'"$COLLECTION"'"}' | awk 'NR==2 {print $1}')
bin/hubctl rule test "$RULE" --event de.hubtask.jumble.entry.received.v1   # the dry run
bin/hubctl rule enable "$RULE"               # a rule is written switched off, always
bin/hubctl rule runs --rule "$RULE"          # what it has done, newest first
bin/hubctl rule run show "$RUN"              # one run, step by step

bin/hubctl jumble intake rotate-token        # the address the inbox accepts deliveries on, once
bin/hubctl jumble ls --status NEW
bin/hubctl jumble convert "$ENTRY" --collection "$COLLECTION"
bin/hubctl jumble dismiss "$ENTRY"           # a state, not a deletion

WEBHOOK=$(bin/hubctl webhook add --url https://example.org/hooks \
  --event de.hubtask.work.item.completed.v1 | awk 'NR==2 {print $1}')
bin/hubctl webhook deliveries "$WEBHOOK" --status FAILED
bin/hubctl webhook replay "$WEBHOOK" "$DELIVERY"   # the same event id, so a repeat is recognisable
bin/hubctl webhook rotate-secret "$WEBHOOK" --grace 0   # 0 retires the old one at once

bin/hubctl events poll de.hubtask.work.item.completed.v1 --since "$CURSOR"
```

What an operator does with an installation is the same client:

```bash
TARGET=$(bin/hubctl backup target add --name nightly --kind LOCAL --config path=daily | awk 'NR==2 {print $1}')
bin/hubctl backup target test "$TARGET"
bin/hubctl backup run --target "$TARGET" --follow --wait 30m
bin/hubctl backup ls --target "$TARGET"       # what is actually lying at the target
bin/hubctl backup verify "$RUN" --follow      # fails the command if it does not verify
bin/hubctl backup schedule set "$SCHEDULE" --trial   # every FULL run is followed by an INSPECT of its own archive

bin/hubctl restore inspect --target "$TARGET" --archive "$ARCHIVE"
bin/hubctl restore run --target "$TARGET" --archive "$ARCHIVE" --mode NEW_TENANT --apply
                                              # a dry run with a report until --apply is typed
bin/hubctl retention add --kind COMPLETED_ITEM --days 90 --action TRASH
bin/hubctl retention preview "$POLICY"        # what it would take, and what stands in the way
bin/hubctl hold place --scope CONTAINER --id "$COLLECTION" --reason "the Meier proceedings"
bin/hubctl audit query --from 2026-08-01 --action auth.
bin/hubctl audit verify --from 2026-08-01 --to 2026-09-01   # red when the chain does not hold
bin/hubctl dsr create --kind ERASURE --subject "$ACCOUNT"
bin/hubctl dsr start "$CASE" --mode ANONYMIZE # the transition that runs the work
bin/hubctl job show "$JOB" --follow
```

Errors are the message catalogue's sentences rather than the problem document behind them — the
server emits codes, never display text ([ADR-0011](docs/adr/ADR-0011-i18n-message-codes.md)).
Which platforms the binary is supported on is in
[support-matrix.md](docs/architecture/support-matrix.md) §4.

---

## Technology

| Area | Choice |
|---|---|
| Language | Go (≥ 1.27), `net/http`, `log/slog` |
| Database | PostgreSQL 16+ (`pgx/v5`, `sqlc`, `goose`) |
| API | OpenAPI 3.1 spec-first (`oapi-codegen`), RFC 9457, cursor pagination |
| Events | Transactional outbox, CloudEvents 1.0, optionally NATS JetStream |
| Automation | Declarative rules, conditions in CEL (`cel-go`) |
| Scheduling | RFC 5545 RRULE (`rrule-go`), PostgreSQL job queue |
| AI | MCP server; AI port with adapters for OpenAI-compatible APIs and Ollama |
| Object storage | S3 or any S3-compatible service, or a local volume |
| Observability | OpenTelemetry, Prometheus; dashboards, alert rules, and runbooks included |
| Resilience | Circuit breakers, bulkheads, load shedding, dead letter, idempotency throughout |
| Backup | Own logical archive format (JSON Lines + manifest), AES-256-GCM, adapters for S3/SFTP/FTPS/WebDAV/SMB/Azure/GCS/rclone/local |
| Offline sync | Delta sync via a change log, hybrid logical clocks, OR-sets, fractional indices |
| Clients | Svelte 5 + TypeScript; the webapp as a plain Vite SPA, served from inside the binary; Tauri 2 shells for desktop and mobile planned |
| Design system | Design tokens (W3C DTCG, Style Dictionary) as the single origin of every visual value; pnpm workspace |
| Deployment | Docker/Podman Compose, Helm for Kubernetes |

---

## Licence

Hubtask is open source under the **[Apache License 2.0](./LICENSE)** — free for any use, commercial
included, with the full feature set and no licence key
([ADR-0080](./docs/adr/ADR-0080-hubtask-is-apache-2-0.md),
[licensing-editions.md](./docs/architecture/licensing-editions.md)). The name and the logo are
covered by [TRADEMARK.md](./TRADEMARK.md), not by the licence. Donations fund the work — see
[the funding section](./docs/architecture/licensing-editions.md#5-funding).

---

## Contributing

Conventional Commits, trunk-based development, Definition of Ready/Done from
[engineering-guidelines.md](./docs/architecture/engineering-guidelines.md).
Architectural changes arrive as an ADR, not as a pull request without context.
See [CONTRIBUTING.md](./CONTRIBUTING.md), [SECURITY.md](./SECURITY.md),
and [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).
