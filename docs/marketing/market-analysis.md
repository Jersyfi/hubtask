# Market Analysis and Positioning

Status: **draft for the owner's decision** · Written 2026-09-09 · Horizon: the `1.0.0` launch

> This is the brief [`roadmap.md`](../roadmap.md) § "The website" has been waiting for. That section
> names five things as "open, and awaited from the owner" — positioning and messaging, the page
> structure, how much of the roadmap is shown, what may be promised about editions and price, and
> the launch moment. This document answers the first four and leaves the fifth alone.
>
> It is a **marketing** document. It contains no architectural decision, and it may not create one:
> where it describes the product it is quoting the architecture documents, and where those and this
> page disagree, they are right. Every claim it proposes for the website carries the document or the
> CI job that proves it, in [`website-1.0.md`](./website-1.0.md) § 5.

---

## 1. The field, as it stands in September 2026

Task management is not one market. It is four, and Hubtask is the only entry that reaches into all
four with one code path — which is an opportunity and, unaddressed, a positioning problem.

| Segment | Who leads it | What buyers there actually want |
|---|---|---|
| **Personal task managers** | Todoist, TickTick, Things, Apple Reminders, Microsoft To Do | Speed, capture, a good phone app, a sense that the tool is theirs |
| **Team work management** | Asana, ClickUp, Monday.com, Notion, Trello, Linear | Shared visibility, low onboarding cost, integrations |
| **Self-hosted / source available** | Vikunja, OpenProject, Plane, Taiga, Redmine, WeKan, Nextcloud Deck, Super Productivity | Control of the data, no per-seat bill, a stack they can operate |
| **Agent-facing surfaces** | The incumbents' own MCP servers (Todoist, Asana, ClickUp, Notion), plus community bridges | An agent that can actually *do* the work, with permissions somebody can reason about |

### 1.1 Four movements that changed the ground under this category

**MCP became table stakes, and that is a finding against us.** Through 2025 and into 2026 the
incumbents shipped first-party MCP servers — Todoist at `ai.todoist.net/mcp`, Asana at
`mcp.asana.com` (its v1 server deprecated in May 2026), with ClickUp and Notion alongside. "We speak
MCP" was a differentiator eighteen months ago. **It is not one now, and the website must not lead
with it as though it were.**

What survives the comparison is *where* the agent runs. Every incumbent server is a hosted OAuth
endpoint: the agent reaches the vendor's cloud, and the content of the tasks goes with it. Hubtask's
MCP server is a presentation adapter inside the installation
([`ai-first.md`](../architecture/ai-first.md) §1.1) — an agent on your own machine talking to your
own box, holding a scope-limited credential, audited as `actor.type = AI_AGENT`, and refused
everything the same credential would be refused over REST. That is the sentence, and it is about
sovereignty rather than about protocol support.

**Sovereignty stopped being an ideological argument and became a procurement one.** European buyers
— public bodies, agencies with public-sector clients, health and finance adjacent teams — now ask
where the data sits and who can be compelled to hand it over. Meanwhile the regulatory floor rose:
GDPR enforcement matured, NIS2 pulled mid-sized organisations into a security-obligation regime, and
the European Accessibility Act has applied since June 2025 to a wide class of digital services. None
of that sells a task manager on its own. All of it disqualifies one that cannot answer.

**The self-hosted field is functionally rich and operationally thin.** This is the most important
finding in this document. Read the self-hosted competitors as an operator rather than as a user and
a pattern appears: they can all draw a kanban board, and almost none of them can tell you that a
backup they wrote last week can be restored, that their audit trail has not been edited, that a
deletion rule will not take something it should not, or that one customer's data cannot be read from
another customer's session. Those are not features their users asked for. They are the features
their users discover the absence of on a bad day.

**Feature sprawl produced a counter-movement.** Linear's rise showed that an opinionated, fast,
visibly well-made tool beats a configurable one. The lesson for a project with 109 use cases in its
catalogue is not to hide them — it is to never present them as a list of 109 things.

### 1.2 The competitors, read honestly

From public documentation, September 2026. Each of these is a good piece of software; the column
that matters is the one where Hubtask is *structurally* different, not merely newer.

| | Reach | Where Hubtask differs structurally |
|---|---|---|
| **Vikunja** | The closest neighbour: self-hosted, Go, task-first, genuinely pleasant | No tenant isolation to enforce, no tamper-evident trail, no backup/restore as product use cases, no native agent surface. AGPL rather than a convertible licence |
| **Plane** | Modern, self-hostable, engineering-team oriented | An operating surface of several services; Hubtask asks for PostgreSQL and nothing else. Plane's model is issues and cycles, not a life and a client roster |
| **OpenProject / Redmine / Taiga** | Mature, deep, project-management shaped | Built for projects with plans and budgets. They do not fit "buy milk", and they were never trying to |
| **WeKan / Focalboard / Kanboard** | A board, quickly | A board is one of four layouts here, and the model underneath it is the product |
| **Nextcloud Deck / Tasks** | Already installed wherever Nextcloud is | A component of a suite, with the API depth that implies |
| **Todoist / TickTick / Things** | The best personal UX in the category, by some distance | Closed, hosted, per-seat. This is where Hubtask's *client* work has the most to prove, and the website must not pretend otherwise |
| **Asana / ClickUp / Monday** | Distribution, integrations, brand | SaaS-only, per-seat, and the data lives with them |

