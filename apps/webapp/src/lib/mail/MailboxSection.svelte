<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Connected mailboxes (ADR-0094): the door for a workspace without a receiving domain - one
  // existing mailbox at Gmail, GMX, web.de or IONOS, or any IMAP or JMAP server. Only mail that
  // arrives after connecting is taken, so connecting a personal mailbox takes nothing already in it.
  //
  // **What is connected comes first** (user-experience.md §7.10): a table section (page-system
  // §9.3) whose rows say each mailbox's state in words and, where a read failed, the failure beside
  // *Check now*. A mailbox opens as a Dialog M; the page holds the list and the dialogs.

  import { Badge, Button, DataTable, EmptyState, Spinner, type DataColumn } from '@hubtask/design-system/components';

  import FailureNote from '../frame/FailureNote.svelte';
  import { mailboxStateOf, type Mailbox } from '../data/mailintake.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import TableSection from '../page/TableSection.svelte';
  import { STATE_TONE, wayOf, type MailboxList } from './mailboxes.svelte.ts';

  interface Props {
    list: MailboxList;
  }

  const { list }: Props = $props();

  const columns: readonly DataColumn[] = $derived([
    { id: 'address', label: t('app.mail.mailbox_column'), priority: 1 },
    { id: 'way', label: t('app.mail.mailbox_column_way'), priority: 2 },
    { id: 'read', label: t('app.mail.mailbox_column_read'), priority: 3 },
    { id: 'state', label: t('app.mail.mailbox_column_state'), isStatus: true },
  ]);

  // Every verb is also in the menu: a phone shows only the ⋯ (PS-18).
  function rowMenu(box: Mailbox) {
    return {
      label: t('app.page.more_for', { name: box.address }),
      items: [
        { id: 'open', label: t('app.page.open') },
        { id: 'check', label: t('app.mail.mailbox_check') },
        { id: 'remove', label: t('app.mail.mailbox_remove'), isDestructive: true, hasSeparatorBefore: true },
      ],
    };
  }

  function onRowMenu(box: Mailbox, id: string): void {
    if (id === 'remove') list.askRemoval(box);
    else if (id === 'check') void list.check(box);
    else list.openedId = box.id;
  }
</script>

<div data-mail-mailboxes>
  <TableSection title={t('app.mail.section_mailboxes')} note={list.isReading ? undefined : String(list.mailboxes.length)} description={t('app.mail.mailboxes_hint')}>
    {#if list.isReading}
      <p class="quiet"><Spinner label={t('app.mail.reading')} /> {t('app.mail.reading')}</p>
    {:else if list.readFailure}
      <FailureNote problem={list.readFailure} />
    {:else}
      {#if list.isCompleting}
        <p class="quiet" data-mailbox-completing><Spinner label={t('app.mail.oauth_completing')} /> {t('app.mail.oauth_completing')}</p>
      {/if}
      {#if list.oauthFailure && !list.isAdding}<div data-mailbox-oauth-failure><FailureNote problem={list.oauthFailure} /></div>{/if}
      {#if list.rowFailure && !list.removing}<FailureNote problem={list.rowFailure} />{/if}
      <DataTable
        label={t('app.mail.section_mailboxes')}
        {columns}
        rows={list.mailboxes}
        key={(box: Mailbox) => box.id}
        onopen={(box: Mailbox) => (list.openedId = box.id)}
        rowLabel={(box: Mailbox) => box.address}
        menu={rowMenu}
        onmenu={onRowMenu}
        actionsLabel={t('app.page.row_actions')}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
      >
        {#snippet cell(box: Mailbox, column: DataColumn)}
          {@const state = mailboxStateOf(box)}
          {#if column.id === 'address'}
            <span class="address" data-mailbox={box.address} data-mailbox-is={state}>{box.address}</span>
          {:else if column.id === 'way'}
            <span class="quiet">{wayOf(box)}</span>
            {#if box.failure && state !== 'off'}
              <!-- The failure beside the act that tries again: whoever presses it knows why. -->
              <span class="failure" data-mailbox-failure>{t(box.failure)}</span>
            {/if}
          {:else if column.id === 'read'}
            <span class="quiet" data-mailbox-state>
              {#if box.checked_at}
                {formatDateTime(box.checked_at, messages.locale)}
              {:else if !box.failure}
                {t('app.mail.mailbox_waiting')}
              {/if}
            </span>
          {:else}
            <Badge size="sm" tone={STATE_TONE[state]}>{t(`app.mail.mailbox_state.${state}`)}</Badge>
          {/if}
        {/snippet}
        {#snippet verb(box: Mailbox)}
          <Button size="sm" tone={box.failure && mailboxStateOf(box) !== 'off' ? 'secondary' : 'subtle'} isBusy={list.working === box.id} busyLabel={t('app.mail.checking')} onclick={() => void list.check(box)}>
            {t('app.mail.mailbox_check')}
          </Button>
        {/snippet}
        {#snippet empty()}
          {#if !list.isCompleting}
            <EmptyState kind="unused" icon="mail" title={t('app.mail.mailboxes_empty_title')} description={t('app.mail.mailboxes_hint')} action={connectAct} />
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  </TableSection>
</div>

{#snippet connectAct()}
  <Button tone="primary" icon="plus" onclick={() => (list.isAdding = true)}>{t('app.mail.mailbox_add')}</Button>
{/snippet}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .address { overflow-wrap: anywhere; font-weight: var(--fw-medium); color: var(--text-primary); }

  .failure { display: block; color: var(--text-danger); font-size: var(--fs-075); }
</style>
