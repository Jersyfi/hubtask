<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How an automation run ended.
  //
  // **Seven states, not four, and three of them are the ones people actually ask about.**
  // `SKIPPED` is a condition that did not match — nothing went wrong, the rule simply did not
  // apply. `THROTTLED` is the rule protecting the workspace from itself. `ABORTED_LOOP` is the
  // causation depth stopping a rule that triggered itself (`automation.md` §3). A badge that drew
  // those three as "failed" would make the runs list lie, and the lie would send somebody looking
  // for a defect that is the system working.
  //
  // **A dry run is a presentation of a run, not an eighth status.** The status is what happened;
  // the variant is whether anything was written. Folding it into the enum would mean seven more
  // states and a matrix nobody can read.
  //
  // **A status this build has never seen still renders.** Tolerance towards unknown values is a
  // binding client requirement: the badge shows the server's own token, neutral, rather than
  // guessing at a tone or showing nothing at all.

  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';

  /** The tokens `RuleRunStatus` declares. A string, because the set may grow without this client. */
  export type RunStatus =
    | 'RUNNING'
    | 'WAITING'
    | 'SUCCEEDED'
    | 'SKIPPED'
    | 'FAILED'
    | 'ABORTED_LOOP'
    | 'THROTTLED'
    | (string & {});

  interface Props {
    /** The status as the server names it. */
    status: RunStatus;
    /**
     * The status in words, resolved (ADR-0011). Absent falls back to the server's own token — for
     * a status this build has no wording for, showing the token beats showing nothing.
     */
    label?: string;
    /** A run that wrote nothing. The same status, marked as a rehearsal. */
    isDryRun?: boolean;
    /** What a dry run is called, resolved. Required with `isDryRun`. */
    dryRunLabel?: string;
  }

  const { status, label, isDryRun = false, dryRunLabel }: Props = $props();

  /**
   * The mark and the tone per status.
   *
   * `SKIPPED` is deliberately quiet rather than a warning: not applying is the ordinary outcome of
   * a rule with a condition. `THROTTLED` and `ABORTED_LOOP` are warnings rather than failures,
   * because in both cases the system did exactly what it was built to do — and in both cases
   * somebody probably wants to know.
   */
  const APPEARANCE = {
    RUNNING: { tone: 'info', icon: 'loader-circle' },
    WAITING: { tone: 'neutral', icon: 'clock' },
    SUCCEEDED: { tone: 'success', icon: 'circle-check' },
    SKIPPED: { tone: 'neutral', icon: 'minus' },
    FAILED: { tone: 'danger', icon: 'circle-alert' },
    ABORTED_LOOP: { tone: 'warning', icon: 'repeat' },
    THROTTLED: { tone: 'warning', icon: 'ban' },
  } as const satisfies Record<string, { tone: string; icon: IconName }>;

  const appearance = $derived(
    (APPEARANCE as Record<string, { tone: string; icon: IconName }>)[status] ?? {
      tone: 'neutral',
      icon: 'info' as IconName,
    },
  );
</script>

<span class="run" data-tone={appearance.tone} data-variant={isDryRun ? 'dry-run' : 'live'}>
  <!-- Rule 3: the mark never stands alone, and it is chosen by the status rather than by the
       caller — two badges of the same status with different marks would be two vocabularies. -->
  <Icon name={appearance.icon} size="sm" />
  <span class="text">{label ?? status}</span>
  {#if isDryRun && dryRunLabel}
    <span class="variant">{dryRunLabel}</span>
  {/if}
</span>

<style>
  .run {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-025) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    min-width: 0;
  }

  .run[data-tone='neutral'] { color: var(--text-secondary); }
  .run[data-tone='info'] { color: var(--text-brand); }
  .run[data-tone='success'] { color: var(--text-success); }
  .run[data-tone='warning'] { color: var(--text-warning); }
  .run[data-tone='danger'] { color: var(--text-danger); }

  /* A rehearsal reads as one at a glance: the outline is drawn rather than solid, and the word is
     there as well, because a border style alone is a colour argument by another name. */
  .run[data-variant='dry-run'] { border-style: dashed; background: none; }

  .variant {
    padding-inline-start: var(--sp-050);
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-secondary);
    font-weight: var(--fw-regular);
  }

  .text { overflow-wrap: anywhere; }
</style>
