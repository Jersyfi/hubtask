<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Widget from './Widget.svelte';
  import WidgetStrip from './WidgetStrip.svelte';

  interface Props {
    mode?: 'today' | 'overview' | 'busy';
  }

  const { mode = 'today' }: Props = $props();
</script>

{#if mode === 'today'}
  <WidgetStrip label="Your day in figures">
    <Widget title="Overdue" tile={{ icon: 'triangle-alert', color: 'red' }} figure="1" figureTone={{ tone: 'danger', word: 'overdue' }} line="since yesterday" act={{ label: 'Show', onclick: () => {} }} />
    <Widget title="Due today" tile={{ icon: 'calendar', color: 'blue' }} figure="4" line="1 of them with a time" />
    <Widget title="Needs your reply" tile={{ icon: 'inbox', product: 'desk' }} figure="2" figureTone={{ tone: 'danger', word: '1 breached' }} line="first reply in 3 min" act={{ label: 'Open', onclick: () => {} }} />
    <Widget title="Time today" tile={{ icon: 'clock', product: 'time' }} figure="3:10" line="of 7:30 h · timer running" act={{ label: 'Stop', onclick: () => {} }} />
    <Widget title="Waits for your yes" tile={{ icon: 'user-check', product: 'goals' }} figure="2" line="1 change, 1 page" />
  </WidgetStrip>
{:else if mode === 'overview'}
  <div class="tiles">
    <Widget title="Your collections" tile={{ icon: 'folder', color: 'brand' }}>
      <ul class="rows"><li><span>Team Atlas – Mobile app</span><span class="quiet">6 open</span></li><li><span>Incidents &amp; requests</span><span class="quiet">10 open</span></li><li><span>Project plan new build</span><span class="quiet">21 %</span></li></ul>
    </Widget>
    <Widget title="New for you" tile={{ icon: 'bell', product: 'pages' }} act={{ label: 'Read', onclick: () => {} }}>
      <p class="text">New ticket system from 1 December</p><p class="quiet">Post by Mehmet · IT service desk · asks for a read receipt</p>
    </Widget>
    <Widget title="Plan" tile={{ icon: 'calendar', product: 'plans' }} figure="6 days" unit="to the sprint's end" href="#plan">
      <p class="text">Sprint 24 – Passkeys</p>
    </Widget>
    <Widget title="Needs you" count="6" tile={{ icon: 'inbox', product: 'desk' }}>
      <ul class="rows"><li><span>SA-7 · VPN drops after 10 minutes</span><span class="quiet">Reply</span></li><li><span>SA-4 · Access to the supplier portal</span><span class="quiet">Take</span></li></ul>
    </Widget>
  </div>
{:else}
  <div class="strip">
    <Widget title="Due today" tile={{ icon: 'calendar', color: 'blue' }} figure="4" isBusy busyLabel="Loading" />
    <Widget title="Your collections" tile={{ icon: 'folder', color: 'brand' }} isBusy busyLabel="Loading" />
  </div>
{/if}

<style>
  .strip { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr)); gap: var(--sp-150); }
  .tiles { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-rail)), 1fr)); gap: var(--sp-300); }
  .rows { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; font-size: var(--fs-090); }
  .rows li { display: flex; justify-content: space-between; gap: var(--sp-100); }
  .text { margin: 0; font-size: var(--fs-090); }
  .quiet { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
