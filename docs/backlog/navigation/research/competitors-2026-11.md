# Competitor navigation research (for the Hubtask navigation concept)

Researched 2026-11-07, web only. Statements are from the cited sources unless marked **[memory]**
(from prior knowledge, not checked against a source in this run) or **[not found]**.

---

## 1. Atlassian unified navigation (Jira, JSM, Confluence, Home/Goals, Loom) — 2024–2026

### Timeline and why
- Early Access Program Oct 2024 for 6 products; rollout from 17 Mar 2025 across Jira, JPD, JSM,
  Confluence, Atlassian Home; 100 % of users by July 2025; individuals could opt out until mid-2025.
- Full-height sidebar announced for Dec 2025 across apps. Stated reason: in the old partial-height
  layout, some parts were fixed and some scrolled, which made toggling confusing, and the
  expand/collapse button was hard to find.
- Design rationale (Atlassian design blog): each product had its own navigation code and
  inconsistent navigation made users relearn tasks; "finding work" was a recurring complaint. The
  2018 redesign forced one identical navigation everywhere and didn't fit product-specific needs, so
  this time the aim was **consistency balanced with flexibility**. Principles: predictability through
  repeated patterns, user control (show/hide items), progressive disclosure.
- Research: 16 Jira users (wanted to customise, find frequent items faster); a Chrome-extension
  pilot with 160 users (valued customisation, wanted to hide items). Settings were harder to find at
  first in the new place, but matched other tools, so they invested in onboarding instead of moving it.
- Audit: hundreds of custom nav components, 23 major inconsistencies. Collapsed to **three building
  blocks: menu button, flyout menu, expandable menu** (required core props + optional description,
  hover actions, expand chevron).
- **Indentation was reduced from 26 px to 12 px** (one token change) because items truncated too
  often on small screens.

### Frame and levels
- **Top bar**: start = app switcher + brand/product (+ sidebar toggle); middle = global search across
  all apps + **Create**; end = AI chat (Rovo), Notifications, Help/Settings, Account. The redesign's
  biggest change: product navigation moved **from the top bar into the sidebar**; top bar keeps only
  universal actions.
- **App switcher** (top left): switch Atlassian apps and sites; can show recent history.
- **Sidebar (Jira)**: For you, Recent, Starred, Apps, Spaces (expandable, each space expands to
  its contents), Dashboards, Filters, Assets, Goals, Teams, **More**. Confluence: For you, Recent,
  Starred, Spaces (+ starred spaces when no space is open), Apps; when inside a space, the space's
  content tree is shown in the sidebar.
  - *For you* = personal hub (recent spaces, viewed/assigned work, starred). *Recent* = visited,
    edited or created items. *Starred* = pinned items.
  - **Customize sidebar**: More → Customize sidebar; checkbox per item to show/hide; drag or
    up/down arrow keys to reorder; personal, not visible to others. Hidden items go behind "More".
  - Space/project settings sit behind the space's **•••** in the sidebar (and the header in
    team-managed spaces).
- **In-space views = horizontal tabs** in the page header (Jira: Summary, Board, List, Timeline,
  Calendar, Forms, … ). Admins drag to reorder, hide via hover ••• → Hide, re-add via **+** in the
  tab bar; a default tab must exist and can't be hidden; List and Apps can't be hidden; changes apply
  to everyone in the space. Tab overflow handling **[not found]**; **[memory]** extra tabs collapse
  into a "More" dropdown.
- Level summary: product = app switcher (top bar); space/project = sidebar (expandable item);
  view = header tabs; item = page; breadcrumbs above the page title **[memory]**.

### Design-system components (`@atlaskit/navigation-system`, `@atlaskit/side-nav-items`)
- Public docs say the package is "currently only available to Atlassians" with frequent breaking
  changes; old `side-navigation` is deprecated.
