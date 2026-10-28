# Module 8 — Platform & ecosystem (`UC-INT`, `UC-AI`, `UC-REC`, `UC-CNV`)

> **Background** — the concept of 2026-10-17; where it differs from [`../products.md`](../products.md),
> that file wins ([`README.md`](./README.md)).

## 1. Goal

Hubtask is where the rest of an organisation's tools meet the work: code links itself to the entry
it implements, the organisation's chat hears what matters, apps add what Hubtask does not do — and
agents do real work under the same permissions as a person, on the organisation's own machine.

## 2. Parts

### 2.1 Connections that count most for a company switching over

| Connection | Proposed |
|---|---|
| Code hosting (self-hosted and public forges) | Commits, branches, merge requests and pipelines that mention an entry key appear on the entry; a merge can take a transition (workflow). Via inbound webhooks + OAuth apps; no forge SDK in core |
| Chat (Matrix first — the EU public sector's choice — then the common commercial ones) | Notification channel (UC-NOT-06) + slash-command-like actions through the API |
| Calendar | CalDAV exists; add meeting-to-notes (an event links a notes page) |
| Files (WebDAV/Nextcloud-like stores, S3) | Link a folder to a hub; attachments by reference |
| Identity | SAML/SCIM (enterprise module) |

### 2.2 Apps — out of process (`NG-in-process-plugins`)

* An **app manifest**: name, the scopes it asks, the webhooks it subscribes, the **UI extension
  points** it fills (a panel on an entry, an action in a menu, a dashboard widget) — rendered by the
  web app in a sandboxed frame with a narrow message API, never script in the app's own page.
* Installed per workspace by an administrator, through the OAuth-app consent that exists
  (UC-INT-08). No marketplace operated by the project (`NG-phone-home`); a catalogue file an
  operator may point at.

### 2.3 AI agents (optional — P-14, `NG-ai-required`)

* **Agents with a job**: triage the jumble, draft a status update, write release notes, summarise
  a page, answer from the wiki with sources, break an epic into work packages — each a suggestion
  a person accepts (UC-AI-04 pattern), each through the same use cases and trail as `AI_AGENT`.
* **Agent as a member**: an agent can be assigned an entry and works through MCP with a service
  account — its own role, its own budget (UC-AI-07), stopped by an administrator at once.
* No content leaves for a model the workspace did not consent to (UC-AI-03).

### 2.4 Records — custom entities (`UC-REC`)

A record type is an item type without completion (contacts, assets, vendors, contracts, risks), with
fields, statuses (optional) and relations to entries. **Decision D-07**, because `COMPLETION` is
mandatory today. Covers: risk register, asset list (service desk), contact list for a club.

### 2.5 Whiteboards (`UC-CNV`)

An infinite canvas per hub: sticky notes, shapes, connectors, frames, images; a sticky note becomes
a task; real-time like pages (same CRDT). Last wave — valued in workshops, rarely decisive in
procurement.

## 3. Use cases (titles)

INT-12 See the code that implements an entry · INT-13 Hear about our work in our chat ·
INT-14 Add an app to our workspace and see what it may do · AI-10 Let an agent take on an
entry and report back · AI-11 Ask our wiki a question and see where the answer came from ·
REC-01 Keep a list of things that are not tasks · REC-02 Relate records and entries ·
CNV-01 Think on a whiteboard and turn notes into tasks.
