<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's *Backlog* tab (UC-PLN-06, UC-PLN-07; work-surface concept §6.2, scene backlog,
  // E-22): the running sprint folded on top, the planned sprint against the velocity of the last
  // three with *Start sprint*, and the backlog as a ranking to carry into it, with the line to type
  // at its foot - the capture field in place (C-4). No per-person capacity (E-22).
  //
  // **Every drag has its keys** (WCAG 2.5.7): a row's handle ranks with Alt and the arrows, its
  // arrow plans it in; a mouse carrying a row onto another ranks it, onto the planned sprint plans
  // it in. The rank is the collection's own order (`:reorder`), the plan a sprint write; both are
  // read again from the server.
  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    Button, Icon, IconButton, Inline, Input, ProgressBar, Select, Skeleton, Textarea,
    hasLeftTheHandle, rankIntent, WorkPage,
  } from '@hubtask/design-system/components';
  import type { Sprint, SprintBacklog, SprintDetail, SprintList, Velocity, WorkItem } from '@hubtask/sync-engine';

  import { announcer } from '../lib/announce.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { items } from '../lib/data/items.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { anchorFor } from '../lib/data/rank.ts';
  import {
    backlogPath, carryTargets, estimateOf, estimateText, planLoad, sprintPath, sprints, sprintsPath, valueOf, velocityPath,
  } from '../lib/data/sprints.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';
  import CaptureDialog from '../lib/capture/CaptureDialog.svelte';
  import CollectionHead from '../lib/workspace/CollectionHead.svelte';
  import CreateDialog from '../lib/product/CreateDialog.svelte';
  import { createPlaces } from '../lib/product/places.svelte.ts';

  interface Props {
    /** The collection the backlog belongs to. */
    collectionId: string;
    onnavigate: (path: string) => void;
  }

  const { collectionId, onnavigate }: Props = $props();

  const zone = $derived(actor.zone);
  const place = $derived(containers.find(collectionId));
  const rolePath = $derived({ hubId: place?.parent_id ?? undefined, collectionId });

  $effect(() => frame.entitle(place ? t('app.sprints.title_of', { place: place.name }) : t('app.sprints.title')));
  $effect(() => {
    const path = rolePath;
    untrack(() => people.open(path));
  });
  // The collection's own level: the version a rank write states.
  $effect(() => {
    const id = collectionId;
    return untrack(() => items.openCollection(id));
  });

  const mayWrite = $derived(holds(people.roleOf(rolePath, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted');

  let list = $state<Sprint[] | undefined>(undefined);
  let backlog = $state<SprintBacklog['data']>([]);
  let velocity = $state<Velocity | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const paths = [sprintsPath(collectionId), backlogPath(collectionId), velocityPath(collectionId)] as const;
    return untrack(() => {
      const stops = [
        engine.subscribe<SprintList>({ path: paths[0] }, (next) => {
          if (next.status === 'ready') { list = next.data.data; refusal = undefined; }
          if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
        }),
        engine.subscribe<SprintBacklog>({ path: paths[1] }, (next) => { if (next.status === 'ready') backlog = next.data.data; }),
        engine.subscribe<Velocity>({ path: paths[2] }, (next) => { if (next.status === 'ready') velocity = next.data; }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  const running = $derived(list?.find((s) => s.state === 'ACTIVE'));
  const planned = $derived(list ? carryTargets(list, '')[0] : undefined);
  /** Where the backlog plans into: the next planned sprint, else the running one. */
  const target = $derived(planned ?? running);

  let runningDetail = $state<SprintDetail | undefined>(undefined);
  let targetDetail = $state<SprintDetail | undefined>(undefined);
  $effect(() => {
    const id = running?.id;
    runningDetail = undefined;
    if (!id) return;
    const path = sprintPath(id, zone);
    return untrack(() => engine.subscribe<SprintDetail>({ path }, (next) => { if (next.status === 'ready') runningDetail = next.data; }));
  });
  $effect(() => {
    const id = target?.id;
    targetDetail = undefined;
    if (!id) return;
    const path = sprintPath(id, zone);
    return untrack(() => engine.subscribe<SprintDetail>({ path }, (next) => { if (next.status === 'ready') targetDetail = next.data; }));
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

  const unit = $derived(target?.unit ?? velocity?.unit ?? 'POINTS');
  const unitLabel = $derived(t(`app.sprints.unit_${unit}`));
  const span = (sprint: Sprint) => t('app.sprints.span', { from: formatPlainDay(sprint.start_on, messages.locale), to: formatPlainDay(sprint.end_on, messages.locale) });
  const runningOpen = $derived((runningDetail?.entries ?? []).filter((e) => !e.removed_at));
  const targetOpen = $derived((targetDetail?.entries ?? []).filter((e) => !e.removed_at));
  const plannedValue = $derived(valueOf(targetOpen, unit));
  const load = $derived(planned && velocity ? planLoad(plannedValue, velocity.average) : undefined);
  const backlogValue = $derived(valueOf(backlog, unit));
  const unestimated = $derived(backlog.filter((entry) => !entry.estimate).length);

  /** The draft of each estimate field, by entry, until it is saved. */
  const drafts = $state<Record<string, string>>({});
  const estimateErrors = $state<Record<string, string | undefined>>({});
  function saveEstimate(itemId: string, title: string) {
    const text = drafts[itemId];
    if (text === undefined) return;
    const input = estimateOf(text);
    if (input === undefined) {
      estimateErrors[itemId] = t('estimates.value_out_of_range', { maximum: '10000' });
      return;
    }
    estimateErrors[itemId] = undefined;
    void act(() => sprints.estimate(collectionId, zone, target?.id, itemId, input), t('app.sprints.estimated', { title })).then(() => delete drafts[itemId]);
  }

  function planIn(itemId: string) {
    const sprint = target;
    if (!sprint) return;
    void act(() => sprints.add(collectionId, zone, sprint.id, [itemId]));
  }
  function takeOut(itemId: string) {
    const sprint = target;
    if (!sprint) return;
    void act(() => sprints.take(collectionId, zone, sprint.id, [itemId]));
  }

  /** The backlog read again: a rank or a capture writes the entries, not the backlog's own read. */
  const rereadBacklog = () => engine.refresh({ path: backlogPath(collectionId) });

  /** Ranks a backlog entry to `position` in the backlog, before the entry standing there now. */
  function rankTo(itemId: string, position: number) {
    const entry = backlog.find((each) => each.item_id === itemId);
    if (!entry) return;
    const before = anchorFor(backlog.map((each) => ({ id: each.item_id })), itemId, position);
    const count = backlog.length;
    void act(async () => {
      const known = items.find(itemId);
      const read = known ? undefined : await engine.refresh<WorkItem>({ path: `/items/${itemId}` });
      const version = known?.version ?? (read?.status === 'ready' ? read.data.version : undefined);
      if (version === undefined) return;
      await items.reorder(itemId, before, version, crypto.randomUUID());
      await rereadBacklog();
      announcer.say(t('app.sprints.ranked', { title: entry.title, position: String(position + 1), count: String(count) }));
    });
  }

  function onHandleKey(event: KeyboardEvent, itemId: string, index: number) {
    const to = rankIntent(event, index, backlog.length);
    if (to === null) return;
    event.preventDefault();
    rankTo(itemId, to);
  }

  // A row carried by a fine pointer: onto another backlog row it takes that row's place, onto the
  // planned sprint it is planned in, from the sprint onto the backlog it goes back. A finger
  // scrolls; its way is the row's arrow and handle, as the keyboard's is.
  let dragging = $state<{ id: string; from: 'backlog' | 'sprint' } | undefined>(undefined);
  let over = $state<{ zone: 'backlog' | 'sprint'; before?: string } | undefined>(undefined);
  function beginDrag(event: PointerEvent, id: string, from: 'backlog' | 'sprint') {
    if (event.pointerType !== 'mouse' || event.button !== 0) return;
    if ((event.target as HTMLElement | null)?.closest('a, input, button:not([data-handle])')) return;
    const origin = { x: event.clientX, y: event.clientY };
    const onMove = (move: PointerEvent) => {
      if (!dragging) {
        if (!hasLeftTheHandle(origin, { x: move.clientX, y: move.clientY })) return;
        dragging = { id, from };
      }
      const under = document.elementFromPoint(move.clientX, move.clientY);
      const zoneOf = under?.closest<HTMLElement>('[data-drop]');
      const row = under?.closest<HTMLElement>('[data-backlog-row]');
      over = zoneOf ? { zone: zoneOf.dataset['drop'] as 'backlog' | 'sprint', before: row?.dataset['backlogRow'] } : undefined;
    };
    const finish = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', finish);
      window.removeEventListener('pointercancel', finish);
      const landed = over;
      const carried = dragging;
      dragging = undefined;
      over = undefined;
      if (!carried || !landed) return;
      if (carried.from === 'backlog' && landed.zone === 'sprint') planIn(carried.id);
      else if (carried.from === 'sprint' && landed.zone === 'backlog') takeOut(carried.id);
      else if (carried.from === 'backlog' && landed.before && landed.before !== carried.id) {
        const at = backlog.findIndex((each) => each.item_id === landed.before);
        const from = backlog.findIndex((each) => each.item_id === carried.id);
        if (at >= 0) rankTo(carried.id, from < at ? at - 1 : at);
      }
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', finish);
    window.addEventListener('pointercancel', finish);
  }

  let isPlanning = $state(false);
  let editing = $state<Sprint | undefined>(undefined);
  const draft = $state({ name: '', goal: '', start_on: '', end_on: '', unit: 'POINTS' as 'POINTS' | 'HOURS' });
  let draftCollection = $state('');
  const placesToPlan = createPlaces('PLANNING', 'COLLECTION', () => isPlanning && !editing);
  const today = $derived(todayIn(zone));
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
      await sprints.create(at, zone, fields);
      if (at !== collectionId) onnavigate(`/collections/${encodeURIComponent(at)}/backlog`);
    }, t('app.sprints.planned', { name: fields.name }));
  }
  /** One sprint runs at a time (UC-PLN-06): while one runs, *Start sprint* says so instead of failing. */
  const startSprint = (sprint: Sprint) => void act(() => sprints.start(collectionId, zone, sprint.id), t('app.sprints.started', { name: sprint.name }));
</script>

{#snippet estimate(entry: { item_id: string; title: string; estimate: SprintBacklog['data'][number]['estimate'] })}
  {#if mayWrite}
    <span class="estimate">
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
    </span>
  {:else if entry.estimate}
    <span class="chip">{estimateText(entry.estimate)}</span>
  {/if}
{/snippet}

<WorkPage>
  {#snippet top()}
  <CollectionHead
    {collectionId}
    current="backlog"
    {onnavigate}
    primary={mayWrite ? { label: t('app.sprints.plan'), icon: 'plus', onclick: () => openPlan(undefined) } : undefined}
  />
  {/snippet}

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !list}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
    {#if mayWrite}<p class="quiet hint" data-backlog-hint>{t('app.sprints.backlog_hint')}</p>{/if}

    {#if running && running.id !== target?.id}
      <!-- The running sprint folded (scene backlog 2): its name, days and load, and the way into it. -->
      <section class="block" aria-label={t('app.sprints.running_region')} data-running>
        <div class="head">
          <Icon name="chevron-right" size="sm" />
          <b class="name">{running.name}</b>
          <span class="state" data-state="ACTIVE">{t('app.sprints.state_ACTIVE')}</span>
          <span class="quiet">{t('app.sprints.running_figure', { span: span(running), count: String(runningOpen.length), value: String(valueOf(runningOpen, running.unit)), unit: t(`app.sprints.unit_${running.unit}`) })}</span>
          <span class="grow"></span>
          <a class="open" href={`/collections/${collectionId}/sprint`}><Icon name="arrow-right" size="sm" />{t('app.sprints.open_sprint')}</a>
        </div>
        <p class="quiet folded">{t('app.sprints.folded', { count: String(runningOpen.length) })}</p>
      </section>
    {/if}

    {#if target}
      <!-- The sprint the backlog plans into (scene backlog 3): against the velocity, with its start. -->
      <section class="block" aria-label={t('app.sprints.planned_region')} data-planned data-drop="sprint" data-over={over?.zone === 'sprint' && dragging?.from === 'backlog' ? '' : undefined}>
        <div class="head">
          <b class="name">{target.name}</b>
          <span class="state" data-state={target.state}>{t(`app.sprints.state_${target.state}`)}</span>
          <span class="quiet">{span(target)}</span>
          <span class="grow"></span>
          {#if load && velocity}
            <span class="load" data-load data-over-velocity={load.isOver ? '' : undefined}>
              <ProgressBar size="sm" label={t('app.sprints.load_label')} value={load.percent}
                valueLabel={t('app.sprints.load', { planned: String(plannedValue), velocity: String(velocity.average), unit: unitLabel })} />
              {#if load.isOver}<span class="over"><Icon name="triangle-alert" size="sm" />{t('app.sprints.load_over')}</span>{/if}
            </span>
          {/if}
          {#if mayWrite}
            <Button tone="subtle" icon="pencil" onclick={() => openPlan(target)}>{t('app.sprints.change')}</Button>
            {#if target.state === 'PLANNED'}
              <Button tone="primary" icon="play" disabledReason={running ? t('sprints.another_running') : undefined} onclick={() => startSprint(target)}>{t('app.sprints.start')}</Button>
            {/if}
          {/if}
        </div>
        {#if !targetDetail}
          <div class="empty"><Skeleton lines={2} /></div>
        {:else if targetOpen.length === 0}
          <p class="quiet empty">{t('app.sprints.planned_empty')}</p>
        {:else}
          <ul class="rows">
            {#each targetOpen as entry (entry.item_id)}
              <li class="row" data-card={entry.item_id} data-dragging={dragging?.id === entry.item_id ? '' : undefined}
                onpointerdown={mayWrite ? (event) => beginDrag(event, entry.item_id, 'sprint') : undefined}>
                <a class="title" href={`/items/${entry.item_id}`}>{entry.title}</a>
                {@render estimate(entry)}
                {#if mayWrite}
                  <IconButton size="sm" icon="arrow-down" label={t('app.sprints.to_backlog_of', { title: entry.title })} onclick={() => takeOut(entry.item_id)} />
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
        {#if mayWrite && dragging?.from === 'backlog'}<p class="drop quiet" aria-hidden="true">{t('app.sprints.drop_here')}</p>{/if}
      </section>
    {:else}
      <section class="block" aria-label={t('app.sprints.planned_region')} data-planned>
        <div class="head">
          <p class="quiet">{list.length === 0 ? t('app.sprints.none_hint') : t('app.sprints.no_planned')}</p>
          <span class="grow"></span>
          {#if mayWrite}<Button tone="secondary" icon="plus" onclick={() => openPlan(undefined)}>{t('app.sprints.plan')}</Button>{/if}
        </div>
      </section>
    {/if}

    <!-- The backlog as a ranking (scene backlog 4): the top comes first; a line to type at its foot. -->
    <section class="block" aria-labelledby="backlog-heading" data-backlog data-drop="backlog" data-over={over?.zone === 'backlog' && dragging?.from === 'sprint' ? '' : undefined}>
      <div class="head">
        <h2 id="backlog-heading" class="name">{t('app.sprints.backlog_column')}</h2>
        <span class="quiet" data-backlog-figure>{t('app.sprints.backlog_figure', { count: String(backlog.length), value: String(backlogValue), unit: unitLabel, unestimated: String(unestimated) })}</span>
      </div>
      {#if backlog.length === 0}
        <p class="quiet empty">{t('app.sprints.backlog_empty')}</p>
      {:else}
        <ol class="rows" aria-labelledby="backlog-heading">
          {#each backlog as entry, index (entry.item_id)}
            <li class="row" data-backlog-row={entry.item_id} data-card={entry.item_id}
              data-dragging={dragging?.id === entry.item_id ? '' : undefined}
              data-before={over?.before === entry.item_id && dragging?.from === 'backlog' && dragging.id !== entry.item_id ? '' : undefined}
              onpointerdown={mayWrite ? (event) => beginDrag(event, entry.item_id, 'backlog') : undefined}>
              {#if mayWrite}
                <button type="button" class="handle" data-handle aria-label={t('app.sprints.rank_of', { title: entry.title })}
                  onkeydown={(event) => onHandleKey(event, entry.item_id, index)}><Icon name="grip-vertical" size="sm" /></button>
              {/if}
              <a class="title" href={`/items/${entry.item_id}`}>{entry.title}</a>
              {@render estimate(entry)}
              {#if mayWrite && target}
                <IconButton size="sm" icon="arrow-up" label={t('app.sprints.into_planned_of', { title: entry.title, sprint: target.name })} onclick={() => planIn(entry.item_id)} />
              {/if}
            </li>
          {/each}
        </ol>
      {/if}
      {#if mayWrite}
        <div class="capture" data-backlog-capture>
          <CaptureDialog inPlace={{ collectionId, keepsOpen: true }} inPlaceLabel={t('app.sprints.capture_line')} oncreated={() => void rereadBacklog()} />
        </div>
      {/if}
    </section>
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

<style>
  .hint { text-align: end; }
  .block { display: flex; flex-direction: column; min-width: 0; border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .block[data-over] { outline: var(--bw-ring) dashed var(--accent-primary); outline-offset: var(--sp-050); }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100) var(--sp-150); padding: var(--sp-150) var(--sp-200); }
  .folded { padding: 0 var(--sp-200) var(--sp-150); }
  .name { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .grow { flex: 1; }
  .open { display: inline-flex; align-items: center; gap: var(--sp-050); min-block-size: var(--layout-target); color: var(--text-primary); font-weight: var(--fw-medium); }
  .state { padding: 0 var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-sm); font-size: var(--fs-075); color: var(--text-secondary); }
  .state[data-state='ACTIVE'] { color: var(--text-brand); background: var(--accent-primary-subtle); }
  .load { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: min(100%, var(--layout-field-md)); }
  .over { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-warning); font-size: var(--fs-075); }
  .rows { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; }
  .row { display: flex; align-items: center; gap: var(--sp-100); min-block-size: var(--layout-target); padding: var(--sp-050) var(--sp-200); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .row[data-dragging] { opacity: 0.6; }
  .row[data-before] { box-shadow: inset 0 var(--bw-thick) 0 var(--accent-primary); }
  .title { flex: 1; min-width: 0; color: var(--text-primary); overflow-wrap: anywhere; }
  .estimate :global(input) { inline-size: var(--sp-1000); }
  .handle { display: inline-grid; place-items: center; min-inline-size: var(--sp-300); min-block-size: var(--sp-300); padding: 0; border: 0; border-radius: var(--r-sm); background: none; color: var(--text-subtle); cursor: grab; }
  .handle:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .chip { padding: 0 var(--sp-050); border-radius: var(--r-sm); background: var(--bg-surface-sunken); color: var(--text-secondary); font-size: var(--fs-075); }
  .drop { margin: var(--sp-100) var(--sp-200) var(--sp-150); padding: var(--sp-100); border: var(--bw-hairline) dashed var(--border-strong); border-radius: var(--r-md); text-align: center; }
  .empty { padding: var(--sp-100) var(--sp-200) var(--sp-150); }
  .capture { padding: var(--sp-100) var(--sp-200) var(--sp-150); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .head, .row { padding-inline: var(--sp-150); }
    .hint { text-align: start; }
    .row :global(button) { min-inline-size: var(--layout-target); min-block-size: var(--layout-target); }
  }
</style>
