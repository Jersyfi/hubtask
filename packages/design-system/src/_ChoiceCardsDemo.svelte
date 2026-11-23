<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ChoiceCards, { type ChoiceCard } from './ChoiceCards.svelte';

  const { mode = 'question' }: { mode?: 'question' | 'marked' | 'gated' } = $props();

  let choice = $state('');
  let start = $state('empty');

  const question: ChoiceCard[] = [
    { value: 'convert', label: 'Convert to columns', description: 'These places then run without a workflow: every entry stays in the column that showed its status, done stays done.' },
    { value: 'pause', label: 'Only pause', description: 'Everything stays as it is and appears unchanged when you switch on again.' },
  ];

  const starts = $derived<ChoiceCard[]>([
    { value: 'empty', label: 'Empty hub', description: 'Two columns, nothing else.', icon: 'plus' },
    { value: 'board', label: 'Team board', description: 'A board with four columns and a review step.', icon: 'columns-3' },
    {
      value: 'desk',
      label: 'Service desk',
      description: 'A portal, queues and promised response times.',
      icon: 'inbox',
      ...(mode === 'gated' ? { disabledReason: 'Only for hubs that others may see.' } : {}),
    },
  ]);
</script>

{#if mode === 'question'}
  <ChoiceCards label="What should happen to the 17 entries?" options={question} bind:value={choice} />
{:else}
  <ChoiceCards label="Start from" options={starts} bind:value={start} />
{/if}
