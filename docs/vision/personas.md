# Personas

The people Hubtask is for. A use case names who acts in it (`actors:`), and a reviewer reads the
screen as that person would: what they know, what they do not, what would confuse them.

One person can be several personas at once — in a private installation the owner, the
administrator and the operator are the same human. That is the case [P-10](./principles.md#p-10-the-smallest-deployment-comes-first)
protects: the product must not make them feel it.

## People who use Hubtask

| ID | Who | Knows | Wants | Is lost by |
|---|---|---|---|---|
| `PE-person` | Somebody organising their own life or work | Their tasks; nothing about servers | Capture fast, find again, be reminded, never lose anything | Jargon, settings they did not ask for, a sign-in that asks for what they cannot give |
| `PE-member` | A person working in a shared workspace: family member, colleague, club member | Their part of the work | See what is theirs and shared with them, hand work on, be told when something changes | Seeing other people's things, controls they may not use |
| `PE-guest` | Somebody given access to a few items: a client, a craftsman, a grandparent | One thing they were asked to look at or do | Get in with the least effort, do that one thing | Anything beyond that one thing |
| `PE-child` | A young member of a family workspace | Their own lists | A simple view of their chores and school work | Everything else in the workspace |

## People who run a workspace

| ID | Who | Knows | Wants | Is lost by |
|---|---|---|---|---|
| `PE-owner` | Whoever a workspace belongs to: the person themselves, a parent, a company's accountable manager | Why the workspace exists | Decide who is in, keep it safe, get everything out | Being locked out of their own workspace, losing it to an administrator |
| `PE-admin` | Somebody who runs a workspace for others: the family's technical person, a team lead, a company's IT administrator | People, roles, sign-in, backups | Invite, structure, set rules once, see what happened | Two screens for one rule, a rule they cannot tell is in force |
| `PE-auditor` | A person who must check without changing: data protection officer, internal audit | Compliance duties | Read configuration and the trail, prove it is intact | Content they should not see, a trail they cannot verify |

## People who run an installation

| ID | Who | Knows | Wants | Is lost by |
|---|---|---|---|---|
| `PE-selfhoster` | Somebody who installs Hubtask on their own machine or server, for themselves or their household | Docker, a little networking | `docker compose up`, a browser, done; updates without fear | A database shell on the first day, platform concepts they will never need |
| `PE-operator` | Whoever runs an installation for others: a provider's staff, an MSP, a company's platform team | Kubernetes or Compose, DNS, mail | Provision, set defaults and locks once, watch health, never see customer content | Tasks that need SQL, values without a type, counts that disagree |
| `PE-platform` | The purchase or customer platform in front of a hosted Hubtask — a machine, acting through a service account | The contract, the price, the customer | Create a workspace on purchase, change its plan, suspend it on non-payment, learn what happened | A credential that belongs to a person who leaves, having to poll |

## People who build on Hubtask

| ID | Who | Knows | Wants | Is lost by |
|---|---|---|---|---|
| `PE-integrator` | Somebody connecting Hubtask to n8n, Zapier, a calendar, a mail flow | HTTP, webhooks | A complete API, stable events, signed deliveries | A feature that exists only on a screen |
| `PE-agent` | An AI agent acting through MCP on somebody's behalf | The tools it is given | Deterministic, idempotent operations, clear refusals | Ambiguity, operations with side effects it cannot see |
| `PE-scripter` | A person at a terminal with `hubctl` or `curl` | The shell | Do what the web app does, in one line, scriptable | A command that cannot do what its screen can |
