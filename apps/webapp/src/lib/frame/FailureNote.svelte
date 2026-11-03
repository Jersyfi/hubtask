<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A refusal or a failure on a screen, in one shape everywhere (user-experience.md §5.8): the
  // reason as the sentence, and the reference - what an administrator looks the request up by
  // (`GET /audit?request_id=`) - with the code behind *Details for support*, with a control that
  // copies it where the browser can copy. A problem with no reference is the sentence alone.
  import { Banner, Button, Disclosure, canCopy } from '@hubtask/design-system/components';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { RenderedProblem } from '../problem.ts';
  import { toast } from './toasts.svelte.ts';

  type Props = { problem: Pick<RenderedProblem, 'message' | 'reference' | 'requestId' | 'sentence' | 'code'> };
  const { problem }: Props = $props();

  const hasClipboard = canCopy(globalThis.navigator?.clipboard);
  const folded = $derived(problem.requestId !== undefined && problem.sentence !== undefined);

  async function copy(requestId: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(requestId);
      toast.show({ text: t('app.reference_copied'), tone: 'success' });
    } catch {
      toast.show({ text: t('app.reference_copy_failed'), tone: 'warning' });
    }
  }
</script>

{#if folded && problem.requestId}
  {@const requestId = problem.requestId}
  <Banner tone="danger" title={problem.sentence ?? problem.message}>
    <Disclosure label={t('app.failure.details')} tone="quiet">
      <p class="details">
        {t('app.error_reference', { request_id: requestId })}{#if problem.code}<span class="code"> · <code>{problem.code}</code></span>{/if}
      </p>
      {#if hasClipboard}
        <div><Button tone="secondary" size="sm" icon="copy" onclick={() => void copy(requestId)}>{t('app.reference_copy')}</Button></div>
      {/if}
    </Disclosure>
  </Banner>
{:else}
  <Banner tone="danger" title={problem.message}>
    {#if problem.reference}{t('app.error_reference', { request_id: problem.reference })}{/if}
  </Banner>
{/if}

<style>
  .details { margin: 0; font-size: var(--fs-075); user-select: all; }
  .code code { font-family: var(--font-mono); }
</style>
