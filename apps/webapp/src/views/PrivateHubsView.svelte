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

  import { untrack } from 'svelte';

  import { Banner, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';

  import PrivateHubsAllowed from '../lib/workspace/PrivateHubsAllowed.svelte';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { privateHubs } from '../lib/data/privatehubs.svelte.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  $effect(() => untrack(() => privateHubs.open()));

  const reading = $derived(privateHubs.state);
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  $effect(() => {
    accounts.resolve(privateHubs.hubs.flatMap((hub) => hub.owners));
  });

  const columns = $derived([
    { id: 'owners', label: t('app.private_hubs.owners') },
    { id: 'collections', label: t('app.private_hubs.collections') },
    { id: 'entries', label: t('app.private_hubs.entries') },
    { id: 'attachments', label: t('app.private_hubs.attachments') },
    { id: 'created', label: t('app.private_hubs.created') },
    { id: 'state', label: t('app.private_hubs.state') },
  ]);

  /** A day as the reader's language writes it; a `date` and a timestamp alike are read in UTC. */
  const dayOf = (value: string) =>
    new Intl.DateTimeFormat(messages.locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(value));

  const ownersOf = (owners: readonly string[]) =>
    owners.length === 0
      ? t('app.private_hubs.nobody')
      : owners.map((each) => accounts.nameOf(each) ?? t('app.people.unnamed')).join(', ');

  $effect(() => page.entitle(t('app.private_hubs.title')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.private_hubs.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'private-hubs', label: t('app.private_hubs.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <PrivateHubsAllowed />

  <Stack gap="300">
    <p class="quiet">{t('app.private_hubs.intro')}</p>

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting">
        <Spinner label={t('app.private_hubs.reading')} />
        <span>{t('app.private_hubs.reading')}</span>
      </p>
    {:else if refusal}
      <Banner tone="danger" title={refusal.message}>
        {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
      </Banner>
    {:else if privateHubs.hubs.length === 0}
      <p class="quiet">{t('app.private_hubs.empty')}</p>
    {:else}
      <Table label={t('app.private_hubs.title')} isLabelHidden {columns}>
        {#each privateHubs.hubs as hub (hub.id)}
          <tr>
            <th scope="row" class="owners">{ownersOf(hub.owners)}</th>
            <td>{hub.collections}</td>
            <td>{hub.entries}</td>
            <td>{formatBytes(hub.attachment_bytes, messages.locale)}</td>
            <td>{dayOf(hub.created_at)}</td>
            <td>
              {#if hub.trashed_at}
                <span class="trashed">
                  {t('app.private_hubs.trashed', { day: hub.purge_on ? dayOf(hub.purge_on) : '—' })}
                </span>
              {:else}
                {t('app.private_hubs.in_use')}
              {/if}
            </td>
          </tr>
        {/each}
      </Table>
    {/if}
  </Stack>
</Stack>

<style>
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
