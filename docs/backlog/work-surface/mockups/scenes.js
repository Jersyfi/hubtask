// The scene list of the work-surface concept: for each scene its id, title, place and numbered
// changes. The numbers are the red pins inside the drawn screen and the numbers of the README's scene
// table; an empty string keeps a number that has no pin of its own. UI labels in quotes are the
// product's German locale strings.
(function () {
  'use strict';
  const { S, PROD } = window.WB;
  const W = { d: 1440, t: 820, m: 390, p: 1440, e: 1440, w: 1440, o: 1440 };
  const DEV = { d: 'Desktop', t: 'Tablet', m: 'Phone', p: 'Beside the board', e: 'Empty', w: 'View workflow', o: 'Without workflow' };

  const RULES = {
    'rules-wb': [
      ['WB-1', 'One object, one shape', 'Entry, ticket, configuration item, goal, release and agent share one head: key with product tile, type, title, and under it the action bar with status, next step, person and due date. Who knows one knows all.'],
      ['WB-2', 'The product colour identifies, it means nothing', 'The sidebar\'s colour appears in a product home\'s head band, at the key, in the empty state and on widgets. Never as a status, never as a button, never in charts.'],
      ['WB-3', 'What matters first, the rest folded', 'Main column: description, parts, conversation. Everything else is in the property rail; an empty section is one line with "+". At most two levels.'],
      ['WB-4', 'Lists are calm tables', 'One region with hairlines, columns by the kind of collection, groups with counts and "+ entry", one act and ⋯ on hover. Density comfortable or compact, remembered on the device.'],
      ['WB-5', 'Figures on top, sentences instead of keys', 'Every product home and every head starts with its figures. Key figures as SummaryCard, trends as a chart in a fixed palette, with a table beside it.'],
      ['WB-6', 'Never empty when there is something', 'A product home does not open on an empty tab when the product has content. What is truly empty shows a sentence, a template as the main way and a side way.'],
      ['WB-7', 'Immediate and visible', 'Every change appears within 100 ms and arrives, moves and leaves in the existing motion roles. Errors appear in place, with undo where possible.'],
      ['WB-8', 'Every device, the same functions', 'Phone, tablet and desktop do the same; only the arrangement changes: a table becomes rows, a rail becomes a sheet, columns become strips, the primary action floats above the bottom bar.']],
    'rules-s': [
      ['S-1', 'Reading first', 'A 720 px reading column (about 70 characters) at 17 px, line height 1.65. Tools appear only when writing: above selected text, with "/" and at a line\'s handle.'],
      ['S-2', 'Trust at the title', 'Right under the title one line: who answers for the page, "Geprüft bis …" or "Prüfung fällig" as a lozenge, who changed it last, the labels.'],
      ['S-3', 'The outline writes itself', '"Auf dieser Seite" comes from the headings, follows on the right and shows where one is. Nobody has to insert a table of contents; the ::toc block stays nonetheless.'],
      ['S-4', 'Work lives in the page', 'Entries, views and boards are live blocks: a head with product tile, "Live" and a way to the original. A checklist line becomes a task in place.'],
      ['S-5', 'Conversation in the margin', 'A comment marks its passage; its card stands beside it at the same height. Comments on the whole page are at the top of the same column, as "Zur ganzen Seite"; writing happens at the foot of the column.'],
      ['S-6', 'Starting without a hurdle', '"Neue Seite" opens the page at once. Templates are chips under the empty title with their preview beside; "/" and "@" are explained in the empty text.']],
    'rules-w': [
      ['W-1', 'The surface belongs to the content', 'Controls as small islands in the four corners and a dock at the foot, together under 8 % of the surface. No head above the board; title, back and state are in the top-left island.'],
      ['W-2', 'One dock, grouped', 'Move · Create · Draw · Conversation · Insert. Every tool shows its shortcut (V, H, N, R, T, L, F, P, M, E, C). Shapes sit in a fan at the tool, not in a select.'],
      ['W-3', 'Tools spring back', 'After a note, select is active again. A double click holds a tool, the held space bar is the hand, Esc always leads back. That avoids errors from modes.'],
      ['W-4', 'Edit where you are', 'A bar above the selection in a fixed order: colour, size, arrow, comment, lock, task from it, ⋯. Connect by dragging a handle, "+" adds the neighbouring note, Tab the next. Drag width and height instead of typing them.'],
      ['W-5', 'Moderation visible to all', 'A running timer or vote stands as one bar at the top centre, the same for everybody. Faces at the top right; a click on one means follow, and a coloured frame shows that one follows.'],
      ['W-6', 'Ideas become work', '"Aufgabe daraus" in the selection bar; for a whole frame "3 Aufgaben". The card shows key, person, status and tick, live like the entry itself.'],
      ['W-7', 'Recognised by its picture', 'Every board has a thumbnail on the product home, in search and embedded in pages. The picture is a reduced drawing of its objects, not a screenshot.']],
    'rules-b': [
      ['B-1', 'A post is a page with a date', 'The same editor, blocks and templates as pages, plus its own ("Ankündigung", "Release-Notiz", "Monatsrückblick", "Willkommen"). Cover, icon and labels as on pages.'],
      ['B-2', 'In the navigation under the contents', 'A section "Beiträge" at the very foot of the hub column, newest first, unread with a dot, then "Alle Beiträge". The only addition to the navigation in this concept, at the owner\'s request.'],
      ['B-3', 'Schedule and feature', '"Veröffentlichen" means now or at a day and time. Scheduled posts are seen only by writers. "Hervorheben bis …" keeps a post on top; afterwards it slides into the list.'],
      ['B-4', 'Discussion below, comments at the text', 'Under the post the discussion: every reader, with replies. At the text the comments as on pages, mostly while drafting. Two names because they are two conversations; a page has only comments.'],
      ['B-5', 'Make arrival visible', '"Gesehen von 38" for everybody; "Lesebestätigung erbitten" with a deadline shows writers "21 von 45" and "Erinnern". Whoever follows the hub is notified; "Neu für dich" appears on Today.'],
      ['B-6', 'The audience only narrows', 'A post belongs to a hub. "Wer ihn sieht" can narrow to groups or give the post to everyone in the workspace; nobody ever sees what they may not read in the hub.']],
    'rules-et': [
      ['ET-1', 'Content in the middle, properties on the right', 'Main column: description, parts, links, affected items, files, conversation — everything that can grow. On the right only properties and short tools (time, requester). So the rail never grows long.'],
      ['ET-2', 'Jump marks with counts', 'Under the head one line "Beschreibung · Teile 2/8 · Verknüpfungen 7 · Dateien 9 · Unterhaltung 13". It stays on top while scrolling and shows where one is. On the phone they are tabs.'],
      ['ET-3', 'Long sections fold themselves', 'Description from 12 lines with "Mehr zeigen"; parts without the done ones on request; links from 5 per group "Alle N"; files from 20 as a list; comments the newest three, then "ältere".'],
      ['ET-4', 'One place for files', 'Uploaded and linked files stand together under "Dateien", as a grid with previews or as a list; linked ones carry their store\'s mark. Dropping works anywhere on the entry. A click opens the file viewer.'],
      ['ET-5', 'Writing as on pages', 'Description and comments get the pages\' editor with the basics: paragraph, bold, italic, strike, code, link, list, checklist, quote, @ for people and entries, / to insert, files. Still stored as Markdown (E-3).'],
      ['ET-6', 'Deadlines as a matrix', 'One row per policy (the requester\'s SLA, then each OLA with its group\'s name), one column per goal (first reply, next reply, resolution). Three columns always suffice; rows grow with the OLAs. The action bar carries one chip: what is breached and the next deadline. Second round: the deadlines stand in the rail as rows by urgency; the matrix opens from "Alle Fristen als Tabelle".'],
      ['ET-7', 'Reply or note — never confused', 'Tabs "Antwort an Hannah Albers | Interne Notiz"; the note is yellow with a lock; the button says "Antwort senden" or "Notiz speichern"; beside it who sees it. Snippet and macro are icons in the bar; the macro menu says what each macro does.']],
    'rules-in': [
      ['IN-1', 'One name: Inventar', '"Inventar" and "Element" everywhere; "Konfigurationselement" only in the help.'],
      ['IN-2', 'An item is an object like every other', 'The same head, jump marks and properties on the right as the entry. The stage is a step bar, because it is a path.'],
      ['IN-3', 'Work on an item visible', 'Every ticket and change that names an item or acts beneath it stands at the item, with its reason.'],
      ['IN-4', 'Upkeep in one place', '"Pflege" gathers what today lies on seven sub-pages, as a list of tasks with one act each.']],
    'rules-q': [
      ['Q-1', 'One language, one checker', 'One parser on the server, one QueryLine component in the design system. Products bring fields, not dialects. Help, suggestions and checks come from the same list of fields.'],
      ['Q-2', 'Chips and text are one line', 'A typed "type: Störung" becomes a chip; a click on the chip changes it. What has no chip form stays as a text chip in the line. No switch, never "too complex".'],
      ['Q-3', 'Suggestions at every keystroke', 'First fields with kind and example, then values from the data with counts, then keywords. Keyboard first: ↑↓, ↵, comma for "or", minus for "not".'],
      ['Q-4', 'The count is there while typing', '"4 Einträge" appears before Enter. An error stands at its chip, with the suggestion that fixes it.'],
      ['Q-5', 'Permissions always hold', 'A query is computed for each person with their permissions, in the application layer (rule 2). A shared query shows nobody more than they may read. Free words never go into the address (UC-WRK-18/4).'],
      ['Q-6', 'Ask in words, answer as a query', 'Where AI is on, "Was hat Priya diese Woche zu Passkeys geschrieben?" writes a visible query one takes over and changes. Without AI only this line is missing.']],
    'rules-c': [
      ['C-1', '"Erstellen" picks the kind first', 'On top the line that already makes entries from a sentence (Q; UC-WRK-26 stays the one way for entries). Below the kinds: last created, what is possible here, "Anfrage an eine Abteilung". Foot: from template, import.'],
      ['C-2', 'The place is preset', '"in Team Atlas ▾" comes from where one stands, else from last time. Kinds one may not create there are absent rather than grey.'],
      ['C-3', 'Small in a dialog, large at once', 'Entry, goal, inventory item and time open their existing dialog. Page, whiteboard, post and dashboard open at once and empty; their templates come there.'],
      ['C-4', 'In place stays in place', '"＋ Eintrag" at the end of a list and "＋ Arbeitspaket" in the entry stay, as a line to type in instead of a dialog. Import stands in the foot of "Erstellen" and still under ⋯.'],
      ['C-5', 'Keys are learnt at the button', 'C or Q opens "Erstellen" everywhere; E, S, W, B, Z, D, K, T jump straight to a kind. The letters stand beside every kind.'],
      ['C-6', 'One search per place', 'Because the header search finds pages ("Hier" or "Seiten"), the pages home loses its own search field.']],
    'rules-f': [
      ['F-1', 'One viewer for every file', 'Full screen over everything: on top name, "2 von 9", download, link, edit; details on the right; at the foot all files of the object; ← → page, Esc closes. The same in entry, page, board, post, inventory and portal.'],
      ['F-2', 'See without downloading', 'Images, PDF, video, audio, text and code, Markdown, CSV and a ZIP\'s listing are shown by Hubtask itself. Office files are shown by the document server; without one the server converts to PDF. What cannot be shown is a card with type and size.'],
      ['F-3', 'Editing only outside', 'Hubtask edits no files. "Bearbeiten" appears only when the administration has entered a document server and one may write; saving creates a new version.'],
      ['F-4', 'Images lie on the board', 'PNG, JPG, GIF, WebP and SVG lie on the surface as images. Every other file becomes a card with a preview of its first page; a double click opens the viewer.'],
      ['F-5', 'Delivered safely', 'Files come from an origin of their own with a sandbox CSP and nosniff; SVG and Markdown only sanitised. Document-server keys are short-lived, per person and file.']],
    'rules-po': [
      ['PO-1', 'Inside: every department without a link', '"Anfragen & Hilfe" lists every department one may ask, with a search over request types and knowledge and "Meine Anfragen · überall". Reached through Create, through search and through "Alle Abteilungen" in every portal. The sidebar stays: back to work is one click.'],
      ['PO-2', 'Outside: only the portal', 'Whoever holds READ nowhere gets the portal frame: start, my requests, approvals, knowledge, language, account. No sidebar, no Today, no search over Hubtask, no Create. Several portals are chosen at the top.'],
      ['PO-3', 'Outsiders approve', 'Approvals for outsiders have a tab of their own with a count and stand on top of the start page (UC-SVC-09/5 exists); deciding works without opening the team\'s entry.'],
      ['PO-4', 'Share knowledge as a space', 'A space (a page with everything under it) can be shared with a portal\'s audience; restricted pages never. A customer gets their handbook, a department its part — without a copy (E-13).'],
      ['PO-5', 'One link, the right view', 'The link to a request opens the entry for the team and the request in the portal for its requester. Nobody lands on "Kein Zugriff".']]
  };

  const SCENES = [
    ['grammar', 'One object, one shape', 'ObjectHead in six products', ['Ticket, story, configuration item, goal, agent and release: key with product tile and type, title, the status as a button with its steps, the next step as the primary button, then person and due date. Before, each product had a head of its own.']],
    ['today', 'Today', '/ · where the app opens', [
      'Short things as one row of cards of equal height on top: overdue, due today, needs your reply, time today, waits for your yes. Each card has its one act. A missing role means a missing card.',
      'Below exactly two columns that end level: "Dein Tag" (overdue, today, upcoming folded, capture on top) and "Braucht dich" with the sprint under it. Both show at most eight rows and "Alle n", so no half column stays empty.',
      'Everything else flows as tiles over the full width (collections, new for you, inbox, recently opened) and fills gaps. Before, a short "Bei dir" stood beside a long right column.',
      'Tablet: one column in the same order. Phone: the cards as a strip to swipe, then your day, then needs you.',
      '"Abschnitte" at the top right hides or orders cards and tiles (not settable today; UC-WRK-34 "Where it ends", see E-17).']],
    ['queue', 'A queue as a list', 'Collection "Störungen & Anfragen" · list view', [
      'The types as lozenges and the figures in the head instead of "Workflow: Change · Störung …" as underlined links.',
      'A toolbar with words: search, filter, active filters as chips, group, columns, density, select.',
      'One region with hairlines instead of eleven separate boxes. The queue\'s columns: key, request, type, requester, priority, SLA as a bar with words, assignee. Groups by status with "+ Eintrag in …". On hover "Öffnen" and ⋯.',
      'Phone: title, one meta line with key, status and priority, the SLA bar under it. A swipe completes or opens the menu; the menu can do the same.']],
    ['board', 'Board', 'Collection "Team Atlas – Mobile App" · board view', [
      '', '',
      'A card like the list row: key with tile on top, person top right, title, labels, a foot with date, parts and comments. WIP in the column head, "+ Eintrag" at the column\'s foot, "Erledigt" folded narrow. A dragged card lifts (raised means movable).',
      'Phone: a column strip with counts, one column, swipe to the next, a long press opens "Verschieben nach …".']],
    ['hub', 'Hub overview', 'Hub "Produktentwicklung" · overview', [
      'The hub\'s tools as tiles with one figure and a small picture (sprint progress, week bars, a goal at risk) instead of five lines of text.',
      'Collections as items with description, people and progress.',
      '"Was sich getan hat" in sentences with person and object tile.']],
    ['deskhome', 'Product home: service desk', '/desk · for you', [
      'A head band with the product tile in the service-desk colour, a figures line and the primary action.',
      'A handler\'s four questions as key figures: my open ones, unassigned, first reply and resolution met.',
      '"Für dich" shows my tickets and the unassigned ones. Before, the page said no desk accepts requests while the desk had 10 open requests.']],
    ['portal', 'Portal', 'As a requester · /portal', [
      'A greeting with a large search: the knowledge base comes before the form.',
      'Request types as tiles with one sentence; the form appears only after the choice (as today).',
      '"Deine Anfragen" with a readable state and the next step in words. Before, the key "in_progress" stood there. The portal needs the status name from the server: an additive contract change.']],
    ['time', 'Time: my week', '/time', [
      'A head with the timer in the time colour and "Zeit erfassen" as the primary action.',
      'A week grid: entries by days, today marked, a totals row, each day\'s capacity as a bar. Before, a list of bookings under day cards.',
      'Hand in at the end, with one sentence on what happens next.',
      'Phone: the timer first, then the day strip and the day\'s bookings.']],
    ['goals', 'Goals', '/goals', ['Progress and confidence side by side, separate. Key results indented with "jetzt von Ziel". State with word and glyph.']],
    ['dashboard', 'View a dashboard', 'Ziele › Dashboards › Servicedesk – Wochenüberblick', [
      'Filters for all widgets on top as chips of the query line (hub, period, group). A widget with a filter of its own says so in its source line.',
      'The grid stays freely configurable (12 columns, up to 24 widgets) — the picture is one of many. Every widget has the same anatomy: title, content, the query in one line, ⋯ with "Als Liste öffnen", "Als Tabelle", "Exportieren". The table is a toggle instead of always under the chart; before, every figure carried an empty table.',
      'Tablet: 6 columns; phone: one column in the grid\'s order, figures in pairs. Nobody keeps a phone layout of their own.']],
    ['dedit', 'Edit a dashboard', 'Edit · add a widget', [
      'Editing is a mode with "Rückgängig", "Verwerfen" and "Fertig"; the grid becomes visible.',
      'Drag widgets by their handle, resize at the corner; keyboard: arrows move, Shift+arrows resize. Before, ten icon buttons per widget and no dragging.',
      'On the right the widget library with previews on one\'s own data, filtered by kind; dragging or clicking inserts, a drop zone shows the size.']],
    ['wedit', 'Set up a widget', 'Widget "Erstellt und gelöst je Woche"', [
      'The widget large in the preview, live with one\'s own data while setting it up.',
      'On the right three tabs Data · Presentation · Display: what is counted (entries, links, time, pages, inventory), which (the query line, inheriting or releasing the dashboard\'s filters), how (count, sum, median … per week), a second series, target and warning lines. Fields are called by their names, never by a raw key.']],
    ['wcat', 'Every widget', 'The widget kinds and their rules', ['Nine kinds with fixed rules: figure with trend, bars, trend line, share (ring), table, matrix, goal progress, and new: list of entries and text. Under each picture stands how the kind is built.']],
    ['plan', 'Plan · schedule as a timeline', 'Neubau Logistikzentrum › Plan · tab schedule, presentation timeline', [
      'Timeline and Gantt are two presentations of the same tab "Zeitplan". Before, the module silently chose one of two builds.',
      'Timeline: light, without columns; dragging moves, dragging the end changes the duration, "ohne Termin" as a tray. Dependencies are shown and marked when broken (UC-PLN-01/6).',
      'Phases as summary bars, milestones as a diamond with date, a today line.', '',
      'The plan\'s tabs stay: Zeitplan · Tabelle · Abhängigkeiten · Releases · Sprints. Sprints appears only where a collection of the hub runs sprints — not here (navigation concept §2.4; today the tab is always there).',
      '"Ansicht" opens the view panel; its count says how much differs from the default. Zoom, earlier/today/later and "Ganzer Plan" stay in the bar.']],
    ['gantt', 'Plan · schedule as Gantt', 'Presentation Gantt · critical path and baseline', [
      'The head says end, critical path, slip against the baseline, tolerance and "ohne Termin" in one sentence. "Als Baseline festhalten" and ⋯ (plan settings, CSV, XLSX, PNG, PDF, take over a project file) stay where they are.',
      'Critical path, baseline and proposals are switches; the legend stands beside them in words.',
      'On the left the outline with start, end, duration, person (columns in the view panel), on the right the bars: dependencies as lines, critical bars hatched and named, a grey stroke = baseline, a line = slack.',
      'Tablet: the same view narrower. Phone: a list with colour bars and "Balken" as a toggle; editing from tablet up.']],
    ['planview', 'Plan · the view panel', 'Tab schedule · view open', [
      '', '', '', '', '', '',
      'One panel for everything about the presentation: presentation, what is seen (Hubtask query, done), rows (group, sort), bars (colour by, dependencies, critical path, slack, progress, milestones, weekends), comparison with a baseline, columns on the left. Saved as a view with name, place and visibility — as in every collection. Today there are only zoom, baseline choice and the plan settings; "neu" marks what is added. Tolerance and managing baselines stay under ⋯ › plan settings.']],
    ['plantable', 'Plan · table', 'Tab table', [
      'The same head and tabs on every page of the plan.',
      'Filter line, baseline, columns, export and view above the table.',
      'The plan\'s proposals as a band with "Alle annehmen" and "Einzeln prüfen"; the proposed start stands in the column (old → new). Today they are ticks "Verschiebung annehmen" in a column of their own.',
      'Columns as today: entry (indented by outline), start, end, working days, slack, slip (with baseline), scheduling "Von Hand" or "Automatisch" as a chip, milestone as a diamond; critical and done as lozenges instead of tick columns.']],
    ['plandeps', 'Plan · dependencies', 'Tab dependencies', [
      'The same head; "Abhängigkeit hinzufügen" as the primary button.',
      'What the plan cannot hold stands on top with an act.',
      'The chip opens "Zeitbezug": four kinds, lag in working days, remove.',
      'A table "Wartet ← auf" with the time relation as a chip (kind and lag).']],
    ['planrel', 'Plan · releases', 'Produktentwicklung › Plan · tab releases', [
      'In this hub a collection runs sprints, so the sprints tab exists. The head sentence: planned releases, the next one, the running sprint.',
      'The releases on an axis with today — published, the chosen one, the next.',
      'The chosen release: state, date, notes, "Veröffentlichen" as the primary button; once published the notes become a page (as today).',
      '"Einträge hinzufügen" from every collection of the hub, by selection or query; the table shows key, type, collection and state.']],
    ['plansprints', 'Plan · sprints', 'Produktentwicklung › Plan · tab sprints', [
      'The sprints tab, because a collection here runs sprints; it gathers the sprints of every collection of the hub. The sprint itself lives in its collection.',
      'The running sprint of each collection as a card: goal, days left, points done, velocity and the burndown as a picture. Today a line of text with a link.',
      'Next: planned sprints with planned points against the velocity.',
      'Collections without sprints with "Sprints einschalten".',
      'Closed sprints: committed and done as figures and as paired bars; the velocity above.']],
    ['sprint', 'A collection\'s sprint', 'Team Atlas – Mobile App · tab "Sprint 24 – Passkeys"', [
      'The collection\'s head and tabs as today: board, the running sprint by name, backlog, list, timeline, time, saved views.',
      'One sprint bar: chooser with state, goal, days left, points done, the burndown as a small picture (a click: large with a table), "Sprint schließen", ⋯ (change, delete). Today burndown and velocity sit folded under the board.',
      'Columns are the workflow\'s statuses with count and points per column; cards as on the board with the estimate as a chip, a missing one as "schätzen". The backlog is no longer a column in the sprint but a tab of its own.',
      'Phone: the sprint as a card with burndown, the columns as strips.']],
    ['backlog', 'Backlog and sprint planning', 'Team Atlas – Mobile App · tab backlog', [
      'Head and tabs as in the sprint.',
      'The running sprint folded on top.',
      'The planned sprint with its dates, planned points against the velocity of the last three sprints, "Sprint starten"; a card is being dragged in.',
      'The backlog as a ranking: dragging orders and plans in, the estimate as a chip, a line to type at the foot. "Sprint planen" creates a new sprint (name, goal, first and last day, counts points or hours — as today).']],
    ['viewcfg', 'Set up a view', 'Team Atlas – Mobile App · list · view open', [
      'The head as today; the "Liste" tab shows with a dot that the view is changed and not yet saved.',
      '"Ansicht · 3 geändert" replaces the four menus filter, sort, fields and views.',
      'One panel, the same in every collection and in the plan: layout (list, outline, board, timeline; calendar new, from the navigation concept), what is seen (Hubtask query), order (group, sort, done), fields to drag and tick — custom fields such as story points too —, for the board columns by, lanes by (new) and limits. Saving at the foot: name, where it lives, who sees it — today\'s choice.',
      'Phone: the same sections as a sheet.']],
    ['homeplans', 'Product home: plans', '/plans · for you · and empty', [
      'A head band with sprint, releases and plans with slip in one sentence.',
      'The running sprint with burndown, the next releases with progress. Before, a list of all 14 hubs, even without a plan.',
      'Only hubs with a plan, each with a strip of its phases and what counts ("3 Tage Verzug").',
      'Empty: its own sentence, "Ersten Plan anlegen", import from a project file, three templates. Before, every product in the empty workspace said the same "… ist ausgeschaltet".']],
    ['homeassets', 'Product home: inventory', '/assets · for you · and empty', [
      'A head band with items, attention and the state of the attestation round.',
      '"Braucht Aufmerksamkeit" across all hubs: no owner, review due, open incident — with place.',
      '', 'Empty: "Mit Grundausstattung beginnen", import from a table, connect a source.']],
    ['homeagents', 'Product home: agents', '/agents · agents · and empty', [
      'A head band: approved, draft, and what waits for me.',
      '"Wartet auf dich" first, with decline and approve in the row. Before, it lay in the second tab.',
      'Agents as cards: for whom they act, model, use of the daily budget, last run.',
      'Empty: what an agent is and may do, three templates; without an AI provider it says who enters one.']],
    ['emptygoals', 'Empty: goals', 'Empty workspace · goals', ['Its own sentence, "Erstes Ziel setzen", a dashboard as the second way, templates for half-year goals, a service-desk dashboard and a portfolio. The filled home is the "Goals" scene above.']],
    ['emptytime', 'Empty: time', 'Empty workspace · time', ['Its own sentence, "Erste Zeit erfassen", set up activities, three ways to record. The filled week is the "Time" scene above.']],
    ['empty', 'Empty state', 'Empty workspace · service desk', [
      'A large product tile, one sentence on what the product does, a template as the main way, "Leer beginnen" as the side way.',
      'Templates as cards below.']]
  ];

  const SP = [
    ['pread', 'Read a page', 'Page "Architekturüberblick Kunden-App" in hub Produktentwicklung', [
      'On top only the path, on the right the faces of co-readers, comments, share, star and ⋯. The title stands once: in the text, large, with the page\'s icon above it. The 16 word buttons are gone (see "Write a page").',
      'The trust line right under the title: Mehmet answers for the page, "Geprüft bis 15. Jan. 2027" as a green lozenge (turning yellow "Prüfung fällig"), who changed it last, the labels. Before, the review lay under ⋯ → Details.',
      '"Auf dieser Seite" comes from the headings and follows on the right. On the tablet it is a button above the text.',
      'Live blocks: an embedded view and a board frame, each with product tile, green "Live" and "In Aufgaben öffnen" or "Board öffnen". Mentioned entries show their status, linked checklist lines their key.',
      'All comments stand on the right in one column: on top "Zur ganzen Seite", below the comments on the text at the height of their passage (marked yellow), at the foot "Zur ganzen Seite kommentieren". Before, whole-page comments stood under the text. At the text\'s end only "Verweist hierher" stays.',
      'Phone: reading first. At the foot "Inhalt", comments and "Bearbeiten"; the text is 16 px, tables scroll inside. Before, a screen of buttons came before the first word.']],
    ['pcover', 'A page\'s cover', 'The same page with a cover · picker open', [
      'A cover over the width of the content, 200 px high (phone 120 px). "Titelbild ändern", "Verschieben" (the chosen crop holds at every width) and remove appear on hover. Before, there was no cover and the web had no field for the icon.',
      'Icon and title stand under the picture, never on it — so contrast holds in every picture and in dark mode.',
      'A picker in four tabs: colours (gradients from the tokens), gallery (shipped, no network), upload (from 1500 px), image provider (only when the administration switches it on; fetched through the server, the photographer named at the picture).']],
    ['pwrite', 'Write a page', 'The same page while writing, Amelia writing along', [
      'Selecting text shows the bar above it: paragraph kind, bold, italic, strike, code, link, comment. The same bar as on the board.',
      '"/" opens insert at the cursor, filtered as one types: first the Hubtask blocks (view, whiteboard, entry), then callout, toggle, image. The fields for formula, status and address appear only after the choice, in the block itself. Before, three fields folded out over the text.',
      'At each line\'s margin on hover "+" and the handle ⋮⋮: drag to move, click for "Umwandeln in …", copy, delete.',
      'On a checklist line "Aufgabe daraus" appears (UC-DOC-03). On top "Gespeichert · Amelia schreibt mit"; her cursor carries her name.',
      'Phone: "Bearbeiten" opens a bar right above the keyboard with Aa, bold, italic, list, checklist, @, / and image; "Fertig" on top closes it.']],
    ['phist', 'A page\'s history', 'History · two versions side by side', [
      'Versions on the left, one per writing session, with person and time; named versions carry their name tag. A sentence explains: bringing back creates a new version, nothing is lost.',
      '"Nebeneinander | Nur Änderungen" and a legend with the number of changed passages.',
      'Two versions side by side (UC-DOC-06/3), removed red and struck through, new green. Bring back and name as buttons on top, not behind a pencil.']],
    ['phome', 'Pages: product home', '/pages · for you', [
      'The head band as in every product; the figures line says how many pages are verified. "Notiz von heute" (UC-DOC-07) as a button of its own beside "Neue Seite".',
      '',
      '"Weiterlesen" as cards with icon, the start of the text, place, person and review state.',
      '"Dir zur Prüfung fällig": the pages I answer for, with "Prüfen" right in the row (UC-DOC-12/4).',
      'Per space, how much is verified. "Bereiche" counts pages instead of open tasks.']],
    ['pnew', 'A new page', 'New page under "Handbuch Produktentwicklung" in a space with drafts', [
      'When the space works with drafts (UC-DOC-15), a band on top says who sees the page, with "Veröffentlichen" and "Verwerfen".',
      'The page is there at once, the cursor in the title. Templates as chips under it, space templates as links. Before, a dialog with nine cards came before the first word.',
      'The empty text explains "/" and "@".',
      'Hovering a template shows its preview with its headings on the right and "Diese Vorlage verwenden".']]
  ];

  const SW = [
    ['wboard', 'The board', 'Whiteboard "Retro Sprint 23"', [
      'Top left one island: back, product tile, title, state as a tick instead of a sentence, star, "Fläche | Liste" (the same board as a list for keyboard and screen reader) and ⋯.',
      'Top right the faces of who is here (a click means follow), "Moderieren" for timer and vote, comments with count, and history.',
      'The dock at the bottom centre in five groups: move, create, draw, conversation, insert. Every key shows its shortcut. Before, 14 equal icons on top that jumped when a panel opened.',
      'Bottom right frames, zoom, fit and the map. The map is small and open only on request; before, it covered the board.',
      'Live cursors with names; comment pins at the object; task cards with key, person, tick and status.',
      'Tablet: the same dock; the pen draws, the finger selects and pans; a hint the first time.',
      'Phone: opens the first frame fitted instead of at 25 %. On top "Lief gut · 1 von 3" to page through the frames.',
      'Phone: a dock with select, note, text and comment, the rest under ⋯. Zoom with two fingers; no second bar.']],
    ['wsel', 'Select and turn it into work', 'A note selected · task from it', [
      'Above the note the bar: ten colours, font size, arrow, comment, lock, "Aufgabe daraus" and ⋯. Before, a form with number fields and selects.',
      '"Aufgabe daraus" asks only what it needs: collection and type. The note\'s text becomes the title, the note becomes a card, its arrows stay (UC-CNV-02/1, /6).',
      'Handles at the edge: dragging one connects, dragging a corner resizes. "+" on the right and below adds a neighbouring note, Tab while writing the next.',
      'The note "Review eine Stunde früher legen" already is a task: TAMA-21, Jana, "Zu erledigen", tickable on the board.',
      'With several objects selected: align, distribute, group, copy and "3 Aufgaben" at once (UC-CNV-02/7).']],
    ['wsess', 'Moderate a session', 'Retro with timer, anonymous vote and follow', [
      'The session bar at the top centre, the same for all: timer with pause and "+1 Min.", the vote with my remaining votes, how many have voted, and "Beenden und zeigen" for the moderator.',
      '"Gerade hier" with faces and "Folgen". Before, a sentence under two forms.',
      'Votes as a figure at the object once the vote is shown.',
      'A follower sees a coloured frame around the board and "Du folgst Amelia · Board bewegen beendet das" (UC-CNV-05/3).']],
    ['whome', 'Whiteboards: product home', '/whiteboards · for you', [
      'The head band as in every product, with open comments in the figures line.',
      'Thumbnails drawn from the board\'s objects instead of grey placeholders, with place, number of objects, last change and who worked on it.',
      'The templates as a row with previews; all three retrospectives separately.',
      '"Offene Kommentare an dich" with their place on the board; a click jumps there (UC-CNV-09).']],
    ['wnew', 'A new board', '"Neues Whiteboard" from the product home', [
      'The board opens at once, in the hub one chooses; the templates come as a sheet over the empty surface, all eight with previews. "Leer lassen" closes the sheet. Before, the page jumped to a hub\'s "Alle Inhalte" and opened a dialog there.',
      'On the empty surface a hint that vanishes with the first object: N for a sticky note, a double click places a note there, the held space bar pans.']]
  ];

  const SQ = [
    ['palette', 'Search in the header', 'Header · "passkey" typed · tablet: before typing', [
      'Scopes as chips with counts above all hits; "Hier: Produktentwicklung" narrows to the hub. A chip is a chip of the query (type: page).',
      'Tablet: before typing, recently opened, saved queries and quick commands.',
      'Phone: search as full screen; scopes as a strip.',
      'Phone: filters as a sheet, "Alle 23 zeigen".']],
    ['search', 'Search page', '/search', [
      'One line: words and chips together, suggestions while typing. The switch "Als Text | Mit Bedienelementen" is gone.',
      'Where AI is on: a question in words becomes a visible query.',
      'Facets with counts: kind, hub, changed, by.',
      'The hit count across all products, sorting, "Als Ansicht speichern".',
      'One hit row for every kind: glyph or key, title with the match, excerpt, place, status. Files and comments too.']],
    ['qline', 'The query line in a queue', 'IT-Service-Desk › Kurz vor Verletzung · filter', [
      '',
      'Chips for field and value; groups with "or" stay as a text chip in the same line. Typing "prio" suggests fields with kind and example, then values with counts.',
      '"Bedingung" and "Gruppe (oder)" for everyone who does not want to type; a click on a chip changes it.',
      'The same query as text — what is saved, shared and understood by the API, a widget and an automation.',
      'Phone: chips as a strip, filters as a sheet, "4 Einträge zeigen".']]
  ];
  const SC = [
    ['create', 'Create', 'Header › Erstellen in hub Produktentwicklung', [
      'On top the line that already makes an entry from a sentence, with the place "in Team Atlas ▾". Below: last created, what is possible here, "Anfrage an eine Abteilung". Before, a dialog for entries only, with four tabs.',
      'Tablet: typing starts at once; Enter creates the entry, as today with Q.',
      'Phone: the ＋ stays in its place in the bottom bar and opens a sheet with eight kinds.']]
  ];
  const SF = [
    ['viewer', 'File viewer', 'Attachment "Lastenheft Wiederherstellung.docx" on TAMA-1', [
      'The file\'s page, shown by the document server or as a converted PDF; nothing is downloaded. Before, a menu with the type "application/zip" and only download.',
      '"Bearbeiten im Dokumenten-Server" only when one is entered and one may write.',
      'Details: file, who uploaded it, what it hangs on, where it is mentioned.',
      'All files of the entry at the foot; ← → page, Esc closes.',
      'Phone: full screen, a swipe pages.']],
    ['wbfiles', 'Files on the whiteboard', 'Whiteboard "Anmeldefluss mit Passkey"', [
      'Images lie on the surface as images.',
      'Other files become a card with their first page; "Ansehen" opens the viewer.',
      'On dropping, a hint explains once what happens with what.']]
  ];
  const SPO = [
    ['portaldir', 'Requests & help for people inside', 'Erstellen › Anfrage an eine Abteilung', [
      'One search over every request type and the shared knowledge of every department.',
      'Every department one may ask. Before, another department\'s portal was reached only by its link.',
      'My requests across all departments, with a readable state.',
      'How one gets there and back: the sidebar stays.']],
    ['portalext', 'Portal for people outside', 'Lena Bauer, Stadtwerke Nord · portal only · tablet: a shared knowledge space', [
      'A frame of its own without sidebar, Today, search over Hubtask and Create; on top start, my requests, approvals, knowledge. Its search covers only what is shared.',
      'Request types as tiles with one sentence.',
      'Knowledge as spaces, not only single articles.',
      'Approvals for outsiders on top, with decline and approve. States in words instead of "in_progress".',
      'Tablet: a shared space with its tree; "Anfrage stellen" at the end of every article.',
      'Phone: the same parts one under the other, at the foot four portal tabs.']]
  ];
  const SB = [
    ['posts', 'A hub\'s posts', 'Produktentwicklung › Beiträge', [
      'Unread, by me, scheduled as a switch; labels as a filter.',
      'A featured post on top, with cover and "Lesebestätigung erbeten".',
      'By month, newest first; each card with cover, opening, person, reading time, discussion.',
      'Scheduled posts are seen only by writers, dashed, with their date.']],
    ['post', 'Read a post', 'Post "Neues Ticketsystem ab 1. Dezember"', [
      'Kind and origin, title, person, date, reading time, "gesehen von 38".',
      'The requested read confirmation as a band with deadline and one button.',
      'The discussion under the post: write, reply, a few fixed reactions.',
      'Comments at the text on the right as on pages; writers see "bestätigt 21 von 45" and "Erinnern".',
      'Phone: reading first, confirming in the text.']],
    ['postwrite', 'Write and schedule a post', 'Draft "Release 4.2 ist da"', [
      '"Veröffentlichen" opens: now or schedule.',
      'Templates as chips as on pages, the post templates first, then every page template.',
      'Schedule with date and time, who sees it, notify, read confirmation, feature, discussion on or off.']]
  ];
  const SD = [
    ['task', 'A normal entry', 'Story TAMA-18 · filled', [
      'The head spans the full width: key, type, sprint or backlog, target version, title; the action bar with status, next step ("Prüfung anfragen"), person, date, priority, points. Only below it the page splits into main column and rail.',
      'Jump marks with counts, staying on top while scrolling.',
      'The description formatted (headings, lists, a mention with status) instead of Markdown as text; from 12 lines "Mehr zeigen".',
      '"Teile" as a table: work packages with progress, activities with their type (call, meeting, review) in colour and glyph; a line to type at the foot.',
      'Links grouped by kind, each with key and status instead of blue underlining.',
      'All 9 files as a grid with previews, switchable to a list; before, only "9 Anhänge" in the details.',
      'Conversation with the tabs Alles · Kommentare · Verlauf — "Alles" first, with status changes and done parts as quiet rows between the comments. The same build as the ticket. The pages\' editor: format, @ mention, / insert, files.',
      'On the right only properties, in the form\'s groups (acceptance, planning, rating), pinned ones on top, formatted ("Ja" instead of "true", "18.500 €"); time with recorded hours.',
      'Phone: properties in a sheet, tabs instead of jump marks, commenting docked at the foot.']],
    ['deskentry', 'A ticket in the service desk', 'Ticket SA-12 · one SLA, two OLAs, filled', [
      'The head spans the full width as on every entry; the next step "Lösen" is the primary button.',
      'One chip in the head for the deadlines: what is breached and the next one.',
      'The deadlines now stand in the rail, on top: one row per deadline (goal · for whom · SLA or OLA), by urgency, with bar and time. Met first replies in one row. Each further OLA is one row more; "Alle Fristen als Tabelle" opens the matrix with business hours and pauses.',
      'Jump marks with counts; the main column starts right with the content.',
      'Parts with activities as in the story.',
      'Affected items as a table with class, state and reached services, below them the other open entries on them.',
      'The reply with tabs "Antwort an Hannah Albers | Interne Notiz", the editor, snippet and macro as icons; the button says "Antwort senden", beside it the status after. Internal notes yellow with "Intern".',
      'On the right under deadlines and triage: details, the requester with organisation and further requests, approvals, time.',
      'The macro menu: each macro with what it does. A macro is an automation with the trigger "Von Hand".',
      'Phone: the deadlines as a list by urgency, replies at the foot with "An Hannah | Intern".',
      '"Einordnung" in the rail: impact, urgency, priority (computed from both) and group, each with a sentence under it. Before, a line under the deadlines.',
      'Conversation with Alles · Antworten · Interne Notizen · Verlauf. "Alles" mixes replies, notes and the history (received, deadline met, group changed). Before, "Alles" and the history were missing on the ticket.']],
    ['wf', 'Workflow: where it goes next', 'Story TAMA-18 · workflow "Story" · and without a workflow', [
      'The status is a button. It opens "Von In Arbeit weiter": every allowed step with its target status and what it asks or needs. Today the steps stand as single buttons in the bar, without target status.',
      'What you may not do stands greyed with its reason ("nur Product Owner · Clara Neumann") instead of being absent. Below, how "Erledigt" is reached and who is needed. "Workflow ansehen" opens the picture.',
      'View workflow (width "View workflow"): the designer\'s picture, read only — one column per category, your status outlined, the steps you can take now in the brand colour, blocked ones dashed. Beside it each step with who and what. "Im Designer öffnen" only with the right to the structure.',
      'Phone: the same list as a sheet from below.',
      'Without a workflow (width "Without workflow"): the default holds — Zu erledigen, In Arbeit, Erledigt, every status reachable. The primary button is "✓ Erledigt" and always completes the entry. Both hold for the ticket alike.']]
  ];
  const SI = [
    ['invlist', 'Inventory: stock', 'IT-Service-Desk › Inventar', [
      'Tabs Bestand · Karte · Zustand · Alarme · Pflege; before, seven sub-pages under "Verwalten ▾".',
      'A query line with asset. fields, list or "Nach Klasse", columns to choose.',
      'The class tree with counts, saved groups under it.',
      'A table with class, stage, owner, location, protection and open entries; problems with a glyph.']],
    ['ci', 'An item', 'Inventar › Portal-API', [
      'A head like every entry over the full width; the stage as a step bar from planned to disposed. Main column and rail start below it.',
      'Jump marks: attributes, relations, impact, work on it, register, files, history.',
      'Relations as a picture with the neighbours\' status and the incident\'s path, the list as a toggle.',
      'Impact in two directions, up to eight steps.',
      '"Arbeit daran": SA-12 and SA-8 name the item, CHA-1 acts beneath it. Before, the ticket was missing at the item.',
      'On the right the owner (a missing one as a hint), protection need own or inherited, origin, groups.']],
    ['invmap', 'Inventory: a service\'s map', 'Map · service Kundenportal', [
      'Choose a service, depth 1–3, "Nur Probleme".',
      'The map shows open incidents red with their path dashed, planned changes with their key.',
      'Below, what is open and planned on this service.']],
    ['invcare', 'Inventory: upkeep', 'Upkeep · health, tasks, round, sources', [
      'Six key figures of health.',
      '"Zu tun" with one act per row.',
      'The running attestation round with progress and "Erinnern".',
      'Sources with last and next run, "Quelle verbinden" and "Importieren".']],
    ['invreg', 'Inventory: devices and licences', 'Register · devices and licences · NB-0422', [
      'A table with number, device, person, state, cost, warranty; the value in use on top.',
      'On the right the life from requested to disposed (with proof), the register data and the next steps.']]
  ];

  const DECISIONS = [
    ['E-1', 'Do the product colours reach inside the products (head band, key, empty states, widgets)?', 'Yes. Never as status, button or chart colour.'],
    ['E-2', 'Do only properties stand on the right, in the form\'s groups, while parts, links, affected items and files stay in the main column?', 'Yes (the entry chapter).'],
    ['E-3', 'Do an entry\'s description and comments get the pages\' editor (formatting, @ mentions, files)?', 'Yes. Changes UC-WRK-02 and UC-WRK-19. Still stored as Markdown.'],
    ['E-4', 'Does a product home\'s "For you" show the product\'s latest changes when nothing personal is there?', 'Yes. Fixes the empty product homes.'],
    ['E-5', 'Comfortable/compact density for lists, remembered on the device?', 'Yes. Like theme and motion, a matter of the device.'],
    ['E-6', 'Does the hub overview become a tile grid of its tools with figures?', 'Yes.'],
    ['E-7', 'Chart palette: seven fixed colours from the existing families, in a fixed order?', 'Yes. New tokens, no new colours.'],
    ['E-8', 'Do verified pages rank before equally good unverified ones in search, and do pages due for review carry their lozenge there?', 'Yes. Changes the order, not what is found (UC-DOC-01/3 stays).'],
    ['E-9', 'May every reader "request a review" from the page\'s owner (a notification)?', 'Later. New behaviour beyond UC-DOC-12; belongs in a use case of its own.'],
    ['E-10', 'Is the language called "Hubtask query", with English keywords and localised names in the suggestions?', 'Yes. One language for search, views, queues, widgets, inventory groups, goals and the API.'],
    ['E-11', 'Does search also find people, file names and comments?', 'Yes. Extends UC-WRK-18. It still does not search inside files.'],
    ['E-12', 'Does an automation choose which entries it applies to with the query line?', 'Yes. Formulas and conditions over values stay CEL, as today.'],
    ['E-13', 'Do outsiders without READ get only the portal frame, and can a whole knowledge space be shared with a portal?', 'Yes. Changes UC-SVC-05 and UC-DOC-13.'],
    ['E-14', 'Does the file viewer get pdf.js (a new dependency, Apache-2.0) and Hubtask a WOPI host for Collabora and ONLYOFFICE/Euro-Office, optionally Microsoft 365?', 'Yes. Editing happens only in the document server.'],
    ['E-15', 'Do posts come into Knowledge as a new kind, with a section at the foot of the hub column, scheduling, audience, read confirmation and a few reactions in the discussion?', 'Yes. Needs new use cases. The only addition to the navigation.'],
    ['E-16', 'Does "Zeitplan" become one view with the presentations timeline and Gantt, retiring the old TimelineView?', 'Yes. Meets UC-PLN-01…09 with one build. The Gantt presentation needs the planning module.'],
    ['E-17', 'May people hide and order Today\'s cards and tiles?', 'Yes. Changes "Where it ends" of UC-WRK-34. Today does not become a free dashboard.'],
    ['E-18', 'Do pages and posts get covers, with an image provider only when the administration switches one on?', 'Yes. A new field on the page, additive.'],
    ['E-19', 'QR labels and handing out and taking back in the inventory?', 'Later. Not there today; belongs in a use case of its own.'],
    ['E-20', 'Does the plan get a view panel with group, sort, colour by, switches for dependencies, critical path, slack and progress, and chosen columns — saved as a view as in every collection?', 'Yes. The tabs schedule, table, dependencies, releases and sprints stay.'],
    ['E-21', 'Does one "View" panel in collections replace the menus filter, sort, fields and views, with "lanes by" on the board and the calendar layout?', 'Yes. Lanes and calendar are new; everything else exists and is only brought together.'],
    ['E-22', 'Does the backlog stop being a column of the sprint tab, and do sprint and backlog show the velocity as a planning aid?', 'Yes. Velocity exists; a per-person sprint capacity does not and is not added.'],
    ['E-23', 'Does an entry\'s status menu show allowed and blocked steps with a reason, and may everyone with READ see the workflow\'s picture (read only)?', 'Yes. The check stays on the server (WorkflowGate); new is only that blocked steps are visible with their reason.']
  ];

  const ALL = SCENES.concat(SP, SW, SQ, SC, SF, SPO, SB, SD, SI);
  const dev = {};
  function el(html) { const t = document.createElement('template'); t.innerHTML = html.trim(); return t.content.firstElementChild; }
  function fit(scaler) {
    const stage = scaler.firstElementChild; const nat = +scaler.dataset.w; const s = Math.min(1, scaler.clientWidth / nat);
    stage.style.width = nat + 'px'; stage.style.transform = `scale(${s})`; scaler.style.height = Math.ceil(stage.offsetHeight * s) + 'px';
  }
  const ro = new ResizeObserver((es) => es.forEach((e) => fit(e.target)));
  const cache = {};
  function after(id, d) { cache[id] = cache[id] || S[id](); return cache[id][d]; }
  function render(box, id) {
    const devs = Object.keys(DEV).filter((d) => after(id, d));
    const d = dev[id] || devs[0];
    box.querySelector('.devs').innerHTML = devs.map((x) => `<button type="button" aria-pressed="${x === d}" data-d="${x}">${DEV[x]}</button>`).join('');
    const pair = box.querySelector('.pair'); pair.innerHTML = '';
    pair.className = 'pair' + (d === 'm' ? ' phone' : '');
    const scaler = el(`<div class="shot scaler" data-w="${W[d]}"><div class="stage" data-theme="${document.getElementById('dark').checked ? 'dark' : 'light'}"></div></div>`);
    scaler.firstElementChild.innerHTML = after(id, d);
    scaler.style.maxInlineSize = W[d] + 'px'; scaler.style.marginInline = 'auto'; scaler.style.inlineSize = '100%';
    if (!document.getElementById('pins').checked) scaler.querySelectorAll('.mk').forEach((x) => x.classList.add('nopins'));
    pair.append(scaler); ro.observe(scaler); requestAnimationFrame(() => fit(scaler)); setTimeout(() => fit(scaler), 400);
  }
  [[SCENES, 'scenes'], [SP, 'scenes-pages'], [SW, 'scenes-wb'], [SB, 'scenes-posts'], [SD, 'scenes-entry'], [SI, 'scenes-inv'], [SQ, 'scenes-query'], [SC, 'scenes-create'], [SF, 'scenes-files'], [SPO, 'scenes-portal']].forEach(([LIST, rid]) => {
    const root = document.getElementById(rid);
    LIST.forEach(([id, title, where, changes]) => {
      const list = changes.map((c, k) => (c ? `<li><b class="n">${k + 1}</b><span>${c}</span></li>` : '')).join('');
      const box = el(`<article class="scene" id="s-${id}"><div class="scene-head"><div><h3>${title}</h3><div class="where">${where}</div><div class="sid">scene ${id}</div></div><ol class="changes">${list}</ol></div>
        <div class="controls"><div class="seg2 devs" role="group" aria-label="Width"></div></div><div class="pair"></div></article>`);
      box.addEventListener('click', (e) => { const b = e.target.closest('button'); if (!b || !b.dataset.d) return; dev[id] = b.dataset.d; render(box, id); });
      root.append(box); render(box, id);
    });
  });
  const rerender = () => ALL.forEach(([id]) => render(document.getElementById('s-' + id), id));
  document.getElementById('pins').addEventListener('change', rerender);
  document.getElementById('dark').addEventListener('change', rerender);
  Object.keys(RULES).forEach((rid) => {
    document.getElementById(rid).innerHTML = RULES[rid].map(([n, h, p]) => `<div class="card"><h4><b class="id">${n}</b>${h}</h4><p>${p}</p></div>`).join('');
  });
  document.getElementById('decisions-body').innerHTML = DECISIONS.map(([n, q, a]) => `<tr><td>${n}</td><td>${q}</td><td>${a}</td></tr>`).join('');
  document.getElementById('swatches').innerHTML = ['plans', 'pages', 'whiteboards', 'desk', 'assets', 'time', 'goals', 'agents'].map((p) => `<div class="sw" data-p="${PROD[p].p}">${window.WB.ic(PROD[p].i, 'l')}<span>${PROD[p].n}<small>label.${PROD[p].p}</small></span></div>`).join('');
})();
