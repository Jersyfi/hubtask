<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A deadline as a bar with its words (WB-4, ET-6): how much of the time is used, what is left,
  // and - in the rail - when it is due. The tone comes with a glyph where it warns, and a paused
  // clock says so with its own mark, so neither rests on colour alone (UX-7).
  import Icon from './Icon.svelte';

  interface Props {
    /** How much of the time is used, 0 to 100. */
    share: number;
    /** What is left or what happened: *3 min left*, *met*, *breached 12 min*. */
    words: string;
    tone?: 'neutral' | 'warning' | 'danger' | 'success';
    /** The clock stands still - waiting for the requester. */
    isPaused?: boolean;
    /** When it is due, under the bar: *today 14:30*. */
    due?: string;
  }

  const { share, words, tone = 'neutral', isPaused = false, due }: Props = $props();
  const step = $derived(Math.min(100, Math.max(0, Math.round(share / 10) * 10)));
  const glyph = $derived(isPaused ? 'pause' : tone === 'danger' || tone === 'warning' ? 'triangle-alert' : undefined);
</script>

<span class="deadline" data-tone={tone} data-paused={isPaused ? '' : undefined} data-due={due ? '' : undefined}>
  <span class="track"><span class="fill" data-share={step}></span></span>
  <span class="words">{#if glyph}<Icon name={glyph} size="sm" />{/if}{words}{#if due}<span class="due"> · {due}</span>{/if}</span>
</span>

<style>
  .deadline { display: inline-flex; align-items: center; gap: var(--sp-100); min-inline-size: var(--sp-1600); max-inline-size: 100%; }
  .deadline[data-due] { display: flex; flex-direction: column; align-items: stretch; gap: var(--sp-050); }

  /* The track is a hairline's colour, not the sunken surface: on the dark surface that one is
     nearly black and the bar's empty part disappears. */
  .track { flex: 1; min-inline-size: var(--sp-500); block-size: var(--sp-050); border-radius: var(--r-full); background: var(--border-subtle); overflow: hidden; }
  .fill { display: block; block-size: 100%; inline-size: 0; border-radius: inherit; background: var(--accent-primary); }
  .deadline[data-tone='warning'] .fill { background: var(--status-warning-accent); }
  .deadline[data-tone='danger'] .fill { background: var(--status-danger-accent); }
  .deadline[data-tone='success'] .fill { background: var(--status-success-accent); }
  .deadline[data-paused] .fill { background: var(--text-subtle); }

  .words { display: inline-flex; align-items: center; gap: var(--sp-025); color: var(--text-secondary); font-size: var(--fs-075); white-space: nowrap; }
  .deadline[data-tone='danger'] .words { color: var(--status-danger-text); font-weight: var(--fw-semibold); }
  .deadline[data-tone='warning'] .words { color: var(--status-warning-text); }
  .due { color: var(--text-subtle); font-weight: var(--fw-regular); }

  .fill[data-share='10'] { inline-size: 10%; } .fill[data-share='20'] { inline-size: 20%; } .fill[data-share='30'] { inline-size: 30%; }
  .fill[data-share='40'] { inline-size: 40%; } .fill[data-share='50'] { inline-size: 50%; } .fill[data-share='60'] { inline-size: 60%; }
  .fill[data-share='70'] { inline-size: 70%; } .fill[data-share='80'] { inline-size: 80%; } .fill[data-share='90'] { inline-size: 90%; }
  .fill[data-share='100'] { inline-size: 100%; }
</style>
