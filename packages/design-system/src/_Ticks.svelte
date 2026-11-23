<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Several answers as visible ticks in a row that wraps: `MultiSelect`'s inline presentation and
  // every option of `CheckboxGroup` (ADR-0110).
  //
  // Underscored: a part, so that the two cannot disagree about the spacing of a row of boxes, what
  // a press emits, or how a group's error reaches each box.

  import Checkbox from './Checkbox.svelte';
  import type { ControlSize } from './control.ts';
  import { toggled, type ChoiceOption } from './multiselect.ts';

  interface Props {
    options: readonly ChoiceOption[];
    selected: readonly string[];
    /** Called once per press with the whole selection after it. */
    onChange: (selected: readonly string[]) => void;
    /** The whole group's reason, for every box that has none of its own. */
    disabledReason?: string;
    /** The group's error, by id: each box is `aria-invalid` and names it (design-system.md §4, case 3). */
    errorId?: string;
    /** `md` is `CheckboxGroup`'s spacing, `sm` the structure panel's denser rows. */
    size?: ControlSize;
  }

  const { options, selected, onChange, disabledReason, errorId, size = 'md' }: Props = $props();
</script>

<div class="ticks" data-size={size}>
  {#each options as option (option.value)}
    <Checkbox
      label={option.label}
      hint={option.hint}
      density={size}
      disabledReason={option.disabledReason ?? disabledReason}
      aria-invalid={errorId ? 'true' : undefined}
      aria-errormessage={errorId}
      bind:checked={() => selected.includes(option.value), () => onChange(toggled(selected, option.value, options))}
    />
  {/each}
</div>

<style>
  .ticks { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-200); }
  .ticks[data-size='sm'] { gap: var(--sp-050) var(--sp-200); }
</style>
