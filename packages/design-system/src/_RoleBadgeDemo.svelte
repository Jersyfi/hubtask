<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper: `label` is required on the component, and a story's args are optional
  // by construction.

  import Inline from './Inline.svelte';
  import RoleBadge from './RoleBadge.svelte';
  import type { ControlSize } from './control.ts';

  const {
    label = 'Admin',
    scopeLabel,
    size = 'md',
    mode = 'one',
  }: {
    label?: string;
    scopeLabel?: string;
    size?: ControlSize;
    mode?: 'one' | 'ladder';
  } = $props();
</script>

{#if mode === 'ladder'}
  <Inline gap="100">
    <RoleBadge label="Owner" scopeLabel="this workspace" />
    <RoleBadge label="Admin" scopeLabel="this workspace" />
    <RoleBadge label="Member" scopeLabel="in Marketing" />
    <RoleBadge label="Contributor" scopeLabel="in Campaign 2026" />
    <RoleBadge label="Viewer" scopeLabel="in Campaign 2026" />
    <RoleBadge label="Guest" scopeLabel="on this entry" />
    <RoleBadge label="Auditor" scopeLabel="this workspace" />
  </Inline>
{:else}
  <RoleBadge {label} {scopeLabel} {size} />
{/if}
