<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry chosen from anywhere the reader can see: one search across every collection
  // (UC-PLN-01 check 1), for drawing a link and for mentioning an entry in a text. What it offers
  // is what search answers, so an entry out of the reader's sight is never offered (check 4).
  //
  // **The search is the full-text one, by `POST`** (`search.peek`): the words are the reader's
  // content and never travel in a URL. A later answer wins over an earlier one by generation, for
  // the reason the search screen gives - typing narrows several times a second.

  import { Input, Spinner } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { search } from '../data/search.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Entries not to offer: the entry itself, and what it is already linked to. */
    excluded?: ReadonlySet<string>;
    /** The entry chosen, or none while nothing is. */
    chosen?: WorkItem;
    onchoose: (item: WorkItem | undefined) => void;
  }

  const { excluded = new Set(), chosen, onchoose }: Props = $props();

  const LIMIT = 8;

  let term = $state('');
  let hits = $state<readonly WorkItem[]>([]);
  let isSearching = $state(false);
  let generation = 0;

  async function ask(value: string) {
    term = value;
    onchoose(undefined);
    generation += 1;
    const mine = generation;
    if (value.trim() === '') {
      hits = [];
      isSearching = false;
      return;
    }
    isSearching = true;
    try {
      const found = await search.peek({ q: value }, LIMIT + excluded.size);
      if (mine !== generation) return;
      hits = found.filter((hit) => !excluded.has(hit.id)).slice(0, LIMIT);
    } catch {
      if (mine === generation) hits = [];
    } finally {
      if (mine === generation) isSearching = false;
    }
  }
</script>

<div class="picker">
  <Input
    label={t('app.links.search')}
    hint={t('app.links.search_hint')}
    value={term}
    oninput={(event) => void ask((event.currentTarget as HTMLInputElement).value)}
  />
  {#if isSearching}
    <p class="quiet"><Spinner label={t('app.links.searching')} /> {t('app.links.searching')}</p>
  {:else if term.trim() !== '' && hits.length === 0}
    <p class="quiet" role="status">{t('app.links.no_hits')}</p>
  {/if}
  {#if hits.length > 0}
    <ul class="hits" aria-label={t('app.links.search')}>
      {#each hits as hit (hit.id)}
        <li>
          <button
            type="button"
            class="hit"
            data-hit={hit.id}
            aria-pressed={chosen?.id === hit.id}
            aria-label={t('app.links.choose', { name: hit.title })}
            onclick={() => onchoose(hit)}
          >
            <span class="title">{hit.title}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker { display: flex; flex-direction: column; gap: var(--sp-100); }

  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .hits { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }

  .hit {
    display: flex;
    align-items: baseline;
    gap: var(--sp-100);
    inline-size: 100%;
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-100);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .hit:hover { background: var(--bg-surface-hover); }
  .hit[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--bg-surface-pressed); }
  .hit:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .title { min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
