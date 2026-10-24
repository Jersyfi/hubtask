<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Which shape the same entries are shown in: a list, a board, and whatever the installation
  // reports next.
  //
  // **Not `Tabs`, and the difference is what the control means.** A tab strip switches between
  // *subjects* and owns the panel it reveals; this switches between *renderings of one subject*
  // and owns nothing — the entries are the same entries, and it is the caller that draws them. A
  // radio group is what that is: choose one of several, arrows move and choose in the same press,
  // which is the ARIA practice for a radio group and what a reader expects of a segmented control.
  //
  // A layout the manifest reports and the client has no surface for is **shown with the reason**
  // rather than left out. Leaving it out would make the switcher disagree with the installation,
  // and a reader who has heard of the timeline would be looking for something that is simply
  // absent; there is no `disabled` boolean in this package for the same reason.
  //
  // **The arrows reach it.** A radio group chooses as it moves, so an unavailable view is the one
  // place where focus and the choice part: the arrows land on it - focusable, `aria-disabled`, its
  // reason a bubble - and the chosen view stays chosen. Skipping it would leave a keyboard reader
  // no way to learn why it is off (design-system.md §4).

  import Reason from './_Reason.svelte';
  import Icon from './Icon.svelte';
  import { rovingIndex } from './focus.ts';
  import type { IconName } from './icons/index.ts';

  /** One layout on offer. The label is resolved text (ADR-0011), never a code. */
  export interface View {
    readonly id: string;
    readonly label: string;
    readonly icon?: IconName;
    /** Why this layout cannot be chosen here. Present means unavailable. */
    readonly unavailableReason?: string;
  }

  interface Props {
    /** What the choice is about. A group of controls with no name is a row of verbs. */
    label: string;
    views: readonly View[];
    selected?: string;
    onselect?: (id: string) => void;
  }

  let { label, views, selected = $bindable(views[0]?.id ?? ''), onselect }: Props = $props();

  let group = $state<HTMLElement | null>(null);
  const viewElements: (HTMLButtonElement | null)[] = $state([]);

  // Svelte's own id, not a random one: unique per instance, so two switchers on one page - the
  // list's and the timeline's - do not share the ids their reasons hang on.
  const instanceId = $props.id();
  const reasonId = (id: string) => `views-${instanceId}-reason-${id}`;

  const chosen = $derived(Math.max(0, views.findIndex((view) => view.id === selected)));
  /** Where focus is; the chosen view unless the arrows went onto an unavailable one. */
  let focused = $state<number | null>(null);
  const stop = $derived(focused ?? chosen);

  function choose(view: View) {
    if (view.unavailableReason !== undefined) return;
    selected = view.id;
    onselect?.(view.id);
  }

  function onKeydown(event: KeyboardEvent) {
    const dir = group !== null && getComputedStyle(group).direction === 'rtl' ? 'rtl' : 'ltr';
    const moved = rovingIndex(event.key, stop, views.length, { orientation: 'horizontal', dir });
    if (moved === null) return;
    event.preventDefault();
    const view = views[moved];
    if (!view) return;
    focused = moved;
    choose(view);
    viewElements[moved]?.focus();
  }

  /** Leaving the group forgets an unavailable view: `Tab` back in lands on the chosen one. */
  function onFocusout(event: FocusEvent) {
    if (group && event.relatedTarget instanceof Node && group.contains(event.relatedTarget)) return;
    focused = null;
  }
</script>

<!-- `tabindex="-1"` on the group: it holds the keydown handler, and the radios carry the roving 0.
     A group that was itself tabbable would put one press between the reader and the choice.
     `Menu` does the same for the same reason. -->
<div
  class="switcher"
  role="radiogroup"
  aria-label={label}
  tabindex="-1"
  bind:this={group}
  onkeydown={onKeydown}
  onfocusout={onFocusout}
>
  {#each views as view, position (view.id)}
    {@const isChosen = view.id === selected}
    {@const isOff = view.unavailableReason !== undefined}
    <button
      type="button"
      class="view"
      role="radio"
      data-view={view.id}
      aria-checked={isChosen}
      aria-disabled={isOff ? 'true' : undefined}
      aria-describedby={isOff ? reasonId(view.id) : undefined}
      tabindex={position === stop ? 0 : -1}
      bind:this={viewElements[position]}
      onclick={() => {
        focused = position;
        choose(view);
      }}
    >
      {#if view.icon}<Icon name={view.icon} size="sm" />{/if}
      <span>{view.label}</span>
    </button>
  {/each}
</div>
<!-- Outside the radio group, whose children are radios; out of the flow wherever they are. -->
{#each views as view, position (view.id)}
  {#if view.unavailableReason !== undefined}
    <Reason id={reasonId(view.id)} text={view.unavailableReason} control={viewElements[position] ?? null} />
  {/if}
{/each}

<style>
  .switcher {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-025);
    /* The concept's segmented control: a recessed track (rule 1) with a small inset, the chosen
       segment raised out of it. */
    padding: var(--sp-025);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
  }

  .view {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-050) var(--sp-150);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    cursor: pointer;
  }

  .view:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  /* Rule 3: the chosen view is not told apart by a tint. It carries the surface *and* the border,
     so the choice reads in greyscale and to a reader who does not perceive the accent. */
  .view[aria-checked='true'] {
    border-color: var(--border-subtle);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    color: var(--text-primary);
    font-weight: var(--fw-semibold);
  }

  /* Rule 5. With roving focus the ring is the only thing that says which view the arrows are on. */
  .view:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .view[aria-disabled='true'] { color: var(--text-subtle); cursor: not-allowed; }
  .view[aria-disabled='true']:hover { background: transparent; color: var(--text-subtle); }

</style>