**The honest weakness.** Against Todoist and Things, Hubtask's phone experience is the thing a
private user will judge it on, and it is the newest part of the product. Against Asana and Monday,
Hubtask has no integrations marketplace and no brand. The website's answer to both is not to claim
otherwise: it is to be aimed at the person for whom control, longevity and automation outrank
polish-on-day-one, and to be visibly excellent at *being that*.

---

## 2. The gap, in one paragraph

Nobody is currently selling **operational seriousness in a personal-scale tool.** The guarantees
Hubtask ships — a restore drill that runs in CI on every change, a hash-chained audit trail with a
`:verify` that names the first broken link, deletion rules that announce and can be stopped, tenant
isolation enforced by the database rather than by a `WHERE` clause, an SLO catalogue, a health
endpoint that says what is missing instead of crashing — are things buyers meet in infrastructure
software and never in a to-do list. Presenting them in a to-do list is a genuinely new proposition,
and it is defensible because it is expensive to copy: it is not a feature, it is how the thing was
built.

---

## 3. Positioning

> **For people and small providers who want their task system to be genuinely theirs, Hubtask is a
> self-hostable task manager built like infrastructure: five levels that fit a life and a client
> roster, an API that is the product rather than an export of a screen, and guarantees about
> backups, audit and deletion that are checked in a pipeline instead of promised in a brochure.**

Three deliberate exclusions, because a position is defined as much by what it refuses:

* **Not "the open source Asana".** It invites a comparison on integrations and seats, both of which
  Hubtask loses today, and it is factually wrong about the licence.
* **Not "AI-powered".** AI is optional, switchable off, and the product with it switched off is the
  whole product ([`ai-first.md`](../architecture/ai-first.md) §2). A site that leads with AI would be
  advertising the one part a buyer may decide never to turn on.
* **Not "privacy-focused" as the headline.** Every self-hosted tool says that; it has stopped
  carrying information. Say the specific thing instead — the trail verifies, the restore is drilled,
  nothing phones home — and let the reader draw the conclusion.

### 3.1 The four audiences, in priority order

| # | Who | What they arrive worried about | The page that answers them |
|---|---|---|---|
| 1 | **The sovereign individual** — homelab, prosumer, developer running their own stack | "Will this still be mine in five years, and can I get my data out?" | Start → Self-hosting → Licence (the Change Date) |
| 2 | **The small provider** — agency, freelancer, consultancy running work for several clients | "Can one instance hold five clients without them seeing each other?" | Use cases → Security → Licence |
| 3 | **The compliance-bound European team** | "Can I answer an access request, prove the trail, and show an accessibility statement?" | Security and privacy → Accessibility |
| 4 | **The automation and agent builder** | "Is the API real, or is it a UI with an export button?" | Developers and agents → Roadmap |

Audience 1 is the volume and the community. Audience 2 is where money would come from. Audience 3
is what makes audience 2 *choose* Hubtask over a cheaper neighbour. Audience 4 is who writes about
it. The site is built in that order.

---

## 4. The five messaging pillars

Each pillar is one page's spine and one section of the start page. Each has a claim, a mechanism,
and a proof — and the proof is the thing no competitor's homepage carries.

| # | Pillar | The claim | The mechanism behind it |
|---|---|---|---|
| 1 | **Five levels, one shape** | A model that holds "buy milk" and a client's release plan without becoming two products | One generalised `WorkItem` with capability profiles; a new level is configuration, not a migration ([`domain-model.md`](../architecture/domain-model.md) §2) |
| 2 | **The API is the product** | Every capability exists once and is reachable from every door | One use case registry behind REST, MCP and automation; a parity check fails the build when the three disagree |
| 3 | **Agents on your own machine** | An agent can do whatever your role allows, and nothing more — without your tasks leaving the box | MCP as a presentation adapter, scoped credentials, `actor.type = AI_AGENT` in the trail |
| 4 | **Provably yours** | Backups that have been restored, a trail that verifies, deletion that announces itself | The restore drill in CI; the audit hash chain and `:verify`; retention preview, grace period and legal hold |
| 5 | **Yours to run** | One image, PostgreSQL, and nobody to ask | Compose or Helm, no telemetry, no licence key that switches anything off, no font loaded from a foreign domain |

### 4.1 The proof device

**Every substantial claim on the website carries a link to the thing that proves it** — the
architecture document, the ADR, or the named CI gate. This is presented as a small recurring
element, described in [`website-1.0.md`](./website-1.0.md) § 3.

