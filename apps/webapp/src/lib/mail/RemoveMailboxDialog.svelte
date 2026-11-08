<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Disconnect the mailbox?* as a Dialog S (PS-6): nothing new arrives from it afterwards, and
  // what it brought stays where it was filed.
  import { Button, Dialog } from '@hubtask/design-system/components';

  import type { Mailbox } from '../data/mailintake.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    box: Mailbox | undefined;
    isWorking: boolean;
    failure?: string;
    onClose: () => void;
    onremove: (box: Mailbox) => void;
  }

  const { box, isWorking, failure, onClose, onremove }: Props = $props();
</script>

<Dialog
  size="s"
  isOpen={box !== undefined}
  title={t('app.mail.mailbox_remove_title', { address: box?.address ?? '' })}
  dismissLabel={t('app.workspace.cancel')}
  {onClose}
  primary={{ label: t('app.mail.mailbox_remove'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.mail.removing'), onclick: () => box && onremove(box) }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body">{t('app.mail.mailbox_remove_note')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
