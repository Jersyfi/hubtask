# Research: how a workspace is addressed, renamed, found and switched (2026-10)

**Kind:** concept input. This is market research and a proposal, not a rule. It feeds the cut of a
concept with the owner ([README](./README.md), "Cutting a milestone"). The rules stay where they
are, in [multi-tenancy.md](../architecture/multi-tenancy.md) and
[identity.md](../architecture/identity.md), until a cut changes them.

**Why it lives here.** [`docs/README.md`](../README.md) has no `research/` directory, and this
report holds no current rule. The backlog holds the way from a concept to a milestone, and this
report is the input to one. Once the concept is cut or dropped, it moves to `docs/archive/`, as the
market analysis did.

**Researched:** 2026-10-16, from vendors' own documentation and source code where possible.
**[secondary]** marks a source that is not the vendor's own. **[unverified]** marks a claim no
source confirmed.

---

## 0. The owner's three questions, and the short answers

| # | Question | Short answer |
|---|---|---|
| 1 | Workspaces with or without a subdomain, chosen by the operator. Where subdomains are on, may a workspace's subdomain change? | **Yes to the choice, but not as a path prefix.** "Without a subdomain" is already single mode, and should become "a host the operator assigns" in multi mode. A path per workspace (`hub.example/w/acme`) would put every workspace in one browser origin. Hubtask's accounts, passkeys and offline caches rely on that origin boundary. **Yes to renaming**, built on the `tenant_host` model: the old host stays as a redirect, and the old name stays held. Everything Hubtask needs for this is the same mechanism milestone CD needs. |
| 2 | Can one address have accounts in several workspaces? How does a person switch? | **Yes, by design**: accounts are per workspace (`account_email_uq` on `(tenant_id, lower(email))`). Switching should be a **device-local list** of the workspaces signed into on this device. [`NG-global-identity`](../vision/non-goals.md) already names this list. It is not a server-side link between accounts. |
| 3 | Must a person know the subdomain? Idea: a general domain with an email field that routes you on. | **Not on screen.** Routing an address straight to its workspace tells anybody who types it where that address has an account, which breaks [P-01](../vision/principles.md#p-01-workspaces-never-see-each-other). The market's safe form, and Zulip's, is **"Find your workspaces"**: the screen gives the same answer for every address, and the list of workspaces goes **by mail** to that address. Add two more things at the general domain: "Go to your workspace" (type the short name) and the device's own list. Routing by **email domain** (not by address) is possible later, only for a workspace that proved it owns the domain. |

---

## 1. What exists today (the premise, checked against `main` on 2026-10-16)

| Fact | Where |
|---|---|
| Two modes. `single` has one workspace at the installation's host. `multi` puts each workspace at `<slug>.<base host>`, with `X-Hubtask-Tenant` as the second source. A path prefix is "deliberately not used". | [multi-tenancy.md](../architecture/multi-tenancy.md) §1, §3 |
| A sign-in resolves its workspace **before** any credential. Resolution answers one identifier or none, never a list. A host without a workspace learns nothing: `/auth/sign-in-rules` answers byte for byte what an undecided workspace answers. | [identity.md](../architecture/identity.md) §3 |
| Accounts belong to one workspace. One address in two workspaces is two accounts, with separate passwords, factors and sessions. | identity.md §2; [`NG-global-identity`](../vision/non-goals.md) |
| `NG-global-identity` already says: "Each workspace at its own address; a workspace switcher may list the person's own accounts on one device". | [non-goals.md](../vision/non-goals.md) |
| UC-ID-01's *Where it ends*: "The address is not remembered on the device; only the last used *method* is." "Choosing a workspace on the sign-in screen is not offered: the host decides the workspace". | [UC-ID-01](../usecases/identity/UC-ID-01-sign-in-with-a-password.md) |
| One refusal for every address; the reset gives the same `202` and does the same work for every address. The attempt ledger counts per address and per IP class. | identity.md §4.3, §7 |
| `tenant_host` holds the hosts a workspace answers at: one canonical, states `PENDING → VERIFIED → ACTIVE → BROKEN`, a DNS verification mark, and one host per workspace installation-wide. Nothing resolves through it yet; `resolve_tenant` reads the slug. | `db/migrations/0104_tenant_host.sql`, `core/domain/model/identity/TenantHost.go` |
| The slug is `^[a-z0-9][a-z0-9-]{2,39}$`, unique, and cannot be changed through any route. The contract says renaming is "the operator's operation at `/admin/tenants`", but no such route exists. | `api/openapi.yaml` (`TenantProvision`, the workspace settings description) |
| **No reserved short names.** UC-INS-07 check 2 says "taken, reserved or not a valid address label is refused", but nothing in `core/` reserves `www`, `api`, `auth`, `admin`, `media` or similar. **A finding** (§7). | UC-INS-07; `core/domain/model/identity/Tenant.go` |
| A provider's return: one redirect per installation, `<base>/auth/callback`. The web app asks `POST /auth/oidc:locate` where the flow belongs and forwards it to the workspace's host. The lookup answers only for a live state, and an unknown one is one `404` that names no workspace. | Branch `claude/cp-14-workspace-address` (identity.md §10.1 there), [evidence](../evidence/README.md) CP-14 |
| Invitation links go to the workspace's host. Reset, address-confirmation and notification mails still use the installation's base. | UC-ID-07 check 7; finding #1549; #1337 |
| A passkey's RP ID is the workspace's **canonical host**, never the registrable parent. "A changed host ends its passkeys." | identity.md §23.3 |
| The web app keeps tokens in `sessionStorage` and uses no cookie session, so each workspace host is its own browser container. | identity.md §14.4 |
| The operators' cross-tenant doors are narrow `SECURITY DEFINER` functions. `subject_tenants(email)` exists, but only for an operator's data subject request. | multi-tenancy.md §2.1 |

Two consequences follow from this premise. **The browser origin is Hubtask's workspace boundary on
the client**: tokens, the offline cache, passkeys and the phishing anchor on the card (UC-ID-01
check 1) all hang off it. And **the server already refuses to say whether a short name exists.**

---

## 2. Question 1: with or without a subdomain, and changing it

### 2.1 Findings: how the market addresses a tenant

| Product | Addressing | Identity model | Source |
|---|---|---|---|
| Slack | Subdomain `acme.slack.com` | One email, signed in per workspace; the list appears after a code | [slack 201663443](https://slack.com/help/articles/201663443-Change-your-workspace-or-org-name-and-URL), [slack 212681477](https://slack.com/help/articles/212681477) |
| Atlassian Cloud | Subdomain `acme.atlassian.net` | One global Atlassian account | [atlassian sites](https://support.atlassian.com/statuspage/docs/understand-atlassian-sites-and-organizations/) |
| Zendesk | Subdomain | One account per tenant; records fully separate **[secondary]** | [zendesk 203660756](https://support.zendesk.com/hc/en-us/articles/203660756-Renaming-your-subdomain), [zendesk 4408831446298](https://support.zendesk.com/hc/en-us/articles/4408831446298) |
| monday.com, Freshdesk, Salesforce, Discourse (hosted) | Subdomain | Per tenant | [monday](https://support.monday.com/hc/en-us/articles/115005322145-How-to-change-your-account-URL-and-name), [freshdesk](https://support.freshdesk.com/support/solutions/articles/50000004704-customizing-your-helpdesk-url), [salesforce](https://developer.salesforce.com/docs/atlas.en-us.identityImplGuide.meta/%20identityImplGuide/domain_name_rename.htm), [discourse](https://meta.discourse.org/t/configure-your-domain-name-for-hosted-discourse/21827) |
| Microsoft 365 | Subdomains `acme.onmicrosoft.com`, `acme.sharepoint.com` | Directory per tenant; B2B guests as separate objects | [sharepoint rename](https://learn.microsoft.com/en-us/sharepoint/change-your-sharepoint-domain-name), [entra b2b](https://learn.microsoft.com/en-us/entra/external-id/what-is-b2b) |
| Google Workspace | Shared host; the tenant from the email domain | Per domain | [google primary domain](https://knowledge.workspace.google.com/admin/domains/change-your-primary-domain-for-google-workspace?hl=en) |
| Notion, Linear | Shared host, workspace in the path | One global account, several workspaces | [notion workspaces](https://www.notion.com/help/intro-to-workspaces), [linear workspaces](https://linear.app/docs/workspaces) |
| GitHub, GitLab | Path (`/acme`) | One global account | [github rename org](https://docs.github.com/en/organizations/managing-organization-settings/renaming-an-organization), [gitlab groups](https://docs.gitlab.com/user/group/manage/) |
| Asana, ClickUp | Shared host plus a numeric ID | One global account | [asana url format](https://forum.asana.com/t/upcoming-new-v1-asana-url-format-in-the-browser/1011489) **[secondary]**, [clickup](https://help.clickup.com/hc/en-us/articles/6310502590487-Create-a-new-Workspace) **[secondary]** |

**The pattern.** Products with **one global account** (GitHub, GitLab, Notion, Linear, Asana,
Plane, Mattermost teams) put the tenant in the path or behind an ID. The session is the person's,
so one origin is natural. Products whose **tenant owns the account or the session** (Zendesk,
Zulip, Slack's per-workspace sign-in, Salesforce, Microsoft 365) give each tenant its own host.
Zulip says so outright: "one subdomain per organization is required to support a user logging
into multiple organizations", because cookies and browser storage are isolated per origin
([zulip multiple organizations](https://zulip.readthedocs.io/en/latest/production/multiple-organizations.html)).
Hubtask is in the second group by decision (`NG-global-identity`).

### 2.2 Findings: self-hosted products and what they configure

| Product | Base URL | Subpath | Tenancy and addressing | Reserved names | Source |
|---|---|---|---|---|---|
| **Zulip** | `EXTERNAL_HOST` (host, optional port) | Not documented | Realms on subdomains, **or the root domain** (`string_id = ''`), or any host via `REALM_HOSTS`; `ROOT_DOMAIN_LANDING_PAGE`, `ROOT_SUBDOMAIN_ALIASES = ["www"]` | ~86 product plus ~320 generic, plurals included | [prod_settings_template.py](https://raw.githubusercontent.com/zulip/zulip/main/zproject/prod_settings_template.py), [default_settings.py](https://raw.githubusercontent.com/zulip/zulip/main/zproject/default_settings.py), [name_restrictions.py](https://raw.githubusercontent.com/zulip/zulip/main/zerver/lib/name_restrictions.py) |
| Mattermost | `SiteURL` | Yes (since 5.1) | One server, teams in the path, one account across teams | Prefix list (`admin`, `api`, `login`, `oauth`, …) | [mattermost config](https://docs.mattermost.com/administration-guide/configure/environment-configuration-settings.html), [subpath](https://developers.mattermost.com/blog/subpath), [team.go](https://raw.githubusercontent.com/mattermost/mattermost/master/server/public/model/team.go) |
| GitLab self-managed | `external_url` | Yes, a relative URL (a subdomain is recommended) | Path namespaces, global accounts | `TOP_LEVEL_ROUTES` and others | [relative url](https://docs.gitlab.com/install/relative_url/), [reserved names](https://docs.gitlab.com/user/reserved_names/) |
| Nextcloud | `overwrite.cli.url`, `trusted_domains` | Yes | Single tenant; scale through "Global Scale" nodes | n/a | [config sample](https://docs.nextcloud.com/server/latest/admin_manual/configuration_server/config_sample_php_parameters.html), [global scale](https://nextcloud.com/blog/nextcloud-announces-global-scale-architecture-as-part-of-nextcloud-12) |
| Rocket.Chat | `ROOT_URL` | Documented, reportedly fragile **[unverified]** | One workspace per deployment; the cloud isolates by container | n/a | [env vars](https://docs.rocket.chat/deployment-environment-variables), [forum](https://forums.rocket.chat/t/ideas-on-multi-tenant-rocket-chat-support/2979) **[secondary]** |
| Outline | `URL` | **[unverified]** | Self-hosted: the one team; cloud: `<team>.<base>` or a custom domain | `RESERVED_SUBDOMAINS` | [passport.ts](https://raw.githubusercontent.com/outline/outline/main/server/utils/passport.ts), [domains.ts](https://raw.githubusercontent.com/outline/outline/main/shared/utils/domains.ts) |
| Plane | `WEB_URL` | **[unverified]** | Workspaces as path slugs, global accounts; the slug "cannot be changed later" | `RESTRICTED_WORKSPACE_SLUGS` (~100) | [plane env](https://developers.plane.so/self-hosting/govern/environment-variables), [create workspace](https://docs.plane.so/introduction/tutorials/create-workspace), [constants.py](https://raw.githubusercontent.com/makeplane/plane/preview/apps/api/plane/utils/constants.py) |
| Discourse | `DISCOURSE_HOSTNAME` | Relative root discouraged for self-hosters | Optional multisite, a hostname per site | n/a | [relative root](https://meta.discourse.org/t/30507), [multisite](https://meta.discourse.org/t/multisite-configuration-with-docker/14084) |
| Mastodon | `LOCAL_DOMAIN`, `WEB_DOMAIN` | **[unverified]** | One domain; "cannot be safely changed later" | n/a | [mastodon config](https://docs.joinmastodon.org/admin/config/) |
| Keycloak | `hostname` (may carry a path) | Yes | Realms by path `/realms/<name>`; a rename changes the issuer | **[unverified]** | [keycloak hostname](https://www.keycloak.org/server/hostname) |

**Zulip is the closest analogue**: realms are tenants, accounts are per realm (`UserProfile`
unique on `(realm, upper(email))`, [users.py](https://raw.githubusercontent.com/zulip/zulip/main/zerver/models/users.py)),
and it covers both shapes, one realm on the root domain and many on subdomains. Its lessons:

1. **The single-organisation server uses the root domain** (the empty subdomain). Hubtask's single
   mode is the same. **Mixing** a root-domain realm with subdomain realms is discouraged where
   users overlap, because the root domain's cookies are visible to the subdomains.
2. **A custom host is a mapping from the realm to any hostname** (`REALM_HOSTS`), with the original
   subdomain kept blocked. Zulip expects to move this into the database, which Hubtask's
   `tenant_host` already is.
3. **One auth subdomain** (`SOCIAL_AUTH_SUBDOMAIN = 'auth'`) receives every provider's callback.
   Hubtask's `<base>/auth/callback` plus `oidc:locate` is the same idea, and it is the reason a
   rename does not touch any provider's registration.
4. **A reserved-name list** of about 400 entries, matched with plurals.
5. **Renaming leaves a placeholder behind** (§2.3), and **"Find my account" mails the list** (§4).

### 2.3 Findings: changing a workspace's address

| Product | Who | Limit | Old address redirects | Old name | What breaks | Source |
|---|---|---|---|---|---|---|
| Slack | Workspace owner | none stated | Yes, "as long as no new groups claim the URL" | **Released** | SSO settings, some apps | [slack 201663443](https://slack.com/help/articles/201663443-Change-your-workspace-or-org-name-and-URL) |
| Atlassian | Org admin, paid plans only | 3 (or 15: its pages disagree) | Browsers yes, no end date; **REST API 401** | **Reserved forever** | Marketplace apps, API clients | [update a product url](https://support.atlassian.com/organization-administration/docs/update-a-product-url/), [can I change](https://support.atlassian.com/organization-administration/docs/can-i-change-the-url-used-to-access-a-product/), [api 4xx](https://confluence.atlassian.com/jirakb/rest-api-calls-failing-with-error-4xx-response-code-after-cloud-site-is-renamed-1387868568.html) |
| Zendesk | Account owner only | none | **None**; old mail links die | not documented | SSO until updated, API, integrations, widgets, mobile SDK; recommends host mapping instead | [zendesk 203660756](https://support.zendesk.com/hc/en-us/articles/203660756-Renaming-your-subdomain) |
| Freshdesk | Admin | **once** | None | not documented | Sessions, links, webhooks, widgets | [freshdesk](https://support.freshdesk.com/support/solutions/articles/50000004704-customizing-your-helpdesk-url) |
| monday.com | Account admin | none stated | **30 days**, the name held unless "Clear old URL" | Held 30 days | SSO/SCIM endpoints; everybody signs in again | [monday](https://support.monday.com/hc/en-us/articles/115005322145-How-to-change-your-account-URL-and-name) **[secondary snippet]** |
| Microsoft SharePoint | SharePoint admin, scheduled | **twice per tenant lifetime**, 30-day cooldown, no way back | **308 for one year** | not stated | Absolute URLs, flows, Teams tabs, app redirect URLs | [sharepoint rename](https://learn.microsoft.com/en-us/sharepoint/change-your-sharepoint-domain-name) |
| Salesforce My Domain | Admin | none | Only the **last** previous name; "not a permanent solution" | **Held until removed** | SSO, API clients that do not follow redirects | [remove previous](https://developer.salesforce.com/docs/atlas.en-us.identityImplGuide.meta/%20identityImplGuide/domain_name_remove_previous.htm) |
| GitHub org | Owner | none | Repositories yes; **org page and API 404** | **Released at once**; popular `owner/repo` pairs retired | Packages, actions, SAML at the IdP | [github rename org](https://docs.github.com/en/organizations/managing-organization-settings/renaming-an-organization) |
| GitLab group | Owner | not with registry tags | Web, Git, API GET only, until claimed | **Released** (squatting possible) | CI includes, image references | [gitlab repository](https://docs.gitlab.com/user/project/repository/), [gitlab groups](https://docs.gitlab.com/user/group/manage/) |
| Linear | Admin | **[unverified]** | Yes (since 2022) | **[unverified]** | | [linear changelog](https://linear.app/changelog/page/12) |
| Notion (site domain) | Owner/admin | | **None** | Notion may reclaim it | Shared links | [notion sites](https://www.notion.com/help/manage-your-notion-sites) |
| **Zulip** | Server admin (`manage.py change_realm_subdomain`); Cloud through support | | Yes: a **deactivated placeholder realm** under the old name shows "the organization has moved", then redirects; deep links keep working; chains flattened | Self-hosted: **held while the placeholder exists**; Cloud: freed **after 3 months** by default, longer on request | Everybody signed out of web, mobile and desktop; API clients re-pointed | [change_realm_subdomain.py](https://raw.githubusercontent.com/zulip/zulip/main/zerver/management/commands/change_realm_subdomain.py), [create_realm.py](https://raw.githubusercontent.com/zulip/zulip/main/zerver/actions/create_realm.py), [change organization url](https://zulip.com/help/change-organization-url) |
| Plane | — | **never** | — | — | — | [create workspace](https://docs.plane.so/introduction/tutorials/create-workspace) |

**Cross-cutting patterns.**

1. **Renaming is the top role's act**, and on several products only the vendor's (Zulip Cloud,
   Discourse Enterprise) or only on paid plans (Atlassian).
2. **Caps exist where the cost is high**: once (Freshdesk), twice with a cooldown (SharePoint), 3
   or 15 (Atlassian).
3. **Redirects cover the browser more often than the API.** Atlassian answers 401, GitHub 404, and
   GitLab redirects only GET.
4. **A released old name is a phishing and squatting risk**: an old mail link then leads into a
   stranger's tenant. Slack, GitHub and GitLab accept that risk; Atlassian, Salesforce and Zulip
   (self-hosted) do not.
5. **SSO breaks almost everywhere**, because the SAML or OIDC endpoints contain the tenant's host.
6. **Vendors steer people to a custom domain or an alias instead** (Zendesk host mapping, Google
   alias domains, Atlassian custom domains with unlimited changes).

### 2.4 Findings: custom domains, briefly (for milestone CD)

Most vendors verify with a CNAME to a vendor target, sometimes plus a TXT record. The vendor issues
the certificate, usually through ACME. A plan gates the feature. The default address keeps working
and redirects. Examples: Atlassian (CNAME, Atlassian-issued certificate, no bring-your-own, any
number of changes;
[atlassian custom domain](https://support.atlassian.com/organization-administration/docs/add-a-custom-domain/)),
Zendesk host mapping (CNAME, free Let's Encrypt certificate or your own;
[zendesk host mapping](https://support.zendesk.com/hc/en-us/articles/4408838571930-Host-mapping-Changing-the-URL-of-your-help-center)),
Notion Sites (CNAME plus TXT, paid add-on;
[notion custom domain](https://www.notion.com/help/connect-a-custom-domain-with-notion-sites)),
Salesforce (your own certificate or its CDN with an `_acme-challenge` CNAME), and Zulip Cloud
(`REALM_HOSTS`, through support, on its larger plan). Hubtask's `tenant_host` with a TXT mark and
the four states matches this. UC-INS-15 already says certificates are the operator's ingress.

### 2.5 What changing the address costs in Hubtask

| Thing | After a rename | Mitigation |
|---|---|---|
| Open web sessions | The tab on the old host keeps its token, but the redirect lands on a new origin with empty `sessionStorage`, so people **sign in again** | Say so in the dialog (P-04). Nothing to build. |
| **Passkeys** | **All end**: the RP ID is the canonical host (identity.md §23.3) | WebAuthn Level 3 **Related Origin Requests**: the old host serves `/.well-known/webauthn` naming the new origin, so the old RP ID keeps working. Shipped in Chrome 128/129 ([chrome blog](https://developer.chrome.com/blog/passkeys-updates-chrome-129)); Firefox has an implementation ([searchfox](https://searchfox.org/firefox-main/source/dom/webauthn/WebAuthnRelatedOriginFetcher.sys.mjs)) with reported failures **[secondary]**; Safari **[unverified]**. Without it, the dialog says passkeys end and asks people to register new ones. |
| Sign-in providers | **Unaffected**: the redirect URI is the installation's `<base>/auth/callback`, and `oidc:locate` builds the target from the workspace's current canonical host | None needed, unlike Slack, Zendesk and monday. Hubtask's one clear advantage. |
| Links already mailed (invitation 14 days, reset 30 minutes, notifications) | Point at the old host | The old host stays a redirect for at least the longest lifetime of a mailed link. First, #1549 must make every mail use the canonical host. |
| Calendar feeds (`hbt_cal_` ICS), CalDAV, MCP clients, `hubctl` profiles, the shells' saved server | Point at the old host; a client may not follow a redirect for writes | The old host keeps **answering the API** for the redirect period (Atlassian's 401 is the counter-example). Name these clients in the dialog. |
| The phishing anchor on the card (UC-ID-01 check 1) | Shows the new host; people may distrust it | Mail every member: what changed, the new address and that the old one redirects (P-11), like ADR-0078's notices. |
| Audit trail, `instance_event` | Keep the old slug as written | Correct as history; the rename is itself an entry in both. |
| A backup restored onto another installation | `tenant_host` is not in the archive ("hosts belong to the installation") | Unchanged. |

**Design consequence.** A rename in Hubtask is: write a new canonical `tenant_host` row, and keep
the old one as a non-canonical row that **redirects the browser, answers the API, and holds the
name**. That is the same machinery UC-INS-15 check 3 asks for ("the provider's address redirects
to the own domain; the API answers on both"). **Rename and custom domain are one feature at two
sizes**, and they belong in one cut.

---

## 3. Question 2: one address in several workspaces, and switching

### 3.1 It happens, in every deployment that has more than one workspace

| Who | Deployment | Example |
|---|---|---|
| A freelancer in several client companies | D6 | One address, three customer workspaces at one provider |
| A person with a family and a club workspace at the same provider | D5 | `familie-meier` and `tsv-nord`, one address |
| A company with a workspace per legal entity or per department | D4 (multi) | `sales`, `hr` |
| An operator who is a member of a customer workspace | D5, D6 | Their own workspace and a support guest account |
| A person on two installations | D7, or any two self-hosters | Two servers, no relation at all |

Single mode (D1–D3) has one workspace, so the case does not arise inside one installation.

### 3.2 Findings: how products switch

| Product | Switching | Identity behind it | Source |
|---|---|---|---|
| Slack | Sidebar of signed-in workspaces in the desktop app; "Add a workspace" | Per-workspace sign-in | [slack 212681477](https://slack.com/help/articles/212681477) |
| **Zulip** | Web: one tab per organisation. Desktop: an organisations sidebar, "Add organization" asks for the URL. Mobile: "Switch account". No cross-realm session. | Per realm | [switching organizations](https://zulip.com/help/switching-between-organizations) |
| Mattermost | The desktop app's server list. The browser cannot hold several servers. Admins may preload or lock the list. | Per server | [connect multiple workspaces](https://docs.mattermost.com/end-user-guide/preferences/connect-multiple-workspaces.html), [custom desktop app](https://docs.mattermost.com/deployment-guide/desktop/distribute-a-custom-desktop-app.html) |
| Microsoft | "Pick an account" lists the accounts used on this device; B2B guests switch organisations in My Account | Home identity plus a guest object per tenant | [switch organizations](https://support.microsoft.com/en-us/accounts-billing/work-school/switch-organizations-in-your-work-or-school-account-portal) |
| Google | Account chooser; several accounts signed in at once | Separate accounts | [google multiple accounts](https://support.google.com/accounts/answer/1721977) |
| Atlassian, Notion, Linear, Auth0 | A site or workspace switcher, or an organisation picker after sign-in | One global account | [jira switch site](https://support.atlassian.com/jira-cloud-ios/docs/switch-between-your-jira-sites/), [notion workspaces](https://www.notion.com/help/intro-to-workspaces), [auth0 orgs](https://auth0.com/docs/manage-users/organizations/login-flows-for-organizations) |

**The pattern for products with per-tenant accounts** (Slack, Zulip, Mattermost, Microsoft's
picker) is a list **on the device** of the tenants this device signed into, each with its own
session. The server keeps no link between the accounts. That is exactly what `NG-global-identity`
allows.

### 3.3 Recommendation for switching

1. **The installed shells first** (F7): a sidebar or menu of workspaces, each with its own
   credential in the keystore, and "Add a workspace" (by address, or from the finder mail, §4).
   This is the Slack, Zulip and Mattermost desktop model, and the shells already hold credentials
   per server.
2. **The web app**: each workspace is its own origin, so a list shared between workspaces needs a
   common origin. Proposal: the **installation's base host** keeps the device's list in its own
   `localStorage`, holding entries of host plus display name only, never an address or a token.
   A workspace adds itself by sending the browser to `<base>/workspaces:remember?host=…` once
   after a sign-in. That target accepts only a host beneath the base or a verified `tenant_host`,
   and the same rule applies when it forwards (the `oidc:locate` forward's rule). The account
   menu's "Switch workspace" opens the base host's list. Opening one is a plain navigation to that
   host, with its own sign-in.
3. **What the list may hold** is the owner's question (D-4): UC-ID-01 says the address is not
   remembered on the device. Hosts and workspace names are not the address, but on a shared
   computer they still reveal where somebody works. Offer "Forget this workspace on this device",
   and never fill the list on a device the person did not choose (the shells' "public device"
   setting, if one comes).
4. **Not built**: a server-side "linked accounts" table, a session that opens a second workspace,
   or unread counts aggregated by the server across workspaces. Each would be a global identity.

---

## 4. Question 3: must a person know their subdomain?

### 4.1 Findings: getting from a general domain to the tenant

| Product | Entry | Routing | Out of band? | Source |
|---|---|---|---|---|
| Slack (current) | Email, then a mailed code | After the code, the workspaces of that email are shown | Partly: a code, then the list on screen | [slack 212681477](https://slack.com/help/articles/212681477) |
| Slack ("find your workspace") | Email | The list of workspaces is **mailed** | Yes | [vcfa kb](https://it.vcfa.edu/kb/slack/signing-into-slack-from-your-browser) **[secondary]** |
| **Zulip** `/accounts/find/` | Up to 10 addresses | One mail per address with every active account (organisation name, host, URL); addresses without accounts get an "account not found" mail. **The screen re-renders with every submitted address whatever matched**, per the code comment, so it cannot be used "to ascertain which email addresses are associated". Rate-limited per IP for each address. | Yes | [registration.py](https://raw.githubusercontent.com/zulip/zulip/main/zerver/views/registration.py), [forms.py](https://raw.githubusercontent.com/zulip/zulip/main/zerver/forms.py), [logging in](https://zulip.com/help/logging-in) |
| Microsoft Entra | Address first | Home Realm Discovery by the **domain**; policies and domain hints | No: on screen, by domain | [hrd policy](https://learn.microsoft.com/en-us/entra/identity/enterprise-apps/home-realm-discovery-policy) |
| Okta | Identifier first | Routing rules by domain, attribute, network or app. Okta: the rules "don't provide security enhancements" | No | [okta idp discovery](https://help.okta.com/en-us/Content/Topics/Security/Identity_Provider_Discovery.htm) |
| Auth0 Organizations | Identifier first, or "type your organisation" | A **verified domain** match picks the organisation; several matches show a picker; HRD to its connection | No: on screen, by domain | [auth0 org login flows](https://auth0.com/docs/manage-users/organizations/login-flows-for-organizations) |
| Atlassian | Email first at id.atlassian.com | A **verified domain** sends the person to the organisation's IdP | No | [atlassian verify domain](https://support.atlassian.com/user-management/docs/verify-a-domain-to-manage-accounts) |
| Mattermost, Zendesk, Zulip self-hosted | The tenant's URL | None; you know it, or you have an invitation | — | [mattermost](https://docs.mattermost.com/end-user-guide/preferences/connect-multiple-workspaces.html) |

**Three safe shapes** recur, and one unsafe one:

1. **Route by email domain, on screen** (Entra, Okta, Auth0, Atlassian). This reveals that a
   *domain* uses this service, which is usually public anyway. It reveals nothing about a person,
   and it works only with a **verified** domain.
2. **Show the list only after a mailbox proof** (Slack's code). Hubtask would need a code step
   that proves only the mailbox. NIST rules email out as an authenticator
   ([SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html) §3.1.3.1), so the list could
   never be more than a list.
3. **Mail the list** (Zulip, Slack's finder). The screen gives the same answer for every address.
4. **Unsafe: route a typed address straight to its workspace.** This is an enumeration oracle.
   Research enumerated Slack accounts by address through an API meant for contacts, beating
   per-account limits by rotating accounts
   ([SlackEnum, Black Hills, 2024](https://www.blackhillsinfosec.com/introducing-slackenum)).
   Microsoft's realm and credential-type endpoints were used to enumerate tenants and users
   ([AADInternals](https://aadinternals.com/post/just-looking/)). Okta ships a "User Enumeration
   Prevention" setting because identifier-first leaks
   ([okta enumeration prevention](https://support.okta.com/help/s/article/about-user-enumeration-prevention)).

### 4.2 Findings: one identity across tenants vs. one account per tenant

| | Global identity (Atlassian, Notion, GitHub, Linear) | Account per tenant (Zendesk, Zulip, Mattermost, GitHub EMU, Hubtask) |
|---|---|---|
| Usability | One sign-in, one switcher | Several sign-ins; needs a finder and a device list |
| Isolation | One credential reaches every tenant; the profile is shared | A breach or a lock stays in one tenant |
| Enumeration | The service knows every tenant of an address: any flow that shows it is an oracle | A finder needs a cross-tenant index, used only for mail |
| SSO and SCIM | Domain claims: a verified domain lets one tenant **manage** the person's global account, which affects their other memberships ([atlassian managed accounts](https://support.atlassian.com/user-management/docs/what-are-managed-accounts/)) | The tenant's provider owns its own accounts outright, with no claim conflict. GitHub's Enterprise Managed Users exist to get exactly this ([github EMU](https://docs.github.com/en/enterprise-cloud@latest/admin/identity-and-access-management/understanding-iam-for-enterprises/abilities-and-restrictions-of-managed-user-accounts)) |
| GDPR | The vendor is close to a controller of the identity | Each workspace is the controller of its accounts; the provider processes for each (Art. 28). A cross-tenant lookup is processing (Art. 4(2), "retrieval", "consultation") and needs its own purpose and minimisation (Art. 5(1)(b), (c); [art. 5](https://gdpr-info.eu/art-5-gdpr/)). **[legal interpretation, not a regulator's]** |

**For Hubtask** the per-workspace account is the decided model (`NG-global-identity`, P-01, ADR-0010),
and it is the more defensible one for D4–D6. Its usability cost is answered by §3.3 and §4.3, not
by a global account.

**The security references** agree with identity.md §4.3 and §7. Respond generically, with the same
status, body and timing; do the same work for every address; rate-limit per address, not only per
account
([OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html),
[OWASP Forgot Password](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)).

### 4.3 Recommendation: the general domain (multi mode only)

The installation's base host draws one signed-out card with three ways on. None of them answers
anything about an address on screen.

1. **Your workspaces on this device**: the list of §3.3, one click each.
2. **Go to your workspace**: type the short name, and the browser goes to `<name>.<base>`.
   **No lookup is made.** A name that does not exist lands on a host whose card answers like an
   undecided workspace, as identity.md §3 already holds. This is Slack's "enter your workspace URL"
   without Slack's "workspace not found".
3. **Find your workspaces**: type your address, and the answer is always the same: "If this address
   has an account in a workspace here, we have sent it a list." The mechanics are those of the
   forgotten password (identity.md §7):
   - one read-only lookup and one job for every address, naming the accounts or nobody;
   - one mail listing each workspace's **display name and canonical host as a link to its sign-in
     card**. It carries no token, no session and no role (`NG-magic-link`);
   - only `ACTIVE` accounts in `ACTIVE` workspaces that allow it (§5.2), and never a managed
     account without an address;
   - an address with no account gets either nothing or a neutral mail, the same for every address
     (the owner's choice, D-3). Zulip sends "not found", which tells the mailbox holder only what
     they could find out anyway;
   - the attempt ledger and the auth bucket per address hash and IP class, and a mail at most once
     per address per interval;
   - the address appears in no log, metric or audit entry (rule 10); a counter without labels, like
     `hubtask_auth_password_reset_requested_total`;
   - a new narrow door: a `SECURITY DEFINER` function answering `(canonical host, display name)`
     for one address, granted to the job's role path only and never answered to a request. It is
     entered in multi-tenancy.md §2.1's doors table beside `subject_tenants(email)`.
4. **Later, and only for D6: route by verified email domain.** A workspace that proved a domain
   (the `tenant_host` TXT mark, reused for an `email_domain` row) and offers a provider under
   `DOMAINS` may let addresses at that domain be sent straight to its card or its provider. This
   reveals only that the domain has a workspace here, and only because that workspace switched it
   on. Public mail domains are refused, as the `GOOGLE` and `MICROSOFT` presets already refuse
   personal directories (identity.md §10.3).

The card at a workspace's own host does not change: the host decides the workspace (UC-ID-01). In
single mode none of this appears (P-05, P-10): the one host is the workspace.

---

## 5. Recommendations for Hubtask

### 5.1 What to build, in this order

| # | What | Deployments | Depends on | Notes |
|---|---|---|---|---|
| 1 | **Reserved short names** (`www`, `api`, `auth`, `admin`, `app`, `media`, `static`, `status`, `mail`, `docs`, `help`, …, plurals, plus the operator's own list) | D4–D7 | nothing | Fixes the finding of §7; Zulip, Outline, Plane and GitLab all have such a list |
| 2 | **Every mail and link from the canonical host** | D4–D7 | nothing | #1549, already filed; a rename is unsafe without it |
| 3 | **The base host's card in multi mode** with "Go to your workspace" (no lookup) | D4–D6 | 1 | Small; answers the owner's Q3 for anyone who knows the name |
| 4 | **The device's workspace list and "Switch workspace"** (shells first, then the web via the base host) | D4–D7 | 3 | Within `NG-global-identity`; D-4 decides what it may hold |
| 5 | **Find your workspaces by mail** | D4–D6 | 2, 3 | A new use case; D-3 |
| 6 | **Renaming a workspace's short name** by an operator, through every door (`/admin/tenants/{id}:rename`, `hubctl admin tenant rename`, the instance area) | D4–D7 | 1, 2, and `tenant_host` resolution | The old host redirects and answers the API; the old name is held; members are mailed. D-2 |
| 7 | **Renaming by the workspace's owner**, where the installation allows it | D5, D6 | 6 | An instance setting with a lock (P-07); a cooldown |
| 8 | **Custom domains** (milestone CD, UC-INS-15) | D6 | 6 | The same redirect and alias machinery |
| 9 | **Operator-assigned hosts** instead of subdomains | D4, D7 | 8 | "Without a subdomain" in multi mode: `tasks-hr.corp.example` as the canonical host, set by the operator with no DNS proof. For a company without a wildcard certificate |
| 10 | **Routing by verified email domain** | D6 | 8 | Optional, opt-in per workspace; D-5 |

Steps 6 and 8 should be one cut, because both turn `tenant_host` from a prepared table into the
resolver. Steps 1 and 2 are bounded fixes that could go into a current sweep.

### 5.2 Installation level vs. workspace level

| Setting | Level | Default | Why |
|---|---|---|---|
| `HUBTASK_TENANCY_MODE`, `HUBTASK_BASE_URL` | Environment | `single`, — | Exist |
| How workspaces are addressed in multi mode: `subdomain` or `assigned` | Environment or instance file | `subdomain` | The ingress decides it (wildcard or not); a running installation does not change it casually |
| Reserved short names (beyond the built-in list) | Instance setting (list) | empty | The operator's own subdomains (`status`, `shop`) |
| Who may rename: `operators` or `operators and owners` | Instance setting with lock | `operators` | P-07; a D5 provider may want self-service, a D6 provider may not |
| Rename cooldown | Instance setting (count of days) | 30 | Microsoft's 30 days; bounds churn and squatting |
| How long an old address redirects and answers | Instance setting (days, product minimum ≥ the longest mailed link) | 365 | SharePoint's year; Zulip Cloud's 3 months is the lower end |
| How long an old name is held after its redirect ends | Instance setting | forever (released only by an operator's act) | Atlassian, Salesforce and Zulip self-hosted; P-01: an old link must never lead into another workspace |
| Find your workspaces on or off | Instance setting with lock | owner's decision (D-3) | Off is the most private; on is the most usable |
| Being listed in Find your workspaces | Workspace (may tighten: opt out) | follows the installation | A D6 customer may not want to be findable; minimisation |
| Rename my workspace (where allowed) | Workspace, `OWNER`, step-up and typed name | — | P-04: the dialog names the cost |
| Email-domain routing | Workspace, per verified domain | off | The workspace publishes its domain; nobody else may |

In single mode none of these appear (P-10). The one host is the installation's, and renaming it is
a change of `HUBTASK_BASE_URL`, an operator's act with the same passkey cost.

### 5.3 What Hubtask should explicitly not do

| No | Why |
|---|---|
| **No on-screen answer that names a workspace for an address**: no "we found you in Acme", no auto-redirect by address, no "this address has no account here" | P-01 ("the fact that an address exists"); identity.md §3, §4.3, §7; the enumeration incidents of §4.1 |
| **No global account, no linked accounts, no session that opens a second workspace** | `NG-global-identity`; P-02: each account's own strongest proof; one breach stays in one workspace |
| **No path-prefixed workspaces** (`/w/acme`) | One origin for all workspaces: shared `localStorage`/IndexedDB for the offline cache, one passkey RP ID for all (identity.md §23.3 forbids the parent), no phishing anchor per host, CSP and media origin shared. multi-tenancy.md §3 rules it out; reversing that is a decision (D-1). Operator-assigned hosts (§5.1 #9) serve the same need without these costs |
| **No mixing of a root-domain workspace with subdomain workspaces** in one installation | Zulip's warning: the parent's storage and cookies are visible to subdomains; and the base host is needed for the callback, `oidc:locate` and the finder |
| **No release of an old name to another workspace while anything may still point at it** | Squatting and phishing: Slack, GitHub and GitLab accept this, Hubtask should not |
| **No sign-in by the finder's mail** | `NG-magic-link`; the mail links to the sign-in card only |
| **No "workspace not found" on the base host** | The server already hides whether a short name exists (identity.md §3) |
| **No email-domain routing without a DNS-proved domain, and never for public mail domains** | Otherwise one workspace claims `gmail.com` or a competitor's domain |
| **No rename in single mode through the product** | The host is the installation's, set by `HUBTASK_BASE_URL` |
| **No rename without the redirect** (Zendesk's and Freshdesk's way) | P-03: links already mailed and saved clients must not die silently |
| **No server-side cross-workspace aggregation** (unread counts, a combined inbox) | It is a global identity by another name; the shells can show per-workspace counts from their own sessions |

### 5.4 Security and privacy constraints

1. **Same answer, same work, same time** for every address at the finder (identity.md §7's rule,
   T-02 in [security.md](../architecture/security.md) §4).
2. **The cross-tenant lookup is a door** (multi-tenancy.md §2.1): one narrow `SECURITY DEFINER`
   function, its answer used only to queue a mail, never returned to a caller. Entered in the doors
   table and the restore drill's list if it adds a table.
3. **The rename is the operator's act through `admin:tenants`** (an elevated session, a step-up and
   the typed name, as deletion has), journalled in `instance_event` and in the workspace's trail. A
   workspace's own rename is an `OWNER` act with a step-up.
4. **The redirect host must stay ours**: the old host is resolved from `tenant_host` and redirects
   to the canonical host the server knows, never to a value in the request (the `oidc:locate`
   rule). An open redirect here would be a phishing tool.
5. **The device list holds no address and no credential**; tokens stay per origin
   (identity.md §14.4).
6. **Data protection**: the finder is processing across controllers (each workspace's) for one
   purpose, helping the data subject reach their own accounts. It needs an entry in the data
   catalogue, a workspace opt-out (minimisation), and no retained lookup beyond the rate-limit key
   (a keyed hash).
7. **Passkeys**: either Related Origin Requests from the old host for the redirect period, or a
   dialog that names the cost (UC-INS-15 check 5 already demands the dialog).

### 5.5 How each recommendation meets what exists

| Recommendation | Accounts per workspace | RLS per tenant | `oidc:locate` | Links to the workspace host |
|---|---|---|---|---|
| Reserved names | — | — | — | Protects the base host's own labels (`auth`, the callback) |
| Base host card, "Go to" | Unchanged | No lookup | The base host already serves the callback; the card is a second use of it | — |
| Device list | Unchanged; each entry signs in on its own | No server state | Same forward rule (only hosts beneath the base) | Entries are canonical hosts |
| Find your workspaces | Reads accounts of many workspaces for one address | A new narrow door, like `subject_tenants` | — | The mail names canonical hosts; needs #1549's helper |
| Rename | The accounts do not move; their tokens carry the tenant id, not the host | `tenant_host` is tenant-scoped; resolution needs a `SECURITY DEFINER` lookup by host like `resolve_tenant` | **No provider re-registration**: the callback is the installation's, and `locate` builds the target from the canonical host | All new links use the new host; the old host redirects old ones |
| Custom domains, assigned hosts | — | Same | Same; `locate`'s "beneath its own host" rule must widen to verified `tenant_host` rows | Same |
| Domain routing | Admission stays §10.4's | A lookup by domain, one workspace or none | Starts a flow on the workspace's host | — |

---

## 6. Candidate decision questions for the owner

Each falls under AGENTS.md "What you do not decide yourself", as noted. Each would be a `decision`
issue in its form, asked once, together.

| # | Question | Why it is the owner's | Proposal |
|---|---|---|---|
| D-1 | Should multi mode offer workspaces **without their own host** (a path prefix, or one shared host with the workspace chosen after sign-in)? Or is "without a subdomain" met by single mode plus **operator-assigned hosts**? | A deviation from multi-tenancy.md §3 ("a path prefix is deliberately not used") and identity.md §23.3 | Assigned hosts, no path mode |
| D-2 | Renaming a workspace's short name: who may (operators only, or owners where the installation allows), how long the old address redirects and answers (proposal 365 days), and is the old name ever released to another workspace (proposal: only by an operator's explicit act)? | A new use case's *Goal* and *How to check*; P-03, P-04 | As §5.2 |
| D-3 | **Find your workspaces** at the base host: build it? Default on or off in multi mode? May a workspace opt out? Does an address without accounts get a mail? | A new use case; a cross-workspace lookup measured against P-01, in `docs/vision/` | Build it; on, with an opt-out per workspace; no mail for an unknown address, so the service never mails a stranger who was typed in |
| D-4 | The **device's workspace list**: may the device remember the workspaces (host and name, never the address) a person signed into, against UC-ID-01's "the address is not remembered on the device"? In the web through the base host, or only in the shells? | UC-ID-01 *Where it ends* | Yes, host and name only, with "Forget this workspace"; web and shells |
| D-5 | **Routing by verified email domain** for D6: a later use case with CD, opt-in per workspace? | A new use case; D6 in `docs/vision/deployments.md` | Yes, later, after CD |
| D-6 | A rename's **cost to passkeys**: accept that they end (as UC-INS-15 check 5 says for a domain), or keep them through Related Origin Requests, which serves a well-known file from the old host as long as it is held? | What a person observes in UC-INS-15 *How to check* | Related Origin Requests where the browser supports them, the dialog naming the cost either way |
| D-7 | Do rename and custom domains go into **one cut** (CD), or does rename come earlier as its own milestone? | The milestone's `Delivers` | One cut |

---

## 7. A finding outside this research

**UC-INS-07 check 2 promises that a reserved short name is refused, but no list exists.**
`core/domain/model/identity/Tenant.go` checks only the grammar (`validTenantSlug`), and the database
checks the same pattern and uniqueness. A workspace may be called `www`, `api`, `auth`, `admin` or
`media`. Where the installation or the platform uses such a label for itself (the integration
environment's `<service>.<environment>.hubtask.eu`,
[deployment.md](../architecture/deployment.md)), a workspace could take it. This should be a
`finding` issue, fixed with a built-in list plus an operator's list (§5.1 #1).
