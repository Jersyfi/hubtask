<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Sprints* of a collection (M2, UC-PLN-06, UC-PLN-07): the chosen sprint as a board - the
  // backlog, what is still to do, what is in progress and what is done, an estimate as a chip on each card - with its
  // burndown in the head and the team's velocity below (user-experience.md §7.5).
  //
  // **The server counts; this screen draws and writes.** Commitment, result, burndown and velocity
  // are the answers of `/sprints/{id}` and `/collections/{id}/velocity`; every start, close, move
  // or estimate is a write, and the sprint is read again after it.
  //
  // **The controls are the keyboard path** (UC-PLN-07 check 5): the burndown's picture has a table
  // of the same days under it, and every card's estimate and move is a real control; a card
  // carried by a mouse onto the other lane makes the same move.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS at the collection the
  // screen shows values, not controls.

  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    BucketColumn, Button, Dialog, Disclosure, IconButton, Inline, Input, PageHeader, Select, Skeleton, Stack, Table, Textarea,
    hasLeftTheHandle, type Column, type PageAction,
  } from '@hubtask/design-system/components';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { Sprint, SprintBacklog, SprintDetail, SprintEntry, SprintList, Velocity } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import {
    backlogPath, burndownLine, burndownTop, carryTargets, estimateOf, estimateText, sprintDays, sprintPath,
    sprints, sprintShown, sprintsPath, velocityPath,
  } from '../lib/data/sprints.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';
  import PlaceStrip from '../lib/workspace/PlaceStrip.svelte';
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
  const placePath = $derived(`/collections/${collectionId}`);
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

  /** A card into the sprint from the backlog, or back. */
  function moveCard(itemId: string, from: 'backlog' | 'sprint') {
    const sprint = chosen;
    if (!sprint) return;
    void act(() => (from === 'backlog' ? sprints.add(collectionId, zone, sprint.id, [itemId]) : sprints.take(collectionId, zone, sprint.id, [itemId])));
  }

  // A card carried by a fine pointer onto the other lane. A finger scrolls the board; its way is
  // the card's arrow, as the keyboard's is.
  let dragging = $state<{ id: string; from: 'backlog' | 'sprint' } | undefined>(undefined);
  let over = $state<string | undefined>(undefined);
  function beginDrag(event: PointerEvent, id: string, from: 'backlog' | 'sprint') {
    if (event.pointerType !== 'mouse' || event.button !== 0) return;
    if ((event.target as HTMLElement | null)?.closest('a, button, input')) return;
    const origin = { x: event.clientX, y: event.clientY };
    const onMove = (move: PointerEvent) => {
      if (!dragging) {
        if (!hasLeftTheHandle(origin, { x: move.clientX, y: move.clientY })) return;
        dragging = { id, from };
      }
      const lane = document.elementFromPoint(move.clientX, move.clientY)?.closest<HTMLElement>('[data-drop]');
      over = lane?.dataset['drop'];
    };
    const finish = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', finish);
      window.removeEventListener('pointercancel', finish);
      const landed = over;
      const carried = dragging;
      dragging = undefined;
      over = undefined;
      if (carried && landed && landed !== carried.from) moveCard(carried.id, carried.from);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', finish);
    window.addEventListener('pointercancel', finish);
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
  const noteOf = (entry: SprintEntry) =>
    entry.outcome ? t(`app.sprints.outcome_${entry.outcome}`) : !entry.committed && chosen?.state !== 'PLANNED' ? t('app.sprints.added_later') : undefined;
  const days = $derived(chosen ? sprintDays(chosen) : 0);
  const top = $derived(chosen && detail ? burndownTop(chosen, detail.burndown) : 0);
  const unitLabel = $derived(chosen ? t(`app.sprints.unit_${chosen.unit}`) : '');
  const states = $derived(list ? list.map((s) => ({ value: s.id, label: t('app.sprints.option', { name: s.name, state: t(`app.sprints.state_${s.state}`) }) })) : []);

  const burndownColumns = $derived<Column[]>([
    { id: 'day', label: t('app.sprints.col_day') },
    { id: 'remaining', label: t('app.sprints.col_remaining'), align: 'end' },
    { id: 'ideal', label: t('app.sprints.col_ideal'), align: 'end' },
    { id: 'scope', label: t('app.sprints.col_scope'), align: 'end' },
  ]);
  const velocityColumns = $derived<Column[]>([
    { id: 'sprint', label: t('app.sprints.col_sprint') },
    { id: 'committed', label: t('app.sprints.col_committed'), align: 'end' },
    { id: 'completed', label: t('app.sprints.col_completed'), align: 'end' },
  ]);
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

  const headActs = $derived.by((): readonly [] | readonly [PageAction] => {
    const sprint = chosen;
    if (!mayWrite || !sprint) return [];
    if (sprint.state === 'PLANNED') {
      return [{ label: t('app.sprints.start'), icon: 'play' as const, onclick: (): void => void act(() => sprints.start(collectionId, zone, sprint.id), t('app.sprints.started', { name: sprint.name })) }];
    }
    if (sprint.state === 'ACTIVE') return [{ label: t('app.sprints.close'), icon: 'circle-check' as const, onclick: () => { carryTo = targets[0]?.id ?? ''; isClosing = true; } }];
    return [];
  });
  const headMenu = $derived([
    ...(mayWrite && chosen && chosen.state !== 'CLOSED' ? [{ id: 'edit', label: t('app.sprints.edit'), icon: 'pencil' as const }] : []),
    ...(mayWrite && chosen?.state === 'PLANNED' ? [{ id: 'delete', label: t('app.sprints.delete'), icon: 'trash' as const, hasSeparatorBefore: true }] : []),
  ]);
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

{#snippet card(entry: { item_id: string; title: string; estimate: SprintEntry['estimate'] }, where: 'backlog' | 'sprint' | 'done', note?: string)}
  {@const isMovable = mayWrite && chosen !== undefined && chosen.state !== 'CLOSED'}
  <li
    class="card"
    data-card={entry.item_id}
    data-dragging={dragging?.id === entry.item_id ? '' : undefined}
    onpointerdown={isMovable && where !== 'done' ? (event) => beginDrag(event, entry.item_id, where) : undefined}
  >
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
        <IconButton
          size="sm"
          icon={where === 'backlog' ? 'arrow-right' : 'arrow-left'}
          label={where === 'backlog' ? t('app.sprints.into_sprint_of', { title: entry.title }) : t('app.sprints.to_backlog_of', { title: entry.title })}
          onclick={() => moveCard(entry.item_id, where)}
        />
        {/if}
      {:else if entry.estimate}
        <span class="chip">{estimateText(entry.estimate)}</span>
      {/if}
    </span>
  </li>
{/snippet}

<Stack gap="200">
  <PageHeader
    title={chosen?.name ?? t('app.sprints.title')}
    isTitleInBar={viewport.isCompact}
    subtitle={figure}
    figure={detail && detail.burndown.length > 0 ? burndownPicture : undefined}
    breadcrumb={place
      ? {
          trail: [{ id: 'place', label: place.name, href: placePath }, { id: 'sprints', label: t('app.sprints.title') }],
          label: t('app.sprints.trail'),
          expandLabel: t('app.sprints.expand_trail'),
          onnavigate: () => onnavigate(placePath),
        }
      : undefined}
    primary={mayWrite ? { label: t('app.sprints.plan'), icon: 'plus', onclick: () => openPlan(undefined) } : undefined}
    secondary={headActs}
    menu={headMenu.length > 0 ? { label: t('app.sprints.more'), items: headMenu, onselect: onMenu } : undefined}
  />
  <PlaceStrip placeId={collectionId} current="sprints" {onnavigate} />

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !list}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

    {#if list.length === 0}
      <ProductEmpty product="PLANNING" place={{ type: 'COLLECTION', id: collectionId }} kind="unused" title={t('app.sprints.none_title')} description={t('app.sprints.none_hint')} create={mayWrite ? { label: t('app.sprints.plan'), onclick: () => openPlan(undefined) } : undefined} />
    {:else if chosen}
      <div class="controls">
        {#if list.length > 1}
          <Select label={t('app.sprints.sprint')} size="sm" value={chosen.id} options={states}
            onchange={(event: Event) => { chosenId = (event.currentTarget as HTMLSelectElement).value; }} />
        {/if}
        {#if chosen.goal}<p class="goal">{t('app.sprints.goal_is', { goal: chosen.goal })}</p>{/if}
      </div>

      <div class="board" data-sprint-board data-closed={chosen.state === 'CLOSED' ? '' : undefined}>
        {#if chosen.state !== 'CLOSED'}
          <div class="lane" data-drop="backlog" data-over={over === 'backlog' ? '' : undefined}>
            <BucketColumn name={t('app.sprints.backlog_column')} count={backlog.length}>
              {#if backlog.length === 0}
                <p class="quiet">{t('app.sprints.backlog_empty')}</p>
              {:else}
                <ul class="cards">
                  {#each backlog as entry (entry.item_id)}{@render card(entry, 'backlog')}{/each}
                </ul>
              {/if}
            </BucketColumn>
          </div>
        {/if}
        <div class="lane" data-drop="sprint" data-over={over === 'sprint' ? '' : undefined}>
          <BucketColumn name={t('app.sprints.todo_column')} count={detail ? toDo.length : null}>
            {#if !detail}
              <Skeleton lines={3} />
            {:else if toDo.length === 0}
              <p class="quiet">{chosen.state === 'CLOSED' ? t('app.sprints.todo_none_closed') : t('app.sprints.todo_empty')}</p>
            {:else}
              <ul class="cards">
                {#each toDo as entry (entry.item_id)}{@render card(entry, 'sprint', noteOf(entry))}{/each}
              </ul>
            {/if}
          </BucketColumn>
        </div>
        {#if hasStartedLane}
          <div class="lane" data-lane="in-progress">
            <BucketColumn name={t('app.sprints.in_progress_column')} count={detail ? inProgress.length : null}>
              {#if !detail}
                <Skeleton lines={2} />
              {:else if inProgress.length === 0}
                <p class="quiet">{t('app.sprints.in_progress_empty')}</p>
              {:else}
                <ul class="cards">
                  {#each inProgress as entry (entry.item_id)}{@render card(entry, 'sprint', noteOf(entry))}{/each}
                </ul>
              {/if}
            </BucketColumn>
          </div>
        {/if}
        <div class="lane">
          <BucketColumn name={t('app.sprints.done_column')} count={detail ? done.length : null} isDoneBucket doneBucketLabel={t('app.sprints.done_column_hint')}>
            {#if detail && done.length === 0}
              <p class="quiet">{t('app.sprints.done_empty')}</p>
            {:else if detail}
              <ul class="cards">
                {#each done as entry (entry.item_id)}{@render card(entry, 'done', noteOf(entry))}{/each}
              </ul>
            {/if}
          </BucketColumn>
        </div>
      </div>

      {#if detail && detail.burndown.length > 0}
        <Disclosure label={t('app.sprints.burndown')} description={t('app.sprints.burndown_hint')} tone="quiet">
          <Table label={t('app.sprints.burndown')} isLabelHidden columns={burndownColumns}>
            {#each detail.burndown as point (point.day)}
              <tr><th scope="row">{formatPlainDay(point.day, messages.locale)}</th><td class="number">{point.remaining}</td><td class="number">{point.ideal}</td><td class="number">{point.scope}</td></tr>
            {/each}
          </Table>
        </Disclosure>
      {/if}
    {/if}

    {#if velocity && velocity.sprints.length > 0}
      <section class="block" aria-labelledby="sprint-velocity">
        <h2 id="sprint-velocity" class="heading">{t('app.sprints.velocity', { value: String(velocity.average), unit: t(`app.sprints.unit_${velocity.unit}`) })}</h2>
        <Table label={t('app.sprints.velocity_table')} isLabelHidden columns={velocityColumns}>
          {#each velocity.sprints as s (s.id)}
            <tr><th scope="row">{s.name}</th><td class="number">{s.committed_value}</td><td class="number">{s.completed_value}</td></tr>
          {/each}
        </Table>
      </section>
    {/if}
  {/if}
</Stack>

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
  <Dialog bind:isOpen={isClosing} title={t('app.sprints.close_title', { name: chosen.name })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.sprints.close_hint')}</p>
      <Select label={t('app.sprints.carry_to')} bind:value={carryTo}
        options={[{ value: '', label: t('app.sprints.carry_backlog') }, ...targets.map((s) => ({ value: s.id, label: s.name }))]} />
      <Inline justify="end"><Button onclick={close}>{t('app.sprints.close')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .controls { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .board { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-board-column)), var(--layout-board-column))); gap: var(--sp-200); align-items: start; }
  .lane { min-width: 0; border-radius: var(--r-md); }
  .lane[data-over] { outline: var(--bw-ring) dashed var(--accent-primary); outline-offset: var(--sp-050); }
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
  .card[data-dragging] { opacity: 0.6; }
  .card-title { color: var(--text-primary); font-weight: var(--fw-medium); }
  .card-foot { display: flex; align-items: start; justify-content: space-between; gap: var(--sp-100); }
  .card-foot :global(input) { inline-size: var(--sp-1000); }
  .chip { padding: 0 var(--sp-050); border-radius: var(--r-sm); background: var(--bg-surface-sunken); color: var(--text-secondary); font-size: var(--fs-075); }
  .goal { margin: 0; }
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
