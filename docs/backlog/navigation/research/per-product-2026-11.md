# Per-product navigation inside a space/container (for the Hubtask hub sidebar)

Researched 2026-11-07, web only. Builds on `competitor-navigation.md` (frame, sidebar primitives,
customise sidebar, page-tree drag rules, complaints) and does not repeat it. Statements are from the
cited sources unless marked **[memory]** (prior knowledge, not checked against a source in this run)
or **[not found]** (searched, no source).

---

## 1. Confluence space sidebar

### What is in the tree
- **One mixed content tree since 2023–2024.** Whiteboards stopped having their own home screen in
  the side navigation: "all whiteboards will appear as part of the content tree, what is currently
  known as the page tree". They are created "just like pages … directly from the content tree using
  the '+' icon or through the Create button". Whiteboards can be children of pages and parents of
  pages or other whiteboards; they inherit restrictions from their tree parent. Staff declined a
  request to keep whiteboards outside the tree: all whiteboards are treated the same as pages.
- Today the tree (the "content hierarchy") holds **pages, live docs, whiteboards, databases, Smart
  Links (embeds of external/other-space work), folders, slides**; **blogs are the exception — they
  "appear in their own section in the content tree"** (i.e. a separate, date-ordered section, not
  nested). Community: "all the pages, databases, whiteboards, live docs, smart links and blogs
  (although blogs get their own section…)".
- **Smart Links in the tree** (GA for all editions 2024): a link to a Jira board, Figma file,
  Google doc, other space etc. becomes a tree node; it "inherits the default permissions of the
  object above it". Added via **+** on the Content heading → Smart Link.
- **Folders** (beta 2024, now on by default, admin can switch off): "simple containers, without any
  information or context built into the container itself"; replace the old "empty parent page"
  workaround; visually distinct from content. **Created only from the tree** ("It is not possible
  to add a folder using the global create button"), because they encode hierarchy. Can nest under
  any content item. Restrictions flow down. **Deleting a folder does not delete contents — they move
  up a level.** (Rovo MCP, June 2026: no folder create/list yet — folders lag in APIs.)
- Per-type icons: each type has its own icon in the tree **[memory]**; not documented on support
  pages **[not found]**.

### Sidebar sections around the tree
- Order in a space **[memory, consistent with sources]**: space name (+ star, •••) → Overview (space
  home) → Content (tree, with **+** and •••) → Blogs → Shortcuts → space Apps (Calendars, Questions,
  Analytics, app add-ons). K15t's review complained the apps sit "a long way down" below the tree.
- **Space shortcuts**: admin-curated links to "helpful or important pieces of content", inside the
  space, to related spaces or the web; admins add and reorder them. Shared for all members.
- **Space features toggles** (Space settings → General → Features): Blogs, Calendars (Premium),
  Analytics (Premium), Automation (Premium), Whiteboards, Databases, Smart Links, Folders, Live
  docs, Slides. Turning one off removes its create option/section. This is Confluence's
  equivalent of ClickUp's ClickApps.
- **Space settings** are not a sidebar item: space name → ••• → Space settings.
- Tree view switcher: **Tree / My visits / Last updated / Title A–Z**; per-user, sticky; drag only in
  Tree view. Title search field on the tree (OR semantics). Hover cards show owner/last update.
- Show-more / lazy loading of long child lists **[not found]**; **[memory]** children load on
  expand, very long levels show "Show more".
- New pages default to creation-date order among siblings (alphabetical only via Reorder pages).

### Move, copy, archive
- Within a space: drag in tree or Move dialog; "attachments, comments, and child items" move with
  it; incoming links kept.
- **Between spaces: allowed.** Needs Space admin / "Manage all content in space" in the source and
  view + add in the destination. It is "essentially deleting them in their current space and adding
  them to the destination space"; links auto-redirect. Moving a folder to another space: community
  reports of folder contents missing afterwards (bugs).
- Copy ("Duplicate"): copies attachments, labels, restrictions; **not** comments, versions,
  watchers; optional "Include children".
- "Hide from tree" = move to root beside the home page (orphan); still in search; listed under
  Space settings → Content → Hidden.
- **Archive**: per-space archive; archived items leave the tree, search, feeds; viewable read-only,
  edit requires restore; restore returns to the original place. Archive permission per space; you
  can always archive what you created. Bulk archive Premium+. Whole spaces can be archived and show
  in an "Archived" tab of the space directory.

Sources:
- https://community.atlassian.com/t5/Confluence-Whiteboards-articles/Major-changes-are-coming-to-your-whiteboards/ba-p/2364522
- https://community.developer.atlassian.com/t/rfc-10-confluence-whiteboards/68942
- https://support.atlassian.com/confluence-cloud/docs/edit-features-for-a-space/
- https://support.atlassian.com/confluence-cloud/docs/improved-confluence-navigation/
- https://support.atlassian.com/confluence-cloud/docs/use-folders-to-organize-your-work/
- https://support.atlassian.com/confluence-cloud/docs/get-started-with-smart-link-embeds/
- https://community.atlassian.com/t5/Confluence-articles/Smart-Links-in-Confluence-s-Content-Tree-All-your-work-in-one/ba-p/2686218
- https://community.atlassian.com/forums/Confluence-User-Group/The-Content-Hierarchy/m-p/3161794
- https://support.atlassian.com/confluence-cloud/docs/decide-how-you-see-pages-in-a-space/
- https://support.atlassian.com/confluence-cloud/docs/move-copy-and-hide-pages/
- https://community.atlassian.com/forums/Confluence-questions/Support-creating-and-reading-Confluence-folders-via-the-Rovo-MCP/qaq-p/3247666
- https://www.atlassian.com/blog/confluence/introducing-page-and-bulk-archiving-in-confluence-cloud
- https://www.k15t.com/rock-the-docs/news/2025/01/62-the-mostly-better-confluence-redesign

---

## 2. Jira space (software / business)

- **"Projects" became "spaces"** across Jira Cloud, JSM, JPD from 1 Sep 2025 (staged through
  autumn). Cosmetic: data, APIs, JQL unchanged; no opt-out. Reason: free the word "project" for
  Atlassian Projects (Home). Atlassian staff admitted "trading one type of user confusion for
  another" (now Confluence spaces and Jira spaces share a word).
- **Sidebar vs tabs split.** Sidebar: a space is an expandable item; expanding shows little —
  users report it lists the space's **boards** (company-managed spaces can have several and
  cross-space boards). Settings via ••• next to the space name. Everything else is **horizontal
  tabs** in the space header.
