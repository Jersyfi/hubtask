<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A23 — Uploads wait for a malware scan

**Alert:** `HubtaskUploadsHeldForScan` · **Severity:** ticket · **Catalogue:** A-23

## The symptom

For more than an hour, at least one workspace holds uploads the scanner has not settled. A file
confirmed while the daemon at `HUBTASK_CLAMD_ADDRESS` did not answer is held `PENDING_SCAN`: it
can be attached and listed, but nobody can download it or see it as a picture until a scan says
it is clean (UC-MED-01 check 10). The gauge counts workspaces, not files.

Nothing is lost while files wait: the bytes are kept, and the `media.scan` job of each such
workspace scans them again every five minutes.

## Immediate action

```bash
# Does the daemon answer? (unix socket or host:port, as configured)
echo PING | nc -U /run/clamav/clamd.ctl      # or: echo PING | nc clamav 3310
# What does the server say about the held files?
kubectl logs deploy/hubtask | grep 'the upload scanner did not answer'
```

* **The log line `the upload scanner did not answer; the file is held` carries
  `dependency.target_not_allowed` or `dependency.target_blocked` in its `error`** → the guard refused the scanner's address, and a `PONG` from a shell proves nothing
  about it. The host named in `HUBTASK_CLAMD_ADDRESS` is allowed beside
  `HUBTASK_HTTP_ALLOWED_HOSTS` by being named, so a refusal means the name resolves to a metadata
  address (`169.254.169.254` and its kin, refused always). Name the daemon by an address on your
  own network, or use a `unix:` socket.
* **No `PONG`** → the daemon is down, still loading its signatures, or out of memory. Restart it;
  the held files settle on the job's next pass, and the alert resolves on the scheduler's next
  tick after that.
* **`PONG`, but files stay held** → the daemon answers too slowly for `HUBTASK_CLAMD_TIMEOUT`
  (default 2m). Raise it. A file the daemon refuses for its size (`INSTREAM size limit exceeded`)
  does not stay held: it is settled `UNSCANNABLE`, never served, and the pass goes on. Uploads are
  capped to `HUBTASK_CLAMD_STREAM_MAX_BYTES` (default 25 MiB), which must match the daemon's
  `StreamMaxLength`.
* **`HUBTASK_CLAMD_ADDRESS` was emptied with files held** → they stay held, never served unscanned.
  Configure the scanner again; there is no switch that releases them unscanned.

## Diagnostic queries

```promql
max(hubtask_media_scan_held_workspaces)
hubtask_job_queue_depth{job_type="media.scan"}
increase(hubtask_job_dead_letter_total{job_type="media.scan"}[1h])
```

## Escalation

A dead-lettered `media.scan` job (A-07) means a pass failed for a reason that is not transient -
look at its last error code. A transient one (storage or database away) never dead-letters it: the
job is a duty, and its failure is its next round. A dead letter is revived by the scheduler after an
hour, doubling to a day for each revival, and the gauge keeps counting its workspace until a later
pass succeeds, so this alert stays firing while the files stay held. If the same error comes back
on every revival, fix its cause; the files settle on the next revival after that.

## Follow-up

Once the gauge is zero, the alert resolves. A file found malicious meanwhile is `INFECTED`, its
bytes removed; the server logs its media id and the signature name.
