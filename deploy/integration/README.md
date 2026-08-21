# The integration environment

One k3s node that runs Hubtask from every push to `main`. What it is for, and what it is
deliberately not, is in [deployment.md §3.1](../../docs/architecture/deployment.md#31-where-integration-runs);
this directory is how it is built and how it is rebuilt.

| | |
|---|---|
| API | `https://api.integration.hubtask.eu` |
| Kubernetes API | `https://k8s.integration.hubtask.eu:6443` |
| Namespace | `hubtask` |
| Deployed by | [`.github/workflows/deploy.yml`](../../.github/workflows/deploy.yml), on every push to `main` |

## Rebuilding it

The whole environment is one script against a fresh Ubuntu host. It is idempotent, so running it
against a live one is safe — and that is the property that matters: an environment that can only be
built once is an environment nobody dares touch.

```bash
scp -r deploy/integration root@<host>:/root/
ssh root@<host> /root/integration/bootstrap.sh
```

Then the one manual step, which is manual on purpose — a credential does not belong in a script's
output, in a repository, or in a chat window:

```bash
ssh root@<host> cat /root/deploy-kubeconfig.yaml | gh secret set KUBE_CONFIG --env integration
```

What the host needs beforehand: ports 22, 80, 443 and 6443 open inbound, and a DNS record
`*.<environment>.hubtask.eu` pointing at it (A and AAAA). Port 80 is not optional — it is where
Let's Encrypt validates.

## What is not here, and why

* **No backup.** Nothing in this environment is anybody's data, and a restore drill against a
  throwaway database would prove nothing about production's. RT-9 belongs to a real target
  ([backup-restore.md](../../docs/architecture/backup-restore.md)).
* **No high availability.** One node, one database pod. The environment answers questions about the
  chart, the migration hook and the rollout — none of which needs a second node.
* **No production hardening of PostgreSQL.** It is a container with a local volume and
  `sslmode=disable` inside the cluster. Production is D-1 and D-2, and both are open on purpose.

## Rotating the deploy credential

The token in GitHub is the only credential this environment hands out, and it is one object:

```bash
kubectl -n hubtask delete secret github-deployer-token
kubectl apply -f deploy/integration/deployer-rbac.yaml
ssh root@<host> /root/integration/bootstrap.sh          # rewrites /root/deploy-kubeconfig.yaml
ssh root@<host> cat /root/deploy-kubeconfig.yaml | gh secret set KUBE_CONFIG --env integration
```

Deleting that secret revokes the access immediately — which is the reason CI never gets the k3s
admin certificate. That one is cluster-admin over everything, and revoking it means re-issuing the
cluster's certificate authority.

## Looking at it as an operator

```bash
ssh root@<host>
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl -n hubtask get pods
kubectl -n hubtask logs deploy/hubtask-api -f
```

The operations port is not routed and will not be: metrics and the health report say what runs, how
much of it and how slowly, and none of that belongs on the public internet
([observability-reliability.md](../../docs/architecture/observability-reliability.md)). Reach it
from the node:

```bash
kubectl -n hubtask exec deploy/hubtask-api -- wget -qO- localhost:9090/readyz
```
