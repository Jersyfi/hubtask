<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who raised the request, their organisation, their other open requests (UC-SVC-03 check 7) and
  // the assets they own (check 9): the side panel a handler reads before answering. The other
  // requests are a search by creator and the assets the server's selection, so it lists only what
  // this reader may see; the address is never shown.

  import { untrack } from 'svelte';
  import type { WorkItem, WorkItemPage } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { otherRequests, requesterRequests } from './desk.ts';

  interface RequesterAsset { id: string; hub_id: string; class: string; name: string; stage: string }
  interface Requester { account_id: string; organisation: { key: string; name: string } | null; assets?: RequesterAsset[] }

  const { item, onnavigate }: { item: WorkItem; onnavigate: (path: string) => void } = $props();

  // Keyed on the entry by its caller, so the paths are fixed for its life.
  const read = resource<Requester>(untrack(() => `/items/${item.id}/requester`));
  const requester = $derived(read.state.status === 'ready' ? read.state.data : undefined);
  const creator = untrack(() => item.created_by);
  const others = creator ? resource<WorkItemPage>({ path: '/search', body: requesterRequests(creator) }) : undefined;
  const open = $derived(others?.state.status === 'ready' ? otherRequests(others.state.data.data ?? [], item.id) : []);

  $effect(() => {
    if (requester) accounts.resolve([requester.account_id]);
  });
</script>

{#if requester}
  <section class="requester" aria-label={t('app.desk.requester.title')} data-requester-panel>
    <h3 class="title">{t('app.desk.requester.title')}</h3>
    <p class="who">
      {accounts.nameOf(requester.account_id) ?? t('app.people.unnamed')}
      {#if requester.organisation}<span class="organisation">· {requester.organisation.name}</span>{/if}
    </p>
    {#if open.length > 0}
      <p class="label">{t('app.desk.requester.open')}</p>
      <ul class="others">
        {#each open as other (other.id)}
          <li><a href={`/items/${other.id}`} onclick={(event) => { event.preventDefault(); onnavigate(`/items/${other.id}`); }}>{other.title}</a></li>
        {/each}
      </ul>
    {:else}
      <p class="label">{t('app.desk.requester.none_open')}</p>
    {/if}
    {#if (requester.assets ?? []).length > 0}
      <p class="label">{t('app.desk.requester.assets')}</p>
      <ul class="others" data-requester-assets>
        {#each requester.assets ?? [] as asset (asset.id)}
          <li><a href={`/cis/${asset.id}`} onclick={(event) => { event.preventDefault(); onnavigate(`/cis/${asset.id}`); }}>{asset.name}</a></li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .requester { display: flex; flex-direction: column; gap: var(--sp-050); font-size: var(--fs-075); }
  .title { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); }
  .who, .label { margin: 0; }
  .label, .organisation { color: var(--text-secondary); }
  .others { margin: 0; padding-inline-start: var(--sp-200); display: grid; gap: var(--sp-025); }
</style>
