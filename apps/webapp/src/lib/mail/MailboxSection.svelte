<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Connected mailboxes (ADR-0094): the door for a workspace without a receiving domain - one
  // existing mailbox at Gmail, GMX, web.de or IONOS, or any IMAP or JMAP server. Only mail that
  // arrives after connecting is taken, so connecting a personal mailbox takes nothing already in it.
  //
  // **What is connected comes first** (user-experience.md §7.10): a table section (page-system
  // §9.3) whose rows say each mailbox's state in words and, where a read failed, the failure beside
  // *Check now*. A mailbox opens as a Dialog M; connecting is the page head's Dialog L.
  //
  // Drawn on every section of the page, the table only on its own: the provider's answer on
  // `/administration/mail/oauth` is handed on here, and the page reads the list for the handles.

  import { untrack } from 'svelte';

  import { Badge, Button, DataTable, EmptyState, Spinner, type DataColumn } from '@hubtask/design-system/components';

  import FailureNote from '../frame/FailureNote.svelte';
  import { announcer } from '../announce.svelte.ts';
  import {
    checkMailbox,
    completeMailboxAuthorization,
    readMailboxes,
    mailboxStateOf,
    removeMailbox,
    type Mailbox,
    type MailboxOAuthProvider,
    type MailboxPreset,
  } from '../data/mailintake.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import TableSection from '../page/TableSection.svelte';
  import { renderProblem } from '../problem.ts';
  import ConnectMailboxDialog from './ConnectMailboxDialog.svelte';
  import MailboxDialog from './MailboxDialog.svelte';
  import RemoveMailboxDialog from './RemoveMailboxDialog.svelte';
  import { STATE_TONE, wayOf } from './mailboxes.ts';

  interface Props {
    /** Told when the list changes, so the page can show addresses through the mailbox. */
    onchange?: (mailboxes: readonly Mailbox[]) => void;
    /** What the provider sent back, on `/administration/mail/oauth`. */
    providerAnswer?: Readonly<Record<string, string>>;
    /** Told once that answer is handed on. */
    onanswered?: () => void;
    /** Whether the connecting dialog is open; the page's head opens it. */
    isAdding?: boolean;
    /** Whether the table is drawn: the page is on this section. */
    isShown?: boolean;
  }

  let { onchange, providerAnswer, onanswered, isAdding = $bindable(false), isShown = true }: Props = $props();

  let oauthProviders = $state<readonly MailboxOAuthProvider[]>([]);
  let redirectUrl = $state('');
  let isCompleting = $state(false);
  let oauthFailure = $state<string | undefined>(undefined);

  let mailboxes = $state<readonly Mailbox[]>([]);
  let presets = $state<readonly MailboxPreset[]>([]);
  let isReading = $state(true);
  let readFailure = $state<string | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let rowFailure = $state<string | undefined>(undefined);
  let openedId = $state<string | undefined>(undefined);
  let removing = $state<Mailbox | undefined>(undefined);

  const opened = $derived(openedId ? mailboxes.find((each) => each.id === openedId) : undefined);

  function take(next: readonly Mailbox[]) {
    mailboxes = next;
    onchange?.(next);
  }

  $effect(() =>
    untrack(() => {
      void readMailboxes()
        .then((read) => {
          presets = read.presets;
          oauthProviders = read.oauth_providers ?? [];
          redirectUrl = read.oauth_redirect_url ?? '';
          take(read.mailboxes);
        })
        .catch((error) => (readFailure = renderProblem(error as never, messages).message))
        .finally(() => (isReading = false));
    }),
  );

  // Back from the provider: hand its answer on once, then drop the code and the state from the address.
  $effect(() => {
    const answer = providerAnswer;
    if (!answer?.['state']) return;
    untrack(() => void complete(answer));
  });

  async function complete(answer: Readonly<Record<string, string>>) {
    oauthFailure = undefined;
    isCompleting = true;
    try {
      const connected = await completeMailboxAuthorization({
        state: answer['state'] ?? '',
        ...(answer['code'] ? { code: answer['code'] } : {}),
        ...(answer['error'] ? { error: answer['error'] } : {}),
      });
      take([...mailboxes.filter((each) => each.id !== connected.id), connected]);
      announcer.say(t('app.mail.mailbox_connected', { address: connected.address }));
    } catch (error) {
      oauthFailure = renderProblem(error as never, messages).message;
    } finally {
      isCompleting = false;
      onanswered?.();
    }
  }

  async function check(box: Mailbox) {
    rowFailure = undefined;
    working = box.id;
    try {
      const checked = await checkMailbox(box.id);
      take(mailboxes.map((each) => (each.id === box.id ? checked : each)));
      announcer.say(checked.failure ? t(checked.failure) : t('app.mail.mailbox_ok', { address: box.address }));
    } catch (error) {
      rowFailure = renderProblem(error as never, messages).message;
    } finally {
      working = undefined;
    }
  }

  async function remove(box: Mailbox) {
    rowFailure = undefined;
    working = box.id;
    try {
      await removeMailbox(box.id);
      take(mailboxes.filter((each) => each.id !== box.id));
      announcer.say(t('app.mail.mailbox_removed', { address: box.address }));
      removing = undefined;
    } catch (error) {
      rowFailure = renderProblem(error as never, messages).message;
    } finally {
      working = undefined;
    }
  }

  function askRemoval(box: Mailbox): void {
    openedId = undefined;
    removing = box;
  }

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
    if (id === 'remove') askRemoval(box);
    else if (id === 'check') void check(box);
    else openedId = box.id;
  }
