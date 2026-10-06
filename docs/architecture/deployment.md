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

The GitHub Container Registry is the default registry; the chart's `image.repository` and the
Compose file's `HUBTASK_IMAGE` make a mirror possible.

`latest` exists and is not recommended for production. The Compose file reads the tag from
`HUBTASK_VERSION`, so a self-hoster pins it.

---

Which runtimes, architectures and PostgreSQL majors are supported — and the CI job that proves each
one — is [support-matrix.md](./support-matrix.md). A row without a job fails the build, and so does
a matrix job without a row.

---

## 2. Modes of operation

One image, several roles ([ADR-0014](../adr/ADR-0014-single-image-multi-role.md)): one binary,
distroless, non-root, read-only root filesystem. `HUBTASK_ROLES` chooses which of `api`, `worker`,
`scheduler` and `automation` a process runs (default: all four). The image also carries the
migrator (`hubtask-migrate`) and the restore drill.

### 2.1 Self-hosting (Docker/Podman)

The reference is `deploy/docker/compose.yaml`: the database, a one-shot migration service, and the
application. The database is not published; the application runs `read_only` with
`no-new-privileges`; there are volumes for media and backups; the application starts only after the
migration ended with `service_completed_successfully`. The reference database image is
`pgvector/pgvector:pg16`, so semantic search is available.

**The first workspace and its owner** are created with `scripts/dev-workspace.sh --bootstrap`. The
browser setup with a one-time code printed at first start is
[UC-INS-01](../usecases/admin/UC-INS-01-start-a-fresh-installation.md) — specified, not built.
Installation settings can be seeded or enforced from a file (`HUBTASK_INSTANCE_FILE`, §6.1).

The application connects as `hubtask_app` — the role the migration creates without `SUPERUSER` or
`BYPASSRLS` — never as the database owner, so row level security is the last boundary in
self-hosting too. The migrator grants that role its login from `HUBTASK_DB_APP_PASSWORD`; the
migration itself never carries a credential (§6.2).

The operations port is published on loopback only (`127.0.0.1:9090`). It carries the metrics and the
health report — `curl localhost:9090/readyz` after an update, a Prometheus on the same host — and
neither belongs on the network ([observability-reliability.md](./observability-reliability.md) §3.2).

