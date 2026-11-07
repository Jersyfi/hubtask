<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import Grid from './Grid.svelte';
  import SummaryCard from './SummaryCard.svelte';
</script>

<Grid columns={{ compact: 1, medium: 2, expanded: 3 }}>
  <SummaryCard title="What is not working" state={{ label: '1', tone: 'warning' }} line="Mail is not sent: the installation has no mail server." link={{ label: 'Details', href: '#health' }} />
  <SummaryCard title="Limits" figure="1 / 1 000" line="automation runs this hour" link={{ label: 'All limits', href: '#limits' }} />
  <SummaryCard title="Rules" state={{ label: '2 need a look', tone: 'danger' }} link={{ label: 'All rules', href: '#rules' }}>
    <ul class="parts">
      <li><a href="#relay">Mail relay</a><Badge tone="danger">Failed 3 times</Badge></li>
      <li><a href="#triage">Triage by label</a><Badge tone="warning">Needs a look</Badge></li>
    </ul>
  </SummaryCard>
  <SummaryCard title="Backup" state={{ label: 'No target', tone: 'danger' }} line="nothing is backed up" link={{ label: 'Back up every night', href: '#backup' }} />
</Grid>

<style>
  .parts { display: grid; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .parts li { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: var(--sp-100); align-items: start; }
  .parts a { color: var(--text-brand); }
  .parts a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
