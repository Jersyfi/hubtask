# Deployment

How Hubtask reaches the world — for our own operation and for self-hosters.
Complements [ci-cd.md](./ci-cd.md) and [ADR-0022](../adr/ADR-0022-github-platform.md).
Decision: [ADR-0023](../adr/ADR-0023-deployment-strategy.md).

---

## 1. Artefacts

A release produces exactly four things, all under the same version:

| Artefact | Location | Purpose |
|---|---|---|
| Container image (multi-arch, amd64 + arm64) | `ghcr.io/<owner>/hubtask:X.Y.Z` | Every mode of operation |
| Helm chart (OCI) | `oci://ghcr.io/<owner>/charts/hubtask` | Kubernetes |
| SBOM (CycloneDX) + signature + provenance | Attached to the image | Supply chain, verifiability |
| GitHub release with a changelog | Repository | Self-hosters, announcement |

`latest` exists but is explicitly not recommended for production — which is why the Compose file
uses `HUBTASK_VERSION` as a variable, so self-hosters can pin.

---

Which runtimes, architectures and PostgreSQL majors are supported — and the CI job that proves
each one — is [support-matrix.md](./support-matrix.md). It is enforced rather than described: a
row without a job fails the build, and so does a matrix job without a row.

---

## 2. Modes of operation

### 2.1 Self-hosting (Docker/Podman)

Two containers plus a migration job. The Compose file under `deploy/docker/compose.yaml` is the
reference: the database is not published externally, the application runs with `read_only` and
`no-new-privileges`, there are volumes for media and backups, and the migration is a separate
service gated on `service_completed_successfully`.

The application connects as `hubtask_app` — the role the migration creates without `SUPERUSER` or
`BYPASSRLS` — never as the database owner, so row level security is the last boundary in
self-hosting too. The migrator grants that role its login (`HUBTASK_DB_APP_PASSWORD`); the
migration itself deliberately does not, because a credential has no business in a migration.

The operations port is published on loopback only (`127.0.0.1:9090`). It carries the metrics and
the health report — `curl localhost:9090/readyz` after an update, and a Prometheus on the same
host — and neither belongs on the network (observability-reliability.md §3.2).

Updating:

```bash
# raise the version in .env, then
docker compose pull && docker compose up -d
```

The migration runs automatically before the application starts. Because migrations are
expand/contract-safe, this also works while the old version is still running.

### 2.2 Kubernetes

Four deployments from **one** image, distinguished by `HUBTASK_ROLES` ([ADR-0014](../adr/ADR-0014-single-image-multi-role.md)):

| Deployment | Role | Particularity |
|---|---|---|
| `api` | `api` | Behind a service/ingress, HPA possible, `maxUnavailable: 0` |
| `worker` | `worker` | Jobs, outbox delivery, backup runs |
| `scheduler` | `scheduler` | Two replicas, but only one active (advisory lock leader) |
| `automation` | `automation` | Its own pool — a rule storm must not starve the interactive path |

The migration runs as a Helm hook (`pre-upgrade`) with an advisory lock. Pods with an incompatible
migration state report themselves not ready rather than writing inconsistently.

---

## 3. Environments

| Environment | Trigger | Approval | Purpose |
|---|---|---|---|
| **local** | `make run` or Compose | — | Development |
| **integration** | Every push to `main` | Automatic | Dogfooding, load tests, migration rehearsals |
| **production** | Tag `v*` | Manual approval through the GitHub environment | Real operation |

The approval hangs off the GitHub `production` environment, not off a convention. That way even an
accidentally created tag cannot trigger anything without a human agreeing — and the AI path never
gets anywhere near a release ([ADR-0022](../adr/ADR-0022-github-platform.md)).

### 3.1 Where `integration` runs

*Decided 2026-08-21 (open point D-4, and the `integration` half of D-1).*

| | |
|---|---|
| Host | One Hetzner vServer, 4 vCPU / 8 GB / 75 GB, Ubuntu 26.04 LTS, amd64 |
| Kubernetes | k3s, single node |
| Ingress | Traefik, the controller k3s already ships |
| TLS | cert-manager with Let's Encrypt, `HTTP-01`, one certificate per host |
| Database | PostgreSQL in the cluster, on a local volume — the environment is rebuildable, not precious |
| Host names | `<service>.<environment>.hubtask.eu`, so `api.integration.hubtask.eu` today and `app.integration.hubtask.eu` when the web client arrives |

