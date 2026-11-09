<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A step of the phone's Browse stack (the navigation concept §9): the sidebar's levels, drawn as
  // touch rows by the sidebar itself from the same reads. From `medium` up the address opens the
  // place the sidebar draws instead (`App.svelte`, `browseElsewhere`).
  import WorkspaceNav from '../lib/frame/WorkspaceNav.svelte';
  import type { PhoneStep } from '../lib/phone.ts';
  import type { Resolution } from '../lib/router.ts';

  interface Props {
    route: Resolution;
    step: PhoneStep;
    onnavigate: (path: string) => void;
  }

  const { route, step, onnavigate }: Props = $props();
</script>

<!-- The stack's step is the frame's own navigation drawn as a page, a kind of its own
     (page-system §7.10) whose box is the sidebar's. -->
<div class="browse" data-page-kind="browse"><WorkspaceNav {route} {step} {onnavigate} /></div>

<style>
  .browse { display: contents; }
</style>
