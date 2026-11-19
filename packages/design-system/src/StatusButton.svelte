<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The status as a lozenge button with the steps out of it (work-surface concept §5, §6.9, E-23).
  // Pressed, it opens "from *In progress* on": every allowed step with its target and what it
  // asks, the steps the reader may not take with their reason, how *Done* is reached, and *View
  // workflow*. Without a workflow the same surface lists every status, the current one ticked. On a
  // phone the surface is a sheet (§7). The check stays on the server; this draws its answer.
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import Sheet from './Sheet.svelte';
  import StateLozenge from './StateLozenge.svelte';
  import { focusReturn, rovingIndex } from './focus.ts';
  import { isCompactWidth } from './media.ts';
  import { openOverlay } from './overlay.ts';
  import { splitSteps, type StatusCategory, type StatusRef, type WorkflowStep } from './steps.ts';
  import type { LozengeLook } from './lozenge.ts';

  interface Props {
    current: StatusRef;
    /** The steps out of `current`, allowed and blocked, as the server answered. */
    steps: readonly WorkflowStep[];
    /** The button's name beyond its word: *Status: In progress. Change status*. */
    label: string;
    /** The surface's heading: *From "In progress" on*, or *Status · no workflow*. */
    menuTitle: string;
    /**
     * Without a workflow: every status, the current one ticked (§6.9). Given, the surface lists
     * these instead of steps, and `steps` only supplies the ids to take.
     */
    statuses?: readonly StatusRef[];
    /** What choosing a done status does without a workflow: *completes the entry*. */
    doneNote?: string;
    /** How the done category is reached, one line under the steps. */
    doneHint?: string;
    /** *View workflow* and where it applies; absent where there is no workflow to show. */
    workflow?: { label: string; scope?: string; onview: () => void };
    /**
     * The look of a state outside the status categories - a configuration item's health, a goal's
     * confidence (`lozengeLook`): its tone and glyph instead of the category's.
     */
    look?: LozengeLook;
    /** Present: the status is read only here, and this says why (P-05). */
    disabledReason?: string;
    /** The id of the element that already says the reason (a read-only notice): the status refers to it and says it no second time. */
    reasonSaidAt?: string;
    onstep?: (step: WorkflowStep) => void;
    closeLabel: string;
    grabLabel?: string;
  }

  const {
    current,
    steps,
    label,
    menuTitle,
    statuses,
    doneNote,
    doneHint,
    workflow,
    look,
    disabledReason,
    reasonSaidAt,
    onstep,
    closeLabel,
    grabLabel,
  }: Props = $props();

  const uid = $props.id();
  const surfaceId = `status-${uid}`;
  const reasonId = `status-${uid}-reason`;

  let isOpen = $state(false);
  let isSheetOpen = $state(false);
  let active = $state(0);
  let anchor = $state<HTMLElement | null>(null);
  let surface = $state<HTMLElement | null>(null);
  let list = $state<HTMLElement | null>(null);

  const TONE: Record<StatusCategory, 'neutral' | 'info' | 'success'> = { todo: 'neutral', progress: 'info', done: 'success' };
  const tone = $derived(look?.tone ?? TONE[current.category]);

  /** One row of the surface: a step (allowed or not), or - without a workflow - a status. */
  interface Row {
    readonly id: string;
    readonly name: string;
    readonly line?: string;
    readonly to: StatusRef;
    readonly blockedReason?: string;
    readonly isCurrent?: boolean;
    readonly step?: WorkflowStep;
  }

  const rows = $derived.by<Row[]>(() => {
    if (statuses) {
      return statuses.map((status) => {
        const step = steps.find((candidate) => candidate.to.id === status.id);
        return {
          id: status.id,
          name: status.label,
          line: status.category === 'done' ? doneNote : undefined,
          to: status,
          isCurrent: status.id === current.id,
          step,
          blockedReason: step?.blockedReason,
        };
      });
    }
    const { allowed, blocked } = splitSteps(steps);
    return [...allowed, ...blocked].map((step) => ({ id: step.id, name: step.label, line: step.asks, to: step.to, blockedReason: step.blockedReason, step }));
  });

  function open() {
    if (disabledReason !== undefined) return;
    if (isCompactWidth()) {
      isSheetOpen = true;
      return;
    }
    const firstUsable = rows.findIndex((row) => !row.isCurrent && row.blockedReason === undefined);
    active = Math.max(0, firstUsable);
    isOpen = true;
  }

  function close(returnFocus = true) {
    if (!isOpen) return;
    isOpen = false;
    if (returnFocus) focusReturn(anchor?.querySelector('button'));
  }

  function choose(row: Row) {
    if (row.blockedReason !== undefined || row.isCurrent || !row.step) return;
    close();
    isSheetOpen = false;
    onstep?.(row.step);
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Tab') return;
    const next = rovingIndex(event.key, active, rows.length);
    if (next === null) return;
    active = next;
    event.preventDefault();
  }

  $effect(() => {
    if (!isOpen || !list) return;
    list.querySelectorAll<HTMLElement>('[role^="menuitem"]')[active]?.focus();
  });

  $effect(() => {
    if (!isOpen || !anchor || !surface) return;
    return openOverlay({ layer: 'popover', trigger: anchor, surface, placement: { side: 'block-end', align: 'start' }, onDismiss: () => close() });
  });
