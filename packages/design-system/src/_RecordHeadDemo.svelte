<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import RecordHead from './RecordHead.svelte';
</script>

<RecordHead
  kind="Group"
  title="Change Advisory Board"
  back={{ label: 'Groups', href: '#groups' }}
  facts="Approves normal and emergency changes · 2 members · 1 role · changed 2 days ago"
  edit={{ label: 'Edit', onclick: () => {} }}
  menu={{
    label: 'More for Change Advisory Board',
    items: [
      { id: 'copy', label: 'Copy as new group' },
      { id: 'file', label: 'View as file' },
      { id: 'delete', label: 'Delete group…', isDestructive: true, hasSeparatorBefore: true },
    ],
    onselect: () => {},
  }}
/>
