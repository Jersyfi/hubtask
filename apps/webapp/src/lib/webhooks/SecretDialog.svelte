<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A webhook's signing secret as a Dialog S (page-system §9.3): **the only time this value exists
  // anywhere but the server's sealed copy.** Held in the caller's one `$state` while the dialog is
  // open and dropped when it closes; the acknowledgement is required, because a secret scrolled
  // past is a secret nobody kept.
  import { Dialog, OneTimeSecret } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    secret: string | undefined;
    onClose: () => void;
  }

  const { secret, onClose }: Props = $props();
</script>

<Dialog size="s" isOpen={secret !== undefined} title={t('app.webhooks.secret_title')} {onClose}>
  {#if secret !== undefined}
    <OneTimeSecret
      value={secret}
      label={t('app.webhooks.secret_title')}
      hint={t('app.webhooks.secret_hint')}
      revealLabel={t('app.webhooks.reveal')}
      hideLabel={t('app.webhooks.hide')}
      copyLabel={t('app.webhooks.copy')}
      copiedLabel={t('app.webhooks.copied')}
      acknowledgementLabel={t('app.webhooks.kept')}
      notAcknowledgedReason={t('app.webhooks.keep_first')}
      dismissLabel={t('app.webhooks.done')}
      onDismiss={onClose}
    />
  {/if}
</Dialog>