- **Layout slots** (must be direct children of Root, in this order — sets tab, reading and skip-link
  order): Banner, TopNav (Start/Middle/End), SideNav (fixed header, scrolling body, fixed footer),
  Main (+ MainStickyHeader), Aside (resizable, not collapsible, default 330 px, 120 px–50 % vw,
  being deprecated), Panel (resizable, collapsible, right of Main). Each SideNav/Aside/Panel/TopNavEnd
  needs a unique landmark label. ≤1024 px: Panel becomes an overlay, Aside moves below Main.
- **SideNav**: resizable via `SideNavPanelSplitter` (double-click splitter to collapse); toggle via
  `SideNavToggleButton` in TopNavStart; shortcut fixed at **Ctrl+[** (since 5.0, not configurable;
  ignored when a modal is open; closes open layers). Panel min width = default width (≤400 px), max
  = half the viewport after the sidebar.
- **Peek**: a collapsed SideNav **peeks (flies out) on hover**; `onPeekStart/onPeekEnd` callbacks;
  slide/peek animation shortened 300 → 200 ms; peek suppressed while a popup in TopNavStart (e.g. the
  app switcher) is open; fixed bug where peek closed when the pointer crossed TopNavStart.
- Mobile: clicking outside the side nav always closes it; dropdown clicks no longer collapse it.
- **Drag and drop** in side nav via Pragmatic drag and drop; `LazyDragHandle` for custom items;
  iOS context menu disabled on draggable anchors. Drop indicator package has a **tree-item variant
  whose line shifts with the item's indent level** (and RTL); list variant: reorder-before, combine,
  reorder-after.
- **Expandable menu item**: chevron button labelled by the item's text (not "Expand/Collapse");
  pressed state on link-variant triggers. **FlyoutBody** got keyboard and infinite-scroll support.
- Hover: hovering an item reveals its actions (••• menu, sometimes +) — "in the same place across
  products, rather than scattered across settings pages".

### Confluence page tree
- Sidebar drag-and-drop (since 2019, cloud): reorder under same parent, move under a new parent,
  change nesting level; descendants move with the page; **collapsed target parent auto-expands while
  hovering**; **Esc cancels**; incoming links preserved; change affects every user of the space;
  "Reorder pages" permission applies; initially only for spaces ≤500 pages. Undo was announced as
  "coming soon" — shipped status **[not found]**.
- Alternative: Content tools → Reorder pages, drag slightly right beneath a page to nest.
- Hover a page → **•••**: Edit, Rename, Star, Get link, Copy, Move, Archive; unpermitted actions
  greyed out. **[memory]** a **+** on hover creates a child page.
- 2025 redesign (K15t review): tighter spacing, slightly bolder type, easier to scan; **lost**
  alternate tree views (recently visited/updated, alphabetical); space-level apps pushed to the very
  bottom below the tree ("a long way down").
- Complaints (2025): accidental drags wreck curated spaces — no audit log, no undo; users ask for a
  confirmation before a drag move.
- Nesting depth limit **[not found]**; **[memory]** no hard limit, lazy loading of children on expand.

### Complaints and Atlassian's responses
- "Cramped" sidebar, more clicks, smaller and less distinguishable icons, banner-sized wasted space,
  "overwhelmingly negative" feedback from some admins; threads continue into Feb 2026.
- Praise: search; ability to hide unused views/nav items.
- Response: full-height sidebar (discoverable toggle), 56 px top bar, careful handling of custom
  branding sites. A user read the full-height change as hiding the app switcher/logo when collapsed
  (unconfirmed).

### Mobile
- Jira mobile: bottom tab bar with **Home** (recent/favourite issues, projects, boards, queues)
  added 2021; notifications with Unread/Direct filters. 2025 mobile changes **[not found]**.
  **[memory]** tabs: Home, Projects/Spaces, Notifications, Search, plus a create action.

Sources:
- https://www.atlassian.com/blog/design/designing-atlassians-new-navigation
- https://support.atlassian.com/navigation/docs/what-is-the-new-navigation-in-jira/
- https://support.atlassian.com/navigation/docs/what-is-the-new-navigation-in-confluence/
- https://support.atlassian.com/jira-software-cloud/docs/manage-and-customize-the-project-navigation/
- https://community.atlassian.com/forums/Navigation-Refresh-articles/A-full-height-sidebar-for-better-navigation/ba-p/3144347
- https://community.atlassian.com/forums/discussion/2800982/the-new-navigation-coming-to-you-in-2025
- https://cdn.jsdelivr.net/npm/@atlaskit/navigation-system@9.2.1/constellation/layout/usage.mdx
- https://cdn.jsdelivr.net/npm/@atlaskit/navigation-system@9.2.1/CHANGELOG.md
- https://atlassian.design/components/side-navigation/usage
- https://atlaskit.atlassian.com/packages/pragmatic-drag-and-drop/react-drop-indicator
- https://community.atlassian.com/t5/Confluence-discussions/Organizing-your-space-just-got-easier-Page-Tree-Drag-amp-Drop-is/m-p/959282
- https://confluence.atlassian.com/confcloud/move-and-reorder-pages-724764858.html
- https://community.atlassian.com/t5/Confluence-articles/Do-more-directly-from-the-Confluence-page-tree/ba-p/1750264
- https://www.k15t.com/rock-the-docs/news/2025/01/62-the-mostly-better-confluence-redesign
- https://community.atlassian.com/forums/Navigation-Refresh-discussions/Feedback-about-degraded-user-experience-and-usability-of-Jira/td-p/3194776
- https://support.atlassian.com/jira-cloud-android/docs/whats-new-in-jira-cloud-for-android/

---

## 2. Linear
- Sidebar: **Inbox**, **My issues**, **Workspace** (projects, initiatives, views; Teams and
  Customers links added Dec 2024), **Favorites**, **Your teams** (each team expands to Issues,
  Cycles, Projects, Triage, …). Levels: workspace → team → view; a single product, no app switcher.
- Dec 2024 "personalized sidebar": drag to reorder, right-click an item to edit, "Customize sidebar"
  for all options, rarely used items hidden behind **More**, unread as **count or dot**.
- Keyboard: Cmd/Ctrl+K command menu (open pages, run actions), `/` search, type an ID (LIN-123),
  two-key go-to shortcuts (G I Inbox, G M My issues), `?` shortcut list; Cmd-click opens in new tab.
- In-page view tabs (All / Active / Backlog) **[memory]**; collapsible sidebar **[memory]**.
- Mobile (Jan 2026): **personalisable bottom toolbar** — reorder main items (e.g. My issues,
  Pulse); pin projects, initiatives, documents via ••• on the item. Defaults and max count
  **[not found]**.

Sources: https://linear.app/enablement/guide/navigating-linear ·
https://linear.app/changelog/2024-12-18-personalized-sidebar ·
https://linear.app/changelog/2026-01-22-customize-your-navigation-in-linear-mobile ·
https://linear.app/changelog/2021-03-25-keyboard-shortcuts-help

## 3. Notion
- Sidebar: fixed Inbox (top right) and workspace name (bottom); top: Search (Cmd/Ctrl+K), Home, AI
  chats, Meetings, Library; sections Recents, Favorites, Agents, Teamspaces, Shared, Private, Notion
  apps; bottom: My tasks, Library, Marketplace, Help, Trash. Sections can be moved, hidden,
  collapsed by clicking the heading. **Sections appear only when they have content** (Favorites
  after the first favourite, Shared after the first share, Teamspaces on paid plans).
- Hover: **•••** (section: sort, item count, move, hide; page: delete, remove from favourites);
  **+** next to each teamspace, Private, and on any page to add a nested sub-page.
- Nesting: **no depth limit**; toggle arrows; database views shown as children marked "•".
- Show-more: Private and Shared show 5…all items; overflow behind "More", which opens a pane with
  search, sort and new-page.
- Drag: drop onto a page = nest (target highlights blue); drag out = un-nest; **dragging from a
  shared area into Private removes everyone else's access** (explicit warning in docs); drag to
  Trash deletes with sub-pages (30-day trash).
- Collapse: `<<`/`>>` or Cmd/Ctrl+\; resize by dragging the right edge. Peek-on-hover **[memory]**:
  hovering the left edge shows the collapsed sidebar as an overlay.
- New page without location goes to Private (preview mode lets you choose).
- Mobile: 2020 bottom bar (search, notifications, create in one tap); a newer **top tab bar**
  (Home, AI Chats, Meeting Notes, Inbox) per a third-party blog, undated. Page tree on phone
  **[memory]**: drill-down via Home / sidebar sheet.

Sources: https://www.notion.com/help/navigate-with-the-sidebar ·
https://www.notion.com/help/guides/structure-sidebar-focused-work-teamspaces ·
https://www.notion.com/releases/2020-02-19 ·
https://matthiasfrank.de/en/notion-updates/new-tabbed-mobile-navigation/

## 4. ClickUp
- Hierarchy: Workspace → Spaces → (optional) Folders → (optional, one level) Subfolders → Lists
  → tasks → subtasks. Subfolders can't nest further. Lists may live in Space, Folder or Subfolder.
- "Modern" sidebar layout (3.0): Spaces, Folders and Lists in one sidebar, separation between the
  three removed, "frees up screen room"; earlier layouts Simple UX (2020) and Clean.
- Complaints: Clean layout (visual separation of Spaces) removed; "white on white on white … it all
  looks the same"; clunky with many Spaces. **4.0**: Home got its own sidebar (Spaces, Chats, All
  Tasks) **duplicating** the main sidebar beside it; nav bar shifts when sidebar collapses; Inbox/My
  Tasks harder to find; Inbox can't be pinned; personal layouts lost on switch; forced rollout.
