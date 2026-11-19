<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The work on an object (IN-3, UC-AST-09, scene `ci`): every entry that names it, or names an
  // object beneath it whose failure it suffers, with its key and title, why it stands here - *names
  // this object* or *on an object beneath: db-01* - and its status; the completed ones folded under
  // *n done*. Under them what mentions the object: pages, comments, boards.
  //
  // *Report an incident* opens the capture in a desk collection of the hub and names the object
  // on what it makes; *Work on it* does the same in any collection of the hub. Both are the
  // capture, the one way an entry is made (§8), for a writer, online (P-05).
  import { untrack } from 'svelte';
  import { Button, Disclosure, FoldingSection, ProductTile, Spinner, StateLozenge } from '@hubtask/design-system/components';
  import type { CiBacklinkList, CiWork, CiWorkEntry, ConfigurationItem, ResourceState } from '@hubtask/sync-engine';

  import { capture } from '../capture/capture.svelte.ts';
  import { cmdb } from '../data/cmdb.svelte.ts';
  import { backlinkHref } from '../data/whiteboards.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { workGroups } from './objectpage.ts';

  interface Props {
    ci: ConfigurationItem;
    work: ResourceState<CiWork>;
    /** The hub's desk collections; empty where the desk is off there or none takes tickets. */
    desks: readonly string[];
    /** A writer, online and on a screen that writes: the two acts are drawn. */
    mayAct: boolean;
  }

  const { ci, work, desks, mayAct }: Props = $props();

  const entries = $derived(work.status === 'ready' ? work.data.data : []);
  const groups = $derived(workGroups(entries));
  const refusal = $derived(work.status === 'failed' ? renderProblem(work.error, messages).message : undefined);

  const CATEGORY: Readonly<Record<string, 'todo' | 'progress' | 'done'>> = { TODO: 'todo', IN_PROGRESS: 'progress', DONE: 'done' };
  const stateOf = (entry: CiWorkEntry) => ({
    label: entry.status?.name ?? (entry.completed ? t('app.object.work_state_done') : t('app.object.work_state_open')),
    category: entry.completed ? 'done' : (CATEGORY[entry.status?.category ?? ''] ?? 'todo'),
  });

  // What mentions it, read once with the page: a reference is the reader's, judged per source.
  let backlinks = $state<CiBacklinkList['data'] | undefined>(undefined);
  $effect(() => {
    const id = ci.id;
    untrack(() => {
      cmdb.backlinks(id).then((read) => (backlinks = read), () => (backlinks = []));
    });
  });

  /** Names the object on what the capture made; a refusal is said, the entry stays. */
  function nameOn(itemIds: readonly string[]) {
    void Promise.all(itemIds.map((id) => cmdb.affect(id, ci.id))).then(
      () => toast.show({ text: t('app.object.named', { name: ci.name }), tone: 'success' }),
      (error) => toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' }),
    );
  }
  const reportIncident = () => capture.open({ collectionId: desks[0], hubId: ci.hub_id, oncreated: nameOn });
  const workOnIt = () => capture.open({ hubId: ci.hub_id, oncreated: nameOn });
</script>

<FoldingSection id="ci-work" title={t('app.object.mark_work')} count={entries.length > 0 ? String(entries.length) : undefined} {actions}>
  {#snippet children()}
    {#if refusal}
      <FailureNote problem={refusal} />
    {:else if work.status !== 'ready'}
      <Spinner label={t('app.assets.loading')} />
    {:else if entries.length === 0}
      <p class="meta none">{t('app.object.work_none')}</p>
    {:else}
      {#if groups.open.length > 0}
        <ul class="rows" data-work="open">{#each groups.open as entry (entry.item_id)}{@render row(entry)}{/each}</ul>
      {/if}
      {#if groups.done.length > 0}
        <Disclosure label={t('app.object.work_done', { count: groups.done.length })} tone="quiet">
          <ul class="rows" data-work="done">{#each groups.done as entry (entry.item_id)}{@render row(entry)}{/each}</ul>
        </Disclosure>
      {/if}
    {/if}

    {#if backlinks && backlinks.length > 0}
      <h3 class="subheading">{t('app.object.mentioned_in')}</h3>
      <ul class="rows" data-mentioned>
        {#each backlinks as link (`${link.source_kind}-${link.source_id}`)}
          <li class="row">
            <a href={backlinkHref(link)}>{link.title}</a>
            <span class="meta">{t(`app.assets.backlink_kind.${link.source_kind}`)}</span>
          </li>
        {/each}
      </ul>
    {/if}
  {/snippet}
</FoldingSection>

{#snippet actions()}
  {#if mayAct}
    {#if desks.length > 0}<Button size="sm" tone="subtle" icon="plus" data-report-incident onclick={reportIncident}>{t('app.object.report_incident')}</Button>{/if}
    <Button size="sm" tone="subtle" icon="plus" data-work-on-it onclick={workOnIt}>{t('app.object.work_on_it')}</Button>
  {/if}
{/snippet}

{#snippet row(entry: CiWorkEntry)}
  {@const state = stateOf(entry)}
  <li class="row" data-work-entry={entry.key ?? entry.item_id} data-reason={entry.reason}>
    <span class="what">
      <ProductTile icon={desks.includes(entry.collection_id) ? 'inbox' : 'square-check'} product={desks.includes(entry.collection_id) ? 'desk' : undefined} size="sm" />
      {#if entry.key}<span class="key">{entry.key}</span>{/if}
      <a href={`/items/${entry.item_id}`}>{entry.title}</a>
    </span>
    <span class="end">
      <span class="reason">
        {#if entry.reason === 'BENEATH' && entry.via}
          {t('app.object.work_beneath')} <a href={`/cis/${entry.via.ci_id}`}>{entry.via.name}</a>
        {:else}
          {t('app.object.work_names')}
        {/if}
      </span>
      <StateLozenge label={state.label} vocabulary="category" state={state.category} />
    </span>
  </li>
{/snippet}

<style>
  .meta { color: var(--text-subtle); font-size: var(--fs-075); }
  .none { margin: 0; }
  .subheading { margin: var(--sp-200) 0 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-secondary); }
  .rows { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; }
  .row {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-050) var(--sp-150);
    min-block-size: var(--sp-500); padding-block: var(--sp-050); border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }
  .row:last-child { border-block-end: 0; }
  .row a { color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  .row a:hover { text-decoration: underline; }
  .what { display: inline-flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; }
  .what a { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .key { color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--fs-075); white-space: nowrap; }
  .end { display: inline-flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: var(--sp-100); margin-inline-start: auto; }
  .reason { color: var(--text-subtle); font-size: var(--fs-075); }
  .reason a { color: var(--text-brand); font-weight: var(--fw-regular); }
</style>
