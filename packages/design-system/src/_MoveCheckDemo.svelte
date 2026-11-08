<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import MoveCheck, { type MoveCheckItem } from './MoveCheck.svelte';

  const { mode = 'obstacle' }: { mode?: 'obstacle' | 'clear' } = $props();

  let isLifted = $state(untrack(() => mode === 'clear'));
  const items = $derived<MoveCheckItem[]>([
    { id: 'right', tone: 'ok', text: 'You may order in both hubs.' },
    { id: 'name', tone: 'ok', text: 'The name is free in IT operations.' },
    isLifted
      ? { id: 'types', tone: 'ok', text: 'The type “Story” is the workspace’s now.' }
      : {
          id: 'types',
          tone: 'obstacle',
          text: '18 entries use the type “Story” of Product development.',
          way: { label: 'Lift the type into the workspace', onselect: () => (isLifted = true) },
          rest: 'or choose a destination that has it.',
        },
    { id: 'access', tone: 'warning', text: '2 people will no longer see it: Mara Lind, Tim Berg. They are notified.' },
    { id: 'structure', tone: 'warning', text: 'Takes the fields, views and rules of IT operations.' },
  ]);
  const obstacles = $derived(items.filter((item) => item.tone === 'obstacle').length);
</script>

<div class="dialog">
  <h2 class="title">Move “Web app”</h2>
  <MoveCheck label="Before moving" {items} toneLabels={{ ok: 'Passed', obstacle: 'Obstacle', warning: 'Warning' }} />
  <div class="acts">
    <Button tone="secondary">Cancel</Button>
    <Button tone="primary" disabledReason={obstacles > 0 ? 'Resolve the obstacle first' : undefined}>Move</Button>
  </div>
</div>

<style>
  .dialog {
    max-inline-size: var(--layout-dialog-wide);
    padding: var(--sp-200) calc(var(--sp-200) + var(--sp-025));
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
  }

  .title { margin: 0 0 var(--sp-100); font-family: var(--font-display); font-size: var(--fs-200); }
  .acts { display: flex; justify-content: flex-end; gap: var(--sp-100); margin-block-start: var(--sp-150); }
</style>
