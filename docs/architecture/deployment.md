# Deployment

How Hubtask reaches the world — for our own operation and for self-hosters. The pipeline that
builds what is deployed is [ci-cd.md](./ci-cd.md); the strategy decisions are
[ADR-0023](../adr/ADR-0023-deployment-strategy.md) and
[ADR-0046](../adr/ADR-0046-production-on-a-platform-namespace.md).

---

## 1. Artefacts

A release produces four things, all under the same version
([versioning-release.md](./versioning-release.md) §1):

| Artefact | Location | Purpose |
|---|---|---|
| Container image (multi-arch, amd64 + arm64) | `ghcr.io/jersyfi/hubtask:vX.Y.Z` | Every mode of operation |
| Helm chart (OCI) | `oci://ghcr.io/jersyfi/charts/hubtask` | Kubernetes |
| SBOM (CycloneDX), keyless signature, provenance | Attached to the image; the SBOM also on the release | Supply chain, verifiability |
| GitHub release | Repository | Release notes, the SBOM, the packaged chart, `THIRD-PARTY-LICENSES.md` |

The chart's `image.repository` and the Compose file's `HUBTASK_IMAGE` make a mirror of the GitHub
Container Registry possible. `latest` exists for trying Hubtask out and is not for production; the
Compose file reads the tag from `HUBTASK_VERSION` and refuses to start without one, so a self-hoster
always runs a release they chose. Each GitHub release attaches `compose.tar.gz`: the Compose file
and a `.env.example` that already names that release.

---

Which runtimes, architectures and PostgreSQL majors are supported, and the CI job that proves each,
is [support-matrix.md](./support-matrix.md).

---

## 2. Modes of operation

One image, several roles ([ADR-0014](../adr/ADR-0014-single-image-multi-role.md)): one binary,
distroless, non-root, read-only root filesystem. `HUBTASK_ROLES` chooses which of `api`, `worker`,
`scheduler` and `automation` a process runs (default: all four). The image also carries the
migrator (`hubtask-migrate`) and the restore drill.

### 2.1 Self-hosting (Docker/Podman)

The reference is `deploy/docker/compose.yaml`: the database (`pgvector/pgvector:0.8.7-pg16`, pinned by digest and moved by Dependabot, so semantic
search is available), a one-shot migration service, and the application. The database is not
published; the application runs `read_only` with `no-new-privileges`, has volumes for media and
backups, and starts only after the migration ended with `service_completed_successfully`.

**The first workspace and its owner** are created in the browser
([UC-INS-01](../usecases/admin/UC-INS-01-start-a-fresh-installation.md)): at its first start the
installation prints a one-time setup code (`docker compose logs app`), and the *Set up Hubtask* card
at the installation's address takes it, the workspace's name and the owner's address and password -
no database shell. The code lasts thirty minutes; `docker compose restart app` prints a new one while
nothing is set up. Operators can also be named in the environment (`HUBTASK_OPERATORS`), and an
installation nobody can run any more gets one back on the machine:
`docker compose exec app hubtask operator add --workspace <short name> --email <address>`
([UC-INS-03](../usecases/admin/UC-INS-03-get-back-into-a-locked-installation.md)); a workspace
suspended or set for deletion with every operator inside it comes back with
`hubtask workspace cancel-deletion --workspace <short name>` and
`hubtask workspace resume --workspace <short name>`. A person who forgot their password where no
mail goes out gets the reset link printed on the machine:
`hubtask account reset-link --workspace <short name> --email <address>` - the reset mail's own
one-time link (only its hash stored, thirty minutes, a newer one replaces it), journalled as the
machine's (`instance.reset_link_issued`). Installation
settings can be seeded or enforced from a file (`HUBTASK_INSTANCE_FILE`, §6.1).

The application connects as `hubtask_app` — created by the migration without `SUPERUSER` or
`BYPASSRLS` — never as the database owner, so row level security is the last boundary in
self-hosting too. The migrator grants that role its login from `HUBTASK_DB_APP_PASSWORD`; the
migration itself never carries a credential (§6.2).

**Memory.** `compose.yaml` sets no memory limit. Whoever sets one on the `app` service sets
`GOMEMLIMIT` beside it, at about 90 % of the limit (`mem_limit: 1g`, `GOMEMLIMIT: 921MiB`): the Go
runtime does not see the container's limit, and without a target of its own it lets the heap grow
until the kernel kills the process rather than collecting harder as it nears the limit
([observability-reliability.md](./observability-reliability.md) §6).

The operations port is published on loopback only (`127.0.0.1:9090`): metrics and the health report
— `curl localhost:9090/readyz` after an update, a Prometheus on the same host — do not belong on the
network ([observability-reliability.md](./observability-reliability.md) §3.2).

**The system backup is the self-hoster's own.** Without an operator for continuous archiving, the
stack offers a documented `pg_dump` and the tenant archives beside it;
[backup-restore.md §8.6](./backup-restore.md#86-the-minimal-path-a-dump-and-what-it-does-not-give)
says which guarantees that does not carry.

**A reverse proxy in front** terminates TLS and must pass three things through to port 8080: the
WebSocket upgrade of `/api/v1/realtime/` (pages and boards are edited live over it), the server-sent
events of `/api/v1/stream` unbuffered, and long-lived connections on both. nginx forwards neither
`Upgrade` nor `Connection` by default; its location for the application needs
`proxy_http_version 1.1;`, `proxy_set_header Upgrade $http_upgrade;`,
`proxy_set_header Connection $connection_upgrade;` (with the usual `map $http_upgrade
$connection_upgrade { default upgrade; '' close; }`), `proxy_buffering off;` and a
`proxy_read_timeout` of some minutes. Without the upgrade everything else works and every page and
board opens read-only; after three connections that never opened while the event stream is live,
the editor names the proxy as the likely cause.

Updating: raise `HUBTASK_VERSION` in `.env`, then `docker compose pull && docker compose up -d`.
Before the migration runs a release this database has not run yet, `upgrade-dump` dumps the
database into the `upgrade-dumps` volume and keeps the last `HUBTASK_UPGRADE_DUMPS_KEPT` (3); a dump
that fails stops the upgrade before any migration ran, so an upgrade never starts without a
recovery point (`HUBTASK_UPGRADE_DUMP=false` for a database backed up otherwise; backup-restore.md
§8.6). The migration runs before the application starts; because migrations are expand/contract-safe
([versioning-release.md](./versioning-release.md) §4), the old version may still be running while
it does.

**Rolling back on Compose.** Write down the release you ran before raising `HUBTASK_VERSION` -
the newest upgrade dump's name carries the release it was taken before. Within one minor (the
schema stays compatible with the previous minor, versioning-release.md §4), set
`HUBTASK_VERSION` back and run `docker compose up -d`: the migrator has nothing to apply, and the
older application runs on the newer schema. Across a contract migration - more than one minor
back, or a release whose notes name one - the older binary refuses to start
(`config.binary_older_than_schema`, §5) and the way back is the dump taken before the upgrade:
set `HUBTASK_VERSION` back, then `./restore.sh --upgrade-dump <file>` (backup-restore.md §8.6).
Writes made on the newer release since are lost with it.

