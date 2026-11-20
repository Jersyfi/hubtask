<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Card from './Card.svelte';
  import Grid from './Grid.svelte';
  import Widget from './Widget.svelte';
</script>

<Grid columns={{ compact: 1, medium: 2, expanded: 3 }}>
  <Widget title="Plans" tile={{ icon: 'calendar', product: 'plans' }} figure="4" unit="days to the sprint's end" href="#plans" />
  <Card element="article" isPressable padding="md">
    <a class="name" href="#collection">Website relaunch</a>
    <span class="line">12 open · Sprint 14</span>
  </Card>
  <Card element="section" padding="none" aria-label="What happened">
    <span class="head">What happened</span>
    <span class="row">Maria changed <a href="#page">Release notes</a></span>
    <span class="row">Jonas opened <a href="#entry">Login fails</a></span>
  </Card>
</Grid>

<style>
  .name { color: var(--text-primary); font-weight: var(--fw-semibold); text-decoration: none; }
  .name::after { content: ''; position: absolute; inset: 0; border-radius: inherit; }
  .name:focus-visible { outline: none; }
  .name:focus-visible::after { outline: var(--bw-ring) solid var(--focus-ring); }
  .line { color: var(--text-secondary); font-size: var(--fs-075); }
  .head { padding: var(--sp-150); border-block-end: var(--bw-hairline) solid var(--border-subtle); font-weight: var(--fw-semibold); }
  .row { padding: var(--sp-100) var(--sp-150); color: var(--text-secondary); font-size: var(--fs-075); }
  .row + .row { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
</style>
