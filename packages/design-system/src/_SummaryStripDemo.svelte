<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import SummaryCard from './SummaryCard.svelte';
  import SummaryStrip from './SummaryStrip.svelte';
</script>

<SummaryStrip label="Your desks in figures" phone={[1, 4]}>
  <SummaryCard lead="figure" title="Mine" figure="4" line="assigned to you" />
  <SummaryCard lead="figure" title="Unassigned" figure="7" line="waiting for someone" />
  <SummaryCard lead="figure" title="Breaching" figure="1" line="within the hour" />
  <SummaryCard lead="figure" title="Kept" figure="92 %" line="the last 30 days" />
</SummaryStrip>
