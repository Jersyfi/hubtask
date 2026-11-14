<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Plan* of a hub or a collection (M2, UC-PLN-02 … 05): the schedule as a Gantt chart, as a
  // table and its dependencies. A hub's are tabs of the tool's one view bar beside *Releases* and
  // *Sprints*, each at its address; a collection's *Timeline* is one view of its bar, so the table
  // and the dependencies stand under its chart (the navigation concept §3: never a second strip).
  // What has no dates is a fold under the axis - a sheet on a phone. The chart's window fits the
  // plan; the tolerance and the baselines are behind *Plan settings* (user-experience.md §7.5).
  //
  // **The server schedules; this screen draws and writes.** Every date is the one
  // `GET /plans/{id}/schedule` answered; a drag, a mode, a milestone, a dependency's timing or an
  // accepted move is a write, and the schedule is read again after it. A client that re-scheduled
  // would be a second scheduler, and two schedulers disagree.
  //
  // **The table is the keyboard path** (UC-PLN-02 check 7, SC 2.5.7): every row has its mode, its
  // milestone mark and its proposed move as real controls, every dependency its kind and lag (in a
  // popover, a bottom sheet on a phone, §5.1), and
  // the entry's own screen - one press on its title - sets its dates. The chart is the picture of
  // the same answer, never the only way to change it.
  //
  // **Offline** the copy answers the plan as it stands (`replica.ts`): no proposal, float or
  // critical chain, which only the server's scheduler reckons; a banner says so.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS at the plan the
  // chart's bars are locked and the table shows values, not controls.
  //
  // **A baseline is deleted behind *Undo*** (§5.9): it leaves the screen at once and the deletion
  // is sent when its message leaves; *Undo* sends nothing, and a closed tab keeps the baseline.

  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    Banner, Button, Checkbox, DataTable, DetailPane, Dialog, Icon, IconButton, StateLozenge, Inline, Input, Popover, Select, Sheet, Skeleton, Stack, Timeline, VisuallyHidden, WorkPage,
    type DataColumn, type DataRowMenu, type PageAction, type TimelineCarry, type TimelineColumn, type TimelineDependency, type TimelineLink,
  } from '@hubtask/design-system/components';
  import type { QueryKey } from '@hubtask/query';
  import type { BaselineComparison, PlanBaselineList, PlanSchedule, PlanTolerance, WorkItem } from '@hubtask/sync-engine';
  import { namesPlan, planNames } from '../lib/data/planNames.svelte.ts';
  import QueryChips from '../lib/entries/QueryChips.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { changeRefusalCode, holds, mayDo, roleOrder, supports } from '../lib/data/capability.svelte.ts';
  import { archivalOfItem } from '../lib/data/lifecycle.ts';
  import EntryActions from '../lib/entries/EntryActions.svelte';
  import EntryMenu from '../lib/entries/EntryMenu.svelte';
  import { attachEntryGestures } from '../lib/entries/entryGestures.ts';
  import { entryMenu } from '../lib/entries/entryMenu.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { addDays, dueInputFor, shift, type Window } from '../lib/data/due.ts';
  import { archivalOf } from '../lib/data/containers.ts';
  import { dependenciesAmong } from '../lib/data/links.ts';
  import { linksAmong } from '../lib/data/links.svelte.ts';
  import { device } from '../lib/device.svelte.ts';
  import ItemView from './ItemView.svelte';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import {
    PLAN_CSV_HEADS, baselinesPath, dependencyDrawn, fitWindow, ganttRows, lagOf, planCsvRows, planRolePath, planXlsxOf, planning, proposedMoves, schedulePath, tolerancePath,
  } from '../lib/data/planning.svelte.ts';
  import { anchorOf, draggedTo, gridOf, lineAtOrBefore, placedBetween, scaleOf, windowOf, type Scale } from '../lib/data/schedule.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDay, formatPlainDay } from '../lib/i18n/datetime.ts';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { firstWeekdayOf } from '../lib/i18n/week.ts';
  import { dateIn, instantOf, todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { platform } from '../lib/platform/index.ts';
  import { csvOf } from '../lib/data/goals.ts';
  import { SPREADSHEET_TYPE } from '../lib/data/xlsx.ts';
  import PlanImport from '../lib/planning/PlanImport.svelte';
  import { drawPlan, pngOf, printPicture } from '../lib/planning/planPicture.ts';
  import ToolHead from '../lib/workspace/ToolHead.svelte';
  import CollectionHead from '../lib/workspace/CollectionHead.svelte';
  import PlanViewPanel from '../lib/planning/PlanViewPanel.svelte';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { views as savedViews } from '../lib/data/views.svelte.ts';
  import { VIEW_SCOPES, queryDocumentOf, scopeIdOf, type ViewScope } from '../lib/data/views.ts';
  import { readQuery, type Names } from '../lib/data/viewquery.ts';
  import {
    headParts, isPlanView, keptRows, scheduleOfEntries, planChanges, planGroupingOf, planSetupOf, planViewDefault, slackEnd, tonesOf, type PlanViewSetup,
  } from '../lib/data/planview.ts';

  interface Props {
    /** The hub or collection the plan is. */
    planId: string;
    /** The hub tool's tab the address names; a collection's *Timeline* shows all three. */
    part?: 'schedule' | 'table' | 'dependencies';
    /**
     * A collection without the planning module (E-16): the light timeline from the entries' own
     * dates, written as the entries' own; no Gantt, table, baseline or proposal, which need the module.
     */
    isEntriesOnly?: boolean;
    /** The entry open beside a collection's timeline (`?item=`), from `large` up. */
    openItemId?: string;
    /** A saved view of the timeline the address names (`?view=`). */
    viewAsked?: string;
    onnavigate: (path: string) => void;
  }

  const { planId, part = 'schedule', isEntriesOnly = false, openItemId, viewAsked, onnavigate }: Props = $props();

  const zone = $derived(actor.zone);
  const today = $derived(todayIn(zone));
  const place = $derived(containers.find(planId));
  const placePath = $derived(place?.type === 'HUB' ? `/hubs/${planId}` : `/collections/${planId}`);
  const rolePath = $derived(planRolePath(planId, place));

  $effect(() => frame.entitle(place ? t('app.planning.title_of', { place: place.name }) : t('app.planning.title')));
  $effect(() => untrack(() => workspace.open()));
  $effect(() => {
    const path = rolePath;
    if (path) untrack(() => people.open(path));
  });

  /** An archived collection's entries move nowhere, whatever the role (I-C3). */
  const isArchived = $derived(place ? archivalOf(place) !== 'active' : false);
  const mayWrite = $derived(holds((rolePath && people.roleOf(rolePath, actor.account?.id, roleOrder())) ?? undefined, 'WRITE_ITEMS').status === 'permitted');

  let schedule = $state<PlanSchedule | undefined>(undefined);
  // While the server is away the plan is the copy's (UC-PLN-02 check 9): the entries' own dates and
  // dependencies, nothing proposed, no float or critical chain - said, so nobody reads it as scheduled.
  let fromCopy = $state(false);
  let refusal = $state<string | undefined>(undefined);
  let baselines = $state<PlanBaselineList['data']>([]);
  /** The baselines deleted behind an *Undo* not yet run out: off the screen, still on the server. */
  let leaving = $state<string[]>([]);
  const kept = $derived(baselines.filter((each) => !leaving.includes(each.id)));
  $effect(() => {
    const path = schedulePath(planId, zone);
    const listed = baselinesPath(planId);
    if (isEntriesOnly) return;
    return untrack(() => {
      const stops = [
        engine.subscribe<PlanSchedule>({ path }, (next) => {
          if (next.status === 'ready') {
            schedule = next.data;
            fromCopy = next.source === 'replica';
            refusal = undefined;
          }
          if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
        }),
        engine.subscribe<PlanBaselineList>({ path: listed }, (next) => {
          if (next.status === 'ready') baselines = next.data.data;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  // The plan's time tolerance and its standing against the latest baseline (configuration.md §10):
  // beyond it the plan is in exception, drawn as a danger banner every reader sees.
  let tolerance = $state<PlanTolerance | undefined>(undefined);
  $effect(() => {
    const path = tolerancePath(planId, zone);
    if (isEntriesOnly) return;
    return untrack(() => engine.subscribe<PlanTolerance>({ path }, (next) => { if (next.status === 'ready') tolerance = next.data; }));
  });
  const mayShape = $derived(holds((rolePath && people.roleOf(rolePath, actor.account?.id, roleOrder())) ?? undefined, 'STRUCTURE').status === 'permitted');
  const lockedAbove = $derived(!!tolerance?.locked_by && tolerance.locked_by !== (place?.type === 'HUB' ? 'HUB' : 'COLLECTION'));
  let toleranceDraft = $state('');
  let toleranceLock = $state(false);
  async function saveTolerance(inherit: boolean) {
    const days = inherit || toleranceDraft.trim() === '' ? null : Number(toleranceDraft);
    if (days !== null && (!Number.isInteger(days) || days < 0 || days > 365)) return;
    try {
      await planning.setTolerance(planId, zone, days, toleranceLock);
      toast.show({ text: t('app.planning.tolerance_saved'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  // The plan's view (E-20): the presentation, what is seen, rows, bars, baseline and columns,
  // saved as a view of the plan's place; `base` is the view it opened with, counted against.
  let viewId = $state('');
  let base = $state<PlanViewSetup>(planViewDefault());
  let setup = $state<PlanViewSetup>(planViewDefault());
  let isPanelOpen = $state(false);
  // Without the module the plan opens on the light timeline, which is all it can draw.
  $effect(() => {
    if (isEntriesOnly && setup.presentation !== 'timeline') untrack(() => {
      base = { ...base, presentation: 'timeline' };
      setup = { ...setup, presentation: 'timeline' };
    });
  });
  const changed = $derived(planChanges(setup, base));
  const isGantt = $derived(setup.presentation === 'gantt');
  $effect(() => {
    const at = planId;
    return untrack(() => savedViews.open(at));
  });
  const planViews = $derived(savedViews.of(planId).filter(isPlanView));
  // The view the address names opens once the plan's views are read.
  $effect(() => {
    const asked = viewAsked;
    if (asked && asked !== viewId && planViews.some((each) => each.id === asked)) untrack(() => openView(asked));
  });
  function openView(id: string) {
    const view = planViews.find((each) => each.id === id);
    viewId = view?.id ?? '';
    const line = view ? (readLine(view.query) ?? '') : '';
    base = view ? planSetupOf(view, line) : planViewDefault();
    setup = { ...base };
  }
  /** The line a stored plan view keeps beside its tree, so it reads back as typed. */
  const readLine = (query: unknown): string | undefined => {
    const line = (query as Record<string, unknown> | undefined)?.['line'];
    return typeof line === 'string' ? line : undefined;
  };
  const baselineId = $derived(setup.baselineId);
  let comparison = $state<BaselineComparison | undefined>(undefined);
  $effect(() => {
    const chosen = baselineId;
    comparison = undefined;
    if (!chosen) return;
    void planning.compare(chosen).then(
      (read) => { if (baselineId === chosen) comparison = read; },
      (error) => { failure = renderProblem(error as never, messages).message; },
    );
  });

  const rows = $derived(schedule ? ganttRows(schedule, comparison, (instant) => dateIn(instant, zone)) : []);
  const byId = $derived(new Map(rows.map((row) => [row.id, row])));

  // The hub's releases and the sprints of the plan's collections, read where the line names one or
  // the rows are grouped by release.
  const planned = planNames(() => ({
    hubId: place?.type === 'HUB' ? planId : (place?.parent_id ?? undefined),
    collectionIds: place?.type === 'COLLECTION' ? [planId] : containers.collectionsOf(planId).map((each) => each.id),
    wanted: namesPlan(setup.line) || setup.groupBy === 'release',
  }));
  const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();
  /** Names typed into the line, looked up among the plan's people, releases and sprints (ADR-0102). */
  const names: Names = {
    person: (name) => (rolePath ? people.candidates(rolePath) : []).find((each) => same(accounts.nameOf(each) ?? '', name)),
    label: () => undefined,
    collection: (name) => containers.collectionsOf(place?.type === 'HUB' ? planId : (place?.parent_id ?? '')).find((each) => same(each.name, name))?.id,
    release: planned.release,
    sprint: planned.sprint,
    nameOf: (field, value) =>
      field === 'collection_id' ? containers.find(value)?.name
        : field === 'release_id' ? planned.releaseName(value)
          : field === 'sprint_id' ? planned.sprintName(value)
            : accounts.nameOf(value),
  };
  const lineRead = $derived(setup.line.trim() === '' ? undefined : readQuery(setup.line, names));
  // The entries of the plan as the item read answers them: who works on each, and what the line finds.
  $effect(() => {
    const read = lineRead;
    const at = planId;
    if (read && !read.ok) return;
    // A release or sprint named waits for the names to be read, so no name is sent as an id.
    if (read && !planned.isReady(setup.line)) return;
    return untrack(() => items.openTimeline(at, read?.ok ? read.query : {}));
  });
  const found = $derived(lineRead?.ok && !isEntriesOnly ? new Set(items.onTimeline(planId).map((each) => each.id)) : undefined);
  // Without the module the entries read are the plan: their own dates, the dependencies among them.
  const read = $derived(isEntriesOnly ? items.onTimeline(planId) : []);
  const readIds = $derived(read.map((each) => each.id));
  const among = linksAmong(() => readIds);
  const entryDependencies = $derived(isEntriesOnly
    ? dependenciesAmong(among.links, read.map((each) => ({
        id: each.id, start: each.start_at ? dateIn(each.start_at, zone) : undefined, due: each.due_at ? dateIn(each.due_at, zone) : undefined,
      })))
    : []);
  $effect(() => {
    if (!isEntriesOnly) return;
    const reading = items.stateOf(`timeline:${planId}`);
    if (reading?.status === 'failed') refusal = renderProblem(reading.error, messages).message;
    if (reading?.status !== 'ready' && read.length === 0) return;
    schedule = scheduleOfEntries(read, entryDependencies, (instant) => dateIn(instant, zone), zone) as unknown as PlanSchedule;
  });
  const assigneeOf = $derived(new Map(items.onTimeline(planId).map((each) => [each.id, each.assignee_id ?? undefined])));
  /** Whether the reader may move this entry: a contributor moves only their own (P-05). */
  const myRole = $derived((rolePath && people.roleOf(rolePath, actor.account?.id, roleOrder())) ?? undefined);
  const mayChange = (id: string) =>
    mayWrite && !isArchived && changeRefusalCode(myRole, assigneeOf.get(id) !== undefined && assigneeOf.get(id) === actor.account?.id) === undefined;
  // An entry's menu on the timeline: the one every surface draws (UC-WRK-28 check 1). The plan
  // holds no level, so the moves wait for the one read when the menu opens (D6).
  const entryOf = (id: string) => items.onTimeline(planId).find((each) => each.id === id) ?? items.find(id);
  const menus = $state<Record<string, EntryMenu | undefined>>({});
  let entryActions = $state<EntryActions | undefined>(undefined);
  let chartWindow = $state<HTMLElement | null>(null);
  function menuOf(entry: WorkItem) {
    return entryMenu(
      {
        sendsLinks: platform.sendLink !== undefined,
        surface: 'timeline',
        item: { title: entry.title, isCompleted: entry.completion?.is_completed ?? false, hasDue: Boolean(entry.due_at) },
        archival: archivalOfItem(entry, false),
        isCollectionReadOnly: isArchived,
        mayChange: mayWrite && changeRefusalCode(myRole, entry.assignee_id != null && entry.assignee_id === actor.account?.id) === undefined,
        mayCreate: mayDo(myRole, 'create', false) === true,
        mayShare: holds(myRole, 'MANAGE_MEMBERS').status === 'permitted',
        carries: (capability) => supports(entry.type, capability).status === 'permitted',
        level: entryActions?.levelOf(entry),
      },
      t,
    );
  }
  function openMenu(id: string) {
    const entry = entryOf(id);
    if (entry) entryActions?.readLevel(entry);
    menus[id]?.open(0);
  }
  // A right click and the context-menu key on a row; the long press is the timeline's own (`onHold`).
  $effect(() => {
    const node = chartWindow;
    if (!node) return;
    return attachEntryGestures(node, {
      idAt: (target) => (target as Element | null)?.closest?.('[data-row]')?.getAttribute('data-row') ?? undefined,
      open: openMenu,
      ignore: '[data-row]',
    });
  });
  $effect(() => {
    const ids = [...assigneeOf.values()];
    untrack(() => accounts.resolve(ids));
  });
  const shownRows = $derived.by(() => {
    const kept = keptRows(rows.map((row) => ({ ...row, parentId: row.entry.parent_id ?? undefined, isDone: row.entry.done })), found, setup.done);
    const byStart = (a: (typeof kept)[number], b: (typeof kept)[number]) =>
      setup.sortBy === 'finish' ? String(a.due ?? '').localeCompare(String(b.due ?? '')) : String(a.start ?? '').localeCompare(String(b.start ?? ''));
    if (setup.groupBy === 'outline') return setup.sortBy === 'manual' ? kept : kept;
    const keyOf = (row: (typeof kept)[number]) =>
      setup.groupBy === 'collection' ? (containers.find(row.entry.collection_id)?.name ?? '')
        : setup.groupBy === 'assignee' ? (accounts.nameOf(assigneeOf.get(row.id) ?? '') ?? '')
          : setup.groupBy === 'release' ? (planned.releaseName(row.entry.release_id) ?? '')
            : '';
    return kept
      .filter((row) => row.kind !== 'summary')
      .map((row) => ({ ...row, depth: 0 }))
      // A row in no group - nobody's, no release - comes after the named ones.
      .sort((a, b) => Number(keyOf(a) === '') - Number(keyOf(b) === '') || keyOf(a).localeCompare(keyOf(b)) || (setup.sortBy === 'manual' ? 0 : byStart(a, b)));
  });
  const tones = $derived(
    setup.colourBy === 'collection' ? tonesOf(rows.map((row) => row.entry.collection_id))
      : setup.colourBy === 'assignee' ? tonesOf(rows.map((row) => assigneeOf.get(row.id)))
        : new Map<string, number>(),
  );
  const toneOf = (row: (typeof rows)[number]) =>
    setup.colourBy === 'collection' ? tones.get(row.entry.collection_id) : setup.colourBy === 'assignee' ? tones.get(assigneeOf.get(row.id) ?? '') : undefined;
  const shortDay = (day: string | undefined, month: 'numeric' | 'short' = 'numeric') =>
    day ? new Intl.DateTimeFormat(messages.locale, { day: 'numeric', month, timeZone: 'UTC' }).format(Date.parse(`${day}T00:00:00Z`)) : '';
  /** The few words after a bar: done, critical and late, a milestone's date, the slack. */
  function noteOf(row: (typeof rows)[number]): string | undefined {
    const slip = slipOf(row.id) ?? 0;
    if (setup.bars.progress && row.entry.done && row.kind !== 'milestone') return t('app.planning.note_done');
    if (row.kind === 'milestone') return slip > 0 ? t('app.planning.note_milestone_late', { date: shortDay(row.due, 'short'), days: String(slip) }) : shortDay(row.due, 'short');
    if (!isGantt) return undefined;
    if (setup.bars.critical && row.isCritical) return slip > 0 ? t('app.planning.note_critical_late', { days: String(slip) }) : t('app.planning.critical_named');
    if (setup.bars.slack && row.entry.total_float_days > 0 && row.kind === undefined && !row.entry.done) return t('app.planning.note_slack', { days: String(row.entry.total_float_days) });
    return undefined;
  }
  const initialsOf = (id: string | undefined) => (accounts.nameOf(id ?? '') ?? '').split(/\s+/u).filter(Boolean).slice(0, 2).map((part) => part[0]?.toUpperCase()).join('');
  const outline = $derived<TimelineColumn[]>(
    isGantt
      ? setup.columns
          .filter((id) => id !== 'slip' || comparison)
          .map((id) => ({ id, label: id === 'assignee' ? '' : t(`app.planview.head_${id}`), isNarrow: id === 'assignee' }))
      : [{ id: 'assignee', label: '', isNarrow: true }],
  );
  const titleOf = (id: string) => byId.get(id)?.title ?? t('app.planning.unseen');
  const proposed = $derived(schedule ? proposedMoves(schedule) : []);
  let accepted = $state<string[]>([]);
  /** What is proposed is drawn as a band while the switch is on (scene gantt 2). */
  let showsProposals = $state(true);
  /** The table's *review one by one*: the column of moves to choose appears. */
  let isReviewing = $state(false);
  const broken = $derived(new Set(isEntriesOnly
    ? entryDependencies.filter((each) => each.isEarly).map((each) => `${each.waitedFor}>${each.waiting}`)
    : (schedule?.conflicts ?? []).map((each) => `${each.predecessor_id}>${each.successor_id}`)));

  // A due date alone is drawn as a point, so a bar carried moves the due and invents no start.
  // The entries with no dates follow, which the chart trays with *Give it dates*.
  const undated = $derived(schedule?.undated ?? []);
  const undatedIds = $derived(new Set(undated.map((each) => each.item_id)));
  const timelineRows = $derived([
    ...shownRows.map((row) => ({
      id: row.id,
      title: row.title,
      start: row.entry.due_only ? undefined : row.start,
      due: row.due,
      kind: row.kind === 'milestone' && !setup.bars.milestones ? undefined : row.kind,
      isCritical: isGantt && setup.bars.critical && row.isCritical,
      depth: row.depth,
      baseline: isGantt ? row.baseline : undefined,
      isLocked: !mayChange(row.id) || row.entry.external || row.entry.summary || row.entry.done,
      isDone: setup.bars.progress && row.entry.done,
      slack: isGantt && setup.bars.slack && !row.isCritical && !row.entry.done && row.kind === undefined ? slackEnd(row.due, row.entry.total_float_days) : undefined,
      note: noteOf(row),
      tone: toneOf(row),
    })),
    ...undated.map((each) => ({ id: each.item_id, title: each.title, isLocked: !mayChange(each.item_id) })),
  ]);
  const criticalCount = $derived(rows.filter((row) => row.isCritical).length);

  const dependencies = $derived<TimelineDependency[]>(
    (schedule?.dependencies ?? []).map((each) => {
      const isEarly = broken.has(`${each.predecessor_id}>${each.successor_id}`);
      const words = { waiting: titleOf(each.successor_id), waited: titleOf(each.predecessor_id), kind: t(`app.planning.kind_${each.dependency_kind}`), lag: String(each.lag_days) };
      return {
        id: each.link_id,
        from: each.predecessor_id,
        to: each.successor_id,
        isEarly,
        label: t(isEarly ? 'app.planning.dependency_broken' : 'app.planning.dependency', words),
      };
    }),
  );

  // The window opens on the whole plan at the finest scale that holds it (§7.5), and the reader
  // moves it; *Fit plan* brings it back. A plan without dates opens where the work would be.
  const weekStart = $derived(firstWeekdayOf(actor.weekStart, messages.locale, manifest.supportedLocales, workspace.weekStartAbove));
  let scale = $state<Scale>('week');
  let shown = $state<Window | undefined>(undefined);
  let anchor = $state<string | undefined>(undefined);
  function fit() {
    const fitted = fitWindow(rows.map((row) => ({ start: row.start, due: row.due })));
    if (!fitted) {
      anchor = anchorOf([], today, scale, weekStart);
      shown = undefined;
      return;
    }
    scale = fitted.scale;
    anchor = lineAtOrBefore(fitted.from, fitted.scale, weekStart);
    shown = { from: anchor, to: fitted.to };
  }
  $effect(() => {
    if (anchor !== undefined || !schedule) return;
    untrack(fit);
  });
  const current = $derived(shown ?? windowOf(anchor ?? today, scale, weekStart));

  const partsFor = (of: Scale): Intl.DateTimeFormatOptions =>
    of === 'day'
      ? { day: 'numeric', timeZone: 'UTC' }
      : of === 'week'
        ? { day: 'numeric', month: 'short', timeZone: 'UTC' }
        : of === 'year'
          ? { year: 'numeric', timeZone: 'UTC' }
          : { month: 'short', year: of === 'quarter' ? '2-digit' : undefined, timeZone: 'UTC' };
  const gridlines = $derived.by(() => {
    const format = new Intl.DateTimeFormat(messages.locale, partsFor(scale));
    return gridOf(current, scale, weekStart).map((at) => ({ at, label: format.format(Date.parse(`${at}T00:00:00Z`)) }));
  });
  const dayLabel = (day: string | null | undefined) => formatPlainDay(day, messages.locale);

  function move(direction: 1 | -1) {
    shown = shift(current, direction);
    announcer.say(t('app.timeline.moved', { from: dayLabel(current.from), to: dayLabel(current.to) }));
  }

  let failure = $state<string | undefined>(undefined);

  async function act(run: () => Promise<unknown>, done?: string) {
    failure = undefined;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  /** A bar dragged: its new dates written as the entry's own, then the plan read again. */
  function carried(carry: TimelineCarry) {
    if (!mayChange(carry.id)) return;
    const row = byId.get(carry.id);
    const title = row?.title ?? titleOfUndated(carry.id);
    // A tray entry carried onto the axis is given its first dates where it was let go.
    if (!row && !undatedIds.has(carry.id)) return;
    const was = !row || row.entry.due_only ? undefined : row.start;
    const next = carry.grab === 'place' || !row
      ? placedBetween(addDays(current.from, carry.from), addDays(current.from, carry.to))
      : draggedTo({ start: was, due: row.due }, carry.grab, carry.to - carry.from);
    const start = next.start !== was && next.start ? (instantOf(next.start, undefined, zone) ?? undefined) : undefined;
    const due = next.due !== row?.due && next.due ? dueInputFor({ date: next.due }, zone) : undefined;
    if (start === undefined && due === undefined) return;
    void act(
      () => isEntriesOnly ? writeDates(carry.id, start, due) : planning.moveEntry(planId, zone, carry.id, start, due),
      t('app.timeline.placed_announced', { title }),
    );
  }

  /** Without the module a bar's dates are written as the entry's own, as its screen writes them. */
  async function writeDates(id: string, start: string | undefined, due: ReturnType<typeof dueInputFor> | undefined) {
    const entry = read.find((each) => each.id === id);
    if (!entry) return;
    let version = entry.version;
    if (start !== undefined) version = (await items.update(id, { start_at: start }, version)).version;
    if (due) await items.setDue(id, due, version);
  }

  /** From `large` an entry opens beside a collection's timeline, else as its page (§11.8). */
  const opensBeside = $derived(place?.type === 'COLLECTION' && viewport.isLarge && device.openMode === 'panel');
  const timelinePath = $derived(`/collections/${planId}/timeline`);
  function openEntry(id: string) {
    onnavigate(opensBeside ? `${timelinePath}?item=${encodeURIComponent(id)}` : `/items/${id}`);
  }
  const openItem = $derived(openItemId ? items.find(openItemId) : undefined);
  const panelBounds = $derived.by(() => {
    void viewport.isLarge;
    const root = getComputedStyle(document.documentElement);
    const width = (token: string) => Number.parseFloat(root.getPropertyValue(token)) || 0;
    return { min: width('--layout-panel-min'), reset: width('--layout-panel-width'), max: Math.round(window.innerWidth * 0.7) };
  });
  function closePane() {
    const rowId = openItemId;
    onnavigate(timelinePath);
    queueMicrotask(() => {
      if (document.activeElement && document.activeElement !== document.body) return;
      document.querySelector<HTMLElement>(`[data-row="${rowId}"] button`)?.focus();
    });
  }

  /** A dependency drawn from one bar onto another row. */
  function linked(link: TimelineLink) {
    const drawn = dependencyDrawn(link.from, link.to);
    if (!drawn || !mayWrite) return;
    void act(
      () => planning.link(planId, zone, drawn.sourceId, drawn.targetId, drawn.type),
      t('app.planning.linked', { waiting: titleOf(link.to), waited: titleOf(link.from) }),
    );
  }

  function applyMoves(ids: readonly string[]) {
    if (ids.length === 0) return;
    void act(async () => {
      const out = await planning.apply(planId, zone, ids);
      accepted = [];
      announcer.say(t('app.planning.applied', { count: String(out.moved.length) }));
    }, t('app.planning.applied', { count: String(ids.length) }));
  }

  /** The scheduling mode chosen for an entry and not yet applied, by entry. */
  const modeDraft = $state<Record<string, 'MANUAL' | 'AUTOMATIC'>>({});

  /** The draft of each dependency's timing, by link, until it is saved. */
  const timing = $state<Record<string, { kind: string; lag: string }>>({});
  $effect(() => {
    for (const each of schedule?.dependencies ?? []) {
      untrack(() => {
        timing[each.link_id] ??= { kind: each.dependency_kind, lag: String(each.lag_days) };
      });
    }
  });
  /** A lag the field cannot send, said at the field it was typed in (§5.10), by link. */
  const lagErrors = $state<Record<string, string | undefined>>({});
  function saveTiming(linkId: string) {
    const draft = timing[linkId];
    const lag = draft ? lagOf(draft.lag) : undefined;
    if (!draft || lag === undefined) {
      lagErrors[linkId] = t('planning.lag_out_of_range', { maximum: '999' });
      return;
    }
    lagErrors[linkId] = undefined;
    void act(() => planning.setTiming(planId, zone, linkId, draft.kind as 'FS' | 'SS' | 'FF' | 'SF', lag), t('app.planning.timing_saved'));
  }

  // A dependency added from the keyboard (SC 2.1.1, 2.5.7): what the chart's knob draws, chosen
  // here instead, its kind and lag written straight after when they are not the default.
  let newWaiting = $state('');
  let newWaited = $state('');
  let newKind = $state<string>('FS');
  let newLag = $state('0');
  let newLagError = $state<string | undefined>(undefined);
  function addDependency() {
    const waiting = newWaiting;
    const waited = newWaited;
    const lag = lagOf(newLag);
    const drawn = waiting && waited ? dependencyDrawn(waited, waiting) : undefined;
    if (!drawn) {
      failure = t('app.planning.choose_two');
      return;
    }
    if (lag === undefined) {
      newLagError = t('planning.lag_out_of_range', { maximum: '999' });
      return;
    }
    newLagError = undefined;
    const kind = newKind as 'FS' | 'SS' | 'FF' | 'SF';
    void act(async () => {
      const made = await planning.link(planId, zone, drawn.sourceId, drawn.targetId, drawn.type);
      if (kind !== 'FS' || lag !== 0) await planning.setTiming(planId, zone, made.id, kind, lag);
      newWaiting = '';
      newWaited = '';
      newKind = 'FS';
      newLag = '0';
      isAdding = false;
    }, t('app.planning.linked', { waiting: titleOf(waiting), waited: titleOf(waited) }));
  }
  /** *Free the date*: the entry kept by hand is scheduled, so the dependency holds again. */
  const isKeptByHand = (itemId: string) => byId.get(itemId)?.entry.mode === 'MANUAL';
  function freeDate(itemId: string) {
    void act(() => planning.setScheduling(planId, zone, itemId, { mode: 'AUTOMATIC' }), t('app.planning.date_freed', { title: titleOf(itemId) }));
  }
  function removeDependency(linkId: string, waitingId: string, waitedId: string) {
    void act(
      () => planning.unlink(planId, zone, linkId, waitingId, waitedId),
      t('app.planning.unlinked', { waiting: titleOf(waitingId), waited: titleOf(waitedId) }),
    );
  }
  const linkable = $derived(rows.filter((row) => !row.entry.external && !row.entry.summary).map((row) => ({ value: row.id, label: row.title })));

  let isTaking = $state(false);
  /** A dependency added from the keyboard: four values of one new thing, a small dialog (§5.1). */
  let isAdding = $state(false);
  let isImporting = $state(false);
  let isSetting = $state(false);

  /** A baseline deleted softly: gone from the screen now, from the server when *Undo* has run out. */
  function deleteBaseline(id: string, name: string) {
    leaving = [...leaving, id];
    if (setup.baselineId === id) setup = { ...setup, baselineId: '' };
    // The dialog closes, so its message - and *Undo* - are not behind a modal.
    isSetting = false;
    const back = () => {
      leaving = leaving.filter((each) => each !== id);
    };
    toast.show({
      text: t('app.planning.baseline_deleted_named', { name }),
      undo: back,
      onleave: () => void act(() => planning.deleteBaseline(planId, id)).then(back),
    });
  }

  /**
   * The plan saved as the reader sees it (UC-PLN-09 check 6): a CSV the import reads back, the same
   * rows as a workbook, a PNG
   * of the chart, or the chart printed - which the browser saves as PDF.
   */
  async function exportAs(id: string) {
    if (id === 'settings') {
      isSetting = true;
      return;
    }
    if (id === 'import') {
      isImporting = true;
      return;
    }
    if (!schedule) return;
    const name = `plan-${place?.handle ?? planId}`;
    const title = place ? t('app.planning.title_of', { place: place.name }) : t('app.planning.title');
    if (id === 'csv') {
      platform.saveFile(new Blob([csvOf([...PLAN_CSV_HEADS], planCsvRows(rows, schedule))], { type: 'text/csv;charset=utf-8' }), `${name}.csv`);
    } else if (id === 'xlsx') {
      platform.saveFile(new Blob([planXlsxOf(rows, schedule) as BlobPart], { type: SPREADSHEET_TYPE }), `${name}.xlsx`);
    } else if (id === 'png') {
      platform.saveFile(await pngOf(drawPlan(rows, title, messages.locale)), `${name}.png`);
    } else if (id === 'pdf') {
      printPicture(drawPlan(rows, title, messages.locale), title);
      return;
    } else return;
    toast.show({ text: t('app.planning.exported'), tone: 'success' });
  }
  let baselineName = $state('');
  function take() {
    const name = baselineName.trim();
    if (!name) return;
    isTaking = false;
    baselineName = '';
    void act(() => planning.takeBaseline(planId, name), t('app.planning.baseline_taken', { name }));
  }

  const scales = $derived(
    (['day', 'week', 'month', 'quarter', 'year'] as const).map((value) => ({ value, label: t(`app.planning.scale_${value}`) })),
  );
  const modes = $derived([
    { value: 'MANUAL', label: t('app.planning.mode_MANUAL') },
    { value: 'AUTOMATIC', label: t('app.planning.mode_AUTOMATIC') },
  ]);
  const kinds = $derived((['FS', 'SS', 'FF', 'SF'] as const).map((value) => ({ value, label: t(`app.planning.kind_${value}`) })));
  const columns = $derived<DataColumn[]>([
    { id: 'entry', label: t('app.planning.col_entry'), priority: 1 },
    { id: 'start', label: t('app.planning.col_start'), priority: 2 },
    { id: 'finish', label: t('app.planning.col_finish'), priority: 2 },
    { id: 'duration', label: t('app.planning.col_duration'), align: 'end', priority: 3 },
    { id: 'float', label: t('app.planning.col_float'), align: 'end', priority: 3 },
    ...(comparison ? [{ id: 'slip', label: t('app.planning.col_slip'), align: 'end' as const, priority: 3 as const }] : []),
    { id: 'mode', label: t('app.planning.col_mode'), priority: 2 },
    { id: 'milestone', label: t('app.planning.col_milestone'), priority: 2 },
    ...(isReviewing || accepted.length > 0 ? [{ id: 'move', label: t('app.planning.col_move'), priority: 2 as const }] : []),
  ]);
  // "Waits ‹ on" (scene plandeps 4): the time relation is one chip, which opens it.
  const dependencyColumns = $derived<DataColumn[]>([
    { id: 'waiting', label: t('app.planning.col_waiting'), priority: 1 },
    { id: 'arrow', label: '', priority: 2 },
    { id: 'waited', label: t('app.planning.col_waited'), priority: 2 },
    { id: 'timing', label: t('app.planning.timing'), priority: 1 },
  ]);
  const timingSaid = (each: { dependency_kind: string; lag_days: number }) =>
    each.lag_days === 0 ? t(`app.planning.kind_${each.dependency_kind}`) : t('app.planning.timing_chip', { kind: t(`app.planning.kind_${each.dependency_kind}`), lag: String(each.lag_days) });
  type PlanRow = (typeof rows)[number];
  type PlanDependency = PlanSchedule['dependencies'][number];
  /** A dependency's ⋯: taking it back (PS-6). */
  const dependencyMenu = (each: PlanDependency): DataRowMenu | undefined => mayWrite
    ? {
        label: t('app.planning.dependency_menu', { waiting: titleOf(each.successor_id), waited: titleOf(each.predecessor_id) }),
        items: [{ id: 'remove', label: t('app.planning.remove_dependency_of', { waiting: titleOf(each.successor_id), waited: titleOf(each.predecessor_id) }), icon: 'trash', isDestructive: true }],
      }
    : undefined;
  /** The dependency whose timing is open in its popover or sheet. */
  let timingOpen = $state<string | undefined>(undefined);
  function saveTimingAndClose(linkId: string) {
    saveTiming(linkId);
    if (!lagErrors[linkId]) timingOpen = undefined;
  }

  // A hub shows the tab its address names; a collection's *Timeline* is one view, all three in it.
  const isHub = $derived(place?.type === 'HUB');
  const showsChart = $derived(!isHub || part === 'schedule');
  const showsTable = $derived(isEntriesOnly ? false : !isHub || part === 'table');
  const showsDependencies = $derived(isEntriesOnly ? false : !isHub || part === 'dependencies');
  // On a phone the fold is a sheet (§5.1): a tray inside a chart that scrolls sideways is not reached.
  const chartRows = $derived(viewport.isCompact ? timelineRows.filter((row) => !undatedIds.has(row.id)) : timelineRows);
  const undatedSaid = $derived(t('app.planning.figure_undated', { count: String(undated.length) }));
  let isUndatedOpen = $state(false);
  const titleOfUndated = (id: string) => undated.find((each) => each.item_id === id)?.title ?? '';
  const slipOf = (id: string) => comparison?.entries.find((each) => each.item_id === id)?.finish_variance_days;
  const headSecondary = $derived<readonly [] | readonly [PageAction]>(
    mayWrite && !isEntriesOnly && rows.length > 0 ? [{ label: t('app.planning.take_baseline'), icon: 'flag', onclick: () => (isTaking = true) }] : [],
  );
  const headMenu = $derived({
    label: t('app.planning.more'),
    items: [
      ...(isEntriesOnly ? [] : [{ id: 'settings', label: t('app.planning.settings'), icon: 'settings' as const }]),
      { id: 'csv', label: t('app.planning.export_csv'), icon: 'file-text' as const, hasSeparatorBefore: !isEntriesOnly },
      { id: 'xlsx', label: t('app.planning.export_xlsx'), icon: 'file-text' as const },
      { id: 'png', label: t('app.planning.export_png'), icon: 'image' as const },
      { id: 'pdf', label: t('app.planning.export_pdf'), icon: 'file-text' as const },
      ...(mayWrite && !isEntriesOnly && place?.type === 'COLLECTION' ? [{ id: 'import', label: t('app.planning.import'), icon: 'cloud-upload' as const, hasSeparatorBefore: true }] : []),
    ],
    onselect: (id: string) => void exportAs(id),
  });

  // The head sentence (scenes plan, gantt): end, critical path, slip, tolerance, undated.
  const planSlip = $derived(
    tolerance?.exception?.slip_days ?? (comparison ? Math.max(0, ...comparison.entries.map((each) => each.finish_variance_days ?? 0)) : undefined),
  );
  const head = $derived(
    !schedule || (rows.length === 0 && undated.length === 0)
      ? []
      : headParts({
          finish: schedule.finish ? dayLabel(schedule.finish) : undefined,
          critical: criticalCount,
          slipDays: planSlip,
          baselineName: kept.find((each) => each.id === setup.baselineId)?.name ?? (tolerance?.exception ? kept[0]?.name : undefined),
          toleranceDays: tolerance?.time_tolerance_days,
          undated: undated.length,
        }),
  );


  const queryKeys = $derived<readonly QueryKey[]>(
    ((manifest.value as { query_keys?: readonly QueryKey[] } | undefined)?.query_keys ?? []).filter((key) => !['asset', 'page', 'goal'].includes(key.product ?? '')),
  );
  const hubOf = $derived(place?.type === 'HUB' ? planId : (place?.parent_id ?? undefined));
  const saveScopes = $derived(
    VIEW_SCOPES.filter((scope) => scope === 'ACCOUNT' || (scope === 'COLLECTION' ? place?.type === 'COLLECTION' : scope === 'HUB') && mayShape)
      .map((scope) => ({ value: scope, label: t(`app.views.scope_${scope}`) })),
  );
  let isSavingView = $state(false);
  let saveFailure = $state<string | undefined>(undefined);
  const documentOf = (now: PlanViewSetup) => ({ ...queryDocumentOf(lineRead?.ok ? lineRead.query : undefined, planId), line: now.line });
  async function savingView(work: () => Promise<{ id: string } | unknown>) {
    isSavingView = true;
    saveFailure = undefined;
    try {
      const saved = await work();
      announcer.say(t('app.views.saved_announced'));
      base = { ...setup };
      if (saved && typeof saved === 'object' && 'id' in saved) viewId = String(saved.id);
    } catch (error) {
      saveFailure = renderProblem(error as never, messages).message;
    } finally {
      isSavingView = false;
    }
  }
  function saveView(name: string, scope: string, sharing: 'PRIVATE' | 'SCOPE') {
    const scopeId = scopeIdOf(scope as ViewScope, { collectionId: planId, hubId: hubOf });
    void savingView(() => savedViews.create(
      {
        scope_type: scope as never,
        ...(scopeId ? { scope_id: scopeId } : {}),
        name,
        layout: 'TIMELINE',
        query: documentOf(setup),
        grouping: planGroupingOf(setup),
        visible_fields: [...setup.columns],
        ...(sharing === 'SCOPE' && scope !== 'ACCOUNT' ? { sharing } : {}),
      } as never,
      crypto.randomUUID(),
    ));
  }
  const currentView = $derived(planViews.find((each) => each.id === viewId));
  function overwriteView() {
    const view = currentView;
    if (!view) return;
    void savingView(() => savedViews.update(view.id, { layout: 'TIMELINE', query: documentOf(setup), grouping: planGroupingOf(setup), visible_fields: [...setup.columns] } as never, view.version));
  }
  const viewOptions = $derived([{ value: '', label: t('app.planview.default_name') }, ...planViews.map((each) => ({ value: each.id, label: each.name }))]);
  const panelOpener = $derived(changed > 0 ? t('app.planning.view_changed', { count: String(changed) }) : t('app.viewcfg.button'));
</script>

{#snippet headFigure()}<span data-head-figure>{#each head as part, index (part.code)}{#if index > 0}{' · '}{/if}<span class={part.isWarning ? 'warn' : undefined} data-head-part={part.code}>{t(part.code, part.params)}</span>{/each}</span>{/snippet}
<!-- From `expanded` the panel *Ansicht* stands beside the plan, which narrows (scene planview). -->
<div class="beside-pane">
<WorkPage toolbar={schedule && !refusal && (rows.length > 0 || undated.length > 0) ? toolbar : undefined}>
  {#snippet top()}
  {#if place?.type === 'COLLECTION'}
    <!-- A collection's *Timeline* is one of its views: its head is the collection's (CollectionHead). -->
    <CollectionHead collectionId={planId} current="timeline" {onnavigate} figure={head.length > 0 ? headFigure : undefined} secondary={headSecondary} menu={headMenu} />
  {:else if place}
    <ToolHead hubId={planId} tool="plan" current={part} {onnavigate} figure={head.length > 0 ? headFigure : undefined} secondary={headSecondary} menu={headMenu} />
  {/if}
  {/snippet}

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !schedule}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else if rows.length === 0 && undated.length === 0}
    <ProductEmpty product="PLANNING" place={{ type: containers.find(planId)?.type === 'HUB' ? 'HUB' : 'COLLECTION', id: planId }}
      kind="unused"
      title={t('app.planning.none')}
      description={t('app.planning.none_hint')}
      create={place ? { label: t('app.planning.none_open', { place: place.name }), onclick: () => onnavigate(placePath) } : undefined}
      templates={mayWrite && place?.type === 'COLLECTION' ? [{ label: t('app.planning.import'), onclick: () => (isImporting = true) }] : []}
    />
  {:else}
    {#if fromCopy}<Banner tone="info">{t('app.planning.from_copy')}</Banner>{/if}

    {#if tolerance?.exception?.exceeded}
      <Banner tone="danger" title={t('app.planning.tolerance')}>
        {t('app.planning.exception', { slip: String(tolerance.exception.slip_days), agreed: dayLabel(tolerance.exception.agreed_finish), tolerance: String(tolerance.exception.tolerance_days) })}
      </Banner>
    {/if}

    {#if proposed.length > 0 && showsProposals && part !== 'dependencies'}
      <!-- What the scheduler proposes, as a band with *accept all* and *review* (scene plantable 3). -->
      <div class="band" role="region" aria-label={t('app.planning.proposes', { count: String(proposed.length) })} data-proposals>
        <span class="band-mark" aria-hidden="true">✦</span>
        <p class="band-text"><strong>{t('app.planning.proposes', { count: String(proposed.length) })}</strong> {t('app.planning.proposes_hint')}</p>
        {#if mayWrite}
          <Button size="sm" tone="subtle" aria-pressed={isReviewing} data-review onclick={() => { isReviewing = !isReviewing; if (isHub && part !== 'table') onnavigate(`/hubs/${planId}/plan/table`); }}>{t('app.planning.review')}</Button>
          <Button size="sm" aria-label={t('app.planning.accept_all')} onclick={() => applyMoves(proposed)}>{t('app.planning.accept_all_count', { count: String(proposed.length) })}</Button>
        {/if}
      </div>
    {/if}

    {#if schedule.conflicts.length > 0}
      <!-- What the plan cannot hold stands on top (scene plandeps 2): each with *Free the date*,
           which lets the scheduler place the entry kept by hand. -->
      <div class="band danger" role="alert" data-conflicts>
        <Icon name="triangle-alert" size="sm" />
        <div class="band-text">
          <b>{t('app.planning.cannot_hold', { count: String(schedule.conflicts.length) })}</b>
          {#each schedule.conflicts as conflict (`${conflict.predecessor_id}>${conflict.successor_id}`)}
            <span class="conflict">
              {t('app.planning.conflict', { waiting: titleOf(conflict.successor_id), waited: titleOf(conflict.predecessor_id), date: dayLabel(conflict.earliest) })}
              {#if mayWrite && schedule.conflicts.length > 1 && isKeptByHand(conflict.successor_id)}
                <Button size="sm" tone="subtle" aria-label={t('app.planning.free_date_of', { title: titleOf(conflict.successor_id) })} onclick={() => freeDate(conflict.successor_id)}>{t('app.planning.free_date')}</Button>
              {/if}
            </span>
          {/each}
        </div>
        {#if mayWrite && schedule.conflicts.length === 1 && schedule.conflicts[0] && isKeptByHand(schedule.conflicts[0].successor_id)}
          {@const only = schedule.conflicts[0]}
          <Button size="sm" tone="subtle" data-free-date aria-label={t('app.planning.free_date_of', { title: titleOf(only.successor_id) })} onclick={() => freeDate(only.successor_id)}>{t('app.planning.free_date')}</Button>
        {/if}
      </div>
    {/if}

    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

    {#if showsChart}
      <!-- design-system-lint-ignore: the pane's width is the person's drag, kept by the device. -->
      <div class="split" data-pane={openItemId ? '' : undefined} style:--panel-width={device.panelWidth ? `${device.panelWidth}px` : undefined}>
      <div class="window" data-plan-view="schedule" bind:this={chartWindow}>
        <Timeline
          label={t('app.planning.chart')}
          from={current.from}
          to={current.to}
          {scale}
          rows={chartRows}
          look={isGantt ? 'gantt' : 'light'}
          titleHead={t(isGantt ? 'app.planview.head_task' : 'app.planview.head_entry')}
          {outline}
          {outlineCell}
          hasWeekends={setup.bars.weekends}
          {gridlines}
          {today}
          todayLabel={t('app.timeline.today_is', { date: dayLabel(today) })}
          todayMark={t('app.timeline.today_mark')}
          emptyLabel={t('app.planning.empty')}
          startHandleLabel={t('app.timeline.move_start')}
          dueHandleLabel={t('app.timeline.move_due')}
          onSelect={openEntry}
          currentId={openItemId}
          onCarry={carried}
          dependencies={setup.bars.dependencies ? dependencies : dependencies.filter((each) => each.isEarly)}
          dependenciesLabel={t('app.timeline.dependencies')}
          onLink={mayWrite && !isEntriesOnly ? linked : undefined}
          linkHandleLabel={t('app.planning.draw_dependency')}
          criticalLabel={t('app.planning.critical_named')}
          resizeLabel={t('app.planning.resize_titles')}
          undatedLabel={undatedSaid}
          isTrayFolded
          {rowAction}
          onHold={openMenu}
        />

      </div>
      {#if openItemId}
        {#key openItemId}
          <DetailPane
            title={openItem?.title ?? ''}
            trail={place ? [place.name] : []}
            dismissLabel={t('app.pane.close')}
            view={{ label: t('app.pane.view'), panelLabel: t('app.pane.view_panel'), pageLabel: t('app.pane.view_page'), onpage: () => { device.setOpenMode('whole'); onnavigate(`/items/${openItemId}`); } }}
            onClose={closePane}
            resize={{ label: t('app.pane.width'), value: device.panelWidth ?? panelBounds.reset, min: panelBounds.min, max: panelBounds.max, reset: panelBounds.reset, onresize: (width) => device.setPanelWidth(width) }}
          >
            <ItemView id={openItemId} {onnavigate} isInPane />
          </DetailPane>
        {/key}
      {/if}
      </div>
    {/if}
    {#if showsTable}
      {#if isHub}{@render planTable()}{:else}
        <section aria-labelledby="plan-table" class="block">
          <Inline justify="between" align="center">
            <h2 id="plan-table" class="heading">{t('app.planning.table')}</h2>
            {@render acceptChosen()}
          </Inline>
          {@render planTable()}
        </section>
      {/if}
    {/if}
    {#if showsDependencies}
      {#if isHub}{@render dependencyTable()}{:else if schedule.dependencies.length > 0 || (mayWrite && linkable.length > 1)}
        <section aria-labelledby="plan-dependencies" class="block">
          <Inline justify="between" align="center">
            <h2 id="plan-dependencies" class="heading">{t('app.timeline.dependencies')}</h2>
            {@render addOpener()}
          </Inline>
          {@render dependencyTable()}
        </section>
      {/if}
    {/if}
  {/if}
</WorkPage>
<PlanViewPanel
  bind:isOpen={isPanelOpen}
  bind:setup
  {base}
  hasGantt={!isEntriesOnly}
  baseName={currentView?.name ?? t('app.planview.default_name')}
  {changed}
  keys={queryKeys}
  baselines={kept.map((each) => ({ value: each.id, label: t('app.planning.baseline_named', { name: each.name }) }))}
  views={viewOptions}
  {viewId}
  onview={openView}
  scopes={saveScopes}
  mayShare={mayShape}
  overwritable={currentView?.name}
  isSaving={isSavingView}
  failure={saveFailure}
  onsave={saveView}
  onoverwrite={overwriteView}
/>
</div>

{#snippet acceptChosen()}
  {#if mayWrite && accepted.length > 0}
    <Button size="sm" onclick={() => applyMoves(accepted)}>{t('app.planning.accept_chosen', { count: String(accepted.length) })}</Button>
  {/if}
{/snippet}

{#snippet addOpener(isPrimary = false)}
  {#if mayWrite && linkable.length > 1}
    <Button size="sm" tone={isPrimary ? 'primary' : 'secondary'} icon="plus" data-add-dependency-open onclick={() => (isAdding = true)}>{t('app.planning.add_dependency')}</Button>
  {/if}
{/snippet}

{#snippet toolbar()}
  {#if showsChart}
    <div class="bars">
      <div class="controls" role="group" aria-label={t('app.planning.chart')}>
        {#if !isEntriesOnly}
          <div class="segments quiet-segments" role="group" aria-label={t('app.planning.presentation')}>
            {#each ['timeline', 'gantt'] as const as each (each)}
              <button type="button" aria-pressed={setup.presentation === each} data-plan-presentation={each} onclick={() => (setup = { ...setup, presentation: each })}>
                <Icon name={each === 'gantt' ? 'list' : 'clock'} size="sm" />{t(`app.planning.presentation_${each}`)}
              </button>
            {/each}
          </div>
        {/if}
        <div class="segments quiet-segments">
          <IconButton size="sm" icon="chevron-left" label={t('app.timeline.previous')} onclick={() => move(-1)} />
          <button type="button" onclick={() => { anchor = today; shown = undefined; }}>{t('app.timeline.today')}</button>
          <IconButton size="sm" icon="chevron-right" label={t('app.timeline.next')} onclick={() => move(1)} />
        </div>
        <div class="segments quiet-segments" role="group" aria-label={t('app.planning.zoom')} data-zoom>
          {#each scales as each (each.value)}
            <button type="button" aria-pressed={scale === each.value} data-scale={each.value} onclick={() => { scale = scaleOf(each.value); anchor = current.from; shown = undefined; }}>{each.label}</button>
          {/each}
        </div>
        <Button size="sm" tone="subtle" icon="maximize-2" data-fit onclick={fit}>{t('app.planning.fit')}</Button>
        <span class="grow-bar"></span>
        <Button size="sm" tone="secondary" icon="sliders-horizontal" data-plan-view-open data-opener="view" onclick={() => (isPanelOpen = true)}>{panelOpener}</Button>
      </div>
      <div class="controls second" role="group" aria-label={t('app.planning.chart_options')}>
        {#if isGantt}
          {@render queryLine()}
          <Button size="sm" tone="secondary" icon="zap" aria-pressed={setup.bars.critical} data-critical-switch onclick={() => (setup = { ...setup, bars: { ...setup.bars, critical: !setup.bars.critical } })}>{t('app.planning.critical_path')}</Button>
          {#if kept.length > 0}
            <Select
              label={t('app.planning.baseline')}
              isLabelHidden
              size="sm"
              value={baselineId}
              options={[{ value: '', label: t('app.planning.baseline_none') }, ...kept.map((each) => ({ value: each.id, label: t('app.planning.baseline_named', { name: each.name }) }))]}
              onchange={(event: Event) => (setup = { ...setup, baselineId: (event.currentTarget as HTMLSelectElement).value })}
            />
          {/if}
          {#if proposed.length > 0}
            <Button size="sm" tone="secondary" icon="sparkles" aria-pressed={showsProposals} data-proposals-switch onclick={() => (showsProposals = !showsProposals)}>{t('app.planning.proposals_count', { count: String(proposed.length) })}</Button>
          {/if}
          <Button size="sm" tone="subtle" icon="columns-3" onclick={() => (isPanelOpen = true)}>{t('app.planning.columns')}</Button>
          <span class="quiet legend">{t('app.planning.legend_gantt')}</span>
          {#if viewport.isCompact && undated.length > 0}
            <Button size="sm" tone="secondary" icon="calendar" data-undated-open onclick={() => (isUndatedOpen = true)}>{t('app.planning.undated_button', { count: String(undated.length) })}</Button>
          {/if}
        {:else}
          <!-- The light look's line, as scene plan draws it: grouping, what is seen, the hint, the tray at the end. -->
          <Button size="sm" tone="subtle" icon="layout-grid" data-plan-group-open onclick={() => (isPanelOpen = true)}>{t('app.planning.group_named', { name: t(`app.planview.group_${setup.groupBy}`) })}</Button>
          {@render queryLine()}
          <span class="quiet">{t('app.planning.legend_timeline')}</span>
          {#if undated.length > 0}
            <span class="grow-bar"></span>
            <Button size="sm" tone="secondary" icon="calendar" data-undated-open onclick={() => (isUndatedOpen = true)}>{t('app.planning.undated_button', { count: String(undated.length) })}</Button>
          {/if}
        {/if}
      </div>
      <span class="window-said"><VisuallyHidden>{t('app.timeline.window', { from: dayLabel(current.from), to: dayLabel(current.to) })}</VisuallyHidden></span>
    </div>
  {:else if part === 'table'}
    <div class="controls" data-fill>
      {@render queryLine()}
      {#if kept.length > 0}
        <Select label={t('app.planning.baseline')} isLabelHidden size="sm" value={baselineId}
          options={[{ value: '', label: t('app.planning.baseline_none') }, ...kept.map((each) => ({ value: each.id, label: t('app.planning.baseline_named', { name: each.name }) }))]}
          onchange={(event: Event) => (setup = { ...setup, baselineId: (event.currentTarget as HTMLSelectElement).value })} />
      {/if}
      <span class="grow-bar"></span>
      <Button size="sm" tone="subtle" icon="download" onclick={() => void exportAs('csv')}>{t('app.planning.export')}</Button>
      <Button size="sm" tone="secondary" icon="sliders-horizontal" data-plan-view-open data-opener="view" onclick={() => (isPanelOpen = true)}>{panelOpener}</Button>
      {@render acceptChosen()}
    </div>
  {:else if part === 'dependencies'}
    <div class="controls" data-fill>
      <span class="quiet grow-bar" data-dependencies-said>{t('app.planning.dependencies_said', { count: String(schedule?.dependencies.length ?? 0) })}</span>
      <!-- One primary: while a dependency's timing is open, its *Übernehmen* is the one. -->
      {@render addOpener(timingOpen === undefined)}
    </div>
  {/if}
{/snippet}

<!-- What is seen, as the chips a collection draws over its list, and *Filter* to the panel (§6.11). -->
{#snippet queryLine()}
  <div class="query-line" data-plan-query>
    <QueryChips line={setup.line} {names} onchange={(next) => (setup = { ...setup, line: next })} />
    <Button size="sm" tone="secondary" icon="funnel" data-opener="view" aria-expanded={isPanelOpen} onclick={() => (isPanelOpen = true)}>{t('app.planning.filter')}</Button>
  </div>
{/snippet}

{#snippet outlineCell(id: string, column: string)}
  {@const row = byId.get(id)}
  {#if row}
    {#if column === 'start'}{row.kind === 'milestone' ? shortDay(row.due) : shortDay(row.start)}
    {:else if column === 'finish'}{row.kind === 'milestone' ? '' : shortDay(row.due)}
    {:else if column === 'duration'}{row.kind === 'milestone' ? '' : t('app.planning.days_short', { days: String(row.entry.duration_days) })}
    {:else if column === 'float'}{row.kind === 'summary' ? '' : t('app.planning.days_short', { days: String(row.entry.total_float_days) })}
    {:else if column === 'slip'}{@const slip = slipOf(id)}{slip ? (slip > 0 ? `+${slip}` : String(slip)) : '–'}
    {:else if column === 'assignee'}{@const who = assigneeOf.get(id)}{#if who && row.kind !== 'summary' && initialsOf(who)}<span class="initials" title={accounts.nameOf(who)}>{initialsOf(who)}</span>{/if}
    {/if}
  {/if}
{/snippet}

{#snippet giveDates(id: string)}
  {#if undatedIds.has(id) && mayWrite}
    <Button size="sm" tone="subtle" aria-label={t('app.planning.give_dates_of', { title: titleOfUndated(id) })} onclick={() => openEntry(id)}>
      {t('app.planning.give_dates')}
    </Button>
  {/if}
{/snippet}

{#snippet rowAction(id: string)}
  {@const entry = entryOf(id)}
  {@render giveDates(id)}
  {#if entry}
    <EntryMenu
      bind:this={menus[id]}
      title={entry.title}
      items={menuOf(entry)}
      onselect={(chosen) => void entryActions?.run(entry, chosen)}
      onopening={() => entryActions?.readLevel(entry)}
    />
  {/if}
{/snippet}

{#snippet planTable()}
  <DataTable label={t('app.planning.table')} {columns} {rows} key={(row: PlanRow) => row.id} rowLabel={(row: PlanRow) => row.title}>
    {#snippet cell(row: PlanRow, column: DataColumn)}
      {#if column.id === 'entry'}
        <span class="what" data-depth={String(Math.min(row.depth, 4))} data-done={row.entry.done ? '' : undefined}>
          {#if row.entry.milestone}<span class="diamond" aria-hidden="true">◆</span>{/if}
          <a href={`/items/${row.id}`}>{row.title}</a>
          {#if row.entry.external}<span class="note">{t('app.planning.outside')}</span>{/if}
          {#if row.entry.done}<StateLozenge label={t('app.planning.done')} tone="success" size="sm" />{/if}
          {#if row.isCritical}<StateLozenge label={t('app.planning.critical')} tone="danger" size="sm" />{/if}
        </span>
      {:else if column.id === 'start'}
        {#if row.entry.moved}<span class="moved" title={t('app.planning.proposed')}>{dayLabel(row.start)}</span><span class="note">{t('app.planning.proposed')}</span>{:else}{dayLabel(row.start)}{/if}
      {:else if column.id === 'finish'}
        {dayLabel(row.due)}
      {:else if column.id === 'duration'}
        <span class="number">{row.entry.milestone ? '–' : row.entry.duration_days}</span>
      {:else if column.id === 'float'}
        <span class="number" data-tight={row.entry.total_float_days <= 0 ? '' : undefined}>{row.entry.milestone ? '–' : t('app.planning.days_short', { days: String(row.entry.total_float_days) })}</span>
      {:else if column.id === 'slip'}
        {@const slip = slipOf(row.id)}
        <span class="number" data-late={slip && slip > 0 ? '' : undefined}>{slip === undefined || slip === null || slip === 0 ? '–' : slip > 0 ? `+${t('app.planning.days_short', { days: String(slip) })}` : String(slip)}</span>
      {:else if column.id === 'mode'}
        {#if mayWrite && !row.entry.external}
          {@const chosenMode = modeDraft[row.id] ?? row.entry.mode}
          <!-- Chosen, then applied: a closed native select changes on every arrow key, and
               each change would write and reschedule the plan (SC 3.2.2). -->
          <Inline gap="050" align="end">
            <Select
              label={t('app.planning.mode_of', { title: row.title })}
              isLabelHidden
              size="sm"
              value={chosenMode}
              options={modes}
              onchange={(event: Event) => (modeDraft[row.id] = (event.currentTarget as HTMLSelectElement).value as 'MANUAL' | 'AUTOMATIC')}
            />
            {#if chosenMode !== row.entry.mode}
              <Button
                size="sm"
                tone="secondary"
                aria-label={t('app.planning.mode_apply_of', { title: row.title })}
                onclick={() => {
                  const mode = chosenMode;
                  void act(() => planning.setScheduling(planId, zone, row.id, { mode })).then(() => delete modeDraft[row.id]);
                }}
              >{t('app.planning.mode_apply')}</Button>
            {/if}
          </Inline>
        {:else}
          {t(`app.planning.mode_${row.entry.mode}`)}
        {/if}
      {:else if column.id === 'milestone'}
        {#if mayWrite && !row.entry.external}
          <Checkbox
            label={t('app.planning.milestone_of', { title: row.title })}
            isLabelHidden
            checked={row.entry.milestone}
            onchange={(event: Event) => {
              const milestone = (event.currentTarget as HTMLInputElement).checked;
              void act(() => planning.setScheduling(planId, zone, row.id, { milestone }));
            }}
          />
        {:else}
          {row.entry.milestone ? t('app.planning.milestone') : ''}
        {/if}
      {:else if column.id === 'move'}
        {#if mayWrite && proposed.includes(row.id)}
          <Checkbox
            label={t('app.planning.accept_of', { title: row.title })}
            isLabelHidden
            checked={accepted.includes(row.id)}
            onchange={(event: Event) => {
              const isOn = (event.currentTarget as HTMLInputElement).checked;
              accepted = isOn ? [...accepted, row.id] : accepted.filter((id) => id !== row.id);
            }}
          />
        {/if}
      {/if}
    {/snippet}
  </DataTable>
{/snippet}

{#snippet dependencyTable()}
    <DataTable
      label={t('app.timeline.dependencies')}
      columns={dependencyColumns}
      rows={schedule?.dependencies ?? []}
      key={(each: PlanDependency) => each.link_id}
      rowLabel={(each: PlanDependency) => titleOf(each.successor_id)}
      menu={dependencyMenu}
      onmenu={(each: PlanDependency) => removeDependency(each.link_id, each.successor_id, each.predecessor_id)}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      actionsLabel={t('app.planning.col_remove')}
    >
      {#snippet cell(each: PlanDependency, column: DataColumn)}
        {#if column.id === 'waiting'}{titleOf(each.successor_id)}
        {:else if column.id === 'arrow'}<span class="arrow" aria-hidden="true"><Icon name="chevron-left" size="sm" /></span>
        {:else if column.id === 'waited'}{titleOf(each.predecessor_id)}
        {:else if mayWrite}{@render timingVerb(each)}
        {:else}<span class="chip" data-broken={broken.has(`${each.predecessor_id}>${each.successor_id}`) ? '' : undefined}>{timingSaid(each)}</span>{/if}
      {/snippet}
    </DataTable>
{/snippet}

<!-- A dependency's kind and lag: two values of one thing, a popover's rung (§5.1); a bottom sheet
     on a phone. -->
{#snippet timingVerb(each: PlanDependency)}
  {@const draft = timing[each.link_id]}
  {@const name = titleOf(each.successor_id)}
  {#if draft}
    {#if viewport.isCompact}
      <button type="button" class="chip" data-timing-chip={each.link_id} data-broken={broken.has(`${each.predecessor_id}>${each.successor_id}`) ? '' : undefined} aria-label={t('app.planning.timing_of', { title: name })} onclick={() => (timingOpen = each.link_id)}>{timingSaid(each)}</button>
      <Sheet title={t('app.planning.timing_title', { title: name })} closeLabel={t('app.page.sheet_close')} grabLabel={t('app.page.sheet_grab')}
        bind:isOpen={() => timingOpen === each.link_id, (open) => (timingOpen = open ? each.link_id : undefined)}>
        {@render timingFields(each.link_id, name, each)}
      </Sheet>
    {:else}
      <Popover label={t('app.planning.timing_title', { title: name })} placement={{ side: 'block-end', align: 'end' }}
        bind:isOpen={() => timingOpen === each.link_id, (open) => (timingOpen = open ? each.link_id : undefined)}>
        {#snippet trigger(props)}
          <button type="button" class="chip" data-timing-chip={each.link_id} data-broken={broken.has(`${each.predecessor_id}>${each.successor_id}`) ? '' : undefined} aria-label={t('app.planning.timing_of', { title: name })} {...props}>{timingSaid(each)}</button>
        {/snippet}
        {@render timingFields(each.link_id, name, each)}
      </Popover>
    {/if}
  {/if}
{/snippet}

{#snippet timingFields(linkId: string, name: string, each: PlanDependency)}
  {@const draft = timing[linkId]}
  {#if draft}
    <div class="timing" data-timing={linkId}>
      <p class="label-quiet">{t('app.planning.timing')}</p>
      <div class="kinds" role="radiogroup" aria-label={t('app.planning.kind_of', { title: name })}>
        {#each kinds as kind (kind.value)}
          <button type="button" role="radio" class="kind" aria-checked={draft.kind === kind.value} data-kind={kind.value} onclick={() => (draft.kind = kind.value)}>{kind.label}</button>
        {/each}
      </div>
      <Input label={t('app.planning.lag_days')} size="sm" bind:value={draft.lag} inputmode="numeric" hint={t('app.planning.lag_hint')} error={lagErrors[linkId]} />
      <Inline justify="end">
        <Button size="sm" tone="subtle" onclick={() => { timingOpen = undefined; removeDependency(linkId, each.successor_id, each.predecessor_id); }}>{t('app.planning.remove')}</Button>
        <Button size="sm" tone="primary" onclick={() => saveTimingAndClose(linkId)}>{t('app.planning.save_timing')}</Button>
      </Inline>
    </div>
  {/if}
{/snippet}

{#if isAdding}
  <!-- Four fields are worked in, not answered: an M, the whole screen on a phone (D-1). -->
  <Dialog bind:isOpen={isAdding} title={t('app.planning.add_dependency')} dismissLabel={t('app.links.cancel')} size="m" primary={{ label: t('app.planning.add_dependency'), onclick: addDependency }}>
    <div class="fields" data-add-dependency>
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      <Select label={t('app.planning.new_waiting')} placeholder={t('app.planning.choose_entry')} bind:value={newWaiting} options={linkable} />
      <Select label={t('app.planning.new_waited')} placeholder={t('app.planning.choose_entry')} bind:value={newWaited} options={linkable.filter((each) => each.value !== newWaiting)} />
      <!-- The time relation as the chip's popover asks it: the kind as four choices, the lag typed. -->
      <p class="label-quiet">{t('app.planning.timing')}</p>
      <div class="kinds" role="radiogroup" aria-label={t('app.planning.col_kind')}>
        {#each kinds as kind (kind.value)}
          <button type="button" role="radio" class="kind" aria-checked={newKind === kind.value} data-kind={kind.value} onclick={() => (newKind = kind.value)}>{kind.label}</button>
        {/each}
      </div>
      <Input label={t('app.planning.lag_days')} bind:value={newLag} inputmode="numeric" hint={t('app.planning.lag_hint')} error={newLagError} />
    </div>
    {#snippet actions()}<Button tone="secondary" onclick={() => (isAdding = false)}>{t('app.links.cancel')}</Button>{/snippet}
  </Dialog>
{/if}

{#if isUndatedOpen}
  <Sheet title={t('app.timeline.undated')} closeLabel={t('app.page.sheet_close')} grabLabel={t('app.page.sheet_grab')} bind:isOpen={isUndatedOpen}>
    <ul class="plain" data-undated>
      {#each undated as each (each.item_id)}
        <li class="baseline">
          <a class="grow" href={`/items/${each.item_id}`}>{each.title}</a>
          {@render giveDates(each.item_id)}
        </li>
      {/each}
    </ul>
  </Sheet>
{/if}

{#if isImporting}
  <Dialog bind:isOpen={isImporting} title={t('app.planning.import')} dismissLabel={t('app.links.cancel')} size="m">
    <PlanImport collectionId={planId} {zone} ondone={() => (isImporting = false)} />
  </Dialog>
{/if}

{#if isTaking}
  <Dialog bind:isOpen={isTaking} title={t('app.planning.take_baseline')} dismissLabel={t('app.links.cancel')} size="s" primary={{ label: t('app.planning.take'), onclick: take }}>
    <Stack gap="150">
      <p class="quiet">{t('app.planning.take_baseline_hint')}</p>
      <Input label={t('app.planning.baseline_name')} bind:value={baselineName} maxlength={120} />
    </Stack>
    {#snippet actions()}<Button tone="secondary" onclick={() => (isTaking = false)}>{t('app.links.cancel')}</Button>{/snippet}
  </Dialog>
{/if}

{#if isSetting}
  <Dialog bind:isOpen={isSetting} title={t('app.planning.settings')} dismissLabel={t('app.links.cancel')} size="m">
    <Stack gap="300">
      <section class="block" aria-labelledby="plan-tolerance" data-plan-tolerance>
        <h2 id="plan-tolerance" class="heading">{t('app.planning.tolerance')}</h2>
        {#if tolerance}
          <p class="quiet">
            {tolerance.time_tolerance_days === null ? t('app.planning.tolerance_none') : t('app.planning.tolerance_days', { days: String(tolerance.time_tolerance_days) })}
            {#if tolerance.time_tolerance_days !== null} · {t('app.planning.tolerance_from', { level: t(`app.planning.tolerance_level_${tolerance.source}`) })}{/if}
            {#if tolerance.locked_by} · {t('app.planning.tolerance_locked', { level: t(`app.planning.tolerance_level_${tolerance.locked_by}`) })}{/if}
          </p>
          {#if tolerance.exception && !tolerance.exception.exceeded}
            <p class="quiet">{t('app.planning.within_tolerance', { forecast: dayLabel(tolerance.exception.forecast_finish), agreed: dayLabel(tolerance.exception.agreed_finish) })}</p>
          {/if}
          {#if mayShape && !lockedAbove}
            <div class="fields">
              <Input label={t('app.planning.tolerance_in_days')} type="number" min="0" max="365" bind:value={toleranceDraft} />
              {#if place?.type === 'HUB'}<Checkbox label={t('app.planning.tolerance_lock')} bind:checked={toleranceLock} />{/if}
            </div>
          {/if}
        {:else}
          <Skeleton lines={2} />
        {/if}
      </section>
      <section class="block" aria-labelledby="plan-baselines" data-plan-baselines>
        <h2 id="plan-baselines" class="heading">{t('app.planning.baselines')}</h2>
        {#if kept.length === 0}
          <p class="quiet">{t('app.planning.baselines_none')}</p>
        {:else}
          <ul class="plain">
            {#each kept as each (each.id)}
              <li class="baseline" data-baseline={each.id}>
                <span class="grow">
                  <span>{each.name}</span>
                  <span class="quiet">{t('app.planning.baseline_kept_on', { date: formatDay(each.taken_at, messages.locale), count: String(each.entry_count) })}</span>
                </span>
                {#if mayWrite}
                  <IconButton size="sm" icon="trash" label={t('app.planning.delete_baseline_of', { name: each.name })} onclick={() => deleteBaseline(each.id, each.name)} />
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </Stack>
    {#snippet actions()}
      {#if tolerance && mayShape && !lockedAbove}<Button tone="subtle" onclick={() => void saveTolerance(true)}>{t('app.planning.tolerance_inherit')}</Button>{/if}
      <Button tone="secondary" onclick={() => (isSetting = false)}>{t('app.links.cancel')}</Button>
      {#if tolerance && mayShape && !lockedAbove}<Button tone="primary" onclick={() => void saveTolerance(false)}>{t('app.planning.tolerance_set')}</Button>{/if}
    {/snippet}
  </Dialog>
{/if}

<EntryActions bind:this={entryActions} onopen={openEntry} {onnavigate} />


<style>
  .beside-pane { display: flex; align-items: flex-start; }
  .beside-pane > :global(:first-child) { flex: 1; min-width: 0; }
  .bars { display: flex; flex-direction: column; gap: var(--sp-150); inline-size: 100%; }
  .controls { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100) var(--sp-150); }
  .grow-bar { flex: 1; }
  .query-line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); min-inline-size: 0; }
  /* The tab's only line: its last acts stand at the end, as the head's do. */
  .controls[data-fill] { flex: 1 1 auto; }
  .legend { margin-inline-start: auto; }
  .segments { display: inline-flex; align-items: center; border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-md); overflow: hidden; background: var(--bg-surface); }
  .segments button { display: inline-flex; align-items: center; gap: var(--sp-050); min-block-size: var(--layout-target); padding-inline: var(--sp-150); font: inherit; font-size: var(--fs-075); color: var(--text-primary); background: transparent; border: none; cursor: pointer; }
  .quiet-segments { background: var(--bg-surface-sunken); border-color: transparent; }
  /* One selected look for every segmented control: the density switch's white chip (T-2). */
  .quiet-segments button[aria-pressed='true'] { color: var(--text-primary); background: var(--bg-surface); font-weight: var(--fw-semibold); }
  .initials { display: inline-flex; align-items: center; justify-content: center; inline-size: var(--sp-300); block-size: var(--sp-300); border-radius: var(--r-full); font-size: var(--fs-065); font-weight: var(--fw-semibold); color: var(--text-brand); background: var(--accent-primary-subtle); }
  .warn { color: var(--text-danger); }
  .band { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); padding: var(--sp-200); border-radius: var(--r-md); background: var(--status-warning-surface); border: var(--bw-hairline) solid var(--status-warning-border); }
  .band-text { flex: 1; min-inline-size: 0; margin: 0; font-size: var(--fs-075); }
  .band.danger { align-items: flex-start; background: var(--status-danger-surface); border-color: var(--status-danger-border); color: var(--text-primary); }
  .band.danger .band-text { display: flex; flex-direction: column; gap: var(--sp-050); }
  .conflict { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .chip { display: inline-flex; align-items: center; min-block-size: var(--sp-300); padding-inline: var(--sp-100); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-sm); font: inherit; font-size: var(--fs-075); color: var(--text-primary); background: var(--bg-surface); white-space: nowrap; }
  button.chip { cursor: pointer; }
  .chip[data-broken] { color: var(--status-danger-text); border-color: var(--status-danger-border); background: var(--status-danger-surface); }
  .arrow { display: inline-flex; color: var(--text-subtle); }
  .kinds { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .kind { min-block-size: var(--layout-target); padding-inline: var(--sp-150); border: var(--bw-hairline) solid var(--border-strong); border-radius: var(--r-md); font: inherit; font-size: var(--fs-075); color: var(--text-primary); background: var(--bg-surface); cursor: pointer; }
  .kind[aria-checked='true'] { color: var(--text-brand); border-color: var(--accent-primary); background: var(--accent-primary-subtle); }
  .label-quiet { margin: 0; color: var(--text-subtle); font-size: var(--fs-065); font-weight: var(--fw-semibold); text-transform: uppercase; }
  .band-mark { color: var(--text-secondary); }
  .baseline { display: flex; align-items: center; gap: var(--sp-100); }
  .grow { display: flex; flex: 1; flex-direction: column; min-inline-size: 0; }
  .window { flex: 1; min-width: 0; }
  .split { display: flex; align-items: flex-start; gap: var(--sp-300); min-width: 0; }
  .block { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }
  .fields { display: grid; gap: var(--sp-150); }
  .timing { display: grid; gap: var(--sp-150); padding: var(--sp-150); min-inline-size: var(--layout-field-sm); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .plain { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-050); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; }
  .what { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }
  .what a { color: var(--text-primary); text-decoration: none; }
  .what a:hover { text-decoration: underline; }
  .what[data-done] a { color: var(--text-secondary); }
  .diamond { color: var(--accent-signature); font-size: var(--fs-065); }
  .moved { color: var(--text-brand); font-weight: var(--fw-semibold); }
  .what[data-depth='1'] { padding-inline-start: var(--sp-150); }
  .what[data-depth='2'] { padding-inline-start: var(--sp-300); }
  .what[data-depth='3'] { padding-inline-start: var(--sp-400); }
  .what[data-depth='4'] { padding-inline-start: var(--sp-500); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .what a { position: relative; }
    .what a::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
