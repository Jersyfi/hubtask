<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Inline from './Inline.svelte';
  import NavItem from './NavItem.svelte';
  import Stack from './Stack.svelte';
  import UnreadDot from './UnreadDot.svelte';
</script>

<Stack gap="200">
  <Inline gap="100" align="center"><UnreadDot label="Not read yet" /> New ticket system from 1 December</Inline>
  <div class="column">
    <NavItem label="New ticket system from 1 December" href="#post-1" figure="today" unreadLabel="Not read yet" />
    <NavItem label="Sprint 23 in review" href="#post-2" figure="6 Nov" />
    <NavItem label="Welcome, Priya!" href="#post-3" figure="2 Nov" />
  </div>
</Stack>

<style>
  .column { display: flex; flex-direction: column; max-inline-size: 40ch; }
</style>
