# The production environment

A namespace on a Kubernetes cluster this project does not operate. What it is, and why it is that
rather than a second node of our own, is in
[deployment.md §3.2](../../docs/architecture/deployment.md#32-where-production-runs) and
[ADR-0046](../../docs/adr/ADR-0046-production-on-a-platform-namespace.md).

**This directory is not what deploys.** The deploy is pull-based: Argo CD in the cluster renders
the chart at a pinned tag against a values file that lives in the operator's own, private
repository, and a tag bump there is the deploy. What is here is the reference that file is written
from, and the interface between the two sides.

| | |
|---|---|
| [`PLATFORM-INTERFACE.md`](./PLATFORM-INTERFACE.md) | **The one file the operator side reads from here**: which Secrets to create, which values only the platform knows, and the two decisions to take before the first sync |
| [`values.reference.yaml`](./values.reference.yaml) | The shape of the operator's values file, with every environment-specific value left empty on purpose |
| The database | A CloudNativePG `Cluster`, rendered by the chart (`database.enabled`), not a manifest applied by hand |
| The restore drill | RT-9, rendered by the chart (`restoreDrill.enabled`): after every release, and weekly between them |

## Why nothing here names the environment

This repository is public and the environment is private. So no host name, bucket, endpoint, quota
number, label value, Secret name, IP address or credential of production is committed — each one is
a values key with an empty default, named in the interface note and set on the operator's side. A
chart that renders without one of them refuses rather than guessing, which is what makes an empty
key a question instead of a wrong answer nobody checked.

The same rule covers what a drill measures. The RPO and RTO it records are internal (decision 7 of
[milestone 0.6.0](../../docs/backlog/milestone-0.6.0.md)): the drill writes them to a location the
operator names, and what reaches this repository is the mechanism and a pass/fail trail.

## What is ours and what is not

**Ours, inside the namespace:** every application manifest and rollout, the CNPG `Cluster` and its
backup stanza, the migrations, the metrics endpoints, the alert rules and the dashboards, the
resource requests and limits, and Secrets the owner creates and we reference by name.

**Never assumed:** cluster-admin, a second namespace, any exposure beyond the one host name, or
that the platform runs our migrations or our application-level restores. Cluster-scoped resources
are outside what this chart renders, by design.

**And never available:** a kubeconfig, a token or a cluster endpoint in GitHub Actions. There is no
CI-to-cluster path and there will be none, which is why everything that has to run *against*
production runs *inside* it, from the chart: the migration as a sync-wave hook, the restore drill
as a `PostSync` hook and a `CronJob`.

## Sizing, and the one number that is not free

The chart's defaults assume a cluster with room; a namespace has a quota, so the operator's values
file sets replicas and requests against it. The arithmetic is ordinary, except in one place:

**Storage has to hold two databases.** There is no second namespace to restore into, so a restore
drill bootstraps a temporary cluster beside the live one and removes it again. While it runs, the
namespace holds two clusters of `database.storage.size` plus the drill's own pod — so the live
database can grow to somewhat less than half of what the quota admits, and past that the drill
fails first. That is a loud failure and the right one: it says the installation can no longer prove
it can recover, which is a thing to fix before the day it matters.

## Rebuilding it

There is no `bootstrap.sh` here, and that is the point: the cluster, the ingress controller, the
certificate handling, the CloudNativePG operator, the Prometheus Operator and the object storage
all exist already. What this project brings is a Helm release, and what the owner brings is the
Secrets and the values file.

The order of a first sync, and what happens if the drill finds nothing to restore yet, is
[PLATFORM-INTERFACE.md §5](./PLATFORM-INTERFACE.md#5-the-order-of-the-first-sync).

## The one manual step

A credential does not belong in a script's output, in a repository, or in a chat window. The owner
creates the Secrets; this repository only names them
([PLATFORM-INTERFACE.md §1](./PLATFORM-INTERFACE.md#1-secrets-the-owner-creates)).

The database's own credentials are the exception that proves it: CloudNativePG generates them, and
the migration reads the owner's DSN straight out of the Secret the operator made — so the one
credential nobody has to handle is the most powerful one.

## What is not proved here yet

The namespace does not exist at the time of writing. Everything above is rendered, linted and — for
the part that matters most — **executed in CI**: `make gate-pitr` runs the drill against a real
CloudNativePG operator and a real object store on a kind cluster, restores to a point between two
writes, and fails if the wrong marker survives.

What only production can close is listed in
[docs/backlog/blocked-production-namespace.md](../../docs/backlog/blocked-production-namespace.md):
the first real drill, the measured RPO and RTO, and A-12 firing against the operator's live
metrics.
