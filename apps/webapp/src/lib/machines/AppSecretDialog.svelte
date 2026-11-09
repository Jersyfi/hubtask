<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What registering answered, once (page-system §9.3's rung for a secret: `OneTimeSecret` in a
  // Dialog S). A confidential app's secret exists nowhere else but the server's hash; a public app
  // gets none, and saying so is better than an empty panel somebody reads as a failure. The secret
  // is held by the caller only while this is open.
  import { Button, Dialog, OneTimeSecret } from '@hubtask/design-system/components';

  import type { RegisteredApp } from '../data/apps.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  const { registered, onClose }: { registered: RegisteredApp | undefined; onClose: () => void } = $props();
</script>

<Dialog size="s" title={registered?.name ?? ''} isOpen={registered !== undefined} {onClose}>
  {#if registered?.client_secret}
    <OneTimeSecret
      value={registered.client_secret}
      label={t('app.apps.secret_title')}
      hint={t('app.apps.secret_hint')}
      revealLabel={t('app.apps.reveal')}
      hideLabel={t('app.apps.hide')}
      copyLabel={t('app.apps.copy')}
      copiedLabel={t('app.apps.copied')}
      acknowledgementLabel={t('app.apps.kept')}
      notAcknowledgedReason={t('app.apps.keep_first')}
      dismissLabel={t('app.apps.done')}
      onDismiss={onClose}
    />
  {:else if registered}
    <p class="body" data-app-no-secret>{t('app.apps.no_secret')}</p>
  {/if}
  {#snippet actions()}
    {#if registered && !registered.client_secret}<Button tone="primary" onclick={onClose}>{t('app.apps.done')}</Button>{/if}
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
