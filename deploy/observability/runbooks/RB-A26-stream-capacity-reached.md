<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A26 — Live connections are refused at the stream cap

**Alert:** `HubtaskStreamCapacityReached` · **Severity:** ticket · **Catalogue:** A-26

## The symptom

For five minutes, new live connections - the change stream, an agent's stream, a sign-in
snapshot or a live document - were refused because a workspace or a pod already held as many as
`HUBTASK_STREAM_PER_TENANT` (default 512) or `HUBTASK_STREAM_PER_PROCESS` (default 1024) allow
(`hubtask_stream_refused_total{reason="tenant"|"process"}`). Snapshots and live documents may hold
at most half of either, so the change streams keep room. The server itself is fine: refused
clients work on, show "Live updates paused" and pull the changes at a slowing interval until a
slot frees. `/meta/health` says the same (`stream.capacity_reached`).

## Immediate action

```promql
sum by (reason) (increase(hubtask_stream_refused_total[15m]))
sum(hubtask_stream_connections)
```

* **`reason="process"`** → the pods are full. Add a replica, or raise `HUBTASK_STREAM_PER_PROCESS`
  where the pods have memory to spare (each connection holds a goroutine and a socket, a few tens
  of kilobytes).
* **`reason="tenant"`** → one workspace holds its whole share on a pod: a large team, or a client
  that leaks connections. Raise `HUBTASK_STREAM_PER_TENANT`, or add a replica so the workspace's
  connections spread.
* **Only `reason="credential"`** does not fire this alert: one client over its own cap
  (`HUBTASK_STREAM_PER_CREDENTIAL`, default 8) is its own reconnect loop.

## Afterwards

Nothing to repair: refused clients reconnect by themselves once a slot frees, and what changed
meanwhile reaches them through their next pull.
