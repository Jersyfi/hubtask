<!-- SPDX-License-Identifier: BUSL-1.1
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

  const chosen = $derived(Math.max(0, views.findIndex((view) => view.id === selected)));

  function choose(view: View) {
    if (view.unavailableReason !== undefined) return;
    selected = view.id;
    onselect?.(view.id);
  }

  function onKeydown(event: KeyboardEvent) {
    const dir = group !== null && getComputedStyle(group).direction === 'rtl' ? 'rtl' : 'ltr';
    const moved = rovingIndex(event.key, chosen, views.length, { orientation: 'horizontal', dir });
    if (moved === null) return;
    event.preventDefault();

    // The arrows skip what cannot be chosen rather than landing on it: a radio group moves the
    // selection as it moves focus, so stopping on an unavailable view would mean choosing it.
    for (let step = 0; step < views.length; step += 1) {
      const at = (moved + step * (moved >= chosen ? 1 : -1) + views.length) % views.length;
      const view = views[at];
      if (view && view.unavailableReason === undefined) {
        choose(view);
        group?.querySelector<HTMLElement>(`[data-view="${view.id}"]`)?.focus();
        return;
      }
    }
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
>
  {#each views as view (view.id)}
    {@const isChosen = view.id === selected}
    {@const isOff = view.unavailableReason !== undefined}
    <button
      type="button"
      class="view"
      role="radio"
      data-view={view.id}
      aria-checked={isChosen}
      aria-disabled={isOff ? 'true' : undefined}
      aria-describedby={isOff ? `reason-${view.id}` : undefined}
      tabindex={isChosen ? 0 : -1}
      onclick={() => choose(view)}
    >
      {#if view.icon}<Icon name={view.icon} size="sm" />{/if}
      <span>{view.label}</span>
    </button>
    {#if isOff}
      <!-- The reason is announced and drawn: a control that cannot be used still says what it
           would have done, and why it cannot. -->
      <span id={`reason-${view.id}`} class="reason">{view.unavailableReason}</span>
    {/if}
  {/each}
</div>

<style>
  .switcher {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-050);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    /* Rule 1: recessed, because it is a child element of the toolbar it sits in. */
    background: var(--bg-surface-sunken);
  }

  .view {
    display: inline-flex;
    align-items: center;
    gap: var(--density-row-gap);
    padding: var(--density-row-block) var(--sp-150);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-100);
    cursor: pointer;
  }

  .view:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  /* Rule 3: the chosen view is not told apart by a tint. It carries the surface *and* the border,
     so the choice reads in greyscale and to a reader who does not perceive the accent. */
  .view[aria-checked='true'] {
    border-color: var(--border-default);
    background: var(--bg-surface);
    color: var(--text-primary);
    font-weight: var(--fw-medium);
  }

  /* Rule 5. With roving focus the ring is the only thing that says which view the arrows are on. */
  .view:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .view[aria-disabled='true'] { color: var(--text-subtle); cursor: not-allowed; }
  .view[aria-disabled='true']:hover { background: transparent; color: var(--text-subtle); }

  .reason { color: var(--text-subtle); font-size: var(--fs-075); max-width: 40ch; }
</style>