It is worth naming why this is a marketing decision and not an engineering habit that leaked onto a
website. Every competitor's homepage makes claims of this shape. None of them can be checked in
under a minute. A site where they *can* be is making a second, larger claim by its structure — that
this project does not say things it cannot show — and that claim is the one the target audience
actually buys. It also disciplines the copy: a sentence with no link is a sentence somebody has to
justify keeping.

---

## 5. Licence and monetisation — what the site may say

**The licence is decided; the money is not.** These are two different sentences and the website must
not blur them.

*Decided, and therefore stated plainly:* Business Source License 1.1, with free use — including
production self-hosting with the full feature set — for private and household purposes, non-profits,
teaching and non-commercial research, and for evaluation and development by anyone. Each released
version converts to Apache-2.0 three years after it is published, per version and irrevocably. That
is [ADR-0013](../adr/ADR-0013-licensing.md), it is the text in [`LICENSE`](../../LICENSE), and it is
the single most persuasive thing on the site for audience 1 — because it is the answer to "what if
this project is abandoned or sold".

*Not decided, and therefore said as not decided:* what a commercial licence costs, how it is
metered, and exactly where the commercial boundary falls.
[`licensing-editions.md`](../architecture/licensing-editions.md) §2 already records that the
freelancer line "is the line most likely to be revisited", and the owner's position in September 2026
is that the monetisation model waits on a final market analysis before `1.0.0`.

**How to present that — the recommendation.** Not as a gap, and not as a coming-soon banner. As a
commitment with a date:

> *What a commercial licence costs is not decided yet. It is being worked out before 1.0, and it
> will be published here before anyone is asked to pay for anything. What is already decided, and
> cannot be taken back, is the part that protects you: every version is free for private use, and
> every version becomes Apache-2.0 three years after its release.*

Three reasons this is the right presentation rather than silence. A buyer in audience 2 who cannot
find a price assumes the worst and leaves; a buyer who is told the price is unsettled *and* told the
guarantee that is settled has been given something. Saying it in public is a commitment against
quietly worsening the terms later. And "we would rather get this right than guess" reads as
seriousness in a category where surprise repricing is a live memory for anybody who has watched a
BSL relicensing.

**What the site must never say**: "open source" (it is source available, and
[`apps/website/CLAUDE.md`](../../apps/website/CLAUDE.md) already makes that a build-level rule), any
price, any date for a price, or that free private use is permanent without the condition
[`licensing-editions.md`](../architecture/licensing-editions.md) §5 attaches to it.

---

## 6. What the site does not collect, and why that is a position

No newsletter, no waiting list, no early-access form, no analytics, no cookies, no embedded video,
no font from a foreign domain, no script at all. Partly this is inherited discipline — each of those
would need a data-catalogue entry with a legal basis and a deletion path
([`data-protection.md`](../architecture/data-protection.md)). But it is also the argument: a product
whose pitch is "your data stays yours" cannot open with a consent banner. **The website is the first
demonstration of the product's claim**, and the cheapest one to get right.

The consequence to accept with open eyes: there is no way to reach people who visited and were not
ready. The substitutes are GitHub stars, the repository's release feed, and GitHub Sponsors — all of
which the visitor opts into on somebody else's terms rather than ours. If a newsletter is ever
wanted, it is a decision with a data-catalogue entry attached, not a form somebody adds.

---

## 7. Risks in this positioning

| Risk | Why it is real | The mitigation already available |
|---|---|---|
| **"Built like infrastructure" reads as "hard to use"** | The proof device and the security page pull hard in that direction | The start page leads with the model and the five levels, not with the audit trail. Rigour is the second thing a visitor meets, never the first |
| **The private user does not care about audit trails** | They genuinely do not, until they do | Audience 1's page is the licence and the self-hosting page. The compliance material lives behind its own navigation entry and is not pushed at them |
| **BSL costs community goodwill** | Some contributors and some aggregator sites treat non-OSI licences as disqualifying | State it plainly and first, with the Change Date beside it. The projects that were damaged by BSL were the ones that *changed to* it; this one starts there |
| **A 1.0 site claiming client parity before the shells ship** | The site is written for 1.0 and today is 0.7.0 | The claim ledger in [`website-1.0.md`](./website-1.0.md) § 5: every sentence that is not true today is listed, with the milestone that makes it true. The site does not go live until they are |
| **No mobile story against Todoist** | It is the first question a private user asks | Do not fight there. The download page says what exists, per platform, from the support matrix — and says nothing about what it is like to use until it is worth saying |

---

## 8. What this document does not decide

The launch moment, the trademark filing ([`roadmap.md`](../roadmap.md) `1.0.0` prerequisite 7), the
wordmark — [`design-system.md`](../design/design-system.md) §9 still lists it as unfinished, and the
website currently uses the three-nested-planes placeholder from the workbench — and whether
hubtask.eu ever carries a page in German beyond the two legal ones. Each is the owner's, and each is
cheaper to decide once there is a site to look at.
