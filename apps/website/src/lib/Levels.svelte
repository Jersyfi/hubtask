<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The five levels, always in the same five label tokens. The product's own colour vocabulary
  // doing the explaining, and the one motif that repeats on every page that talks about the model.
  let { tone = 'full' }: { tone?: 'full' | 'quiet' } = $props();
</script>

<p class="levels" data-tone={tone} aria-label="The five levels: hub, collection, task, work package, activity">
  <span class="site-chip site-chip-slate">Hub</span>
  <span class="sep" aria-hidden="true">/</span>
  <span class="site-chip site-chip-blue">Collection</span>
  <span class="sep" aria-hidden="true">/</span>
  <span class="site-chip site-chip-teal">Task</span>
  <span class="sep" aria-hidden="true">/</span>
  <span class="site-chip site-chip-violet">Work&nbsp;package</span>
  <span class="sep" aria-hidden="true">/</span>
  <span class="site-chip site-chip-amber">Activity</span>
</p>
