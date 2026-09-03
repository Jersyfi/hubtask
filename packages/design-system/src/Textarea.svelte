<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Several lines of text. The same field furniture as Input, and one decision of its own: it
  // grows downwards rather than scrolling, because rule 4 says everything grows by 40 % and a note
  // written in German in a box sized for English is a note read through a slot.

  import type { HTMLTextareaAttributes } from 'svelte/elements';

  import Field from './_Field.svelte';
  import type { Disableable } from './control.ts';

  interface Props extends Omit<HTMLTextareaAttributes, 'disabled'>, Disableable {
    label: string;
    hint?: string;
    error?: string;
    isRequired?: boolean;
    /** Lines shown before it grows. It never shrinks below this and never stops at it. */
    rows?: number;
    value?: string;
  }

  let {
    label,
    hint,
    error,
    disabledReason,
    isRequired = false,
    rows = 3,
    value = $bindable(''),
    ...rest
  }: Props = $props();
</script>

<Field {label} {hint} {error} {disabledReason} {isRequired}>
  {#snippet children({ id, describedBy, invalid })}
    <textarea
      {id}
      {rows}
      class="textarea"
      data-invalid={invalid ? '' : undefined}
      bind:value
      disabled={disabledReason !== undefined}
      required={isRequired}
      aria-invalid={invalid ? 'true' : undefined}
      aria-describedby={describedBy}
      {...rest}
    ></textarea>
  {/snippet}
</Field>

<style>
  .textarea {
    display: block;
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

  .textarea:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-color: var(--border-strong);
  }

  .textarea[data-invalid] { border-color: var(--text-danger); border-width: var(--bw-thick); }

  .textarea::placeholder { color: var(--text-subtle); }

  .textarea:disabled {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    color: var(--text-subtle);
    cursor: not-allowed;
  }
</style>
