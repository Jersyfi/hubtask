<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What the last messages became (UC-JUM-10): one table for *Recently arrived* on *E-mail* and
  // for what one mailbox brought on its record page (page-system §9.3), so the two say a message
  // the same way (PS-14).
  import { DataTable, EmptyState, type DataColumn } from '@hubtask/design-system/components';

  import type { MailReceipt } from '../data/mailintake.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    label: string;
    receipts: readonly MailReceipt[];
  }

  const { label, receipts }: Props = $props();

  const columns: readonly DataColumn[] = $derived([
    { id: 'subject', label: t('app.mail.log_subject'), priority: 1 },
    { id: 'from', label: t('app.mail.log_from'), priority: 2 },
    { id: 'when', label: t('app.mail.log_when'), priority: 3 },
    { id: 'became', label: t('app.mail.log_became'), isStatus: true },
  ]);

  function outcomeOf(receipt: MailReceipt): string {
    const became = t(`app.mail.outcome.${receipt.outcome}`);
    return receipt.reason ? `${became} · ${t(`app.mail.reason.${receipt.reason}`)}` : became;
  }

  function hrefOf(receipt: MailReceipt): string | undefined {
    if (receipt.item_id) return `/items/${receipt.item_id}`;
    if (receipt.entry_id) return '/jumble';
    return undefined;
  }
</script>

<DataTable
  {label}
  {columns}
  rows={receipts}
  key={(receipt: MailReceipt) => receipt.id}
  rowLabel={(receipt: MailReceipt) => receipt.subject || receipt.sender}
  shownLabel={receipts.length === 0 ? undefined : t('app.page.showing', { count: String(receipts.length) })}
>
  {#snippet cell(receipt: MailReceipt, column: DataColumn)}
    {@const href = hrefOf(receipt)}
    {#if column.id === 'subject'}
      <span class="name" data-receipt={receipt.outcome}>{receipt.subject || '—'}</span>
    {:else if column.id === 'from'}
      <span class="quiet">{receipt.sender}</span>
    {:else if column.id === 'when'}
      <span class="quiet">{formatDateTime(receipt.received_at, messages.locale)}</span>
    {:else if href}
      <a {href}>{outcomeOf(receipt)}</a>
    {:else}
      <span>{outcomeOf(receipt)}</span>
    {/if}
  {/snippet}
  {#snippet empty()}<EmptyState kind="unused" title={t('app.mail.log_empty')} />{/snippet}
</DataTable>

<style>
  .quiet { color: var(--text-secondary); }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); overflow-wrap: anywhere; }
</style>
