<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import OverviewPage from './OverviewPage.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import SummaryCard from './SummaryCard.svelte';
  import Refused from './_RefusedDemo.svelte';

  const { content = 'full' }: { content?: 'full' | 'empty' | 'refused' } = $props();
  const isEmpty = $derived(content === 'empty');
</script>

<OverviewPage head={{ title: 'Administration', subtitle: isEmpty ? 'Nordlicht GmbH · 1 person · no hub yet' : 'Nordlicht GmbH · 9 people · 4 hubs' }} refused={content === 'refused' ? refused : undefined}>
  {#snippet focal()}
    <h2 class="focal-title">Getting started <span class="focal-note">{isEmpty ? '0 of 5 done' : '2 of 5 done'}</span></h2>
    <ProgressBar label="Getting started" value={isEmpty ? 0 : 40} valueLabel={isEmpty ? '0 of 5' : '2 of 5'} />
    <p class="focal-next">Next: <a href="#mail">Connect a mailbox</a>, so mail to the desk becomes requests.</p>
  {/snippet}
  <SummaryCard title="What is not working" state={{ label: '1', tone: 'warning' }} line="Mail is not sent: the installation has no mail server." link={{ label: 'Details', href: '#health' }} />
  <SummaryCard title="Limits" figure={isEmpty ? '0 / 1 000' : '1 / 1 000'} line="automation runs this hour" link={{ label: 'All limits', href: '#limits' }} />
  <SummaryCard title="Rules" state={isEmpty ? { label: 'None yet' } : { label: 'All working', tone: 'success' }} link={{ label: 'All rules', href: '#rules' }} />
  <SummaryCard title="Backup" state={{ label: 'No target', tone: 'danger' }} line="nothing is backed up" link={{ label: 'Back up every night', href: '#backup' }} />
  <SummaryCard title="Privacy requests" line="None owed in the next 7 days" link={{ label: 'All requests', href: '#privacy' }} />
  <SummaryCard title="Waiting for your word" line="No proposal or update waiting" link={{ label: 'All changes', href: '#changes' }} />
</OverviewPage>

{#snippet refused()}<Refused what="the administration" />{/snippet}

<style>
  .focal-title { display: flex; align-items: baseline; gap: var(--sp-100); margin: 0; font-size: var(--fs-400); }
  .focal-note { margin-inline-start: auto; color: var(--text-subtle); font-size: var(--fs-100); font-weight: var(--fw-regular); }
  .focal-next { margin: 0; }
  .focal-next a { color: var(--text-brand); }
  .focal-next a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
