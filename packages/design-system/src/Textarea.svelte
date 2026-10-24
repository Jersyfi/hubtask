<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Several lines of text. The same field furniture as Input, and one decision of its own: it
  // grows downwards rather than scrolling, because rule 4 says everything grows by 40 % and a note
  // written in German in a box sized for English is a note read through a slot.

  import type { HTMLTextareaAttributes } from 'svelte/elements';

  import Field from './_Field.svelte';
  import type { Disableable } from './control.ts';

  /** A stretch of the text marked behind it: a token the field read, say. Offsets in the value. */
  export interface TextMark {
    readonly start: number;
    readonly end: number;
  }

  interface Props extends Omit<HTMLTextareaAttributes, 'disabled'>, Disableable {
    label: string;
    /** See `_Field`: the label read but not drawn. */
    isLabelHidden?: boolean;
    /**
     * Stretches of the value drawn as marked, behind the text: what a capture line recognised.
     * Decoration only - the meaning is said elsewhere, never by the colour alone (rule 3).
     */
    marks?: readonly TextMark[];
    hint?: string;
    error?: string;
    isRequired?: boolean;
    /** Lines shown before it grows. It never shrinks below this and never stops at it. */
    rows?: number;
    value?: string;
  }

  let {
    label,
    isLabelHidden = false,
    marks = [],
    hint,
    error,
    disabledReason,
    isRequired = false,
    rows = 3,
    value = $bindable(''),
    ...rest
  }: Props = $props();

  /** The value cut at the marks, for the layer behind the field. */
  const pieces = $derived.by(() => {
    const sorted = [...marks].filter((mark) => mark.end > mark.start).sort((a, b) => a.start - b.start);
    const out: { text: string; isMarked: boolean }[] = [];
    let at = 0;
    for (const mark of sorted) {
      if (mark.start < at) continue;
      if (mark.start > at) out.push({ text: value.slice(at, mark.start), isMarked: false });
      out.push({ text: value.slice(mark.start, mark.end), isMarked: true });
      at = mark.end;
    }
    out.push({ text: value.slice(at), isMarked: false });
    return out;
  });

  let backdrop = $state<HTMLElement | null>(null);
</script>

<!-- Unavailable, it stays editable to the platform and the guard refuses the press and every edit
     (reason.ts), as `Input` does: a read-only field turns `Backspace` into "back" in some engines. -->
<Field {label} {isLabelHidden} {hint} {error} {disabledReason} guardOptions={{ isPressRefused: true, isEditRefused: true }} {isRequired}>
  {#snippet children({ id, describedBy, invalid })}
    <div class="host" data-marked={marks.length > 0 ? '' : undefined}>
    {#if marks.length > 0}
      <!-- The same text in the same box behind the field, with the marks painted; the field above
           is transparent, so the person reads one text and sees the marked words. A trailing
           space keeps a final line break's height. -->
      <div class="backdrop textarea" aria-hidden="true" bind:this={backdrop}>{#each pieces as piece, index (index)}{#if piece.isMarked}<mark>{piece.text}</mark>{:else}{piece.text}{/if}{/each}&#32;</div>
    {/if}
    <textarea
      {id}
      {rows}
      class="textarea"
      data-invalid={invalid ? '' : undefined}
      bind:value
      {...rest}
      aria-disabled={disabledReason !== undefined ? 'true' : undefined}
      required={isRequired && disabledReason === undefined}
      aria-invalid={invalid ? 'true' : undefined}
      aria-describedby={describedBy}
      onscroll={(event) => {
        if (backdrop) backdrop.scrollTop = event.currentTarget.scrollTop;
      }}
    ></textarea>
    </div>
  {/snippet}
</Field>

<style>
  .host { position: relative; }

  /* Behind the field and exactly its box: same padding, border, font and wrapping. Both classes,
     so the field's own minimum and grip below do not reach it. */
  .backdrop.textarea {
    position: absolute;
    inset: 0;
    min-height: 0;
    overflow: hidden;
    border-color: transparent;
    color: transparent;
    white-space: pre-wrap;
    overflow-wrap: break-word;
    pointer-events: none;
    resize: none;
  }

  .backdrop mark {
    border-radius: var(--r-xs);
    background: var(--accent-primary-subtle);
    color: transparent;
  }

  /* The field over its backdrop lets the marks show through. */
  .host[data-marked] textarea { position: relative; background: transparent; }
  .host[data-marked] { border-radius: var(--r-md); background: var(--bg-surface); }

  .textarea {
    display: block;
    /* The padding inside the width, or a full-width field is wider than its column by two
       paddings - and the page scrolls sideways on a phone. */
    box-sizing: border-box;
    width: 100%;
    padding: var(--density-control-md-block) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    /* Vertical only: a horizontal resize breaks the column it sits in, and `both` lets a person
       make the layout wrong in a way nothing recovers from. */
    resize: vertical;
    /* Grows with its content where the browser can, rather than scrolling inside a fixed box. */
    field-sizing: content;
    min-height: var(--sp-800);
  }

  /* Focused, the field's own border takes the focus colour and doubles inward (design-system.md
     §10): the edge the person sees is the field's, not a second ring a gap outside it. */
  /* One line asked for is one line: no taller box and no grip, growing only as it wraps. */
  .textarea[rows='1'] { min-height: 0; resize: none; }

  .textarea:focus-visible {
    outline: none;
    border-color: var(--focus-ring);
    box-shadow: inset 0 0 0 var(--bw-hairline) var(--focus-ring);
  }

  .textarea[data-invalid] { border-color: var(--status-danger-border); border-width: var(--bw-thick); background: var(--status-danger-surface); }

  .textarea::placeholder { color: var(--text-subtle); }

  .textarea[aria-disabled='true'] {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    color: var(--text-subtle);
    cursor: not-allowed;
  }
</style>
