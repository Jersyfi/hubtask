<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A refusal or a failure on a screen, in one shape everywhere: the sentence, the reference where
  // the sentence does not carry it, and a control that copies the reference - what an
  // administrator looks the request up by (`GET /audit?request_id=`). Offered only where the
  // browser can copy.
  import { Banner, Button, canCopy } from '@hubtask/design-system/components';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { RenderedProblem } from '../problem.ts';
  import { toast } from './toasts.svelte.ts';

  type Props = { problem: Pick<RenderedProblem, 'message' | 'reference' | 'requestId'> };
  const { problem }: Props = $props();

  const hasClipboard = canCopy(globalThis.navigator?.clipboard);

  async function copy(requestId: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(requestId);
      toast.show({ text: t('app.reference_copied'), tone: 'success' });
    } catch {
      toast.show({ text: t('app.reference_copy_failed'), tone: 'warning' });
    }
  }
</script>

{#snippet copyControl()}
  <Button tone="secondary" size="sm" icon="copy" onclick={() => void copy(problem.requestId ?? '')}>{t('app.reference_copy')}</Button>
{/snippet}

<Banner tone="danger" title={problem.message} action={problem.requestId && hasClipboard ? copyControl : undefined}>
  {#if problem.reference}{t('app.error_reference', { request_id: problem.reference })}{/if}
</Banner>
