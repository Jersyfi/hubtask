<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A13 — The cluster is running two migration versions

**Alert:** `HubtaskMigrationVersionsDiverge` · **Severity:** ticket · **Catalogue:** A-13

## The symptom

Pods disagree about which migration their build embeds, and have for fifteen minutes. Every pod
reports the version compiled into it (`hubtask_migration_version`), so a spread is a rolling
update in flight — and one that holds is a rollout stuck halfway, with old and new code serving
side by side.

That is a state migrations are designed to survive (expand/contract, ADR-0003), so nothing is
corrupting — but the longer it holds, the longer the window in which somebody ships a change
that assumed the rollout finished.

## Immediate action

```bash
# What is actually running?
kubectl get pods -o custom-columns=NAME:.metadata.name,IMAGE:.spec.containers[0].image

# Why is the rollout stuck?
kubectl rollout status deploy -l app.kubernetes.io/name=hubtask --timeout=1s
kubectl get events --sort-by=.lastTimestamp | tail -20
```

* **New pods CrashLooping** → read their logs; a startup that fails after migrations ran is the
  classic shape. Roll back the deployment — the migrations are forward-only and the old code
  runs on the migrated schema by design.
* **New pods Pending** → resources or scheduling; the rollout is stuck for cluster reasons, not
  application ones.
* **No rollout at all** (`kubectl rollout history` shows none recent) → somebody is running a
  stray pod on an old image; find and delete it.

## Diagnostic queries

```promql
hubtask_migration_version
count by (version) (hubtask_build_info)
```

## Escalation

If a rollback is impossible and the new pods cannot start, this becomes an availability incident
as old pods cycle out — page whoever owns the deployment pipeline.

## Follow-up

A rollout stuck on the readiness probe is worth checking against A-03's runbook; the two firing
together tell the whole story.
