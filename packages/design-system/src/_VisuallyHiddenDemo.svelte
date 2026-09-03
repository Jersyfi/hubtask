<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The frame around VisuallyHidden, whose whole job is to be invisible.
  //
  // There is nothing to look at, which is the point - so the demo shows what a reader cannot see
  // in two ways instead: the workbench's tab walk reaches the skip link, and the icon-only button
  // has an accessible name that no pixel carries.

  import VisuallyHidden from './VisuallyHidden.svelte';

  interface Props {
    /** `name` is the icon-only control, `skip` is the link that appears when it takes focus. */
    mode?: 'name' | 'skip';
  }

  const { mode = 'name' }: Props = $props();
</script>

{#if mode === 'skip'}
  <p class="note">
    Tab into this pane. The link is not painted until it has focus, and it is announced either way.
  </p>
  <VisuallyHidden as="div" isFocusable>
    <a class="skip" href="#main">Skip to content</a>
  </VisuallyHidden>
  <p id="main" class="note">The content the link skips to.</p>
{:else}
  <p class="note">
    The button below has no visible text. Its name comes from the hidden span, which is why an icon
    that means "archive" is announced as "Archive" rather than as nothing at all.
  </p>
  <button type="button" class="icon-button">
    <span aria-hidden="true">⌫</span>
    <VisuallyHidden>Archive this task</VisuallyHidden>
  </button>
{/if}

<style>
  .note {
    max-width: 48ch;
    margin: 0 0 var(--sp-200);
    color: var(--text-secondary);
    font-size: var(--fs-100);
  }

  /* The tint alone was not enough: `bg-surface-sunken` sits one step from `bg-canvas` in both
     modes, and on a screen that is not showing every step of the neutral ramp the control was
     invisible. It is a surface with a boundary now, which is what says "this is a control" in the
     first place (rule 5's neighbour: a control is identified by more than its colour). */
  .icon-button {
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    cursor: pointer;
  }

  .skip {
    display: inline-block;
    padding: var(--sp-100) var(--sp-150);
    border-radius: var(--r-md);
    background: var(--accent-primary);
    color: var(--text-inverse);
  }
</style>