**The system backup is the self-hoster's own.** Without an operator for continuous archiving, the
stack offers a documented `pg_dump` and the tenant archives beside it;
[backup-restore.md §8.6](./backup-restore.md#86-the-minimal-path-a-dump-and-what-it-does-not-give)
says which guarantees that does not carry.

Updating:

```bash
# raise HUBTASK_VERSION in .env, then
docker compose pull && docker compose up -d
```

The migration runs before the application starts. Because migrations are expand/contract-safe
([versioning-release.md](./versioning-release.md) §4), the old version may still be running while it
does.

### 2.2 Kubernetes

Four deployments from **one** image, distinguished by `HUBTASK_ROLES`:

| Deployment | Role | Particularity |
|---|---|---|
| `api` | `api` | Behind a service/ingress; HPA available (`roles.api.autoscaling`); its own load-shedding threshold |
| `worker` | `worker` | Jobs, outbox delivery, backup runs |
| `scheduler` | `scheduler` | Two replicas, one active (advisory lock leader, [observability-reliability.md](./observability-reliability.md) §15) |
| `automation` | `automation` | Its own pool — a rule storm must not starve the interactive path |

**The migration** runs as a Job before the rollout: a Helm hook (`pre-install,pre-upgrade`) and,
under Argo CD, a `Sync`-phase hook in a sync wave after the database, so that a chart which also
renders its CloudNativePG `Cluster` (§3.2) migrates a database that exists. The migrator holds an
advisory lock for the length of the run, so two migrators never run at once.

* **The migration's DSN is its own.** The Job reads its DSN from a separate secret key
  (`migration.dsnSecretKey`, optionally in `migration.dsnSecretName`), so the chart connects the
  application as `hubtask_app` and migrates as the owner. With `database.enabled` that is the
  operator-generated `<database.name>-app` Secret's `uri`, and the owner's credential is never
  copied by hand. `migration.appPasswordSecretKey` grants `hubtask_app` its login after migrating.
* **The migrator waits for the database** (`migration.connectWait`, `HUBTASK_DB_CONNECT_WAIT`)
  rather than trusting the sync wave to have waited; whether Argo CD knows what a healthy `Cluster`
  looks like is the platform's knowledge.
* **A migration whose number is lower than one already applied is applied late**
  ([versioning-release.md](./versioning-release.md) §4).

---

## 3. Environments

| Environment | Trigger | Approval | Purpose |
|---|---|---|---|
| **local** | `make run` or Compose | — | Development |
| **integration** | Every push to `main` (`deploy.yml`) | Automatic | Dogfooding, load tests, migration rehearsals |
| **production** | A tag bump in the operator's Argo CD Application — the cluster pulls | The GitHub environment `production` approves publishing the version; the deploy is the operator's | Real operation |

The approval hangs off the GitHub `production` environment, not off a convention: an accidental tag
publishes nothing without a human agreeing, and no AI path reaches a release
([ADR-0022](../adr/ADR-0022-github-platform.md)). The approval releases an image and a chart under a
version; **it deploys nothing.** Production pulls what was published (§4).

### 3.1 Where `integration` runs

| | |
|---|---|
| Host | One Hetzner vServer, 4 vCPU / 8 GB / 75 GB, Ubuntu 26.04 LTS, amd64 |
| Kubernetes | k3s, single node |
| Ingress | Traefik, the controller k3s ships |
| TLS | cert-manager with Let's Encrypt, `HTTP-01`, one certificate per host |
| Database | PostgreSQL in the cluster, on a local volume — the environment is rebuildable, not precious |
| Mail | A catcher in the namespace, over STARTTLS behind a certificate authority the cluster issued to itself: the application takes production's path and nothing leaves the node |
| Host names | `<service>.<environment>.hubtask.eu`: `api.integration.hubtask.eu`; a workspace is a subdomain of it (`demo.api.integration.hubtask.eu`). A wildcard record covers the environment, so a new service is a deployment and not a DNS change |
| Deploy | `deploy.yml`: builds `:main-<sha>`, signs it under its own identity, `cosign verify` against it, then `helm upgrade` with the environment's values file, without `--atomic` — a failed rollout leaves the previous pods serving and the failed Job's logs readable |
| Monitoring | Prometheus and Alertmanager in the cluster ([observability-reliability.md](./observability-reliability.md) §13.1) |

The operations port is never routed. An own single node is enough for what `integration` has to
prove — the chart, the migration hook and the rolling update — and it is honest about what it is:
no node failure is rehearsed, and the database shares a disk with the workload. Neither property may
be carried into production.

### 3.2 Where `production` runs

Decided in [ADR-0046](../adr/ADR-0046-production-on-a-platform-namespace.md).

| | |
|---|---|
| Cluster | A single private k3s cluster operated by a platform from a private operator repository, not by this project |
| Namespace | One. Nothing this project deploys is cluster-scoped; the Argo `AppProject` admits that namespace and a fixed list of kinds |
| Deploy identity | **None in GitHub.** Argo CD in the cluster pulls this chart at a pinned tag; no kubeconfig, token or endpoint of production exists in this repository or its workflows |
| Ingress | The platform's. TLS is a platform-issued wildcard, referenced by Secret *name* from values; the chart creates no `Certificate` |
| Host name | One, reachable through the VPN only, set by the operator and not written in this repository. The operations port is not routed |
| Database | PostgreSQL through the platform's CloudNativePG operator; the `Cluster` is a template of our chart (`database.enabled`), in our namespace |
| System backups | CNPG's continuous WAL archiving and a daily base backup to object storage with Object Lock, plus the platform's volume snapshots as a second net |
| Media | An S3 bucket of its own, separate from the backup bucket |
| Monitoring | The platform's Prometheus Operator scrapes our `ServiceMonitor`s and evaluates our `PrometheusRule`s; alerts route by namespace; dashboards ship as ConfigMaps for the platform's Grafana. The selector labels are set by the operator |
| Deployed by | The operator bumps the tag in its Argo CD Application (multi-source: this chart at the tag, its values file in the private repository); Argo CD renders and applies |
| Image policy | The chart can render a Kyverno `ClusterPolicy` (`imagePolicy.enabled`, default off) that admits only images signed by the release or the integration deploy identity; whether to apply it is the platform's decision |

**What we own, and what we never assume.** Inside the namespace: every application manifest, the
CNPG `Cluster` and its backup stanza, the migrations, the metrics endpoints, rules and dashboards,
the resource requests and limits, and Secrets the owner creates and we reference by name. Outside it,
by design: cluster-scoped resources, other namespaces, cluster-admin, any exposure beyond the one host
name. The platform runs neither our migrations nor our application-level restores. **What must run
against production therefore runs from the chart, inside the cluster:** the migration as a hook in a
sync wave, the restore drill as a `PostSync` hook of every release and as a `CronJob` between them
([backup-restore.md §8.5](./backup-restore.md#85-the-operator-procedure-point-in-time-recovery)).

**Two consequences.** The restore drill can restore only into our own namespace, as a second CNPG
cluster bootstrapped from the object store — so the resource quota bounds the live database to less
than half of what it admits. And the restore runbook must be executable by a person alone.

**Nothing of production is written here.** This repository is public and the environment is
private: no host name, bucket, endpoint, quota, label value, Secret name or credential of production
is committed. Every such value is a values key marked *set by the operator*, and the complete list
is [`deploy/production/PLATFORM-INTERFACE.md`](../../deploy/production/PLATFORM-INTERFACE.md). The
RPO and RTO a drill measures stay internal
([observability-reliability.md](./observability-reliability.md) §13.2).

---

## 4. Push or pull?

**`integration` is push-based; production is pull-based.**

* **`integration`:** `deploy.yml` runs `helm upgrade` against the cluster with a deploy identity
  that cannot create namespaces (`KUBE_CONFIG`, environment `integration`). One cluster and one
  operator make push the simpler path to understand and debug. The image is verified before the
  upgrade (§3.1).
* **Production:** Argo CD renders this chart at a pinned tag with a values file in the operator's
  private repository. A tag bump there is the deploy. Nothing in this repository's workflows can
  reach that cluster.

Both paths share the chart and differ only in who runs `helm`.

---

## 5. Rollout safety

| Measure | Effect |
|---|---|
| `maxUnavailable: 0`, `maxSurge: 1` | No capacity loss during the rollout |
| Readiness gate + a PodDisruptionBudget per role (`minAvailable: 1`) | No pod disappears before its replacement is ready |
| Migration before the rollout, expand/contract | The old and new versions run simultaneously without harm |
| Migration versions watched across pods | `hubtask_migration_version` per pod; alert A-13 fires when pods disagree for more than 15 minutes. A pod does **not** yet compare its schema at startup or readiness (§8, D-5) |
| Graceful shutdown | On `SIGTERM` the process marks itself not ready, keeps serving for `HUBTASK_SHUTDOWN_DEREGISTER_SECONDS`, then drains in-flight requests within `HUBTASK_SHUTDOWN_GRACE_SECONDS` and releases job leases |
| `terminationGracePeriodSeconds` (120) ≥ the job timeout plus the drain budget | No job is cut off mid-work |

**Rollback:** deploy the previous chart version — `helm rollback` for `integration`, the previous
tag in the Argo CD Application for production. That works only because the schema stays backwards
compatible for at least one minor version, which is why expand/contract is the precondition for
rolling back at all. A rollback across a contract migration needs a restore from backup
([backup-restore.md](./backup-restore.md)).

---

## 6. Configuration

Exclusively `HUBTASK_*` environment variables (12-factor), behind `core/port/environment/Port.go`.
`HUBTASK_DB_DSN`, `HUBTASK_SECRET_KEY`, `HUBTASK_S3_ACCESS_KEY`, `HUBTASK_S3_SECRET_KEY`,
`HUBTASK_SMTP_PASSWORD`, each `HUBTASK_ENCRYPTION_KEY_<ID>` and `HUBTASK_DB_APP_PASSWORD` are also
read from `<NAME>_FILE`, so Docker and Kubernetes secrets work without the detour through the
environment.

There is **no default value for a secret**. If one is missing, the process does not start and says
why. A generated key would be worse than a startup error: after a restart, everything encrypted with
it would be unreadable.

A configuration error names its variable through a message code (`config.db_dsn_missing`), and all
problems are reported at once. Durations are Go syntax (`30s`, `5m`, `1h30m`); a bare number is
rejected rather than guessed at.

### 6.1 Reference

Required, no default:

| Variable | Meaning |
|---|---|
| `HUBTASK_DB_DSN` | PostgreSQL connection, as `hubtask_app` |
| `HUBTASK_SECRET_KEY` | The installation secret, at least 32 characters. It peppers stored credential hashes, signs cursors and mints media and feed tokens — every purpose derived through its own label ([security.md](./security.md) §5). It is **not** the key backups and stored credentials are encrypted with; that is the keyring below |

Everything else has a self-hosting default:

| Variable | Default | Meaning |
|---|---|---|
| `HUBTASK_ROLES` | `api,worker,scheduler,automation` | Which roles this process starts |
| `HUBTASK_TENANCY_MODE` | `single` | `single` for self-hosting, `multi` for provider operation ([multi-tenancy.md](./multi-tenancy.md)) |
| `HUBTASK_INSTANCE_FILE` | — | A file of installation settings ([ADR-0070](../adr/ADR-0070-the-instance-layer.md)). Empty: the settings live only in the database |
| `HUBTASK_INSTANCE_FILE_MODE` | `seed` | `seed` writes the file once, at first start; `enforce` writes it at every start and the routes that would change those settings refuse. One source per mode, never two |
| `HUBTASK_HTTP_ADDR` / `HUBTASK_OPS_ADDR` | `:8080` / `:9090` | Public and operations port |
| `HUBTASK_BASE_URL` | — | Absolute URL of the installation; without it links in mails and feeds are wrong (warning) |
| `HUBTASK_UI_ENABLED` | `true` | Serves the embedded web interface at `/` ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)). `false` answers `/` with 404 and leaves the API untouched. Reported as the `web_ui` feature in `/meta/capabilities` |
| `HUBTASK_LOG_FORMAT` / `HUBTASK_LOG_LEVEL` | `json` / `info` | `json` or `text`; `debug`, `info`, `warn`, `error` |
| `HUBTASK_METRICS_TENANT_LABEL` | `false` | Adds `tenant_id` to metrics; off because many tenants explode the cardinality ([observability-reliability.md](./observability-reliability.md) §3.2) |
| `HUBTASK_TRACING_ENABLED` | `false` | Exports traces over OTLP/HTTP |
| `HUBTASK_TRACING_ENDPOINT` | — | The OTLP/HTTP endpoint URL; required when tracing is on |
| `HUBTASK_TRACING_SAMPLE_RATIO` | `0.05` | The share of ordinary traces kept; errors and slow requests are kept regardless ([observability-reliability.md](./observability-reliability.md) §3.3) |
| `HUBTASK_SHUTDOWN_GRACE_SECONDS` | `30` | Deadline for in-flight requests after `SIGTERM` |
| `HUBTASK_SHUTDOWN_DEREGISTER_SECONDS` | `15` | How long the process keeps serving after marking itself not ready. Removing a pod from a load balancer is not synchronous with stopping it, so a process that closes its listener at once is still sent requests it cannot answer ([RT-8 evidence](../archive/evidence/RT-8-2026-08-21.md)). `0` is right where nothing routes the traffic |
| `HUBTASK_STEP_UP_WINDOW` | `5m` | How long a step-up grant stays valid for the one action it was given for ([security.md](./security.md) §5) |
| `HUBTASK_ENCRYPTION_KEYS` | — | The master keyring for envelope encryption: key identifiers separated by commas, **current first**; lower-case letters, digits and underscores. Empty: the installation starts and refuses to store anything that would have to be sealed, rather than storing it in the clear |
| `HUBTASK_ENCRYPTION_KEY_<ID>` (`_FILE`) | — | The material of one key named above, at least 32 characters, one variable per key so each can be its own mounted secret. A key named and not supplied fails startup |
| `HUBTASK_DB_MAX_CONNS` / `HUBTASK_DB_MIN_CONNS` | `10` / `2` | Pool size **per process**; several roles mean several pools |
| `HUBTASK_DB_CONNECT_TIMEOUT` | `5s` | Connection deadline |
| `HUBTASK_DB_STATEMENT_TIMEOUT` | `5s` | Query budget on the interactive path |
| `HUBTASK_DB_WORKER_STATEMENT_TIMEOUT` | `60s` | Query budget for background work |
| `HUBTASK_DB_MAX_CONN_LIFETIME` / `HUBTASK_DB_MAX_CONN_IDLE_TIME` | `1h` / `30m` | Bounds reuse, so a failover reaches the pool |
| `HUBTASK_STORAGE_KIND` | `local` | `local` or `s3` |
| `HUBTASK_STORAGE_LOCAL_PATH` | `/var/lib/hubtask/media` | Media directory for `local` |
| `HUBTASK_S3_ENDPOINT`, `_REGION`, `_BUCKET`, `_ACCESS_KEY`, `_SECRET_KEY`, `_USE_PATH_STYLE` | — / `us-east-1` / — / — / — / `true` | S3 or an S3-compatible service; with `kind=s3` the bucket and both keys are mandatory. The web interface's policy names the bucket's origin in `connect-src` and `img-src` — the endpoint's under path style, `https://<bucket>.<endpoint host>` under virtual-hosted ([ADR-0047](../adr/ADR-0047-media-origin-in-the-interface-policy.md)). **The bucket needs a CORS rule for the upload**, which Hubtask cannot set: allowed origin exactly the interface's origin (never `*`), method `PUT`, header `Content-Type`, no credentials — the presigned URL is the credential. A cover draws without it; an upload without it fails in the browser with a message naming the bucket |
| `HUBTASK_SMTP_HOST`, `_PORT`, `_USER`, `_PASSWORD`, `_FROM`, `_SECURITY`, `_TIMEOUT` | — / `587` / — / — / — / `starttls` / `10s` | Without a host, mail degrades (warning). With one, `_FROM` is mandatory |
| `HUBTASK_BACKUP_LOCAL_PATH` | `/var/lib/hubtask/backups` | The volume a `local` backup target writes inside. A target's own path is relative to it and cannot leave it. Empty: this installation serves no local targets |
| `HUBTASK_BACKUP_TENANT_TARGETS` | `false` | Lets a tenant configure its own backup target in provider operation ([backup-restore.md](./backup-restore.md) §2). A target a tenant chose is an egress channel the operator did not. No meaning in single mode. A target on a private network also needs `HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS` |
| `HUBTASK_RESTORE_DRILL_RECORD_FILE` | — | A file holding the Unix timestamp of the last restore drill that passed, written by `hubtask-restore-drill` ([backup-restore.md](./backup-restore.md) §8.5) and read at every scrape as `hubtask_restore_drill_last_success_timestamp_seconds` (alert A-20). The chart mounts the drill's record here; in a Compose stack the operator writes it after a restore they checked. Empty: the series is absent rather than zero |
| `HUBTASK_AI_DUPLICATE_THRESHOLD` | `0` (the built-in 0.85) | The cosine similarity two entries must reach before either is proposed as the other's duplicate. 0.85 is measured ([evidence](../archive/evidence/K-04-2026-09-11.md)): a paraphrase is found more often than not, and a false candidate reaches about four entries in a hundred. Outside 0…1 is refused at startup. Meaningful only where semantic search is available |
| *(the workspace's embedding model)* | set per workspace, `hubctl ai … --embedding-model` | Not an environment variable; named here because its width is an operator concern. The index holds 1536 dimensions ([ADR-0054](../adr/ADR-0054-embedding-width.md)): a narrower model is stored padded; a wider one is refused before the first text is sent where the provider can say its width, and at the first batch where it cannot, with `ai.embedding_too_wide` in the worker's log. Search then stays lexical, `/meta/capabilities` answers `semantic_search: false` and `/meta/health` reports the provider degraded |
| `HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER` | `false` | The operator's confirmation that a workspace may configure an AI provider processing outside the EEA ([data-protection.md](./data-protection.md) §6). A workspace administrator cannot set it. Which provider a workspace uses is the workspace's configuration |
| `HUBTASK_RATE_LIMIT_ANONYMOUS_PER_MINUTE` | `60` | Per IP, unauthenticated |
| `HUBTASK_RATE_LIMIT_TOKEN_PER_MINUTE` | `600` | Per token |
| `HUBTASK_RATE_LIMIT_TENANT_PER_MINUTE` | `3000` | Per tenant |
| `HUBTASK_RATE_LIMIT_AUTH_PER_MINUTE` | `10` | Sign-in, password reset, invitation |
| `HUBTASK_RATE_LIMIT_BURST` | `60` | How much of a budget may be spent at once. A browser opening a page is a burst — the web app's first paint of a cold entry page is 24 requests — and the minute's budget, not the burst, is what bounds a caller |
| `HUBTASK_LOAD_SHED_INFLIGHT` | `64` | Requests in flight above which deferrable work — bulk, export, search, the query shapes — is refused with `503` and `Retry-After` ([observability-reliability.md](./observability-reliability.md) §6). Per role: the chart sets it on the `api` deployment. `0` switches shedding off |
| `HUBTASK_LOAD_SHED_RETRY_AFTER` | `5s` | What a shed caller is told to wait |
| `HUBTASK_MAX_BODY_BYTES` / `HUBTASK_MAX_UPLOAD_BYTES` | `1 MiB` / `64 MiB` | Request and upload limit (T-17) |
| `HUBTASK_MAX_MAIL_BYTES` | `25 MiB` | The mail intake's own bound — what the common mail providers accept |
| `HUBTASK_REQUEST_TIMEOUT` | `30s` | Server-side deadline every handler inherits |
| `HUBTASK_CORS_ALLOWED_ORIGINS` | — | Complete origins (`https://app.example.com`), comma-separated. Empty closes the browser side; a bare host name or a trailing slash fails startup. `*` is allowed on its own and stays safe because credentials are never sent ([security.md](./security.md) §9) |
| `HUBTASK_CORS_MAX_AGE` | `10m` | How long a browser may cache the preflight answer |
| `HUBTASK_HTTP_TIMEOUT` / `HUBTASK_HTTP_CONNECT_TIMEOUT` | `10s` / `5s` | Budget for one outbound call, and for its connection attempt (T-07) |
| `HUBTASK_HTTP_MAX_RESPONSE_BYTES` | `1 MiB` | Cap on what is read from an outbound response (T-17) |
| `HUBTASK_HTTP_MAX_REDIRECTS` | `3` | Hops followed, each re-checked from scratch; `0` follows none, `10` is the maximum |
| `HUBTASK_HTTP_ALLOWED_HOSTS` | — | Egress allowlist, comma-separated host names. Empty means every public address; in multi mode an empty list warns (T-07). An installation that calls nobody names `webhook.invalid`, which never resolves |
| `HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS` | `false` | Allows outbound calls into RFC 1918, loopback and link-local. Warns when set |
| `HUBTASK_QUEUE_POLL_INTERVAL` | `2s` | Wait after a round that found no job; the floor under how late a job without a wake-up starts |
| `HUBTASK_QUEUE_BATCH_SIZE` | `10` | Jobs claimed per round; a full batch is followed by the next round at once |
| `HUBTASK_JOB_TIMEOUT` | `60s` | Deadline for one job. The claim's lease is this plus 30 s |
| `HUBTASK_JOB_MAX_ATTEMPTS` | `8` | Attempts before a job goes to the dead letter with the code of its last failure (A-07) |
| `HUBTASK_JOB_RETRY_BASE` / `HUBTASK_JOB_RETRY_MAX` | `5s` / `15m` | Exponential backoff with full jitter between attempts |
| `HUBTASK_SCHEDULER_TICK_INTERVAL` | `10s` | How often the scheduler leader acts, and so how quickly a standby notices the leader is gone |
| `HUBTASK_OUTBOX_BATCH_SIZE` | `100` | Events delivered per dispatch round |
| `HUBTASK_OUTBOX_MIN_INTERVAL` / `HUBTASK_OUTBOX_MAX_INTERVAL` | `1s` / `15s` | The dispatcher's adaptive poll: the first after a round that delivered something, the second for a quiet tenant. The maximum stays well under SLO-4's 30 seconds |
| `HUBTASK_TRIGGER_POLL_LAG` | `60s` | How far behind the present `GET /integrations/triggers/{eventType}` reads ([automation.md](./automation.md) §3.2). `occurred_at` is stamped by the writing transaction, not by its commit, so rows younger than this are withheld from page and cursor alike. It must outlast the longest transaction that appends an event; the default equals `HUBTASK_DB_WORKER_STATEMENT_TIMEOUT` |
| `HUBTASK_NATS_URL` | — | The optional message bus ([ADR-0042](../adr/ADR-0042-nats-client.md)), `nats://host:4222`, several comma-separated. **Empty is the off switch:** no connection, no subscriber, no job, and `/meta/health` reports the bus as `disabled` |
| `HUBTASK_NATS_SUBJECT_PREFIX` | `hubtask` | The first token of every subject. An event lands on `<prefix>.<tenant id>.<type without the de.hubtask. namespace>`, so a consumer binds `<prefix>.<id>.>` for one workspace |
| `HUBTASK_NATS_CREDENTIALS_FILE` | — | A mounted NATS credentials file (the nkeys/JWT form `nsc` writes), or empty for a server that takes none |
| `HUBTASK_NATS_CONNECT_TIMEOUT` / `HUBTASK_NATS_PUBLISH_TIMEOUT` | `5s` / `10s` | The first connection, and one publish including its ack. An unreachable bus at startup is not a startup failure: the process serves, the outbox holds, the client reconnects |
| `HUBTASK_TOMBSTONE_WINDOW` | `2160h` (90 days) | The maximum offline window ([offline-sync.md](./offline-sync.md) §7): how long the marker of a removal outlives it, and the lower bound an automatic deletion observes. Lowering it lets a deleted object come back from a device that has not checked in |
| `HUBTASK_RETENTION_BATCH_SIZE` | `1000` | Rows one pass of a deletion run reads ([data-retention.md](./data-retention.md) §5) |
| `HUBTASK_RETENTION_INTERVAL` | `1h` | Wait after a pass that reached the end of a tenant's trash; a pass that filled its batch comes back at once |
| `HUBTASK_HLC_SKEW` | `5m` | How far a device's clock reading may stand from server time before a push replaces it ([offline-sync.md](./offline-sync.md) §4.1); a replaced reading is logged with the device and the drift |
| `HUBTASK_MEDIA_STAGING_GRACE` | `24h` | How long a staged upload may stay unconfirmed before it counts as abandoned |
| `HUBTASK_MEDIA_UNREFERENCED_GRACE` | `1h` | How long a confirmed media object may point at nothing before it counts as an orphan. Never zero |
| `HUBTASK_MEDIA_ORPHAN_GRACE` | `1h` | How long a marked object waits before its bytes go — the window for recovering a mistaken removal by hand |
| `HUBTASK_MEDIA_RECONCILE_BATCH_SIZE` | `100` | Orphans one reclamation pass removes |
| `HUBTASK_MEDIA_RECONCILE_INTERVAL` | `6h` | Wait after a pass that found nothing left to reclaim ([data-protection.md](./data-protection.md) §5) |
| `HUBTASK_DEFAULT_LOCALE` | `en` | BCP 47; the last link in the chain request → account → tenant → installation |
| `HUBTASK_DEFAULT_TIMEZONE` | `UTC` | IANA name, never a fixed offset |
| `HUBTASK_LOCALE_DIR` | — | A directory of `<tag>.json` catalogues laid over the embedded ones, read once at start ([i18n-l10n.md](./i18n-l10n.md) §1). A path that is not a directory, or a file that is not a catalogue, refuses to start |

**Semantic search needs pgvector in the database** ([ADR-0050](../adr/ADR-0050-pgvector-as-a-capability.md)).
The reference Compose stack has it. The chart's default database image (`database.imageName`,
`ghcr.io/cloudnative-pg/postgresql:17.6`) does **not**, and is deliberately left alone, because the
database is the platform's (ADR-0046). An operator who wants semantic search sets
`database.imageName` to an image carrying the extension, or uses a managed PostgreSQL that offers
it. Without it nothing breaks: the migration detects the extension, search stays lexical, and
`/meta/capabilities` answers `semantic_search: false`.

### 6.2 The migrator

`hubtask-migrate up` reads its own variables, not the server's:

| Variable | Meaning |
|---|---|
| `HUBTASK_DB_DSN` (`_FILE`) | The connection **as the database owner** — never the application's DSN |
| `HUBTASK_DB_APP_PASSWORD` (`_FILE`) | Grants `hubtask_app` its login after the migrations. URL-safe characters (it travels inside the application's DSN). Empty where the cluster manages database roles itself |
| `HUBTASK_DB_CONNECT_WAIT` | How long to keep trying to reach the database before giving up. Unset: one attempt |

The restore drill's variables (`HUBTASK_DRILL_*`) are set by the chart
([backup-restore.md](./backup-restore.md) §8.5).

---

## 7. What happens during a release

1. The tag `vX.Y.Z` is created (`make release-tag VERSION_TO_TAG=X.Y.Z`).
2. `release.yml` waits for approval of the `production` environment.
3. `make verify` runs again on the tag.
4. The multi-arch image is built and pushed to `ghcr.io`.
5. The SBOM is produced, the image is signed keylessly, and provenance is attested.
6. `THIRD-PARTY-LICENSES.md` is regenerated from what this build links.
7. The Helm chart is packaged with `version` and `appVersion` from the tag and pushed.
8. The GitHub release is created with generated notes, the SBOM, the chart and the licence list; a
   tag with a hyphen is marked as a pre-release.
9. The operator bumps the tag in its Argo CD Application, and production pulls the published chart
   and image. Nothing in this workflow touches the cluster (§4).

Every step fails loudly. There is no path on which an image is published without gates, without a
signature, or without approval — and no path on which this repository deploys to production.

---

## 8. Open points

| # | Point | Needed by |
|---|---|---|
| D-5 | A pod compares the schema it was built for with the database's at startup and readiness, and reports itself not ready on a mismatch. Today only the `migration` field of the health report and alert A-13 exist, and nothing fills the field | Before `1.0.0` |
