---
id: UC-SYN-07
title: Install the app on my computer or phone and keep it current
context: sync
actors: [PE-person, PE-member, PE-selfhoster, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-05, P-09, P-11, P-13]
state: specified
tasks: [F7-02, F7-03, F7-06, F7-08, F7-09, F7-10]
checked_by: []
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

* Check 1: not met — releases publish the server's image and chart only; there is no installed app.
* Check 2: not met — the download page says there are no installed apps yet; it offers them with `0.9.5` (#1353).
* Check 3: not met — there is no installed app, and a passkey cannot be used from a webview's origin.
* Check 4: not met — there is no installed app.
* Check 5: not met — there is no updater.
* Check 6: not met — there is no Android app.
* Check 7: not met — no build leaves administration out.
* Check 8: not met — there is no Android app.
* Check 9: not met — there is no smoke walk in a webview.
