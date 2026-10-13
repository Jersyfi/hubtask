# ADR-0084 — The first major's shells cost nothing to ship

**Status:** accepted · **Date:** 2026-10-13

**Rule lives in:** [roadmap.md](../roadmap.md) (F7, `0.9.5`, the `1.0.0` prerequisites),
[support-matrix.md](../architecture/support-matrix.md) §7

## Context

[ADR-0031](./ADR-0031-tauri-app-shell.md) chose Tauri 2 for installable clients on five platforms
and the roadmap tied `1.0.0` to all five: signed installers for Windows, macOS and Linux and live
store listings for iOS and Android (prerequisite 19), the webview matrix across WebView2,
WKWebView and WebKitGTK (prerequisite 18), and store review started in `0.9.5`.

Two of those platforms cannot be shipped without paying: macOS outside the App Store needs an
Apple Developer Program membership for signing and notarisation, and iOS has no distribution
outside the App Store at all. The stores themselves (App Store, Google Play, Microsoft Store) each
charge a fee or require a paid account. F7 was left uncut precisely because "no shell task can
close without" those accounts. Hubtask is Apache-2.0 with nothing sold
([ADR-0080](./ADR-0080-hubtask-is-apache-2-0.md)); a release that depends on recurring fees to
third parties is a release that can be blocked by them.

## Decision

The owner decided on 2026-10-13:

* **Up to `2.0`, the shells are the ones that can be built and installed without cost:** Windows,
  Linux and Android, distributed **outside every store** — as direct downloads from the project's
  releases (Windows and Linux installers, an Android APK signed with the project's own key), kept
  current by Tauri's updater, which signs with a key of its own and needs no account.
* **macOS, iOS and every store listing come with `2.0`**: Apple signing and notarisation, the App
  Store, Google Play and the Microsoft Store. The milestone *2.0.0 — The shells in the stores*
  holds that work.
* The webview matrix of `1.0.0` is WebView2 and WebKitGTK, plus Android's system WebView;
  WKWebView joins with the macOS and iOS shells. The web app stays tested on WebKit in the
  browser lane as before.

## Consequences

* F7 can be cut and closed without anybody buying an account: *F7 — The shells, free to install*.
* An installer that is not code-signed by a commercial certificate meets the operating system's
  warning (SmartScreen on Windows, "unknown sources" on Android). The download page says so and
  publishes checksums and the signature that the updater checks; the warning is the price of the
  decision, not a defect.
* Client parity (`1.0.0` prerequisite 15) is demonstrated on web, Windows, Linux and Android; macOS
  and iOS users use the web app until `2.0`.
* ADR-0035's platform build counter stays: Android needs `versionCode` for its own updates even
  outside a store.
* ADR-0031 stands for the framework and the closed PWA path; only its five-platform promise moves
  in time.
