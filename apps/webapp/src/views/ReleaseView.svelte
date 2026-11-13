<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Releases* of a hub (M2, UC-PLN-08, scene planrel): the head says what is coming, the next one
  // and the sprint running; the releases stand on an axis of months, listed beside the chosen one,
  // which carries *Publish*, how much is done, its entries and *Add entries* from every collection
  // of the hub. A release has no trash, so deleting it - under the head's ⋯ - asks (§5.9).
  //
  // **The server ships; this screen picks and writes.** What is done, where open work goes and
  // the notes page are the answers of `/releases/{id}` and `:publish`; the release is read again
  // after every write.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS at the hub the screen
  // shows the releases and their entries, not controls.

  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    Button, Checkbox, Dialog, Icon, IconButton, Input, ProgressBar, Select, Skeleton, Stack, StateLozenge, Textarea,
    DataTable, WorkPage, type DataColumn,
  } from '@hubtask/design-system/components';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { Release, ReleaseDetail, ReleaseList, SprintList } from '@hubtask/sync-engine';
  import { referenceState } from '../lib/data/links.ts';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { releaseAxis, releaseCandidates, releaseGroups, releasePath, releases, releaseShown, releasesPath, releaseTargets } from '../lib/data/releases.svelte.ts';
  import { sprintsPath } from '../lib/data/sprints.svelte.ts';
  import { sprintsRunning } from '../lib/product/home.ts';
  import { sprintDaysLeft } from '../lib/today.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { formatDay, formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ToolHead from '../lib/workspace/ToolHead.svelte';

  interface Props {
    /** The hub the releases belong to. */
    hubId: string;
    onnavigate: (path: string) => void;
  }

  const { hubId, onnavigate }: Props = $props();

  const place = $derived(containers.find(hubId));
  const rolePath = $derived({ hubId });
  const collections = $derived(containers.collectionsOf(hubId));

  $effect(() => frame.entitle(place ? t('app.releases.title_of', { place: place.name }) : t('app.releases.title')));
  $effect(() => {
    const path = rolePath;
    untrack(() => people.open(path));
  });
  $effect(() => {
    const id = hubId;
    return untrack(() => containers.openLevel(id));
  });

  const mayWrite = $derived(holds(people.roleOf(rolePath, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted');

  let list = $state<ReleaseList['data'] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = releasesPath(hubId);
    return untrack(() => engine.subscribe<ReleaseList>({ path }, (next) => {
      if (next.status === 'ready') { list = next.data.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });

  let chosenId = $state('');
  const chosen = $derived(list?.find((r) => r.id === chosenId) ?? (list ? releaseShown(list) : undefined));
  let detail = $state<ReleaseDetail | undefined>(undefined);
  $effect(() => {
    const id = chosen?.id;
    detail = undefined;
    if (!id) return;
    const path = releasePath(id);
    return untrack(() => engine.subscribe<ReleaseDetail>({ path }, (next) => { if (next.status === 'ready') detail = next.data; }));
  });

  // What is done of every release, for its mark and its card: the list counts them itself.
  const counted = $derived<Record<string, { done: number; all: number }>>(Object.fromEntries((list ?? []).map((r) => [r.id, { done: r.done_count, all: r.done_count + r.open_count }])));

  // The sprint running in one of the hub's collections, for the head sentence.
  const collectionKey = $derived(collections.map((c) => c.id).join(','));
  let sprintLists = $state<Record<string, SprintList['data']>>({});
  $effect(() => {
    const ids = collectionKey === '' ? [] : collectionKey.split(',');
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<SprintList>({ path: `${sprintsPath(id)}?state=ACTIVE` }, (next) => {
        if (next.status === 'ready') sprintLists = { ...sprintLists, [id]: next.data.data };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });
  const runningSprint = $derived(sprintsRunning(sprintLists, collections.map((c) => c.id), 1)[0]);

  // *Add entries* (scene planrel 4): the entries of every collection of the hub, or of one.
  let isAdding = $state(false);
  let sourceId = $state('');
  let offeredBy = $state<Record<string, { id: string; title: string; collection_id: string }[]>>({});
  $effect(() => {
    const ids = isAdding && collectionKey !== '' ? collectionKey.split(',') : [];
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<{ data: { id: string; title: string }[] }>({ path: `/items?collection_id=${id}&page_size=100` }, (next) => {
        if (next.status === 'ready') offeredBy = { ...offeredBy, [id]: next.data.data.map((e) => ({ id: e.id, title: e.title, collection_id: id })) };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });
  const offered = $derived(collections.filter((c) => !sourceId || c.id === sourceId).flatMap((c) => offeredBy[c.id] ?? []));
  const candidates = $derived(releaseCandidates(offered, (detail?.entries ?? []).map((e) => e.item_id)));

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

  let selected = $state<string[]>([]);

  let isPlanning = $state(false);
  let editing = $state<Release | undefined>(undefined);
  const draft = $state({ name: '', description: '', release_on: '' });
  const today = $derived(todayIn(actor.zone));
  // A refusal while a dialog is open is said inside it, and a missing field at the field (§5.10).
  let dialogFailure = $state<string | undefined>(undefined);
  let isTried = $state(false);
  const nameError = $derived(isTried && !draft.name.trim() ? t('app.releases.name_needed') : undefined);
  const dayError = $derived(isTried && !draft.release_on ? t('app.releases.day_needed') : undefined);
  function openPlan(release: Release | undefined) {
    editing = release;
    draft.name = release?.name ?? '';
    draft.description = release?.description ?? '';
    draft.release_on = release?.release_on ?? today;
    dialogFailure = undefined;
    isTried = false;
    isPlanning = true;
  }
  async function inDialog(run: () => Promise<unknown>, done: string, close: () => void) {
    dialogFailure = undefined;
    try {
      await run();
      close();
      toast.show({ text: done, tone: 'success' });
    } catch (error) {
      dialogFailure = renderProblem(error as never, messages).message;
    }
  }
  function plan() {
    isTried = true;
    const fields = { ...draft, name: draft.name.trim() };
    if (!fields.name || !fields.release_on) return;
    const changing = editing;
    const close = () => (isPlanning = false);
    if (changing) {
      void inDialog(() => releases.update(hubId, changing, fields), t('app.releases.changed', { name: fields.name }), close);
      return;
    }
    void inDialog(async () => {
      const made = await releases.create(hubId, fields);
      chosenId = made.id;
    }, t('app.releases.planned', { name: fields.name }), close);
  }

  // A release has no trash to come back from, so its deletion asks, naming what it costs (§5.9).
  let isDeleting = $state(false);
  function remove() {
    const gone = chosen;
    if (!gone) return;
    void inDialog(async () => {
      await releases.remove(hubId, gone.id);
      chosenId = '';
      selected = [];
    }, t('app.releases.deleted'), () => (isDeleting = false));
  }

  let isReleasing = $state(false);
  let carryTo = $state('');
  let writeNotes = $state(true);
  const targets = $derived(list && chosen ? releaseTargets(list, chosen.id) : []);
  function publish() {
    const release = chosen;
    if (!release) return;
    void inDialog(() => releases.publish(hubId, release.id, writeNotes, carryTo || undefined), t('app.releases.released', { name: release.name }), () => (isReleasing = false));
  }

  // The head's figure (§5.1): what is coming, the next one and its day, what went out.
  const groups = $derived(list ? releaseGroups(list, today) : undefined);
  const figureText = $derived.by(() => {
    if (!groups || (groups.planned.length === 0 && groups.released.length === 0)) return undefined;
    const next = groups.planned[0];
    return [
      groups.planned.length > 0 ? t('app.releases.figure_planned', { count: String(groups.planned.length) }) : undefined,
      next ? t(groups.late.has(next.id) ? 'app.releases.figure_late' : 'app.releases.figure_next', { name: next.name, day: formatPlainDay(next.release_on, messages.locale) }) : undefined,
      groups.released.length > 0 ? t('app.releases.figure_released', { count: String(groups.released.length) }) : undefined,
      runningSprint ? t('app.releases.figure_sprint', { name: runningSprint.name, count: String(sprintDaysLeft(runningSprint.end_on, today)) }) : undefined,
    ].filter((part): part is string => !!part).join(' · ');
  });
  const figureNext = $derived(groups?.planned[0]);
  const isPlanned = $derived(chosen?.state === 'PLANNED');
  const mayChange = $derived(mayWrite && isPlanned);

  const entryColumns = $derived<DataColumn[]>([
    { id: 'entry', label: t('app.releases.col_entry'), priority: 1 },
    { id: 'type', label: t('app.releases.col_type'), priority: 2 },
    { id: 'collection', label: t('app.releases.col_collection'), priority: 3 },
    { id: 'state', label: t('app.releases.col_state'), priority: 1, isStatus: true },
    ...(mayChange ? [{ id: 'out', label: '', priority: 1 as const }] : []),
  ]);
  const collectionName = (id: string) => containers.find(id)?.name ?? '';
  const axis = $derived(list ? releaseAxis(list, today) : undefined);
  const monthLabel = (day: string) => new Intl.DateTimeFormat(messages.locale, { month: 'short', timeZone: 'UTC' }).format(Date.parse(`${day}T00:00:00Z`));
  const dayShort = (day: string) => new Intl.DateTimeFormat(messages.locale, { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(Date.parse(`${day}T00:00:00Z`));
  const shownDay = (r: Release) => (r.state === 'RELEASED' && r.released_at ? r.released_at.slice(0, 10) : r.release_on);
  const percentOf = (id: string) => { const c = counted[id]; return c && c.all > 0 ? Math.round((c.done / c.all) * 100) : 0; };
  const daysTo = (day: string) => Math.round((Date.parse(`${day}T00:00:00Z`) - Date.parse(`${today}T00:00:00Z`)) / 86_400_000);
  function choose(id: string) {
    chosenId = id;
    selected = [];
  }
  function startPublish() {
    carryTo = targets[0]?.id ?? '';
    writeNotes = true;
    dialogFailure = undefined;
    isReleasing = true;
  }
  const headMenu = $derived(mayChange
    ? { label: t('app.releases.more'), items: [{ id: 'delete', label: t('app.releases.delete'), icon: 'trash' as const, isDestructive: true }], onselect: () => { dialogFailure = undefined; isDeleting = true; } }
    : undefined);
  function addPicked() {
    const release = chosen;
    const ids = [...selected];
    if (!release || ids.length === 0) return;
    void inDialog(() => releases.add(hubId, release.id, ids), t('app.releases.added', { count: String(ids.length) }), () => { isAdding = false; selected = []; });
  }
  type InRelease = NonNullable<typeof detail>['entries'][number];
</script>

{#snippet headFigure()}<span data-head-figure>{#if groups && groups.planned.length > 0}{t('app.releases.figure_planned', { count: String(groups.planned.length) })}{/if}{#if figureNext}{groups && groups.planned.length > 0 ? ' · ' : ''}<b>{t(groups?.late.has(figureNext.id) ? 'app.releases.figure_late' : 'app.releases.figure_next', { name: figureNext.name, day: formatPlainDay(figureNext.release_on, messages.locale) })}</b>{/if}{#if groups && groups.released.length > 0}{' · '}{t('app.releases.figure_released', { count: String(groups.released.length) })}{/if}{#if runningSprint}{' · '}{t('app.releases.figure_sprint', { name: runningSprint.name, count: String(sprintDaysLeft(runningSprint.end_on, today)) })}{/if}</span>{/snippet}
<WorkPage>
  {#snippet top()}
  <ToolHead {hubId} tool="plan" current="releases" {onnavigate}
    figure={figureText ? headFigure : undefined}
    primary={mayWrite ? { label: t('app.releases.plan'), icon: 'plus', onclick: () => openPlan(undefined) } : undefined}
    menu={headMenu}
  />
  {/snippet}

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !list}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else if list.length === 0}
    <ProductEmpty product="PLANNING" place={{ type: 'HUB', id: hubId }}
      kind="unused"
      title={t('app.releases.none')}
      description={t('app.releases.none_hint')}
      create={mayWrite ? { label: t('app.releases.plan'), onclick: () => openPlan(undefined) } : undefined}
    />
  {:else if chosen}
    {#if failure}<FailureNote problem={failure} />{/if}

    <!-- The releases on an axis of months (scene planrel 2); a mark chooses its release. -->
    {#if axis && !viewport.isCompact}
      <!-- design-system-lint-ignore: a mark's place on the axis is the fraction its day is. -->
      <section class="axis" aria-label={t('app.releases.axis')} data-release-axis style:--lanes={String(Math.max(1, ...axis.marks.map((m) => m.lane + 1)))}>
        {#each axis.months as month (month.day)}
          <!-- design-system-lint-ignore: the month's place is the fraction of the axis it starts at. -->
          <span class="month" style:inset-inline-start={`${month.at * 100}%`}>{monthLabel(month.day)}</span>
        {/each}
        <!-- design-system-lint-ignore: today's place is the fraction of the axis it is. -->
        <i class="today" aria-hidden="true" style:inset-inline-start={`${axis.today * 100}%`}></i>
        {#each axis.marks as mark (mark.id)}
          {@const release = list.find((r) => r.id === mark.id)}
          {#if release}
            {@const count = counted[release.id]}
            <!-- design-system-lint-ignore: the mark's place is the fraction its day is; its lane the row it takes. -->
            <button type="button" class="mark" data-state={groups?.late.has(release.id) ? 'late' : release.state} aria-pressed={release.id === chosen.id} data-release-mark={release.id}
              style:inset-inline-start={`${mark.at * 100}%`} style:--lane={String(mark.lane)} onclick={() => choose(release.id)}>
              <i class="diamond" aria-hidden="true"></i>
              <span class="mark-text"><b>{release.name}</b>
                <span>{release.state === 'RELEASED' ? t('app.releases.mark_released', { day: dayShort(shownDay(release)) }) : count ? t('app.releases.mark_planned', { day: dayShort(release.release_on), done: String(count.done), all: String(count.all) }) : dayShort(release.release_on)}</span></span>
            </button>
          {/if}
        {/each}
      </section>
    {/if}

    <div class="layout">
      <!-- Every release as a card beside the chosen one: its state, day and how much is done. -->
      <ul class="cards" aria-label={t('app.releases.title')}>
        {#each [...(groups?.planned ?? []), ...(groups?.released ?? [])] as release (release.id)}
          <li>
            <button type="button" class="release" aria-pressed={release.id === chosen.id} data-release-card={release.id} onclick={() => choose(release.id)}>
              <span class="release-name"><Icon name="tag" size="sm" /><b>{release.name}</b></span>
              <span class="quiet">{t('app.releases.option', { name: t(groups?.late.has(release.id) ? 'app.releases.badge_late' : `app.releases.badge_${release.state}`), state: dayShort(shownDay(release)) })}</span>
              <ProgressBar size="sm" label={t('app.releases.progress', { name: release.name })} value={percentOf(release.id)} />
            </button>
          </li>
        {/each}
        {#if mayWrite}
          <li><Button tone="subtle" icon="plus" onclick={() => openPlan(undefined)}>{t('app.releases.plan')}</Button></li>
        {/if}
      </ul>

      {#if chosen}
      {@const shown = chosen}
      {@const isLate = groups?.late.has(shown.id) ?? false}
      <section class="card" aria-label={shown.name} data-release-chosen={shown.id}>
        <div class="card-head">
          <div class="card-what">
            <span class="line">
              <StateLozenge vocabulary="release" state={isLate ? 'late' : shown.state === 'RELEASED' ? 'released' : 'planned'} size="sm"
                label={t(isLate ? 'app.releases.badge_late' : `app.releases.badge_${shown.state}`)} />
              <span class="quiet">
                {#if shown.state === 'RELEASED' && shown.released_at}{t('app.releases.released_on', { day: formatDay(shown.released_at, messages.locale) })}
                {:else}{t('app.releases.due_on', { day: formatPlainDay(shown.release_on, messages.locale) })} · {t('app.releases.in_days', { count: String(daysTo(shown.release_on)) })}{/if}
              </span>
            </span>
            <h2 class="name">{shown.name}</h2>
            {#if shown.description}<p class="about">{shown.description}</p>{/if}
          </div>
          {#if mayChange}
            <Button tone="subtle" icon="pencil" onclick={() => openPlan(shown)}>{t('app.releases.change')}</Button>
            <Button icon="send" data-release-publish onclick={startPublish}>{t('app.releases.publish')}</Button>
          {/if}
        </div>

        {#if detail}
          <div class="progress">
            <span class="bar"><ProgressBar label={t('app.releases.progress', { name: shown.name })} value={percentOf(shown.id)} /></span>
            <span data-release-done>{t('app.releases.counts', { done: String(detail.done_count), open: String(detail.open_count) })}</span>
            {#if shown.state === 'PLANNED'}<span class="quiet">· {t('app.releases.notes_coming')}</span>
            {:else if shown.notes_page_id}<a href={`/pages/${shown.notes_page_id}`}>{t('app.releases.notes')}</a>{/if}
          </div>
        {/if}

        <div class="entries-head">
          <h3 id="release-entries" class="label">{t('app.releases.in_release', { count: String(detail?.entries.length ?? 0) })}</h3>
          {#if mayChange && collections.length > 0}
            <Button size="sm" tone="subtle" icon="plus" data-release-add onclick={() => { selected = []; dialogFailure = undefined; isAdding = true; }}>{t('app.releases.add_entries')}</Button>
          {/if}
        </div>
        {#if !detail}
          <Skeleton lines={3} />
        {:else if detail.entries.length === 0}
          <ProductEmpty kind="settled" title={t(mayChange ? 'app.releases.empty_add' : 'app.releases.empty')} />
        {:else}
          <DataTable label={t('app.releases.in_release', { count: String(detail.entries.length) })} columns={entryColumns} rows={detail.entries}
            key={(entry: InRelease) => entry.item_id} rowLabel={(entry: InRelease) => entry.title}>
            {#snippet cell(entry: InRelease, column: DataColumn)}
              {#if column.id === 'entry'}{#if entry.key}<span class="key">{entry.key}</span> {/if}<a href={`/items/${entry.item_id}`}>{entry.title}</a>
              {:else if column.id === 'type'}{entry.type_name}
              {:else if column.id === 'collection'}{collectionName(entry.collection_id)}
              {:else if column.id === 'state'}{@const state = referenceState({ completed: entry.done, status_name: entry.status_name, status_category: entry.status_category }, t)}<StateLozenge size="sm" vocabulary="category" state={state.state} label={state.label} />
              {:else}<IconButton size="sm" icon="x" label={t('app.releases.take_out', { title: entry.title })} onclick={() => void act(() => releases.take(hubId, shown.id, [entry.item_id]))} />{/if}
            {/snippet}
          </DataTable>
        {/if}
        <p class="quiet" data-release-query>{t('app.releases.from_everywhere')} <code>plan.release: {shown.id}</code></p>
      </section>
      {/if}
    </div>
  {/if}
</WorkPage>

{#if isAdding && chosen}
  <Dialog size="m" bind:isOpen={isAdding} title={t('app.releases.add_entries')} dismissLabel={t('app.links.cancel')}
    primary={{ label: t('app.releases.add', { count: String(selected.length) }), onclick: addPicked }}>
    <Stack gap="150">
      {#if collections.length > 1}
        <Select label={t('app.releases.from')} size="sm" value={sourceId}
          options={[{ value: '', label: t('app.releases.all_collections') }, ...collections.map((c) => ({ value: c.id, label: c.name }))]}
          onchange={(event: Event) => { sourceId = (event.currentTarget as HTMLSelectElement).value; }} />
      {/if}
      {#if candidates.length === 0}
        <ProductEmpty kind="settled" title={t('app.releases.all_in')} />
      {:else}
        <!-- A list to tick, not a table: a dialog holds one choice, and a table's sticky head
             would stand over its first row. -->
        <ul class="picks" aria-label={t('app.releases.candidates')} data-release-candidates>
          {#each candidates as entry (entry.id)}
            <li>
              <Checkbox label={entry.title} checked={selected.includes(entry.id)}
                onchange={(event: Event) => { const on = (event.currentTarget as HTMLInputElement).checked; selected = on ? [...selected, entry.id] : selected.filter((id) => id !== entry.id); }} />
              <span class="quiet">{collectionName(entry.collection_id)}</span>
            </li>
          {/each}
        </ul>
      {/if}
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isAdding = false)}>{t('app.links.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isPlanning}
  <Dialog size="m" bind:isOpen={isPlanning} title={editing ? t('app.releases.edit') : t('app.releases.plan')} dismissLabel={t('app.links.cancel')} primary={{ label: editing ? t('app.releases.save') : t('app.releases.plan'), onclick: plan }}>
    <Stack gap="150">
      <Input label={t('app.releases.name')} bind:value={draft.name} maxlength={120} isRequired error={nameError} />
      <Textarea label={t('app.releases.description')} bind:value={draft.description} maxlength={2000} />
      <Input label={t('app.releases.release_on')} type="date" bind:value={draft.release_on} isRequired error={dayError} />
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isPlanning = false)}>{t('app.links.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isReleasing && chosen}
  <Dialog size="m" bind:isOpen={isReleasing} title={t('app.releases.publish_title', { name: chosen.name })} dismissLabel={t('app.links.cancel')} primary={{ label: t('app.releases.publish'), onclick: publish }}>
    <Stack gap="150">
      <p class="quiet">{t('app.releases.publish_hint')}</p>
      {#if detail}<p>{t('app.releases.counts', { done: String(detail.done_count), open: String(detail.open_count) })}</p>{/if}
      <Select label={t('app.releases.carry_to')} bind:value={carryTo}
        options={[{ value: '', label: t('app.releases.carry_out') }, ...targets.map((r) => ({ value: r.id, label: r.name }))]} />
      <Checkbox label={t('app.releases.write_notes')} bind:checked={writeNotes} />
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isReleasing = false)}>{t('app.links.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isDeleting && chosen}
  <Dialog size="s" bind:isOpen={isDeleting} title={t('app.releases.delete_title', { name: chosen.name })} dismissLabel={t('app.links.cancel')} primary={{ label: t('app.releases.delete'), onclick: remove, tone: 'danger' }}>
    <Stack gap="150">
      <p>{t('app.releases.delete_cost', { count: String(detail?.entries.length ?? 0) })}</p>
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDeleting = false)}>{t('app.releases.keep')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .axis { position: relative; block-size: calc(var(--sp-500) + var(--lanes) * var(--sp-500)); padding-block-start: var(--sp-300); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); overflow: hidden; }
  .month { position: absolute; inset-block: 0; padding: var(--sp-050) var(--sp-100); border-inline-start: var(--bw-hairline) solid var(--border-subtle); color: var(--text-subtle); font-size: var(--fs-065); }
  .today { position: absolute; inset-block: 0; inline-size: var(--bw-thick); background: var(--status-danger-accent); }
  .mark { position: absolute; inset-block-start: calc(var(--sp-400) + var(--lane) * var(--sp-500)); display: flex; align-items: flex-start; gap: var(--sp-050); padding: 0; border: none; font: inherit; text-align: start; color: var(--text-primary); background: transparent; cursor: pointer; }
  .diamond { flex: none; inline-size: var(--sp-150); block-size: var(--sp-150); margin-block-start: var(--sp-025); margin-inline-start: calc(var(--sp-150) / -2); transform: rotate(45deg); background: var(--text-subtle); }
  .mark[data-state='RELEASED'] .diamond { background: var(--status-success-accent); }
  .mark[data-state='PLANNED'] .diamond { background: var(--accent-primary); }
  .mark[data-state='late'] .diamond { background: var(--status-warning-accent); }
  .mark-text { display: flex; flex-direction: column; font-size: var(--fs-075); white-space: nowrap; }
  .mark-text span { color: var(--text-secondary); }
  .mark[aria-pressed='true'] b { color: var(--text-brand); }
  .layout { display: grid; grid-template-columns: minmax(0, var(--layout-board-column)) minmax(0, 1fr); gap: var(--sp-300); align-items: start; }
  .cards { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .release { display: flex; flex-direction: column; gap: var(--sp-050); inline-size: 100%; padding: var(--sp-150) var(--sp-200); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-md); font: inherit; text-align: start; color: var(--text-primary); background: var(--bg-surface); cursor: pointer; }
  .release[aria-pressed='true'] { border-color: var(--accent-primary); box-shadow: 0 0 0 var(--bw-hairline) var(--accent-primary); }
  .release-name { display: inline-flex; align-items: center; gap: var(--sp-050); }
  .release-name b { font-weight: var(--fw-semibold); }
  .card { display: flex; flex-direction: column; gap: var(--sp-200); padding: var(--sp-300); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); min-inline-size: 0; }
  .card-head { display: flex; flex-wrap: wrap; align-items: flex-start; gap: var(--sp-150); }
  .card-what { display: flex; flex: 1; flex-direction: column; gap: var(--sp-050); min-inline-size: 0; }
  .line { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .name { margin: 0; font-size: var(--fs-400); font-weight: var(--fw-semibold); }
  .about { margin: 0; color: var(--text-secondary); }
  .progress { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); font-size: var(--fs-075); }
  .bar { flex: 1; min-inline-size: var(--layout-field-sm); }
  .entries-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-150); }
  .label { margin: 0; color: var(--text-secondary); font-size: var(--fs-065); font-weight: var(--fw-semibold); text-transform: uppercase; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .quiet code { padding-inline: var(--sp-050); border-radius: var(--r-sm); background: var(--bg-recess); color: var(--text-primary); font-family: var(--font-mono); overflow-wrap: anywhere; }
  .key { color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--fs-090); }
  .picks { display: flex; flex-direction: column; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .picks li { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-150); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .layout { grid-template-columns: minmax(0, 1fr); }
  }
</style>
