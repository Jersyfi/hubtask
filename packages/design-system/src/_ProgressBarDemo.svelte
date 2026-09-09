<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper. `label` is required on the component — a bar with no accessible name
  // is a bar a screen reader announces as nothing — and a story's args are optional by
  // construction, so the demo supplies one and passes the rest through.

  import ProgressBar from './ProgressBar.svelte';
  import type { ControlSize } from './control.ts';

  const {
    label = 'Uploading the attachment',
    value,
    valueLabel,
    size = 'md',
  }: { label?: string; value?: number; valueLabel?: string; size?: ControlSize } = $props();
</script>

<ProgressBar {label} {value} {valueLabel} {size} />
