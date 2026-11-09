<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import DangerZone from './DangerZone.svelte';
  import Dialog from './Dialog.svelte';
  import Button from './Button.svelte';

  let isOpen = $state(false);

  // No props: the story's arguments, if any, are ignored.
  const {}: Record<string, unknown> = $props();
</script>

<DangerZone
  title="Danger zone"
  acts={[
    { id: 'delete', title: 'Delete this group', cost: 'Its 2 people keep their own roles; the role it grants ends for them.', actionLabel: 'Delete group…' },
    { id: 'archive', title: 'Archive this group', cost: 'It stays in the archive for 30 days and grants nothing meanwhile.', actionLabel: 'Archive…', disabledReason: 'An administrator may archive a group.' },
  ]}
  onact={() => (isOpen = true)}
/>
<Dialog bind:isOpen size="s" title="Delete “Change Advisory Board”?" primary={{ label: 'Delete group', tone: 'danger', onclick: () => (isOpen = false) }}>
  Its 2 people keep their own roles. The role “CAB member” it grants ends for them. This cannot be undone.
  {#snippet actions()}<Button tone="subtle" onclick={() => (isOpen = false)}>Cancel</Button>{/snippet}
</Dialog>