### 2.2 Kubernetes

Four deployments from **one** image, distinguished by `HUBTASK_ROLES`:

| Deployment | Role | Particularity |
|---|---|---|
| `api` | `api` | Behind a service/ingress; HPA available (`roles.api.autoscaling`); its own load-shedding threshold |
| `worker` | `worker` | Jobs, outbox delivery, backup runs |
| `scheduler` | `scheduler` | Two replicas, one active (advisory lock leader, [observability-reliability.md](./observability-reliability.md) §15) |
| `automation` | `automation` | Its own pool — a rule storm must not starve the interactive path. It alone claims the rule jobs (`automation.run`, `.http`, `.schedule`, `.check`); a `worker` without the role claims every other kind, a process holding both claims all. With `roles.automation.enabled: false` the chart gives the worker both roles |

**Each role's memory limit sets its `GOMEMLIMIT`** to 90 % of it, in bytes
(`roles.<role>.resources.limits.memory`), so the runtime collects harder before the kernel kills the
pod; a role without a limit gets none, and a limit the chart cannot read refuses to render.

**The migration** runs as a Job before the rollout: a Helm hook (`pre-install,pre-upgrade`) and,
under Argo CD, a `Sync`-phase hook in a sync wave after the database, so that a chart which also
renders its CloudNativePG `Cluster` (§3.2) migrates a database that exists. The migrator holds an
advisory lock for the run, so two migrators never run at once.

* **The migration's DSN is its own** (`migration.dsnSecretKey`, optionally in
  `migration.dsnSecretName`): the application connects as `hubtask_app`, the migration as the owner.
  With `database.enabled` that is the operator-generated `<database.name>-app` Secret's `uri`, never
  copied by hand. `migration.appPasswordSecretKey` grants `hubtask_app` its login after migrating.
* **The migrator waits for the database** (`migration.connectWait`, `HUBTASK_DB_CONNECT_WAIT`)
  rather than trusting the sync wave.
* **The run's bound is the chart's** (`migration.timeout`, `HUBTASK_MIGRATE_TIMEOUT`): a
  `helm upgrade --timeout` shorter than it kills a migration the migrator would have finished.
* **A migration whose number is lower than one already applied is applied late**
  ([versioning-release.md](./versioning-release.md) §4).

---

## 3. Environments

| Environment | Trigger | Approval | Purpose |
|---|---|---|---|
| **local** | `make run` or Compose | — | Development |
| **integration** | Every push to `main` (`deploy.yml`) | Automatic | Dogfooding, load tests, migration rehearsals |
| **production** | A tag bump in the operator's Argo CD Application — the cluster pulls | The GitHub environment `production` approves publishing the version; the deploy is the operator's | Real operation |

The approval on the GitHub `production` environment means an accidental tag publishes nothing
without a human, and no AI path reaches a release ([ADR-0022](../adr/ADR-0022-github-platform.md)).
It releases an image and a chart; **it deploys nothing** — production pulls what was published (§4).

### 3.1 Where `integration` runs

| | |
|---|---|
| Host | One Hetzner vServer, 4 vCPU / 8 GB / 75 GB, Ubuntu 26.04 LTS, amd64 |
| Kubernetes | k3s, single node; Traefik, the ingress k3s ships |
| TLS | cert-manager with Let's Encrypt, `HTTP-01`, one certificate per host |
| Database | In production's shape: CloudNativePG with one instance, the WAL archive to an object-store bucket and the restore drill as a CronJob — smaller in size, same in shape ([ADR-0082](../adr/ADR-0082-integration-in-production-shape.md)). Built in the repository (`deploy/integration/`); applied to the server by whoever holds its access, whose first drill record under `docs/evidence/` says it runs |
| Mail | A catcher in the namespace, over STARTTLS behind a CA the cluster issued to itself: production's path, and nothing leaves the node |
| Host names | `<service>.<environment>.hubtask.eu` (`api.integration.hubtask.eu`); a workspace is a subdomain of it. A wildcard record covers the environment |
| Deploy | `deploy.yml`: builds `:main-<sha>`, signs it under its own identity, verifies it, then `helm upgrade` without `--atomic` — a failed rollout leaves the previous pods serving and the failed Job's logs readable |
| Monitoring | Prometheus and Alertmanager in the cluster ([observability-reliability.md](./observability-reliability.md) §13.1) |

The operations port is never routed. A single node proves the chart, the migration hook and the
rolling update, and the database's shape proves the operator's path, the roles, the migration as a
non-superuser and the restore before production meets them; it rehearses no node failure, and the
database shares a disk with the workload — neither may be carried into production.

### 3.2 Where `production` runs

Decided in [ADR-0046](../adr/ADR-0046-production-on-a-platform-namespace.md).

