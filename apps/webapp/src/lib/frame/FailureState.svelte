<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A screen or a list that could not be read (voice-and-tone.md §4.4, not an empty state), in the
  // failure's one shape (user-experience.md §5.8): the reason as the sentence, the retry where the
  // caller offers one, and the reference with the code behind *Details for support*, as FailureNote
  // draws them for a refused act.
  import { ErrorState } from '@hubtask/design-system/components';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { RenderedProblem } from '../problem.ts';
  import FailureDetails from './FailureDetails.svelte';

  type Props = {
    problem: Pick<RenderedProblem, 'message' | 'reference' | 'requestId' | 'sentence' | 'code'>;
    retryLabel?: string;
    onRetry?: () => void;
  };
  const { problem, retryLabel = t('app.retry'), onRetry }: Props = $props();

  const folded = $derived(problem.requestId !== undefined && problem.sentence !== undefined);
</script>

{#if folded && problem.requestId}
  {@const requestId = problem.requestId}
  <ErrorState title={problem.sentence ?? problem.message} {retryLabel} {onRetry}>
    {#snippet details()}<FailureDetails {requestId} code={problem.code} />{/snippet}
  </ErrorState>
{:else}
  <ErrorState title={problem.message} reference={problem.reference} referenceLabel={t('app.reference')} {retryLabel} {onRetry} />
{/if}
