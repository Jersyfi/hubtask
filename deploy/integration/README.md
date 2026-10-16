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
scp -r deploy root@<host>:/root/
ssh root@<host>
export HUBTASK_WAL_ACCESS_KEY=… HUBTASK_WAL_SECRET_KEY=…   # the WAL bucket's keys, below
/root/deploy/integration/bootstrap.sh
```

The two keys are read once, into the Secret `hubtask-wal`, and never printed; a later run finds the
Secret and does not ask again.

The whole `deploy/` directory rather than `deploy/integration` alone: the script builds
Prometheus's rules ConfigMap from `deploy/observability/alerts/`, so that what evaluates on this
cluster is the file the gate tests and not a copy of it. The script says so and stops if the
directory is missing, rather than starting a Prometheus with no rules.

Then the one manual step, which is manual on purpose — a credential does not belong in a script's
output, in a repository, or in a chat window:

```bash
ssh root@<host> cat /root/deploy-kubeconfig.yaml | gh secret set KUBE_CONFIG --env integration
```

What the host needs beforehand: ports 22, 80, 443 and 6443 open inbound, and a DNS record
`*.<environment>.hubtask.eu` pointing at it (A and AAAA). Port 80 is not optional — it is where
Let's Encrypt validates.

## The database

PostgreSQL runs in production's shape ([ADR-0082](../../docs/adr/ADR-0082-integration-in-production-shape.md)):
a CloudNativePG `Cluster` of one instance named `hubtask-db`, which the release owns
([`values.yaml`](./values.yaml), `database`); its WAL archive and a daily base backup in the
object-store bucket `hubtask-integration-wal`; and the restore drill as a CronJob at 04:30, never
after a release, because integration releases on every push. The operator is installed by
`bootstrap.sh` at the version and checksum `gate-pitr` drills (`make gate-pitr`; a test holds the
two pins equal). Smaller in size, the same in shape: one instance, 4 GiB, a quarter of a gigabyte
requested.

| | |
|---|---|
| The live cluster | `kubectl -n hubtask get cluster hubtask-db` |
| The archive | `kubectl -n hubtask get backups` (base backups), `cnpg_collector_*` in Prometheus |
| The last drill | `kubectl -n hubtask get configmap hubtask-restore-drill -o yaml` |
| A drill now | `kubectl -n hubtask create job --from=cronjob/hubtask-restore-drill drill-$(date +%s)` |

### Rebuilding the database

What turned the StatefulSet of before into the cluster, and what rebuilds the cluster later. The
data is nobody's ([ADR-0082](../../docs/adr/ADR-0082-integration-in-production-shape.md)), and the
workspace is bootstrapped again afterwards. It deletes a database, so a person runs it; the
script stops rather than doing it itself.

1. **The bucket.** In the Hetzner console, check whether Object Storage offers Object Lock. If
   it does, create `hubtask-integration-wal` with Object Lock in compliance mode and a default
   retention of 35 days - equal to `database.backup.retentionPolicy`, as `k8s/values.yaml` requires.
   If it does not, create it without, and say so in the first drill's record. Make one access key
   for it.
2. **The old database away** (only where it still runs):

   ```bash
   kubectl -n hubtask delete statefulset hubtask-db
   kubectl -n hubtask delete service hubtask-db
   kubectl -n hubtask delete pvc -l app.kubernetes.io/name=postgresql
   kubectl -n hubtask delete secret hubtask-db hubtask-secrets
   ```

3. **Bootstrap**, with the bucket's keys exported (above). It installs the operator and makes the
   secrets and the drill's Role.
4. **Deploy**: re-run the last `deploy.yml` on `main`. After a rebuild it creates the cluster first
   and migrates into it once it is ready.
5. **The workspace**, again (below).
6. **The first drill**: start one now (the table above), and when the record says it passed,
   write it down under `docs/evidence/` as `RESTORE-DRILL-integration-<date>.md`: the date, the
   commit, whether the bucket has Object Lock, the RPO and RTO the record measured, and whether the
   8 GB node held the live cluster and the temporary one together. That record is UC-BAK-08
   check 6.

## A workspace somebody can sign in to

A rebuilt environment has no workspace. Its first start prints a one-time setup code on the
server's console, valid for thirty minutes (restarting the deployment prints a new one while nothing
is set up); [`scripts/dev-workspace.sh`](../../scripts/dev-workspace.sh) spends it on the first
workspace and its owner, who becomes the installation's operator, and mints the control plane's
token behind a step-up — the same way it does locally (`CONTRIBUTING.md` § "Signing in locally"):

```bash
export SSH="ssh -i <key> root@<host>"
CODE="$($SSH kubectl -n hubtask logs deploy/hubtask-api | grep -o 'setup code [0-9A-Z-]*' | tail -1 | cut -d' ' -f3)"
HUBTASK_SETUP_CODE="$CODE" HUBTASK_DEMO_PASSWORD='…' \
scripts/dev-workspace.sh --bootstrap --api https://api.integration.hubtask.eu --slug operations --name Operations
```

The bootstrap prints the token once; it goes into the GitHub environment `integration` beside
`KUBE_CONFIG`, and into the shell that runs the second step — the shell keeps whatever it held
before, so export the one just printed:

```bash
HUBTASK_ADMIN_TOKEN=… HUBTASK_DEMO_PASSWORD='…' \
scripts/dev-workspace.sh --api https://api.integration.hubtask.eu --slug demo
```

Then `https://demo.api.integration.hubtask.eu/` is a sign-in screen, and `owner@example.org` with
that password gets in. Every API pod prints a code of its own; any one of them works, once. A lost
token is replaced by the operator minting a new one (*Profile → Access tokens*, or `POST
/auth/tokens` behind a step-up); `--bootstrap` runs only on an installation that is not set up.

