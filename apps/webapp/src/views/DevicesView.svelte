<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The devices that hold a copy of this workspace for the reader (F6-07).
  //
  // **A table, and the newest first** (ADR-0065 decision 3), for the reason the sessions beside it
  // are one: four facts a row, read across, is unreadable at the number of devices a year
  // produces. **A forgotten device is blocked, not erased** (N-03), so the row says so rather than
  // going — what it has not sent yet, it keeps.

  import { untrack } from 'svelte';

  import { Button, Dialog, EmptyState, ErrorState, Skeleton, Stack, Table } from '@hubtask/design-system/components';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { devices } from '../lib/data/devices.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const account = $derived(actor.account);

  $effect(() => untrack(() => devices.open()));

  const held = $derived(devices.state);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The device a forget is being confirmed for. */
  let forgetting = $state<string | undefined>(undefined);

  const columns = [
    { id: 'device', label: t('app.devices.name') },
    { id: 'platform', label: t('app.devices.platform') },
    { id: 'seen', label: t('app.devices.last_seen') },
    { id: 'standing', label: t('app.devices.standing') },
    { id: 'forget', label: t('app.devices.forget'), isLabelHidden: true, align: 'end' as const },
  ];

  const shown = $derived(
    [...devices.all].sort((a, b) => {
      const mine = devices.isThisDevice(a.id) !== devices.isThisDevice(b.id);
      if (mine) return devices.isThisDevice(a.id) ? -1 : 1;
      return String(b.last_seen_at ?? '').localeCompare(String(a.last_seen_at ?? ''));
    }),
  );

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
  <Stack gap="300">
    <SettingsHead row="devices" />

    <Stack gap="150">
      <p class="quiet">{t('app.devices.intro')}</p>

      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}

      {#if held === undefined || held.status === 'loading' || held.status === 'idle'}
        <div aria-busy="true"><Skeleton lines={2} /></div>
      {:else if held.status === 'failed'}
        <ErrorState
          title={renderProblem(held.error, messages).message}
          retryLabel={t('app.retry')}
          onRetry={() => devices.open()}
        />
      {:else if shown.length === 0}
        <p class="quiet">{t('app.devices.none')}</p>
      {:else}
        <Table label={t('app.devices.title')} isLabelHidden {columns}>
          {#each shown as device (device.id)}
            <tr>
              <th scope="row" class="what">
                {device.display_name || t('app.devices.unnamed')}
                {#if devices.isThisDevice(device.id)}<span class="here">{t('app.devices.this_device')}</span>{/if}
              </th>
              <td>{device.platform || '—'}</td>
              <td>{when(device.last_seen_at)}</td>
              <td>{device.blocked ? t('app.devices.forgotten') : t('app.devices.synchronising')}</td>
              <td class="end">
                {#if !device.blocked && !devices.isThisDevice(device.id)}
                  <Button tone="danger" size="sm" onclick={() => (forgetting = device.id)}>
                    {t('app.devices.forget')}
                  </Button>
                {/if}
              </td>
            </tr>
          {/each}
        </Table>
      {/if}
    </Stack>
  </Stack>

  <Dialog
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

  .end { text-align: end; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
