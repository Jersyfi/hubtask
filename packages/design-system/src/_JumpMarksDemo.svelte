<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import JumpMarks from './JumpMarks.svelte';
  import FoldingSectionDemo from './_FoldingSectionDemo.svelte';

  interface Props {
    /** The demo's one layout; the axis of the workbench changes the width. */
    mode?: 'page';
  }

  const { mode = 'page' }: Props = $props();
  let current = $state('jump-description');
</script>

<div class="page" data-mode={mode}>
  <JumpMarks
    label="Sections of TAMA-18"
    bind:current
    marks={[
      { id: 'jump-description', label: 'Description' },
      { id: 'jump-parts', label: 'Parts', count: '2/8' },
      { id: 'jump-links', label: 'Links', count: '7' },
      { id: 'jump-files', label: 'Files', count: '9' },
      { id: 'jump-conversation', label: 'Conversation', count: '13' },
    ]}
  />
  <FoldingSectionDemo prefix="jump" />
</div>

<style>
  .page { max-inline-size: var(--layout-reading); }
</style>
