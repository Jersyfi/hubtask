<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The question a final act asks first (ux-inventory.md §E0, "final = a dialog"): what ends, and
  // that it does not come back. *Cancel* sends nothing; the confirmation is the one write, busy in
  // place, and the dialog closes either way - a refusal is said on the page, not behind the dialog.
  //
  // For the acts of Your settings that take access away: ending a session, withdrawing an app or a
  // token. None of them has an undo - a session ended or a token withdrawn is gone on the server -
  // so none of them happens on one press.

  import { Button, Dialog } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Whether the question is asked; the caller holds what it is about. */
    isOpen: boolean;
    title: string;
    /** What happens, and that it is final. */
    body: string;
    confirmLabel: string;
    busyLabel: string;
    onConfirm: () => Promise<void>;
    onClose: () => void;
  }

  const { isOpen, title, body, confirmLabel, busyLabel, onConfirm, onClose }: Props = $props();

  let isBusy = $state(false);

  async function confirm() {
    isBusy = true;
    try {
      await onConfirm();
    } finally {
      isBusy = false;
      onClose();
    }
  }
</script>

<Dialog {title} {isOpen} dismissLabel={t('app.workspace.cancel')} {onClose}>
  <p class="body">{body}</p>
  {#snippet actions()}
    <Button tone="secondary" onclick={onClose}>{t('app.workspace.cancel')}</Button>
    <Button tone="danger" {isBusy} {busyLabel} onclick={() => void confirm()}>{confirmLabel}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
