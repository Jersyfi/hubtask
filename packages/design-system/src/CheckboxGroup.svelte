<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Several of a few, all of them visible at once - the scopes of a token, the types a field
  // applies to.
  //
  // A fieldset and not a row of loose boxes, because the question belongs to the group: its legend
  // is what a screen reader names when focus enters the first box, and "choose at least one" is a
  // fault of the group rather than of any one box.
  //
  // **What is wrong is said once, for the group** (design-system.md §4, case 3). A fieldset's
  // `group` role takes no `aria-invalid` (the reason `Radio` gives), so the group is marked
  // `data-invalid` and described by the error, and each box is `aria-invalid` as well - the box is
  // what a form moves focus to, and what a screen reader announces as it arrives.

  import Checkbox from './Checkbox.svelte';
  import type { Disableable } from './control.ts';

  export interface CheckboxOption {
    readonly value: string;
    /** Display text: a resolved message code (ADR-0011), or a name the reader gave. */
    readonly label: string;
    readonly hint?: string;
    readonly disabledReason?: string;
  }

  interface Props extends Disableable {
    /** The question the options answer. Rendered as the group's legend. */
    label: string;
    options: readonly CheckboxOption[];
    hint?: string;
    /** What is wrong now - nothing chosen where the form needs one. Names the fix (voice-and-tone.md §3.1). */
    error?: string;
    /** The values ticked. */
    selected?: readonly string[];
    onChange?: (selected: readonly string[]) => void;
  }

  const { label, options, hint, error, disabledReason, selected = [], onChange }: Props = $props();

  // Svelte's own id, not a random one: unique per instance and the same on every render.
  const instanceId = $props.id();
  const uid = `checkbox-group-${instanceId}`;
  const hintId = $derived(hint ? `${uid}-hint` : undefined);
  const errorId = $derived(error ? `${uid}-error` : undefined);
  const describedBy = $derived([errorId, hintId].filter(Boolean).join(' ') || undefined);

  function toggle(value: string, isOn: boolean): void {
    const next = isOn ? [...selected.filter((held) => held !== value), value] : selected.filter((held) => held !== value);
    onChange?.(next);
  }
</script>

<fieldset class="group" data-invalid={error ? '' : undefined} aria-describedby={describedBy}>
  <legend class="legend">{label}</legend>
  {#if hint}<p id={hintId} class="message">{hint}</p>{/if}

  <div class="options">
    {#each options as option (option.value)}
      <Checkbox
        label={option.label}
        hint={option.hint}
        disabledReason={option.disabledReason ?? disabledReason}
        aria-invalid={error ? 'true' : undefined}
        aria-errormessage={errorId}
        bind:checked={() => selected.includes(option.value), (isOn: boolean) => toggle(option.value, isOn)}
      />
    {/each}
  </div>

  {#if error}
    <!-- Rule 3: the sentence is the signal; the border of each box is only its echo. -->
    <p id={errorId} class="message error">{error}</p>
  {/if}
</fieldset>

<style>
  .group {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-width: 0;
    margin: 0;
    padding: 0;
    border: 0;
  }

  .legend {
    padding: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    text-align: start;
  }

  .options { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-200); }

  .message {
    max-width: 60ch;
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
    text-align: start;
  }

  .message.error { color: var(--text-danger); }
</style>
