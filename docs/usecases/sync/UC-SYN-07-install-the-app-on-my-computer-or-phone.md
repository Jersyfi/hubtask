---
id: UC-SYN-07
title: Install the app on my computer or phone and keep it current
context: sync
actors: [PE-person, PE-member, PE-selfhoster, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-05, P-09, P-11, P-13]
state: partial
tasks: [F7-02, F7-03, F7-06, F7-08, F7-09, F7-10]
checked_by: [core/application/service/identity/AppClient_test.go, test/integration/app_client_test.go, apps/webapp/src/lib/platform/appSession.test.ts, apps/webapp/src/lib/signin/appSignIn.test.ts, apps/webapp/src/lib/signin/keyRelay.test.ts, apps/webapp/src/lib/signin/keyWait.test.ts, apps/webapp/e2e/apppasskey.test.mjs, apps/webapp/src-tauri/src/signin.rs, apps/webapp/src/lib/platform/appWorkspaces.test.ts, apps/webapp/build/check-mobile-bundle.test.js, apps/webapp/src/lib/platform/target.test.ts, apps/webapp/src/lib/navigation.test.ts, apps/webapp/src/lib/webApp.test.ts, apps/webapp/src/lib/entries/entryMenu.test.ts, apps/webapp/build/release-apps.test.js, apps/webapp/src-tauri/src/update.rs, apps/webapp/src/lib/platform/update.test.ts, apps/webapp/src/lib/frame/appupdate.test.ts, apps/webapp/e2e/webview/smoke.mjs, apps/webapp/src-tauri/src/walk.rs]
---

# Install the app on my computer or phone and keep it current

## Goal

A person installs Hubtask on Windows, Linux or Android without a store and without paying anybody,
can tell that the download is the project's own, signs in to their workspace there with any way
the workspace allows, and the app stays current without them having to look for a new version.

## Story

The family's technical parent downloads the Windows installer from the project's releases, compares
its checksum and accepts the system's warning about an unknown publisher, which the download page
explained. The app asks for the workspace's address and opens the workspace's sign-in card in the
system browser; after the passkey prompt, the app is signed in. Weeks later it offers the new
version and installs it. On the Android phone the same works with the APK; the household's
settings are not in the app, and the menu says they are in the web app, with a link.

## How to check

1. Each release of the project offers a Windows installer, a Linux AppImage and `.deb`, and an
   Android APK as direct downloads, each with its checksum, and lists no store.
2. The download page names each file with its checksum, says that the apps are in no store before
   `2.0`, and explains the operating system's warning about an installer without a commercial
   signature.
3. On first start the app asks for a workspace's address; signing in there offers every way in the
   workspace allows, a passkey included, and the app's session is kept in the platform's keystore,
   not in a file.
4. The app adds a workspace of the same or of another installation by its address, and signs in
   to each on its own.
5. The Windows and Linux app finds a newer release by itself, checks it against the project's
   update key, and installs it after the person agrees; an update whose signature does not match is
   refused and the app says so.
6. The Android app says when a newer release exists and opens its download.
7. In the Android app the workspace's administration and the installation's area are not offered;
   where they would be, the menu names them and opens the web app of the server the app is signed
   in to.
8. In the Android app the system's back gesture goes back in the app's history, and sharing an
   entry opens the system's share sheet.
9. The same smoke walk - sign in, open a hub, create, edit and complete an entry, sign out - passes
   in the Windows app on WebView2, the Linux app on WebKitGTK and the Android app on the system
   WebView.

## Where it ends

* No store listing, no macOS and no iOS app before `2.0`
  ([ADR-0084](../../adr/ADR-0084-the-first-majors-shells-cost-nothing-to-ship.md)).
* No commercial code-signing certificate: the operating system's warning is the price of a free
  download, explained, not hidden.
* No administration on the phone ([ADR-0032](../../adr/ADR-0032-client-capability-matrix.md)); the
  desktop apps have it all.
* What the app keeps offline and how it merges is UC-SYN-01 to UC-SYN-05; the list of workspaces
  is UC-ID-25.

## Today

* Check 1: not met until a release runs — `release.yml` builds, signs and publishes every app with one checksum file, and stops before publishing while the project's two keys are not created and recorded (versioning-release.md §8); the first release with the apps is the acceptance's run.
* Check 2: not met — the download page says there are no installed apps yet; it offers them with `0.9.5` (#1353).
* Check 3: met on the desktop, short of the walk through a password, a provider and a virtual passkey in the built app; a step-up that wants a passkey goes through the system browser too. On Android the session and the copy's key are sealed under the Android Keystore and the code comes back to `eu.hubtask.app:/oauth`, built and not yet walked on a device; a step-up at a provider still leaves the app's window rather than going through the system browser.
* Check 4: met on the desktop - *Add a workspace* by address, a session per workspace in the keystore and a copy each; the Android app has the same list, as a sheet from the bottom, not yet walked on a device.
* Check 5: met in the build, short of one updater run from a released version to its successor (`1.0.0` prerequisite 19), which waits for the update key (versioning-release.md §8) — the Windows app and the AppImage look for a newer release at start through `update.json`, check the manifest and then the package under the update key before they offer *Install and restart*, and a manifest or a package with another signature is refused and named (a shell test); the `.deb` names the newer release and opens its download. The key in the shell's configuration is a placeholder a release refuses until the owner records the key.
* Check 6: met in the build — the Android app reads the release's manifest, names a newer release on *Today* and in *About Hubtask*, and opens the release's page, where the APK stands beside its checksum; not yet walked on a device - the smoke walk of check 9 does not reach the notice.
* Check 7: met in the build — the Android bundle carries no screen of the administration or the installation's area (a build test), and the account menu names both with the web app's address and opens it there; not yet walked on an emulator - the smoke walk of check 9 does not open the menu's affordance.
* Check 8: partly met — *Send link…* hands the entry's title and link to the system's share sheet, and the back gesture goes back in the webview's history as the shell's activity does; both are built and not yet walked on an emulator - the smoke walk of check 9 uses neither.
* Check 9: met in the build, short of the lane's first green run on Windows and Android — the `shell` lane walks each app in its own engine, WebView2, WebKitGTK and Android's system WebView on an emulator, through the engine's WebDriver against a server of the same commit (`apps/webapp/e2e/webview/smoke.mjs`), and reads each write back from the server; the system's browser is stood in for by the walk build's `walk` feature, the consent answered through the API the consent screen calls. A failure in one engine fails the lane. Walked green on WebKitGTK outside CI, at a desktop's and a phone's width; WebView2 and the emulator run only in the lane.
