<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who raised the request, their organisation, their other open requests (UC-SVC-03 check 7) and
  // the assets they own (check 9): the side panel a handler reads before answering. The other
  // requests are a search by creator and the assets the server's selection, so it lists only what
  // this reader may see; the address is never shown. A request the mail intake filed for a sender
  // without an account was created by the intake's owner: it names nobody and lists nothing.

  import { untrack } from 'svelte';
  import { ListRow } from '@hubtask/design-system/components';
  import type { WorkItem, WorkItemPage } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { otherRequests, requesterRequests } from './desk.ts';

  interface RequesterAsset { id: string; hub_id: string; class: string; name: string; stage: string }
  interface Requester { account_id: string; by_mail?: boolean; organisation: { key: string; name: string } | null; assets?: RequesterAsset[] }

  const { item, onnavigate }: { item: WorkItem; onnavigate: (path: string) => void } = $props();

  // Keyed on the entry by its caller, so the paths are fixed for its life.
  const read = resource<Requester>(untrack(() => `/items/${item.id}/requester`));
  const requester = $derived(read.state.status === 'ready' ? read.state.data : undefined);
  const creator = untrack(() => item.created_by);
  const others = creator ? resource<WorkItemPage>({ path: '/search', body: requesterRequests(creator) }) : undefined;
  const open = $derived(others?.state.status === 'ready' ? otherRequests(others.state.data.data ?? [], item.id) : []);

  $effect(() => {
    if (requester && !requester.by_mail) accounts.resolve([requester.account_id]);
  });
</script>

{#if requester}
  <section class="requester" aria-label={t('app.desk.requester.title')} data-requester-panel>
    <h3 class="title">{t('app.desk.requester.title')}</h3>
    {#if requester.by_mail}
      <p class="who" data-requester-by-mail>{t('app.desk.requester.by_mail')}</p>
    {:else}
      <p class="who">
        {accounts.nameOf(requester.account_id) ?? t('app.people.unnamed')}
        {#if requester.organisation}<span class="organisation">· {requester.organisation.name}</span>{/if}
      </p>
    {/if}
    {#if requester.by_mail}
      <!-- The intake owner's requests are not the sender's. -->
    {:else if open.length > 0}
      <p class="label">{t('app.desk.requester.open')}</p>
      <ul class="others">
        {#each open as other (other.id)}
          <li><ListRow href={`/items/${other.id}`} onactivate={() => onnavigate(`/items/${other.id}`)} label={other.title}><span class="name">{other.title}</span></ListRow></li>
        {/each}
      </ul>
    {:else}
      <p class="label">{t('app.desk.requester.none_open')}</p>
    {/if}
    {#if (requester.assets ?? []).length > 0}
      <p class="label">{t('app.desk.requester.assets')}</p>
      <ul class="others" data-requester-assets>
        {#each requester.assets ?? [] as asset (asset.id)}
          <li><ListRow href={`/cis/${asset.id}`} onactivate={() => onnavigate(`/cis/${asset.id}`)} label={asset.name}><span class="name">{asset.name}</span></ListRow></li>
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
  .others { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-025); }
  .name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
