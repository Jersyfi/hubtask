<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where you are in the five levels, and how to get back out of them.
  //
  // design-system.md §4 asks for one thing this component could not have invented: from `medium`
  // down it collapses to `Hub / … / Parent / Current`. That is the hierarchy of domain-model.md
  // §3.4 rather than an arbitrary truncation - the first and the last two are the ones that answer
  // "which workspace" and "what am I looking at", and the levels between them are the ones a
  // reader can reconstruct.
  //
  // The collapsed middle stays **reachable**. A breadcrumb that hid a level with no way to get to
  // it has removed navigation rather than saved space, so the ellipsis is a real control that
  // expands in place - and it is a button rather than a menu because what it reveals is the same
  // list, not a different one.
  //
  // The trail is data rather than children for the reason a menu's items are: which entry is the
  // last one, and which are the middle, are questions about a list.

  import Icon from './Icon.svelte';
  import { collapseTrail, type Crumb } from './structure.ts';

  interface Props {
    /** What the trail is called, for a reader who arrives at it by keyboard. */
    label: string;
    trail: readonly Crumb[];
    /** The name of the control that reveals the collapsed levels. */
    expandLabel: string;
    onnavigate?: (id: string) => void;
  }

  const { label, trail, expandLabel, onnavigate }: Props = $props();

  let isExpanded = $state(false);

  // Which three, and how many are behind the ellipsis, is `collapseTrail` in structure.ts — a
  // question about a list, and therefore one that is answered where it can be tested.
  const collapsed = $derived(collapseTrail(trail, isExpanded));
  const shown = $derived(collapsed.shown);
  const hiddenCount = $derived(collapsed.hidden);
</script>

<nav class="breadcrumb" aria-label={label}>
  <ol class="trail">
    {#each shown as { crumb, index } (crumb.id)}
      <!-- The ellipsis sits before the level that follows the gap, so the order reads left to
           right in either direction: it is a list item like any other, not an overlay. -->
      {#if hiddenCount > 0 && index === trail.length - 2}
        <li class="crumb collapsed">
          <button
            type="button"
            class="ellipsis"
            aria-label={expandLabel}
            onclick={() => (isExpanded = true)}
          >
            <Icon name="ellipsis" size="sm" />
          </button>
          <span class="separator" aria-hidden="true"><Icon name="chevron-right" size="sm" /></span>
        </li>
      {/if}
      <li class="crumb">
        {#if crumb.href === undefined}
          <!-- The current level is not a link, and says so rather than only looking different. -->
          <span class="current" aria-current="page">{crumb.label}</span>
        {:else}
          <a
            class="link"
            href={crumb.href}
            onclick={(event) => {
              if (!onnavigate) return;
              event.preventDefault();
              onnavigate(crumb.id);
            }}>{crumb.label}</a
          >
          <span class="separator" aria-hidden="true"><Icon name="chevron-right" size="sm" /></span>
        {/if}
      </li>
    {/each}
  </ol>
</nav>

<style>
  .breadcrumb { min-width: 0; }

  .trail {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-050);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--fs-075);
  }

  .crumb {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-width: 0;
  }

  /* `start`/`end` and never left/right: the separator points the way the text runs, and the icon
     is mirrored by the document direction rather than by a second glyph. */
  .separator {
    display: inline-flex;
    flex: none;
    color: var(--text-subtle);
  }

  :global([dir='rtl']) .separator { transform: scaleX(-1); }

  .link,
  .current {
    display: block;
    max-width: 24ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .link {
    color: var(--text-secondary);
    text-decoration: none;
    border-radius: var(--r-sm);
  }

  .link:hover { color: var(--text-primary); text-decoration: underline; }

  .current { color: var(--text-primary); font-weight: var(--fw-medium); }

  .ellipsis {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: var(--density-control-sm-min);
    min-height: var(--density-control-sm-min);
    padding: 0;
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .ellipsis:hover { color: var(--text-primary); }

  .link:focus-visible,
  .ellipsis:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }
</style>