| | |
|---|---|
| Cluster | A private k3s cluster operated by a platform, not by this project |
| Namespace | One. Nothing this project deploys is cluster-scoped; the Argo `AppProject` admits that namespace and a fixed list of kinds |
| Deploy identity | **None in GitHub.** Argo CD pulls this chart at a pinned tag; no kubeconfig, token or endpoint of production exists in this repository |
| Ingress | The platform's. TLS is a platform-issued wildcard, referenced by Secret *name* from values; the chart creates no `Certificate` |
| Host name | One, reachable through the VPN only, set by the operator and not written here. The operations port is not routed |
| Database | PostgreSQL through the platform's CloudNativePG operator; the `Cluster` is a template of our chart (`database.enabled`), in our namespace |
| System backups | CNPG's continuous WAL archiving and a daily base backup to object storage with Object Lock, plus the platform's volume snapshots as a second net |
| Media | An S3 bucket of its own, separate from the backup bucket |
| Monitoring | The platform's Prometheus Operator scrapes our `ServiceMonitor`s and evaluates our `PrometheusRule`s; alerts route by namespace; dashboards ship as ConfigMaps for its Grafana |
| Deployed by | A tag bump in the operator's Argo CD Application (this chart at the tag, its values file in the private repository) |
| Image policy | The chart can render a Kyverno `ClusterPolicy` (`imagePolicy.enabled`, default off) admitting only images signed by the release or the integration deploy identity; applying it is the platform's decision |