**Why an own server rather than managed Kubernetes.** What `integration` has to prove is that the
chart, the migration hook and the rolling update behave — none of which needs a control plane
somebody else operates. A managed cluster would add a bill and a provider-shaped path that
production might not take anyway, and it would not make a single one of those answers more true. A
single node is honest about what this environment is: dogfooding, load tests and migration
rehearsals for one operator.

**What that costs, and why it is acceptable here.** One node means no node failure is ever
rehearsed, and the database shares a disk with the workload. Both are fine for `integration` and
neither may be carried into production unexamined — which is exactly what D-1's remaining half and
D-2 are for.

**Why the host names carry the environment.** `api.integration.hubtask.eu` leaves production the
shorter `api.hubtask.eu`, and a new service is a new prefix rather than a rename. A wildcard record
covers the whole environment, so adding one is a deployment and not a DNS change. The operations
port is not among them: it stays unrouted, inside the cluster ([observability-reliability.md](./observability-reliability.md)).

---

## 4. Push or pull?

**Decision: start push-based, stay GitOps-ready.**

The workflow calls `helm upgrade` against the cluster. With one cluster and one operator that is
easier to understand and debug than an additional component inside the cluster. The costs are
known: cluster access sits as a secret in GitHub, and the cluster's actual state is not
automatically reconciled with git.

Moving to GitOps (Argo CD or Flux) is prepared for and becomes worthwhile at the point where
several clusters or several operators appear: the chart is already a versioned OCI artefact, and
the per-environment values live in their own files. At that point cluster access in GitHub
disappears entirely — the cluster pulls its own desired state.

---

## 5. Rollout safety

| Measure | Effect |
|---|---|
| `maxUnavailable: 0`, `maxSurge: 1` | No capacity loss during the rollout |
| Readiness gate + PodDisruptionBudget | No pod disappears before its replacement is ready |
| Migration before the rollout, expand/contract | The old and new versions run simultaneously without harm |
| Schema drift detection | A pod with the wrong migration state never becomes ready |
| Graceful shutdown with a grace period | Deregister from `/readyz` first, then drain in-flight requests |
| `terminationGracePeriodSeconds` ≥ the job timeout | No job is cut off mid-work |

**Rollback:** deploy the previous chart version (`helm rollback`). That works only because
migrations are backwards compatible for at least one minor version — which is why expand/contract
is not a matter of style but the precondition for being able to roll back at all. A rollback across
a contract migration requires a restore from backup
([backup-restore.md](./backup-restore.md)).

---

## 6. Configuration

Exclusively `HUBTASK_*` environment variables (12-factor), behind
`core/port/environment/Port.go`. Secrets are additionally available as `HUBTASK_*_FILE`, so that
Docker and Kubernetes secrets can be used without the detour through environment variables.

There is **no default value for a secret**. If one is missing, the process does not start and says
why. An automatically generated key would be worse than a startup error: after a restart, every
piece of data encrypted with it would be unreadable.

A configuration error names its variable through a message code (`config.db_dsn_missing`), and all
problems are reported at once — an operator setting up an installation wants the whole list, not
one problem per restart.

### 6.1 Reference

Required, no default:

| Variable | Meaning |
|---|---|
| `HUBTASK_DB_DSN` | PostgreSQL connection |
| `HUBTASK_SECRET_KEY` | The installation secret, at least 32 characters. It peppers stored credential hashes, signs cursors and mints media and feed tokens — every purpose derived through its own label (security.md §5). It is **not** the key backups and stored credentials are encrypted with; that is the keyring below |

Everything else has a self-hosting default:

