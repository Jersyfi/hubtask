<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Reset second factor* asks first (fifth answers 6): it removes the person's authenticator app
  // and recovery codes and signs them out everywhere, so a single keystroke on a focused row must
  // not do it. The fresh proof the server demands comes after this, from the step-up prompt.

  import { Button, Dialog } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Whose second factor is reset; the dialog is open exactly while there is one. */
    who: string | undefined;
    isBusy?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  }

  const { who, isBusy = false, onConfirm, onCancel }: Props = $props();
</script>

<Dialog
  size="s"
  title={t('app.people.factor_reset_title')}
  isOpen={who !== undefined}
  dismissLabel={t('app.workspace.cancel')}
  onClose={onCancel}
>
  {#snippet actions()}
    <Button onclick={onCancel}>{t('app.workspace.cancel')}</Button>
    <Button tone="danger" {isBusy} busyLabel={t('app.people.working')} onclick={onConfirm}>
      {t('app.people.factor_reset_confirm')}
    </Button>
  {/snippet}
  {#if who !== undefined}
    <p class="body">{t('app.people.factor_reset_body', { who })}</p>
  {/if}
</Dialog>

<style>
  .body { margin: 0; }
</style>
