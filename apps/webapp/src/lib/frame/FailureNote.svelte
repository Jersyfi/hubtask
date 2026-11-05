<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A refusal or a failure on a screen, in one shape everywhere (user-experience.md §5.8): the
  // reason as the sentence, and the reference with the code behind *Details for support*
  // (FailureDetails). A problem with no reference is the sentence alone. What else the banner
  // holds - a list of what stands in the way, a way out - comes as its children.
  import type { Snippet } from 'svelte';
  import { Banner } from '@hubtask/design-system/components';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { RenderedProblem } from '../problem.ts';
  import FailureDetails from './FailureDetails.svelte';

  type Problem = Pick<RenderedProblem, 'message' | 'reference' | 'requestId' | 'sentence' | 'code'>;
  type Props = {
    /** The rendered problem; a sentence alone where the screen kept only that, which folds nothing. */
    problem: Problem | string;
    /** More of the same refusal, drawn inside the banner above the details. */
    children?: Snippet;
  };
  const { problem: given, children }: Props = $props();
  const problem = $derived<Problem>(typeof given === 'string' ? { message: given } : given);

  const folded = $derived(problem.requestId !== undefined && problem.sentence !== undefined);
</script>

{#if folded && problem.requestId}
  <Banner tone="danger" title={problem.sentence ?? problem.message}>
    {@render children?.()}
    <FailureDetails requestId={problem.requestId} code={problem.code} />
  </Banner>
{:else}
  <Banner tone="danger" title={problem.message}>
    {@render children?.()}
    {#if problem.reference}{t('app.error_reference', { request_id: problem.reference })}{/if}
  </Banner>
{/if}