## When nobody can run it

An environment whose operator register is empty while it holds more than one workspace - the
operator account deleted, a database restored from before the register, a bootstrap skipped - says
so in its health report (`config.operator_register_empty`). The way back is on the machine, with
the server's own configuration, and nowhere else:

```bash
$SSH kubectl -n hubtask exec deploy/hubtask-api -- \
  hubtask operator add --workspace operations --email ops@example.org
```

It adds that account to the register, records it in the installation's journal as done from the
machine, and says what it did; an address that matches no account in that workspace is refused.
The person then signs in and opens *Installation*.

## Mail

Two mail catchers run on the node, and they are not interchangeable. The application sends
through `smtp` in the `hubtask` namespace — invitations, reminders, the notifications a person
turned on — over STARTTLS behind a certificate authority the cluster issued to itself
([`smtp.yaml`](./smtp.yaml)); the application trusts that authority through `SSL_CERT_DIR`, the
way a self-hoster trusts the authority behind an internal relay. Alertmanager delivers into
`mailpit` in the `monitoring` namespace, below. A reminder in the alert mailbox would be noise in
the one mailbox the daily check reads, and an alert in the application's would be a person's
notification nobody sent — so two catchers, two namespaces, and nothing in common but the
protocol.

Both take what they are given and forward to nobody, which is what makes recipients at
`example.org` safe here. Reading what the application sent:

```bash
kubectl -n hubtask port-forward svc/smtp 8025:8025 >/dev/null 2>&1 &
curl -s localhost:8025/api/v1/messages | head -c 2000
kill %1
```

## Monitoring

The environment watches itself (`observability-reliability.md` §14, O-1): Prometheus scrapes every
role's operations port, evaluates the three shipped rule files, and hands what fires to
Alertmanager, which delivers by SMTP into a mail catcher in the same namespace. Nothing has an
Ingress — the way in is the kubeconfig.

| | |
|---|---|
| Namespace | `monitoring` |
| Applied by | `bootstrap.sh`, from [`monitoring.yaml`](./monitoring.yaml) |
| History | `ALERTS{alertstate="firing"}` in Prometheus, 45 days |
| Now | `/api/v2/alerts` in Alertmanager |
| Delivered | `/api/v1/messages` in Mailpit |

The mail catcher is the receiver because the environment's alerts are read by whoever is working
on it. Delivery is SMTP all the same — the receiver a provider would use — so a real mail server
later is an address and a credential in the config, not a different routing. What is deliberately
*not* here is a pager: an alert that fires at three in the morning waits in Mailpit until somebody
looks, and this environment is one where that is the right answer.

### The daily check

[`alert-check.sh`](./alert-check.sh) asks the four questions in the order they matter, from your
own machine, and changes nothing:

