<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ResizeHandle from './ResizeHandle.svelte';

  interface Props {
    edge?: 'inline-start' | 'inline-end';
  }

  const { edge = 'inline-end' }: Props = $props();
  let width = $state(240);
</script>

<div class="stage" data-edge={edge}>
  <div class="region" style:--demo-width={`${width}px`}>
    <ResizeHandle label="Width of the navigation" value={width} min={200} max={400} reset={240} {edge} onresize={(next) => (width = next)} />
    <p>{width}</p>
  </div>
</div>

<style>
  .stage {
    display: flex;
    min-block-size: calc(6 * var(--sp-400));
    border: var(--bw-hairline) solid var(--border-subtle);
    background: var(--bg-canvas);
  }

  .stage[data-edge='inline-start'] { justify-content: flex-end; }

  .region {
    position: relative;
    inline-size: var(--demo-width);
    padding: var(--sp-200);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-variant-numeric: tabular-nums;
  }
</style>
