<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Details for support* under a failure (user-experience.md §5.8): the reference an administrator
  // looks the request up by (`GET /audit?request_id=`) and the code, with a control that copies the
  // reference where the browser can copy. FailureNote and FailureState draw it alike.
  import { Button, Disclosure, canCopy } from '@hubtask/design-system/components';
  import { t } from '../i18n/i18n.svelte.ts';
  import { toast } from './toasts.svelte.ts';

  type Props = { requestId: string; code?: string | undefined };
  const { requestId, code }: Props = $props();

  const hasClipboard = canCopy(globalThis.navigator?.clipboard);

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(requestId);
      toast.show({ text: t('app.reference_copied'), tone: 'success' });
    } catch {
      toast.show({ text: t('app.reference_copy_failed'), tone: 'warning' });
    }
  }
</script>

<Disclosure label={t('app.failure.details')} tone="quiet">
  <p class="details">
    {t('app.error_reference', { request_id: requestId })}{#if code}<span class="code"> · <code>{code}</code></span>{/if}
  </p>
  {#if hasClipboard}
    <div><Button tone="secondary" size="sm" icon="copy" onclick={() => void copy()}>{t('app.reference_copy')}</Button></div>
  {/if}
</Disclosure>

<style>
  .details { margin: 0; font-size: var(--fs-075); user-select: all; }
  .code code { font-family: var(--font-mono); }
</style>
