<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script module lang="ts">
  export interface Deadline {
    readonly id: string;
    /** What must happen: *Resolution*, *Next reply*. */
    readonly name: string;
    /** Whose it is and of which kind: *Team 1st level · OLA*. */
    readonly context?: string;
    /** What is left or what happened: *breached 40 min*, *1 h 05 min*. */
    readonly words: string;
    /** How much of the time is used, 0 to 100. */
    readonly share: number;
    readonly tone?: 'neutral' | 'warning' | 'danger' | 'success';
    readonly isPaused?: boolean;
    /** Under the bar: *was due 12 Nov, 12:00*, *until 10:30 · new after every reply*. */
    readonly due?: string;
  }
</script>

<script lang="ts">
  // The deadlines of a ticket in its rail (ET-6, second round): one row per deadline, the most
  // urgent first - breached, then at risk, then by the share of time used -, each with its name,
  // whose it is, what is left in its tone and with a glyph, a bar across the row and the due time
  // under it. A line says what was kept, and one act opens all deadlines as a table, where the
  // matrix of every kind and group stands.
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';

  interface Props {
    deadlines: readonly Deadline[];
    /** *First reply: 3 of 3 kept*. */
    keptLabel?: string;
    /** *All deadlines as a table*. */
    table?: { label: string; onclick: () => void };
  }

  const { deadlines, keptLabel, table }: Props = $props();

  const RANK = { danger: 0, warning: 1, neutral: 2, success: 3 } as const;
  const ordered = $derived(
    [...deadlines].sort((a, b) => RANK[a.tone ?? 'neutral'] - RANK[b.tone ?? 'neutral'] || b.share - a.share),
  );
  const step = (share: number) => Math.min(100, Math.max(0, Math.round(share / 10) * 10));
</script>

<div class="deadlines">
  <ul class="list">
    {#each ordered as deadline (deadline.id)}
      {@const glyph = deadline.isPaused ? 'pause' : deadline.tone === 'danger' || deadline.tone === 'warning' ? 'triangle-alert' : undefined}
      <li class="deadline" data-tone={deadline.tone ?? 'neutral'} data-paused={deadline.isPaused ? '' : undefined}>
        <span class="head">
          <span class="name"><b>{deadline.name}</b>{#if deadline.context}<span class="context">· {deadline.context}</span>{/if}</span>
          <span class="words">{#if glyph}<Icon name={glyph} size="sm" />{/if}{deadline.words}</span>
        </span>
        <span class="track"><span class="fill" data-share={step(deadline.share)}></span></span>
        {#if deadline.due}<span class="due">{deadline.due}</span>{/if}
      </li>
    {/each}
  </ul>
  {#if keptLabel}<p class="kept"><Icon name="check" size="sm" />{keptLabel}</p>{/if}
  {#if table}<span class="table"><Button tone="subtle" size="sm" icon="table" onclick={table.onclick}>{table.label}</Button></span>{/if}
</div>

<style>
  .deadlines { display: flex; flex-direction: column; gap: var(--sp-100); font-size: var(--fs-090); }
  .list { margin: 0; padding: 0; list-style: none; }
  .deadline { display: flex; flex-direction: column; gap: var(--sp-050); padding-block: var(--sp-100); }
  .deadline + .deadline { border-block-start: var(--bw-hairline) solid var(--border-subtle); }

  .head { display: flex; align-items: baseline; gap: var(--sp-100); }
  .name { flex: 1; min-inline-size: 0; color: var(--text-primary); }
  .name b { font-weight: var(--fw-semibold); }
  .context { margin-inline-start: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); }
  .words { display: inline-flex; align-items: center; gap: var(--sp-025); flex: none; font-weight: var(--fw-semibold); white-space: nowrap; }
  .deadline[data-tone='danger'] .words { color: var(--status-danger-text); }
  .deadline[data-tone='warning'] .words { color: var(--status-warning-text); }

  .track { display: block; block-size: var(--sp-050); border-radius: var(--r-full); background: var(--border-subtle); overflow: hidden; }
  .fill { display: block; block-size: 100%; inline-size: 0; border-radius: inherit; background: var(--accent-primary); }
  .deadline[data-tone='warning'] .fill { background: var(--status-warning-accent); }
  .deadline[data-tone='danger'] .fill { background: var(--status-danger-accent); }
  .deadline[data-tone='success'] .fill { background: var(--status-success-accent); }
  .deadline[data-paused] .fill { background: var(--text-subtle); }
  .fill[data-share='10'] { inline-size: 10%; } .fill[data-share='20'] { inline-size: 20%; } .fill[data-share='30'] { inline-size: 30%; }
  .fill[data-share='40'] { inline-size: 40%; } .fill[data-share='50'] { inline-size: 50%; } .fill[data-share='60'] { inline-size: 60%; }
  .fill[data-share='70'] { inline-size: 70%; } .fill[data-share='80'] { inline-size: 80%; } .fill[data-share='90'] { inline-size: 90%; }
  .fill[data-share='100'] { inline-size: 100%; }

  .due { color: var(--text-subtle); font-size: var(--fs-075); }
  .kept { display: flex; align-items: center; gap: var(--sp-050); margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .table { display: flex; justify-content: center; }
  .table :global(.button) { color: var(--text-brand); font-weight: var(--fw-semibold); }
</style>
