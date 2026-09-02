# ADR-0041 — The NATS client library: nats.go, confined to one adapter

**Status:** accepted · **Date:** 2026-09-02

## Context

[ADR-0007](ADR-0007-events-outbox-cloudevents.md) has listed NATS JetStream as an optional consumer of the
outbox since the day it was written: a dispatcher reads `outbox_event` and hands each event to every
subscriber, "and optionally NATS JetStream". H-14 builds that subscriber, for operators whose
consumers speak NATS rather than webhooks.

It needs a client, and a client is a supply chain decision (`0.6.0` decision 4, CLAUDE.md "what you
do not decide yourself"). Unlike [ADR-0036](ADR-0036-oidc-token-verification.md)'s, this one is not
about parsing hostile input on an authentication path — a NATS server is infrastructure the operator
runs and trusts. It is about something else: **an optional dependency that must cost nothing when it
is not configured**, and about which parts of a protocol an installation would otherwise have to
reimplement.

## Decision

**`github.com/nats-io/nats.go`, pinned, imported by exactly one package**
(`infrastructure/eventbus`), with the confinement proved by `gate-architecture` the way `cel-go`'s
is (ADR-0009, `TestTheExpressionEngineIsBehindOneAdapter`).

At v1.53.1 it brings exactly three new modules into `go.mod`:

| Module | Licence |
|---|---|
| `github.com/nats-io/nats.go` | Apache-2.0 |
| `github.com/nats-io/nkeys` | Apache-2.0 |
| `github.com/nats-io/nuid` | Apache-2.0 |

Everything else it needs — `golang.org/x/crypto`, `net`, `sys`, `text` — is already required, at
newer versions. `make gate-licenses` refuses the AGPL/GPL/LGPL families
([ADR-0013](ADR-0013-licensing.md)); Apache-2.0 passes, and relicensing stays possible.

## Options

1. **The official client (chosen).**
2. **A minimal publisher written here.** The NATS wire protocol is text, and a JetStream publish is
   one `PUB` and one reply carrying an ack — genuinely small. What it would not have is TLS verified
   against a cluster, nkeys or JWT credentials, and reconnection with server discovery. That is not
   a list of luxuries: it is the difference between "works against a NATS server on a private
   network with no authentication" and "works against the NATS an operator actually runs". The
   project does write its own where the surface is small and closed — TOTP is dependency-free, the
   router is `net/http` — and this surface is neither.
3. **Do not build the adapter.** Webhooks already deliver the same CloudEvents, so the honest
   version of this option is deleting ADR-0007's sentence rather than leaving a promise unbuilt. It
   was weighed and declined: the operators this is for have a bus already, and asking them to run a
   webhook receiver in front of it is asking them to bridge in the wrong direction.

## Consequences

**Positive:** an operator with a NATS estate receives the events without a bridge; the client
handles the authentication modes their cluster uses; the dependency is one package deep and one
architecture test keeps it there.

**Negative:** three modules, and a transport whose failure modes now belong to this system's
`/meta/health`. Both are bounded — the first by the confinement test, the second by the breaker
and the outbox behind it.

**Cost when unconfigured, which is the whole of what "optional" has to mean:** no connection is
attempted, no goroutine starts, and the subscriber is not registered with the dispatcher. It is
linked into the binary and it is inert — the same shape the S3 and SMTP adapters already have, and
it is a test rather than an intention.

**The degradation row is proved rather than promised.** `observability-reliability.md` §7 has
carried "NATS (optional) → fallback to the database outbox dispatch → no visible change" since the
document was written. Stopping the NATS container opens the breaker, `/meta/health` and the
degraded-mode metric say so, the outbox holds, and delivery resumes on return without a restart —
RT-1's container discipline applied to a new dependency.

**Replays are withheld.** A restore's events reach only subscribers that opted in
(`eventbus.TakesReplays`, `backup-restore.md` §8.4), and this one does not: an external bus
receiving four hundred replayed events would fire whatever its consumers do, which is the thing
§8.4 exists to prevent.
