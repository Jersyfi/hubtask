// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The work surface's coherence across products (work-surface concept §4, §7): one control, one
// look and one behaviour wherever it stands. Read from the source, so every screen is held whether
// a walk opens it or not.

import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const srcDir = fileURLToPath(new URL('../', import.meta.url));

function svelteFiles(dir = srcDir): string[] {
  const found: string[] = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) found.push(...svelteFiles(path));
    else if (name.endsWith('.svelte')) found.push(path);
  }
  return found;
}

const sources = svelteFiles().map((path) => ({ path: path.slice(srcDir.length), text: readFileSync(path, 'utf8') }));

/** Every component tag of this name in the source, from `<Name` to its closing `>`, braces counted. */
function tagsOf(text: string, name: string): string[] {
  const tags: string[] = [];
  const pattern = new RegExp(`<${name}\\b`, 'gu');
  for (const match of text.matchAll(pattern)) {
    let depth = 0;
    for (let i = match.index; i < text.length; i += 1) {
      const ch = text[i];
      if (ch === '{') depth += 1;
      else if (ch === '}') depth -= 1;
      else if (ch === '>' && depth === 0) { tags.push(text.slice(match.index, i + 1)); break; }
    }
  }
  return tags;
}

test('a pressed toggle never turns primary: the pressed look is the design system’s (WB-2)', () => {
  const offenders: string[] = [];
  for (const { path, text } of sources) {
    for (const tag of [...tagsOf(text, 'Button'), ...tagsOf(text, 'IconButton')]) {
      if (/aria-pressed=/u.test(tag) && /tone=\{[^}]*'primary'/u.test(tag)) offenders.push(`${path}: ${tag.slice(0, 80)}`);
    }
  }
  assert.deepEqual(offenders, []);
});

