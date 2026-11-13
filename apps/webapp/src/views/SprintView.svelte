<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Sprints* of a collection (M2, UC-PLN-06, UC-PLN-07): the chosen sprint as one bar - goal, days
  // left, points, a small burndown, *close sprint* - over the board's columns, which carry the
  // workflow's statuses, each with its points; on a phone one column under a strip. The backlog is
  // its own tab, never a column of the sprint (work-surface concept §6.2, E-22). The velocity stays
  // below (user-experience.md §7.5).
  //
  // **The server counts; this screen draws and writes.** Commitment, result, burndown and velocity
  // are the answers of `/sprints/{id}` and `/collections/{id}/velocity`; every start, close, move
  // or estimate is a write, and the sprint is read again after it.
  //
  // **The controls are the keyboard path** (UC-PLN-07 check 5): the burndown's picture has a table
  // of the same days under it, and every card's estimate and move is a real control.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS at the collection the
  // screen shows values, not controls.
  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    BucketColumn, Button, Dialog, Disclosure, Icon, IconButton, Inline, Input, Select, Skeleton, Stack, Textarea,
    DataTable, ViewSwitcher, WorkPage, type DataColumn, type View,
  } from '@hubtask/design-system/components';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { Sprint, SprintDetail, SprintEntry, SprintList, Velocity, WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { buckets } from '../lib/data/buckets.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import {
    burndownLine, burndownTop, carryTargets, entriesToRead, estimateOf, estimateText, sprintColumns, sprintDays, sprintPath,
    sprints, sprintShown, sprintsPath, valueOf, velocityPath,
  } from '../lib/data/sprints.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';
  import CollectionHead from '../lib/workspace/CollectionHead.svelte';
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

  /** The draft of each estimate field, by entry, until it is saved. */
  const drafts = $state<Record<string, string>>({});
  /** An estimate the field cannot send, said at the field (§5.10), by entry. */
  const estimateErrors = $state<Record<string, string | undefined>>({});
  /** Saved when the chip is left or Enter pressed: no per-card *Save* (§7.5). */
  function saveEstimate(itemId: string, title: string) {
    const text = drafts[itemId];
    if (text === undefined) return;
    const input = estimateOf(text);
    if (input === undefined) {
      estimateErrors[itemId] = t('estimates.value_out_of_range', { maximum: '10000' });
      return;
    }
    estimateErrors[itemId] = undefined;
    void act(() => sprints.estimate(collectionId, zone, chosen?.id, itemId, input), t('app.sprints.estimated', { title })).then(() => delete drafts[itemId]);
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
  // The sprint's columns are the board's, which carry the workflow's statuses (§6.2): each entry in
  // the column it stands in. Without a board, or for a closed sprint whose entries have moved on,
  // to do, in progress and done.
  $effect(() => {
    const id = collectionId;
    return untrack(() => {
      const stops = [buckets.open(id), items.openCollection(id)];
      return () => stops.forEach((stop) => stop());
    });
  });
  const isLevelRead = $derived(buckets.stateOf(collectionId)?.status === 'ready' && items.stateOf(`container:${collectionId}`)?.status === 'ready');
  // The collection's level is a page; an open entry beyond it has its column read on its own, so
  // every entry of the sprint stands in its column.
  let columnRead = $state<Record<string, string | null>>({});
  let reading = $state(0);
  const toRead = $derived(isLevelRead && hasStartedLane ? entriesToRead(open, (itemId) => itemId in columnRead || items.find(itemId) !== undefined) : []);
  $effect(() => {
    const ids = toRead;
    if (ids.length === 0) return;
    untrack(() => {
      reading += 1;
      void Promise.all(ids.map((itemId) => engine.refresh<WorkItem>({ path: `/items/${itemId}` }).then((read) => [itemId, read.status === 'ready' ? (read.data.bucket_id ?? null) : null] as const)))
        .then((pairs) => { columnRead = { ...columnRead, ...Object.fromEntries(pairs) }; })
        .catch(() => { columnRead = { ...columnRead, ...Object.fromEntries(ids.map((itemId) => [itemId, null])) }; })
        .finally(() => { reading -= 1; });
    });
  });
  const isPlaced = $derived(isLevelRead && toRead.length === 0 && reading === 0);
  const columns = $derived(
    hasStartedLane && isPlaced
      ? sprintColumns(open, buckets.of(collectionId), (itemId) => items.find(itemId)?.bucket_id ?? columnRead[itemId], t('app.board.unbucketed'))
      : undefined,
  );
  type Lane = { key: string; name: string; isDone: boolean; entries: readonly SprintEntry[]; empty: string };
  const lanes = $derived<Lane[]>(
    columns
      ? columns.map((column) => ({ key: column.id ?? 'none', name: column.name, isDone: column.isDone, entries: column.entries, empty: t('app.sprints.column_empty') }))
      : [
          { key: 'todo', name: t('app.sprints.todo_column'), isDone: false, entries: toDo, empty: chosen?.state === 'CLOSED' ? t('app.sprints.todo_none_closed') : t('app.sprints.todo_empty') },
          ...(hasStartedLane ? [{ key: 'in-progress', name: t('app.sprints.in_progress_column'), isDone: false, entries: inProgress, empty: t('app.sprints.in_progress_empty') }] : []),
          { key: 'done', name: t('app.sprints.done_column'), isDone: true, entries: done, empty: t('app.sprints.done_empty') },
        ],
  );
  // On a phone one column at a time, the strip above saying which and how many (§7, scene sprint 4).
  const isOneColumn = $derived(viewport.isCompact);
  // The done column starts folded, as on the board (scene sprint): a closed sprint keeps it open,
  // its result is what it shows.
  let unfolded = $state<Record<string, boolean>>({});
  const isFoldedLane = (lane: { key: string; isDone: boolean }) => lane.isDone && hasStartedLane && !isOneColumn && !unfolded[lane.key];
  let shownLaneId = $state<string | undefined>(undefined);
  const shownLane = $derived(lanes.some((lane) => lane.key === shownLaneId) ? (shownLaneId as string) : (lanes.find((lane) => !lane.isDone && lane.entries.length > 0) ?? lanes[0])?.key ?? '');
  const stripViews = $derived<View[]>(lanes.map((lane) => ({ id: lane.key, label: t('app.board.column_with_count', { name: lane.name, count: String(lane.entries.length) }) })));
  const noteOf = (entry: SprintEntry) =>
    entry.outcome ? t(`app.sprints.outcome_${entry.outcome}`) : !entry.committed && chosen?.state !== 'PLANNED' ? t('app.sprints.added_later') : undefined;
  const days = $derived(chosen ? sprintDays(chosen) : 0);
  const top = $derived(chosen && detail ? burndownTop(chosen, detail.burndown) : 0);
  const unitLabel = $derived(chosen ? t(`app.sprints.unit_${chosen.unit}`) : '');
  const states = $derived(list ? list.map((s) => ({ value: s.id, label: t('app.sprints.option', { name: s.name, state: t(`app.sprints.state_${s.state}`) }) })) : []);

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

  // The head's figure (§5.1): the time box and how far it is, from the server's own burndown.
  const figure = $derived.by(() => {
    if (!chosen) return undefined;
    const span = t('app.sprints.span', { from: formatPlainDay(chosen.start_on, messages.locale), to: formatPlainDay(chosen.end_on, messages.locale) });
    if (chosen.state === 'CLOSED') {
      return `${span} · ${t('app.sprints.completed', { count: String(chosen.completed_count), value: String(chosen.completed_value), unit: unitLabel })}`;
    }
    if (chosen.state === 'PLANNED') return `${span} · ${t('app.sprints.entries_planned', { count: String(open.length) })}`;
    const left = Math.max(0, Math.round((Date.parse(`${chosen.end_on}T00:00:00Z`) - Date.parse(`${today}T00:00:00Z`)) / 86_400_000) + 1);
    const last = detail?.burndown.at(-1);
    const parts = [t('app.sprints.days_left', { count: String(left) })];
    if (last) parts.push(t('app.sprints.done_of', { done: String(last.scope - last.remaining), scope: String(last.scope), unit: unitLabel }));
    return parts.join(' · ');
  });

  const headMenu = $derived([
    ...(mayWrite && chosen && chosen.state !== 'CLOSED' ? [{ id: 'edit', label: t('app.sprints.edit'), icon: 'pencil' as const }] : []),
    ...(mayWrite && chosen?.state === 'PLANNED' ? [{ id: 'delete', label: t('app.sprints.delete'), icon: 'trash' as const, hasSeparatorBefore: true }] : []),
  ]);
  const startSprint = (sprint: Sprint) => void act(() => sprints.start(collectionId, zone, sprint.id), t('app.sprints.started', { name: sprint.name }));
  const askClose = () => { carryTo = targets[0]?.id ?? ''; isClosing = true; };
  /** A column's points: its entries' estimates in the sprint's unit (UC-PLN-07 check 2). */
  const columnFigure = (entries: readonly SprintEntry[]) => (chosen ? t('app.sprints.column_value', { value: String(valueOf(entries, chosen.unit)), unit: unitLabel }) : '');
  const daysLeft = $derived(chosen ? Math.max(0, Math.round((Date.parse(`${chosen.end_on}T00:00:00Z`) - Date.parse(`${today}T00:00:00Z`)) / 86_400_000) + 1) : 0);
  const lastPoint = $derived(detail?.burndown.at(-1));

  function onMenu(id: string) {
    const sprint = chosen;
    if (!sprint) return;
    if (id === 'edit') openPlan(sprint);
    if (id === 'delete') {
      chosenId = '';
      void act(() => sprints.remove(collectionId, zone, sprint.id), t('app.sprints.deleted'));
    }
  }
</script>

{#snippet burndownPicture()}
  {#if chosen && detail && detail.burndown.length > 0}
    <svg class="spark" viewBox="0 0 400 100" preserveAspectRatio="none" role="img" aria-label={t('app.sprints.burndown_label', { remaining: String(detail.burndown.at(-1)?.remaining ?? 0), unit: unitLabel })}>
      <polyline class="ideal" points={burndownLine(detail.burndown, 'ideal', days, top, 400, 100)} />
      <polyline class="scope" points={burndownLine(detail.burndown, 'scope', days, top, 400, 100)} />
      <polyline class="remaining" points={burndownLine(detail.burndown, 'remaining', days, top, 400, 100)} />
    </svg>
  {/if}
{/snippet}

{#snippet card(entry: { item_id: string; title: string; estimate: SprintEntry['estimate'] }, where: 'sprint' | 'done', note?: string)}
  {@const isMovable = mayWrite && chosen !== undefined && chosen.state !== 'CLOSED'}
  <li class="card" data-card={entry.item_id}>
    <a class="card-title" href={`/items/${entry.item_id}`}>{entry.title}</a>
    {#if note}<span class="note">{note}</span>{/if}
    <span class="card-foot">
      {#if isMovable}
        <Input
          label={t('app.sprints.estimate_of', { title: entry.title })}
          isLabelHidden
          size="sm"
          placeholder={t('app.sprints.estimate_placeholder')}
          value={drafts[entry.item_id] ?? estimateText(entry.estimate)}
          error={estimateErrors[entry.item_id]}
          oninput={(event: Event) => (drafts[entry.item_id] = (event.currentTarget as HTMLInputElement).value)}
          onchange={() => saveEstimate(entry.item_id, entry.title)}
        />
        {#if where !== 'done'}
        <IconButton size="sm" icon="arrow-left" label={t('app.sprints.to_backlog_of', { title: entry.title })} onclick={() => toBacklog(entry.item_id)} />
        {/if}
      {:else if entry.estimate}
        <span class="chip">{estimateText(entry.estimate)}</span>
      {/if}
    </span>
  </li>
{/snippet}

{#snippet sprintFigure()}
  <!-- The sprint's own figure in the collection's figure's place: its days and how far it is; the
       burndown's picture is the bar's, on the sprint's tab. -->
  <span data-sprint-figure>{figure}</span>
{/snippet}

{#snippet points(entries: readonly SprintEntry[])}
  <span class="points" data-points>{columnFigure(entries)}</span>
{/snippet}

{#snippet sprintBar(sprint: Sprint)}
  <!-- One bar for the sprint (scene sprint 2): which, its goal, the days left, the points done, the
       burndown small, and its one act. -->
  <section class="sprint-bar" aria-label={t('app.sprints.bar')} data-sprint-bar>
    {#if list && list.length > 1}
      <Select label={t('app.sprints.sprint')} size="sm" value={sprint.id} options={states}
        onchange={(event: Event) => { chosenId = (event.currentTarget as HTMLSelectElement).value; }} />
    {:else}
      <b class="sprint-name">{sprint.name}</b>
    {/if}
    <span class="state" data-state={sprint.state}>{t(`app.sprints.state_${sprint.state}`)}</span>
    <dl class="facts">
      <div class="fact goal"><dt>{t('app.sprints.goal')}</dt><dd title={sprint.goal}>{sprint.goal || '—'}</dd></div>
      {#if sprint.state === 'ACTIVE'}
        <div class="fact"><dt>{t('app.sprints.left')}</dt><dd><b>{t('app.sprints.days_left', { count: String(daysLeft) })}</b>
          <span>{t('app.sprints.span', { from: formatPlainDay(sprint.start_on, messages.locale), to: formatPlainDay(sprint.end_on, messages.locale) })}</span></dd></div>
      {:else}
        <div class="fact"><dt>{t('app.sprints.left')}</dt><dd><span>{t('app.sprints.span', { from: formatPlainDay(sprint.start_on, messages.locale), to: formatPlainDay(sprint.end_on, messages.locale) })}</span></dd></div>
      {/if}
      <div class="fact"><dt>{t('app.sprints.done_label')}</dt><dd data-sprint-done>
        <b>{lastPoint ? t('app.sprints.done_of', { done: String(lastPoint.scope - lastPoint.remaining), scope: String(lastPoint.scope), unit: unitLabel }) : t('app.sprints.done_of', { done: String(valueOf(done, sprint.unit)), scope: String(valueOf(open, sprint.unit)), unit: unitLabel })}</b>
        <span>{t('app.sprints.entries_planned', { count: String(open.length) })}</span></dd></div>
    </dl>
    {@render burndownPicture()}
    {#if mayWrite && sprint.state === 'ACTIVE'}
      <Button tone="secondary" icon="check" onclick={askClose}>{t('app.sprints.close')}</Button>
    {:else if mayWrite && sprint.state === 'PLANNED'}
      <Button tone="primary" icon="play" onclick={() => startSprint(sprint)}>{t('app.sprints.start')}</Button>
    {/if}
  </section>
{/snippet}

<WorkPage>
  {#snippet top()}
  <CollectionHead
    {collectionId}
    current="sprint"
    {onnavigate}
    figure={figure ? sprintFigure : undefined}
    primary={mayWrite ? { label: t('app.sprints.plan'), icon: 'plus', onclick: () => openPlan(undefined) } : undefined}
    menu={headMenu.length > 0 ? { label: t('app.sprints.more'), items: headMenu, onselect: onMenu } : undefined}
  />
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

      {#if chosen.state !== 'CLOSED'}<p class="quiet hint"><Icon name="info" size="sm" />{t('app.sprints.from_backlog')}</p>{/if}

      {#if detail && detail.burndown.length > 0}
        <Disclosure label={t('app.sprints.burndown')} description={t('app.sprints.burndown_hint')} tone="quiet">
          <DataTable label={t('app.sprints.burndown')} columns={burndownColumns} rows={detail.burndown} key={(point: Point) => point.day}>
            {#snippet cell(point: Point, column: DataColumn)}
              {#if column.id === 'day'}{formatPlainDay(point.day, messages.locale)}
              {:else}<span class="number">{column.id === 'remaining' ? point.remaining : column.id === 'ideal' ? point.ideal : point.scope}</span>{/if}
            {/snippet}
          </DataTable>
        </Disclosure>
      {/if}
    {/if}

    {#if velocity && velocity.sprints.length > 0}
      <section class="block" aria-labelledby="sprint-velocity">
        <h2 id="sprint-velocity" class="heading">{t('app.sprints.velocity', { value: String(velocity.average), unit: t(`app.sprints.unit_${velocity.unit}`) })}</h2>
        <DataTable label={t('app.sprints.velocity_table')} columns={velocityColumns} rows={velocity.sprints} key={(past: Past) => past.id}>
          {#snippet cell(past: Past, column: DataColumn)}
            {#if column.id === 'sprint'}{past.name}
            {:else}<span class="number">{column.id === 'committed' ? past.committed_value : past.completed_value}</span>{/if}
          {/snippet}
        </DataTable>
      </section>
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
  /* One row that scrolls sideways, as scene board 4 draws it: never wrapped onto a second line. */
  .strip :global(.switcher) { flex-wrap: nowrap; }
  .board { display: flex; flex-wrap: wrap; gap: var(--sp-200); align-items: flex-start; }
  .lane { flex: 0 1 var(--layout-board-column); min-width: 0; border-radius: var(--r-md); }
  .lane[data-folded] { flex: none; }
  .board[data-one] .lane { flex: 1 1 100%; }
  .cards { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .card {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .card-title { color: var(--text-primary); font-weight: var(--fw-medium); }
  .card-foot { display: flex; align-items: start; justify-content: space-between; gap: var(--sp-100); }
  .card-foot :global(input) { inline-size: var(--sp-1000); }
  .chip { padding: 0 var(--sp-050); border-radius: var(--r-sm); background: var(--bg-surface-sunken); color: var(--text-secondary); font-size: var(--fs-075); }
  .sprint-bar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-200); padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .sprint-name { font-weight: var(--fw-semibold); }
  .state { padding: 0 var(--sp-100); border-radius: var(--r-sm); font-size: var(--fs-075); color: var(--text-brand); background: var(--accent-primary-subtle); }
  .facts { display: flex; flex-wrap: wrap; gap: var(--sp-300); flex: 1; min-inline-size: 0; margin: 0; }
  .fact { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
  .fact dt { font-size: var(--fs-075); color: var(--text-secondary); }
  .fact dd { display: flex; flex-direction: column; margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
  .fact dd b { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .fact.goal { flex: 1 1 var(--sp-1600); }
  .fact.goal dd { display: block; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .points { font-size: var(--fs-075); color: var(--text-secondary); font-variant-numeric: tabular-nums; }
  .hint { display: flex; align-items: center; gap: var(--sp-050); }
  .spark { flex: none; inline-size: var(--sp-1600); block-size: var(--sp-400); border-radius: var(--r-sm); background: var(--bg-surface-sunken); }
  .spark polyline { fill: none; stroke-width: var(--bw-thick); vector-effect: non-scaling-stroke; }
  .spark .remaining { stroke: var(--accent-primary); }
  .spark .ideal { stroke: var(--border-strong); stroke-dasharray: 4 4; }
  .spark .scope { stroke: var(--text-subtle); }
  .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; }
</style>
