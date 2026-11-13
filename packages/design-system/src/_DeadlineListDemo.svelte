<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import DeadlineList from './DeadlineList.svelte';
  import { TICKET_DEADLINES } from './_workFixtures.ts';

  interface Props {
    keptLabel?: string;
  }

  const { keptLabel = 'First reply: 3 of 3 kept' }: Props = $props();
  let said = $state('');
</script>

<div class="column">
  <DeadlineList deadlines={TICKET_DEADLINES} {keptLabel} table={{ label: 'All deadlines as a table', onclick: () => (said = 'Would open the deadline matrix') }} />
  <p class="said" role="status">{said}</p>
</div>

<style>
  .column { max-inline-size: var(--layout-rail); }
  .said { color: var(--text-subtle); font-size: var(--fs-075); }
</style>
