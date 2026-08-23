# ADR-0031 — Tauri 2 shells for desktop and mobile; the PWA path is closed

**Status:** accepted · **Date:** 2026-08-23

## Context

[ADR-0021](./ADR-0021-offline-sync.md) promises clients that work entirely without a network, and
its own context note said the clients would "probably" be a PWA. That assumption was a
placeholder, and this ADR retires it. What has firmed up since:

* [offline-sync.md](../architecture/offline-sync.md) §9 makes binding demands on every client's
  local store: encrypted at rest, deleted completely on sign-out and on `ACCESS_REVOKED`, and —
  implicitly but essentially — **actually there** when the user returns. A local queue of
  unsynced mutations is user data; storage the platform may silently evict cannot hold it.
* [ADR-0030](./ADR-0030-svelte-frontend-framework.md) settles one Svelte codebase for the product
  UI. Whatever carries it to desktop and mobile must carry *that* codebase, not a second one.
* The EU is the core market (hubtask.eu, GDPR-first per
  [ADR-0018](./ADR-0018-privacy-by-design.md)), so platform behaviour in the EU is a first-order
  input, not a footnote.

The question: how does Hubtask become an **installable client** on desktop (Windows, macOS,
Linux) and mobile (iOS, Android)?

## Options

**A. A progressive web app.** Rejected, and the rejection is final rather than deferred — the
web app stays a pure browser application: no web app manifest install, no install prompt, no
service-worker install setup. The reasons are functional, verified against the sources under
References:

1. **Browser storage on iOS is not dependable.** Since iOS 13.4 / Safari 13.1, WebKit's tracking
   prevention deletes *all* script-writable storage (IndexedDB, LocalStorage, service worker
   registrations) after seven days of Safari use without interacting with the site. An installed
   home-screen web app is exempt from that seven-day timer — but its storage remains best-effort,
   evictable under storage pressure, and the Persistent Storage API can only *request* protection,
   with the grant left to browser heuristics (Chromium has historically keyed it to engagement
   signals such as notification permission). A mutation queue held in storage with those semantics
   breaks ADR-0021's promise the first time the platform tidies up.
2. **No Background Sync on iOS — in any browser.** WebKit does not implement the Background Sync
   API, and every iOS browser is required to use WebKit, so there is no browser on iOS where
   deferred sync after the tab closes works.
3. **Split storage between the Safari tab and the installed app.** A home-screen web app gets its
   own storage partition. A user who works in the browser for two weeks and then "installs" starts
   from an empty store — with an unsynced local queue potentially stranded in the tab's partition.
4. **No install path worth the name on iOS.** There is no automatic install prompt
   (`beforeinstallprompt` is unsupported); installation is a manual trip through the share menu,
   and there is no app-store distribution at all.
5. **Platform risk in the core market.** Apple disabled home-screen web apps outright in the EU in
   the iOS 17.4 beta (February 2024) citing the DMA, and restored them only after public protest
   and the European Commission opening scrutiny. A capability the platform owner has already
   switched off once, in exactly our market, is not a foundation for an install story.
6. **Desktop coverage fails the "all platforms" bar.** Firefox has no PWA install on Linux or
   macOS (Windows support only began appearing in 2025); Linux installs exist only through
   Chromium-based browsers. "Installable on all platforms" is simply not achievable as a PWA.

**B. Electron for desktop, plus a separate mobile framework.** Electron carries no mobile story,
so mobile would need React Native or Flutter — a second framework and a second codebase,
contradicting ADR-0030, doubling every screen, and bundling a full Chromium per desktop install
besides.

**C. Native applications per platform.** SwiftUI + Kotlin + something for three desktops:
four to five codebases rendering one design system. Out of reach for one maintainer and out of
proportion to what a shell must do here.

**D. Capacitor.** The closest cousin: web code in native shells. But it is mobile-first —
desktop support leans on wrapping Electron, inheriting option B's weight — and its runtime bridge
is JavaScript in a bundled webview per platform convention, without a shared systems-language
core for the pieces that must be native (encrypted storage, keychain access).

**E. Tauri 2 (chosen).**

## Decision

**Tauri 2 provides the installable clients on all five platforms — Windows, macOS, Linux, iOS,
Android — wrapping the one Svelte codebase from ADR-0030.** Tauri 2 has been stable since October
2024, its mobile targets (iOS, Android) ship from the same Rust core as desktop, and it uses each
platform's system webview rather than bundling one, keeping installers small and the runtime
surface the platform's own.

**There is exactly one installation path per platform: the Tauri client.** The web app remains a
pure browser application — no manifest-based install, no install prompt, no service-worker
install setup. ADR-0028's `worker-src 'self' blob:` stays: it permits web workers and any future
delivery-cache worker, none of which constitute an install story.

