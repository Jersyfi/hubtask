<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import LiveBlock from './LiveBlock.svelte';

  interface Props {
    mode?: 'view' | 'board';
  }

  const { mode = 'view' }: Props = $props();
  let said = $state('');
</script>

<div class="page">
  {#if mode === 'view'}
    <LiveBlock title="Team Atlas – open, label customer-app" icon="square-check" liveLabel="Live" openLabel="Open in tasks" onopen={() => (said = 'Opened')}>
      <ul class="rows">
        <li>TAMA-1 · Sign in with a passkey</li>
        <li>TAMA-10 · Orders without a network</li>
      </ul>
    </LiveBlock>
  {:else}
    <LiveBlock title="Sign-in flow · frame “Flow”" icon="layout-template" product="whiteboards" liveLabel="Live" openLabel="Open the board" onopen={() => (said = 'Opened')}>
      <p class="board">The frame, drawn live.</p>
    </LiveBlock>
  {/if}
  <p aria-live="polite">{said}</p>
</div>

<style>
  .page { max-inline-size: var(--layout-reading); }
  .rows { margin: 0; padding: var(--sp-100) var(--sp-150); list-style: none; display: grid; gap: var(--sp-100); }
  .board { margin: 0; padding: var(--sp-300); color: var(--text-subtle); }
</style>
