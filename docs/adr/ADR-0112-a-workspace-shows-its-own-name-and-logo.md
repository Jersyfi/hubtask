# ADR-0112 — A workspace shows its own name and logo

**Status:** accepted · **Date:** 2026-11-23 · **Decided:** 2026-11-22, by the owner (#1868)

**Rule lives in:** [identity.md](../architecture/identity.md) §18, and
[multi-tenancy.md](../architecture/multi-tenancy.md) §4.1

## Context

Every header said the product's name, whatever workspace it belonged to. A team, a family or a
company wants its own name and logo where it works, and invited clients should meet the company,
not a tool. The licence draws no line here any more: ADR-0059 is superseded by
[ADR-0080](./ADR-0080-hubtask-is-apache-2-0.md), the product is Apache-2.0, and the only boundary
left is the name and the logo ([TRADEMARK.md](../../TRADEMARK.md), Apache-2.0 §6). A paid hosted
service may not use the product's name without permission, so replacing it is the compliant path
for that party, not a loophole.

## Decision

* **Three pictures and one choice per workspace.** A logo for the light theme, one for the dark
  theme and a square symbol; the header and the sign-in card show the product's name (the
  default), the workspace's name, or the logo alone. The logo alone needs a picture - a logo, or
  the symbol it falls back to - and taking the last picture away returns the choice to the
  product's name in the same write.
* **The pictures in a table of their own**, one row each keyed into the workspace's media and
  counted by the media reconciliation; the choice is a key of the workspace's settings. Not a
  reference inside the settings document: it would have no key and no count, and the
  reconciliation would remove the picture. Not three keys on the workspace's row: a restore writes
  that row before the media, so the keys could not be restored.
* **Raster only.** A PNG, JPEG, GIF or WebP image judged from its bytes, at most 5 MB, judged when
  it is set and at every read. A list of its own, not the inline list a desk's logo and a person's
  picture use, which also holds the video and audio a browser plays. No SVG: a browser may execute
  one (`security.md` T-11).
* **Allowed by default, lockable above.** *The workspace may show its own name and logo* is the
  sign-in rule's switch `own_branding`, levelled as *Private hubs allowed* is: on by default, off
  the stricter value, the installation and a plan switch it off and lock it. Off refuses every
  change to the pictures and the choice with `workspace.branding_off`, naming the level that
  switched it off, and a client shows the product's name; what is stored stays.
* **Whoever may change the workspace's name** sets the pictures and the choice (STRUCTURE at the
  workspace). Every change is in the workspace's trail with the media object's identifier, never
  its file name or content.
* **Before sign-in, at the workspace's own address only.** The sign-in rules carry the choice and
  the pictures it draws, and one public read serves exactly those pictures, streamed by the
  application as the image they were stored as, under an address that changes with the picture.
  Only where the address itself names the workspace: the installation's own address, a header
  naming a workspace included, shows the product's mark, so the read tells nobody more than the
  rules at that address already do (`security.md` T-34).
* **Other devices learn of it through the workspace's `setting` record** (decided 2026-11-23, O3).
  A change of the name, a picture, the choice or the switch writes the record the module switches
  write, with the same content and the same receivers
  ([ADR-0108](./ADR-0108-a-module-switch-and-a-board-comment-are-change-records.md)), and a
  client reads the workspace again on it. Not a record kind of its own: more protocol for one read.
* **Not in this cut:** an installation-wide branding, colours and fonts, a favicon, a mail head,
  the installed apps' names and icons. The product's name and version stay readable on the about
  page.

## Alternatives

* *A setting of its own beside the sign-in rule*: a second place for three levels, a lock and a
  source that the rule already answers.
* *SVG with sanitising*: a sanitiser is a new dependency and a parser in the request path, for a
  format a raster export covers.
* *The operator's branding for every workspace*: a media object no workspace owns, the costliest
  part, and asked for by nobody yet.

## Consequences

The workspace's answer carries `branding`; the media usage `WORKSPACE_LOGO` exists; every member
reads the pictures the workspace shows. The archive carries the pictures after the media
(`workspace_pictures`) and the choice with the workspace's settings. `SignInRules` carries
`workspace_header` and `workspace_pictures`, `GET /auth/workspace-pictures/{mediaId}` serves them,
and the workspace's `setting` record is written at every such change. The configuration document
carries the switch and not the choice, which names pictures no second workspace holds.