| Variable | Default | Meaning |
|---|---|---|
| `HUBTASK_ROLES` | `api,worker,scheduler,automation` | Which roles this process starts (ADR-0014) |
| `HUBTASK_BACKUP_LOCAL_PATH` | `/var/lib/hubtask/backups` | The volume a `local` backup target writes inside. A target's own path is relative to it and cannot leave it, which is what keeps "write my backups to /etc" out of reach of somebody who administers the instance but not the machine. Empty means this installation serves no local targets |
| `HUBTASK_BACKUP_TENANT_TARGETS` | `false` | Lets a tenant configure its own backup target in provider operation (`backup-restore.md` §2). A backup target is an egress channel, and one a tenant chose is an egress channel the operator did not. It has no meaning in single-tenant operation, where the tenant's owner *is* the instance administrator. A target on a private network additionally needs `HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS` |
| `HUBTASK_ENCRYPTION_KEYS` | — | The master keyring for envelope encryption, as key identifiers separated by commas, **current first** (E-02). Lower-case letters, digits and underscores. Empty means this installation encrypts nothing: it starts, and refuses to store anything that would have to be sealed rather than storing it in the clear |
| `HUBTASK_ENCRYPTION_KEY_<ID>` (`_FILE`) | — | The material of one key named above, at least 32 characters, one variable per key so that each can be its own mounted secret. A key named and not supplied fails startup — a ring quietly missing a key is a value nobody notices until an old archive will not open |
| `HUBTASK_HTTP_ADDR` / `HUBTASK_OPS_ADDR` | `:8080` / `:9090` | Public and operations port |
| `HUBTASK_BASE_URL` | — | Absolute URL of the installation; without it links in emails and feeds are wrong (warning) |
| `HUBTASK_TENANCY_MODE` | `single` | `single` for self-hosting, `multi` for provider operation (ADR-0010) |
| `HUBTASK_LOG_FORMAT` / `HUBTASK_LOG_LEVEL` | `json` / `info` | `json` or `text`; `debug`, `info`, `warn`, `error` |
| `HUBTASK_UI_ENABLED` | `true` | Serves the embedded web interface at `/` ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)). `false` answers `/` with 404 and leaves the API untouched — for an installation that is an API and nothing else. Reported to clients as the `web_ui` feature in `/meta/capabilities` |
| `HUBTASK_SHUTDOWN_GRACE_SECONDS` | `30` | Deadline for in-flight requests after `SIGTERM` |
| `HUBTASK_SHUTDOWN_DEREGISTER_SECONDS` | `15` | How long the process keeps serving after marking itself not ready, before it stops accepting connections. Removing a pod from a load balancer is not synchronous with stopping it, so a process that closes its listener at once is still sent requests it can no longer answer — RT-8 measured that as 502s during a rollout ([evidence](../evidence/RT-8-2026-08-21.md)). It is a property of whatever routes the traffic: `0` is right where nothing does |
| `HUBTASK_DB_APP_PASSWORD` (`_FILE`) | — | Read by `hubtask-migrate`, not the server: grants `hubtask_app` its login after the migrations, so the application never connects as the owner. URL-safe characters (it travels inside the DSN) |
| `HUBTASK_DB_MAX_CONNS` / `HUBTASK_DB_MIN_CONNS` | `10` / `2` | Pool size **per process**; several roles mean several pools |
| `HUBTASK_DB_CONNECT_TIMEOUT` | `5s` | Connection deadline |
| `HUBTASK_DB_STATEMENT_TIMEOUT` | `5s` | Query budget on the interactive path |
| `HUBTASK_DB_WORKER_STATEMENT_TIMEOUT` | `60s` | Query budget for background work |
| `HUBTASK_DB_MAX_CONN_LIFETIME` / `HUBTASK_DB_MAX_CONN_IDLE_TIME` | `1h` / `30m` | Bounds reuse, so a failover reaches the pool |
| `HUBTASK_STORAGE_KIND` | `local` | `local` or `s3` |
| `HUBTASK_STORAGE_LOCAL_PATH` | `/var/lib/hubtask/media` | Media directory for `local` |
| `HUBTASK_S3_ENDPOINT`, `_REGION`, `_BUCKET`, `_ACCESS_KEY`, `_SECRET_KEY`, `_USE_PATH_STYLE` | — / `us-east-1` / — / — / — / `true` | S3 or an S3-compatible service; with `kind=s3` the bucket and both keys are mandatory |
| `HUBTASK_SMTP_HOST`, `_PORT`, `_USER`, `_PASSWORD`, `_FROM`, `_SECURITY`, `_TIMEOUT` | — / `587` / — / — / — / `starttls` / `10s` | Without a host, email degrades (warning). With one, `_FROM` is mandatory |
| `HUBTASK_RATE_LIMIT_ANONYMOUS_PER_MINUTE` | `60` | Per IP, unauthenticated |
| `HUBTASK_RATE_LIMIT_TOKEN_PER_MINUTE` | `600` | Per token |
| `HUBTASK_RATE_LIMIT_TENANT_PER_MINUTE` | `3000` | Per tenant |
| `HUBTASK_RATE_LIMIT_AUTH_PER_MINUTE` | `10` | Login, password reset, invitation |
| `HUBTASK_RATE_LIMIT_BURST` | `20` | How much of a budget may be spent at once |
| `HUBTASK_MAX_BODY_BYTES` / `HUBTASK_MAX_UPLOAD_BYTES` | `1 MiB` / `64 MiB` | Request and upload limit (T-17) |
| `HUBTASK_MAX_MAIL_BYTES` | `25 MiB` | The mail intake's own bound (G-11). Its own because a message is not a document: bounding it by the request limit would make the route useless, and by the upload limit would make it a way to store files. 25 MiB is what the mail providers people actually use accept — a message bigger than that is one their sender could not have delivered either |
| `HUBTASK_REQUEST_TIMEOUT` | `30s` | Server-side deadline every handler inherits |
| `HUBTASK_CORS_ALLOWED_ORIGINS` | — | Complete origins (`https://app.example.com`), comma-separated. Empty closes the browser side entirely; a bare host name or a trailing slash fails startup rather than silently matching nothing. `*` is allowed on its own and stays safe because credentials are never sent (security.md §9) |
| `HUBTASK_CORS_MAX_AGE` | `10m` | How long a browser may cache the preflight answer |
| `HUBTASK_HTTP_TIMEOUT` / `HUBTASK_HTTP_CONNECT_TIMEOUT` | `10s` / `5s` | Budget for one outbound call, and for its connection attempt (T-07) |
| `HUBTASK_HTTP_MAX_RESPONSE_BYTES` | `1 MiB` | Cap on what is read from an outbound response (T-17) |
| `HUBTASK_HTTP_MAX_REDIRECTS` | `3` | Hops followed, each re-checked from scratch; `0` follows none, `10` is the maximum |
| `HUBTASK_HTTP_ALLOWED_HOSTS` | — | Egress allowlist, comma-separated host names. Empty means every public address; in multi-tenant operation an empty list warns (T-07) |
| `HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS` | `false` | Allows outbound calls into RFC 1918, loopback and link-local. Warns when set — it turns a webhook into a port scanner of the host network |
| `HUBTASK_QUEUE_POLL_INTERVAL` | `2s` | Wait after a round that found no job. It is the floor under how late a job scheduled without a wake-up can start |
| `HUBTASK_QUEUE_BATCH_SIZE` | `10` | Jobs claimed per round. A full batch is followed by the next round without waiting |
| `HUBTASK_JOB_TIMEOUT` | `60s` | Deadline for one job. The claim's lease is this plus 30s, derived rather than configured: a lease that expires while its job runs is a job two workers are doing |
| `HUBTASK_JOB_MAX_ATTEMPTS` | `8` | Attempts before a job goes to the dead letter with the code of its last failure (alert A-07) |
| `HUBTASK_JOB_RETRY_BASE` / `HUBTASK_JOB_RETRY_MAX` | `5s` / `15m` | Exponential backoff with full jitter between attempts |
| `HUBTASK_SCHEDULER_TICK_INTERVAL` | `10s` | How often the scheduler leader acts, and therefore how quickly a standby notices the leader is gone (ADR-0008) |
| `HUBTASK_OUTBOX_BATCH_SIZE` | `100` | Events delivered per dispatch round |
| `HUBTASK_OUTBOX_MIN_INTERVAL` / `HUBTASK_OUTBOX_MAX_INTERVAL` | `1s` / `15s` | The dispatcher's adaptive poll: the first after a round that delivered something, the second for a quiet tenant. The maximum is the worst case for SLO-4 and stays well under its 30 seconds |
| `HUBTASK_TRIGGER_POLL_LAG` | `60s` | How far behind the present `GET /integrations/triggers/{eventType}` reads (automation.md §3.2). The endpoint pages the outbox in `(occurred_at, id)` order, and `occurred_at` is stamped by the writing transaction rather than by its commit — so a transaction that began before one already answered can still commit a row sorting behind the cursor, and a poller past it would step over the event and never know. Rows younger than this are withheld from the page and from the cursor together. It has to outlast the longest transaction that appends an event: the default is `HUBTASK_DB_WORKER_STATEMENT_TIMEOUT`, the longest write this installation bounds. Lower it for a fresher trigger only if you know your writes are shorter; raise it with the worker's budget |
| `HUBTASK_TOMBSTONE_WINDOW` | `2160h` (90 days) | The maximum offline window (offline-sync.md §7). Two things at once: how long the marker of a removal outlives it, and the lower bound an automatic deletion observes before removing at all. Lowering it lets an automatic deletion outrun a device that has not checked in, which is how a deleted object comes back |
| `HUBTASK_RETENTION_BATCH_SIZE` | `1000` | Rows one pass of a deletion run reads. Batches so that a large deletion does not hold one transaction open across the whole of it (data-retention.md §5) |
| `HUBTASK_RETENTION_INTERVAL` | `1h` | Wait after a pass that reached the end of a tenant's trash. A pass that filled its batch comes back at once instead — there is known work left |
| `HUBTASK_MEDIA_STAGING_GRACE` | `24h` | How long a staged upload may stay unconfirmed before the media reconciliation treats it as abandoned. It has to outlast the fifteen-minute upload window comfortably: a client still pushing 64 MiB up a slow line has abandoned nothing |
| `HUBTASK_MEDIA_UNREFERENCED_GRACE` | `1h` | How long a confirmed media object may point at nothing before the reconciliation calls it an orphan. Never zero: an object points at nothing between its confirmation and the first thing that uses it, and again between a detachment and the next attachment, and a pass landing in either window would mark a file somebody is in the middle of using |
| `HUBTASK_MEDIA_ORPHAN_GRACE` | `1h` | How long a marked media object waits before its bytes go. The window in which a mistaken removal is still recoverable by hand: the row says what it was and the bytes are still where they were |
| `HUBTASK_MEDIA_RECONCILE_BATCH_SIZE` | `100` | Orphans one reclamation pass removes. Each costs a call to a bucket, so a pass that took them all would be a pass nobody can stop |
| `HUBTASK_MEDIA_RECONCILE_INTERVAL` | `6h` | Wait after a pass that found nothing left to reclaim. A pass that filled its batch comes back at once instead (data-protection.md §5) |
| `HUBTASK_DEFAULT_LOCALE` | `en` | BCP 47; the last link in the chain request → account → tenant → installation |
| `HUBTASK_DEFAULT_TIMEZONE` | `UTC` | IANA name, never a fixed offset — an offset cannot represent daylight saving |

