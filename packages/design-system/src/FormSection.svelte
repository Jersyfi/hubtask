<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One titled group of a form, inside a `FormLayout` (page-system §6). In a dialog a heading over
  // its fields; on a page a heading on the canvas over one region of `SettingRow`s. A rare section
  // is folded (`isFolded`): in a dialog behind a `Disclosure` - the one a dialog may hold - and on a
  // page as the region's last line.
  import { getContext, type Snippet } from 'svelte';

  import Disclosure from './Disclosure.svelte';
  import RegionHead from './_RegionHead.svelte';

  interface Props {
    title: string;
    description?: string;
    /** *3 chosen*: a count beside the title, on a page. */
    note?: string;
    /** A section the person rarely needs, folded until opened. */
    isFolded?: boolean;
    children: Snippet;
  }

  const { title, description, note, isFolded = false, children }: Props = $props();

  const place = getContext<(() => 'dialog' | 'page') | undefined>('form-layout-place')?.() ?? 'dialog';
</script>

{#if place === 'page'}
  <section class="page-section">
    {#if isFolded}
      <div class="region folded">
        <Disclosure label={title} {description} tone="quiet">{@render children()}</Disclosure>
      </div>
    {:else}
      <RegionHead {title} {note} {description} />
      <div class="region">{@render children()}</div>
    {/if}
  </section>
{:else if isFolded}
  <Disclosure label={title} {description}>
    <div class="fields">{@render children()}</div>
  </Disclosure>
{:else}
  <fieldset class="dialog-section">
    <legend class="legend">{title}</legend>
    {#if description}<p class="description">{description}</p>{/if}
    <div class="fields">{@render children()}</div>
  </fieldset>
{/if}

<style>
  .page-section { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }

  .region {
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    overflow: clip;
  }

  .folded { padding: var(--sp-150) var(--sp-250); color: var(--text-secondary); font-size: var(--fs-100); }
  .folded :global(.setting) { margin-inline: calc(var(--sp-250) * -1); color: var(--text-primary); }

  .dialog-section { display: flex; flex-direction: column; gap: var(--sp-200); margin: 0; padding: 0; border: 0; min-width: 0; }

  .legend {
    inline-size: 100%;
    padding: 0 0 var(--sp-050);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
  }

  /* A legend is not a flex item; the gap under it is its own. */
  .legend + * { margin-block-start: var(--sp-200); }

  .description { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .fields { display: flex; flex-direction: column; gap: var(--sp-200); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .folded { padding-inline: var(--sp-200); }
    .folded :global(.setting) { margin-inline: calc(var(--sp-200) * -1); }
  }
</style>
