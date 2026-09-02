<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A17 — A certificate expires in under two weeks

**Alert:** `HubtaskCertificateExpiring` · **Severity:** ticket · **Catalogue:** A-17

## The symptom

A certificate is within 14 days of its end and automatic renewal has not replaced it. Renewal
is supposed to be invisible — this alert firing means the automation broke, with two weeks of
margin to fix it before browsers start refusing connections.

## Where the certificates live

Hubtask's own key material does not expire: webhook signing secrets and the encryption keyring
rotate by operator action, not by calendar. TLS is terminated in front of the process — in our
own operation by Traefik with certificates from **cert-manager** (Let's Encrypt HTTP-01,
deploy/integration). This rule therefore reads cert-manager's exporter
(`certmanager_certificate_expiration_timestamp_seconds`); on a stack without cert-manager
metrics it has no series and stays silent by design — the A-20 precedent for a rule ahead of an
environment's wiring, dormant rather than `absent()`-noisy.

## Immediate action

```bash
# What does cert-manager think it is doing?
kubectl get certificates -A
kubectl describe certificaterequest -A | tail -40

# The classic causes, in order of frequency:
kubectl get challenges -A          # a stuck ACME challenge
kubectl logs -n cert-manager deploy/cert-manager --since 1h | grep -i error
```

* **A stuck HTTP-01 challenge** → the ingress route for `/.well-known/acme-challenge/` is not
  reaching the solver; usually an ingress or DNS change since the last renewal.
* **Rate-limited by the CA** → too many issuances (a crash loop re-requesting); wait out the
  window, fix the loop.
* **The Certificate resource is gone** → somebody cleaned it up; re-apply the manifest.

## Diagnostic queries

```promql
(certmanager_certificate_expiration_timestamp_seconds - time()) / 86400
certmanager_certificate_ready_status{condition="False"}
```

## Escalation

Under 3 days remaining, treat as a page: an expired certificate is a full outage for every
client that validates.

## Follow-up

If renewal broke because of an ingress change, add the challenge path to that change's
checklist. The 14-day threshold exists to make this a calm ticket — keep it that way.
