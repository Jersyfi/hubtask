<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The question a sign-out asks while changes are still waiting to be sent (offline-sync.md
  // §9.6): signing out deletes this device's store, the queue with it, so the person chooses -
  // send first, or discard them. *Send first* signs out only once nothing is waiting any more;
  // while the server cannot be reached it says so and keeps the dialog open.

  import { Button, Dialog } from '@hubtask/design-system/components';

  import { session } from '../session.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** How many changes are waiting; zero closes the dialog. */
    count: number;
    onClose: () => void;
  }

  const { count, onClose }: Props = $props();

  let isSending = $state(false);
  let left = $state<number | undefined>(undefined);
  const waiting = $derived(left ?? count);

  async function sendFirst() {
    isSending = true;
    try {
      left = await session.sendUnsent();
      if (left === 0) {
        close();
        await session.signOut();
      }
    } finally {
      isSending = false;
    }
  }

  async function discard() {
    close();
    await session.signOut();
  }

  function close() {
    left = undefined;
    onClose();
  }
</script>

<Dialog title={t('app.signout.unsent_title')} isOpen={count > 0} dismissLabel={t('app.workspace.cancel')} onClose={close}>
  <p class="body">{t('app.signout.unsent_body', { count: waiting })}</p>
  {#if left !== undefined && left > 0}
    <p class="body" role="status">{t('app.signout.unsent_still')}</p>
  {/if}
  {#snippet actions()}
    <Button tone="secondary" onclick={close}>{t('app.workspace.cancel')}</Button>
    <Button tone="danger" onclick={() => void discard()}>{t('app.signout.unsent_discard')}</Button>
    <Button tone="primary" isBusy={isSending} busyLabel={t('app.signout.unsent_sending')} onclick={() => void sendFirst()}>{t('app.signout.unsent_send')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