```bash
export HUBTASK_INTEGRATION_SSH="ssh -i ~/.ssh/<key> root@<host>"
deploy/integration/alert-check.sh
```

It exits 0 when nothing needs a person and 1 when something does, so it can be run by a schedule
as well as by hand.

1. **Is the alerting path alive?** A watchdog mail older than a day means Prometheus, Alertmanager
   or the catcher has stopped — and that every silence underneath it is unexplained. It is asked
   first because it is the failure that hides all the others.
2. **What is firing**, with the runbook for each: the runbook is the answer to "what do I do about
   it", and this script deliberately does not try to be a second one.
3. **What is pending** — about to fire, and cheaper to deal with before it does.
4. **Is every process still scraped?** A pod nobody scrapes reports nothing, and an alert that
   reads its metrics cannot fire while it is gone.

Two things are expected here and are not reasons to act. The watchdog fires permanently — that is
what it is for, and it is reported under question 1 rather than counted as an incident. And A-12's
backup half, `HubtaskBackupStale`, fires because this environment runs no application backups by
the decision below; it is printed with that said rather than hidden, because a check that silently
drops an alert is a check that lies. A-12's recovery half - the archive, the base backup, the
recoverability point - is real here and is not expected to fire.

If a run finds something, the fix path is the alert's runbook, and a fix that changes this
repository goes through a branch and a pull request like any other.

```bash
# from the host: forward, ask, stop. Port-forwarding rather than curl against the cluster IP,
# because the IP changes and the service name does not.
kubectl -n monitoring port-forward svc/alertmanager 9093:9093 >/dev/null 2>&1 &
curl -s localhost:9093/api/v2/alerts | head -c 2000
kill %1

kubectl -n monitoring port-forward svc/mailpit 8025:8025 >/dev/null 2>&1 &
curl -s localhost:8025/api/v1/messages | head -c 2000
kill %1
```

## What is not here, and why

* **No application backups.** The workspace's own backups to a target - what A-12's backup half
  watches - are a workspace's choice, and nothing here is anybody's data. The installation's
  recovery is the database's WAL archive and the daily drill, as production's is
  ([ADR-0082](../../docs/adr/ADR-0082-integration-in-production-shape.md)); `HubtaskBackupStale`
  is routed to `muted` for that reason, and A-12's recovery half routes like every other alert.
* **No high availability.** One node, one database instance, as production's single instance. The
  environment answers questions about the chart, the migration hook, the rollout and the recovery
  path - none of which needs a second node - and rehearses no node failure. The database shares a
  disk with the workload.
* **No egress.** The application's allowlist names one host, the e2e walk's webhook target, and
  it never resolves (`values.yaml`). The list exists because production runs with one (T-07) and
  an empty list means "anywhere public"; it is short because nothing here has anywhere to go. A
  walk that needs a receiver that answers adds its host to the list, in the diff, where the
  addition can be read.
* **No silenced A-14.** The three findings that once filled the ticket mailbox for a week were
  answered by making them untrue — a mail server, an allowlist, and a warning that had outlived
  the code it warned about — rather than by a route to `muted`. The one route to `muted` is
  `HubtaskBackupStale`'s, for a decision this list records above; a finding this environment does
  not mean to keep is a finding to fix.

## What an RT-8 run leaves behind

RT-8 needs an empty database at the previous schema, and it gets one by adding rather than
deleting — the environment's own database is never touched. [`rt8-database.sh`](./rt8-database.sh)
makes it: a second database `hubtask_rt8` in the live cluster, as a CloudNativePG `Database` object
owned by `hubtask_migrator`, and a secret with its two DSNs. The run's release points at that
secret. So a run leaves these behind, and they are listed here because state nobody wrote down is
state nobody dares remove:

| | |
|---|---|
| `hubtask_rt8` | the `Database` object `rt8` and the database it made, holding the run's items |
| `hubtask-secrets-rt8` | the secret pointing at it |
| `ghcr.io/jersyfi/hubtask:rt8-*` | images imported into containerd for the run, never pushed anywhere |

They are the fixtures for the next run, so leaving them is reasonable. Removing them is:

```bash
deploy/integration/rt8-database.sh down
k3s ctr images ls -q | grep ':rt8-' | xargs -r k3s ctr images rm
```

The environment itself is unaffected either way: it runs against `hubtask`, and the values file is
what says so.

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
