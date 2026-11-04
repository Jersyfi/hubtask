<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One template as a person chooses it (user-experience.md §5.2): its mark, its name, one sentence
  // of what it is for, the products it switches on as chips, a quiet line of where it stands, and
  // the one act. Never its key or version as the subtitle - that is what a person cannot read.
  //
  // A list item, so a gallery is a list a screen reader counts; the act is the caller's, because
  // what choosing means (a plan first, an apply, a preview) is the screen's and not the card's.

  import type { Snippet } from 'svelte';
  import type { HTMLLiAttributes } from 'svelte/elements';

  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';

  interface Props extends Omit<HTMLLiAttributes, 'class' | 'style'> {
    /** The template's name, in the reader's language. */
    name: string;
    /** One sentence of what it is for. */
    summary?: string;
    mark?: IconName;
    /** The products it switches on, by their names. */
    products?: readonly string[];
    /** Where it stands, quietly: *In use*, *Update available*, its origin. */
    note?: string;
    /** Whether the note is news the reader should act on (an update), drawn in the brand's colour. */
    isNoteNews?: boolean;
    /** The card's act: *Take*, *Update*, *Preview*. */
    action?: Snippet;
    /** What taking it creates, in words, folded under *Preview* (a `Disclosure`): drawn before the act. */
    details?: Snippet;
    /** A small drawing of what the template makes, in place of the mark (a board's frames, a page's outline). */
    preview?: Snippet;
    /** The card is the one chosen in a gallery that is a choice (a create dialog); the act says it in words too. */
    isChosen?: boolean;
    /** Tighter, for a gallery inside a dialog where the cards share the space with a form. */
    isCompact?: boolean;
  }

  const { name, summary, mark = 'layout-template', products = [], note, isNoteNews = false, action, details, preview, isChosen = false, isCompact = false, ...rest }: Props = $props();
</script>

<li class="template" data-chosen={isChosen ? '' : undefined} data-compact={isCompact ? '' : undefined} {...rest}>
  {#if preview}
    <span class="preview" aria-hidden="true">{@render preview()}</span>
  {:else}
    <span class="mark" aria-hidden="true"><Icon name={mark} /></span>
  {/if}
  <span class="name">{name}</span>
  {#if summary}<span class="summary">{summary}</span>{/if}
  {#if products.length > 0}
    <span class="products">
      {#each products as product (product)}<span class="product">{product}</span>{/each}
    </span>
  {/if}
  {#if note}<span class="note" data-news={isNoteNews ? '' : undefined}>{note}</span>{/if}
  {#if details}<div class="details">{@render details()}</div>{/if}
  {#if action}<span class="action">{@render action()}</span>{/if}
</li>

<style>
  .template {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--sp-050);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    min-inline-size: 0;
  }

  /* The chosen card differs by its edge's width as well as its colour (SC 1.4.1). */
  .template[data-chosen] {
    border-color: var(--accent-primary);
    box-shadow: 0 0 0 var(--bw-thick) var(--accent-primary);
  }

  .preview {
    display: block;
    align-self: stretch;
    block-size: var(--sp-1000);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    overflow: hidden;
  }

  .template[data-compact] { padding: var(--sp-100); gap: var(--sp-025); }
  .template[data-compact] .preview { block-size: var(--sp-800); }

  .mark {
    display: inline-flex;
    padding: var(--sp-050);
    border-radius: var(--r-md);
    background: var(--accent-primary-subtle);
    color: var(--text-brand);
  }

  .name { color: var(--text-primary); font-weight: var(--fw-semibold); overflow-wrap: anywhere; }
  .summary { color: var(--text-secondary); font-size: var(--fs-075); }

  .products { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .product {
    padding: 0 var(--sp-050);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .note { color: var(--text-subtle); font-size: var(--fs-075); }
  .note[data-news] { color: var(--text-brand); }

  /* The act sits at the card's foot, whatever the sentence above it took. */
  .details { align-self: stretch; min-inline-size: 0; }
  .action { margin-top: auto; padding-top: var(--sp-050); }
</style>
