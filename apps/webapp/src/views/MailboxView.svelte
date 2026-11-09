<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A connected mailbox (ADR-0094, page-system §9.3): its record page - *‹ E-mail*, the address, its
  // state, how it is reached, *Check now* - then what recently arrived through it, its settings as
  // read-only facts (nothing of a mailbox changes after connecting), and disconnecting in the
  // `DangerZone`, asked in a Dialog S.
  import { untrack } from 'svelte';

  import { RecordPage, Skeleton, type MenuItem, type RecordList } from '@hubtask/design-system/components';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { mailboxStateOf, readMailReceipts, type MailReceipt } from '../lib/data/mailintake.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { MailboxList, STATE_TONE, wayOf } from '../lib/mail/mailboxes.svelte.ts';
  import ReceiptTable from '../lib/mail/ReceiptTable.svelte';
  import RemoveMailboxDialog from '../lib/mail/RemoveMailboxDialog.svelte';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();
  const LIST = '/administration/mail/mailboxes';

  const boxes = new MailboxList();
  let receipts = $state<readonly MailReceipt[]>([]);
  let isReadingReceipts = $state(true);
  const mayShape = $derived(holdsHere('AUTOMATION') === true);

  $effect(() => {
    if (!mayShape) return;
    untrack(() => {
      void boxes.read();
      void readMailReceipts().then(
        (rows) => (receipts = rows),
        () => undefined,
      ).finally(() => (isReadingReceipts = false));
    });
  });

  const box = $derived(boxes.mailboxes.find((each) => each.id === id));
  const standing = $derived(box ? mailboxStateOf(box) : 'off');
  const title = $derived(box?.address ?? (boxes.isReading ? t('app.mail.reading') : t('app.mail.mailbox_not_found')));
  $effect(() => page.entitle(title));

  /** What arrived through this mailbox: a receipt names the mailbox it came through as its door. */
  const arrived = $derived(box ? receipts.filter((receipt: MailReceipt) => receipt.door === box.address) : []);
  const lists = $derived<RecordList[]>([{ id: 'arrived', title: t('app.mail.mailbox_arrived'), count: isReadingReceipts ? undefined : String(arrived.length) }]);

  const facts = $derived(
    box
      ? {
          title: t('app.mail.mailbox_settings'),
          rows: [
            { label: t('app.mail.mailbox_column_way'), value: wayOf(box) },
            ...(box.port ? [{ label: t('app.mail.mailbox_port'), value: String(box.port) }] : []),
            { label: t('app.mail.mailbox_username'), value: box.username },
            { label: t('app.mail.mailbox_folder'), value: box.folder },
            { label: t('app.mail.mailbox_done_folder'), value: box.done_folder },
            { label: t('app.mail.mailbox_plus'), value: box.plus_addresses ? t('app.mail.mailbox_plus_yes') : t('app.mail.mailbox_plus_no') },
            { label: t('app.mail.mailbox_column_read'), value: box.checked_at ? formatDateTime(box.checked_at, messages.locale) : t('app.mail.mailbox_waiting') },
            ...(box.failure && standing !== 'off' ? [{ label: t('app.mail.mailbox_failure'), value: t(box.failure) }] : []),
            { label: t('app.mail.mailbox_connected_at'), value: formatDateTime(box.created_at, messages.locale) },
          ],
        }
      : undefined,
  );

  const menu = $derived<MenuItem[]>(box ? [{ id: 'remove', label: t('app.mail.mailbox_remove'), icon: 'trash', isDestructive: true }] : []);

  async function remove(): Promise<void> {
    const removing = boxes.removing;
    if (removing && (await boxes.remove(removing))) onnavigate(LIST);
  }
</script>

<RecordPage
  record={{
    kind: t('app.mail.mailbox_kind'),
    title,
    back: { label: t('app.admin.mail'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status: box ? { label: t(`app.mail.mailbox_state.${standing}`), tone: STATE_TONE[standing] } : undefined,
    facts: box ? wayOf(box) : undefined,
    edit: box
      ? { label: t('app.mail.mailbox_check'), icon: 'rotate-ccw', onclick: () => void boxes.check(box), disabledReason: boxes.working === box.id ? t('app.mail.checking') : undefined }
      : undefined,
    menu: box ? { label: t('app.page.more_for', { name: box.address }), items: menu, onselect: () => boxes.askRemoval(box) } : undefined,
  }}
  lists={box ? lists : []}
  {facts}
  danger={box
    ? { title: t('app.page.danger_zone'), acts: [{ id: 'remove', title: t('app.mail.mailbox_remove_act'), cost: t('app.mail.mailbox_remove_note'), actionLabel: t('app.mail.mailbox_remove') }], onact: () => boxes.askRemoval(box) }
    : undefined}
  refused={!mayShape ? notPermitted : boxes.readFailure ? unread : !boxes.isReading && !box ? missing : undefined}
>
  {#snippet list()}
    {#if boxes.rowFailure && !boxes.removing}<FailureNote problem={boxes.rowFailure} />{/if}
    {#if isReadingReceipts}
      <Skeleton lines={3} />
    {:else}
      <div data-mailbox-arrived><ReceiptTable label={t('app.mail.mailbox_arrived')} receipts={arrived} /></div>
    {/if}
  {/snippet}
</RecordPage>

{#snippet notPermitted()}<WorkspaceGate permission="AUTOMATION" />{/snippet}
{#snippet unread()}<FailureNote problem={boxes.readFailure ?? ''} />{/snippet}
{#snippet missing()}<p class="quiet">{t('app.mail.mailbox_not_found')}</p>{/snippet}

<RemoveMailboxDialog
  box={boxes.removing}
  isWorking={boxes.working !== undefined && boxes.working === boxes.removing?.id}
  failure={boxes.rowFailure}
  onClose={() => (boxes.removing = undefined)}
  onremove={() => void remove()}
/>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
