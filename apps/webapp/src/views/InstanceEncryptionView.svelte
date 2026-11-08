<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The keyring, and what still names each key (ADR-0045, security.md §8.1).
  //
  // **This screen reads and does not turn.** It is the concept's own exception to the parity rule:
  // "der Schlüsselring bleibt in der Umgebung und in `/admin/encryption`; ein Dashboard zeigt seinen
  // Zustand und dreht ihn nicht". A rotation is an operator with the new key in their hand and the
  // environment in front of them — a button here could start one that no process is holding the key
  // for, which is how a ring ends up with values nothing can open.
  //
  // **The count is the gate, not the calendar.** A key may leave the ring when nothing names it any
  // more; a key that is gone while a row still names it is a row that answers a refusal until
  // somebody puts the key back. That is the one state on this screen worth a colour.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, DataTable, EmptyState, FilterBar, ListPage, Spinner, Stack, type DataColumn } from '@hubtask/design-system/components';
  import type { EncryptionStatus } from '@hubtask/sync-engine';

  type EncryptionKey = EncryptionStatus['keys'][number];

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    if (!instance.isElevated) return;
    const stop = untrack(() => instance.openEncryption());
    return stop;
  });

  const reading = $derived(instance.encryption);
  const status = $derived(reading.status === 'ready' ? reading.data : undefined);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const columns = $derived<DataColumn[]>([
    { id: 'key', label: t('app.instance.column_key'), priority: 1 },
    { id: 'standing', label: t('app.instance.column_standing'), priority: 2, isStatus: true },
    { id: 'sealed', label: t('app.instance.column_sealed'), align: 'end', priority: 2 },
  ]);

  let search = $state('');
  const keys = $derived(status?.keys ?? []);
  const shown = $derived(keys.filter((key) => matchesSearch(search, [key.key_id])));

  /** A key nothing holds while something still names it — the one state that needs repairing. */
  const stranded = $derived(
    (status?.keys ?? []).filter((key) => !key.in_ring && key.sealed_values > 0),
  );

  $effect(() => page.entitle(t('app.instance.encryption')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <ListPage head={{ title: t('app.instance.encryption'), breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.encryption') }) }} refused={door ?? (unreadable ? failed : undefined)}>
      {#snippet intro()}
        <Stack gap="100">
          <p class="quiet">{t('app.instance.encryption_intro')}</p>
          {#if status && status.active_key_id !== ''}
            <p class="quiet-line">{t('app.instance.encryption_active', { key: status.active_key_id })}</p>
          {/if}
        </Stack>
      {/snippet}
      {#snippet filters()}
        {#if status && status.active_key_id !== ''}
          <FilterBar
            searchLabel={t('app.instance.encryption_search')}
            clearSearchLabel={t('app.page.clear_search')}
            bind:search
            countLabel={t('app.page.found', { shown: String(shown.length), count: String(keys.length) })}
          />
        {/if}
      {/snippet}

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else if status && status.active_key_id === ''}
        <EmptyState kind="unused" title={t('app.instance.encryption_none')} />
      {:else if status}
        {#if stranded.length > 0}
          <Banner tone="danger" title={t('app.instance.encryption_stranded_title')}>
            <p class="banner-line">
              {t('app.instance.encryption_stranded', { keys: stranded.map((key) => key.key_id).join(', ') })}
            </p>
            <!-- What would stay unreadable, by kind: the count is the installation's, never a workspace's. -->
            <ul class="kinds">
              {#each stranded as key (key.key_id)}
                {#each key.stores as kind (kind.store)}
                  <li>{t('app.instance.encryption_stranded_kind', { key: key.key_id, count: kind.sealed_values, store: kind.store })}</li>
                {/each}
              {/each}
            </ul>
          </Banner>
        {/if}
        <DataTable
          label={t('app.instance.encryption')}
          {columns}
          rows={shown}
          key={(key: EncryptionKey) => key.key_id}
          rowLabel={(key: EncryptionKey) => key.key_id}
          shownLabel={t('app.page.showing', { count: String(shown.length) })}
        >
          {#snippet cell(key: EncryptionKey, column: DataColumn)}
            {#if column.id === 'key'}
              <span class="mono">{key.key_id}</span>
            {:else if column.id === 'standing'}
              {#if key.active}
                <Badge tone="success">{t('app.instance.key_active')}</Badge>
              {:else if !key.in_ring}
                <Badge tone="danger">{t('app.instance.key_missing')}</Badge>
              {:else if key.sealed_values === 0}
                <Badge tone="neutral">{t('app.instance.key_removable')}</Badge>
              {:else}
                <Badge tone="warning">{t('app.instance.key_held')}</Badge>
              {/if}
            {:else}
              <span class="numeric">{key.sealed_values}</span>
            {/if}
          {/snippet}
          {#snippet empty()}
            <EmptyState kind="filtered" title={t('app.instance.encryption_none_found')}>
              {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
            </EmptyState>
          {/snippet}
        </DataTable>
        <!-- Said where the absent buttons would be, rather than left to be noticed. -->
        <Banner tone="info" title={t('app.instance.encryption_rotation_title')}>
          {t('app.instance.encryption_rotation')}
        </Banner>
      {/if}
    </ListPage>
  {/snippet}
</InstanceGate>

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-line { margin: 0; max-inline-size: 60ch; color: var(--text-secondary); font-size: var(--fs-100); }
  .mono { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
  .numeric { font-variant-numeric: tabular-nums; }
  .banner-line { margin: 0; }
  .kinds { margin: 0; padding-inline-start: var(--sp-300); }
</style>