Durations are Go syntax (`30s`, `5m`, `1h30m`). A bare number is rejected rather than guessed at.

---

## 7. What happens during a release

1. The tag `vX.Y.Z` is created (`make release-tag VERSION_TO_TAG=X.Y.Z`).
2. The workflow waits for approval of the `production` environment.
3. All gates run again — on the tag, not on an old run.
4. Build the multi-arch image and push it to `ghcr.io`.
5. Produce the SBOM, sign the image keylessly, attach provenance.
6. Package and publish the Helm chart with `version` and `appVersion` from the tag.
7. GitHub release with a changelog generated from the Conventional Commits.
8. Deployment to `production`.

Every step fails loudly. There is no path on which an image is published without gates, without a
signature, or without approval.

---

## 8. Open points

| # | Point | Needed by |
|---|---|---|
| D-1 | Decide the target environment for `production`. `integration` is decided and running ([§3.1](#31-where-integration-runs)); production is deliberately not the same decision, because it is the one coupled to D-2 | `0.6.0` |
| D-2 | Database: own container, operator, or managed service — affects PITR and the restore drill | `0.6.0` |
| D-3 | Evaluate moving to GitOps once there is more than one cluster or more than one operator | `0.9.0` |
| ~~D-4~~ | ~~Domain, TLS approach, and ingress controller~~ — decided in [§3.1](#31-where-integration-runs): `<service>.<environment>.hubtask.eu`, cert-manager with Let's Encrypt, and Traefik | `0.2.0` |