- Views as tabs per List/Space **[memory]** (List, Board, Calendar, Gantt, + View).

Sources: https://feedback.clickup.com/changelog/modern-layout ·
https://feedback.clickup.com/feature-requests/p/bring-back-clean-layout-on-the-sidebar-for-separation-of-spaces ·
https://feedback.clickup.com/feature-requests/p/change-40-sidebar-navigation-back ·
https://help.clickup.com/hc/en-us/articles/13856392825367-Intro-to-the-Hierarchy (via search snippet)

## 5. Asana
- 2023 sidebar: "My views" (My tasks, Inbox, My dashboard) at top; Insights (Reporting, Portfolios,
  Goals); Favorites renamed Starred, auto-populated by usage.
- Oct 2025 test: sidebar split into **four modes — Work, Plan, Workflow, Company**, "organizes
  features by the type of work you're focusing on", meant to scale to new features.
- Complaints: wastes horizontal space (asked for collapse), **starred items split across modes**,
  lower-tier users see paid features they can't use, Teams slower to reach, smaller font, no
  opt-out, admins couldn't tell who was in the A/B test.
- Mobile **[not found]**.

Sources: https://forum.asana.com/t/asanas-new-navigation-experience/1104749 ·
https://help.asana.com/s/article/new-navigation-in-asana ·
https://asana.com/inside-asana/more-navigation-improvements ·
https://forum.asana.com/t/ultimate-guide-to-the-new-sidebar-redesign-get-your-team-prepared/356082/1

