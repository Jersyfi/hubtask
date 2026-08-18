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
| `HUBTASK_SECRET_KEY` | Master key for envelope encryption, at least 32 characters |

Everything else has a self-hosting default:

| Variable | Default | Meaning |
|---|---|---|
| `HUBTASK_ROLES` | `api,worker,scheduler,automation` | Which roles this process starts (ADR-0014) |
| `HUBTASK_HTTP_ADDR` / `HUBTASK_OPS_ADDR` | `:8080` / `:9090` | Public and operations port |
| `HUBTASK_BASE_URL` | — | Absolute URL of the installation; without it links in emails and feeds are wrong (warning) |
| `HUBTASK_TENANCY_MODE` | `single` | `single` for self-hosting, `multi` for provider operation (ADR-0010) |
| `HUBTASK_LOG_FORMAT` / `HUBTASK_LOG_LEVEL` | `json` / `info` | `json` or `text`; `debug`, `info`, `warn`, `error` |
| `HUBTASK_SHUTDOWN_GRACE_SECONDS` | `30` | Deadline for in-flight requests after `SIGTERM` |
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
| D-1 | Decide the target environment for `integration` and `production` (own server, managed Kubernetes, Hetzner/Scaleway/hyperscaler) | `0.2.0` |
| D-2 | Database: own container, operator, or managed service — affects PITR and the restore drill | `0.6.0` |
| D-3 | Evaluate moving to GitOps once there is more than one cluster or more than one operator | `0.9.0` |
| D-4 | Domain, TLS approach, and ingress controller | `0.2.0` |