What follows for the offline promise: **installed clients carry it; the browser does not.** The
browser webapp speaks the same sync protocol and may hold a best-effort local cache for
resilience across brief disconnects, but ADR-0021's "fully able to work without a network" is
promised only where offline-sync.md §9's storage requirements are actually satisfiable —
encrypted local storage under the application's control, which Tauri provides (the concrete
per-platform persistence is [ADR-0033](./ADR-0033-shared-client-architecture.md)'s decision,
answering open point SY-D for first-party clients).

The shells are deliberately thin. Business logic stays in the shared TypeScript packages and the
server; Rust code in the shells is limited to platform integration (storage, keychain, window and
lifecycle handling, notifications). A shell that grows domain logic is a bug in this decision.

**Sequencing:** desktop shells first, mobile after. Tauri's mobile targets are stable but younger
than its desktop path, with rough edges around plugins and signing; letting desktop harden the
shared plumbing first is the cheap mitigation. Each Tauri plugin adopted (SQL, keychain,
notifications, updater) is a dependency decision under CLAUDE.md's rule, taken in the respective
work package.

## Consequences

* Users install a real application from real distribution channels — including the app stores on
  mobile, which the PWA route could never reach on iOS — and updates follow platform conventions
  (the updater strategy is decided with the shell work packages).
* One more toolchain enters the repository: Rust, confined to the shell directories. It joins the
  same bargain as Node ([ADR-0027](./ADR-0027-monorepo-structure.md) rule 5): a backend-only
  contributor never needs it, `go build ./...` never touches it, and CI runs it only where shell
  code changed.
* The shells talk to a Hubtask server over HTTPS like any other client; nothing is embedded
  server-side, so [ADR-0028](./ADR-0028-embedded-web-ui.md) is untouched. The server URL becomes
  user configuration in the shells (self-hosters point at their own instance).
* Tauri's system-webview approach means rendering differences between WebView2, WKWebView and
  WebKitGTK are ours to test. The design system's constraint to boring, well-supported CSS keeps
  the exposure small; the shell work packages own a webview smoke matrix.
* The platform risk shifts from "Apple may break home-screen web apps" to "app store review" —
  a known, navigable process rather than a capability that can vanish.
* arc42 §2.2 C-14's open half ("frontend feature set") is handled by
  [ADR-0032](./ADR-0032-client-capability-matrix.md); this ADR fixes the delivery vehicles.

### Backlog impact

| Work package | Target |
|---|---|
| Tauri 2 desktop shell (Windows/macOS/Linux): thin shell, storage + keychain integration, CI lane | frontend track (roadmap phase 5) |
| Tauri 2 mobile shell (iOS/Android): shell, signing, store pipeline | frontend track (roadmap phase 5), after desktop |
| Updater and distribution strategy per platform | frontend track, with the desktop shell |

No issue is created now; both packages enter `docs/roadmap.md` under the frontend track and are
cut into issues when their milestone opens.

## References

* Tauri 2.0 stable announcement (2 Oct 2024) — https://v2.tauri.app/blog/tauri-20/
* Tauri release line (2.x, current through 2026) — https://tauri.app/release/core/
* WebKit: 7-day cap on script-writable storage (ITP, Safari 13.1/iOS 13.4) — https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/
* Background Sync API unsupported in Safari/WebKit — https://caniuse.com/background-sync
* iOS PWA limitations incl. storage partitioning and install path — https://www.mobiloud.com/blog/progressive-web-apps-ios
* Apple disables EU home-screen web apps in iOS 17.4 beta — https://www.macrumors.com/2024/02/15/ios-17-4-web-apps-removed-apple/
* Apple reverses after Commission scrutiny (1 Mar 2024) — https://www.macrumors.com/2024/03/01/apple-walks-back-decision-to-disable-eu-web-apps/
* Firefox desktop PWA status (Windows-only, 2025; Linux/macOS in nightly) — https://www.ghacks.net/2025/08/22/experimental-firefox-now-supports-progressive-web-apps-on-windows/

## Notes

Related: [ADR-0030](./ADR-0030-svelte-frontend-framework.md) (the one codebase the shells wrap),
[ADR-0021](./ADR-0021-offline-sync.md) (the offline promise this makes keepable),
[offline-sync.md](../architecture/offline-sync.md) §9 and open points SY-B/SY-D,
[ADR-0032](./ADR-0032-client-capability-matrix.md) (what runs where),
[ADR-0033](./ADR-0033-shared-client-architecture.md) (how the codebase is cut so all targets build
from it), [ADR-0018](./ADR-0018-privacy-by-design.md) (why EU platform behaviour is first-order).
