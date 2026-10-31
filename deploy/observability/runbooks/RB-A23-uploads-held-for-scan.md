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

* **No `PONG`** → the daemon is down, still loading its signatures, or out of memory. Restart it;
  the held files settle on the job's next pass, and the alert resolves on the scheduler's next
  tick after that.
* **`PONG`, but files stay held** → the daemon answers too slowly for `HUBTASK_CLAMD_TIMEOUT`
  (default 2m), or refuses the stream (`INSTREAM size limit exceeded`: the daemon's
  `StreamMaxLength` below the upload limit). Raise the one
  that is short.
* **`HUBTASK_CLAMD_ADDRESS` was emptied with files held** → they stay held, never served unscanned.
  Configure the scanner again; there is no switch that releases them unscanned.

## Diagnostic queries

```promql
max(hubtask_media_scan_held_workspaces)
hubtask_job_queue_depth{job_type="media.scan"}
increase(hubtask_job_dead_letter_total{job_type="media.scan"}[1h])
```

## Escalation

A dead-lettered `media.scan` job (A-07) means a pass failed for a reason other than the scanner
not answering - look at its last error code. Held files with no open job then wait for the next
confirmation in that workspace to seed the job again.

## Follow-up

Once the gauge is zero, the alert resolves. A file found malicious meanwhile is `INFECTED`, its
bytes removed; the server logs its media id and the signature name.
