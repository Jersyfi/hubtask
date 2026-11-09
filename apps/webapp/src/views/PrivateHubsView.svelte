<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's private hubs as an administrator may see them (UC-ID-16 check 3, ADR-0073 §3):
  // whose they are, how much they hold, and - for one whose last member left - the day it goes.
  //
  // **No name, and no way in.** The answer carries none, and the rows link nowhere: a private hub
  // reads as not found to whoever holds nothing in it, and a link that answered "not found" would
  // say the screen is broken rather than that the hub is somebody else's.
  //
  // **The owners are the row's heading.** A row needs something to be read by, and "whose" is the
  // one thing about the hub the list may say; a hub nobody owns any more says so in its place.
  //
  // **The emergency access is the one way in, and only for the owner** (ADR-0073 §4). The row draws
  // it exactly where the server says it would be accepted (`emergency_access.offered`, P-05); while
  // the owner's hour runs, the row says until when and links to the hub - the only row that does.
  //
  // **A list page** (page-system §7.1): the hubs are objects of one kind to find - by owner, by
  // state - and nothing here is created, so the head has no primary. *Private hubs allowed* is one
  // switch that acts at once, the in-place rung (§5.1), and stands in the page's intro.

  import { untrack } from 'svelte';

  import { Button, DataTable, EmptyState, FilterBar, ListPage, Radio, Spinner, Stack, type DataColumn, type Filter } from '@hubtask/design-system/components';

  import PrivateHubsAllowed from '../lib/workspace/PrivateHubsAllowed.svelte';
  import EmergencyAccessDialog from '../lib/workspace/EmergencyAccessDialog.svelte';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { adminTrail } from '../lib/administration/trail.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import type { PrivateHubSummary as PrivateHub } from '@hubtask/sync-engine';
  import { privateHubs } from '../lib/data/privatehubs.svelte.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate?: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => untrack(() => privateHubs.open()));

  /** The hub the emergency dialog is open for, or none. */
  let opening = $state<string | undefined>(undefined);
  let isOpening = $state(false);

  const hasEmergencyColumn = $derived(
    privateHubs.hubs.some((hub) => hub.emergency_access.offered || hub.emergency_access.open_until),
  );

  function hubPath(id: string): string {
    return `/hubs/${id}`;
  }

  const reading = $derived(privateHubs.state);
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  $effect(() => {
    accounts.resolve(privateHubs.hubs.flatMap((hub) => hub.owners));
  });

  const columns: readonly DataColumn[] = $derived([
    { id: 'owners', label: t('app.private_hubs.owners'), priority: 1 },
    { id: 'collections', label: t('app.private_hubs.collections'), priority: 3 },
    { id: 'entries', label: t('app.private_hubs.entries'), priority: 2 },
    { id: 'attachments', label: t('app.private_hubs.attachments'), priority: 3 },
    { id: 'created', label: t('app.private_hubs.created'), priority: 3 },
    { id: 'state', label: t('app.private_hubs.state'), isStatus: true },
    ...(hasEmergencyColumn ? [{ id: 'emergency', label: t('app.private_hubs.emergency'), priority: 2 as const }] : []),
  ]);

  let search = $state('');
  /** `''` any, `use` in use, `trash` in the trash. */
  let stateFilter = $state('');
  const listed = $derived(privateHubs.hubs);
  const shown = $derived(
    listed.filter((hub) =>
      matchesSearch(search, [ownersOf(hub.owners)]) &&
      (stateFilter === '' || (stateFilter === 'trash') === Boolean(hub.trashed_at)),
    ),
  );
  const stateWords: Record<string, string> = $derived({ '': t('app.page.any'), use: t('app.private_hubs.in_use'), trash: t('app.private_hubs.in_trash') });
  const chips = $derived<Filter[]>([
    { id: 'state', label: t('app.private_hubs.state_chip', { state: stateWords[stateFilter] ?? '' }), isActive: stateFilter !== '' },
  ]);

  function open(hub: PrivateHub): void {
    opening = hub.id;
    isOpening = true;
  }

  const title = $derived(t('app.private_hubs.title'));
  const head = $derived({ title, breadcrumb: adminTrail({ id: 'private-hubs', label: title }) });

  /** A day as the reader's language writes it; a `date` and a timestamp alike are read in UTC. */
  const dayOf = (value: string) =>
    new Intl.DateTimeFormat(messages.locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(value));

  const ownersOf = (owners: readonly string[]) =>
    owners.length === 0
      ? t('app.private_hubs.nobody')
      : owners.map((each) => accounts.nameOf(each) ?? t('app.people.unnamed')).join(', ');

  $effect(() => page.entitle(title));
