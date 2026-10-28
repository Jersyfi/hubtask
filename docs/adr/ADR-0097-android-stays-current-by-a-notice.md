# ADR-0097 — Android stays current by a notice

**Status:** accepted · **Date:** 2026-10-28 · **Accepted:** 2026-10-28 · **Decided:** by the owner,
[decision #1764](https://github.com/Jersyfi/hubtask/issues/1764) · **Amends:**
[ADR-0084](./ADR-0084-the-first-majors-shells-cost-nothing-to-ship.md)

**Rule lives in:** [roadmap.md](../roadmap.md) (`1.0.0` prerequisite 19),
[UC-SYN-07](../usecases/sync/UC-SYN-07-install-the-app-on-my-computer-or-phone.md) checks 5 and 6,
[milestone-F7.md](../backlog/milestone-F7.md) Decision 5

## Context

ADR-0084 keeps every shell current "by Tauri's updater". Tauri 2's updater runs on Windows, Linux
and macOS only. An Android app outside a store cannot replace itself silently either: the system
installs an APK only after the person agrees, and only if it is signed with the same key.

## Decision

* Windows and Linux stay current by Tauri's updater, as ADR-0084 says.
* The Android app reads the same signed update manifest; when a newer release exists it says so and
  opens the APK's download, and the system installer does the rest. The app asks for no permission
  to install packages.
* `1.0.0` prerequisite 19's updater run is the desktop's; the Android notice is proven by its own
  test.

## Consequences

* On Android an update takes one tap more than on a desktop, and needs no sensitive permission or
  download path of our own.
* Android itself refuses an APK signed with another key, so the project's APK key is as critical as
  the update key (F7 Decision 10).
