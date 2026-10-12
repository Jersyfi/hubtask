<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *New start password* asks first (UC-ID-20 check 5): it ends every session of the person and
  // their current password, so a single keystroke on a focused row must not do it. The fresh proof
  // the server demands comes after this, from the step-up prompt.

  import { Button, Dialog } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Whose start password is renewed; the dialog is open exactly while there is one. */
    who: string | undefined;
    isBusy?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  }

  const { who, isBusy = false, onConfirm, onCancel }: Props = $props();
</script>

<Dialog
  title={t('app.people.renew_title')}
  isOpen={who !== undefined}
  dismissLabel={t('app.workspace.cancel')}
  onClose={onCancel}
>
  {#snippet actions()}
    <Button onclick={onCancel}>{t('app.workspace.cancel')}</Button>
    <Button tone="primary" {isBusy} busyLabel={t('app.people.working')} onclick={onConfirm}>
      {t('app.people.renew_confirm')}
    </Button>
  {/snippet}
  {#if who !== undefined}
    <p class="body">{t('app.people.renew_body', { who })}</p>
  {/if}
</Dialog>

<style>
  .body { margin: 0; }
</style>
