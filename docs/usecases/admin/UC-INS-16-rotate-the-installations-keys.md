---
id: UC-INS-16
title: Rotate the installation's keys and know every secret moved
context: admin
actors: [PE-operator, PE-selfhoster]
deployments: [D1, D4, D5, D6, D7]
serves: [P-03, P-11]
state: partial
tasks: []
checked_by: []
---

# Rotate the installation's keys and know every secret moved

## Goal

An operator adds a new master key, re-seals every stored secret under it — webhook secrets, backup
target credentials, AI keys, provider secrets, at workspace and installation level — sees that
nothing is left under the old key, and only then removes it.

## Story

The operator adds the new key to the environment and restarts. *Installation → Keys* counts, per
key, how many secrets it still seals. A re-seal moves them; when the old key's count is zero, the
screen says it can be removed.

## How to check

1. The key screen counts secrets per key across every kind, at both levels, without naming which
   workspace holds which.
2. Re-sealing moves every secret of every kind, including the installation's own provider and AI
   secrets.
3. Removing a key that still seals something is refused, or the health report says which kind would
   become unreadable.
4. Nothing is shown in plain text at any point.

## Where it ends

* The keys themselves stay in the environment ([ADR-0045](../../adr/ADR-0045-master-key-in-the-environment.md));
  the screen shows state, it does not turn the ring.

## Today

* Check 1: not met — the census counts per workspace only, so the installation's provider secrets are counted once for every workspace and the installation's own level is not counted on its own, tracked in #1068.
* Check 2: not met — the re-seal runs per workspace; the installation's provider secrets are never moved, tracked in #1068.
