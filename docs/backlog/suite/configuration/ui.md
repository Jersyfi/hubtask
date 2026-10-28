# The administration's screens — structure, patterns, what changes

Built in the frame and grammar G1–G8 of the approved concept "Hubtask als ein System" (settings are
pages: sections on the left, content on the right, a save bar that appears on change, every value
with its source). Its mockups are the binding look.

## 1. Information architecture

### Workspace — *Administration*

| Group | Sections | Was |
|---|---|---|
| **Overview** | health (the configuration check, limits, failing rules, backups, requests owed), proposals waiting (agents' plans, template updates) | overview |
| **How we work** | Templates · Modules · Types & forms · Fields · Workflows · Calendars & SLAs · Rules & runs · Shared views & dashboards | presets, rules, runs (scattered) |
| **People & access** | People · Groups · Roles · Who may do what · Sign-in · Legal · Machines & apps | people, sign-in, legal, machines |
| **Connections** | Mail · Webhooks · Voice · AI | split over automation and AI |
| **Data & records** | Private hubs · Backup · Retention · Restore · Trail · Privacy · Limits | data, records, quotas |
| **Changes** | History & versions · Drafts · Configuration file · Sandbox | — |

### Hub and collection — *Settings*

The same sections, filtered to what the level may configure: General · Template · Types & forms ·
Workflow · Views · Rules · People. A section shows shared blocks read-only with *Make a copy here*
and their source, never a second editor.

### Installation — the same anatomy

Overview · Workspaces · Plans · Templates · Modules · Settings (the catalogue at this level) ·
Providers · AI · Backup targets · Platform · Encryption · Operators · Journal · Configuration file.

## 2. Patterns every settings screen follows

| # | Pattern | What the person sees |
|---|---|---|
| A1 | **Source on every value** | a chip — *Hubtask default*, *Installation*, *Plan Business*, *Workspace*, *Hub Support*, *Set here* — a lock where locked, *Reset* where set here |
| A2 | **Why** | the chip opens: where the value comes from, who locked it and when, what the reader may do, who to ask |
| A3 | **Preview before save** | the save bar says what the change reaches ("4 collections, 37 open entries move status, 2 people lose *Approve*") and opens the plan |
| A4 | **Draft and publish** for blocks shared by many (workflow, form, role, SLA, template) | edits are a draft; publish shows the plan |
| A5 | **Configuration check** | findings with severity and a fix link: a required field no channel can fill, a step nobody may take, a status no step reaches, a rule that loops, two fields with one meaning, a lock that could move up |
| A6 | **View as** | choose a person or role; the app is shown as they would see it, with a banner; "Can Anna create a *change* in *IT*? No — her role lacks the channel *Create* for that type" |
| A7 | **Settings search** | the palette finds a setting by words and lists it at every level the reader may reach |
| A8 | **Basic first, advanced folded** | advanced controls fold out on the same page, never a second page |
| A9 | **Start from a template** | empty sections offer templates first |
| A10 | **View as file** | every page shows its part of the configuration document — the bridge to `hubctl` and agents |
| A11 | **Who may do what** | a matrix of settings × levels × roles generated from the catalogue, filterable by person ("why may Lukas edit SLAs? — role *Service lead* at hub *Support*") |

## 3. By the person in front of it

| Person | What they configure | What they must never meet |
|---|---|---|
| Member (`PE-member`) | own views, notifications, own rules, own templates | settings of others, a control they cannot use |
| Requester, guest | nothing | the team's words, statuses, fields the form hides |
| Shaper of a hub or collection | the place's settings within locks | the workspace's sections |
| Administrator (`PE-admin`) | the workspace: how we work, people, connections, data | the installation's internals |
| Auditor (`PE-auditor`) | reads every section and the history, changes nothing | content |
| Operator (`PE-operator`) | the installation, plans, installation templates, locks | a workspace's content |
| Platform (`PE-platform`) | plans, provisioning, through the API | — |
| Agent (`PE-agent`) | what its token and role allow, through plan and apply | a hidden door with more abilities than the screen |

## 4. Existing screens that change

| Screen today | Change |
|---|---|
| Administration navigation (six groups) | the six groups of §1; *Changes* added |
| *Workspace* (`WorkspaceSettingsView`) | becomes *General*, with A1 on language, zone, week start where the installation sets them |
| *Presets* (`PresetsView`) | becomes *Templates*: the gallery of four origins, versions, *Update available* |
| *Quotas* | moves to *Data & records → Limits*, values with A1 |
| *People* (people, groups, permissions tabs) | *Roles* and *Who may do what* added; *permissions* tab becomes the matrix A11 |
| *Sign-in*, *Legal*, *AI*, *Mail*, *Voice* | adopt A1–A3 and the save bar; values set by installation or plan shown with their lock |
| *Rules*, *Runs* | one list for every level, filtered by place; the same list in a collection's *Rules* section (S-3 resolved) |
| Collection and hub settings (`ContainerView`, `StructurePanel`) | sections of §1; *Structure* becomes part of *Template*; *Fields* becomes *Types & forms* |
| Custom fields dialog (`CustomFieldsDialog`) | replaced by the form editor of the type |
| Capture (`capture/`) | renders the target type's form; unfolds required and conditional fields |
| Entry panel and page (`ItemView`) | render the same form in edit mode |
| Import, mail intake, voice | validate through the type's form; refusals name the field |
| Installation settings (`InstanceSettingsView`) | the same catalogue component as the workspace's |
| Plans editor | the same catalogue component, plus modules and templates offered |
| Search / palette | finds settings, templates and objects of every module |
| Today | unchanged in anatomy; module cards appear only where on (P-17) |
