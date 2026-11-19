<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Sprints* of a collection (M2, UC-PLN-06, UC-PLN-07), as scene `sprint` draws it: the chosen
  // sprint as one bar - goal, days left, points, a small burndown, *close sprint* -, a filter line,
  // and the board's columns, which carry the workflow's statuses but the backlog's, each with its
  // count and points, done folded narrow; on a phone the bar a card and one column under a strip.
  // The backlog is its own tab, never a column of the sprint (§6.2, E-22). The burndown's press
  // unfolds it large, with its table and the velocity (user-experience.md §7.5).
  //
  // **The server counts; this screen draws and writes.** Commitment, result, burndown and velocity
  // are the answers of `/sprints/{id}` and `/collections/{id}/velocity`; every start, close, move
  // or estimate is a write, and the sprint is read again after it.
  //
  // **The controls are the keyboard path** (UC-PLN-07 check 5): the burndown's picture has a table
  // of the same days, and every card's points chip and move is a real control.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS at the collection the
  // screen shows values, not controls.
  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    BucketColumn, Button, Dialog, Icon, IconButton, Inline, Input, LabelChip, Menu, Select, Skeleton, Stack, Textarea,
    DataTable, ViewSwitcher, VisuallyHidden, WorkPage, type DataColumn, type MenuItem, type View,
  } from '@hubtask/design-system/components';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { Sprint, SprintDetail, SprintEntry, SprintList, Velocity, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { buckets } from '../lib/data/buckets.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { labels } from '../lib/data/labels.svelte.ts';
  import { boardShapeIn, workflows, workflowsIn } from '../lib/data/workflows.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import {
    burndownLine, burndownTop, carryTargets, entriesToRead, sprintColumns, sprintDays, sprintPath,
    sprints, sprintShown, sprintsPath, valueOf, velocityPath, type EstimateInput,
  } from '../lib/data/sprints.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { formatPlainDay, formatShortDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { ticketKeyOf } from '../lib/data/ticket.ts';
  import CollectionHead from '../lib/workspace/CollectionHead.svelte';
  import DueMark from '../lib/entries/DueMark.svelte';
  import PointsChip from '../lib/planning/PointsChip.svelte';
  import EntryKey from '../lib/entries/EntryKey.svelte';
  import PeopleMarks from '../lib/people/PeopleMarks.svelte';
  import CreateDialog from '../lib/product/CreateDialog.svelte';
  import { createPlaces } from '../lib/product/places.svelte.ts';

  interface Props {
    /** The collection the sprints belong to. */
    collectionId: string;
    onnavigate: (path: string) => void;
    /** The home's create act asked for the create dialog (user-experience.md §5.2): opened once. */
    isCreateAsked?: boolean;
  }

  const { collectionId, onnavigate, isCreateAsked = false }: Props = $props();

  const zone = $derived(actor.zone);
  const place = $derived(containers.find(collectionId));
  const rolePath = $derived({ hubId: place?.parent_id ?? undefined, collectionId });

  $effect(() => frame.entitle(place ? t('app.sprints.title_of', { place: place.name }) : t('app.sprints.title')));
  $effect(() => {
    const path = rolePath;
    untrack(() => people.open(path));
  });

  const mayWrite = $derived(holds(people.roleOf(rolePath, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted');

  // Opened once, when the reader's role here has answered that they may write.
  let isCreateOpened = false;
  $effect(() => {
    if (!isCreateAsked || isCreateOpened || !mayWrite) return;
    isCreateOpened = true;
    untrack(() => openPlan(undefined));
  });

  let list = $state<Sprint[] | undefined>(undefined);
  let velocity = $state<Velocity | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const paths = [sprintsPath(collectionId), velocityPath(collectionId)] as const;
    return untrack(() => {
      const stops = [
        engine.subscribe<SprintList>({ path: paths[0] }, (next) => {
          if (next.status === 'ready') { list = next.data.data; refusal = undefined; }
          if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
        }),
        engine.subscribe<Velocity>({ path: paths[1] }, (next) => { if (next.status === 'ready') velocity = next.data; }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  let chosenId = $state('');
  const chosen = $derived(list?.find((s) => s.id === chosenId) ?? (list ? sprintShown(list) : undefined));
  let detail = $state<SprintDetail | undefined>(undefined);
  $effect(() => {
    const id = chosen?.id;
    detail = undefined;
    if (!id) return;
    const path = sprintPath(id, zone);
    return untrack(() => engine.subscribe<SprintDetail>({ path }, (next) => { if (next.status === 'ready') detail = next.data; }));
  });

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

  /** An estimate saved from a card's points chip (UC-PLN-07 check 5). */
  function saveEstimate(itemId: string, title: string, input: EstimateInput | null) {
    void act(() => sprints.estimate(collectionId, zone, chosen?.id, itemId, input), t('app.sprints.estimated', { title }));
  }
  /** A card back to the backlog. */
  function toBacklog(itemId: string) {
    const sprint = chosen;
    if (!sprint) return;
    void act(() => sprints.take(collectionId, zone, sprint.id, [itemId]));
  }

  let isPlanning = $state(false);
  /** The sprint the dialog changes; undefined while it plans a new one. */
  let editing = $state<Sprint | undefined>(undefined);
  const draft = $state({ name: '', goal: '', start_on: '', end_on: '', unit: 'POINTS' as 'POINTS' | 'HOURS' });
  /** Where a new sprint is planned (§5.2): this collection, or another where the plans are on. */
  let draftCollection = $state('');
  const placesToPlan = createPlaces('PLANNING', 'COLLECTION', () => isPlanning && !editing);
  function openPlan(sprint: Sprint | undefined) {
    editing = sprint;
    draft.name = sprint?.name ?? '';
    draft.goal = sprint?.goal ?? '';
    draft.start_on = sprint?.start_on ?? today;
    draft.end_on = sprint?.end_on ?? '';
    draft.unit = sprint?.unit ?? 'POINTS';
    draftCollection = collectionId;
    isPlanning = true;
  }
  function plan() {
    const fields = { ...draft, name: draft.name.trim() };
    if (!fields.name || !fields.start_on || !fields.end_on) return;
    isPlanning = false;
    const changing = editing;
    if (changing) {
      const { unit: _unit, ...rest } = fields;
      const change = changing.state === 'ACTIVE' ? { name: rest.name, goal: rest.goal, end_on: rest.end_on } : { ...rest, unit: fields.unit };
      void act(() => sprints.update(collectionId, zone, changing, change), t('app.sprints.changed', { name: fields.name }));
      return;
    }
    const at = draftCollection || collectionId;
    void act(async () => {
      const made = await sprints.create(at, zone, fields);
      if (at === collectionId) chosenId = made.id;
      else onnavigate(`/collections/${encodeURIComponent(at)}/sprints`);
    }, t('app.sprints.planned', { name: fields.name }));
  }

  let isClosing = $state(false);
  let carryTo = $state('');
  const targets = $derived(list && chosen ? carryTargets(list, chosen.id) : []);
  function close() {
    const sprint = chosen;
    if (!sprint) return;
    isClosing = false;
    void act(() => sprints.close(collectionId, zone, sprint.id, carryTo || undefined), t('app.sprints.closed', { name: sprint.name }));
  }

  const open = $derived((detail?.entries ?? []).filter((e) => !e.removed_at));
  // *In progress* is its own lane while the sprint runs or is planned (§7.5); a closed sprint keeps
  // what was left open in one lane with what became of it.
  const hasStartedLane = $derived(chosen?.state !== 'CLOSED');
  const toDo = $derived(open.filter((e) => !e.done && !(hasStartedLane && e.started)));
  const inProgress = $derived(hasStartedLane ? open.filter((e) => !e.done && e.started) : []);
  const done = $derived(open.filter((e) => e.done));
  // The sprint's columns are the board's, which carry the workflow's statuses but the backlog's
  // (§6.2, E-22): each entry in the column it stands in. Without a board, or for a closed sprint
  // whose entries have moved on, to do, in progress and done.
  $effect(() => {
    const id = collectionId;
    return untrack(() => {
      const stops = [buckets.open(id), items.openCollection(id), labels.open(id), workflows.open()];
      return () => stops.forEach((stop) => stop());
    });
  });
  const workflow = $derived(boardShapeIn(workflows.list, collectionId, place?.parent_id ?? undefined));
  const workflowNames = $derived(workflowsIn(workflows.list, collectionId, place?.parent_id ?? undefined).map((each) => each.name ?? each.key));
  const isLevelRead = $derived(buckets.stateOf(collectionId)?.status === 'ready' && items.stateOf(`container:${collectionId}`)?.status === 'ready');
  // The collection's level is a page; an open entry beyond it is read on its own, so every entry
  // of the sprint stands in its column and its card carries its key, labels and people.
  let beyond = $state<Record<string, WorkItem | null>>({});
  let reading = $state(0);
  const toRead = $derived(isLevelRead && hasStartedLane ? entriesToRead(open, (itemId) => itemId in beyond || items.find(itemId) !== undefined) : []);
  $effect(() => {
    const ids = toRead;
    if (ids.length === 0) return;
    untrack(() => {
      reading += 1;
      void Promise.all(ids.map((itemId) => engine.refresh<WorkItem>({ path: `/items/${itemId}` }).then((read) => [itemId, read.status === 'ready' ? read.data : null] as const)))
        .then((pairs) => { beyond = { ...beyond, ...Object.fromEntries(pairs) }; })
        .catch(() => { beyond = { ...beyond, ...Object.fromEntries(ids.map((itemId) => [itemId, null])) }; })
        .finally(() => { reading -= 1; });
    });
  });
  const itemOf = (itemId: string): WorkItem | undefined => items.find(itemId) ?? beyond[itemId] ?? undefined;
  const isPlaced = $derived(isLevelRead && toRead.length === 0 && reading === 0);
  const columns = $derived(
    hasStartedLane && isPlaced
      ? sprintColumns(open, buckets.of(collectionId), (itemId) => itemOf(itemId)?.bucket_id, t('app.board.unmapped'), workflow)
      : undefined,
  );

  // The filter line (scene sprint): who it is assigned to, *only mine*, and words of the key or title.
  let assignee = $state('');
  let words = $state('');
  const me = $derived(actor.account?.id ?? '');
  const assignees = $derived([...new Set(open.map((entry) => itemOf(entry.item_id)?.assignee_id).filter((id): id is string => Boolean(id)))]);
  const assigneeOptions = $derived([
    { value: '', label: t('app.sprints.assignee_all') },
    ...assignees.map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  ]);
  const isFiltered = $derived(assignee !== '' || words.trim() !== '');
  const keyOf = (itemId: string) => ticketKeyOf(place, itemOf(itemId)?.number) ?? '';
  function isKept(entry: SprintEntry): boolean {
    if (assignee && itemOf(entry.item_id)?.assignee_id !== assignee) return false;
    const wanted = words.trim().toLocaleLowerCase();
    return !wanted || entry.title.toLocaleLowerCase().includes(wanted) || keyOf(entry.item_id).toLocaleLowerCase().includes(wanted);
  }

  type Lane = { key: string; name: string; isDone: boolean; isAside: boolean; entries: readonly SprintEntry[]; empty: string };
  const lanes = $derived<Lane[]>(
    (columns
      ? columns.map((column) => ({ key: column.id ?? 'none', name: column.name, isDone: column.isDone, isAside: column.isAside === true, entries: column.entries, empty: t('app.sprints.column_empty') }))
      : [
          { key: 'todo', name: t('app.sprints.todo_column'), isDone: false, isAside: false, entries: toDo, empty: chosen?.state === 'CLOSED' ? t('app.sprints.todo_none_closed') : t('app.sprints.todo_empty') },
          ...(hasStartedLane ? [{ key: 'in-progress', name: t('app.sprints.in_progress_column'), isDone: false, isAside: false, entries: inProgress, empty: t('app.sprints.in_progress_empty') }] : []),
          { key: 'done', name: t('app.sprints.done_column'), isDone: true, isAside: false, entries: done, empty: t('app.sprints.done_empty') },
        ]
    ).map((lane) => (isFiltered ? { ...lane, entries: lane.entries.filter(isKept), empty: t('app.sprints.filtered_empty') } : lane)),
  );
  // On a phone one column at a time, the strip above saying which and how many (§7, scene sprint 4).
  const isOneColumn = $derived(viewport.isCompact);
  // The done column starts folded, as on the board (scene sprint), and so does the column-less
  // one beside it: a closed sprint keeps done open, its result is what it shows.
  let unfolded = $state<Record<string, boolean>>({});
  const isFoldedLane = (lane: { key: string; isDone: boolean; isAside: boolean }) => (lane.isDone || lane.isAside) && hasStartedLane && !isOneColumn && !unfolded[lane.key];
  let shownLaneId = $state<string | undefined>(undefined);
  const shownLane = $derived(lanes.some((lane) => lane.key === shownLaneId) ? (shownLaneId as string) : (lanes.find((lane) => !lane.isDone && lane.entries.length > 0) ?? lanes[0])?.key ?? '');
  // The phone's strip names the open columns; done is reached by its own segment at the end.
  const stripViews = $derived<View[]>(lanes.map((lane) => ({ id: lane.key, label: t('app.sprints.strip_column', { name: lane.name, count: String(lane.entries.length) }) })));
  const noteOf = (entry: SprintEntry) =>
    entry.outcome ? t(`app.sprints.outcome_${entry.outcome}`) : !entry.committed && chosen?.state !== 'PLANNED' ? t('app.sprints.added_later') : undefined;
  const days = $derived(chosen ? sprintDays(chosen) : 0);
  const top = $derived(chosen && detail ? burndownTop(chosen, detail.burndown) : 0);
  const unitLabel = $derived(chosen ? t(`app.sprints.unit_${chosen.unit}`) : '');
  const unitShort = $derived(chosen ? t(`app.sprints.unit_short_${chosen.unit}`) : '');
  // The chooser names the sprints alone: the chosen one's state is the chip beside its name.
  const states = $derived(list ? list.map((s) => ({ value: s.id, label: s.name })) : []);

  const burndownColumns = $derived<DataColumn[]>([
    { id: 'day', label: t('app.sprints.col_day'), priority: 1 },
    { id: 'remaining', label: t('app.sprints.col_remaining'), align: 'end', priority: 1, isStatus: true },
    { id: 'ideal', label: t('app.sprints.col_ideal'), align: 'end', priority: 2 },
    { id: 'scope', label: t('app.sprints.col_scope'), align: 'end', priority: 3 },
  ]);
  const velocityColumns = $derived<DataColumn[]>([
    { id: 'sprint', label: t('app.sprints.col_sprint'), priority: 1 },
    { id: 'committed', label: t('app.sprints.col_committed'), align: 'end', priority: 2 },
    { id: 'completed', label: t('app.sprints.col_completed'), align: 'end', priority: 1, isStatus: true },
  ]);
  type Point = NonNullable<typeof detail>['burndown'][number];
  type Past = NonNullable<typeof velocity>['sprints'][number];
  const today = $derived(todayIn(zone));

  const startSprint = (sprint: Sprint) => void act(() => sprints.start(collectionId, zone, sprint.id), t('app.sprints.started', { name: sprint.name }));
  const askClose = () => { carryTo = targets[0]?.id ?? ''; isClosing = true; };
  /** A column's points: its entries' estimates in the sprint's unit (UC-PLN-07 check 2). */
  const columnFigure = (entries: readonly SprintEntry[]) => (chosen ? t('app.sprints.column_value', { value: String(valueOf(entries, chosen.unit)), unit: unitShort }) : '');
  const daysLeft = $derived(chosen ? Math.max(0, Math.round((Date.parse(`${chosen.end_on}T00:00:00Z`) - Date.parse(`${today}T00:00:00Z`)) / 86_400_000) + 1) : 0);
  const lastPoint = $derived(detail?.burndown.at(-1));
  const doneShort = $derived(
    chosen
      ? t('app.sprints.done_short', {
          done: String(lastPoint ? lastPoint.scope - lastPoint.remaining : valueOf(done, chosen.unit)),
          scope: String(lastPoint ? lastPoint.scope : valueOf(open, chosen.unit)),
          unit: unitShort,
        })
      : '',
  );
  const daysText = $derived(chosen?.state === 'ACTIVE' ? t('app.sprints.days', { count: String(daysLeft) }) : '');
  const spanText = $derived(chosen ? t('app.sprints.span_short', { from: formatShortDay(chosen.start_on, messages.locale), to: formatShortDay(chosen.end_on, messages.locale) }) : '');

  /** The burndown large, its table and the velocity: unfolded by the bar's burndown. */
  let isBurndownOpen = $state(false);

  // The bar's ⋯: the sprint's own verbs; on a phone, where the bar has no chooser and no button,
  // the other sprints and *close sprint* too.
  const barMenu = $derived<MenuItem[]>([
    ...(isOneColumn && mayWrite && chosen?.state === 'ACTIVE' ? [{ id: 'close', label: t('app.sprints.close'), icon: 'check' as const }] : []),
    ...(isOneColumn && mayWrite && chosen?.state === 'PLANNED' ? [{ id: 'start', label: t('app.sprints.start'), icon: 'play' as const }] : []),
    ...(isOneColumn && list ? list.filter((s) => s.id !== chosen?.id).map((s, at) => ({ id: `show:${s.id}`, label: t('app.sprints.show_sprint', { name: s.name }), hasSeparatorBefore: at === 0 && mayWrite })) : []),
    ...(mayWrite && chosen && chosen.state !== 'CLOSED' ? [{ id: 'edit', label: t('app.sprints.edit'), icon: 'pencil' as const, hasSeparatorBefore: isOneColumn }] : []),
    ...(mayWrite ? [{ id: 'plan', label: t('app.sprints.plan'), icon: 'plus' as const }] : []),
    ...(mayWrite && chosen?.state === 'PLANNED' ? [{ id: 'delete', label: t('app.sprints.delete'), icon: 'trash' as const, hasSeparatorBefore: true }] : []),
  ]);

  function onMenu(id: string) {
    const sprint = chosen;
    if (id === 'plan') openPlan(undefined);
    if (id.startsWith('show:')) chosenId = id.slice('show:'.length);
    if (!sprint) return;
    if (id === 'close') askClose();
    if (id === 'start') startSprint(sprint);
    if (id === 'edit') openPlan(sprint);
    if (id === 'delete') {
      chosenId = '';
      void act(() => sprints.remove(collectionId, zone, sprint.id), t('app.sprints.deleted'));
    }
  }
</script>

{#snippet sprintFigure()}
  <!-- The sprint's days in the collection's figure's place (R-11, scene sprint 1); the bar below
       holds the rest. -->
  {#if chosen}<span data-sprint-figure>{chosen.state === 'ACTIVE' ? `${chosen.name} · ${t('app.sprints.days_left', { count: String(daysLeft) })}` : `${chosen.name} · ${spanText}`}</span>{/if}
{/snippet}

{#snippet burndownPicture(size: 'small' | 'large')}
  {#if chosen && detail && detail.burndown.length > 0}
    <svg class={size === 'large' ? 'chart' : 'spark'} viewBox="0 0 400 100" preserveAspectRatio="none" role="img"
      aria-label={t(size === 'large' ? 'app.sprints.burndown_large_label' : 'app.sprints.burndown_label', { remaining: String(detail.burndown.at(-1)?.remaining ?? 0), unit: unitLabel })}>
      <polyline class="ideal" points={burndownLine(detail.burndown, 'ideal', days, top, 400, 100)} />
      <polyline class="scope" points={burndownLine(detail.burndown, 'scope', days, top, 400, 100)} />
      <polyline class="remaining" points={burndownLine(detail.burndown, 'remaining', days, top, 400, 100)} />
    </svg>
  {/if}
{/snippet}

{#snippet card(entry: SprintEntry, where: 'sprint' | 'done', note?: string)}
  {@const isMovable = mayWrite && chosen !== undefined && chosen.state !== 'CLOSED' && where !== 'done'}
  {@const item = itemOf(entry.item_id)}
  {@const carried = (item?.label_ids ?? []).map((labelId) => labels.of(collectionId).find((each) => each.id === labelId)).filter((label) => label !== undefined)}
  <li class="card" data-card={entry.item_id}>
    <!-- The board's card (scene sprint 3): the key and the person on top, the title, the labels,
         then the date and the points chip. -->
    <span class="card-top">
      <span class="card-key"><Icon name="square-check" size="sm" />{#if item}<EntryKey collection={place} number={item.number} />{/if}</span>
      {#if item?.assignee_id}<PeopleMarks assigneeId={item.assignee_id} />{/if}
    </span>
    <a class="card-title" href={`/items/${entry.item_id}`}>{entry.title}</a>
    {#if carried.length > 0}
      <span class="card-labels">{#each carried.slice(0, 2) as label (label.id)}<LabelChip name={label.name} colorToken={label.color_token} />{/each}</span>
    {/if}
    <span class="card-foot">
      {#if item}<DueMark {item} />{/if}
      <PointsChip title={entry.title} estimate={entry.estimate} isEditable={mayWrite && chosen?.state !== 'CLOSED'} onsave={(input) => saveEstimate(entry.item_id, entry.title, input)} />
      {#if note}<span class="note">{note}</span>{/if}
    </span>
    {#if isMovable}
      <!-- In the card's corner, shown on hover and focus: the keyboard's way back to the backlog. -->
      <span class="card-move"><IconButton size="sm" icon="arrow-left" label={t('app.sprints.to_backlog_of', { title: entry.title })} onclick={() => toBacklog(entry.item_id)} /></span>
    {/if}
  </li>
{/snippet}

{#snippet points(entries: readonly SprintEntry[])}
  <span class="points" data-points>{columnFigure(entries)}</span>
{/snippet}

{#snippet barMenuButton()}
  {#if barMenu.length > 0}
    <Menu label={t('app.sprints.more')} items={barMenu} placement={{ side: 'block-end', align: 'end' }} onselect={onMenu}>
      {#snippet trigger(props)}<IconButton icon="ellipsis" label={t('app.sprints.more')} tone={isOneColumn ? 'subtle' : 'secondary'} {...props} />{/snippet}
    </Menu>
  {/if}
{/snippet}

{#snippet sprintBar(sprint: Sprint)}
  <!-- One bar for the sprint (scene sprint 2): which, its goal, the days left, the points done, the
       burndown small, and its one act; on a phone a card of state, days and points, the goal and
       the burndown wide (scene sprint 4). -->
  <section class="sprint-bar" aria-label={t('app.sprints.bar')} data-sprint-bar data-phone={isOneColumn ? '' : undefined}>
    {#if isOneColumn}
      <span class="phone-line">
        <span class="state" data-state={sprint.state}>{t(`app.sprints.state_${sprint.state}`)}</span>
        <span class="phone-figure" data-sprint-done>{[sprint.state === 'ACTIVE' ? t('app.sprints.days_left', { count: String(daysLeft) }) : spanText, doneShort].join(' · ')}</span>
        {@render barMenuButton()}
      </span>
      {#if sprint.goal}<span class="phone-goal">{sprint.goal}</span>{/if}
      <button type="button" class="burndown-open" aria-expanded={isBurndownOpen} onclick={() => (isBurndownOpen = !isBurndownOpen)}>
        {@render burndownPicture('small')}<VisuallyHidden>{t('app.sprints.burndown')}</VisuallyHidden>
      </button>
    {:else}
      <span class="chooser">
        <Icon name="repeat" size="sm" />
        {#if list && list.length > 1}
          <Select label={t('app.sprints.sprint')} isLabelHidden size="sm" value={sprint.id} options={states}
            onchange={(event: Event) => { chosenId = (event.currentTarget as HTMLSelectElement).value; }} />
        {:else}
          <b class="sprint-name">{sprint.name}</b>
        {/if}
        <span class="state" data-state={sprint.state}>{t(`app.sprints.state_${sprint.state}`)}</span>
      </span>
      <dl class="facts">
        <div class="fact goal"><dt>{t('app.sprints.goal')}</dt><dd title={sprint.goal}>{sprint.goal || '—'}</dd></div>
        <div class="fact"><dt>{t('app.sprints.left')}</dt><dd>{#if daysText}<b>{daysText}</b>{/if}<span>{spanText}</span></dd></div>
        <div class="fact"><dt>{t('app.sprints.done_label')}</dt><dd data-sprint-done><b>{doneShort}</b>
          <span>{t('app.sprints.entries', { count: String(open.length) })}</span></dd></div>
      </dl>
      {#if detail && detail.burndown.length > 0}
        <button type="button" class="burndown-open" aria-expanded={isBurndownOpen} onclick={() => (isBurndownOpen = !isBurndownOpen)}>
          {@render burndownPicture('small')}
          <span class="burndown-word">{t('app.sprints.burndown')}<Icon name={isBurndownOpen ? 'chevron-up' : 'chevron-right'} size="sm" /></span>
        </button>
      {/if}
      {#if mayWrite && sprint.state === 'ACTIVE'}
        <Button tone="secondary" icon="check" onclick={askClose}>{t('app.sprints.close')}</Button>
      {:else if mayWrite && sprint.state === 'PLANNED'}
        <Button tone="primary" icon="play" onclick={() => startSprint(sprint)}>{t('app.sprints.start')}</Button>
      {/if}
      {@render barMenuButton()}
    {/if}
  </section>
{/snippet}

<WorkPage>
  {#snippet top()}
  <CollectionHead {collectionId} current="sprint" {onnavigate} figure={chosen ? sprintFigure : undefined} />
  {/snippet}

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !list}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

    {#if list.length === 0}
      <ProductEmpty product="PLANNING" place={{ type: 'COLLECTION', id: collectionId }} kind="unused" title={t('app.sprints.none_title')} description={t('app.sprints.none_hint')} create={mayWrite ? { label: t('app.sprints.plan'), onclick: () => openPlan(undefined) } : undefined} />
    {:else if chosen}
      {@render sprintBar(chosen)}

      {#if isBurndownOpen && detail && detail.burndown.length > 0}
        <!-- The burndown large on the bar's press, its days as a table, and the velocity beside it:
             the plan's aid, never a limit (E-22). -->
        <section class="burndown-large" aria-labelledby="sprint-burndown" data-burndown>
          <h2 id="sprint-burndown" class="heading">{t('app.sprints.burndown_hint')}</h2>
          {@render burndownPicture('large')}
          <DataTable label={t('app.sprints.burndown')} columns={burndownColumns} rows={detail.burndown} key={(point: Point) => point.day}>
            {#snippet cell(point: Point, column: DataColumn)}
              {#if column.id === 'day'}{formatPlainDay(point.day, messages.locale)}
              {:else}<span class="number">{column.id === 'remaining' ? point.remaining : column.id === 'ideal' ? point.ideal : point.scope}</span>{/if}
            {/snippet}
          </DataTable>
          {#if velocity && velocity.sprints.length > 0}
            <h2 class="heading">{t('app.sprints.velocity', { value: String(velocity.average), unit: t(`app.sprints.unit_${velocity.unit}`) })}</h2>
            <DataTable label={t('app.sprints.velocity_table')} columns={velocityColumns} rows={velocity.sprints} key={(past: Past) => past.id}>
              {#snippet cell(past: Past, column: DataColumn)}
                {#if column.id === 'sprint'}{past.name}
                {:else}<span class="number">{column.id === 'committed' ? past.committed_value : past.completed_value}</span>{/if}
              {/snippet}
            </DataTable>
          {/if}
        </section>
      {/if}

      {#if !isOneColumn}
        <!-- The filter line (scene sprint): who, only mine, words; what the columns are, on the right. -->
        <div class="filter-line" role="search" aria-label={t('app.sprints.filter')} data-sprint-filter>
          <span class="query">
            <span class="assignee-chip">
              <span class="chip-key">{t('app.sprints.assignee')}</span>
              <Select label={t('app.sprints.assignee')} isLabelHidden size="sm" value={assignee} options={assigneeOptions}
                onchange={(event: Event) => (assignee = (event.currentTarget as HTMLSelectElement).value)} />
              {#if assignee}<IconButton size="sm" icon="x" label={t('app.sprints.assignee_clear')} onclick={() => (assignee = '')} />{/if}
            </span>
            <input class="words" type="search" aria-label={t('app.sprints.filter_placeholder')} placeholder={t('app.sprints.filter_placeholder')} bind:value={words} />
          </span>
          {#if me}
            <Button size="sm" tone="secondary" icon="user" aria-pressed={assignee === me} onclick={() => (assignee = assignee === me ? '' : me)}>{t('app.sprints.only_mine')}</Button>
          {/if}
          {#if columns && workflowNames.length > 0}<span class="columns-by quiet">{t('app.sprints.columns_by', { name: workflowNames.join(', ') })}</span>{/if}
        </div>
      {/if}

      {#if isOneColumn && lanes.length > 1}
        <div class="strip" data-sprint-strip>
          <ViewSwitcher label={t('app.sprints.columns')} views={stripViews} selected={shownLane} onselect={(id) => (shownLaneId = id)} />
        </div>
      {/if}
      <div class="board" data-sprint-board data-by-columns={columns ? '' : undefined} data-closed={chosen.state === 'CLOSED' ? '' : undefined} data-one={isOneColumn ? '' : undefined}>
        {#each lanes as lane (lane.key)}
          {#if !isOneColumn || lane.key === shownLane}
            <div class="lane" data-lane={lane.key} data-folded={isFoldedLane(lane) ? '' : undefined}>
              <BucketColumn
                name={lane.name}
                count={detail ? lane.entries.length : null}
                isDoneBucket={lane.isDone}
                doneBucketLabel={lane.isDone ? t('app.sprints.done_column_hint') : undefined}
                isFolded={isFoldedLane(lane)}
                unfoldLabel={t('app.board.unfold', { name: lane.name })}
                onunfold={() => (unfolded = { ...unfolded, [lane.key]: true })}
              >
                {#snippet actions()}{#if detail}{@render points(lane.entries)}{/if}{/snippet}
                {#if !detail}
                  <Skeleton lines={2} />
                {:else if lane.entries.length === 0}
                  <p class="quiet">{lane.empty}</p>
                {:else}
                  <ul class="cards">
                    {#each lane.entries as entry (entry.item_id)}{@render card(entry, entry.done ? 'done' : 'sprint', noteOf(entry))}{/each}
                  </ul>
                {/if}
              </BucketColumn>
            </div>
          {/if}
        {/each}
      </div>

      {#if chosen.state !== 'CLOSED' && !isOneColumn}<p class="quiet hint"><Icon name="info" size="sm" />{t('app.sprints.from_backlog')}</p>{/if}
    {/if}
  {/if}
</WorkPage>

{#if isPlanning}
  <CreateDialog
    bind:isOpen={isPlanning}
    title={editing ? t('app.sprints.edit') : t('app.sprints.plan')}
    cancelLabel={t('app.links.cancel')}
    nameLabel={t('app.sprints.name')}
    bind:name={draft.name}
    maxlength={120}
    place={editing ? undefined : { current: draftCollection, fallback: place ? [containers.find(place.parent_id ?? '')?.name, place.name].filter(Boolean).join(' › ') : '', places: placesToPlan.list, onchoose: (id) => (draftCollection = id) }}
    primary={{ label: editing ? t('app.sprints.save') : t('app.sprints.plan'), onclick: plan }}
  >
    <Textarea label={t('app.sprints.goal')} bind:value={draft.goal} maxlength={500} />
    <Inline gap="100">
      <Input label={t('app.sprints.start_on')} type="date" bind:value={draft.start_on} disabledReason={editing?.state === 'ACTIVE' ? t('sprints.running_start_fixed') : undefined} />
      <Input label={t('app.sprints.end_on')} type="date" bind:value={draft.end_on} />
    </Inline>
    <Select label={t('app.sprints.unit')} bind:value={draft.unit} disabledReason={editing?.state === 'ACTIVE' ? t('sprints.running_start_fixed') : undefined}
      options={[{ value: 'POINTS', label: t('app.sprints.unit_POINTS') }, { value: 'HOURS', label: t('app.sprints.unit_HOURS') }]} />
  </CreateDialog>
{/if}

{#if isClosing && chosen}
  <Dialog size="s" bind:isOpen={isClosing} title={t('app.sprints.close_title', { name: chosen.name })} dismissLabel={t('app.links.cancel')} primary={{ label: t('app.sprints.close'), onclick: close }}>
    <Stack gap="150">
      <p class="quiet">{t('app.sprints.close_hint')}</p>
      <Select label={t('app.sprints.carry_to')} bind:value={carryTo}
        options={[{ value: '', label: t('app.sprints.carry_backlog') }, ...targets.map((s) => ({ value: s.id, label: s.name }))]} />
    </Stack>
    {#snippet actions()}<Button tone="secondary" onclick={() => (isClosing = false)}>{t('app.links.cancel')}</Button>{/snippet}
  </Dialog>
{/if}

<style>
  .strip { overflow-x: auto; padding-block-end: var(--sp-050); }
  /* One row that scrolls sideways, as scene sprint 4 draws it: never wrapped onto a second line. */
  .strip :global(.switcher) { flex-wrap: nowrap; }
  /* One row of columns (scene sprint 3), done folded narrow at its end; sideways scroll beyond. */
  .board { display: flex; flex-wrap: nowrap; gap: var(--sp-150); align-items: flex-start; overflow-x: auto; }
  .lane { flex: none; border-radius: var(--r-md); }
  .board[data-one] .lane { flex: 1 1 100%; min-inline-size: 0; }
  /* On a phone the one column is the screen's width (scene sprint 4). */
  .board[data-one] .lane :global(.column) { inline-size: 100%; }
  .cards { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .card {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-raised);
    box-shadow: var(--shadow-raised);
  }
  .card-top { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); min-block-size: var(--sp-300); }
  .card-key { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-secondary); }
  .card-title { color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  .card-title:hover { text-decoration: underline; }
  .card-labels { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .card-foot { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .card-foot:empty { display: none; }
  .card-move { position: absolute; inset-block-end: var(--sp-100); inset-inline-end: var(--sp-100); opacity: 0; }
  .card:hover .card-move, .card-move:focus-within { opacity: 1; }
  @media (hover: none) { .card-move { opacity: 1; } }
  .sprint-bar { display: flex; flex-wrap: nowrap; align-items: center; gap: var(--sp-200); padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .chooser { display: inline-flex; flex: none; align-items: center; gap: var(--sp-050); padding-inline-start: var(--sp-100); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-md); }
  /* The chooser's select sits inside the drawn box, with its state chip: no second border. */
  .chooser :global(.shell), .assignee-chip :global(.shell) { border-color: transparent; background: transparent; }
  .chooser :global(.select) { font-weight: var(--fw-semibold); }
  .sprint-name { font-weight: var(--fw-semibold); }
  .state { flex: none; padding: 0 var(--sp-100); border: var(--bw-hairline) solid var(--accent-primary); border-radius: var(--r-sm); font-size: var(--fs-075); color: var(--text-brand); background: var(--accent-primary-subtle); }
  .chooser .state { margin-inline-end: var(--sp-100); }
  /* One bar (scene sprint 2): the facts side by side beside the chooser, the goal giving way first. */
  .facts { display: flex; flex-wrap: nowrap; gap: var(--sp-300); flex: 1 1 auto; min-inline-size: 0; margin: 0; }
  .fact { display: flex; flex: 0 1 auto; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
  .fact :is(dt, dd > *) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .fact dt { font-size: var(--fs-075); color: var(--text-secondary); }
  .fact dd { display: flex; flex-direction: column; margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
  .fact dd b { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .fact.goal { flex: 1 1 0; min-inline-size: var(--sp-1000); overflow: hidden; }
  .fact.goal dd { display: block; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sprint-bar > :global(*) { flex-shrink: 0; }
  .sprint-bar > .facts { flex-shrink: 1; }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    /* Narrower than the desk: the facts on a line of their own under the chooser and the acts. */
    .sprint-bar:not([data-phone]) { flex-wrap: wrap; }
    .sprint-bar:not([data-phone]) > .facts { flex: 1 1 100%; order: 1; }
    .sprint-bar:not([data-phone]) > .burndown-open { margin-inline-start: auto; }
  }
  .burndown-open { display: inline-flex; flex-direction: column; align-items: flex-end; gap: var(--sp-025); padding: 0; border: 0; background: none; color: var(--text-secondary); font: inherit; font-size: var(--fs-075); cursor: pointer; }
  .burndown-word { display: inline-flex; align-items: center; gap: var(--sp-025); }
  /* The phone's card (scene sprint 4): state, days and points on one line, the goal, the burndown wide. */
  .sprint-bar[data-phone] { flex-direction: column; align-items: stretch; gap: var(--sp-100); }
  .phone-line { display: flex; align-items: center; gap: var(--sp-100); }
  .phone-figure { flex: 1 1 auto; min-inline-size: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .phone-goal { color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sprint-bar[data-phone] .burndown-open { align-items: stretch; }
  .sprint-bar[data-phone] .spark { inline-size: 100%; block-size: var(--sp-600); background: none; }
  .points { font-size: var(--fs-075); color: var(--text-secondary); font-variant-numeric: tabular-nums; }
  .hint { display: flex; align-items: center; gap: var(--sp-050); }
  .spark { flex: none; inline-size: calc(var(--sp-1000) * 1.25); block-size: var(--sp-400); }
  .chart { inline-size: 100%; block-size: calc(var(--sp-1000) * 2); border-radius: var(--r-sm); background: var(--bg-surface-sunken); }
  :is(.spark, .chart) polyline { fill: none; stroke-width: var(--bw-thick); vector-effect: non-scaling-stroke; }
  :is(.spark, .chart) .remaining { stroke: var(--accent-primary); }
  :is(.spark, .chart) .ideal { stroke: var(--border-strong); stroke-dasharray: 4 4; }
  :is(.spark, .chart) .scope { stroke: var(--text-subtle); }
  .burndown-large { display: flex; flex-direction: column; gap: var(--sp-150); min-width: 0; padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .filter-line { display: flex; align-items: center; gap: var(--sp-150); min-inline-size: 0; }
  .query { display: flex; flex: 0 1 calc(var(--sp-1000) * 4.5); align-items: center; gap: var(--sp-100); min-inline-size: 0; padding: var(--sp-050) var(--sp-100); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-md); background: var(--bg-surface); }
  .assignee-chip { display: inline-flex; flex: none; align-items: center; gap: var(--sp-050); padding-inline-start: var(--sp-100); border-radius: var(--r-sm); background: var(--bg-surface-sunken); font-size: var(--fs-075); }
  .assignee-chip :global(.select) { font-weight: var(--fw-semibold); }
  .chip-key { color: var(--text-secondary); }
  .words { flex: 1 1 auto; min-inline-size: 0; border: 0; background: transparent; color: var(--text-primary); font: inherit; font-size: var(--fs-075); }
  .words:focus { outline: none; }
  .query:focus-within { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .columns-by { margin-inline-start: auto; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .note { color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .words { align-self: stretch; min-block-size: var(--layout-target); }
  }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .card-title { position: relative; }
    .card-title::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
