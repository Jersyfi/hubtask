<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What a screen that broke while drawing becomes (the frame's error boundary): one sentence, a
  // client reference, Reload and Copy details - instead of an empty page. The frame around it
  // stays, so the person can go elsewhere. The details carry no content (crash.ts).
  import { Button, ErrorState, Inline, VisuallyHidden } from '@hubtask/design-system/components';
  import { t } from '../i18n/i18n.svelte.ts';
  import { toast } from './toasts.svelte.ts';
  import { currentEntry } from './chunkfailure.ts';
  import { crashDetails, newReference } from './crash.ts';

  type Props = {
    error: unknown;
    /** Draws the screen again; absent where the whole application failed. */
    reset?: () => void;
  };
  const { error, reset }: Props = $props();

  const reference = newReference();
  const at = new Date();

  async function copy(): Promise<void> {
    const details = crashDetails({
      reference, build: currentEntry(), path: location.pathname, userAgent: navigator.userAgent, at, error,
    });
    try {
      await navigator.clipboard.writeText(details);
      toast.show({ text: t('app.crash_copied'), tone: 'success' });
    } catch {
      toast.show({ text: t('app.crash_copy_failed'), tone: 'warning' });
    }
  }
</script>

<section class="failed" data-crash={reference}>
  <VisuallyHidden as="h1">{t('app.crashed_title')}</VisuallyHidden>
  <ErrorState
    title={t('app.crashed')}
    reference={reference}
    referenceLabel={t('app.crash_reference')}
    retryLabel={t('app.view_reload')}
    onRetry={() => location.reload()}
  />
  <Inline justify="center">
    <Button tone="secondary" onclick={copy}>{t('app.crash_copy')}</Button>
    {#if reset}<Button tone="subtle" onclick={reset}>{t('app.crash_retry')}</Button>{/if}
    <a href="/">{t('app.back_to_start')}</a>
  </Inline>
</section>

<style>
  .failed {
    display: flex;
    flex-direction: column;
    gap: var(--sp-200);
    padding-block: var(--sp-400);
  }
</style>
