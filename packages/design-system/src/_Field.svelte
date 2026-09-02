<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The label, the hint, the error and the reason - the parts every text control repeats.
  //
  // Underscored, so check-stories reads it as a part rather than as a component §4 owes an entry
  // (ADR-0037). A part shared by three components rather than by one, which is what makes it worth
  // extracting: three copies of "which id does the error message hang on" is three chances to get
  // it wrong. It renders no control of its own - the caller passes one in, already wired to the
  // ids this hands back.

  import type { Snippet } from 'svelte';

  import Stack from './Stack.svelte';

  interface Props {
    /** The visible label. Sentence case (voice-and-tone.md §1.1). */
    label: string;
    /** Standing help, shown before anything goes wrong. */
    hint?: string;
    /** What is wrong now. Names the fix, not only the fault (voice-and-tone.md §3.1). */
    error?: string;
    /** Why the control cannot be used. */
    disabledReason?: string;
    isRequired?: boolean;
    /** Takes the ids to hang on the control: `{ id, describedBy, invalid }`. */
    children: Snippet<[{ id: string; describedBy: string | undefined; invalid: boolean }]>;
  }

  const { label, hint, error, disabledReason, isRequired = false, children }: Props = $props();

  const uid = `field-${Math.random().toString(36).slice(2, 9)}`;
  const hintId = $derived(hint ? `${uid}-hint` : undefined);
  const errorId = $derived(error ? `${uid}-error` : undefined);
  const reasonId = $derived(disabledReason ? `${uid}-reason` : undefined);
  // Order matters: a screen reader reads them in this order, and the error is the thing to hear
  // first when there is one.
  const describedBy = $derived([errorId, reasonId, hintId].filter(Boolean).join(' ') || undefined);
</script>

<Stack as="div" gap="050" class="field">
  <label class="label" for={uid}>
    {label}{#if isRequired}<span class="required" aria-hidden="true">*</span>{/if}
  </label>

  {@render children({ id: uid, describedBy, invalid: error !== undefined })}

  {#if error}
    <!-- Rule 3: colour never stands alone. The message is text, and the border is only the echo. -->
    <p id={errorId} class="message error">{error}</p>
  {/if}
  {#if disabledReason}
    <p id={reasonId} class="message">{disabledReason}</p>
  {/if}
  {#if hint}
    <p id={hintId} class="message">{hint}</p>
  {/if}
</Stack>

<style>
  .label {
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    /* `start`, never left: RTL is a requirement and not a later port (§3). */
    text-align: start;
  }

  .required {
    margin-inline-start: var(--sp-025);
    color: var(--text-danger);
  }

  .message {
    max-width: 60ch;
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
    text-align: start;
    overflow-wrap: anywhere;
  }

  .message.error { color: var(--text-danger); }
</style>
