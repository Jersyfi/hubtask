<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One subject, several views of it - and one stop in the tab order, not one per tab.
  //
  // That is the whole keyboard design and it comes from the ARIA authoring practices rather than
  // from taste: the arrows move between tabs, `Tab` leaves the strip and lands in the panel. A
  // strip where every tab was tabbable would make a reader press `Tab` six times to get past a
  // component whose purpose is to be a single choice. `rovingIndex` in focus.ts is the arithmetic,
  // shared with `Menu`, so the two cannot come to disagree about what `Home` does.
  //
  // Selection follows focus, which the practices allow when showing a panel is cheap. It is here:
  // the panels are already rendered by the caller and switching is a class change. If a panel ever
  // costs a request, this is the decision to revisit - and it is one line.

  import type { Snippet } from 'svelte';

  import { rovingIndex } from './focus.ts';

  /** One tab. `id` is what the caller switches on, never display text. */
  export interface Tab {
    readonly id: string;
    /** Resolved text (ADR-0011). */
    readonly label: string;
    /** Why this view cannot be opened. Present means unavailable - there is no `disabled` boolean. */
    readonly disabledReason?: string;
  }

  interface Props {
    /** What the set of views is called. */
    label: string;
    tabs: readonly Tab[];
    /** The selected tab's id. Bindable, because the caller usually owns the route. */
    selected?: string;
    onselect?: (id: string) => void;
    /** The panel for the selected tab. The caller renders one; this owns the association. */
    children?: Snippet;
  }

  let { label, tabs, selected = $bindable(tabs[0]?.id ?? ''), onselect, children }: Props = $props();

  const group = `tabs-${Math.random().toString(36).slice(2, 9)}`;

  // A `title` is not an accessible description: it is a tooltip a pointer finds and a keyboard
  // reader does not. Where a view cannot be opened the reason is rendered and pointed at, exactly
  // as `Button` does it — the reason and the state cannot come apart (design-system.md §4).
  const reasonId = (id: string) => `${group}-reason-${id}`;
  let strip = $state<HTMLElement | null>(null);

  const index = $derived(Math.max(0, tabs.findIndex((tab) => tab.id === selected)));

  function choose(next: number) {
    const tab = tabs[next];
    if (!tab || tab.disabledReason !== undefined) return;
    selected = tab.id;
    onselect?.(tab.id);
    // Focus follows the choice so that the arrows keep working from where the reader now is.
    strip?.querySelector<HTMLElement>(`[data-index="${next}"]`)?.focus();
  }

  function onKeydown(event: KeyboardEvent) {
    // Horizontal, and therefore mirrored in RTL by `rovingIndex` rather than here: which key means
    // "the next one" is a direction question, and it is answered in one place.
    const next = rovingIndex(event.key, index, tabs.length, {
      orientation: 'horizontal',
      dir: strip && getComputedStyle(strip).direction === 'rtl' ? 'rtl' : 'ltr',
    });
    if (next === null) return;
    event.preventDefault();
    choose(next);
  }
</script>

<div class="tabs">
  <!-- `tabindex="-1"` and not 0: the strip is one stop in the tab order, and the stop is the
       selected *tab*, not the container around it. The attribute is here because the role is an
       interactive one and a focusable ancestor is what lets a programmatic `focus()` land. -->
  <div
    class="strip"
    role="tablist"
    aria-label={label}
    tabindex="-1"
    bind:this={strip}
    onkeydown={onKeydown}
  >
    {#each tabs as tab, position (tab.id)}
      <button
        type="button"
        role="tab"
        class="tab"
        data-index={position}
        id={`${group}-tab-${tab.id}`}
        aria-controls={`${group}-panel`}
        aria-selected={tab.id === selected}
        aria-disabled={tab.disabledReason === undefined ? undefined : true}
        aria-describedby={tab.disabledReason === undefined ? undefined : reasonId(tab.id)}
        tabindex={position === index ? 0 : -1}
        onclick={() => choose(position)}
      >
        {tab.label}
      </button>
    {/each}
  </div>

  {#each tabs.filter((tab) => tab.disabledReason !== undefined) as tab (tab.id)}
    <span id={reasonId(tab.id)} class="reason">{tab.disabledReason}</span>
  {/each}

  {#if children}
    <div
      class="panel"
      role="tabpanel"
      id={`${group}-panel`}
      aria-labelledby={`${group}-tab-${selected}`}
      tabindex="0"
    >
      {@render children()}
    </div>
  {/if}
</div>

<style>
  .tabs { display: flex; flex-direction: column; gap: var(--sp-200); min-width: 0; }

  .strip {
    display: flex;
    gap: var(--sp-050);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    overflow-x: auto;
  }

  .tab {
    flex: none;
    padding: var(--density-control-md-block) var(--sp-200);
    min-height: var(--density-control-md-min);
    border: 0;
    /* The indicator is a border on the tab itself rather than a sliding element: rule 6 confines
       motion to opacity and transform, and a sliding underline animates neither. */
    border-block-end: var(--bw-thick) solid transparent;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-100);
    cursor: pointer;
    transition:
      color var(--motion-state-duration) var(--motion-state-easing),
      border-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .tab:hover { color: var(--text-primary); }

  /* Rule 3: the selected tab is not only a colour. It carries the indicator and the weight, so it
     reads as selected in greyscale and to a reader who does not perceive the accent. */
  .tab[aria-selected='true'] {
    color: var(--text-primary);
    border-block-end-color: var(--accent-primary);
    font-weight: var(--fw-medium);
  }

  .tab[aria-disabled='true'] { color: var(--text-subtle); cursor: not-allowed; }

  /* Rule 5's ring, at rule 5's offset. The strip clips nothing — it scrolls rather than hiding
     overflow — so an outset ring is visible on the first and last tab as well. */
  .tab:focus-visible,
  .panel:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .reason {
    display: block;
    max-width: 40ch;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .panel { min-width: 0; }

  @media (prefers-reduced-motion: reduce) {
    .tab { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .tab { transition-duration: var(--dur-instant); }
</style>