**What we own, and what we never assume.** We own everything inside the namespace — manifests, the
CNPG `Cluster` and its backup stanza, migrations, metrics, rules, dashboards, resources, and
references to Secrets the owner creates — and nothing outside it: no cluster-scoped resource, other
namespace, cluster-admin, or exposure beyond the one host name. The platform runs neither our migrations nor our application-level restores, so
**what must run against production runs from the chart, inside the cluster:** the migration as a
hook in a sync wave, the restore drill as a `PostSync` hook of every release and as a `CronJob`
between them ([backup-restore.md §8.5](./backup-restore.md#85-the-operator-procedure-point-in-time-recovery)).

**Two consequences.** The restore drill can restore only into our own namespace, as a second CNPG
cluster bootstrapped from the object store — so the resource quota bounds the live database to less
than half of what it admits. And the restore runbook must be executable by a person alone.

**Nothing of production is written here:** no host name, bucket, endpoint, quota, label value,
Secret name or credential. Every such value is a values key marked *set by the operator*, listed in
[`deploy/production/PLATFORM-INTERFACE.md`](../../deploy/production/PLATFORM-INTERFACE.md). The RPO
and RTO a drill measures stay internal
([observability-reliability.md](./observability-reliability.md) §13.2).

---

## 4. Push or pull?

**`integration` is push-based; production is pull-based.** Both share the chart and differ only in
who runs `helm`.

* **`integration`:** `deploy.yml` runs `helm upgrade` with a deploy identity that cannot create
  namespaces (`KUBE_CONFIG`, environment `integration`), after verifying the image (§3.1). One
  cluster and one operator make push the simpler path to understand and debug.
* **Production:** Argo CD renders this chart at a pinned tag with a values file in the operator's
  private repository; a tag bump there is the deploy. Nothing in this repository's workflows can
  reach that cluster.

---

## 5. Rollout safety

| Measure | Effect |
|---|---|
| `maxUnavailable: 0`, `maxSurge: 1` | No capacity loss during the rollout |
| Readiness gate + a PodDisruptionBudget per role (`minAvailable: 1`) | No pod disappears before its replacement is ready |
| Migration before the rollout, expand/contract | The old and new versions run simultaneously without harm |
| Migration versions watched across pods | `hubtask_migration_version` per pod; alert A-13 fires when pods disagree for more than 15 minutes |
| Readiness compares the schema | A pod is ready only when every migration it embeds is applied (`/readyz` answers `migration:behind` otherwise, and the health report's `migration` says `behind`); a database migrated further is `ahead` and fine — the expand step of the next release — unless it passed a contract step this binary predates: its compatibility floor (`schema_compatibility_floor()`, raised by each contract migration) above the binary's newest migration answers `migration:too_old`, the report's `migration` says `too_old` with `compatible_from`, and the binary refuses to start (`config.binary_older_than_schema`). Read through `applied_schema_versions()`, numbers only; the ledger stays closed to the application role. `/startupz` does not compare: a failing startup probe restarts a pod that is only waiting for its migration. Behind, a worker claims no job and a scheduler neither takes nor keeps leadership, until the migration has run |
| Readiness survives an overload | `/readyz` fails on PostgreSQL only when it is down: refused, unauthenticated or a failed round trip. A request pool whose every connection is in use for a second is `postgres.pool_busy`, degraded in the health report and still ready — an overload is the load shedder's and A-11's to answer, not a reason to take every pod out of the Service at once |
| Graceful shutdown | On `SIGTERM` the process marks itself not ready, keeps serving for `HUBTASK_SHUTDOWN_DEREGISTER_SECONDS` while it asks its open streams to end in ten waves spread over that wait (so their clients do not reconnect elsewhere all at once), then drains in-flight requests within `HUBTASK_SHUTDOWN_GRACE_SECONDS` and releases job leases |
| The previous release's interface assets in the image | The release build copies the previous release image's `/app/ui/assets` into `/app/ui-previous/assets`, and each image keeps its own at `/app/ui/assets` for the next. A content-hashed file the embedded bundle lacks is served from there, immutable like the rest, so a tab still running the previous release that asks a new pod for one of its chunks gets it instead of a 404. Nothing else is taken from it. A chunk of the new release asked of a pod still on the previous one is answered by the build comparison of api-guidelines.md §8; the first release to write `/app/ui/assets` carries no previous assets |
| `terminationGracePeriodSeconds` (120) ≥ the job timeout plus the drain budget | No job is cut off mid-work |

**Rollback:** deploy the previous chart version — `helm rollback` for `integration`, the previous
tag in the Argo CD Application for production. That works only because the schema stays backwards
compatible for at least one minor version; a rollback across a contract migration needs a restore
from backup ([backup-restore.md](./backup-restore.md)), and the older binary refuses to start rather
than serve on a schema it cannot use.

---

## 6. Configuration

Exclusively `HUBTASK_*` environment variables (12-factor), behind `core/port/environment/Port.go`.
The server also reads `HUBTASK_DB_DSN`, `HUBTASK_SECRET_KEY`, `HUBTASK_S3_ACCESS_KEY`,
`HUBTASK_S3_SECRET_KEY`, `HUBTASK_SMTP_PASSWORD`, `HUBTASK_MAIL_INBOUND_KEY`, each
`HUBTASK_MAIL_OAUTH_<PROVIDER>_CLIENT_SECRET` and each `HUBTASK_ENCRYPTION_KEY_<ID>` from
`<NAME>_FILE`, and the migrator its two secrets (§6.2), so Docker and Kubernetes secrets work
without the detour through the environment.

There is **no default value for a secret**. If one is missing, the process does not start and says
why: a generated key would make everything encrypted with it unreadable after a restart.

A configuration error names its variable through a message code (`config.db_dsn_missing`), and all
problems are reported at once. Durations are Go syntax (`30s`, `5m`, `1h30m`); a bare number is
rejected rather than guessed at.

### 6.1 Reference

Every variable, its default and its validation are in
`infrastructure/environment/EnvConfig.go`; what each means is documented on its field in
`core/port/environment/Port.go`. `deploy/docker/.env.example` is the self-hosting starting point.
Compose reads `.env` only to fill `compose.yaml`, which hands the application the variables it
names and no others; every variable the example offers is passed through and read by the
configuration, and every variable the table below names is passed through or listed in
`test/architecture` as one the reference stack leaves at its default (tuning, mostly). Such a
variable reaches the application only once it is added to `compose.yaml` too. The instance file
needs its mount as well: the commented `instance.json` line under the application's volumes.

A malformed instance file refuses the start (the table below), so a changed file is checked before
the restart: `hubtask instance check [file]` runs every reader the start runs - the level, the
plans, the presets, the AI models with the variables they name, the platform subscriptions with
their secret files, the upload scanner and the business calendars - without a database, and prints
each refusal (exit 1). It runs where the server runs, so the variables and files are the server's:
`docker compose run --rm --no-deps app instance check` before `docker compose up -d`. The chart
runs it as a pre-upgrade hook (`instanceCheck.enabled`, on where an instance file is named), so an
upgrade stops with the refusal in the hook's log while the running pods keep serving. What only the
database can judge (a plan against the installation's values) is still the start's to refuse.

Required, with no default: `HUBTASK_DB_DSN` (the connection as `hubtask_app`) and
`HUBTASK_SECRET_KEY` — the installation secret, at least 32 characters. It peppers stored
credential hashes, signs cursors, mints media and feed tokens and keys the audit trail's
fingerprints of sensitive values, every purpose derived through its own label
([security.md](./security.md) §5, §8); it is **not** the key backups and stored credentials are
encrypted with, which is the keyring below. Changing it ends every session and token, and the audit
fingerprints written before and after the change no longer compare ([audit.md](./audit.md) §4).

Every variable the server's configuration reads has a row below; `make gate-architecture` fails on
one that has none, and on a row naming a variable no code reads:

| Variable | Default | Rule |
|---|---|---|
| `HUBTASK_ROLES`, `HUBTASK_TENANCY_MODE` | all four roles, `single` | §2; `multi` is provider operation ([multi-tenancy.md](./multi-tenancy.md)) |
| `HUBTASK_HTTP_ADDR`, `HUBTASK_OPS_ADDR` | `:8080` / `:9090` | The API and interface listener / the operations port - health, readiness and metrics, never public (§3) |
| `HUBTASK_LOG_FORMAT`, `HUBTASK_LOG_LEVEL` | `json`, `info` | `text` for reading by eye; `debug`, `warn` or `error`. Anything else refuses the start (`config.log_format_invalid`, `config.log_level_invalid`) |
| `HUBTASK_WORKSPACE_HOSTS` | `subdomain` | In multi mode, `subdomain` puts each workspace at `<short name>.<base host>` behind a wildcard; `assigned` gives each one a host the operator names when it is created - *New workspace*, `hubctl admin tenant create --host`, `POST /admin/tenants` and the first start's setup ask for it, refuse the installation's own host, one another workspace uses and what is no host name, and check no DNS; the short name is then made from the host where none is given, and a start writes no subdomain rows. The record and the certificate are the operator's. An installation with workspaces keeps its choice: switching is not offered (UC-INS-20). *Built:* `assigned`. Beside either, a workspace's owner may set up an own domain where the installation allows it (`owners_own_domains`); its records, its certificate and the ingress that asks are below the table ([ADR-0095](../adr/ADR-0095-a-workspaces-address-resolves-through-its-hosts.md)) |
| `HUBTASK_INSTANCE_FILE`, `_MODE` | — / `seed` | A file of installation settings ([ADR-0070](../adr/ADR-0070-the-instance-layer.md)). `seed` writes it once, at first start; `enforce` writes it at every start, and the routes that would change those settings refuse. One source per mode, never two. Its `ai_models` list offers AI models to every workspace ([ADR-0072](../adr/ADR-0072-ai-at-the-installation-level.md)), each key named by an environment variable (`api_key_env`) and never written in the file; `seed` offers them where none is offered, `enforce` makes the file the list. Its `plans` list holds plans in the API's shape ([multi-tenancy.md](./multi-tenancy.md) §4.2), written after the level: `seed` where no plan exists, `enforce` at every start, removing a plan the file no longer names unless a workspace is on it, and leaving a plan unchanged untouched. `enforce` closes the API's plan writes whether or not the file names plans; single mode skips them. Its `presets` list holds the installation's presets in the API's shape (`name`, `preset`, `settings`, `is_default`, `locks`; UC-INS-18), known by name and written after the level: `seed` where the installation has none, `enforce` at every start as the whole set, the default and its locks included, leaving an unchanged one untouched; `enforce` closes the API's preset writes; single mode skips them. Its `platform_subscriptions` list (`target_url`, `secret_file`) subscribes a purchase platform to the installation's events ([multi-tenancy.md](./multi-tenancy.md) §4.3), each secret read from the file it names: `seed` writes them where no subscription stands yet and leaves them the operator's, `enforce` makes the file the set at every start, rotates a secret only where the file's changed, and closes the API's subscription writes - only where the file names subscriptions; single mode skips them. Its `business_calendars` list holds the installation's business calendars in the shape `PUT /calendars/{key}` takes (`key`, `name`, `zone`, `hours`, `holidays`; UC-SVC-01), read at every start in either mode and written nowhere: a plan or capacity naming a key a workspace holds no calendar under counts on the installation's, a workspace's of the same key stands in front of it, and a malformed one or a key named twice refuses the start. Its `upload_scanner` (`address`, optional `timeout`) names the ClamAV daemon uploads are scanned by ([security.md](./security.md) §4), read at every start in either mode; `HUBTASK_CLAMD_ADDRESS` and `_TIMEOUT` override it, and a malformed block refuses the start |
| `HUBTASK_BASE_URL` | — | Absolute URL of the installation; without it links in mails and feeds are wrong (warning `config.base_url_missing`), and a host only this machine reaches - `localhost`, a loopback address - warns `config.base_url_loopback`. The Compose stack refuses to start without it. In multi mode its host is, beside the workspaces' own, the one host a request may use: any other names no workspace ([multi-tenancy.md](./multi-tenancy.md) §3) |
| `HUBTASK_OPERATORS` | — | Operator accounts named at start, `workspace:address` comma-separated (a bare address in single mode); adds, never removes ([identity.md](./identity.md) §19). A malformed entry refuses the start (`config.operators_invalid`) |
| `HUBTASK_UI_ENABLED` | `true` | `false` answers `/` with 404 and leaves the API untouched; the `web_ui` feature in `/meta/capabilities` ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)) |
| `HUBTASK_UI_FRAME_ANCESTORS` | empty | Complete origins, comma-separated, that may show the interface in a frame (a portal embedding it, such as openDesk's); empty keeps `frame-ancestors 'none'`. The interface's policy only, never the API's; a wildcard or anything but an origin is refused at start ([security.md](./security.md) §9) |
| `HUBTASK_UI_FRAME_SOURCES` | empty | Complete https origins, comma-separated, that a page's embed may show in a sandboxed frame (a dashboard, a video); empty frames nothing and draws every embed as a link card. Becomes the interface's `frame-src` only, never the API's, and is answered in `/meta/capabilities` as `frame_sources`; a wildcard, a path or `http` is refused at start with `config.frame_source_invalid` ([security.md](./security.md) §9) |
| `HUBTASK_METRICS_TENANT_LABEL`, `HUBTASK_TRACING_*` | `false`; tracing off, ratio `0.05` | [observability-reliability.md](./observability-reliability.md) §3.2, §3.3; an endpoint is required when tracing is on |
| `HUBTASK_SHUTDOWN_DEREGISTER_SECONDS`, `HUBTASK_SHUTDOWN_GRACE_SECONDS` | `15` / `30` | §5. A load balancer removes a pod asynchronously, so a process that closes its listener at once is still sent requests ([RT-8 evidence](../archive/evidence/RT-8-2026-08-21.md)); `0` is right where nothing routes the traffic |
| `HUBTASK_STEP_UP_WINDOW` | `5m` | How long a step-up grant stays valid for its one action ([identity.md](./identity.md) §16.2) |
| `HUBTASK_ENCRYPTION_KEYS`, `HUBTASK_ENCRYPTION_KEY_<ID>` (`_FILE`) | — | The master keyring, **current first** ([security.md](./security.md) §8.1); each key's material (at least 32 characters) in its own variable, so each can be its own mounted secret. A key named and not supplied fails startup. Empty: the installation starts and refuses to store anything that would have to be sealed, rather than storing it in the clear. The chart sets them from `encryption.keys` (the IDs, current first) and reads each key from `encryption-key-<id>` in `encryption.existingSecret` (`existingSecret` when empty) |
| `HUBTASK_DB_MAX_CONNS` / `_MIN_CONNS` | `10` / `2` | Pool size **per process**; several roles mean several pools |
| `HUBTASK_DB_STATEMENT_TIMEOUT` / `HUBTASK_DB_WORKER_STATEMENT_TIMEOUT` | `5s` / `60s` | Query budget on the interactive path / for background work |
| `HUBTASK_DB_CONNECT_TIMEOUT`, `HUBTASK_DB_MAX_CONN_LIFETIME`, `HUBTASK_DB_MAX_CONN_IDLE_TIME` | `5s`, `1h`, `30m` | Reaching the database once; how long a pooled connection is reused, so that a failover reaches the pool rather than pinning it to a former primary; how long an idle one is kept |
| `HUBTASK_STORAGE_KIND`, `HUBTASK_S3_*` | `local` | With `s3`, the bucket and both keys are mandatory; path style is the default. The interface's policy names the bucket's origin ([ADR-0047](../adr/ADR-0047-media-origin-in-the-interface-policy.md)). **The bucket needs a CORS rule for the upload**, which Hubtask cannot set: allowed origin exactly the interface's origin (never `*`), method `PUT`, header `Content-Type`, no credentials — the presigned URL is the credential. Without it an upload fails in the browser with a message naming the bucket |
| `HUBTASK_STORAGE_LOCAL_PATH` | `/var/lib/hubtask/media` | Where `local` storage keeps the media - **a backup of the installation has to include this volume** (the Compose stack mounts `media` there); empty with `local` refuses the start (`config.storage_path_missing`) |
| `HUBTASK_CLAMD_ADDRESS`, `HUBTASK_CLAMD_TIMEOUT`, `HUBTASK_CLAMD_STREAM_MAX_BYTES` | —, `2m`, `25 MiB` | The ClamAV daemon uploads are scanned by ([security.md](./security.md) §4), overriding the instance file's `upload_scanner`; a scan past the timeout holds the file; with a scanner, uploads are capped to the daemon's stream limit, so no accepted file is one it refuses for size. Empty: no scan |
| `HUBTASK_SMTP_*` | port `587`, `starttls`, `10s` | Without a host, mail degrades (warning); with one, `_FROM` is mandatory |
| `HUBTASK_MAIL_INBOUND_DOMAIN`, `HUBTASK_MAIL_INBOUND_KEY` (`_FILE`) | — | The operator's mail domain that workspaces' addresses live on, and the key its inbound service presents on `POST /mail/inbound` as bearer token or basic password ([automation.md](./automation.md) §5). Empty: the route takes nothing |
| `HUBTASK_MAIL_OAUTH_<PROVIDER>_CLIENT_ID`, `HUBTASK_MAIL_OAUTH_<PROVIDER>_CLIENT_SECRET` (`_FILE`) | — | `GOOGLE` or `MICROSOFT`: the installation's own OAuth apps for connected mailboxes, a provider counted once both are set ([automation.md](./automation.md) §5); without one a workspace connects through an app of its own |
| `HUBTASK_MAILBOX_WATCH_LIMIT` | `1000` | Connected IMAP mailboxes the watching worker of a shard holds open (IDLE), one per workspace before any workspace's second; the rest arrive by the minute's read, counted in `hubtask_mailbox_watches_unheld` (A-38) |
| `HUBTASK_MAILBOX_WATCH_SHARDS` | `1` | How many shards the workspaces are split into for the watch (1-64), by a hash of the workspace; each shard is held by one worker under an advisory lock of its own, which takes one connection of that worker's background pool. One: one worker holds every mailbox |
| `HUBTASK_MAILBOX_WATCH_SHARDS_PER_WORKER` | the shards | How many shards one worker leads at most, so they spread over the workers. Workers times this below the shards leaves shards nobody leads, to the minute's read (A-38, `HubtaskMailboxWatchShardUnled`); with one worker more than the shards need, a stopped worker's shards are taken over |
| `HUBTASK_ALEXA_SKILL_IDS` | — | The Alexa skills this installation publishes for every workspace, comma-separated ([ADR-0093](../adr/ADR-0093-voice-assistants-through-a-skill-of-our-own.md)). Empty: each workspace sets up a skill of its own |
| `HUBTASK_BACKUP_LOCAL_PATH` | `/var/lib/hubtask/backups` | The volume a `local` backup target writes inside; a target's path is relative to it and cannot leave it. Empty: no local targets |
| `HUBTASK_BACKUP_TENANT_TARGETS` | `false` | Lets a tenant choose its own backup target in multi mode — an egress channel the operator did not choose ([backup-restore.md](./backup-restore.md) §2) |
| `HUBTASK_BACKUP_CONCURRENT_DOWNLOADS` | `2` | Owners' workspace downloads one process streams at once; each holds a pool connection and a snapshot for its transfer, so the next is refused with `503` and `Retry-After` ([backup-restore.md](./backup-restore.md) §7). `0` is no bound |
| `HUBTASK_RESTORE_DRILL_RECORD_FILE` | — | The last passed restore drill's Unix timestamp ([backup-restore.md](./backup-restore.md) §8.5), read at every scrape for A-20. The chart mounts the drill's record; Compose points it into its `restore-drill` volume, where the operator copies it after a restore they checked. Empty: the series is absent, not zero; a path that cannot be read is `config.restore_drill_record_unreadable` |
| `HUBTASK_AI_DUPLICATE_THRESHOLD` | `0` (the built-in, measured 0.85) | The duplicate floor ([ai-first.md](./ai-first.md) §2); outside 0…1 is refused at startup |
| *(the workspace's embedding model)* | set per workspace, `hubctl ai … --embedding-model` | Not a variable; its width is an operator concern. The index holds 1536 dimensions ([ADR-0054](../adr/ADR-0054-embedding-width.md)): a narrower model is stored padded; a wider one is refused — before the first text where the provider states its width, else at the first batch, with `ai.embedding_too_wide` in the worker's log — and search stays lexical (`semantic_search: false`, the provider degraded in `/meta/health`). A model re-tagged under the same name keeps its recorded width until the entry goes stale or the process restarts |
| `HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER` | `false` | The operator's, never a workspace's, confirmation of AI processing outside the EEA ([data-protection.md](./data-protection.md) §6) |
| `HUBTASK_RATE_LIMIT_*_PER_MINUTE`, `HUBTASK_RATE_LIMIT_BURST`, `HUBTASK_RATE_LIMIT_*_BURST` | `60` per IP unauthenticated; per token `600` in multi mode, `6000` in single; per tenant `3000` in multi mode, `0` (no workspace wall) in single; `10` for sign-in, reset and invitation; burst `60` per credential; the workspace's own burst (`HUBTASK_RATE_LIMIT_TENANT_BURST`) defaults to half its minute's budget, at least the credential's | Each credential carries its own budget. The burst covers a browser opening a page (a cold first paint is 24 requests); the minute's budget bounds a caller. The workspace's bucket is shared by everyone in it, so its burst is deeper: fifty people opening a cold page together pass |
| `HUBTASK_LOAD_SHED_INFLIGHT`, `_RETRY_AFTER` | `64` / `5s` | Requests in flight above which deferrable work is refused with `503` and `Retry-After` ([observability-reliability.md](./observability-reliability.md) §6). Per role: the chart sets it on `api`. `0` switches shedding off |
| `HUBTASK_STREAM_PER_CREDENTIAL`, `_PER_TENANT`, `_PER_PROCESS` | `8` / `512` in multi mode, the process's cap in single / `1024` | Live connections one process holds - change streams, agents' streams, sign-in snapshots, live documents - per client, per workspace and in all; past a cap a new one is refused with `503` and `Retry-After`, never an open one dropped. Snapshots and live documents hold at most half of the workspace's and the process's share, and a client's are counted apart from its streams. A refusal at the workspace's or the process's cap warns on `/meta/health` (`stream.capacity_reached`) and raises A-26. `0` is no cap |
| `HUBTASK_MAX_BODY_BYTES` / `_UPLOAD_BYTES` / `_MAIL_BYTES`, `HUBTASK_REQUEST_TIMEOUT` | `1 MiB` / `64 MiB` / `25 MiB`, `30s` | Request, upload and mail intake limits (T-17); the deadline every handler inherits but the two held connections, the change stream and a live document |
| `HUBTASK_TRUSTED_PROXIES` | — | Networks (CIDR or address), comma-separated, whose `X-Forwarded-For` / `Forwarded` is believed; the client is the right-most hop outside them. Empty believes no header: behind an ingress every client then shares one rate limit and sessions record the proxy, and `/meta/health` warns (`config.trusted_proxies_missing`) once a forwarded header arrives. The chart sets it from `config.trustedProxies` ([security.md](./security.md) §9) |
| `HUBTASK_CORS_ALLOWED_ORIGINS` | — | Complete origins (`https://app.example.com`), comma-separated. Empty closes the browser side; a bare host name or a trailing slash fails startup. `*` is allowed on its own and stays safe because credentials are never sent ([security.md](./security.md) §9) |
| `HUBTASK_CORS_MAX_AGE` | `10m` | How long a browser may cache a preflight answer |
| `HUBTASK_HTTP_TIMEOUT` / `_CONNECT_TIMEOUT`, `_MAX_RESPONSE_BYTES`, `_MAX_REDIRECTS` | `10s` / `5s`, `1 MiB`, `3` | Outbound calls (T-07, T-17); every redirect hop is re-checked, `10` is the maximum |
| `HUBTASK_HTTP_CA_FILE` | — | A PEM file of further roots that outbound calls through `GuardedClient` trust beside the system's, for a self-hoster's private CA ([security.md](./security.md) §10); read once at start. Empty adds none and never replaces the system roots; a file that cannot be read or holds no certificate refuses the start (`config.http_ca_file_unreadable`, `config.http_ca_file_invalid`). Other outbound dials (the object store, IMAP, LDAP) are not affected |
| `HUBTASK_HTTP_ALLOWED_HOSTS`, `HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS` | —, `false` | The egress allowlist: empty means every public address and warns in multi mode (T-07); an installation that calls nobody names `webhook.invalid`. Private networks (RFC 1918, loopback, link-local) warn when allowed |
| `HUBTASK_JOB_TIMEOUT`, `_MAX_ATTEMPTS`, `_RETRY_BASE` / `_MAX`, `HUBTASK_SCHEDULER_TICK_INTERVAL` | `60s`, `8`, `5s` / `15m`, `10s` | The queue and the scheduler ([observability-reliability.md](./observability-reliability.md) §15) |
| `HUBTASK_LONG_JOB_TIMEOUT` | `6h` | One pass of a backup, restore, workspace or audit export, or import; the worker extends its lease every third of a lease while it runs, so a killed worker's pass is taken over within one lease |
| `HUBTASK_QUEUE_CONCURRENCY` | `4` | How many jobs one worker process runs at once; a round claims no more than there are free slots, so a long job holds one slot, not the jobs behind it. Each running job holds a background pool connection: above `HUBTASK_DB_MAX_CONNS` minus 3 the worker runs that many and logs it ([observability-reliability.md](./observability-reliability.md) §15) |
| `HUBTASK_QUEUE_POLL_INTERVAL`, `HUBTASK_QUEUE_BATCH_SIZE` | `2s`, `10` | The wait after a round that found no job - the floor under a reminder's punctuality (SLO-5) - and how many jobs one round claims at most |
| `HUBTASK_OUTBOX_MAX_INTERVAL` | `15s` | The dispatcher's poll for a quiet tenant; stays well under SLO-4's 30 seconds |
| `HUBTASK_OUTBOX_MIN_INTERVAL`, `HUBTASK_OUTBOX_BATCH_SIZE` | `1s`, `100` | The dispatcher's poll after a round that delivered something, and how many events one round delivers |
| `HUBTASK_TRIGGER_POLL_LAG` | `60s` | How far behind the present trigger polling reads ([automation.md](./automation.md) §3.2). It must outlast the longest transaction that appends an event; the default equals `HUBTASK_DB_WORKER_STATEMENT_TIMEOUT` |
| `HUBTASK_NATS_URL`, `HUBTASK_NATS_SUBJECT_PREFIX` | —, `hubtask` | The optional message bus ([ADR-0042](../adr/ADR-0042-nats-client.md)). **An empty URL is the off switch:** no connection, no subscriber, no job, the bus `disabled` in `/meta/health`. An unreachable bus is not a startup failure: the outbox holds, the client reconnects. An event lands on `<prefix>.<tenant id>.<type without the de.hubtask. namespace>`, so a consumer binds `<prefix>.<id>.>` for one workspace |
| `HUBTASK_NATS_CREDENTIALS_FILE`, `HUBTASK_NATS_CONNECT_TIMEOUT`, `HUBTASK_NATS_PUBLISH_TIMEOUT` | —, `5s`, `10s` | A mounted NATS credentials file (a path, so the JWT and seed are never a string the process holds), or empty for a server that takes none; the bound on the first connection and on one publish with its ack |
| `HUBTASK_TOMBSTONE_WINDOW` | `2160h` (90 days) | The maximum offline window and the lower bound of an automatic deletion ([offline-sync.md](./offline-sync.md) §7). Lowering it lets a deleted object come back from a device that has not checked in |
| `HUBTASK_HLC_SKEW` | `5m` | The permitted device clock deviation ([offline-sync.md](./offline-sync.md) §4.1) |
| `HUBTASK_MEDIA_STAGING_GRACE`, `_UNREFERENCED_GRACE`, `_ORPHAN_GRACE` | `24h`, `1h`, `1h` | Media reclamation ([data-protection.md](./data-protection.md) §5); the unreferenced grace is never zero |
| `HUBTASK_MEDIA_RECONCILE_BATCH_SIZE`, `HUBTASK_MEDIA_RECONCILE_INTERVAL`, `HUBTASK_RETENTION_BATCH_SIZE`, `HUBTASK_RETENTION_INTERVAL` | `100`, `6h`, `1000`, `1h` | Media reclamation and retention: how many orphans or rows one pass takes, so a pass can be stopped between batches, and how long a workspace's sweep waits after a pass that found nothing left ([data-retention.md](./data-retention.md)) |
| `HUBTASK_DEFAULT_LOCALE` / `_TIMEZONE`, `HUBTASK_LOCALE_DIR` | `en` / `UTC`, — | BCP 47; an IANA name, never a fixed offset; catalogues laid over the embedded ones ([i18n-l10n.md](./i18n-l10n.md) §1). An invalid value refuses to start |

**An own domain and the ingress that asks** (ADR-0095 §4, §5; UC-INS-15). An owner's own domain
is proven by two records, fixed here: a **CNAME** from the domain to `domains.<installation host>`
and a **TXT** record at `_hubtask.<domain>` carrying `hubtask-domain=<mark>`. The operator points
`domains.<installation host>` at the ingress once, as a CNAME or an address; every own domain's
record follows it, and a chain through it is followed to its end. An **apex** - a domain that is
itself registered, such as `acme.com` - takes no CNAME, so the field refuses it
(`admin.own_domain_apex`) and asks for a name in front of it, such as `tasks.acme.com`; offering A
and AAAA records instead would bind every owner's zone to the ingress's addresses. The check looks
each record up through the host's own name servers, every lookup under
`HUBTASK_HTTP_CONNECT_TIMEOUT` (rule 7); it is DNS, not HTTP, so the egress allowlist does not apply.
The ingress obtains the certificate itself, on demand, after asking the **operations port**
`GET /tls/allowed?domain=<host>` - `200` for a workspace's serving host (an own domain once verified,
never once blocked), `404` for every other, `503` when the answer cannot be had. An own domain is
**active** once the ingress asked for it and a request reached the workspace through it; until then
the screen says its certificate is on its way. With Caddy:

```caddyfile
{
  on_demand_tls {
    ask http://hubtask:9090/tls/allowed
  }
}

https:// {
  tls {
    on_demand
  }
  reverse_proxy hubtask:8080
}
```

The operations port is never public, so the question is no lookup for strangers. An ingress that
cannot ask - one holding a wildcard certificate only - serves the installation's own hosts, and an
own domain stays *verified* there without a certificate.

**Semantic search needs pgvector in the database** ([ADR-0050](../adr/ADR-0050-pgvector-as-a-capability.md)).
The reference Compose stack has it. The chart's default database image (`database.imageName`,
`ghcr.io/cloudnative-pg/postgresql:17.6`) does **not**, and is left alone because the database is
the platform's (ADR-0046). An operator who wants semantic search sets `database.imageName` to an
image carrying the extension, or uses a managed PostgreSQL that offers it. Without it nothing
breaks: search stays lexical and `/meta/capabilities` answers `semantic_search: false`.

### 6.2 The migrator

`hubtask-migrate up` reads its own variables, not the server's:

| Variable | Meaning |
|---|---|
| `HUBTASK_DB_DSN` (`_FILE`) | The connection **as the database owner** — never the application's DSN |
| `HUBTASK_DB_APP_PASSWORD` (`_FILE`) | Grants `hubtask_app` its login after the migrations. URL-safe characters (it travels inside the application's DSN). Empty where the cluster manages database roles itself |
| `HUBTASK_DB_CONNECT_WAIT` | How long to keep trying to reach the database. Unset: one attempt |
| `HUBTASK_MIGRATE_TIMEOUT` | The bound on the whole run, `30m` by default. Whatever runs the migrator (`helm --timeout`, a job deadline) must allow at least this long, or it kills a run the migrator would have finished; a killed run is resumed by the next one, which drops a backfill's leftover policy first |
| `HUBTASK_MIGRATE_LOCK_TIMEOUT` | How long a statement of a transactional migration waits for a table lock, `5s` by default; the file is then retried five times with a growing pause and the open transactions it waited behind are logged (process, kind, first word of the statement). A `NO TRANSACTION` file runs without the bound |

The restore drill's variables (`HUBTASK_DRILL_*`) are set by the chart
([backup-restore.md](./backup-restore.md) §8.5).

### 6.3 Inside a workplace portal and a Matrix room

A sovereign workplace (openDesk, for one) shows its applications as tiles of one portal and keeps
its conversations in Matrix rooms. Hubtask joins both without code of the portal's:

* **The portal's tile** is a link with a name and an icon: the name *Hubtask*, the link the
  installation's `HUBTASK_BASE_URL` (in multi mode a workspace's own address), the icon
  `/favicon.svg` beside it. Opened in a new tab it needs nothing more. Opened inside the portal's
  frame, the portal's origin goes into `HUBTASK_UI_FRAME_ANCESTORS`; the API never takes it
  ([security.md](./security.md) §9).
* **A room's widget** is the hub itself: the hub's *Settings → Chat* shows, for a Matrix bot, the
  address a room adds as a widget (UC-INT-18 check 5), and the chat client's origin goes into
  `HUBTASK_UI_FRAME_ANCESTORS`. The address is no credential: whoever opens the widget signs in as
  themselves - once inside the frame, which the browser keeps apart from a tab of its own - and sees
  what they may.
* **A Nextcloud dashboard widget and unified search provider** read two routes as the person
  whose token they carry: `GET /api/v1/glance/open-entries` (their open entries, due first) and
  `POST /api/v1/glance:search` (entries and pages by their words), each answering title, ticket key
  or passage, and a link into the web app (UC-INT-20). A personal access token with `items:read`
  alone is enough; the answer holds only what that person may see. The Nextcloud app that calls
  them is a PHP package of its own repository, not part of this one.
* A sign-in through the workplace's identity provider inside a frame needs the provider to allow
  being framed by the installation, or the provider's page opens in its own window; that is the
  provider's setting.

---

## 7. What happens during a release

1. The tag `vX.Y.Z` is created (`make release-tag VERSION_TO_TAG=X.Y.Z`).
2. `release.yml` builds the installed apps from the tag (the Windows installer, the AppImage and
   the `.deb`, the universal APK), without a secret, and waits for approval of the `production`
   environment.
3. `make verify` runs again on the tag.
4. The multi-arch image is built and pushed to `ghcr.io`.
5. The SBOM is produced, the image is signed keylessly, and provenance is attested.
6. `THIRD-PARTY-LICENSES.md` is regenerated from what this build links.
7. The Helm chart is packaged with `version` and `appVersion` from the tag and pushed.
8. The APK is signed with the project's key, every app and the update manifest `latest.json` with
   the update key, each checked against what [versioning-release.md](./versioning-release.md) §9
   records; one `SHA256SUMS` covers every file the release publishes.
9. The GitHub release is created with generated notes, the SBOM, the chart, the licence list, the
   apps with their signatures, `latest.json` and `SHA256SUMS`; a tag with a hyphen is marked as a
   pre-release.
10. The operator bumps the tag in its Argo CD Application, and production pulls the published chart
    and image. Nothing in this workflow touches the cluster (§4).

Every step fails loudly. No path publishes an image or an app without gates, signature or approval,
and no path deploys from this repository to production.

---

## 8. Open points

| # | Point | Needed by |
|---|---|---|
| D-5 | Closed: a pod compares its embedded migrations with the database's at readiness and is not ready while one is missing (§5, "Readiness compares the schema"). Deliberately not at startup — a failing startup probe restarts the pod in a loop instead of letting it wait; the start logs the comparison | — |
