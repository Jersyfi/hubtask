# Module 7 — Enterprise administration (`UC-ID`, `UC-INS`, `UC-AUD`)

> **Background** — the concept of 2026-10-17; where it differs from [`../products.md`](../products.md),
> that file wins ([`README.md`](./README.md)).

## 1. Goal

A company with thousands of people and a compliance department runs Hubtask the way it runs its
other critical systems: its directory decides who exists, its own roles decide who may do what, its
SIEM sees what happened, and a change to the configuration is tested before it reaches everyone.

## 2. Parity and gaps

| Capability (market) | Hubtask today | Proposed |
|---|---|---|
| SAML 2.0 SSO | OIDC; SAML after 1.0 (`NG-saml-before-1`) | SAML in `1.1` (already planned) |
| SCIM 2.0 provisioning, deprovisioning, groups | After 1.0 | SCIM users and groups in `1.1` |
| Custom roles, permission schemes | Seven fixed roles | **Permission schemes**: a role is a named set of permissions from the closed catalogue (`READ`, `WRITE`, `STRUCTURE`, `MANAGE_MEMBERS`, `TRANSITION:<id>`, …); the seven roles are the shipped scheme; "nobody grants above their own role" generalises to "nobody grants a permission they do not hold" — **decision D-05** |
| Field-level security (hide a salary field) | — | A field scheme marks a field visible/editable per role |
| Issue-level security (security levels) | Shares at `ITEM` scope, private hubs | Security levels per entry within a collection, a narrowing like private hubs |
| Admin audit log, export, retention | Hash-chained trail, verify, anchor, export | + **Streaming to a SIEM** (syslog/RFC 5424 over TLS, or a webhook in a CEF-like JSON) |
| IP allowlists, session policies, device trust | Session rules (ADR-0068) | IP allow/deny per workspace; already-strong session rules |
| Data residency | Self-hosted: wherever you run it | Unchanged — the strongest answer in the market |
| BYOK / customer-managed keys | Sealed secrets, key rotation | Per-workspace key from an external KMS (decision later) |
| Sandbox / staging, promote configuration | — | **Configuration as code**: workflows, types, schemes, forms, rules, roles exported and imported as a versioned file; a *sandbox workspace* copied without content |
| Bulk admin: user lifecycle, licence-like seat views | People admin, managed accounts | Admin insights: active people, inactive accounts, module usage — numbers, no content (P-01) |
| Compliance artefacts (SOC 2, ISO 27001) | Data catalogue, TOMs, gates | For self-hosters a *control mapping* (which product control supports which ISO 27001 Annex A control), never a certification claim |
| Release tracks (rapid / managed) | One version | Unchanged; operators pin |
| Legal hold, eDiscovery | Legal hold, export | eDiscovery search across a workspace for the auditor role, content shown only under a hold and recorded — **decision** (touches P-01-like content boundaries for auditors) |

## 3. Use cases (titles)

ID-26 Sign in through our SAML directory · ID-27 Let our directory create, change and remove
people and groups · ID-28 Define our own roles from the permissions Hubtask knows ·
ID-29 Hide a field from those who must not see it · AUD-08 Stream the trail to our security
monitoring · INS-22 Keep a workspace's configuration in a file and apply it elsewhere ·
INS-23 Try a configuration change in a copy before everybody gets it · ID-30 Allow sign-in
only from our networks.
