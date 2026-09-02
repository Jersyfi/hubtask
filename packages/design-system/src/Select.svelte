<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One of a known few. A native `<select>` and not a listbox of our own, deliberately: the native
  // control is keyboard-operable, screen-reader correct and uses the platform's own picker on a
  // phone, and none of those is free in a reimplementation. The day a select has to show an icon
  // per option is the day wave 1b's `Menu` answers it (ADR-0039), not this component.

  import type { HTMLSelectAttributes } from 'svelte/elements';

  import Field from './_Field.svelte';
  import Icon from './Icon.svelte';
  import type { ControlSize, Disableable } from './control.ts';

  export interface Option {
    readonly value: string;
    /** Display text: a resolved message code, never a sentence written here (ADR-0011). */
    readonly label: string;
    readonly disabledReason?: string;
  }

  interface Props extends Omit<HTMLSelectAttributes, 'disabled' | 'size'>, Disableable {
    label: string;
    options: readonly Option[];
    hint?: string;
    error?: string;
    isRequired?: boolean;
    size?: ControlSize;
    /** Shown first and unselectable, for a field that starts empty. */
    placeholder?: string;
    value?: string;
  }

  let {
    label,
    options,
    hint,
    error,
    disabledReason,
    isRequired = false,
    size = 'md',
    placeholder,
    value = $bindable(''),
    ...rest
  }: Props = $props();
</script>

<Field {label} {hint} {error} {disabledReason} {isRequired}>
  {#snippet children({ id, describedBy, invalid })}
    <div class="shell" data-size={size} data-invalid={invalid ? '' : undefined}>
      <select
        {id}
        class="select"
        bind:value
        disabled={disabledReason !== undefined}
        required={isRequired}
        aria-invalid={invalid ? 'true' : undefined}
        aria-describedby={describedBy}
        {...rest}
      >
        {#if placeholder}
          <option value="" disabled selected={value === ''}>{placeholder}</option>
        {/if}
        {#each options as option (option.value)}
          <option value={option.value} disabled={option.disabledReason !== undefined}>
            {option.label}
          </option>
        {/each}
      </select>
      <!-- Decoration only: the native control already has its own affordance on some platforms,
           and this one is `aria-hidden` so it is never announced twice. -->
      <Icon name="chevron-down" size="sm" />
    </div>
  {/snippet}
</Field>

<style>
  .shell {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding-inline-end: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-subtle);
  }

  .shell[data-size='md'] { min-height: var(--sp-500); }
  .shell[data-size='sm'] { min-height: var(--sp-400); }

  .shell:has(.select:focus-visible) {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-color: var(--border-strong);
  }

  .shell[data-invalid] { border-color: var(--text-danger); border-width: var(--bw-thick); }

  .shell:has(.select:disabled) {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    cursor: not-allowed;
  }

  .select {
    flex: 1;
    min-width: 0;
    padding-block: var(--sp-100);
    padding-inline-start: var(--sp-150);
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    /* The chevron above replaces the platform's, so the two do not both appear. */
    appearance: none;
  }

  .shell[data-size='sm'] .select { font-size: var(--fs-075); }

  .select:focus { outline: none; }
  .select:disabled { color: var(--text-subtle); cursor: not-allowed; }
</style>