- **Tabs**: admins set the default tab, rename, remove, re-add and reorder; changes apply to all;
  **List and Apps cannot be removed**; a default tab must exist and depends on type/style/features.
  Typical software set **[memory]**: Summary, Timeline, Backlog, Board (active sprint), Calendar,
  List, Forms, Goals, Releases, Reports, Code/Development, Security, Pages (linked Confluence space
  or Smart Links), Shortcuts, Components, Archived work items, Apps; overflow goes into "More".
  Exact list **[not found]** on support pages.
- **Filters, Dashboards, Plans, Goals, Assets, Teams are global** sidebar items, not per space;
  dashboards and filters are reachable "from anywhere in Jira", starred ones on top. A dashboard or
  filter is not owned by a space (it may only query one). **[memory]** Plans are a top-level
  sidebar item; a plan's scope is a set of boards, spaces and filters.
- **Starring / order**: starred spaces sit at the top of Spaces; since mid-2025 **starred spaces can
  be reordered** in the sidebar ("You can now reorder the starred projects in your sidebar
  navigation"); the non-starred list is not manually orderable **[memory: recent/alphabetical]**.
- **Space categories**: admin-defined labels used to filter the space directory **[memory]**; not a
  sidebar folder. No folders for spaces in the Jira sidebar **[not found]**.
- **Create**: "+" next to Spaces in the sidebar (or Settings → Spaces → Create space) → template
  gallery by app/category (Scrum, Kanban, Bug tracking, ITSM, Customer service, HR, Marketing…) →
  choose team-managed vs company-managed (separate from template). Team-managed creatable by any
  user by default; company-managed by Jira admins only. Custom space templates = Enterprise.
  A space can later be converted to another template/type.
- **Archive**: Premium/Enterprise only; Jira/site admins archive any space, space admins only
  team-managed; restore via Settings → Spaces → Archived. Standard plan only has delete (trash).

Sources:
- https://community.atlassian.com/forums/Jira-articles/Evolving-Jira-terminology-Projects-will-soon-be-Spaces/ba-p/3034977
- https://community.atlassian.com/forums/Jira-articles/More-on-the-move-from-Jira-Projects-to-Spaces/ba-p/3085938
- https://community.atlassian.com/forums/Jira-articles/Jira-Spaces-have-landed/bc-p/3119693
- https://blog.deiser.com/en/atlassian-changes-jira-project-to-jira-space
- https://support.atlassian.com/navigation/docs/what-is-the-new-navigation-in-jira/
- https://support.atlassian.com/jira-software-cloud/docs/manage-and-customize-the-project-navigation/
- https://community.atlassian.com/t5/Jira-articles/Jira-s-fresh-look-and-navigation/ba-p/2958737
- https://community.atlassian.com/forums/Jira-articles/You-can-now-reorder-the-starred-projects-in-your-sidebar/ba-p/3075508
- https://support.atlassian.com/jira-cloud-administration/docs/create-and-edit-a-project/
- https://support.atlassian.com/jira-software-cloud/docs/custom-project-templates/
- https://support.atlassian.com/jira-cloud-administration/docs/convert-a-project-to-a-different-template-or-type/
- https://support.atlassian.com/jira-cloud-administration/docs/archive-a-project/

---

## 3. Jira Service Management service space

- Sidebar (agent): **Queues, Customers (customers + organizations), Reports, Raise a request
  (opens the portal), Knowledge base, Channels** (email, help center, widget, Slack/Teams);
  operations items **Services, Alerts, On-call** can be added to the sidebar ("Add Services,
  Alerts, and On-call to your sidebar"). Admins show/hide sidebar items (Reports, Channels, KB).
- **Queues are the one product area that stays in the sidebar as a live list.** Selecting Queues
  opens a secondary list of all queues **with the work-item count per queue**. Queues can be
  **popped out** of the sidebar so they stay visible when the sidebar is collapsed.
- **Queue grouping**: **Starred** (personal, top) → **Priority group** queues (admin-defined named
  sets; agent switches group via the group name next to Queues; a queue may be in several groups;
  admins order queues within a group) → **View all / Other** queues (not in a group, "collapsed by
  default", counts **not** refreshed). Starred and group queues refresh counts on sync. Limits: 300
  queues per work category; 50 in the team priority group, overflow goes to Other. Starred queues
  cannot be reordered (feature request).
- **ITSM work categories**: Service requests, Incidents, Problems, Changes, Post-incident reviews —
  each a sidebar entry showing its own queues; enabling a category adds features (incident
  timeline, alerts, on-call; change calendar).
- Knowledge base, Directory, Customer notifications use **horizontal navigation** (tabs) inside
  their area — i.e. sidebar entry opens an app-like page with its own tabs.
- **ITSM template creates**: request portal, service catalog of common **request types** (each
  mapped to a work type), default **queues**, **SLAs**, four ITIL workflows (service request,
  incident, problem, change), change queues/reports/calendar, on-call/alerting.
- Assets in JSM: linked to incidents/changes via Assets fields; Assets itself is a global app
  (see §4).

Sources:
- https://support.atlassian.com/navigation/docs/what-is-the-new-navigation-in-jira-service-management/
- https://support.atlassian.com/jira-service-management-cloud/docs/get-to-know-your-service-project-sidebar
- https://support.atlassian.com/jira-service-management-cloud/docs/arrange-queues-into-groups-to-prioritize-work-for-your-team/
- https://support.atlassian.com/jira-service-management-cloud/docs/star-your-favourite-queues/
- https://community.atlassian.com/t5/Jira-Service-Management-articles/More-queues-better-queues-in-Jira-Service-Management/ba-p/2257860
- https://jira.atlassian.com/browse/JSDCLOUD-16012
- https://support.atlassian.com/jira-service-management-cloud/docs/how-categories-work-in-it-service-management-projects/
- https://support.atlassian.com/jira-service-management-cloud/docs/learn-about-the-it-service-management-template/
- https://www.atlassian.com/software/jira/templates/it-service-management

---

## 4. Assets (CMDB)

- **Global app, not per space**: "select Assets from your sidebar", then **Object schemas**. A
  schema is the container; JSM spaces reference schemas through Assets custom fields.
- Inside a schema: **left panel = schema tree of object types** (nestable: one parent, many
  children; "mainly for navigation and readability", empty object types allowed as groupers;
  optional attribute inheritance); right = objects of the selected type (list/detail). New object
  type at root or under the selected one via the icon at the top of the tree.
- So Assets is "a single entry that opens an app-like page with its own internal tree".

Sources:
- https://support.atlassian.com/assets/docs/building-an-object-schema-for-it-assets-management-itam/
- https://www.atlassian.com/software/jira/service-management/product-guide/tips-and-tricks/assets-cloud-get-started

---

## 5. Plans, Goals, Dashboards

- **Plans** (Premium/Enterprise): global; a plan's scope = chosen **boards, spaces and filters**; a
  sandbox until changes are saved back. Older limits: 5,000 items, 100 spaces per plan.
- **Goals** (Atlassian Home, formerly Atlas): global app; linked to **individual work items** (Goals
  field in list and item view), not to whole spaces — linking a whole space is an open feature
  request (ATLAS-335). Confluence strategy pages link goals. A Goals tab can be shown in a Jira
  space **[memory]**.
- **Dashboards**: global sidebar item; not owned by a space (see §2).
- Pattern: cross-container planning/reporting objects live outside the container and point in.

Sources:
- https://www.atlassian.com/software/jira/guides/advanced-roadmaps
- https://confluence.atlassian.com/advancedroadmapsserver0329/creating-plans-1021218735.html
- https://support.atlassian.com/jira-software-cloud/docs/link-and-unlink-goals-to-issues-in-your-jira-list/
- https://support.atlassian.com/platform-experiences/docs/understand-how-goals-and-projects-work-with-jira/
- https://jira.atlassian.com/browse/ATLAS-335

---

## 6. Others, compared

### Notion teamspaces
- Container: teamspace (sidebar section with **+** and •••); content: pages and databases in one
  nested tree; database views appear as children marked "•" (see earlier file).
- Move: drag into a teamspace or ••• → **Move to** → pick teamspace; needs **Full access**; moved
  content takes the teamspace's permissions. Full-page databases cannot become teamspaces.
- Sources: https://www.notion.com/help/transfer-content-to-another-account ·
  https://www.notion.com/help/guides/how-workspace-owners-can-set-up-teamspaces-for-their-organization

### ClickUp
- Container: Space (→ Folder → List). **ClickApps are toggled per Space** (in creation and Space
  settings; API "enable ClickApps for it"): custom fields, time tracking, sprints, etc.
- **Docs and Whiteboards** can be placed in the Spaces sidebar at Space/Folder/List level, or live
  in the Docs/Whiteboards hubs; also addable as a **view tab** on a List **[memory]**. Dashboards
  are hub-level; "Dashboards in the sidebar" is a feature request.
- Move List/Folder: ••• → Move → pick Space/Folder; moving a non-hierarchy object (doc, dashboard)
  needs Settings → Move To; drag-to-move for those is a feature request. Spaces cannot move between
  Workspaces.
- Sources: https://help.clickup.com/hc/en-us/articles/6309390855319-How-to-Create-a-Space ·
  https://developer.clickup.com/reference/updatespace ·
  https://help.clickup.com/hc/en-us/articles/14128804095767-Move-a-Doc ·
  https://feedback.clickup.com/feature-requests/p/drag-drop-from-sidebar-to-list-folder-or-space-view ·
  https://feedback.clickup.com/feature-requests/p/dashboards-in-the-sidebar ·
  https://feedback.clickup.com/feature-requests/p/move-spaces-from-one-workspace-to-another

### monday.com
- Container: workspace; content: **boards, docs, dashboards, forms in one tree**, grouped by
  **folders and nested subfolders** (API: folders contain folders). Drag in the left pane to
  reorder/move into folders; nested-folder drag reported as "glitchy" (opens all subfolders).
- Move to another workspace: ••• → Move to → Move to workspace, or drag; only board owners/admins,
  only to workspaces they belong to; moving a folder across workspaces leaves private/shareable
  boards behind (community). API "move object" supports folder, workspace and position.
- Sources: https://support.monday.com/hc/en-us/articles/7278527605906-Understanding-monday-com-s-structural-hierarchy ·
  https://support.monday.com/hc/en-us/articles/115005316845-Folders ·
  https://developer.monday.com/api-reference/docs/move-object ·
  https://community.monday.com/t/moving-boards-to-folders/120753 ·
  https://community.monday.com/t/folder-moving-between-workspaces/7525

### Linear
- Container: team (sidebar item expanding to Issues, Cycles, Projects, Triage, Views **[memory]**);
  projects can be **shared by several teams** with one lead team; a project page has list/board/
  timeline. **Documents are not a tree**: attached to projects, initiatives, teams, issues or
  cycles (project → Resources → + → Document). Issues move between projects with Shift+P.
- Sources: https://linear.app/docs/projects · https://linear.app/docs/project-documents ·
  https://linear.app/docs/documents

### Microsoft Loop / Planner
- Loop: workspace with a page tree; drag up/down to reorder, drop onto a page to make a subpage;
  ••• → Move to workspace (2023) vs "Add to workspace" (shares/links, link icon). Planner: plans
  pinned via "Add to pins" appear in the left list; "My Plans" aggregates plans from Planner, To Do,
  Loop, Teams, Outlook.
- Sources: https://learn.microsoft.com/en-us/answers/questions/4455611/how-to-remove-a-loop-page-from-a-workspace-(or-to ·
  https://sharepointmaven.com/share-a-page-between-multiple-loop-workspaces/ ·
  https://support.microsoft.com/en-us/planner/pin-selected-plans ·
  https://support.microsoft.com/en-us/planner/training/see-all-your-plans-with-my-plans

### Zendesk / Freshservice
- Zendesk: Views list in the agent sidebar with counts; shared (admin) and personal views;
  **agents reorder only personal views, admins both**; sidebar shows only the first N (older 12
  shared/8 personal; 2026 blogs say 100/10 — unconfirmed). Managed in Admin Center → Workspaces →
  Agent tools → Views.
- Freshservice: built-in views (e.g. "New & My Open Tickets"), saved custom views shareable by
  admins, starred views; **no manual reorder or alphabetical sort** (open ideas, 2026).
- Sources: https://support.zendesk.com/hc/en-us/articles/360036770674 ·
  https://www.eesel.ai/blog/zendesk-views-permissions-who-sees ·
  https://support.freshservice.com/support/solutions/articles/154774-filter-tickets-using-views ·
  https://community.freshworks.com/ideas/why-can-t-we-sort-ticket-views-alphabetically-46223 ·
  https://community.freshworks.com/ideas/enable-drag-and-drop-in-starred-views-42402

### ServiceNow
- Unified navigation header menus: **All** (every application/module — the classic navigator),
  **Favorites**, **History**, Workspaces, Admin; one menu can be pinned to stay open on the left.
- Service Operations Workspace: thin left rail (Home landing with assignments; Inbox; **List** icon).
  List panel has **Lists** tab (predefined, categorised: incidents, catalog tasks, requests,
  problems) and **My Lists** tab (personal). Workspace = app-like page per role.
- Sources: https://www.servicenow.com/docs/r/platform-user-interface/using-the-next-experience-global-header.html ·
  https://horizon.servicenow.com/guidelines/unified-navigation ·
  https://docs.servicenow.com/bundle/sandiego-it-service-management/page/product/service-operations-workspace/concept/service-operations-workspace-ui.html ·
  https://docs.servicenow.com/bundle/vancouver-it-operations-management/page/product/service-operations-workspace-itom/task/create-list-sow-itom.html

---

## Cross-cutting observations
1. **Documents-like content converges on one mixed tree** (Confluence pages + whiteboards +
   databases + links + folders; Notion; monday boards/docs/dashboards; Loop). Blogs (time-ordered)
   are the exception that gets its own section.
2. **Work-tracking containers do not put views in the tree**: Jira shows views as header tabs; the
   sidebar only lists boards. Linear: fixed sub-items per team.
3. **Service queues are the exception that stays a live sidebar list with counts**, grouped
   Starred → admin groups → collapsed "Other".
4. **Large structured apps (Assets, KB, Plans) are a single entry opening an app page with its own
   internal tree or tabs.**
5. **Cross-container objects (dashboards, filters, plans, goals) live outside containers and point
   in**; Atlassian has no per-space dashboards.
6. **Feature toggles per container** (Confluence Features, ClickUp ClickApps) decide which sections
   and create options appear.
7. **Folders are pure containers**, created only in the tree, deleting them lifts children up.
8. **Moves between containers are allowed but heavy** (Confluence: delete+add, needs admin in
   source; Notion: permissions change; monday: private boards left behind; ClickUp: no cross-
   workspace move). Container reordering is personal and usually limited to starred items.
