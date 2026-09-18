<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry of the hierarchy, at any of its three levels.
  //
  // §4 asks for four variants — `TASK`, `WORK_PACKAGE`, `ACTIVITY` and the collapsed state — and
  // the fourth is not a fourth type: it is whether the children are shown. So `type` says which
  // mark and which indent, and `expansion` says whether this row currently hides anything.
  //
  // It knows the three type names and nothing else about them. Which capabilities each carries is
  // the manifest's answer (`CapabilityGate`, F2-07), and a row that decided for itself that an
  // activity has no labels would be the hard-coded matrix `domain-model.md` §2's extension example
  // exists to rule out.
  //
  // Built on `ListRow`, so a row that navigates is a link and a row that selects is a button — and
  // the completion control sits in the leading slot, outside the activation, because ticking an
  // entry off is not a way to open it.

  import type { Snippet } from 'svelte';

  import Checkbox from './Checkbox.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import ListRow from './ListRow.svelte';
  import type { IconName } from './icons/index.ts';

  /** Whether this row hides children, shows them, or has none. */
  export type Expansion = 'collapsed' | 'expanded' | 'leaf';

  interface Props {
    /** The item type, as the manifest names it — a string, because the set grows without this client. */
    type: string;
    /** The entry's own words. Resolved content, not a message code: this one is the reader's text. */
    title: string;
    isCompleted?: boolean;
    /** How deep under the collection, for the indent. `domain-model.md` §3.4's `depth`. */
    depth?: number;
    expansion?: Expansion;
    href?: string;
    /** The name of the control that ticks it off. Required: it is a control, so it has a name. */
    completeLabel: string;
    /** Why completion is unavailable — archived, or a role that may not. There is no boolean. */
    completeDisabledReason?: string;
    /** The name of the control that shows or hides the children. */
    expandLabel?: string;
    onToggleComplete?: () => void;
    onToggleExpand?: () => void;
    /** Labels, a badge, a menu. Not part of the row's own activation. */
    trailing?: Snippet;
    /**
     * A change this device made and the server has not yet confirmed (F6-06): the row carries
     * the `pending` motion role beside a word, so it reads without motion too. Resolved text -
     * "Waiting to be sent" - and absent for an entry the server has.
     */
    pendingLabel?: string;
  }

  const {
    type,
    title,
    isCompleted = false,
    depth = 0,
    expansion = 'leaf',
    href,
    completeLabel,
    completeDisabledReason,
    expandLabel,
    onToggleComplete,
    onToggleExpand,
    trailing,
    pendingLabel,
  }: Props = $props();

  /**
   * The mark for a type, and the fallback for one this client has never heard of.
   *
   * A type the manifest reports and the icon set has no mark for still gets a row: "tolerant
   * behaviour towards unknown fields" is a binding client requirement (`roadmap.md` phase 5), and
   * refusing to draw an entry because its type is new would be the opposite of tolerant.
   */
  const MARKS: Record<string, IconName> = {
    TASK: 'task',
    WORK_PACKAGE: 'work-package',
    ACTIVITY: 'activity',
  };
  const mark = $derived(MARKS[type] ?? 'circle-check');
</script>

<div class="task-row" style:--depth={depth} data-type={type} data-completed={isCompleted ? '' : undefined} data-pending={pendingLabel !== undefined ? '' : undefined}>
  <ListRow {href} {trailing}>
    {#snippet leading()}
      <!-- The twist first, so the titles of a level line up whether or not a row has children. -->
      <span class="twist">
        {#if expansion !== 'leaf' && expandLabel}
          <IconButton
            icon={expansion === 'expanded' ? 'chevron-down' : 'chevron-right'}
            label={expandLabel}
            size="sm"
            onclick={() => onToggleExpand?.()}
          />
        {/if}
      </span>
      <!-- A checkbox, not an icon button, because that is what completion is: a two-state control
           a screen reader announces as checked or not. Its label is announced and not drawn — the
           title beside it is what the reader sees, and "Completed" printed against every row would
           be noise on screen and nothing in the tree. -->
      <Checkbox
        label={completeLabel}
        isLabelHidden
        checked={isCompleted}
        disabledReason={completeDisabledReason}
        onchange={() => onToggleComplete?.()}
      />
      <span class="mark" aria-hidden="true"><Icon name={mark} size="sm" /></span>
    {/snippet}

    <span class="title">{title}</span>
    {#if pendingLabel !== undefined}
      <!-- The change is on its way: the mark turns with the `pending` role, the word says why. -->
      <span class="pending"><span class="pending-mark" aria-hidden="true"><Icon name="cloud-upload" size="sm" /></span>{pendingLabel}</span>
    {/if}
  </ListRow>
</div>

<style>
  /* The indent mirrors itself: `padding-inline-start` and not `padding-left`, so a deep entry in
     Arabic is indented from the right. */
  .task-row { padding-inline-start: calc(var(--depth) * var(--sp-300)); }

  .twist,
  .mark { display: inline-flex; flex: none; width: var(--sp-400); justify-content: center; }

  .mark { color: var(--text-subtle); }

  .title { overflow-wrap: anywhere; }

  /* A change waiting to be sent: the `pending` role on the mark - a continuous indicator with
     no end - and the word beside it, because a pulse alone is a colour argument (rule 3) and is
     stilled under reduced motion (rule 6). Opacity only. */
  .pending {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    margin-inline-start: var(--sp-100);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .pending-mark {
    display: inline-flex;
    animation: breathe var(--motion-pending-duration) var(--motion-pending-easing) infinite alternate;
  }

  @keyframes breathe {
    from { opacity: 1; }
    to { opacity: 0.4; }
  }

  @media (prefers-reduced-motion: reduce) {
    .pending-mark { animation: none; }
  }

  :global([data-motion='reduced']) .pending-mark { animation: none; }

  /* Rule 3: a completed entry is not told apart by colour alone — the mark is filled and the words
     are struck, so it reads in greyscale. */
  .task-row[data-completed] .title {
    color: var(--text-subtle);
    text-decoration: line-through;
  }

  /* Tier 1 of §7, every completion: the type mark settles with the `celebration` role - a
     satisfying micro-animation, part of the normal motion system, in transform and opacity alone.
     Under reduced motion the struck title above is the acknowledgement (rule 6). */
  .task-row[data-completed] .mark {
    animation: settle var(--motion-celebration-duration) var(--motion-celebration-easing) both;
  }

  @keyframes settle {
    0% { opacity: 0.4; transform: scale(0.7); }
    60% { opacity: 1; transform: scale(1.15); }
    100% { opacity: 1; transform: scale(1); }
  }

  @media (prefers-reduced-motion: reduce) {
    .task-row[data-completed] .mark { animation: none; }
  }

  :global([data-motion='reduced']) .task-row[data-completed] .mark { animation: none; }
</style>
