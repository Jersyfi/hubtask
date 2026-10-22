<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where something goes when it leaves the place it is in.
  //
  // The one destination that cannot be a menu item. Up, down, top, bottom, inside the one above and
  // out one level are all positions the reader can see; another collection — or another hub — is a
  // choice out of everything the workspace holds, and a menu of that is a menu nobody can read.
  //
  // It knows nothing about entries or containers. Both are "a thing, and a list of places it could
  // go", and the caller that has the list is the caller that knows what the places are called. Two
  // dialogs would be two places to fix the same focus behaviour.
  //
  // Every string is a prop, resolved (ADR-0011). The component writes no English of its own, which
  // is the same rule the design system holds its components to.

  import { Button, Dialog, Select } from '@hubtask/design-system/components';

  import { focusFirst } from '../forms/missing.ts';

  interface Props {
    isOpen?: boolean;
    /** What is being moved, as the dialog's name. */
    title: string;
    /** The chooser's label, and what it says before a choice is made. */
    label: string;
    placeholder: string;
    options: readonly { readonly value: string; readonly label: string }[];
    /** What to say when there is nowhere to go. */
    emptyLabel: string;
    /** What will not travel, where something will not. Absent when nothing can be lost. */
    warning?: string;
    confirmLabel: string;
    busyLabel: string;
    cancelLabel: string;
    /** Why the confirm is unavailable until a destination is chosen. */
    chooseFirstLabel: string;
    isBusy?: boolean;
    onmove: (value: string) => void;
  }

  let {
    isOpen = $bindable(false),
    title,
    label,
    placeholder,
    options,
    emptyLabel,
    warning,
    confirmLabel,
    busyLabel,
    cancelLabel,
    chooseFirstLabel,
    isBusy = false,
    onmove,
  }: Props = $props();

  let chosen = $state('');
  /** *Move* was pressed with nothing chosen: the list says so (design-system.md §4, case 3). */
  let isChoiceMissing = $state(false);
  let body = $state<HTMLElement | null>(null);

  function move() {
    if (chosen === '') {
      isChoiceMissing = true;
      queueMicrotask(() => focusFirst(body));
      return;
    }
    onmove(chosen);
  }
</script>

<Dialog bind:isOpen {title} dismissLabel={cancelLabel}>
  {#if options.length === 0}
    <p class="none">{emptyLabel}</p>
  {:else}
    <div bind:this={body}>
      <Select
        {label}
        {placeholder}
        {options}
        bind:value={chosen}
        error={isChoiceMissing && chosen === '' ? chooseFirstLabel : undefined}
        onchange={() => (isChoiceMissing = false)}
      />
    </div>
    {#if warning}<p class="warning">{warning}</p>{/if}
  {/if}

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{cancelLabel}</Button>
    <!-- Never off for want of a choice, and never silent: pressed with nothing chosen, the list
         says so and takes focus. -->
    <Button {isBusy} {busyLabel} onclick={move}>
      {confirmLabel}
    </Button>
  {/snippet}
</Dialog>

<style>
  .none,
  .warning { margin: 0; max-width: 64ch; font-size: var(--fs-075); color: var(--text-secondary); }

  /* Rule 3: the warning is a sentence saying what will be lost, and the colour only reinforces it. */
  .warning { color: var(--text-warning); }
</style>
