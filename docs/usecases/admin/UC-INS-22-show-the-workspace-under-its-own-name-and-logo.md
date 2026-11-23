---
id: UC-INS-22
title: Show the workspace under its own name and logo
context: admin
actors: [PE-owner, PE-admin, PE-member, PE-guest, PE-operator, PE-platform]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-06, P-07, P-08, P-09, P-12]
state: partial
tasks: []
checked_by: [core/domain/model/identity/Branding_test.go, core/domain/model/identity/OwnBrandingSwitch_test.go, core/domain/model/media/Object_test.go, core/application/service/identity/WorkspaceBranding_test.go, core/application/service/media/ReadMedia_test.go, test/integration/workspace_branding_test.go, test/contract/workspace_branding_test.go, core/application/service/identity/SignInPictures_test.go, core/application/service/identity/WorkspaceAnnounce_test.go, core/application/service/setting/ModuleRecords_test.go, presentation/rest/WorkspacePictureController_test.go, presentation/rest/SignInRulesController_test.go, test/integration/workspace_look_record_test.go, test/integration/erasure_shown_pictures_test.go]
---

# Show the workspace under its own name and logo

## Goal

A team, a family or a company works under its own name and logo: its members and the people it
invites meet the workspace, not the tool. The installation and a plan allow it by default and can
switch it off; the product's name stays readable where somebody looks for what software runs.

## Story

The Unterlindenbach choir uploads its round emblem as a symbol and its banner as a logo, and
chooses its own name for the header. The members see the choir's mark when they open the app,
on the phone and the laptop, in light and in dark. A provider that hosts workspaces for clubs
leaves it allowed; a company's platform team that wants every workspace to read alike switches it
off for the installation, and every settings screen says that the installation decided it.

## How to check

1. Whoever may change the workspace's name sets a logo for the light theme, a logo for the dark
   theme and a square symbol; the workspace keeps each and every member reads it.
2. A file that is not a PNG, JPEG, GIF or WebP image - an SVG included - or that is larger than
   5 MB is refused with a stated reason, and the workspace's pictures stay as they were.
3. Whoever may change the workspace's name chooses what the header shows: the product's name, the
   workspace's name, or the logo alone; a workspace that chose nothing shows the product's name, as
   before.
4. The logo alone cannot be chosen while the workspace has neither a logo nor a symbol, and
   removing the last picture returns the header to the product's name.
5. Every member sees the choice in the header, on the web, the desktop app and the phone, in light
   and in dark.
6. Every member sees the workspace's symbol in the navigation and on the current workspace's row of
   the workspace switcher; the other workspaces in the switcher keep their letter.
7. A member signed in on another device sees a change after their next sync, without signing in
   again.
8. The sign-in card at the workspace's address shows what the header's choice says.
9. The browser tab's title names the workspace.
10. The workspace's settings show the installation's and the plan's default and lock beside the
    setting.
11. Where the installation or the plan switched it off, every change to the pictures and the choice
    is refused, and the refusal says who switched it off.
12. The product's name and version stay readable on the about page.
13. Every change to the pictures and the choice is in the workspace's trail, naming the media
    object and never its file name or content.

## Where it ends

No colours and no fonts of a workspace's own. No app icon, store name, splash or favicon of a
workspace's own, and no logo in the head of a mail. No branding the installation sets for every
workspace at once. No SVG. Other workspaces in the device's list of workspaces keep their letter.

## Today

* Check 5: not met — the header draws only the product's mark, tracked in #1868.
* Check 6: not met — the navigation and the switcher draw no symbol, tracked in #1868.
* Check 7: not met — the web app does not read the workspace again on its `setting` record, tracked in #1868.
* Check 8: not met — the sign-in card does not draw the choice and the pictures the rules carry, tracked in #1868.
* Check 9: not met — the tab's title is fixed, tracked in #1868.
* Check 10: not met — the settings screen has no branding group, tracked in #1868.
* Check 12: not met — the about page draws no line with the product's name beside a workspace's own, tracked in #1868.
