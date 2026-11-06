<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's recent activity on its overview (user-experience.md §7.9): the entries of its
  // collections changed last, as the generic query answers them for this reader - so nothing the
  // reader may not read is named. Drawn once answered and only where something changed.

  import { untrack } from 'svelte';

  import { engine } from '../data/engine.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  import type { ItemQueryResult, WorkItem } from '@hubtask/sync-engine';

  /** How many entries the overview names; the collection's own list holds the rest. */
  const RECENT_SHOWN = 5;

  interface Props {
    hubId: string;
  }

  const { hubId }: Props = $props();

  let recent = $state<readonly WorkItem[] | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    recent = undefined;
    return untrack(() => engine.subscribe<ItemQueryResult>({
      path: '/items:query',
      body: { scope: { container_id: at, include_descendants: true }, sort: [{ field: 'updated_at', dir: 'DESC' }], page: { size: RECENT_SHOWN } },
    }, (next) => {
      if (next.status === 'ready') recent = next.data.data.slice(0, RECENT_SHOWN);
    }));
  });
  const where = (entry: WorkItem) => containers.find(entry.collection_id)?.name ?? '';
</script>

{#if recent && recent.length > 0}
  <section class="recent" aria-labelledby="hub-recent">
    <h2 class="heading" id="hub-recent">{t('app.hub_overview.recent')}</h2>
    <ul>
      {#each recent as entry (entry.id)}
        <li data-recent-entry={entry.id}>
          <a href={`/items/${entry.id}`}>
            <span class="title">{entry.title}</span>
            {#if entry.updated_at}
              <span class="where">{t('app.hub_overview.recent_where', { place: where(entry), when: formatDateTime(entry.updated_at, messages.locale) })}</span>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .recent { display: flex; flex-direction: column; gap: var(--sp-100); min-inline-size: 0; }

  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); color: var(--text-primary); }

  ul { display: flex; flex-direction: column; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }

  a {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--sp-050) var(--sp-100);
    padding-block: var(--sp-050);
    color: var(--text-primary);
    text-decoration: none;
    font-size: var(--fs-075);
  }

  a:hover .title { text-decoration: underline; }
  a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-sm); }

  .title { min-inline-size: 0; overflow-wrap: anywhere; font-weight: var(--fw-medium); }
  .where { color: var(--text-secondary); }
</style>
