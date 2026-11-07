<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Grid from './Grid.svelte';
  import SummaryCard from './SummaryCard.svelte';
</script>

<Grid columns={{ compact: 1, medium: 2, expanded: 3 }}>
  <SummaryCard title="What is not working" state={{ label: '1', tone: 'warning' }} line="Mail is not sent: the installation has no mail server." link={{ label: 'Details', href: '#health' }} />
  <SummaryCard title="Limits" figure="1 / 1 000" line="automation runs this hour" link={{ label: 'All limits', href: '#limits' }} />
  <SummaryCard title="Backup" state={{ label: 'No target', tone: 'danger' }} line="nothing is backed up" link={{ label: 'Back up every night', href: '#backup' }} />
</Grid>
