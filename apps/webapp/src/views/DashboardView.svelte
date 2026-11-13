<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One dashboard (UC-GOL-04 check 8): its widgets on the twelve-column grid, each drawn with the
  // design system's colours and a table beside its chart, computed now for this reader. A widget
  // the reader cannot run says why in place, and the others still draw (check 4).
  //
  // **Editing** happens on the grid: a widget is added, changed, moved and resized by whole cells
  // with buttons - which a keyboard and a screen reader can press - and the dashboard is saved
  // whole, held to the version read. A phone stacks the widgets in reading order.
  //
  // **No figure is stored or computed here** (check 2): the figures are read again after a save,
  // and a widget's table is handed over as a CSV file of the rows it shows.
  //
  // **Two series** (check 15): a widget may draw resolved beside created, each period as its
  // running total, and with both what is left - every number the server's.
  //
  // **By mail** (check 9): every reader may have the dashboard sent to them, daily or on one
  // weekday at an hour of their zone; the server computes the mail as them when it is sent.

  import { tick, untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    Button, Dialog, Icon, IconButton, Inline, Input, Menu, PageHeader, Select, Skeleton, Stack, VisuallyHidden, WorkPage,
  } from '@hubtask/design-system/components';
  import { formatQuery, parseQuery, type QueryKey } from '@hubtask/query';
  import type { Dashboard, DashboardDigest, DashboardFigures, DashboardWidgetWrite, GoalList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { ticketKeyOf } from '../lib/data/ticket.ts';
  import { lineOf, personNamed, readQuery, type Names } from '../lib/data/viewquery.ts';
  import QueryStrip from '../lib/entries/QueryStrip.svelte';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { entryTypes, typeNameOf } from '../lib/data/entrytypes.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    MAX_WIDGETS, csvOf, listFilter, dashboardPath, digestPath, figureRows, figuresPath, goals, hubOf, leftRows, moved, nextBox, readingOrder, resized, runningRows,
    seriesTable, widgetWrite, type FigureNaming, type FigureRow, type SeriesTable,
  } from '../lib/data/goals.svelte.ts';
  import { SPREADSHEET_TYPE, xlsxOf } from '../lib/data/xlsx.ts';
  import { views } from '../lib/data/views.svelte.ts';
  import { handOver } from '../lib/data/listhandoff.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import WidgetChart, { type ListEntry } from '../lib/goals/WidgetChart.svelte';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import SharingPicker, { type Sharing } from '../lib/goals/SharingPicker.svelte';
  import WidgetEditor from '../lib/goals/WidgetEditor.svelte';
  import { formatDateTime, formatPlainDay, formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { platform } from '../lib/platform/index.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    dashboardId: string;
    onnavigate: (path: string) => void;
  }

  const { dashboardId, onnavigate }: Props = $props();

  let dashboard = $state<Dashboard | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = dashboardPath(dashboardId);
    return untrack(() => engine.subscribe<Dashboard>({ path }, (next) => {
      if (next.status === 'ready') { dashboard = next.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });
  let figures = $state<DashboardFigures | undefined>(undefined);
  let figuresRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = figuresPath(dashboardId);
    return untrack(() => engine.subscribe<DashboardFigures>({ path }, (next) => {
      if (next.status === 'ready') { figures = next.data; figuresRefusal = undefined; }
      if (next.status === 'failed') figuresRefusal = renderProblem(next.error, messages).message;
    }));
  });
  $effect(() => frame.entitle(dashboard ? dashboard.name : t('app.dashboards.dashboard')));
  $effect(() => frame.locate(dashboard ? hubOf(dashboard) : undefined));

  /** A private dashboard is its owner's; a shared one is changed by whoever the server lets. */
  const mayEdit = $derived(dashboard !== undefined && (dashboard.sharing !== 'PRIVATE' || dashboard.owner_id === actor.account?.id));
  const placeHub = $derived(hubOf(dashboard));
  const placeName = $derived(placeHub ? (containers.find(placeHub)?.name ?? t('app.goals.hub_unread')) : t('app.goals.workspace'));
  const locale = $derived(messages.locale);

  let failure = $state<string | undefined>(undefined);
  async function act(run: () => Promise<unknown>, done?: string): Promise<boolean> {
    failure = undefined;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    }
  }

  // --- Editing on the grid ----------------------------------------------------------------------

  let isEditing = $state(false);
  let layout = $state<DashboardWidgetWrite[]>([]);
  /** A widget's empty answer named by its real cause (§7.7), never one sentence for all. */
  function emptyOf(widget: { source: string; goal_ids?: readonly string[] }): string {
    if (widget.source === 'GOALS') return t((widget.goal_ids ?? []).length === 0 ? 'app.dashboards.empty_goals_none' : 'app.dashboards.empty_goals_unread');
    if (widget.source === 'VIEW') return t('app.dashboards.empty_view');
    return t('app.dashboards.empty_query');
  }

  /** The layouts before each change, newest last: *Rückgängig* takes one back. */
  let history = $state<DashboardWidgetWrite[][]>([]);
  let isLibraryOpen = $state(false);
  function startEditing() {
    if (!dashboard) return;
    layout = dashboard.widgets.map(widgetWrite);
    history = [];
    isEditing = true;
  }
  function change(next: DashboardWidgetWrite[]) {
    history = [...history, layout];
    layout = next;
  }
  function undo() {
    const last = history[history.length - 1];
    if (!last) return;
    history = history.slice(0, -1);
    layout = last;
  }
  function discard() {
    isEditing = false;
    isLibraryOpen = false;
    history = [];
  }
  const boxOf = (each: DashboardWidgetWrite) => ({ x: each.x ?? 0, y: each.y ?? 0, w: each.w ?? 6, h: each.h ?? 2 });
  const shown = $derived<DashboardWidgetWrite[]>(isEditing ? layout : (dashboard?.widgets ?? []).map(widgetWrite));
  const ordered = $derived(readingOrder(shown.map((widget, index) => ({ ...boxOf(widget), widget, index }))));
  function place(index: number, next: { x: number; y: number; w: number; h: number }, isLive = false) {
    const placed = layout.map((each, i) => (i === index ? { ...each, ...next } : each));
    if (isLive) layout = placed;
    else change(placed);
    const widget = placed[index];
    if (widget) announced = t('app.dashboards.where_of', { title: widget.title, x: String(next.x + 1), y: String(next.y + 1), w: String(next.w), h: String(next.h) });
  }
  let announced = $state('');

  // Dragging by the handle and resizing by the corner move by whole cells (WCAG 2.5.7: the
  // keyboard does both too - arrows move, Shift with arrows resizes).
  let grid = $state<HTMLElement | undefined>(undefined);
  function dragFrom(event: PointerEvent, index: number, mode: 'move' | 'size') {
    const article = (event.currentTarget as HTMLElement).closest('article');
    const start = layout[index];
    if (!article || !grid || !start || event.button !== 0) return;
    event.preventDefault();
    const gap = Number.parseFloat(getComputedStyle(grid).columnGap) || 0;
    const box = boxOf(start);
    const cellW = (article.offsetWidth + gap) / box.w;
    const cellH = (article.offsetHeight + gap) / box.h;
    const handle = event.currentTarget as HTMLElement;
    handle.setPointerCapture(event.pointerId);
    history = [...history, layout];
    const fromX = event.clientX;
    const fromY = event.clientY;
    const onMove = (next: PointerEvent) => {
      const dx = Math.round((next.clientX - fromX) / cellW);
      const dy = Math.round((next.clientY - fromY) / cellH);
      place(index, mode === 'move' ? moved(box, dx, dy) : resized(box, dx, dy), true);
    };
    const onUp = () => {
      handle.removeEventListener('pointermove', onMove);
      handle.removeEventListener('pointerup', onUp);
      handle.removeEventListener('pointercancel', onUp);
    };
    handle.addEventListener('pointermove', onMove);
    handle.addEventListener('pointerup', onUp);
    handle.addEventListener('pointercancel', onUp);
  }
  function keyed(event: KeyboardEvent, index: number, mode: 'move' | 'size') {
    const steps: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] };
    const step = steps[event.key];
    const widget = layout[index];
    if (!step || !widget) return;
    event.preventDefault();
    const box = boxOf(widget);
    const handle = event.currentTarget as HTMLElement;
    place(index, mode === 'size' || event.shiftKey ? resized(box, step[0], step[1]) : moved(box, step[0], step[1]));
    // Moving in reading order moves the element, which drops its focus: the handle keeps it.
    void tick().then(() => handle.focus());
  }

  // --- The library: the nine kinds, previewed with one's own data -------------------------------

  type Kind = DashboardWidgetWrite['chart'];
  const KINDS: readonly Kind[] = ['NUMBER', 'BAR', 'LINE', 'LIST', 'PIE', 'TABLE', 'MATRIX', 'PROGRESS', 'TEXT'];
  const KIND_GROUP: Record<Kind, string> = { NUMBER: 'figures', BAR: 'charts', LINE: 'charts', PIE: 'charts', TABLE: 'lists', MATRIX: 'charts', LIST: 'lists', PROGRESS: 'goals', TEXT: 'text' };
  let librarySearch = $state('');
  let libraryGroup = $state('all');
  const libraryKinds = $derived(KINDS.filter((kind) => (libraryGroup === 'all' || KIND_GROUP[kind] === libraryGroup)
    && `${t(`app.dashboards.kind_${kind}`)} ${t(`app.dashboards.kind_${kind}_hint`)}`.toLowerCase().includes(librarySearch.trim().toLowerCase())));
  const filterTree = $derived.by(() => {
    const parsed = dashboard?.filter ? parseQuery(dashboard.filter) : undefined;
    return parsed?.ok ? parsed.query.filter : undefined;
  });
  let previewRows = $state<FigureRow[]>([]);
  let previewEntries = $state<ListEntry[]>([]);
  $effect(() => {
    if (!isLibraryOpen) return;
    const filter = filterTree;
    return untrack(() => {
      const closers = [
        engine.subscribe<{ data: Record<string, unknown>[] }>({ path: '/items:aggregate', body: { ...(filter ? { filter } : {}), aggregate: { function: 'COUNT', group_by: 'status' } } }, (next) => {
          if (next.status === 'ready') previewRows = figureRows(next.data.data, { group: (value) => groupName('status', value), period: (day) => day });
        }),
        engine.subscribe<{ data: Record<string, unknown>[] }>({ path: '/items:query', body: { ...(filter ? { filter } : {}), size: 3 } }, (next) => {
          if (next.status === 'ready') previewEntries = next.data.data.map((row) => ({ id: String(row['id']), key: ticketKeyOf(typeof row['collection_id'] === 'string' ? containers.find(row['collection_id']) : undefined, row['number'] as number) ?? '', title: String(row['title'] ?? ''), href: `/items/${String(row['id'])}`, column: '' }));
        }),
      ];
      return () => closers.forEach((close) => close());
    });
  });
  let preset = $state<DashboardWidgetWrite | undefined>(undefined);
  /** A kind taken from the library opens the editor with it; the widget lands where the drop asked, else under the rest. */
  function addKind(kind: Kind) {
    const source = kind === 'PROGRESS' ? 'GOALS' : kind === 'TEXT' ? 'TEXT' : 'QUERY';
    const query = kind === 'LIST' ? { q: 'is: open sort: due' }
      : kind === 'LINE' ? { aggregate: { function: 'COUNT', bucket: { field: 'created_at', period: 'week' } } }
        : kind === 'NUMBER' ? { aggregate: { function: 'COUNT' } }
          : kind === 'MATRIX' ? { aggregate: { function: 'COUNT', group_by: 'status', bucket: { field: 'created_at', period: 'week' } } }
            : { aggregate: { function: 'COUNT', group_by: 'status' } };
    preset = { title: t(`app.dashboards.kind_${kind}`), chart: kind, source, ...(source === 'QUERY' ? { query } : {}), ...(kind === 'LIST' ? { column: 'due_at' } : {}), x: 0, y: 0, w: 6, h: 2 } as DashboardWidgetWrite;
    openWidget(-1);
  }
  let isDropping = $state(false);
  async function saveLayout() {
    const at = dashboard;
    if (!at) return;
    const ok = await act(() => goals.updateDashboard(at, { name: at.name, sharing: at.sharing, group_ids: at.group_ids ?? [], filter: at.filter ?? '', widgets: layout }), t('app.dashboards.saved'));
    if (ok) { isEditing = false; isLibraryOpen = false; history = []; }
  }

  // The widget dialog: a new one (`editingIndex` -1) or a change.
  let isWidgetOpen = $state(false);
  let editingIndex = $state(-1);
  $effect(() => {
    if (!isWidgetOpen) return;
    const hub = placeHub ?? undefined;
    return untrack(() => {
      const closers = [views.open()];
      if (hub) closers.push(views.open(hub));
      return () => closers.forEach((close) => close());
    });
  });
  const summaryViews = $derived(isWidgetOpen
    ? [...views.of(), ...(placeHub ? views.of(placeHub) : [])].filter((each, i, all) => each.query && 'aggregate' in each.query && all.findIndex((other) => other.id === each.id) === i)
    : []);
  let goalChoices = $state<GoalList['data']>([]);
  $effect(() => {
    if (!isWidgetOpen) return;
    return untrack(() => engine.subscribe<GoalList>({ path: '/goals' }, (next) => { if (next.status === 'ready') goalChoices = next.data.data; }));
  });
  function openWidget(index: number) {
    editingIndex = index;
    isWidgetOpen = true;
  }
  function saveWidget(write: DashboardWidgetWrite) {
    if (editingIndex < 0) {
      change([...layout, { ...write, ...nextBox(layout.map(boxOf), 6, 2) }]);
    } else {
      change(layout.map((each, i) => (i === editingIndex ? { ...write, ...boxOf(each) } : each)));
    }
    isWidgetOpen = false;
    preset = undefined;
  }

  // --- Name and sharing, deleting ---------------------------------------------------------------

  let isRenaming = $state(false);
  const draft = $state({ name: '', sharing: 'PRIVATE' as Sharing, groupIds: [] as string[] });
  async function saveName() {
    const at = dashboard;
    if (!at) return;
    const ok = await act(() => goals.updateDashboard(at, { name: draft.name.trim(), sharing: draft.sharing, group_ids: draft.sharing === 'GROUPS' ? draft.groupIds : [], filter: at.filter ?? '', widgets: at.widgets.map(widgetWrite) }), t('app.dashboards.saved'));
    if (ok) isRenaming = false;
  }
  function openSharing() {
    if (!dashboard) return;
    draft.name = dashboard.name;
    draft.sharing = dashboard.sharing;
    draft.groupIds = [...(dashboard.group_ids ?? [])];
    isRenaming = true;
  }
  let isDeleting = $state(false);
  async function remove() {
    const at = dashboard;
    if (!at) return;
    const ok = await act(() => goals.removeDashboard(at), t('app.dashboards.deleted', { name: at.name }));
    if (ok) { isDeleting = false; onnavigate('/goals?tab=dashboards'); }
  }

  // --- By mail ----------------------------------------------------------------------------------

  let isDigestOpen = $state(false);
  let digest = $state<DashboardDigest | undefined>(undefined);
  const digestDraft = $state({ cadence: 'WEEKLY' as 'DAILY' | 'WEEKLY', weekday: '1', hour: '8' });
  $effect(() => {
    if (!isDigestOpen) return;
    const path = digestPath(dashboardId);
    return untrack(() => engine.subscribe<DashboardDigest>({ path }, (next) => {
      if (next.status === 'ready') {
        digest = next.data;
        digestDraft.cadence = next.data.cadence;
        digestDraft.weekday = String(next.data.weekday ?? 1);
        digestDraft.hour = String(next.data.hour);
      }
      // Not found is a reader without a digest: the form starts empty.
      if (next.status === 'failed') digest = undefined;
    }));
  });
  const weekdays = $derived(Array.from({ length: 7 }, (_, i) => ({
    value: String(i + 1),
    // 5 January 2026 is a Monday.
    label: new Intl.DateTimeFormat(locale, { weekday: 'long', timeZone: 'UTC' }).format(new Date(Date.UTC(2026, 0, 5 + i))),
  })));
  const hours = $derived(Array.from({ length: 24 }, (_, h) => ({
    value: String(h),
    label: new Intl.DateTimeFormat(locale, { hour: 'numeric', minute: '2-digit', timeZone: 'UTC' }).format(new Date(Date.UTC(2026, 0, 5, h))),
  })));
  async function saveDigest() {
    const ok = await act(() => goals.setDigest(dashboardId, {
      cadence: digestDraft.cadence,
      hour: Number(digestDraft.hour),
      ...(digestDraft.cadence === 'WEEKLY' ? { weekday: Number(digestDraft.weekday) } : {}),
      time_zone: actor.zone,
    }), t('app.dashboards.digest_saved'));
    if (ok) isDigestOpen = false;
  }
  async function stopDigest() {
    const ok = await act(() => goals.removeDigest(dashboardId), t('app.dashboards.digest_stopped'));
    if (ok) { digest = undefined; isDigestOpen = false; }
  }

  // --- Figures ----------------------------------------------------------------------------------

  /** A widget's summary document: its own for QUERY, its saved view's for VIEW. */
  function aggregateOf(widget: DashboardWidgetWrite): { group_by?: unknown; bucket?: { period?: unknown } } | undefined {
    const query = widget.source === 'QUERY' ? widget.query : views.of().concat(placeHub ? views.of(placeHub) : []).find((each) => each.id === widget.view_id)?.query;
    const aggregate = (query as { aggregate?: unknown } | null | undefined)?.aggregate;
    return aggregate && typeof aggregate === 'object' ? aggregate as never : undefined;
  }
  const groupFields = $derived(new Set(shown.map((widget) => aggregateOf(widget)?.group_by).filter((field): field is string => typeof field === 'string')));
  const readsViews = $derived(shown.some((widget) => widget.source === 'VIEW'));
  // A group is named by what it is a value of, so the names are read where a widget groups by them.
  $effect(() => {
    const wanted = { types: groupFields.has('entry_type'), views: readsViews, hub: placeHub ?? undefined };
    return untrack(() => {
      const closers = [
        ...(wanted.types ? [entryTypes.open()] : []),
        ...(wanted.views ? [views.open(), ...(wanted.hub ? [views.open(wanted.hub)] : [])] : []),
      ];
      return () => closers.forEach((close) => close());
    });
  });
  $effect(() => {
    if (!groupFields.has('assignee_id') && !groupFields.has('created_by')) return;
    const ids = (figures?.widgets ?? []).flatMap((each) => each.data.map((row) => row['group'])).filter((id): id is string => typeof id === 'string');
    untrack(() => accounts.resolve(ids));
  });
  /** A group's value in words (user-experience.md §5.6), by the field it is a value of. */
  function groupName(field: unknown, value: string): string {
    switch (field) {
      case 'entry_type': return typeNameOf(entryTypes.all, value, (level) => t('app.entry_types.level', { level }));
      case 'type': return t('app.entry_types.level', { level: value });
      case 'assignee_id':
      case 'created_by': return accounts.nameOf(value) ?? t('app.people.unnamed');
      case 'collection_id': return containers.find(value)?.name ?? value;
      case 'is_completed': return t(value === 'true' ? 'app.dashboards.group_done' : 'app.dashboards.group_open');
      case 'status': return ['todo', 'in_progress', 'done'].includes(value) ? t(`workflows.status.${value}`) : value;
      default: return value;
    }
  }
  /** A period's first day as the reader's date (§5.10): a month by its name, a week by its first day. */
  function periodName(period: unknown, day: string): string {
    if (period === 'month') {
      const at = Date.parse(`${day}T00:00:00Z`);
      return Number.isNaN(at) ? day : new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(at);
    }
    const date = formatPlainDay(day, locale);
    return period === 'week' ? t('app.dashboards.week_of', { day: date }) : date;
  }
  function namingOf(widget: DashboardWidgetWrite): FigureNaming {
    const aggregate = aggregateOf(widget);
    return { group: (value) => groupName(aggregate?.group_by, value), period: (day) => periodName(aggregate?.bucket?.period, day) };
  }

  const figureOf = (id: string | undefined) => (id ? figures?.widgets.find((each) => each.id === id) : undefined);
  /** A widget's own rows, as running totals where it is cumulative. */
  function rowsOf(widget: DashboardWidgetWrite): FigureRow[] {
    const rows = figureRows(figureOf(widget.id)?.data ?? [], namingOf(widget));
    return widget.cumulative ? runningRows(rows) : rows;
  }
  /** A widget with a second series as one table: its own, the second, and with running totals what is left. */
  function tableOf(widget: DashboardWidgetWrite): SeriesTable | undefined {
    const figure = figureOf(widget.id);
    if (!widget.compare || !figure) return undefined;
    const running = widget.cumulative === true;
    const second = figureRows(figure.compare?.data ?? [], namingOf(widget));
    return seriesTable([
      { name: widget.compare.first_title, rows: rowsOf(widget), isRunning: running },
      ...(figure.compare?.error ? [] : [{ name: widget.compare.title, rows: running ? runningRows(second) : second, isRunning: running }]),
      ...(figure.remaining ? [{ name: t('app.dashboards.series_left'), rows: leftRows(figure.remaining, namingOf(widget)) }] : []),
    ]);
  }
  function exportWidget(widget: DashboardWidgetWrite, asSheet = false) {
    const title = widget.title;
    const table = tableOf(widget);
    if (table) {
      exportTable(title, [t('app.dashboards.col_period'), ...table.series.map((each) => each.name)],
        table.labels.map((label, i) => [label, ...table.series.map((each) => ({ text: each.texts[i] ?? '', value: each.values[i] }))]), asSheet);
      return;
    }
    const rows = rowsOf(widget);
    exportTable(title, [t('app.dashboards.col_group'), t('app.dashboards.col_period'), t('app.dashboards.col_value')],
      rows.map((row) => [row.label, row.column ?? '', { text: row.text, value: row.value }]), asSheet);
  }
  type Cell = string | { text: string; value: number | undefined };

  // --- The filter every widget inherits (§6.5) --------------------------------------------------

  /** Names for the line as the collections read them (ADR-0102): a place, a person among those of the dashboard's place. */
  const peoplePath = $derived(placeHub ? { hubId: placeHub } : {});
  $effect(() => {
    const path = peoplePath;
    return untrack(() => people.open(path));
  });
  const names: Names = {
    person: (name) => personNamed(name, people.candidates(peoplePath), (id) => accounts.nameOf(id)),
    label: () => undefined,
    collection: (name) => {
      const wanted = name.toLowerCase();
      return [...containers.hubs, ...containers.loadedCollections].find((each) => each.name.toLowerCase() === wanted)?.id;
    },
    nameOf: (field, value) => (field === 'hub_id' || field === 'collection_id' ? containers.find(value)?.name : accounts.nameOf(value)),
  };
  /** A stored query text - identifiers for names - as the line a person reads. */
  function namedLine(text: string | undefined): string {
    if (!text) return '';
    const parsed = parseQuery(text);
    if (!parsed.ok || !parsed.query.filter) return text;
    return lineOf({ filter: parsed.query.filter as never }, names) ?? text;
  }
  const filterLine = $derived(namedLine(dashboard?.filter));
  const filterPieces = $derived(filterLine === '' ? [] : filterLine.match(/(?:[^\s"]+:\s*(?:"[^"]*"|\S+))|"[^"]*"|\S+/gu) ?? []);
  const queryKeys = $derived<readonly QueryKey[]>(((manifest.value as { query_keys?: readonly QueryKey[] } | undefined)?.query_keys ?? []).filter((key) => !['asset', 'page', 'goal'].includes(key.product ?? '')));
  let filterRefusal = $state<string | undefined>(undefined);
  async function saveFilter(next: string) {
    const at = dashboard;
    if (!at) return;
    const read = readQuery(next, names);
    if (!read.ok) { filterRefusal = t(read.error.code); return; }
    if (read.unknown.length > 0) { filterRefusal = t('app.dashboards.filter_unknown', { names: read.unknown.join(', ') }); return; }
    filterRefusal = undefined;
    const filter = read.query.filter ? formatQuery({ filter: read.query.filter as never }) : '';
    if (filter === (at.filter ?? '')) return;
    await act(() => goals.updateDashboard(at, { name: at.name, sharing: at.sharing, group_ids: at.group_ids ?? [], filter, widgets: at.widgets.map(widgetWrite) }));
  }
  /** Widgets that do not take the whole filter: one fed by a saved view, or one that released a field. */
  const ownFilters = $derived(shown.filter((widget) => widget.source === 'VIEW' || (widget.released?.length ?? 0) > 0).length);

  // --- One widget's anatomy: title, content, its query in one line, ⋯ -----------------------------

  let tables = $state<Record<string, boolean>>({});
  const viewName = (id: string | null | undefined) => views.of().concat(placeHub ? views.of(placeHub) : []).find((each) => each.id === id)?.name ?? t('app.dashboards.view_unread');
  /** The widget's query in one line, as its foot shows it. */
  function sourceLine(widget: DashboardWidgetWrite): string {
    if (widget.source === 'VIEW') return t('app.dashboards.from_view', { name: viewName(widget.view_id) });
    if (widget.source === 'GOALS') return t('app.dashboards.from_goals', { count: String(widget.goal_ids?.length ?? 0) });
    if (widget.source !== 'QUERY') return '';
    const query = (widget.query ?? {}) as { q?: unknown; filter?: unknown };
    const own = typeof query.q === 'string' ? query.q : query.filter ? (lineOf({ filter: query.filter as never }, names) ?? '') : '';
    const released = widget.released?.length ? t('app.dashboards.released_line', { fields: widget.released.join(', ') }) : '';
    return [own, released].filter(Boolean).join(' · ');
  }
  /** The widget's own filter as a tree: its stored one, or its line read. */
  function ownTree(widget: DashboardWidgetWrite): Record<string, unknown> | undefined {
    const query = (widget.query ?? {}) as { q?: unknown; filter?: unknown };
    if (typeof query.q === 'string') {
      const parsed = parseQuery(query.q);
      return parsed.ok ? (parsed.query.filter as Record<string, unknown> | undefined) : undefined;
    }
    return query.filter && typeof query.filter === 'object' ? query.filter as Record<string, unknown> : undefined;
  }
  // A widget's scope is a place: the hubs' collections are read so a scope is known as one.
  const scopesWidgets = $derived(shown.some((widget) => widget.source === 'QUERY'));
  const hubIds = $derived(containers.hubs.map((hub) => hub.id).join(' '));
  $effect(() => {
    if (!scopesWidgets || hubIds === '') return;
    const ids = hubIds.split(' ');
    const opened = untrack(() => ids.map((id) => containers.openLevel(id)));
    return () => opened.forEach((close) => close());
  });
  /** The collection a widget counts in, where its list can be opened. */
  function countedCollection(widget: DashboardWidgetWrite): string | undefined {
    const query = widget.query as { scope_container_id?: unknown; scope?: { container_id?: unknown } } | undefined;
    const scope = query?.scope?.container_id ?? query?.scope_container_id;
    return typeof scope === 'string' && containers.find(scope)?.type === 'COLLECTION' ? scope : undefined;
  }
  /** Where the widget's entries are listed: its saved view, or the collection it counts in. */
  function listHref(widget: DashboardWidgetWrite): string | undefined {
    if (widget.source === 'VIEW' && widget.view_id) {
      const view = views.of().concat(placeHub ? views.of(placeHub) : []).find((each) => each.id === widget.view_id) as { container_id?: string | null } | undefined;
      return view?.container_id ? `/collections/${view.container_id}?view=${widget.view_id}` : undefined;
    }
    const scope = countedCollection(widget);
    return scope ? `/collections/${scope}` : undefined;
  }
  /**
   * Opens the widget's list. A counted collection opens with the widget's line - what the
   * dashboard passes on and its own - as a changed, unsaved view; the line goes over in memory,
   * never in the address (security.md §9).
   */
  function openList(widget: DashboardWidgetWrite) {
    const href = listHref(widget);
    if (!href) return;
    const scope = widget.source === 'QUERY' ? countedCollection(widget) : undefined;
    if (scope) {
      const tree = listFilter(filterTree as Record<string, unknown> | undefined, widget.released ?? [], ownTree(widget), containers.find(scope)?.parent_id);
      const line = tree ? lineOf({ filter: tree as never }, names) : '';
      if (line) handOver(scope, line);
    }
    onnavigate(href);
  }
  function widgetMenu(widget: DashboardWidgetWrite, key: string) {
    const href = listHref(widget);
    return [
      { id: 'list', label: t('app.dashboards.open_list'), icon: 'list' as const, ...(href ? {} : { disabledReason: t('app.dashboards.open_list_none') }) },
      { id: 'table', label: t(tables[key] ? 'app.dashboards.as_chart' : 'app.dashboards.as_table'), icon: 'table' as const },
      { id: 'csv', label: t('app.dashboards.export_csv'), icon: 'file-text' as const, hasSeparatorBefore: true },
      { id: 'xlsx', label: t('app.dashboards.export_xlsx'), icon: 'layout-template' as const },
    ];
  }
  function onWidgetMenu(widget: DashboardWidgetWrite, key: string, id: string) {
    if (id === 'list') openList(widget);
    if (id === 'table') tables = { ...tables, [key]: !tables[key] };
    if (id === 'csv') exportWidget(widget);
    if (id === 'xlsx') exportWidget(widget, true);
  }
  /** A LIST widget's entries: key, title and its one column in words. */
  function entriesOf(widget: DashboardWidgetWrite): ListEntry[] {
    const now = Date.now();
    return (figureOf(widget.id)?.data ?? []).map((row) => {
      const id = String(row['id'] ?? '');
      const collection = typeof row['collection_id'] === 'string' ? containers.find(row['collection_id']) : undefined;
      const key = ticketKeyOf(collection, typeof row['number'] === 'number' ? row['number'] : undefined) ?? '';
      const { text, tone } = columnText(widget.column ?? '', row[widget.column ?? ''], now);
      return { id, key, title: String(row['title'] ?? ''), href: `/items/${id}`, column: text, ...(tone ? { tone } : {}) };
    });
  }
  function columnText(field: string, value: unknown, now: number): { text: string; tone?: 'danger' | 'warning' } {
    if (value === null || value === undefined || value === '') return { text: '' };
    if ((field === 'due_at' || field.endsWith('_at')) && typeof value === 'string') {
      const at = Date.parse(value);
      if (field === 'due_at' && at < now) return { text: t('app.dashboards.overdue'), tone: 'danger' };
      return { text: formatRelative(value, locale, now), ...(field === 'due_at' && at - now < 86_400_000 ? { tone: 'warning' as const } : {}) };
    }
    if (field === 'assignee_id' || field === 'created_by') return { text: accounts.nameOf(String(value)) ?? t('app.people.unnamed') };
    if (field === 'status') return { text: groupName('status', String(value)) };
    return { text: String(value) };
  }
  const computedAgo = $derived(figures ? formatRelative(figures.computed_at, locale) : '');
  function exportTable(title: string, heads: string[], lines: Cell[][], asSheet: boolean) {
    const name = title.replace(/[^\p{L}\p{N}]+/gu, '-').slice(0, 60) || 'widget';
    if (asSheet) {
      // A number stays a number in a sheet, so the value is the row's own, not its text.
      const sheet = xlsxOf('Widget', heads, lines.map((line) => line.map((cell) => (typeof cell === 'string' ? cell : cell.text === '' || cell.value === undefined ? '' : cell.value))));
      platform.saveFile(new Blob([sheet as BlobPart], { type: SPREADSHEET_TYPE }), `${name}.xlsx`);
      toast.show({ text: t('app.dashboards.exported_sheet', { title }), tone: 'success' });
      return;
    }
    const csv = csvOf(heads, lines.map((line) => line.map((cell) => (typeof cell === 'string' ? cell : cell.text))));
    platform.saveFile(new Blob([csv], { type: 'text/csv;charset=utf-8' }), `${name}.csv`);
    toast.show({ text: t('app.dashboards.exported', { title }), tone: 'success' });
  }
</script>

<!-- A work page, not an overview: the person lays its widgets out on their own grid (page-system §7.8). -->
<WorkPage>
  {#snippet top()}
  <PageHeader
    title={dashboard?.name ?? t('app.dashboards.dashboard')}
    subtitle={dashboard ? [t(`app.dashboards.subtitle_${dashboard.sharing}`, { place: placeName }), ...(computedAgo ? [t('app.dashboards.computed_ago', { when: computedAgo })] : [])].join(' · ') : undefined}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [{ id: 'dashboards', label: t('app.goals.tab_dashboards'), href: '/goals?tab=dashboards' }, { id: 'dashboard', label: dashboard?.name ?? '' }],
      label: t('app.goals.trail'),
      expandLabel: t('app.goals.expand_trail'),
      onnavigate: () => onnavigate('/goals?tab=dashboards'),
    }}
    primary={undefined}
    secondary={mayEdit
      ? isEditing
        ? [{ label: t('app.dashboards.add_widget'), icon: 'plus', onclick: () => { isLibraryOpen = !isLibraryOpen; }, ...(layout.length >= MAX_WIDGETS ? { disabledReason: t('dashboards.too_many_widgets') } : {}) }]
        : [{ label: t('app.dashboards.edit'), icon: 'pencil', onclick: startEditing }, { label: t('app.dashboards.share'), icon: 'users', onclick: openSharing }]
      : []}
    menu={dashboard ? {
      label: t('app.goals.more'),
      items: [
        { id: 'digest', label: t('app.dashboards.digest'), icon: 'mail' as const },
        ...(mayEdit ? [
          { id: 'rename', label: t('app.dashboards.rename'), icon: 'pencil' as const },
          { id: 'delete', label: t('app.dashboards.delete'), icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true },
        ] : []),
      ],
      onselect: (id: string) => {
        if (id === 'digest') isDigestOpen = true;
        if (id === 'cancel') discard();
        if (id === 'rename') openSharing();
        if (id === 'delete') isDeleting = true;
      },
    } : undefined}
  />
  {/snippet}

  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !dashboard}
    <div aria-busy="true"><Skeleton lines={8} /></div>
  {:else}
    {#if figuresRefusal}<FailureNote problem={figuresRefusal} />{/if}

    <!-- The filter every widget inherits (§6.5): chips of one query line; a reader reads them. -->
    {#if mayEdit || filterPieces.length > 0}
      <section class="filterbar" aria-label={t('app.dashboards.filter_all')} data-dashboard-filter>
        <span class="filter-lead"><Icon name="funnel" size="sm" />{t('app.dashboards.filter_all')}</span>
        {#if mayEdit}
          <QueryStrip line={filterLine} keys={queryKeys} onchange={(next) => void saveFilter(next)} />
        {:else}
          <ul class="chips">
            {#each filterPieces as piece, i (i)}
              {@const at = piece.indexOf(':')}
              <li class="chip">{#if at > 0}<span class="chip-key">{piece.slice(0, at)}</span><span>{piece.slice(at + 1).trim()}</span>{:else}{piece}{/if}</li>
            {/each}
          </ul>
        {/if}
        {#if ownFilters > 0}<span class="own">{t('app.dashboards.own_filters', { count: String(ownFilters) })}</span>{/if}
      </section>
      {#if filterRefusal}<p class="failure" role="alert">{filterRefusal}</p>{/if}
    {/if}
    {#if isEditing}
      <div class="band" role="toolbar" aria-label={t('app.dashboards.editing')} data-edit-band>
        <span class="quiet">{t('app.dashboards.editing_hint')}</span>
        <Inline gap="100">
          <Button icon="undo-2" disabledReason={history.length === 0 ? t('app.dashboards.undo_none') : undefined} onclick={undo}>{t('app.dashboards.undo')}</Button>
          <Button onclick={discard}>{t('app.dashboards.discard_short')}</Button>
          <Button tone="primary" icon="check" onclick={() => void saveLayout()}>{t('app.dashboards.done')}</Button>
        </Inline>
      </div>
    {/if}

    {#if ordered.length === 0}
      <ProductEmpty product="GOALS" place={{ type: 'TENANT' }} kind={mayEdit ? 'unused' : 'settled'} icon="gauge" title={t('app.dashboards.no_widgets_title')}
        description={mayEdit ? t('app.dashboards.no_widgets') : undefined}
        create={mayEdit && !isEditing ? { label: t('app.dashboards.add_widget'), onclick: () => { startEditing(); openWidget(-1); } } : undefined} />
    {:else}
      <div class="board" class:with-side={isEditing && isLibraryOpen}>
      <div bind:this={grid} class="grid" class:editing={isEditing} class:stacked={viewport.isCompact} class:half={!viewport.isCompact && viewport.isBelowExpanded}>
        {#each ordered as each (each.widget.id ?? `new-${each.index}`)}
          {@const figure = figureOf(each.widget.id)}
          {@const key = each.widget.id ?? `new-${each.index}`}
          {@const foot = sourceLine(each.widget)}
          <article class="widget" style:--x={each.x + 1} style:--y={each.y + 1} style:--w={each.w} style:--h={each.h} style:--half={Math.min(6, Math.max(1, Math.ceil(each.w / 2)))} aria-label={each.widget.title} data-widget={each.widget.chart}>
            <header class="widget-head">
              {#if isEditing}
                <button type="button" class="handle" aria-label={t('app.dashboards.move_handle', { title: each.widget.title })} aria-describedby="dashboard-keys"
                  onpointerdown={(event) => dragFrom(event, each.index, 'move')} onkeydown={(event) => keyed(event, each.index, 'move')}><Icon name="grip-vertical" size="sm" /></button>
              {/if}
              <h2 class="heading">{each.widget.title}</h2>
              {#if isEditing}
                <IconButton icon="settings" size="sm" tone="subtle" label={t('app.dashboards.change_widget', { title: each.widget.title })} onclick={() => openWidget(each.index)} />
                <IconButton icon="trash" size="sm" tone="subtle" label={t('app.dashboards.remove_widget', { title: each.widget.title })} onclick={() => change(layout.filter((_, i) => i !== each.index))} />
              {/if}
              {#if !isEditing && figure && !figure.error && each.widget.chart !== 'TEXT'}
                <Menu label={t('app.dashboards.widget_menu', { title: each.widget.title })} items={widgetMenu(each.widget, key)} placement={{ side: 'block-end', align: 'end' }} onselect={(id) => onWidgetMenu(each.widget, key, id)}>
                  {#snippet trigger(props)}<IconButton icon="ellipsis" size="sm" tone="subtle" label={t('app.dashboards.widget_menu', { title: each.widget.title })} {...props} />{/snippet}
                </Menu>
              {/if}
            </header>
            {#if !figures && !isEditing}
              <div aria-busy="true"><Skeleton lines={3} /></div>
            {:else if !figure}
              <p class="quiet">{t('app.dashboards.not_computed')}</p>
            {:else if figure.error}
              <p class="refused" role="status">{t(figure.error)}</p>
            {:else}
              <div class="content">
                <WidgetChart title={each.widget.title} chart={each.widget.chart} rows={each.widget.chart === 'LIST' ? [] : rowsOf(each.widget)} table={tableOf(each.widget)}
                  entries={each.widget.chart === 'LIST' ? entriesOf(each.widget) : []} text={each.widget.text ?? ''} target={each.widget.target} warnAt={each.widget.warn_at}
                  isTable={tables[key] === true} empty={emptyOf(each.widget)} />
              </div>
              {#if figure.compare?.error}<p class="refused" role="status">{t('app.dashboards.series_refused', { title: each.widget.compare?.title ?? '', reason: t(figure.compare.error) })}</p>{/if}
              {#if figure.truncated || figure.compare?.truncated}<p class="quiet">{t('app.dashboards.truncated')}</p>{/if}
            {/if}
            {#if foot}<p class="foot" data-widget-query>{foot}</p>{/if}
            {#if isEditing}
              <button type="button" class="corner" aria-label={t('app.dashboards.resize_handle', { title: each.widget.title })} aria-describedby="dashboard-keys"
                onpointerdown={(event) => dragFrom(event, each.index, 'size')} onkeydown={(event) => keyed(event, each.index, 'size')}></button>
            {/if}
          </article>
        {/each}
        {#if isEditing && isDropping}
          <div class="drop" role="region" aria-label={t('app.dashboards.drop_here')} ondragover={(event) => event.preventDefault()}
            ondrop={(event) => { event.preventDefault(); isDropping = false; const kind = event.dataTransfer?.getData('text/plain') as Kind; if (KINDS.includes(kind)) addKind(kind); }}>
            <Icon name="plus" size="sm" />{t('app.dashboards.drop_here_cells', { w: '6', h: '2' })}
          </div>
        {/if}
      </div>
      {#if isEditing && isLibraryOpen}
        <aside class="side" aria-label={t('app.dashboards.library')} data-widget-library>
          <header class="side-head"><h2 class="heading">{t('app.dashboards.library')}</h2><IconButton icon="x" size="sm" tone="subtle" label={t('app.links.cancel')} onclick={() => (isLibraryOpen = false)} /></header>
          <Input label={t('app.dashboards.library_search')} isLabelHidden placeholder={t('app.dashboards.library_search')} bind:value={librarySearch} />
          <div class="kinds" role="group" aria-label={t('app.dashboards.library_groups')}>
            {#each ['all', 'figures', 'charts', 'lists', 'goals', 'text'] as group (group)}
              <button type="button" class="kind-chip" aria-pressed={libraryGroup === group} onclick={() => (libraryGroup = group)}>{t(`app.dashboards.library_${group}`)}</button>
            {/each}
          </div>
          <ul class="cards">
            {#each libraryKinds as kind (kind)}
              <li>
                <button type="button" class="card" draggable="true" data-kind={kind} onclick={() => addKind(kind)}
                  ondragstart={(event) => { event.dataTransfer?.setData('text/plain', kind); isDropping = true; }} ondragend={() => (isDropping = false)}>
                  <span class="preview" aria-hidden="true">
                    <WidgetChart title={t(`app.dashboards.kind_${kind}`)} chart={kind === 'PROGRESS' ? 'BAR' : kind} rows={kind === 'LINE' ? previewRows.map((row, i) => ({ ...row, period: String(i) })) : previewRows}
                      entries={previewEntries} text={t('app.dashboards.kind_TEXT_sample')} empty={t('app.dashboards.library_no_data')} />
                  </span>
                  <span class="card-text"><strong>{t(`app.dashboards.kind_${kind}`)}</strong><span class="quiet">{t(`app.dashboards.kind_${kind}_hint`)}</span></span>
                </button>
              </li>
            {/each}
          </ul>
          <p class="quiet">{t('app.dashboards.library_hint')}</p>
        </aside>
      {/if}
      </div>
      <VisuallyHidden><span id="dashboard-keys">{t('app.dashboards.keys_hint')}</span><span aria-live="polite">{announced}</span></VisuallyHidden>
    {/if}
  {/if}
</WorkPage>

{#if isWidgetOpen}
  <Dialog size="l" bind:isOpen={isWidgetOpen} title={editingIndex < 0 ? t('app.dashboards.add_widget') : t('app.dashboards.change_widget', { title: layout[editingIndex]?.title ?? '' })} dismissLabel={t('app.links.cancel')}>
    <WidgetEditor widget={editingIndex < 0 ? preset : layout[editingIndex]} {summaryViews} goals={goalChoices} onsave={saveWidget} />
  </Dialog>
{/if}

{#if isDigestOpen}
  <Dialog size="m" bind:isOpen={isDigestOpen} title={t('app.dashboards.digest_title')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.dashboards.digest_hint')}</p>
      <Select label={t('app.dashboards.digest_cadence')} bind:value={digestDraft.cadence}
        options={[{ value: 'DAILY', label: t('app.dashboards.digest_DAILY') }, { value: 'WEEKLY', label: t('app.dashboards.digest_WEEKLY') }]} />
      {#if digestDraft.cadence === 'WEEKLY'}
        <Select label={t('app.dashboards.digest_weekday')} bind:value={digestDraft.weekday} options={weekdays} />
      {/if}
      <Select label={t('app.dashboards.digest_hour')} hint={t('app.dashboards.digest_zone', { zone: actor.zone })} bind:value={digestDraft.hour} options={hours} />
      {#if digest}
        <p class="quiet">{t('app.dashboards.digest_next', { when: formatDateTime(digest.next_at, locale) })}</p>
      {/if}
      <Inline justify="end">
        {#if digest}<Button tone="danger" onclick={() => void stopDigest()}>{t('app.dashboards.digest_stop')}</Button>{/if}
        <Button tone="primary" onclick={() => void saveDigest()}>{t('app.dashboards.digest_save')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

{#if isRenaming}
  <Dialog size="m" bind:isOpen={isRenaming} title={t('app.dashboards.rename')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.dashboards.col_name')} bind:value={draft.name} maxlength={200} />
      <SharingPicker bind:sharing={draft.sharing} bind:groupIds={draft.groupIds} />
      <Inline justify="end"><Button disabledReason={draft.name.trim() === '' ? t('app.dashboards.name_needed') : draft.sharing === 'GROUPS' && draft.groupIds.length === 0 ? t('app.dashboards.groups_needed') : undefined} onclick={() => void saveName()}>{t('app.goals.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isDeleting && dashboard}
  <Dialog size="s" bind:isOpen={isDeleting} title={t('app.dashboards.delete_title', { name: dashboard.name })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.dashboards.delete_hint')}</p>
      <Inline justify="end"><Button tone="danger" onclick={() => void remove()}>{t('app.dashboards.delete')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(12, minmax(0, 1fr));
    grid-auto-rows: minmax(var(--sp-1000), auto);
    gap: var(--sp-200);
  }
  .widget {
    grid-column: var(--x) / span var(--w);
    grid-row: var(--y) / span var(--h);
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-width: 0;
    padding: var(--sp-150) var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  /* A tablet halves the columns and places the widgets in reading order; a phone stacks them. */
  .grid.half { grid-template-columns: repeat(6, minmax(0, 1fr)); grid-auto-flow: row dense; }
  .grid.half .widget { grid-column: span var(--half); grid-row: auto; }
  .grid.stacked { display: flex; flex-direction: column; }
  .board { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); align-items: start; }
  .board.with-side { grid-template-columns: minmax(0, 1fr) var(--layout-pane-width); }
  .grid.editing { padding: var(--sp-100); border: var(--bw-hairline) dashed var(--border-strong); border-radius: var(--r-lg); }
  .grid.editing .widget { position: relative; }
  .handle { display: inline-grid; place-items: center; padding: 0; border: none; background: none; color: var(--text-subtle); cursor: grab; touch-action: none; }
  .handle:focus-visible, .corner:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .corner { position: absolute; inset-block-end: var(--sp-050); inset-inline-end: var(--sp-050); inline-size: var(--sp-200); block-size: var(--sp-200); padding: 0; border: none; border-block-end: var(--bw-thick) solid var(--text-subtle); border-inline-end: var(--bw-thick) solid var(--text-subtle); background: none; cursor: nwse-resize; touch-action: none; }
  .drop { grid-column: 1 / -1; display: flex; align-items: center; justify-content: center; gap: var(--sp-050); min-block-size: var(--sp-1000); border: var(--bw-thick) dashed var(--accent-primary); border-radius: var(--r-lg); background: var(--accent-primary-subtle); color: var(--text-brand); }
  .band { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); padding: var(--sp-100) var(--sp-150); border-radius: var(--r-lg); background: var(--accent-primary-subtle); }
  .side { display: flex; flex-direction: column; gap: var(--sp-150); padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .side-head { display: flex; align-items: center; justify-content: space-between; }
  .kinds { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .kind-chip { padding: var(--sp-025) var(--sp-100); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-full); background: var(--bg-surface); color: var(--text-primary); font: inherit; font-size: var(--fs-075); cursor: pointer; }
  .kind-chip[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); color: var(--text-brand); }
  .cards { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .card { display: grid; grid-template-columns: var(--sp-1000) minmax(0, 1fr); gap: var(--sp-150); inline-size: 100%; padding: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); color: var(--text-primary); font: inherit; text-align: start; cursor: grab; }
  .card:hover { border-color: var(--border-strong); }
  .preview { overflow: hidden; block-size: var(--sp-800); padding: var(--sp-050); border-radius: var(--r-sm); background: var(--bg-surface-sunken); pointer-events: none; font-size: var(--fs-050); }
  .card-text { display: flex; flex-direction: column; gap: var(--sp-025); font-size: var(--fs-075); }
  .widget-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); min-block-size: var(--sp-300); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .content { flex: 1 1 auto; min-width: 0; }
  .foot { margin: 0; overflow: hidden; color: var(--text-subtle); font-family: var(--font-mono); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  .filterbar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100) var(--sp-150); padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .filterbar :global([data-query-strip]) { flex: 1 1 0; }
  .filter-lead { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); white-space: nowrap; }
  .chips { display: flex; flex: 1 1 0; flex-wrap: wrap; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .chip { display: inline-flex; gap: var(--sp-050); padding: var(--sp-025) var(--sp-100); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-sm); font-size: var(--fs-075); }
  .chip-key { color: var(--text-secondary); }
  .own { margin-inline-start: auto; color: var(--text-secondary); font-size: var(--fs-075); white-space: nowrap; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .refused { margin: 0; color: var(--text-warning); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
