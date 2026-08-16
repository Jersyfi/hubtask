# ADR-0014 — One container image, several roles

**Status:** accepted · **Date:** 2026-08-14

## Context
Operation in Docker, Podman, **and** Kubernetes is required, ideally from the same build, along with
horizontal scalability. Private individuals want one container; providers want independently
scalable components (API, worker, scheduler, automation).

## Decision
One binary, one multi-arch image (amd64/arm64), distroless, non-root, with a read-only root
filesystem. The active process roles are configured through `HUBTASK_ROLES` (default: all).
Self-hosting: one container with every role. Kubernetes: one deployment per role from the same
image; `scheduler` with one replica and advisory lock leader election, everything else scaled
horizontally. Migrations run as a separate job (`cmd/migrate`, the Helm `pre-upgrade` hook), never
at API startup. Configuration exclusively through environment variables (12-factor) with safe
defaults.

## Options
1. **One image, roles by configuration (chosen).**
2. Separate images per component — a cleaner separation, but n builds, n version states, and a more complicated self-hosting story.
3. An all-in-one process only — simple, but no independent scaling, and automation load spikes hit the API.
4. Separate codebases/repositories per service — the strongest separation, contradicts ADR-0002 and the self-hosting goal.

## Consequences
**Positive:** one build, one version state, one scan, one signing operation; identical behaviour in
every environment; scaling is a deployment question, not a code question; moving from small to large
operation requires no migration of the application.
**Negative:** the image contains the code of every role (slightly larger, a larger theoretical
attack surface); a wrongly set `HUBTASK_ROLES` can silently disable features; a shared release
cadence.
**Countermeasures:** `/readyz` and the startup log report the active roles; an alert when no
instance takes on a mandatory role (`worker`, `scheduler`); the Helm values set the roles
explicitly; configuration validation at startup aborts on an unknown role.
