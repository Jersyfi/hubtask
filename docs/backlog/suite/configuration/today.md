# The administration today — read from `main` (b832f6ceb, 2026-10-28)

Read from the code and the subject documents, not walked in a running app. Paths are the evidence.

## 1. The places

| Level | Who | Where | What is there |
|---|---|---|---|
| Installation | operator (`admin:tenants`, step-up for one hour) | `/instance` — overview, workspaces, plans, settings, providers, AI, backup targets, platform, encryption, operators, journal (`apps/webapp/src/lib/navigation.ts`) | settings with value **and lock**, a kind declared in the domain, resolved `product → installation → plan → workspace`; one versioned document read by three doors: the screen, `hubctl admin`, the instance file (`multi-tenancy.md` §4.1) |
| Plan | the purchase platform, the operator | `/instance/plans`, `/admin/plans`, `hubctl admin plan`, the instance file | values and locks in the installation's areas, three entitlements (`multi-tenancy.md` §4.2) |
| Workspace | owner, administrator (`STRUCTURE`, `MANAGE_MEMBERS`, …) | `/administration` in six groups: *Workspace and people* (workspace, presets, quotas, people, machines), *Sign-in* (sign-in, legal), *Automation* (rules, runs, webhooks, voice, mail), *Data* (private hubs, backup, retention, restore), *Records* (audit, privacy), *AI* | 27 routes; each page its own form |
| Hub, collection | whoever holds `STRUCTURE` on it | `/hubs/:id/settings/:section`, `/collections/:id/settings/:section` — general, structure, labels, fields, templates, automation, people | the eight structure settings with source mark, lock and reset; presets |
| Person | everybody | *Your settings* (`/profile/...`) | appearance, notifications, security, sessions, devices, apps, tokens; own presets; own rules |

## 2. What already works the way the whole product should

* **Value, lock, origin.** Installation settings and structure settings both answer where a value
  comes from and who locked it (P-06, P-07). A screen draws a key from its declared kind and keeps
  no list of its own (`multi-tenancy.md` §4.1).
* **One document, three doors** — for the installation only: the instance file (`seed` /
  `enforce`), `hubctl admin`, the screens, with the same refusals and an `ETag` version.
* **Every use case is an MCP tool** generated from the registry (`presentation/mcp/ToolRegistry.go`),
  under the same permissions (P-08). An agent can already change anything a person can — one call
  at a time.
* **Preview before narrowing.** A structure change that would hide something is previewed
  (`PreviewContainerStructure`); a plan move has a dry run.
* **Presets in four origins** — Hubtask, installation, workspace, own (UC-WRK-30, UC-INS-18) — and
  narrowing hides, never deletes (decision #1620).

## 3. Where it already splits — the same failure the owner describes in the market

| # | Split | Evidence | Consequence for a person |
|---|---|---|---|
| S-1 | **Three configuration mechanisms.** Installation settings (kind-declared keys), structure settings (layers), and about twenty workspace pages each with its own form and save | `InstanceSettingsView.svelte`, `StructurePanel.svelte`, `SignInSettingsView.svelte`, `AiSettingsView.svelte`, `MailView.svelte` … | the same question — "where does this value come from, can I change it, who locked it" — is answered on some pages and not on others |
| S-2 | **A file only for the installation.** A workspace's configuration (structure, fields, rules, views, sign-in, AI, mail) has no document to export, compare or apply | instance file in `multi-tenancy.md` §4.1; `tenant-export.md` exports data, not configuration | an administrator or an agent cannot copy a set-up to a second workspace, review it, or roll it back |
| S-3 | **Rules in two places.** A rule's scope is the workspace, a hub or a collection (`automation.md` §1), managed in `/administration/rules` and again in a collection's *Automation* section | `automation.md` §1; `ContainerView.svelte` section `automation` | two lists that look different for one thing |
| S-4 | **Fields without a form.** A custom field has `required` and nothing else; *Flow* adds one gate (`REQUIRED_BEYOND_FIRST_COLUMN`). There is no order, section, condition or per-role read-only — so there is nothing yet that could split, and nothing yet that could be logic | `domain-model.md` §3.5, `FlowGate` | the moment the service desk needs a form with logic, the market's mistake (a simple create screen beside a rich form) is one step away |
| S-5 | **Fixed roles.** Seven roles; permission *what* is fixed, only *where* varies | `domain-model.md` §3.2 | a company cannot say "only the change manager approves changes" |
| S-6 | **No "who may configure this" view.** The matrix of setting × level × role exists in the code (declared kinds, `STRUCTURE`, locks), not on a screen | — | an administrator learns the boundaries by meeting refusals |
| S-7 | **Templates stop at structure.** A preset carries the eight structure values; item templates carry entry trees; neither carries fields, rules, views, pages or roles together, and a preset is copied without a version, so it cannot be upgraded | `domain-model.md` §2 "nothing follows a preset afterwards" | no "ITIL service desk" or "Scrum team" in one step, and no improvement of one later |

None of S-1 … S-7 is a defect today; each is where the suite would grow a second solution if the
model is not settled first.