</script>

{#if isShown}
  <div data-mail-mailboxes>
    <TableSection title={t('app.mail.section_mailboxes')} note={isReading ? undefined : String(mailboxes.length)} description={t('app.mail.mailboxes_hint')}>
      {#if isReading}
        <p class="quiet"><Spinner label={t('app.mail.reading')} /> {t('app.mail.reading')}</p>
      {:else if readFailure}
        <FailureNote problem={readFailure} />
      {:else}
        {#if isCompleting}
          <p class="quiet" data-mailbox-completing><Spinner label={t('app.mail.oauth_completing')} /> {t('app.mail.oauth_completing')}</p>
        {/if}
        {#if oauthFailure && !isAdding}<div data-mailbox-oauth-failure><FailureNote problem={oauthFailure} /></div>{/if}
        {#if rowFailure}<FailureNote problem={rowFailure} />{/if}
        <DataTable
          label={t('app.mail.section_mailboxes')}
          {columns}
          rows={mailboxes}
          key={(box: Mailbox) => box.id}
          onopen={(box: Mailbox) => (openedId = box.id)}
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
            <Button size="sm" tone={box.failure && mailboxStateOf(box) !== 'off' ? 'secondary' : 'subtle'} isBusy={working === box.id} busyLabel={t('app.mail.checking')} onclick={() => void check(box)}>
              {t('app.mail.mailbox_check')}
            </Button>
          {/snippet}
          {#snippet empty()}
            {#if !isCompleting}
              <EmptyState kind="unused" icon="mail" title={t('app.mail.mailboxes_empty_title')} description={t('app.mail.mailboxes_hint')} action={connectAct} />
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </TableSection>
  </div>
{/if}

{#snippet connectAct()}
  <Button tone="primary" icon="plus" onclick={() => (isAdding = true)}>{t('app.mail.mailbox_add')}</Button>
{/snippet}

<ConnectMailboxDialog bind:isOpen={isAdding} {presets} {oauthProviders} {redirectUrl} {oauthFailure} onconnected={(box) => take([...mailboxes, box])} />
<MailboxDialog box={opened} isChecking={working === opened?.id} onClose={() => (openedId = undefined)} oncheck={(box) => void check(box)} onremove={askRemoval} />
<RemoveMailboxDialog box={removing} isWorking={working === removing?.id} failure={rowFailure} onClose={() => (removing = undefined)} onremove={(box) => void remove(box)} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .address { overflow-wrap: anywhere; font-weight: var(--fw-medium); color: var(--text-primary); }

  .failure { display: block; color: var(--text-danger); font-size: var(--fs-075); }
</style>
