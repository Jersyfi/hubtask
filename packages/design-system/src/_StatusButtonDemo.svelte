<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { untrack } from 'svelte';

  import StatusButton from './StatusButton.svelte';
  import { FREE_STATUSES, IN_PROGRESS, STORY_STEPS } from './_workFixtures.ts';
  import { defaultSteps, type StatusRef, type WorkflowStep } from './steps.ts';

  interface Props {
    mode?: 'workflow' | 'free' | 'readonly' | 'blocked';
  }

  const { mode = 'workflow' }: Props = $props();

  let current = $state<StatusRef>(untrack(() => (mode === 'free' ? FREE_STATUSES[1]! : IN_PROGRESS)));
  let said = $state('');

  const onstep = (step: WorkflowStep) => {
    said = `Took "${step.label}"`;
    if (mode === 'free') current = step.to;
  };
  const allBlocked: WorkflowStep[] = STORY_STEPS.map((step) => ({ ...step, isNext: false, blockedReason: step.blockedReason ?? 'only the person in “Accepted by”' }));
</script>

<div class="stage">
  {#if mode === 'workflow'}
    <StatusButton
      {current}
      steps={STORY_STEPS}
      label={`Status: ${current.label}. Change status`}
      menuTitle={`From “${current.label}” on`}
      doneHint="Done is reached through In review → Accept; Clara Neumann accepts (field “Accepted by”)."
      workflow={{ label: 'View workflow', scope: '“Story” · applies in Team Atlas', onview: () => (said = 'Opened the workflow, read only') }}
      closeLabel="Close"
      grabLabel="Close"
      {onstep}
    />
  {:else if mode === 'free'}
    <StatusButton
      {current}
      steps={defaultSteps(current, FREE_STATUSES)}
      statuses={FREE_STATUSES}
      doneNote="completes the entry"
      label={`Status: ${current.label}. Change status`}
      menuTitle="Status · no workflow, every status reachable"
      workflow={{ label: 'Default workflow', scope: 'your own: Administration › Workflows', onview: () => (said = 'Opened workflows') }}
      closeLabel="Close"
      {onstep}
    />
  {:else if mode === 'blocked'}
    <StatusButton {current} steps={allBlocked} label={`Status: ${current.label}. Change status`} menuTitle={`From “${current.label}” on`} closeLabel="Close" {onstep} />
  {:else}
    <StatusButton {current} steps={STORY_STEPS} label="Status" menuTitle="" disabledReason="You can read this story; changing it needs the Edit right in Team Atlas." closeLabel="Close" />
  {/if}
  <p class="said" role="status">{said}</p>
</div>

<style>
  .stage { display: flex; flex-direction: column; align-items: flex-start; gap: var(--sp-200); min-block-size: var(--sp-1600); }
  .said { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
