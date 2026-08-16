# Helm Chart

The chart is published as an OCI artefact with every release:

```bash
helm install hubtask oci://ghcr.io/Jersyfi/charts/hubtask --version 0.1.0 \
  --values my-values.yaml
```

The templates arrive with milestone `0.1.0` (task "Deployment skeleton").
Planned: one deployment per role, a service for `api`, the migration job as a pre-upgrade hook with
an advisory lock, a PodDisruptionBudget, optionally an HPA, a ServiceMonitor, a NetworkPolicy, and a
ConfigMap/Secret pair.

Principles that must not be watered down in the templates:

* `/healthz` is the liveness probe and checks **no** dependencies — otherwise a database outage
  takes down every pod at once (ADR-0016).
* `/readyz` is readiness; pods with an incompatible migration state do not report themselves ready.
* `terminationGracePeriodSeconds` ≥ the longest job timeout.
* No secrets in `values.yaml` — only references to existing secrets.