</script>

<ListPage {head}>
  {#snippet intro()}
    <Stack gap="200">
      <p class="quiet">{t('app.private_hubs.intro')}</p>
      <PrivateHubsAllowed />
    </Stack>
  {/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.private_hubs.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={chips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(stateFilter === '' ? 0 : 1) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(shown.length) })}
      clearLabel={t('app.page.clear')}
      onclear={() => { search = ''; stateFilter = ''; }}
      countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(listed.length) }) : undefined}
    >
      {#snippet picker()}
        <Radio label={t('app.private_hubs.state')} bind:value={stateFilter} options={['', 'use', 'trash'].map((value) => ({ value, label: stateWords[value] ?? '' }))} />
      {/snippet}
    </FilterBar>
  {/snippet}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting">
      <Spinner label={t('app.private_hubs.reading')} />
      <span>{t('app.private_hubs.reading')}</span>
    </p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={title}
      {columns}
      rows={shown}
      key={(hub: PrivateHub) => hub.id}
      rowLabel={(hub: PrivateHub) => ownersOf(hub.owners)}
      menu={(hub: PrivateHub) => (hub.emergency_access.offered && !hub.emergency_access.open_until
        ? { label: t('app.page.more_for', { name: ownersOf(hub.owners) }), items: [{ id: 'emergency', label: t('app.private_hubs.emergency_open') }] }
        : undefined)}
      onmenu={(hub: PrivateHub) => open(hub)}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
    >
      {#snippet cell(hub: PrivateHub, column: DataColumn)}
        {#if column.id === 'owners'}
          <span class="owners">{ownersOf(hub.owners)}</span>
        {:else if column.id === 'collections'}
          {hub.collections}
        {:else if column.id === 'entries'}
          {hub.entries}
        {:else if column.id === 'attachments'}
          {formatBytes(hub.attachment_bytes, messages.locale)}
        {:else if column.id === 'created'}
          {dayOf(hub.created_at)}
        {:else if column.id === 'state'}
          {#if hub.trashed_at}
            <span class="trashed">{t('app.private_hubs.trashed', { day: hub.purge_on ? dayOf(hub.purge_on) : '—' })}</span>
          {:else}
            {t('app.private_hubs.in_use')}
          {/if}
        {:else if hub.emergency_access.open_until}
          <span class="open">
            {t('app.private_hubs.emergency_running', { until: formatDateTime(hub.emergency_access.open_until, messages.locale) })}
          </span>
          <a
            href={hubPath(hub.id)}
            onclick={(event) => {
              event.preventDefault();
              onnavigate?.(hubPath(hub.id));
            }}
          >
            {t('app.private_hubs.go_to_hub')}
          </a>
        {/if}
      {/snippet}
      {#snippet verb(hub: PrivateHub)}
        {#if hub.emergency_access.offered && !hub.emergency_access.open_until}
          <Button tone="subtle" size="sm" data-emergency={hub.id} onclick={() => open(hub)}>{t('app.private_hubs.emergency_open')}</Button>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if listed.length === 0}
          <EmptyState kind="unused" title={t('app.private_hubs.empty')} />
        {:else}
          <EmptyState kind="filtered" title={t('app.private_hubs.none_found')} />
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#if opening}
  <EmergencyAccessDialog
    bind:isOpen={isOpening}
    hubId={opening}
    onopened={(id) => onnavigate?.(hubPath(id))}
  />
{/if}

<style>
  .open { display: block; color: var(--text-warning); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .owners { color: var(--text-primary); font-weight: var(--fw-medium); }

  .trashed { color: var(--text-warning); }
</style>
