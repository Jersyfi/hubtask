<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A connected mailbox as a Dialog M (page-system §5.1): it has no lists of its own and no settings
  // after connecting - its state, how it is reached, when it was last read and why a read failed -
  // so the record-page rung does not fit. *Check now* is its verb; disconnecting closes this dialog
  // and asks in a Dialog S, never a second dialog over this one.
  import { Badge, Banner, Button, Dialog, Stack } from '@hubtask/design-system/components';

  import { mailboxStateOf, type Mailbox } from '../data/mailintake.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { STATE_TONE, wayOf } from './mailboxes.ts';

  interface Props {
    box: Mailbox | undefined;
    isChecking: boolean;
    onClose: () => void;
    oncheck: (box: Mailbox) => void;
    onremove: (box: Mailbox) => void;
  }

  const { box, isChecking, onClose, oncheck, onremove }: Props = $props();
  const state = $derived(box ? mailboxStateOf(box) : 'off');
</script>

<Dialog
  size="m"
  isOpen={box !== undefined}
  title={box?.address ?? ''}
  dismissLabel={t('app.page.close')}
  {onClose}
  primary={box ? { label: t('app.mail.mailbox_check'), isBusy: isChecking, busyLabel: t('app.mail.checking'), onclick: () => oncheck(box) } : undefined}
>
  {#if box}
    <Stack gap="150">
      <span><Badge tone={STATE_TONE[state]}>{t(`app.mail.mailbox_state.${state}`)}</Badge></span>
      <dl class="facts">
        <dt>{t('app.mail.mailbox_column_way')}</dt>
        <dd>{wayOf(box)}</dd>
        <dt>{t('app.mail.mailbox_column_read')}</dt>
        <dd>{box.checked_at ? formatDateTime(box.checked_at, messages.locale) : t('app.mail.mailbox_waiting')}</dd>
      </dl>
      {#if box.failure && state !== 'off'}
        <Banner tone={state === 'failing' ? 'danger' : 'warning'}><span data-mailbox-failure>{t(box.failure)}</span></Banner>
      {/if}
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="danger" onclick={() => box && onremove(box)}>{t('app.mail.mailbox_remove')}</Button>
    <Button tone="subtle" onclick={onClose}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .facts { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: var(--sp-100) var(--sp-200); margin: 0; }
  .facts dt { color: var(--text-secondary); }
  .facts dd { margin: 0; overflow-wrap: anywhere; }
</style>