## 6. Microsoft 365 / Teams / Loop
- Teams **app bar** (left rail): apps pinned by user (right-click Pin, drag from "More apps") or by
  admin policy (e.g. Copilot auto-pinned for licensed users); an unpinned open app shows italic and
  disappears when you switch away. Mobile: Copilot in Teams' header, Outlook mobile in the tab bar.
- Personal-app design guidance: tabs of equal hierarchy, the first tab holds the most relevant
  content, merge closely related tabs, never open to a blank screen — show content tied to the
  user's activity; keep in-host navigation minimal.
- Loop workspace sidebar **[not found]**.

Sources: https://learn.microsoft.com/microsoftteams/platform/concepts/design/personal-apps ·
https://mc.merill.net/message/MC922627 ·
https://www.techrepublic.com/fr/article/how-to-customize-the-microsoft-teams-app-bar/

## 7. monday.com
- Left pane is the hub: workspaces (dropdown switcher), folders (via ••• above Favorites),
  boards/dashboards/docs; Favorites (starred boards, dashboards, docs). Newer layout adds AI
  Sidekick, Vibe, Notetaker, Agents, Apps, More.
- Complaint: in a new UI Favorites were buried behind the workspace switcher; later moved back to
  the top level of the pane. Users asked for the older workspace bar back.
