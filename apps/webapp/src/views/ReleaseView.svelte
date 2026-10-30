<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Releases* of a hub (M2, UC-PLN-08): the hub's releases, the selected one's entries done and
  // open, the entries of one of the hub's collections to add, and the release with its notes.
  //
  // **The server ships; this screen picks and writes.** What is done, where open work goes and
  // the notes page are the answers of `/releases/{id}` and `:publish`; the release is read again
  // after every write.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS at the hub the screen
  // shows the releases and their entries, not controls.

  import { untrack } from 'svelte';
  import {
    Banner, Button, Checkbox, Dialog, Inline, Input, PageHeader, Select, Skeleton, Stack, Table, Textarea,
    type Column,
  } from '@hubtask/design-system/components';
  import type { Release, ReleaseDetail, ReleaseList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { releaseCandidates, releasePath, releases, releaseShown, releasesPath, releaseTargets } from '../lib/data/releases.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';

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
  function openPlan(release: Release | undefined) {
    editing = release;
    draft.name = release?.name ?? '';
    draft.description = release?.description ?? '';
    draft.release_on = release?.release_on ?? today;
    isPlanning = true;
  }
  function plan() {
    const fields = { ...draft, name: draft.name.trim() };
    if (!fields.name || !fields.release_on) return;
    isPlanning = false;
    const changing = editing;
    if (changing) {
      void act(() => releases.update(hubId, changing, fields), t('app.releases.changed', { name: fields.name }));
      return;
    }
    void act(async () => {
      const made = await releases.create(hubId, fields);
      chosenId = made.id;
    }, t('app.releases.planned', { name: fields.name }));
  }

  let isReleasing = $state(false);
  let carryTo = $state('');
  let writeNotes = $state(true);
  const targets = $derived(list && chosen ? releaseTargets(list, chosen.id) : []);
  function publish() {
    const release = chosen;
    if (!release) return;
    isReleasing = false;
    void act(() => releases.publish(hubId, release.id, writeNotes, carryTo || undefined), t('app.releases.released', { name: release.name }));
  }

  const options = $derived(list ? list.map((r) => ({ value: r.id, label: t('app.releases.option', { name: r.name, state: t(`app.releases.state_${r.state}`) }) })) : []);
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

<Stack gap="300">
  <PageHeader
    title={t('app.releases.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={place
      ? {
          trail: [{ id: 'place', label: place.name, href: placePath }, { id: 'releases', label: t('app.releases.title') }],
          label: t('app.releases.trail'),
          expandLabel: t('app.releases.expand_trail'),
          onnavigate: () => onnavigate(placePath),
        }
      : undefined}
    secondary={mayWrite ? [{ label: t('app.releases.plan'), icon: 'plus', onclick: () => openPlan(undefined) }] : []}
  />

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !list}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

    {#if list.length === 0}
      <p class="quiet">{t('app.releases.none')}</p>
    {:else if chosen}
      <div class="controls">
        <Select label={t('app.releases.release')} size="sm" value={chosen.id} {options}
          onchange={(event: Event) => { chosenId = (event.currentTarget as HTMLSelectElement).value; selected = []; }} />
        {#if mayWrite && chosen.state === 'PLANNED'}
          <Button size="sm" tone="secondary" icon="pencil" onclick={() => openPlan(chosen)}>{t('app.releases.edit')}</Button>
          <Button size="sm" onclick={() => { carryTo = targets[0]?.id ?? ''; isReleasing = true; }}>{t('app.releases.publish')}</Button>
          <Button size="sm" tone="subtle" icon="trash" onclick={() => { const gone = chosen.id; chosenId = ''; void act(() => releases.remove(hubId, gone), t('app.releases.deleted')); }}>{t('app.releases.delete')}</Button>
        {/if}
      </div>

      <section class="block" aria-labelledby="release-head">
        <h2 id="release-head" class="heading">{chosen.name}</h2>
        {#if chosen.description}<p>{chosen.description}</p>{/if}
        <p class="quiet">
          {chosen.state === 'RELEASED' && chosen.released_at
            ? t('app.releases.released_on', { day: chosen.released_at.slice(0, 10) })
            : t('app.releases.due_on', { day: chosen.release_on })}
          {#if detail} · {t('app.releases.counts', { done: String(detail.done_count), open: String(detail.open_count) })}{/if}
        </p>
        {#if chosen.notes_page_id}
          <p><a href={`/pages/${chosen.notes_page_id}`}>{t('app.releases.notes')}</a></p>
        {/if}
      </section>

      <div class="columns">
        <section class="block" aria-labelledby="release-entries">
          <Inline justify="between" align="center">
            <h3 id="release-entries" class="heading">{t('app.releases.in_release', { count: String(detail?.entries.length ?? 0) })}</h3>
            {#if mayWrite && chosen.state === 'PLANNED' && pickedIn.length > 0}
              <Button size="sm" tone="secondary" onclick={() => { const ids = pickedIn; selected = []; void act(() => releases.take(hubId, chosen.id, ids)); }}>{t('app.releases.take', { count: String(pickedIn.length) })}</Button>
            {/if}
          </Inline>
          {#if !detail}
            <Skeleton lines={3} />
          {:else}
            <Table label={t('app.releases.in_release', { count: String(detail.entries.length) })} isLabelHidden columns={entryColumns}>
              {#each detail.entries as entry (entry.item_id)}
                <tr>
                  <td>
                    {#if mayWrite && chosen.state === 'PLANNED'}
                      <Checkbox label={t('app.releases.pick', { title: entry.title })} isLabelHidden checked={selected.includes(entry.item_id)}
                        onchange={(event: Event) => toggle(entry.item_id, (event.currentTarget as HTMLInputElement).checked)} />
                    {/if}
                  </td>
                  <th scope="row"><a href={`/items/${entry.item_id}`}>{entry.title}</a></th>
                  <td>{entry.type_name}</td>
                  <td>{entry.done ? t('app.releases.done') : t('app.releases.open')}</td>
                </tr>
              {/each}
            </Table>
          {/if}
        </section>

        {#if mayWrite && chosen.state === 'PLANNED'}
          <section class="block" aria-labelledby="release-candidates">
            <Inline justify="between" align="end">
              {#if collections.length > 1}
                <Select label={t('app.releases.from')} size="sm" value={source}
                  options={collections.map((c) => ({ value: c.id, label: c.name }))}
                  onchange={(event: Event) => { sourceId = (event.currentTarget as HTMLSelectElement).value; }} />
              {:else}
                <h3 id="release-candidates" class="heading">{t('app.releases.candidates')}</h3>
              {/if}
              {#if pickedOut.length > 0}
                <Button size="sm" onclick={() => { const ids = pickedOut; selected = []; void act(() => releases.add(hubId, chosen.id, ids)); }}>{t('app.releases.add', { count: String(pickedOut.length) })}</Button>
              {/if}
            </Inline>
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
          </section>
        {/if}
      </div>
    {/if}
  {/if}
</Stack>

{#if isPlanning}
  <Dialog bind:isOpen={isPlanning} title={editing ? t('app.releases.edit') : t('app.releases.plan')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.releases.name')} bind:value={draft.name} maxlength={120} />
      <Textarea label={t('app.releases.description')} bind:value={draft.description} maxlength={2000} />
      <Input label={t('app.releases.release_on')} type="date" bind:value={draft.release_on} />
      <Inline justify="end"><Button onclick={plan}>{editing ? t('app.releases.save') : t('app.releases.plan')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isReleasing && chosen}
  <Dialog bind:isOpen={isReleasing} title={t('app.releases.publish_title', { name: chosen.name })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.releases.publish_hint')}</p>
      <Select label={t('app.releases.carry_to')} bind:value={carryTo}
        options={[{ value: '', label: t('app.releases.carry_out') }, ...targets.map((r) => ({ value: r.id, label: r.name }))]} />
      <Checkbox label={t('app.releases.write_notes')} bind:checked={writeNotes} />
      <Inline justify="end"><Button onclick={publish}>{t('app.releases.publish')}</Button></Inline>
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
</style>
