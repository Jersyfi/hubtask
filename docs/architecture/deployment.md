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

## 2. Modes of operation

### 2.1 Self-hosting (Docker/Podman)

Two containers plus a migration job. The Compose file under `deploy/docker/compose.yaml` is the
reference: the database is not published externally, the application runs with `read_only` and
`no-new-privileges`, there are volumes for media and backups, and the migration is a separate
service gated on `service_completed_successfully`.

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