- Board views as tabs **[memory]**.

Sources: https://support.monday.com/hc/en-us/articles/360009142179-The-favorites-section ·
https://community.monday.com/t/whats-the-point-of-favorites-if-theyre-not-readily-available-in-the-new-ui/63169 ·
https://support.monday.com/hc/en-us/articles/115005316845-Folders ·
https://support.monday.com/hc/en-us/articles/35276662798098-Navigating-monday-s-AI-work-platform

---

## Patterns worth adopting
- **Top bar for universal actions only (switcher, search, Create, notifications, account); product
  navigation in the sidebar** — Atlassian's biggest change; a bird's-eye view that dropdowns can't give.
- **One frame across all products, product-specific sidebar content** — Atlassian's lesson from the
  2018 redesign: consistency of shape, flexibility of content.
- **Three primitives: menu item, expandable item, flyout** — covers almost every nav need and keeps
  products consistent.
- **Personal sections: For you / Recent / Starred** at the top of the sidebar — the core research
  wish was "find my frequent things faster".
- **Personal "Customize sidebar"** (show/hide, drag or arrow-key reorder, overflow into "More") —
  most-praised part of Atlassian's and Linear's changes; personal, invisible to others.
- **Hover actions (••• and +) in a fixed position on every item** — one place for item actions
  instead of settings pages; + creates a child in place.
- **Small indent (~12 px) per level** — Atlassian measured truncation at 26 px.
- **Collapsible + resizable sidebar, peek on hover when collapsed, Ctrl+[ toggle, double-click
  splitter collapses** — keeps space without losing access; full-height so the toggle stays findable.
- **Views as header tabs with admin reorder/hide/+add and a guaranteed default tab** — clear
  separation of "where" (sidebar) from "how I look at it" (tabs).
- **Drag-and-drop tree: line indicator that follows indent level, auto-expand collapsed parent on
  hover, Esc cancels, children move with parent, arrow-key alternative** — the Confluence/Pragmatic
  rules.
- **Show-first-N with "More" opening a searchable pane** (Notion) — keeps long sections bounded.
- **Sections appear only once they have content** (Notion) — clean first-run sidebar.
- **Cmd-K command menu, `/` search, go-to two-key shortcuts, `?` cheat sheet** (Linear).
- **Mobile: short, personalisable bottom bar + pinning** (Linear 2026); Home of recent items as a tab
  (Jira) instead of reproducing the tree.
- **Landmark-ordered layout slots with unique labels** — accessibility and skip links for free.
- **Never open a product on a blank screen; first view shows the user's own activity** (Teams).

## Anti-patterns to avoid
- **Duplicated navigation (a Home sidebar repeating the main sidebar)** — ClickUp 4.0's top
  complaint.
- **Flattening levels until they look the same ("white on white")** — ClickUp Modern; keep visual
  distinction between levels and between containers.
- **Splitting favourites/starred across modes or products** — Asana 2025; one Starred list.
- **Burying favourites behind a switcher** — monday.com had to move them back.
- **Pushing apps/actions to the bottom below a long tree** — Confluence "a long way down".
- **Undiscoverable collapse toggle / mixed fixed and scrolling regions** — Atlassian had to ship
  full-height sidebar.
- **Drag-and-drop that silently changes shared structure or permissions with no undo or audit** —
  Confluence complaints; Notion's drag-to-Private revokes access. Offer undo and warn on access change.
- **Showing features users can't use** (paid tiers) in the navigation — Asana complaint.
- **Forced rollout without opt-out, notice, or preserved personal layout** — Atlassian, Asana,
  ClickUp all drew backlash.
- **Small, indistinct icons and cramped sidebars eating horizontal space** — Atlassian/Asana complaints.
- **Deep indentation** — truncation on small screens (Atlassian 26 px).
- **Removing alternative tree views (recent, alphabetical) without replacement** — Confluence loss.
