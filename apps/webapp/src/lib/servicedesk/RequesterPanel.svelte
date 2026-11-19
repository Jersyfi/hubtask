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
  import { containers } from '../data/containers.svelte.ts';
  import { ticketKeyOf } from '../data/ticket.ts';
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
  /** A request as the line names it: its ticket key where its collection has one, else its title. */
  const keyOf = (other: WorkItem) => ticketKeyOf(containers.find(other.collection_id), other.number);

  $effect(() => {
    if (requester && !requester.by_mail) accounts.resolve([requester.account_id]);
  });
</script>

{#if requester}
  <div class="requester" data-requester-panel>
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
      <!-- One line, as the ticket's scene has it: the count, then each request by its key, each a link
           whose name also carries its title. -->
      <p class="label" data-requester-open>
        {open.length === 1 ? t('app.desk.requester.open_one') : t('app.desk.requester.open_many', { count: String(open.length) })}
        {#each open as other, index (other.id)}
          {@const key = keyOf(other)}
          {#if index > 0}<span aria-hidden="true">, </span>{/if}
          <a class="other" href={`/items/${other.id}`} aria-label={key ? t('app.desk.requester.other_named', { key, title: other.title }) : undefined}
            onclick={(event) => { if (event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey) { event.preventDefault(); onnavigate(`/items/${other.id}`); } }}>{key ?? other.title}</a>
        {/each}
      </p>
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
  </div>
{/if}

<style>
  .requester { display: flex; flex-direction: column; gap: var(--sp-050); font-size: var(--fs-075); }
  .who, .label { margin: 0; }
  .label, .organisation { color: var(--text-secondary); }
  .others { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-025); }
  .name { overflow-wrap: anywhere; }
  .other { color: var(--text-brand); font-weight: var(--fw-semibold); text-decoration: none; overflow-wrap: anywhere; }
  .other:hover { text-decoration: underline; }
  .other:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
