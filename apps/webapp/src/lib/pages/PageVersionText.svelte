<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One version of a page drawn as the page draws its text, read only (UC-DOC-06/3), with each
  // top-level block it took out or put in marked. The marks come from `compareBlocks`, block for
  // block in the order the editor draws them.

  import { createPageEditor, type Editor } from '@hubtask/editor';

  import { t } from '../i18n/i18n.svelte.ts';
  import './page-text.css';

  interface Props {
    body: string;
    /** One mark per top-level block; absent, nothing is marked. */
    marks?: readonly ('same' | 'removed' | 'added')[];
    label: string;
  }

  const { body, marks = [], label }: Props = $props();

  let host = $state<HTMLElement | undefined>(undefined);
  let editor = $state<Editor | undefined>(undefined);

  $effect(() => {
    const element = host;
    if (!element) return;
    const made = createPageEditor({ element, body, editable: false, label, fallback: t('app.pages.reference_loading') });
    editor = made;
    return () => {
      made.destroy();
      editor = undefined;
    };
  });

  $effect(() => {
    const seen = editor;
    const each = marks;
    if (!seen) return;
    const blocks = Array.from(seen.view.dom.children);
    blocks.forEach((block, at) => {
      const mark = each[at];
      if (mark && mark !== 'same') block.setAttribute('data-change', mark);
      else block.removeAttribute('data-change');
    });
  });
</script>

<div class="version page-text" bind:this={host} data-page-version></div>

<style>
  .version { min-inline-size: 0; }
  .version :global(.page-body) { min-block-size: 0; padding-block: 0; font-size: var(--fs-100); }
  .version :global([data-change]) {
    padding-inline: var(--sp-050);
    border-radius: var(--r-sm);
  }
  .version :global([data-change='removed']) {
    background: var(--status-danger-surface);
    color: var(--status-danger-text);
    text-decoration: line-through;
  }
  .version :global([data-change='added']) {
    background: var(--status-success-surface);
    color: var(--status-success-text);
  }
  .version :global([data-change] p) { color: inherit; }
</style>
