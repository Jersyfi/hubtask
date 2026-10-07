<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The keyring, and what still names each key (SI-12, ADR-0045, security.md §8.1).
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

  import { Badge, Banner, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    const stop = untrack(() => instance.openEncryption());
    return stop;
  });

  const reading = $derived(instance.encryption);
  const status = $derived(reading.status === 'ready' ? reading.data : undefined);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const columns = $derived([
    { id: 'key', label: t('app.instance.column_key') },
    { id: 'standing', label: t('app.instance.column_standing') },
    { id: 'sealed', label: t('app.instance.column_sealed') },
  ]);

  /** A key nothing holds while something still names it — the one state that needs repairing. */
  const stranded = $derived(
    (status?.keys ?? []).filter((key) => !key.in_ring && key.sealed_values > 0),
  );

  $effect(() => page.entitle(t('app.instance.encryption')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.encryption')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.encryption_intro')}</p>

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else if status && status.active_key_id === ''}
        <Banner tone="info">{t('app.instance.encryption_none')}</Banner>
      {:else if status}
        {#if stranded.length > 0}
          <Banner tone="danger" title={t('app.instance.encryption_stranded_title')}>
            {t('app.instance.encryption_stranded', { keys: stranded.map((key) => key.key_id).join(', ') })}
          </Banner>
        {/if}

        <p class="quiet-line">{t('app.instance.encryption_active', { key: status.active_key_id })}</p>

        <Table label={t('app.instance.encryption')} isLabelHidden columns={columns}>
          {#each status.keys as key (key.key_id)}
            <tr>
              <td class="mono">{key.key_id}</td>
              <td>
                {#if key.active}
                  <Badge tone="success">{t('app.instance.key_active')}</Badge>
                {:else if !key.in_ring}
                  <Badge tone="danger">{t('app.instance.key_missing')}</Badge>
                {:else if key.sealed_values === 0}
                  <Badge tone="neutral">{t('app.instance.key_removable')}</Badge>
                {:else}
                  <Badge tone="warning">{t('app.instance.key_held')}</Badge>
                {/if}
              </td>
              <td class="numeric">{key.sealed_values}</td>
            </tr>
          {/each}
        </Table>

        <!-- Said where the absent buttons would be, rather than left to be noticed. -->
        <Banner tone="info" title={t('app.instance.encryption_rotation_title')}>
          {t('app.instance.encryption_rotation')}
        </Banner>
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-line { margin: 0; max-inline-size: 60ch; color: var(--text-secondary); font-size: var(--fs-100); }
  .mono { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
  .numeric { font-variant-numeric: tabular-nums; }
</style>
