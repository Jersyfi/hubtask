<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import DeadlineBar from './DeadlineBar.svelte';
</script>

<div class="column">
  <DeadlineBar share={30} words="1 day 2 h" />
  <DeadlineBar share={64} words="1 h 40 min" tone="warning" />
  <DeadlineBar share={100} words="3 min left" tone="danger" />
  <DeadlineBar share={45} words="paused" isPaused />
  <DeadlineBar share={100} words="met" tone="success" />
  <div class="rail"><DeadlineBar share={80} words="40 min left" tone="warning" due="today 14:30" /></div>
</div>

<style>
  .column { display: flex; flex-direction: column; gap: var(--sp-200); max-inline-size: var(--layout-rail); }
  .rail { padding-block-start: var(--sp-100); }
</style>