test('no screen paints a design-system button with a tone of its own', () => {
  const offenders = sources.filter(({ text }) => /:global\(\[data-bar='task'\]\[data-tone=/u.test(text)).map(({ path }) => path);
  assert.deepEqual(offenders, []);
});

test('the application names the sheet a menu becomes on a phone, and only the navigation keeps its menus anchored', () => {
  const app = sources.find(({ path }) => path === 'App.svelte');
  assert.match(app?.text ?? '', /setCloseLabel\(t\('app\.sheet_close'\)\)/u);
  const anchored = sources.filter(({ text }) => /<Menu isAnchored/u.test(text)).map(({ path }) => path);
  for (const path of anchored) assert.match(path, /^lib\/frame\//u, `${path} keeps a menu at its trigger outside the navigation`);
});

/** The dialogs in a file: their opening tag and their body up to the closing tag. */
function dialogsOf(text: string): { tag: string; body: string }[] {
  return tagsOf(text, 'Dialog').map((tag) => {
    const start = text.indexOf(tag) + tag.length;
    return { tag, body: text.slice(start, text.indexOf('</Dialog>', start)) };
  });
}

test('the products’ dialogs hand their one primary to the dialog and keep no submit in their body (D-2)', () => {
  const files = [
    'views/GoalsView.svelte', 'views/TimeView.svelte', 'views/DashboardView.svelte', 'views/PagesView.svelte',
    'lib/entries/DuplicateDialog.svelte',
  ];
  for (const file of files) {
    const text = sources.find(({ path }) => path === file)?.text ?? '';
    for (const { tag, body } of dialogsOf(text)) {
      const inBody = body.split('{#snippet actions()}')[0] ?? '';
      const submits = tagsOf(inBody, 'Button').filter((button) => !/size="sm"/u.test(button) && (!/tone=/u.test(button) || /tone="(primary|danger)"/u.test(button)));
      if (submits.length > 0) assert.match(tag, /primary=\{/u, `${file}: ${tag.slice(0, 60)} keeps its submit in the body`);
      assert.doesNotMatch(body, /<Button tone="subtle"[^>]*>\{t\('app\.(pages|links|workspace)\.cancel'\)\}/u, `${file}: a cancel is subtle`);
    }
  }
});

test('a read-only dialog has no primary: the workflow’s picture closes by its ✕', () => {
  const text = sources.find(({ path }) => path === 'lib/entries/EntryStatus.svelte')?.text ?? '';
  const picture = dialogsOf(text).find(({ tag }) => tag.includes('app.workflows.picture'));
  assert.ok(picture);
  assert.doesNotMatch(picture.body, /tone="primary"/u);
});

test('the date, person and labels pickers open from the ⋯ as the rail opens them: a popover, a sheet on a phone (D-3)', () => {
  const text = sources.find(({ path }) => path === 'lib/entries/EntryActions.svelte')?.text ?? '';
  assert.match(text, /<Popover label=\{title\} bind:isOpen=\{isPickerOpen\} at=\{door\}/u);
  assert.doesNotMatch(text, /<Dialog size="m" isOpen=\{true\} title=\{t\('app\.(due\.title|people\.assignee|entry_menu\.labels)'\)/u);
  assert.match(text, /title=\{t\('app\.entry_menu\.move_title'/u);
});

test('the board’s start is the ladder’s dialog, not a panel of its own (D-4)', () => {
  const text = sources.find(({ path }) => path === 'lib/whiteboards/BoardCanvas.svelte')?.text ?? '';
  assert.doesNotMatch(text, /role="dialog"/u);
  assert.match(text, /<Dialog size="l" isOpen=\{true\} title=\{t\('app\.whiteboards\.begin_title'\)\}/u);
});

test('a row’s and a card’s state chip follow its entry type’s own level, as the server answers it per type (L-5)', () => {
  const view = sources.find(({ path }) => path === 'views/ContainerView.svelte')?.text ?? '';
  // From the collection's own read: the replica's record carries no level (it drew Keine Spalte).
  assert.match(view, /const mode = stateModes\?\.\[entryType\];/u);
  assert.match(view, /containers\.levelOf\(wanted, \(level, byType\) => \{/u);
  assert.equal((view.match(/\{levelOfType\}/gu) ?? []).length, 2, 'the list and the board both receive it');
  const list = sources.find(({ path }) => path === 'lib/entries/EntryList.svelte')?.text ?? '';
  assert.match(list, /const mode = own\?\.mode \?\? stateMode;/u);
  const board = sources.find(({ path }) => path === 'lib/entries/Board.svelte')?.text ?? '';
  assert.match(board, /chipOf\(own\?\.mode \?\? level, card/u);
});

test('the reader’s own acts in the hub’s activity are sentences of their own, one for every verb (V-1)', () => {
  const de = JSON.parse(readFileSync(new URL('../../../../locales/de.json', import.meta.url), 'utf8')) as Record<string, string>;
  const verbs = Object.keys(de).filter((key) => key.startsWith('app.hub_overview.did_')).map((key) => key.slice('app.hub_overview.did_'.length));
  for (const verb of verbs) assert.match(de[`app.hub_overview.you_did_${verb}`] ?? '', /^Du hast /u, verb);
});

test('the products are named as the navigation names them, and the app says du (V-2, V-4)', () => {
  const de = JSON.parse(readFileSync(new URL('../../../../locales/de.json', import.meta.url), 'utf8')) as Record<string, string>;
  assert.equal(de['app.modules.name_service_desk'], de['app.nav.desk']);
  assert.equal(de['app.modules.name_planning'], de['app.nav.plans']);
  // Formal address: an imperative with "Sie" or a "Sie" before a verb of the reader. The
  // inventory's screens are their own family's (concept §6.10) and are swept there.
  const formal = /\b(Wählen|Geben|Nennen|Bitten|Prüfen|Versuchen|Kopieren|Planen|Verschieben|Halten|Legen|Fügen|Haken|Übernehmen|Erfassen|Holen) Sie\b|\b(mit|als) Ihnen\b|\bSie (haben|dürfen)\b/u;
  const offenders = Object.entries(de).filter(([key, text]) => key.startsWith('app.') && !key.startsWith('app.assets.') && !key.startsWith('app.cmdb_admin.') && formal.test(text));
  assert.deepEqual(offenders.map(([key]) => key), []);
});

test('every list keeps its column head; a field without a column goes to the meta line under the title (L-1, §7)', () => {
  const list = sources.find(({ path }) => path === 'lib/entries/EntryList.svelte')?.text ?? '';
  assert.match(list, /const columnFields = \$derived\(hasColumns \? cellFields\.slice\(0, fitCount\) : cellFields\);/u);
  assert.match(list, /<span class="meta-line">\{#each metaFields as field \(field\)\}/u);
  assert.match(list, /\.fields-head \.cell\[data-overflow\] \{ position: absolute; visibility: hidden; \}/u);
  // The type is a column under its head, not a mark beside the title outside every column.
  assert.match(list, /showsTypes && !root && !fields\.includes\('type'\) \? \['type' as const\]/u);
});

test('a list row\'s labels carry no ✕ and its type one look: a mark and a word (L-1, L-2)', () => {
  const list = sources.find(({ path }) => path === 'lib/entries/EntryList.svelte')?.text ?? '';
  const values = list.slice(list.indexOf('{#snippet fieldValue'), list.indexOf('{/snippet}', list.indexOf('{#snippet fieldValue')));
  assert.doesNotMatch(values, /onRemove=/u);
  assert.match(values, /<span class="type-word" data-entry-type=\{type\.key\}><Icon name=\{iconOf\(type\)\} size="sm" \/>\{typeName\(type\.key\)\}<\/span>/u);
  // A desk's list names its columns as its queue does (§6.2).
  assert.match(list, /requester: 'app\.desk_queues\.col_requester', people: 'app\.desk_queues\.col_assignee'/u);
});

test('every list\'s row on a phone is the stacked row: title, meta line, no grip (L-3, §7)', () => {
  const list = sources.find(({ path }) => path === 'lib/entries/EntryList.svelte')?.text ?? '';
  assert.match(list, /const isPhoneRow = \$derived\(viewport\.isCompact && !root\);/u);
  assert.match(list, /trailing=\{isPhoneRow \? phoneEnd : rowEnd\}/u);
  assert.match(list, /!isArranged && !isPhoneRow && \(!isSimple/u, 'no grip on a phone row');
});

test('a collection\'s foot add says where it adds, as every group\'s does (L-6, §8)', () => {
  const list = sources.find(({ path }) => path === 'lib/entries/EntryList.svelte')?.text ?? '';
  assert.match(list, /<AddRow label=\{root \? control\.label : footAddLabel\}/u);
  assert.match(list, /t\('app\.entries\.add_named_in', \{ name, group \}\) : t\('app\.entries\.add_in', \{ group \}\)/u);
});

test('a release has the one head every object has: its state a status button, publishing the primary (H-1, WB-1)', () => {
  const view = sources.find(({ path }) => path === 'views/ReleaseView.svelte')?.text ?? '';
  assert.match(view, /<ObjectHead isCompact hasActsOnPhone\s+objectKey=\{\{ label: t\('app\.releases\.release'\), icon: 'tag', product: 'plans' \}\}/u);
  assert.match(view, /next=\{mayChange \? \{ label: t\('app\.releases\.publish'\), icon: 'send', onclick: startPublish \} : undefined\}/u);
  assert.match(view, /<StatusButton\s+current=\{\{ id: lozenge, label: stateWord, category: 'todo' \}\}/u);
  assert.doesNotMatch(view, /class="card-head"/u);
});

test('the agents\' home opens with its product band and figures line, as every product home does (H-3, WB-5)', () => {
  const view = sources.find(({ path }) => path === 'views/AgentsView.svelte')?.text ?? '';
  assert.match(view, /<ProductBand product="agents" icon="zap" name=\{title\}/u);
  assert.match(view, /figures=\{figureParts\} \/>/u);
  assert.match(view, /<ListPage \{head\} top=\{isProductHome \? band : undefined\}/u);
});

test('every tab of the goals\' home starts with its figures, the dashboards\' and portfolios\' too (H-4, WB-5)', () => {
  const view = sources.find(({ path }) => path === 'views/GoalsView.svelte')?.text ?? '';
  assert.match(view, /current === 'portfolios' \? \(portfolioList \? \[t\('app\.goals\.figure_portfolios'/u);
  assert.match(view, /current === 'dashboards' \? \(dashboardList \? \[t\('app\.goals\.figure_dashboards'/u);
});

test('the portal\'s titles take the two sizes every head has: a page head\'s and a band\'s (H-5)', () => {
  const article = sources.find(({ path }) => path === 'views/PortalArticleView.svelte')?.text ?? '';
  assert.match(article, /\.title \{ margin: 0; font-size: var\(--fs-600\);/u);
  const home = sources.find(({ path }) => path === 'views/PortalView.svelte')?.text ?? '';
  assert.match(home, /\.greeting \{ margin: 0; font-size: var\(--fs-450\);/u);
  assert.doesNotMatch(home + article, /font-size: var\(--fs-(400|500)\); font-weight: var\(--fw-bold\)/u);
});

test('an agent\'s empty parts are one line each; a goal\'s and a portfolio\'s heads are the entry\'s (H-6, H-7)', () => {
  const agent = sources.find(({ path }) => path === 'views/AgentView.svelte')?.text ?? '';
  for (const part of ['ledger', 'jobs', 'cases', 'runs']) assert.match(agent, new RegExp(`emptyOf\\(${part}\\.state as never, 'app\\.agents\\.${part}_empty'\\)`, 'u'));
  for (const view of ['views/GoalView.svelte', 'views/PortfolioView.svelte']) {
    const text = sources.find(({ path }) => path === view)?.text ?? '';
    assert.match(text, /\.heading \{ margin: 0; font-family: var\(--font-display\); font-size: var\(--fs-200\); font-weight: var\(--fw-semibold\);/u, view);
  }
});

test('a page says nothing before its first sentence: no published hint, the live line under the text (M-2, §2.1)', () => {
  const view = sources.find(({ path }) => path === 'views/PagesView.svelte')?.text ?? '';
  assert.doesNotMatch(view, /app\.pages\.published_hint/u);
  const editor = sources.find(({ path }) => path === 'lib/pages/PageEditor.svelte')?.text ?? '';
  assert.ok(editor.indexOf('<p class="status" aria-live="polite">') > editor.indexOf('<div class="editor page-text" bind:this={host}></div>'), 'the live line stands under the text');
});

test('a chosen segment or chip never takes the primary\'s fill: one segmented look, one chip tint (T-2)', () => {
  for (const { path, text } of sources) {
    const style = text.slice(text.indexOf('<style'));
    assert.doesNotMatch(style, /\[aria-(pressed|checked)='true'\][^{]*\{[^}]*background: var\(--accent-primary\);/u, path);
  }
});

test('a new page has one primary, its Veröffentlichen: a template\'s preview offers its use as a secondary (B-1)', () => {
  const preview = sources.find(({ path }) => path === 'lib/pages/PageStartPreview.svelte')?.text ?? '';
  assert.match(preview, /<Button size="sm" tone="secondary" \{isBusy\} data-page-start-use/u);
});

test('every small control listed reaches a thumb\'s target on a phone (P-2, §7)', () => {
  const grows = (path: string, selector: string) => {
    const text = sources.find((each) => each.path === path)?.text ?? '';
    const media = text.slice(text.lastIndexOf('@media (pointer: coarse), (width < '));
    assert.ok(media.includes(`${selector}::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2))`), `${path} ${selector}`);
  };
  grows('lib/entries/StateChip.svelte', 'button.chip');
  grows('lib/planning/PointsChip.svelte', 'button.chip');
  grows('lib/workspace/PlacePeople.svelte', '.members');
  grows('views/SearchView.svelte', '.chip');
  grows('lib/entries/EntryList.svelte', '.group-head .twist');
  grows('views/DashboardView.svelte', '.corner');
  const calendar = sources.find((each) => each.path === 'lib/entries/CalendarView.svelte')?.text ?? '';
  assert.match(calendar, /\.entry \{ display: flex; align-items: center; min-block-size: var\(--layout-target\); \}/u);
});

test('a switched-off product is its start page: its glyph, its name, its sentence, the switch, cards in a row (E-1, E-2, E-3)', () => {
  const off = sources.find(({ path }) => path === 'lib/frame/ModuleOff.svelte')?.text ?? '';
  const [hero] = tagsOf(off, 'EmptyState').filter((tag) => tag.includes('isHome'));
  assert.ok(hero, 'the product form of the empty state');
  assert.match(hero, /icon=\{row\?\.icon/u, 'the product\'s own glyph');
  assert.match(hero, /title=\{name\}/u, 'the product\'s name as the title');
  assert.match(off, /<TemplateCards templates=\{cards\} shape="row"/u);
  // Time offers its three ways to record (shared with its start page while on), goals and time a
  // side way beside the switch.
  assert.match(off, /module === 'TIME' \? timeWays\(starts\.cards/u);
  const time = sources.find(({ path }) => path === 'views/TimeView.svelte')?.text ?? '';
  assert.match(time, /timeWays\(starts\.cards/u, 'time\'s start page draws other cards');
  assert.match(off, /GOALS: \{ code: 'app\.goals\.empty_side_dashboard'/u);
  const cards = sources.find(({ path }) => path === 'lib/product/TemplateCards.svelte')?.text ?? '';
  assert.match(cards, /\.cards\[data-shape='row'\] \{ grid-template-columns: repeat\(var\(--cards\), minmax\(0, 1fr\)\); \}/u);
});

test('an empty collection starts from a template first and adds an entry beside it (E-4)', () => {
  const list = sources.find(({ path }) => path === 'lib/entries/EntryList.svelte')?.text ?? '';
  assert.match(list, /\{#if isTemplateFirst\}<Button tone="primary" icon="copy" onclick=\{ontemplate\}>/u);
  assert.match(list, /<Button tone=\{isTemplateFirst \? 'secondary' : 'primary'\} icon="plus" data-opener="add-entry"/u);
});

test('a list grouped by status heads each group with the queue\'s badge and no chip per top row (L-2)', () => {
  const list = sources.find(({ path }) => path === 'lib/entries/EntryList.svelte')?.text ?? '';
  assert.match(list, /\{#if badge\}<StateLozenge label=\{name\} tone=\{badge\} \/>/u);
  assert.match(list, /const rowChipOf = \(row: Row\) => \(isByStatus && row\.depth === 0 \? undefined : chipFor\(row\.item\)\);/u);
  assert.match(list, /\.group-head \.twist \{ position: relative; z-index: 1; \}/u, 'the toggle stands above the next row (P-2)');
});

test('a link a component leaves unstyled takes the design system\'s link style, underlined in running text only (C-1)', () => {
  const css = readFileSync(join(srcDir, 'app.css'), 'utf8');
  assert.match(css, /:where\(a:any-link\) \{\s*color: var\(--text-brand\);\s*text-decoration: none;/u, 'an unstyled link keeps the browser\'s underline');
  assert.match(css, /:where\(a:any-link:hover\),\s*:where\(p, blockquote\) :where\(a:any-link\) \{\s*text-decoration: underline;/u, 'a link in a sentence lost its line');
});

test('the widget library names its preview\'s statuses, read while it is open and named once the names arrive (C-2)', () => {
  const view = sources.find(({ path }) => path === 'views/DashboardView.svelte')?.text ?? '';
  assert.match(view, /const previewRows = \$derived<FigureRow\[\]>\(figureRows\(previewCounts, \{ group: \(value\) => groupName\('status', value\)/u, 'the preview is named once, as read');
  assert.match(view, /statuses: [^\n]*\|\| isLibraryOpen \|\| \(draftWidget !== undefined && aggregateOf\(draftWidget\)\?\.group_by === 'status'\)/u, 'the statuses are not read for the library or the editor');
});

test('a split button\'s caret is named one way: more ways to its own act (V-6)', () => {
  for (const path of ['views/ProductHomeView.svelte', 'lib/servicedesk/DeskHome.svelte', 'views/HubDeskView.svelte', 'views/AgentsView.svelte', 'views/ContentsView.svelte']) {
    const text = sources.find((each) => each.path === path)?.text ?? '';
    assert.match(text, /label: t\('app\.split_more', \{ action: t\(/u, `${path} names its caret its own way`);
  }
});

test('no text is set at a size off the type scale: a page\'s headings, a side section\'s, a ring\'s share (Y-1)', () => {
  const css = readFileSync(join(srcDir, 'lib/pages/page-text.css'), 'utf8');
  assert.match(css, /\.page-text \.page-body h2 \{ font-size: var\(--fs-500\); \}/u);
  assert.match(css, /\.page-text \.page-body h3 \{ font-size: var\(--fs-400\); \}/u);
  const pages = sources.find(({ path }) => path === 'views/PagesView.svelte')?.text ?? '';
  assert.match(pages, /h3\.heading \{ margin: 0; font-size: var\(--fs-100\);/u);
  const chart = sources.find(({ path }) => path === 'lib/goals/WidgetChart.svelte')?.text ?? '';
  assert.doesNotMatch(chart, /font-size="\d/u, 'a chart sets text in its drawing\'s units');
});
