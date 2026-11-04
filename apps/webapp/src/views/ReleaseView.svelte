<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Releases* of a hub (M2, UC-PLN-08, user-experience.md §5.1): the head says what is coming and
  // what went out, the chosen release is a card with how much of it is done, then its entries and
  // the entries of one of the hub's collections to add. Releasing, changing and deleting are the
  // head's acts; a release has no trash, so deleting it asks (§5.9).
  //
  // **The server ships; this screen picks and writes.** What is done, where open work goes and
  // the notes page are the answers of `/releases/{id}` and `:publish`; the release is read again
  // after every write.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS at the hub the screen
  // shows the releases and their entries, not controls.

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Checkbox, Dialog, Inline, Input, PageHeader, ProgressBar, Select, Skeleton, Stack, Table, Textarea,
    type Column,
  } from '@hubtask/design-system/components';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { Release, ReleaseDetail, ReleaseList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { releaseCandidates, releaseGroups, releasePath, releases, releaseShown, releasesPath, releaseTargets } from '../lib/data/releases.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDay, formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';
  import PlaceStrip from '../lib/workspace/PlaceStrip.svelte';

  interface Props {
    /** The hub the releases belong to. */
    hubId: string;
    onnavigate: (path: string) => void;
  }

  const { hubId, onnavigate }: Props = $props();

  const place = $derived(containers.find(hubId));
  const placePath = $derived(`/hubs/${hubId}`);
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

  let list = $state<Release[] | undefined>(undefined);
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

  /** The collection whose top-level entries are offered to join the release. */
  let sourceId = $state('');
  const source = $derived(sourceId || collections[0]?.id || '');
  let offered = $state<{ id: string; title: string }[]>([]);
  $effect(() => {
    const id = source;
    offered = [];
    if (!id) return;
    const path = `/items?collection_id=${id}&page_size=100`;
    return untrack(() => engine.subscribe<{ data: { id: string; title: string; is_completed?: boolean }[] }>({ path }, (next) => {
      if (next.status === 'ready') offered = next.data.data.map((e) => ({ id: e.id, title: e.title }));
    }));
  });
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
  function toggle(id: string, isOn: boolean) {
    selected = isOn ? [...selected, id] : selected.filter((each) => each !== id);
  }
  const inRelease = $derived(new Set((detail?.entries ?? []).map((e) => e.item_id)));
  const pickedIn = $derived(selected.filter((id) => inRelease.has(id)));
  const pickedOut = $derived(selected.filter((id) => !inRelease.has(id)));

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
  const figure = $derived.by(() => {
    if (!groups || (groups.planned.length === 0 && groups.released.length === 0)) return undefined;
    const next = groups.planned[0];
    return [
      groups.planned.length > 0 ? t('app.releases.figure_planned', { count: String(groups.planned.length) }) : undefined,
      next ? t(groups.late.has(next.id) ? 'app.releases.figure_late' : 'app.releases.figure_next', { name: next.name, day: formatPlainDay(next.release_on, messages.locale) }) : undefined,
      groups.released.length > 0 ? t('app.releases.figure_released', { count: String(groups.released.length) }) : undefined,
    ].filter((part): part is string => !!part).join(' · ');
  });
  const isPlanned = $derived(chosen?.state === 'PLANNED');
  const mayChange = $derived(mayWrite && isPlanned);
  const shareDone = $derived(detail && detail.done_count + detail.open_count > 0 ? Math.round((100 * detail.done_count) / (detail.done_count + detail.open_count)) : undefined);

  // Coming first, the soonest on top, then what went out, the latest on top.
  const options = $derived(groups ? [...groups.planned, ...groups.released].map((r) => ({
    value: r.id,
    label: t('app.releases.option', { name: r.name, state: t(groups.late.has(r.id) ? 'app.releases.state_late' : `app.releases.state_${r.state}`) }),
  })) : []);
  const entryColumns = $derived<Column[]>([
    { id: 'pick', label: t('app.releases.col_pick'), isLabelHidden: true },
    { id: 'entry', label: t('app.releases.col_entry') },
    { id: 'type', label: t('app.releases.col_type') },
    { id: 'state', label: t('app.releases.col_state') },
  ]);
  const candidateColumns = $derived<Column[]>([
    { id: 'pick', label: t('app.releases.col_pick'), isLabelHidden: true },
    { id: 'entry', label: t('app.releases.col_entry') },
  ]);
</script>

<Stack gap="200">
  <PageHeader
    title={t('app.releases.title')}
    isTitleInBar={viewport.isCompact}
    subtitle={figure}
    breadcrumb={place
      ? {
          trail: [{ id: 'place', label: place.name, href: placePath }, { id: 'releases', label: t('app.releases.title') }],
          label: t('app.releases.trail'),
          expandLabel: t('app.releases.expand_trail'),
          onnavigate: () => onnavigate(placePath),
        }
      : undefined}
    primary={mayWrite ? { label: t('app.releases.plan'), icon: 'plus', onclick: () => openPlan(undefined) } : undefined}
    secondary={mayChange && chosen
      ? [
          { label: t('app.releases.publish'), icon: 'send', onclick: () => { carryTo = targets[0]?.id ?? ''; writeNotes = true; dialogFailure = undefined; isReleasing = true; } },
          { label: t('app.releases.edit'), icon: 'pencil', onclick: () => openPlan(chosen) },
        ]
      : []}
    menu={mayChange
      ? { label: t('app.releases.more'), items: [{ id: 'delete', label: t('app.releases.delete'), icon: 'trash', isDestructive: true }], onselect: () => { dialogFailure = undefined; isDeleting = true; } }
      : undefined}
  />
  <PlaceStrip placeId={hubId} current="plan" view="releases" {onnavigate} />

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !list}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else if list.length === 0}
    <ProductEmpty
      kind="unused"
      title={t('app.releases.none')}
      description={t('app.releases.none_hint')}
      create={mayWrite ? { label: t('app.releases.plan'), onclick: () => openPlan(undefined) } : undefined}
    />
  {:else if chosen}
    {#if failure}<Banner tone="danger">{failure}</Banner>{/if}

    {#if list.length > 1}
      <div class="chooser">
        <Select label={t('app.releases.release')} size="sm" value={chosen.id} {options}
          onchange={(event: Event) => { chosenId = (event.currentTarget as HTMLSelectElement).value; selected = []; }} />
      </div>
    {/if}

    <section class="card" aria-labelledby="release-head">
      <Inline gap="100" align="center" isWrapping>
        <h2 id="release-head" class="name">{chosen.name}</h2>
        {#if groups?.late.has(chosen.id)}
          <Badge tone="warning">{t('app.releases.badge_late')}</Badge>
        {:else if chosen.state === 'RELEASED'}
          <Badge tone="success">{t('app.releases.badge_RELEASED')}</Badge>
        {:else}
          <Badge tone="info">{t('app.releases.badge_PLANNED')}</Badge>
        {/if}
      </Inline>
      <p class="quiet">
        {chosen.state === 'RELEASED' && chosen.released_at
          ? t('app.releases.released_on', { day: formatDay(chosen.released_at, messages.locale) })
          : t('app.releases.due_on', { day: formatPlainDay(chosen.release_on, messages.locale) })}
      </p>
      {#if chosen.description}<p class="about">{chosen.description}</p>{/if}
      {#if detail && shareDone !== undefined}
        <ProgressBar label={t('app.releases.progress', { name: chosen.name })} value={shareDone}
          valueLabel={t('app.releases.counts', { done: String(detail.done_count), open: String(detail.open_count) })} size="sm" />
      {/if}
      {#if chosen.notes_page_id}
        <p><a href={`/pages/${chosen.notes_page_id}`}>{t('app.releases.notes')}</a></p>
      {/if}
    </section>

    <div class="columns">
      <section class="block" aria-labelledby="release-entries">
        <Inline justify="between" align="center">
          <h3 id="release-entries" class="heading">{t('app.releases.in_release', { count: String(detail?.entries.length ?? 0) })}</h3>
          {#if mayChange && pickedIn.length > 0}
            <Button size="sm" tone="secondary" onclick={() => { const ids = pickedIn; selected = []; void act(() => releases.take(hubId, chosen.id, ids)); }}>{t('app.releases.take', { count: String(pickedIn.length) })}</Button>
          {/if}
        </Inline>
        {#if !detail}
          <Skeleton lines={3} />
        {:else if detail.entries.length === 0}
          <ProductEmpty kind="settled" title={t(mayChange ? 'app.releases.empty_add' : 'app.releases.empty')} />
        {:else}
          <Table label={t('app.releases.in_release', { count: String(detail.entries.length) })} isLabelHidden
            columns={mayChange ? entryColumns : entryColumns.slice(1)}>
            {#each detail.entries as entry (entry.item_id)}
              <tr>
                {#if mayChange}
                  <td>
                    <Checkbox label={t('app.releases.pick', { title: entry.title })} isLabelHidden checked={selected.includes(entry.item_id)}
                      onchange={(event: Event) => toggle(entry.item_id, (event.currentTarget as HTMLInputElement).checked)} />
                  </td>
                {/if}
                <th scope="row"><a href={`/items/${entry.item_id}`}>{entry.title}</a></th>
                <td>{entry.type_name}</td>
                <td>
                  {#if entry.done}<Badge tone="success">{t('app.releases.done')}</Badge>{:else}<Badge tone="neutral">{t('app.releases.open')}</Badge>{/if}
                </td>
              </tr>
            {/each}
          </Table>
        {/if}
      </section>

      {#if mayChange}
        <section class="block" aria-labelledby="release-candidates">
          <Inline justify="between" align="end">
            <h3 id="release-candidates" class="heading">{t('app.releases.candidates')}</h3>
            {#if pickedOut.length > 0}
              <Button size="sm" onclick={() => { const ids = pickedOut; selected = []; void act(() => releases.add(hubId, chosen.id, ids)); }}>{t('app.releases.add', { count: String(pickedOut.length) })}</Button>
            {/if}
          </Inline>
          {#if collections.length > 1}
            <Select label={t('app.releases.from')} size="sm" value={source}
              options={collections.map((c) => ({ value: c.id, label: c.name }))}
              onchange={(event: Event) => { sourceId = (event.currentTarget as HTMLSelectElement).value; }} />
          {/if}
          {#if collections.length === 0}
            <ProductEmpty kind="settled" title={t('app.releases.no_collections')} />
          {:else if candidates.length === 0}
            <ProductEmpty kind="settled" title={t('app.releases.all_in')} />
          {:else}
            <Table label={t('app.releases.candidates')} isLabelHidden columns={candidateColumns}>
              {#each candidates as entry (entry.id)}
                <tr>
                  <td>
                    <Checkbox label={t('app.releases.pick', { title: entry.title })} isLabelHidden checked={selected.includes(entry.id)}
                      onchange={(event: Event) => toggle(entry.id, (event.currentTarget as HTMLInputElement).checked)} />
                  </td>
                  <th scope="row"><a href={`/items/${entry.id}`}>{entry.title}</a></th>
                </tr>
              {/each}
            </Table>
          {/if}
        </section>
      {/if}
    </div>
  {/if}
</Stack>

{#if isPlanning}
  <Dialog bind:isOpen={isPlanning} title={editing ? t('app.releases.edit') : t('app.releases.plan')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.releases.name')} bind:value={draft.name} maxlength={120} isRequired error={nameError} />
      <Textarea label={t('app.releases.description')} bind:value={draft.description} maxlength={2000} />
      <Input label={t('app.releases.release_on')} type="date" bind:value={draft.release_on} isRequired error={dayError} />
      {#if dialogFailure}<Banner tone="danger">{dialogFailure}</Banner>{/if}
      <Inline justify="end"><Button onclick={plan}>{editing ? t('app.releases.save') : t('app.releases.plan')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isReleasing && chosen}
  <Dialog bind:isOpen={isReleasing} title={t('app.releases.publish_title', { name: chosen.name })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.releases.publish_hint')}</p>
      {#if detail}<p>{t('app.releases.counts', { done: String(detail.done_count), open: String(detail.open_count) })}</p>{/if}
      <Select label={t('app.releases.carry_to')} bind:value={carryTo}
        options={[{ value: '', label: t('app.releases.carry_out') }, ...targets.map((r) => ({ value: r.id, label: r.name }))]} />
      <Checkbox label={t('app.releases.write_notes')} bind:checked={writeNotes} />
      {#if dialogFailure}<Banner tone="danger">{dialogFailure}</Banner>{/if}
      <Inline justify="end"><Button onclick={publish}>{t('app.releases.publish')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isDeleting && chosen}
  <Dialog bind:isOpen={isDeleting} title={t('app.releases.delete_title', { name: chosen.name })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p>{t('app.releases.delete_cost', { count: String(detail?.entries.length ?? 0) })}</p>
      {#if dialogFailure}<Banner tone="danger">{dialogFailure}</Banner>{/if}
      <Inline justify="end" gap="100">
        <Button tone="subtle" onclick={() => (isDeleting = false)}>{t('app.releases.keep')}</Button>
        <Button tone="danger" icon="trash" onclick={remove}>{t('app.releases.delete')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .chooser { max-inline-size: var(--layout-board-column); }
  .card { display: flex; flex-direction: column; gap: var(--sp-100); padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); }
  .name { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .about { margin: 0; }
  .columns { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, var(--layout-board-column)), 1fr)); gap: var(--sp-200); }
  .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
