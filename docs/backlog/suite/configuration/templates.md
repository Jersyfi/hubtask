# Templates — Hubtask's direction, the customer's own world

## 1. One kind of template

A **template** bundles building blocks so that one choice sets up a way of working. It replaces the
market's project templates, sample projects and solution packs, and it contains today's preset as
its structure part — a preset is a template that bundles only the eight structure values, so no
existing preset is lost.

| Part | Example in *ITIL service desk* |
|---|---|
| Structure (the preset's eight values) | levels *Request → Task*, board as the default view, figure *flow* |
| Modules it needs | workflows, service, knowledge, insight |
| Types, fields, forms | *Incident*, *Service request*, *Problem*, *Change*; impact, urgency, computed priority; forms with conditions |
| Workflows and their steps | incident: New → Assigned → In progress → Waiting → Resolved → Closed |
| Calendars and SLAs | business hours Mon–Fri 8–18; P1 response 15 min, resolution 4 h |
| Rules | escalate a P1 to the on-call group; close resolved incidents after 5 business days |
| Views and dashboards | queues *Mine*, *Unassigned*, *About to breach*; SLA and flow dashboard |
| Pages | a knowledge space, the guide "How we work with this desk" |
| Roles it asks for | *Service desk agent*, *Change manager*, *CAB member* — mapped to groups when applied |
| Questions on applying | working hours, time zone, which group answers first |
| Sample entries (optional, marked) | three incidents to try the flow on, removed in one click |

Scopes: a **hub template** (most templates), a **collection template**, and a **workspace starter**
(several hubs, asked once at the first start — UC-WRK-30 check 5's starter set, grown).

## 2. Four origins, as presets have today

| Origin | Who keeps it | Offered to |
|---|---|---|
| Hubtask | the project, per release | every workspace, unless the installation or plan withholds it |
| Installation | the operator; may offer, withhold, lock a default per plan | the installation's workspaces |
| Workspace | `CONFIGURE` at the workspace | everybody in it |
| Own | a person | that person |

Any configured hub can be saved as a template ("Save as template"); it records the template it came
from.

## 3. Versions and upgrades

* A template has a version. A hub or collection created from one records `template@version`; its
  configuration is **copied**, as presets are today — nothing follows a template silently
  (consistent with decisions #1619–#1622).
* When a newer version exists, the place shows *Update available*. The update is a
  **three-way comparison**: what the template changed between the two versions, against what the
  place changed itself. Each change is taken or left one by one; a change the place made itself is
  never overwritten without being shown. Taking changes is a plan and an apply like any other
  (`config-document.md`).
* A template is linted before it is published (`ui.md` A5); a broken template is not offered.

## 4. Hubtask's catalogue (the direction it gives)

Grouped by what people come for; each names the standard it follows (`standards.md`).

| Family | Templates |
|---|---|
| Personal and household | List, Tasks, Household, Shopping, Habits, Travel |
| Teams | Tasks, Kanban team, Scrum team, Bug tracking, Content calendar, Event planning |
| Projects | Classic project (PRINCE2 / PMBOK-aligned: stages, tolerances, RAID, change control, baselines), Hybrid project, Client project (agency) |
| Programmes and portfolio | Portfolio, OKR, Scaled agile (PI planning, ART board, ROAM risks) |
| Service | ITIL service desk, Internal IT, Facility requests, HR requests, Customer support |
| Business processes | Approvals (purchase, leave, travel), Onboarding / offboarding, Procurement, CAPA (ISO 9001), Risk and controls register (ISO 27001), Records of processing (GDPR) |

The catalogue is curated: few, good, standard-aligned. Customers build their own world on them.
