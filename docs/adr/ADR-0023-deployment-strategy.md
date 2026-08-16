# ADR-0023: Push-based deployment with manual approval, GitOps-ready

* **Status:** accepted
* **Date:** 2026-08-16
* **Concerns:** operations, delivery
* **Related:** [ADR-0014](./ADR-0014-single-image-multi-role.md), [ADR-0022](./ADR-0022-github-platform.md), [deployment.md](../architecture/deployment.md)

## Context

Hubtask has to reach three very different environments: a private individual's machine
(Docker/Podman), the operator's own Kubernetes cluster, and eventually other operators' clusters. At
the same time, a single person works on the project at the start, partly assisted by Claude Code —
every additional moving part costs disproportionately much here.

The usual recommendation for Kubernetes is GitOps (Argo CD, Flux): the cluster pulls its own desired
state from git, and no cluster access sits in CI. That is the more robust model in the long run, but
it means another component in the cluster, a second repository or directory for desired states, and
an additional debugging path when something does not arrive.

## Decision

1. **Start push-based:** the release workflow calls `helm upgrade` against the cluster. Cluster access sits as a secret in the GitHub environment, not in the repository.
2. **Approval through GitHub environments:** `integration` without approval (every push to `main`), `production` only after manual consent. That way even an accidental tag cannot trigger anything.
3. **Everything GitOps will later need is already built that way:** the chart as a versioned OCI artefact in the registry, per-environment values in their own files, and no imperative steps in the deployment beyond `helm upgrade`. Switching over is therefore a replacement of the last step, not a rebuild.
4. **One image, four deployments** in the cluster (`api`, `worker`, `scheduler`, `automation`) — a bulkhead between the interactive and the background path.
5. **Rollbackability is a property of the migrations, not of the deployment:** `helm rollback` works only because migrations are expand/contract-safe and backwards compatible for at least one minor version.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: push-based with approval, GitOps-ready** | The fewest moving parts at the start; the later switch is prepared for and cheap. |
| GitOps from the start (Argo CD/Flux) | More robust in the long run and with no cluster access in CI, but an additional component, an additional failure path, and a second place for desired states — not justified today with one cluster and one person. To be reassessed once there are several clusters or operators (D-3). |
| Deploying by hand | Not reproducible, no traceability, and it collides with the signature and provenance chain. |
| Publishing the container image only, leaving deployment entirely to the operator | Exactly right for self-hosters and implemented that way — but not sufficient for our own operation or for the restore drills. |
| Automatic production deployment without approval | Faster, but a release is the point at which mistakes are most expensive; the approval is simultaneously the boundary the AI path cannot cross. |

## Consequences

**Positive**

* A release is a single, traceable operation with gates, a signature, an SBOM, and an approval.
* Self-hosters and our own operation use the same artefacts — no untested special path.
* No AI path gets past a human approval.

**Negative / countermeasures**

* *Cluster access as a secret in GitHub.* → Separate credentials per environment, tightly bounded rights (our own namespace only), documented rotation; it disappears entirely on the later switch to GitOps.
* *No automatic reconciliation between git and the cluster's actual state.* → The chart version and `hubtask_build_info` as a metric make divergence visible; alert A-13 fires on inconsistent versions in the cluster.
* *Manual approval delays releases.* → Deliberately accepted; `integration` stays fully automatic, so feedback stays fast.
