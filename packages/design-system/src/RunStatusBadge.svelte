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

<!-- Bold for the two states somebody has to act on (ADR-0061, F9-04): a failed run, and a rule
     that stopped itself. Everything else is the subtle form, so that a list of twenty runs is
     twenty quiet rows and the one that matters is the one that is seen. -->
<span
  class="run"
  data-tone={appearance.tone}
  data-emphasis={status === 'FAILED' || status === 'ABORTED_LOOP' ? 'bold' : 'subtle'}
  data-variant={isDryRun ? 'dry-run' : 'live'}
>
  <!-- Rule 3: the mark never stands alone, and it is chosen by the status rather than by the
       caller — two badges of the same status with different marks would be two vocabularies. -->
  <Icon name={appearance.icon} size="sm" />
  <span class="text">{label ?? status}</span>
  {#if isDryRun && dryRunLabel}
    <span class="variant">{dryRunLabel}</span>
  {/if}
</span>

<style>
  /* The tone's four roles, as `Badge` reads them (ADR-0061). */
  .run {
    --run-surface: var(--status-neutral-surface);
    --run-border: var(--status-neutral-border);
    --run-text: var(--status-neutral-text);
    --run-accent: var(--status-neutral-accent);
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-025) var(--sp-100);
    border: var(--bw-hairline) solid var(--run-border);
    border-radius: var(--r-full);
    background: var(--run-surface);
    color: var(--run-text);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    min-width: 0;
  }

  .run[data-tone='info'] { --run-surface: var(--status-info-surface); --run-border: var(--status-info-border); --run-text: var(--status-info-text); --run-accent: var(--status-info-accent); }
  .run[data-tone='success'] { --run-surface: var(--status-success-surface); --run-border: var(--status-success-border); --run-text: var(--status-success-text); --run-accent: var(--status-success-accent); }
  .run[data-tone='warning'] { --run-surface: var(--status-warning-surface); --run-border: var(--status-warning-border); --run-text: var(--status-warning-text); --run-accent: var(--status-warning-accent); }
  .run[data-tone='danger'] { --run-surface: var(--status-danger-surface); --run-border: var(--status-danger-border); --run-text: var(--status-danger-text); --run-accent: var(--status-danger-accent); }

  .run[data-emphasis='bold'] {
    background: var(--run-accent);
    border-color: var(--run-accent);
    color: var(--text-inverse);
    font-weight: var(--fw-semibold);
  }

  /* A rehearsal reads as one at a glance: the outline is drawn rather than solid, and the word is
     there as well, because a border style alone is a colour argument by another name. */
  .run[data-variant='dry-run'] { border-style: dashed; background: none; color: var(--run-text); border-color: var(--run-border); }

  .variant {
    padding-inline-start: var(--sp-050);
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-secondary);
    font-weight: var(--fw-regular);
  }

  .text { overflow-wrap: anywhere; }
</style>
