<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import FloatingBar from './FloatingBar.svelte';
  import IconButton from './IconButton.svelte';
</script>

<div class="room">
  <FloatingBar role="group" aria-label="History">
    <IconButton size="sm" icon="undo-2" label="Undo" />
    <IconButton size="sm" icon="redo-2" label="Redo" />
  </FloatingBar>
</div>

<style>
  .room { display: flex; padding: var(--sp-300); background: var(--bg-surface-sunken); }
</style>
