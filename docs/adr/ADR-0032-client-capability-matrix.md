# ADR-0032 — The client capability matrix: parity by default, restrictions justified one by one

**Status:** accepted · **Date:** 2026-08-23

## Context

arc42 §2.2 C-14 left the frontend feature set open. With the delivery vehicles decided —
the browser app ([ADR-0028](./ADR-0028-embedded-web-ui.md),
[ADR-0030](./ADR-0030-svelte-frontend-framework.md)) and the Tauri clients
([ADR-0031](./ADR-0031-tauri-app-shell.md)) — "which client can do what" needs an answer, and it
needs to be a *decided* answer: feature distribution across clients is the kind of thing that
otherwise gets decided by accident, one screen at a time, until nobody can say why mobile lacks
something.

The starting position is unusually favourable: all clients render **one Svelte codebase**
([ADR-0033](./ADR-0033-shared-client-architecture.md)), so parity is the cheap default rather
than triple work. C-13 (no feature crippling of the self-hosted variant) sets the spirit:
restrictions are the exception and carry the burden of proof.

Three feature areas structure the decision:

* **End-user features** — everything in the use case catalogue a member uses to work: hubs,
  collections, items, comments, labels, views, search, notifications, offline sync.
* **Profile configuration** — the account's own settings: locale, time zone, theme, notification
  preferences, reminders defaults, device management, own security (password, MFA enrolment,
  sessions).
* **Administration** — tenant-level operation: tenant settings, member and role administration,
  quotas, backup and restore, retention policies, audit query and export, data subject requests,
  OIDC/security policy, licensing, automation rule administration.

## Options

**A. Full parity everywhere, no exceptions.** The purest reading of "all functions in all
clients". Rejected only where a concrete, named cost exists — see the matrix; for desktop it is
in fact the decision.

**B. A "companion app" model — mobile as a reduced capture-and-check client.** The common
industry pattern. Rejected: it institutionalises a second-class client, invites per-feature
bargaining forever, and contradicts the product promise. Mobile is a full working client for
end-user features, not a satellite.

**C. Parity as the default, with administration web-only as the single permitted fallback
(chosen), applied per area with individual justification.**

## Decision

**Parity is the default. Any feature not explicitly restricted in the matrix below ships in every
client, and a new feature ships everywhere unless it states and justifies its row here.**

| Area | Web | Desktop (Tauri) | Mobile (Tauri) |
|---|---|---|---|
| End-user features | ✅ full | ✅ full | ✅ full |
| Profile configuration | ✅ full | ✅ full | ✅ full |
| Administration | ✅ full | ✅ full | ⬈ via web, with one exception (below) |

**The web app always carries the complete feature set** — end-user features *and*
administration. It is the client every deployment has (embedded in the binary), the one that
needs no installation, and therefore the floor under everything else.

**Desktop is full parity, administration included.** The desktop shell renders the same bundle on
the same screen class as the browser; excluding administration there would cost a deliberate gate
in shared code and buy nothing. No restriction is justified, so none exists.

**Mobile ships end-user features and profile configuration in full; administration is not built
for the phone form factor and is reached via the web app instead.** This is the one restriction,
and its justification, per the rule, is explicit:

1. **Administration is online-only by nature.** [offline-sync.md](../architecture/offline-sync.md)
   §1 already excludes permissions, automation administration, backups, restores, exports and
   tenant administration from offline operation — the defining strength of the installed mobile
   client is exactly the part administration cannot use.
2. **High consequence on the smallest screen.** Restores, retention policies, role changes and
   tenant deletion are rare, deliberate, high-blast-radius operations that want step-up
   authentication ([roadmap](../roadmap.md) `0.6.0`) and room to read — the desk, not the queue
   at the supermarket.
3. **Admin surfaces churn against store review cadence.** Administration grows with every
   operational milestone (backup targets, retention chains, audit tooling). On mobile each
   iteration rides an app-store review cycle; on the web it ships with the server it administers
   ([ADR-0028](./ADR-0028-embedded-web-ui.md) makes UI and API the same commit).

The exception within the exception: **member and role management at container scope** (inviting
someone to a hub, changing a member's role on a collection) is an end-user collaboration feature,
not tenant administration, and follows the parity row — it ships on mobile. The line runs between
"working with people in my containers" and "operating the tenant".

Mechanically, mobile does not hide the tenant: administrative areas appear as navigation entries
that state where the capability lives and link out to the deployment's web app (the URL is known —
it is the server the client is signed into). A capability that exists but lives elsewhere is
shown as exactly that, in the spirit of the design system's `CapabilityGate` ("a disabled field
with a reason").

Client capability is a **presentation-layer concern only**. The API serves every operation to
every authenticated client uniformly ([ADR-0004](./ADR-0004-api-first-openapi.md)); nothing
server-side knows which client is asking, and rule 14 of CLAUDE.md stands. The matrix governs
what UI is built, never what the API permits.

## Consequences

* "Why doesn't mobile have X?" has a written answer with exactly one legitimate form: X is tenant
  administration. Anything else on that question is a bug against this ADR.
* Every future feature declares its area; features in the end-user and profile areas ship to all
  three clients from the same code. The Definition of Ready in
  [engineering-guidelines.md](../architecture/engineering-guidelines.md) §2 gains a line item
  ("client availability: which area of the capability matrix, and — if restricted — the
  justification") when this ADR is accepted.
* The mobile bundle carries no admin screens, which keeps its surface, its review scope and its
  test matrix smaller — the benefit that pays for maintaining the one restriction.
* If the restriction ever proves wrong (a real operator population administering primarily from
  phones), the fallback is cheap in exactly one direction: the code exists and is shipped by
  including it, per supersede. The reverse — clawing back a shipped surface — would be expensive,
  which is why the restrictive default is taken now.
* A future third-party client is bound by none of this; the API is uniform. The matrix is a
  first-party product decision, not a contract term.

### Backlog impact

| Work package | Target |
|---|---|
| Capability-matrix implementation: area tagging in the webapp's navigation/routing so the mobile build excludes admin routes, plus the "lives on the web" affordance | frontend track (roadmap phase 5), with the mobile shell |
| DoR amendment in engineering-guidelines.md §2 (client-availability line item) | with acceptance of this ADR (documentation change, no issue) |

No issue is created now; the implementation package enters `docs/roadmap.md` under the frontend
track.

## Notes

Related: [ADR-0030](./ADR-0030-svelte-frontend-framework.md) and
[ADR-0033](./ADR-0033-shared-client-architecture.md) (why parity is cheap),
[ADR-0031](./ADR-0031-tauri-app-shell.md) (the clients this distributes over),
[ADR-0028](./ADR-0028-embedded-web-ui.md) (why the web app is the floor),
[offline-sync.md](../architecture/offline-sync.md) §1 (the online-only operations),
arc42 §2.2 C-13/C-14, CLAUDE.md rule 14 (the API stays client-blind).
