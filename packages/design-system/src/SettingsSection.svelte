<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One section of settings at its own address (page-system §6, PS-9, PS-10): its heading, a
  // sentence and its state on the canvas; below them one region of `SettingRow`s, the rare ones
  // folded under *Advanced* at its end; and one save model - the `SaveBar` in `save` for values,
  // or controls that act at once. A region, never a raised card (§4.2 rule 3).
  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import Disclosure from './Disclosure.svelte';
  import RegionHead from './_RegionHead.svelte';

  interface Props {
    title: string;
    description?: string;
    /** *2 set here*, *Default*, *Locked*. */
    status?: string;
    /** *View as file*: the section's values as the configuration reads them. */
    viewAsFile?: { label: string; onclick: () => void };
    /** The rows. */
    children: Snippet;
    /** The rare rows, folded at the region's end. */
    advanced?: { label: string; description?: string; rows: Snippet };
    /** The section's `SaveBar`, sticky at the content's foot. */
    save?: Snippet;
  }

  const { title, description, status, viewAsFile, children, advanced, save }: Props = $props();
</script>

<section class="settings-section">
  <RegionHead {title} note={status} {description}>
    {#snippet acts()}
      {#if viewAsFile}<Button tone="subtle" size="sm" onclick={viewAsFile.onclick}>{viewAsFile.label}</Button>{/if}
    {/snippet}
  </RegionHead>
  <div class="region">
    {@render children()}
    {#if advanced}
      <div class="advanced">
        <Disclosure label={advanced.label} description={advanced.description} tone="quiet">
          {@render advanced.rows()}
        </Disclosure>
      </div>
    {/if}
  </div>
  {#if save}<div class="save">{@render save()}</div>{/if}
</section>

<style>
  .settings-section { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }

  .region {
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    overflow: clip;
  }

  .advanced {
    padding: var(--sp-150) var(--sp-250);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-secondary);
    font-size: var(--fs-100);
  }

  /* The folded rows run to the region's edges like the others, under the toggle's own gutter. */
  .advanced :global(.setting) { margin-inline: calc(var(--sp-250) * -1); color: var(--text-primary); }

  /* The save bar stays at the content's foot while the section scrolls (§4.3), above the bottom
     bar on a phone. */
  .save { position: sticky; inset-block-end: var(--sp-200); z-index: var(--z-sticky); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .advanced { padding-inline: var(--sp-200); }
    .advanced :global(.setting) { margin-inline: calc(var(--sp-200) * -1); }
    .save { inset-block-end: calc(var(--layout-bottombar-height) + var(--sp-100) + env(safe-area-inset-bottom, 0)); }
  }
</style>
