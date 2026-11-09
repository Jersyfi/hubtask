<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The devices that hold a copy of this workspace for the reader.
  //
  // **A table, and the newest first** (ADR-0065 decision 3), for the reason the sessions beside it
  // are one: four facts a row, read across, is unreadable at the number of devices a year
  // produces. **A forgotten device is blocked, not erased**, so the row says so rather than
  // going — what it has not sent yet, it keeps.

  import { untrack } from 'svelte';

  import { Button, DataTable, Dialog, EmptyState, FilterBar, ListPage, Skeleton, type DataColumn, type DataRowMenu } from '@hubtask/design-system/components';
  import FailureState from '../lib/frame/FailureState.svelte';

  import { page } from '../lib/frame/page.svelte.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';

  import { actor } from '../lib/data/account.svelte.ts';
  import { devices } from '../lib/data/devices.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { sorting } from '../lib/sorting.svelte.ts';

  const account = $derived(actor.account);

  $effect(() => untrack(() => devices.open()));

  const held = $derived(devices.state);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The device a forget is being confirmed for. */
  let forgetting = $state<string | undefined>(undefined);

  // `$derived`, because the headings are words: a language chosen on the screen beside this one
  // changes them, and a list built once would keep the language it was built in.
  const columns = $derived<DataColumn[]>([
    { id: 'device', label: t('app.devices.name'), isSortable: true, priority: 1 },
    { id: 'platform', label: t('app.devices.platform'), isSortable: true, priority: 2 },
    { id: 'seen', label: t('app.devices.last_seen'), isSortable: true, priority: 1 },
    { id: 'standing', label: t('app.devices.standing'), isSortable: true, priority: 2 },
  ]);

  let search = $state('');

  // This device first, then the most recently seen. It is the list's own order and not a column,
  // which is why the third press of a heading matters: it is the only way back to it.
  const ordered = $derived(
    [...devices.all].sort((a, b) => {
      const mine = devices.isThisDevice(a.id) !== devices.isThisDevice(b.id);
      if (mine) return devices.isThisDevice(a.id) ? -1 : 1;
      return String(b.last_seen_at ?? '').localeCompare(String(a.last_seen_at ?? ''));
    }),
  );

  // A person's devices are counted on one hand; what this list wanted was the last-seen column
  // as something to sort by rather than as something to read down.
  const found = $derived(ordered.filter((device) => matchesSearch(search, [device.display_name ?? undefined, device.platform ?? undefined])));
  const order = sorting({
    rows: () => found,
    read: (device, columnId) => {
      if (columnId === 'platform') return device.platform;
      if (columnId === 'seen') return device.last_seen_at;
      if (columnId === 'standing') {
        return device.blocked ? t('app.devices.forgotten') : t('app.devices.synchronising');
      }
      return device.display_name;
    },
  });

  const continued = new Continued(() => order.rows);
  $effect(() => {
    void search;
    untrack(() => continued.restart());
  });

  const title = $derived(profileTitle('devices'));
  $effect(() => page.entitle(title));

  const nameOf = (device: (typeof devices.all)[number]) => device.display_name || t('app.devices.unnamed');

  // Forgetting is destructive: the row's ⋯, confirmed by a Dialog S (PS-6). This device and one
  // already forgotten have nothing to forget.
  function rowMenu(device: (typeof devices.all)[number]): DataRowMenu | undefined {
    if (device.blocked || devices.isThisDevice(device.id)) return undefined;
    return { label: t('app.page.more_for', { name: nameOf(device) }), items: [{ id: 'forget', label: t('app.devices.forget_menu'), isDestructive: true }] };
  }

  function when(at: string | null | undefined): string {
    return at ? formatDateTime(at, messages.locale) : t('app.devices.never_seen');
  }

  async function forgetDevice(): Promise<void> {
    const id = forgetting;
    forgetting = undefined;
    if (!id) return;
    failure = undefined;
    try {
      await devices.forget(id);
      announcer.say(t('app.devices.forgotten_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <ListPage head={profileHead('devices')}>
    {#snippet intro()}<p class="quiet">{t('app.devices.intro')}</p>{/snippet}
    {#snippet filters()}
      <FilterBar
        searchLabel={t('app.devices.search')}
        clearSearchLabel={t('app.page.clear_search')}
        bind:search
        countLabel={held?.status === 'ready' ? t('app.page.found', { shown: String(found.length), count: String(devices.all.length) }) : undefined}
      />
    {/snippet}

    {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}

    {#if held === undefined || held.status === 'loading' || held.status === 'idle'}
      <div aria-busy="true"><Skeleton lines={2} /></div>
    {:else if held.status === 'failed'}
      <FailureState problem={renderProblem(held.error, messages)} onRetry={() => devices.open()} />
    {:else}
      <DataTable
        label={title}
        {columns}
        rows={continued.shown}
        key={(device) => device.id}
        rowLabel={nameOf}
        menu={rowMenu}
        onmenu={(device) => (forgetting = device.id)}
        actionsLabel={t('app.page.row_actions')}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
        sort={order.sort}
        onSort={(next) => order.by(next)}
        shownLabel={devices.all.length === 0 ? undefined : continued.shownLabel}
        more={continued.more}
        perPage={continued.perPage}
      >
        {#snippet cell(device, column: DataColumn)}
          {#if column.id === 'device'}
            <span class="what">{nameOf(device)}</span>
            {#if devices.isThisDevice(device.id)}<span class="here">{t('app.devices.this_device')}</span>{/if}
          {:else if column.id === 'platform'}
            {device.platform || '—'}
          {:else if column.id === 'seen'}
            {when(device.last_seen_at)}
          {:else}
            {device.blocked ? t('app.devices.forgotten') : t('app.devices.synchronising')}
          {/if}
        {/snippet}
        {#snippet empty()}
          {#if devices.all.length === 0}
            <EmptyState kind="unused" title={t('app.devices.none')} />
          {:else}
            <EmptyState kind="filtered" title={t('app.devices.filtered')}>
              {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
            </EmptyState>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  </ListPage>

  <Dialog
    size="s"
    title={t('app.devices.confirm_title')}
    isOpen={forgetting !== undefined}
    dismissLabel={t('app.devices.cancel')}
    onClose={() => (forgetting = undefined)}
  >
    {#snippet actions()}
      <Button onclick={() => (forgetting = undefined)}>{t('app.devices.cancel')}</Button>
      <Button tone="danger" onclick={() => void forgetDevice()}>{t('app.devices.confirm')}</Button>
    {/snippet}
    {t('app.devices.confirm_body')}
  </Dialog>
{/if}

<style>
  .what { font-weight: var(--fw-medium); }

  /* Which row is the device in front of the reader, beside its name: true of exactly one row. */
  .here {
    margin-inline-start: var(--sp-100);
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-full);
    background: var(--bg-surface-pressed);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-regular);
  }


  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