</script>

<span class="anchor" bind:this={anchor}>
  {#if disabledReason !== undefined}
    <span class="status" data-tone={tone} data-readonly="" aria-describedby={reasonSaidAt ?? reasonId}>{#if look?.icon}<Icon name={look.icon} size="sm" />{/if}{current.label}</span>
    {#if !reasonSaidAt}<span class="reason" id={reasonId}>{disabledReason}</span>{/if}
  {:else}
    <button
      type="button"
      class="status"
      data-tone={tone}
      aria-label={label}
      aria-haspopup="menu"
      aria-expanded={isOpen || isSheetOpen}
      aria-controls={isOpen ? surfaceId : undefined}
      onclick={() => (isOpen ? close() : open())}
      onkeydown={(event) => {
        if (event.key === 'ArrowDown') {
          open();
          event.preventDefault();
        }
      }}
    >
      {#if look?.icon}<Icon name={look.icon} size="sm" />{/if}<span>{current.label}</span><Icon name="chevron-down" size="sm" />
    </button>
  {/if}
</span>

{#snippet row(item: Row, index: number, isMenu: boolean)}
  <button
    type="button"
    class="row"
    role={isMenu ? (statuses ? 'menuitemradio' : 'menuitem') : undefined}
    aria-checked={isMenu && statuses ? item.isCurrent === true : undefined}
    aria-current={!isMenu && item.isCurrent ? 'true' : undefined}
    tabindex={isMenu ? (index === active ? 0 : -1) : undefined}
    aria-disabled={item.blockedReason !== undefined ? 'true' : undefined}
    aria-describedby={item.blockedReason !== undefined || item.line ? `${surfaceId}-${item.id}-line` : undefined}
    data-blocked={item.blockedReason !== undefined ? '' : undefined}
    onclick={() => choose(item)}
  >
    <span class="mark">
      {#if statuses}
        {#if item.isCurrent}<Icon name="check" size="sm" />{/if}
      {:else}
        <Icon name={item.blockedReason !== undefined ? 'lock' : 'arrow-right'} size="sm" />
      {/if}
    </span>
    <span class="text">
      {#if statuses}
        <span class="target-inline"><StateLozenge label={item.to.label} tone={TONE[item.to.category]} /></span>
      {:else}
        <span class="name">{item.name}</span>
      {/if}
      {#if item.blockedReason !== undefined || item.line}
        <span class="line" id={`${surfaceId}-${item.id}-line`}>{item.blockedReason ?? item.line}</span>
      {/if}
    </span>
    {#if !statuses}<span class="target"><StateLozenge label={item.to.label} tone={item.blockedReason !== undefined ? 'neutral' : TONE[item.to.category]} /></span>{/if}
  </button>
{/snippet}

{#if isOpen}
  <div class="surface" id={surfaceId} bind:this={surface}>
    <p class="title" id={`${surfaceId}-title`}>{menuTitle}</p>
    <div class="list" role="menu" aria-labelledby={`${surfaceId}-title`} tabindex="-1" bind:this={list} onkeydown={onKeydown}>
      {#each rows as item, index (item.id)}
        {@render row(item, index, true)}
      {/each}
    </div>
    {#if doneHint}
      <p class="hint"><Icon name="info" size="sm" /><span>{doneHint}</span></p>
    {/if}
    {#if workflow}
      <div class="foot">
        <button type="button" class="workflow" onclick={() => { close(false); workflow.onview(); }} onkeydown={(event) => { if (event.key === 'Tab' && !event.shiftKey) close(false); }}>
          <Icon name="git-branch" size="sm" />{workflow.label}
        </button>
        {#if workflow.scope}<span class="scope">{workflow.scope}</span>{/if}
      </div>
    {/if}
  </div>
{/if}

<Sheet bind:isOpen={isSheetOpen} title={menuTitle} {closeLabel} {grabLabel}>
  <div class="sheet-rows">
    {#each rows as item, index (item.id)}
      {@render row(item, index, false)}
    {/each}
  </div>
  {#if doneHint}<p class="hint"><Icon name="info" size="sm" /><span>{doneHint}</span></p>{/if}
  {#if workflow}
    <div class="sheet-foot">
      <Button tone="secondary" icon="git-branch" isFull onclick={() => { isSheetOpen = false; workflow.onview(); }}>{workflow.label}</Button>
      {#if workflow.scope}<span class="scope">{workflow.scope}</span>{/if}
    </div>
  {/if}
</Sheet>

<style>
  .anchor { display: inline-flex; align-items: center; gap: var(--sp-100); }

  /* The lozenge as a button (mock `.statusbtn`): the category's tint, its border, its words. */
  .status {
    --sb-surface: var(--status-neutral-surface);
    --sb-border: var(--status-neutral-border);
    --sb-text: var(--status-neutral-text);
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-sm-min);
    padding: var(--sp-050) var(--sp-150);
    border: var(--bw-hairline) solid var(--sb-border);
    border-radius: var(--r-md);
    background: var(--sb-surface);
    color: var(--sb-text);
    font: inherit;
    font-size: var(--fs-090);
    font-weight: var(--fw-semibold);
    white-space: nowrap;
    cursor: pointer;
    transition: box-shadow var(--motion-state-duration) var(--motion-state-easing);
  }

  .status[data-tone='info'] {
    --sb-surface: var(--status-info-surface);
    --sb-border: var(--status-info-border);
    --sb-text: var(--status-info-text);
  }

  .status[data-tone='success'] {
    --sb-surface: var(--status-success-surface);
    --sb-border: var(--status-success-border);
    --sb-text: var(--status-success-text);
  }

  .status[data-tone='warning'] {
    --sb-surface: var(--status-warning-surface);
    --sb-border: var(--status-warning-border);
    --sb-text: var(--status-warning-text);
  }

  .status[data-tone='danger'] {
    --sb-surface: var(--status-danger-surface);
    --sb-border: var(--status-danger-border);
    --sb-text: var(--status-danger-text);
  }

  .status[data-tone='brand'] {
    --sb-surface: var(--accent-primary-subtle);
    --sb-border: var(--accent-primary);
    --sb-text: var(--text-brand);
  }

  .status:hover,
  .status[aria-expanded='true'] { box-shadow: 0 0 0 var(--sp-025) var(--accent-primary-subtle); }

  .status:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .status[data-readonly] { cursor: default; }
  .reason { color: var(--text-subtle); font-size: var(--fs-075); }

  .surface {
    position: fixed;
    z-index: var(--z-popover);
    display: flex;
    flex-direction: column;
    inline-size: min(var(--layout-dialog-s), calc(100vw - 2 * var(--sp-200)));
    max-block-size: 70vh;
    overflow: auto;
    margin: var(--sp-050);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
    font-weight: var(--fw-regular);
    animation: open var(--motion-attach-duration) var(--motion-attach-easing) both;
  }

  .title {
    margin: var(--sp-050) var(--sp-100) var(--sp-050);
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .list,
  .sheet-rows { display: flex; flex-direction: column; gap: var(--bw-hairline); }

  .row {
    display: grid;
    grid-template-columns: var(--sp-200) minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--sp-100);
    min-block-size: var(--sp-500);
    padding: var(--sp-050) var(--sp-100);
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-090);
    text-align: start;
    cursor: pointer;
  }

  .row:hover,
  .row:focus { background: var(--accent-primary-subtle); }

  .row:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .row[data-blocked] { color: var(--text-subtle); cursor: not-allowed; }
  .row[data-blocked]:hover { background: transparent; }
  .row[aria-checked='true'],
  .row[aria-current='true'] { cursor: default; }

  .mark { display: inline-grid; place-items: center; color: var(--text-secondary); }
  .text { display: flex; flex-direction: column; gap: var(--bw-hairline); min-inline-size: 0; }
  .name { font-weight: var(--fw-semibold); }
  .row[data-blocked] .name { font-weight: var(--fw-medium); }
  .line { color: var(--text-secondary); font-size: var(--fs-075); }
  .row[data-blocked] .line { color: var(--text-subtle); }
  .target-inline { display: inline-flex; }

  .hint {
    display: grid;
    grid-template-columns: var(--sp-200) minmax(0, 1fr);
    gap: var(--sp-100);
    margin: var(--sp-050) 0 0;
    padding: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .foot {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    margin-block-start: var(--sp-050);
    padding: var(--sp-100) var(--sp-100) var(--sp-050);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  .workflow {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-025) var(--sp-050);
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-brand);
    font: inherit;
    font-size: var(--fs-090);
    font-weight: var(--fw-semibold);
    cursor: pointer;
  }

  .workflow:hover { background: var(--bg-surface-hover); }

  .workflow:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .scope { margin-inline-start: auto; color: var(--text-subtle); font-size: var(--fs-075); }
  .sheet-foot { display: flex; flex-direction: column; gap: var(--sp-100); margin-block-start: var(--sp-150); }
  .sheet-foot .scope { margin-inline-start: 0; text-align: center; }

  /* On a phone a row is a thumb's target, as the workflow's phone mockup draws it: taller, its
     name at the reading size, a hairline between the rows instead of a gap. */
  .sheet-rows { gap: 0; }
  .sheet-rows .row { min-block-size: var(--sp-800); padding-block: var(--sp-150); border-radius: 0; font-size: var(--fs-100); }
  .sheet-rows > :global(* + *) { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .sheet-rows .name { font-size: var(--fs-200); }

  @keyframes open {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .surface { animation: none; }
    .status { transition: none; }
  }

  :global([data-motion='reduced']) .surface { animation: none; }
  :global([data-motion='reduced']) .status { transition: none; }
</style>
