<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A40 — Live editing does not get through the public address

**Alert:** `HubtaskRealtimeUnreachable` · **Severity:** ticket · **Catalogue:** A-40

## The symptom

Every few minutes each process serving the API opens a WebSocket to its own probe
(`/api/v1/realtime/probe`) through `HUBTASK_BASE_URL` - the way a browser comes in, through
whatever proxy is in front. For a quarter of an hour none got through
(`hubtask_dependency_up{dependency="realtime_public"} == 0`). The health report names
`realtime_public` down and *live editing* as the degraded feature, so the administration and
instance overviews say it too. People can still work - entries, pages and whiteboards save -
but nobody sees another's edits as they happen.

## Immediate action

Open the probe from outside yourself:

```sh
curl -si -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' \
  -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' "$HUBTASK_BASE_URL/api/v1/realtime/probe" | head -1
```

* **`HTTP/1.1 101 Switching Protocols`** → the path works from where you are; the server's own
  attempt does not. Check that the server can resolve and reach its public host (a split DNS,
  an egress firewall between the server and its own proxy).
* **`200`, `400` or `426`** → the proxy does not pass the upgrade. Forward the `Upgrade` and
  `Connection` headers for `/api/v1/realtime/` (nginx: `proxy_http_version 1.1`,
  `proxy_set_header Upgrade $http_upgrade`, `proxy_set_header Connection "upgrade"`) and keep its
  read timeout above 90 seconds.
* **`502` or `504`** → the proxy reaches no process; see RB-A03.

## Afterwards

The alert clears once a process gets through again; the probe reads up at its next attempt,
within five minutes. A base URL that is wrong is a configuration fault elsewhere too (links in
mail, sign-in callbacks): correct `HUBTASK_BASE_URL` rather than the probe.
