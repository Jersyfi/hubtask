<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Sprints* of a collection (M2, UC-PLN-06, UC-PLN-07): the sprints with their goal, the
  // selected sprint's entries beside the backlog, its burndown and the team's velocity.
  //
  // **The server counts; this screen draws and writes.** Commitment, result, burndown and velocity
  // are the answers of `/sprints/{id}` and `/collections/{id}/velocity`; every start, close, move
  // or estimate is a write, and the sprint is read again after it.
  //
  // **The tables are the keyboard path** (UC-PLN-07 check 5): the burndown's picture has a table
  // of the same days beside it, and every entry's estimate and move is a real control.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS at the collection the
  // screen shows values, not controls.

  import { untrack } from 'svelte';
  import {
    Banner, Button, Checkbox, Dialog, Inline, Input, PageHeader, Select, Skeleton, Stack, Table, Textarea,
    type Column,
  } from '@hubtask/design-system/components';
  import type { Sprint, SprintBacklog, SprintDetail, SprintList, Velocity } from '@hubtask/sync-engine';

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
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** The collection the sprints belong to. */
    collectionId: string;
    onnavigate: (path: string) => void;
  }

  const { collectionId, onnavigate }: Props = $props();

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
  function saveEstimate(itemId: string, title: string) {
    const input = estimateOf(drafts[itemId] ?? '');
    if (input === undefined) {
      failure = t('estimates.value_out_of_range', { maximum: '10000' });
      return;
    }
    void act(() => sprints.estimate(collectionId, zone, chosen?.id, itemId, input), t('app.sprints.estimated', { title }));
  }

  let selected = $state<string[]>([]);
  function toggle(id: string, isOn: boolean) {
    selected = isOn ? [...selected, id] : selected.filter((each) => each !== id);
  }

  let isPlanning = $state(false);
  /** The sprint the dialog changes; undefined while it plans a new one. */
  let editing = $state<Sprint | undefined>(undefined);
  const draft = $state({ name: '', goal: '', start_on: '', end_on: '', unit: 'POINTS' as 'POINTS' | 'HOURS' });
  function openPlan(sprint: Sprint | undefined) {
    editing = sprint;
    draft.name = sprint?.name ?? '';
    draft.goal = sprint?.goal ?? '';
    draft.start_on = sprint?.start_on ?? today;
    draft.end_on = sprint?.end_on ?? '';
    draft.unit = sprint?.unit ?? 'POINTS';
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
    void act(async () => {
      const made = await sprints.create(collectionId, zone, fields);
      chosenId = made.id;
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
  const days = $derived(chosen ? sprintDays(chosen) : 0);
  const top = $derived(chosen && detail ? burndownTop(chosen, detail.burndown) : 0);
  const unitLabel = $derived(chosen ? t(`app.sprints.unit_${chosen.unit}`) : '');
  const states = $derived(list ? list.map((s) => ({ value: s.id, label: t('app.sprints.option', { name: s.name, state: t(`app.sprints.state_${s.state}`) }) })) : []);

  const entryColumns = $derived<Column[]>([
    { id: 'pick', label: t('app.sprints.col_pick'), isLabelHidden: true },
    { id: 'entry', label: t('app.sprints.col_entry') },
    { id: 'estimate', label: t('app.sprints.col_estimate') },
    { id: 'state', label: t('app.sprints.col_state') },
  ]);
  const backlogColumns = $derived<Column[]>([
    { id: 'pick', label: t('app.sprints.col_pick'), isLabelHidden: true },
    { id: 'entry', label: t('app.sprints.col_entry') },
    { id: 'estimate', label: t('app.sprints.col_estimate') },
  ]);
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
  const inSprint = $derived(new Set(open.map((e) => e.item_id)));
  const pickedInSprint = $derived(selected.filter((id) => inSprint.has(id)));
  const pickedInBacklog = $derived(selected.filter((id) => !inSprint.has(id)));
  const today = $derived(todayIn(zone));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.sprints.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={place
      ? {
          trail: [{ id: 'place', label: place.name, href: placePath }, { id: 'sprints', label: t('app.sprints.title') }],
          label: t('app.sprints.trail'),
          expandLabel: t('app.sprints.expand_trail'),
          onnavigate: () => onnavigate(placePath),
        }
      : undefined}
    secondary={mayWrite ? [{ label: t('app.sprints.plan'), icon: 'plus', onclick: () => openPlan(undefined) }] : []}
  />

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !list}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

    {#if list.length === 0}
      <p class="quiet">{t('app.sprints.none')}</p>
    {:else if chosen}
      <div class="controls">
        <Select label={t('app.sprints.sprint')} size="sm" value={chosen.id} options={states}
          onchange={(event: Event) => { chosenId = (event.currentTarget as HTMLSelectElement).value; selected = []; }} />
        {#if mayWrite && chosen.state === 'PLANNED'}
          <Button size="sm" onclick={() => void act(() => sprints.start(collectionId, zone, chosen.id), t('app.sprints.started', { name: chosen.name }))}>{t('app.sprints.start')}</Button>
          <Button size="sm" tone="subtle" icon="trash" onclick={() => { const gone = chosen.id; chosenId = ''; void act(() => sprints.remove(collectionId, zone, gone), t('app.sprints.deleted')); }}>{t('app.sprints.delete')}</Button>
        {/if}
        {#if mayWrite && chosen.state !== 'CLOSED'}
          <Button size="sm" tone="secondary" icon="pencil" onclick={() => openPlan(chosen)}>{t('app.sprints.edit')}</Button>
        {/if}
        {#if mayWrite && chosen.state === 'ACTIVE'}
          <Button size="sm" onclick={() => { carryTo = targets[0]?.id ?? ''; isClosing = true; }}>{t('app.sprints.close')}</Button>
        {/if}
      </div>

      <section class="block" aria-labelledby="sprint-head">
        <h2 id="sprint-head" class="heading">{chosen.name}</h2>
        {#if chosen.goal}<p>{t('app.sprints.goal_is', { goal: chosen.goal })}</p>{/if}
        <p class="quiet">
          {t('app.sprints.span', { from: chosen.start_on, to: chosen.end_on })}
          {#if chosen.state !== 'PLANNED'} · {t('app.sprints.committed', { count: String(chosen.committed_count), value: String(chosen.committed_value), unit: unitLabel })}{/if}
          {#if chosen.state === 'CLOSED'} · {t('app.sprints.completed', { count: String(chosen.completed_count), value: String(chosen.completed_value), unit: unitLabel })}{/if}
        </p>
      </section>

      <div class="columns">
        <section class="block" aria-labelledby="sprint-entries">
          <Inline justify="between" align="center">
            <h3 id="sprint-entries" class="heading">{t('app.sprints.in_sprint', { count: String(open.length) })}</h3>
            {#if mayWrite && chosen.state !== 'CLOSED' && pickedInSprint.length > 0}
              <Button size="sm" tone="secondary" onclick={() => { const ids = pickedInSprint; selected = []; void act(() => sprints.take(collectionId, zone, chosen.id, ids)); }}>{t('app.sprints.to_backlog', { count: String(pickedInSprint.length) })}</Button>
            {/if}
          </Inline>
          {#if !detail}
            <Skeleton lines={3} />
          {:else}
            <Table label={t('app.sprints.in_sprint', { count: String(open.length) })} isLabelHidden columns={entryColumns}>
              {#each open as entry (entry.item_id)}
                <tr>
                  <td>
                    {#if mayWrite && chosen.state !== 'CLOSED'}
                      <Checkbox label={t('app.sprints.pick', { title: entry.title })} isLabelHidden checked={selected.includes(entry.item_id)}
                        onchange={(event: Event) => toggle(entry.item_id, (event.currentTarget as HTMLInputElement).checked)} />
                    {/if}
                  </td>
                  <th scope="row"><a href={`/items/${entry.item_id}`}>{entry.title}</a></th>
                  <td>
                    {#if mayWrite && chosen.state !== 'CLOSED'}
                      <Inline gap="050">
                        <Input label={t('app.sprints.estimate_of', { title: entry.title })} size="sm" value={drafts[entry.item_id] ?? estimateText(entry.estimate)}
                          oninput={(event: Event) => (drafts[entry.item_id] = (event.currentTarget as HTMLInputElement).value)} />
                        <Button size="sm" tone="subtle" onclick={() => saveEstimate(entry.item_id, entry.title)}>{t('app.sprints.save')}</Button>
                      </Inline>
                    {:else}
                      {estimateText(entry.estimate)}
                    {/if}
                  </td>
                  <td>
                    {entry.done ? t('app.sprints.done') : t('app.sprints.open')}
                    {#if entry.outcome}<span class="note">{t(`app.sprints.outcome_${entry.outcome}`)}</span>{/if}
                    {#if !entry.committed && chosen.state !== 'PLANNED'}<span class="note">{t('app.sprints.added_later')}</span>{/if}
                  </td>
                </tr>
              {/each}
            </Table>
          {/if}
        </section>

        {#if chosen.state !== 'CLOSED'}
          <section class="block" aria-labelledby="sprint-backlog">
            <Inline justify="between" align="center">
              <h3 id="sprint-backlog" class="heading">{t('app.sprints.backlog', { count: String(backlog.length) })}</h3>
              {#if mayWrite && pickedInBacklog.length > 0}
                <Button size="sm" onclick={() => { const ids = pickedInBacklog; selected = []; void act(() => sprints.add(collectionId, zone, chosen.id, ids)); }}>{t('app.sprints.to_sprint', { count: String(pickedInBacklog.length) })}</Button>
              {/if}
            </Inline>
            <Table label={t('app.sprints.backlog', { count: String(backlog.length) })} isLabelHidden columns={backlogColumns}>
              {#each backlog as entry (entry.item_id)}
                <tr>
                  <td>
                    {#if mayWrite}
                      <Checkbox label={t('app.sprints.pick', { title: entry.title })} isLabelHidden checked={selected.includes(entry.item_id)}
                        onchange={(event: Event) => toggle(entry.item_id, (event.currentTarget as HTMLInputElement).checked)} />
                    {/if}
                  </td>
                  <th scope="row"><a href={`/items/${entry.item_id}`}>{entry.title}</a></th>
                  <td>
                    {#if mayWrite}
                      <Inline gap="050">
                        <Input label={t('app.sprints.estimate_of', { title: entry.title })} size="sm" value={drafts[entry.item_id] ?? estimateText(entry.estimate)}
                          oninput={(event: Event) => (drafts[entry.item_id] = (event.currentTarget as HTMLInputElement).value)} />
                        <Button size="sm" tone="subtle" onclick={() => saveEstimate(entry.item_id, entry.title)}>{t('app.sprints.save')}</Button>
                      </Inline>
                    {:else}
                      {estimateText(entry.estimate)}
                    {/if}
                  </td>
                </tr>
              {/each}
            </Table>
          </section>
        {/if}
      </div>

      {#if detail && detail.burndown.length > 0}
        <section class="block" aria-labelledby="sprint-burndown">
          <h3 id="sprint-burndown" class="heading">{t('app.sprints.burndown')}</h3>
          <svg class="chart" viewBox="0 0 400 100" preserveAspectRatio="none" role="img" aria-label={t('app.sprints.burndown_label', { remaining: String(detail.burndown.at(-1)?.remaining ?? 0), unit: unitLabel })}>
            <polyline class="ideal" points={burndownLine(detail.burndown, 'ideal', days, top, 400, 100)} />
            <polyline class="scope" points={burndownLine(detail.burndown, 'scope', days, top, 400, 100)} />
            <polyline class="remaining" points={burndownLine(detail.burndown, 'remaining', days, top, 400, 100)} />
          </svg>
          <Table label={t('app.sprints.burndown')} isLabelHidden columns={burndownColumns}>
            {#each detail.burndown as point (point.day)}
              <tr><th scope="row">{point.day}</th><td class="number">{point.remaining}</td><td class="number">{point.ideal}</td><td class="number">{point.scope}</td></tr>
            {/each}
          </Table>
        </section>
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
  <Dialog bind:isOpen={isPlanning} title={editing ? t('app.sprints.edit') : t('app.sprints.plan')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.sprints.name')} bind:value={draft.name} maxlength={120} />
      <Textarea label={t('app.sprints.goal')} bind:value={draft.goal} maxlength={500} />
      <Inline gap="100">
        <Input label={t('app.sprints.start_on')} type="date" bind:value={draft.start_on} disabledReason={editing?.state === 'ACTIVE' ? t('sprints.running_start_fixed') : undefined} />
        <Input label={t('app.sprints.end_on')} type="date" bind:value={draft.end_on} />
      </Inline>
      <Select label={t('app.sprints.unit')} bind:value={draft.unit} disabledReason={editing?.state === 'ACTIVE' ? t('sprints.running_start_fixed') : undefined}
        options={[{ value: 'POINTS', label: t('app.sprints.unit_POINTS') }, { value: 'HOURS', label: t('app.sprints.unit_HOURS') }]} />
      <Inline justify="end"><Button onclick={plan}>{editing ? t('app.sprints.save') : t('app.sprints.plan')}</Button></Inline>
    </Stack>
  </Dialog>
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
  .columns { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, var(--layout-board-column)), 1fr)); gap: var(--sp-200); }
  .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; }
  .chart { width: 100%; height: var(--sp-1600); background: var(--bg-surface-sunken); }
  .chart polyline { fill: none; stroke-width: var(--bw-thick); vector-effect: non-scaling-stroke; }
  .chart .remaining { stroke: var(--accent-primary); }
  .chart .ideal { stroke: var(--border-strong); stroke-dasharray: 4 4; }
  .chart .scope { stroke: var(--text-subtle); }
</style>
