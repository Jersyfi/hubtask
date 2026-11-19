<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import PriorityMark from './PriorityMark.svelte';
</script>

<div class="row">
  <PriorityMark level={1} label="P1 Critical" />
  <PriorityMark level={2} label="P2 High" />
  <PriorityMark level={3} label="P3 Medium" />
  <PriorityMark level={4} label="P4 Low" />
</div>

<style>
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-300); }
</style>
