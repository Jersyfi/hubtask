<!-- SPDX-License-Identifier: Apache-2.0
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

  /** Options under one heading. The native `<optgroup>`, which every platform's picker draws. */
  export interface OptionGroup {
    readonly label: string;
    readonly options: readonly Option[];
  }

  interface Props extends Omit<HTMLSelectAttributes, 'disabled' | 'size'>, Disableable {
    label: string;
    options: readonly Option[];
    /**
     * Options arranged under headings, drawn after `options`. For a list long enough that a person
     * scans by what the entries are about before reading them - forty event types, say. A group
     * with no options is not drawn.
     */
    groups?: readonly OptionGroup[];
    hint?: string;
    error?: string;
    isRequired?: boolean;
    size?: ControlSize;
    /** Shown first and unselectable, for a field that starts empty. */
    placeholder?: string;
    value?: string;
    /**
     * How an unavailable option is written in the open list: its label and its reason, joined by
     * the caller from a message code - the separator and the order are a language's, not ours
     * (voice-and-tone.md §0). Without it, an unavailable option shows its label, and its reason
     * only as a pointer's tooltip.
     */
    describeUnavailable?: (label: string, reason: string) => string;
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
    groups = [],
    value = $bindable(''),
    describeUnavailable,
    ...rest
  }: Props = $props();

  const unavailable = $derived(disabledReason !== undefined);

  /**
   * What an option says. The option that is chosen says its label alone - the closed control
   * shows it, and a reason there would read as the value - and the others say their reason where
   * the person tries them, in the open list.
   */
  function textOf(option: Option): string {
    if (option.disabledReason === undefined || option.value === value || !describeUnavailable) return option.label;
    return describeUnavailable(option.label, option.disabledReason);
  }

  const all = $derived([...options, ...groups.flatMap((group) => group.options)]);
  /**
   * An unavailable select offers only what it holds: it stays focusable and readable, and a
   * keyboard or a picker has nothing else to choose (design-system.md §4).
   */
  const current = $derived(all.filter((option) => option.value === value));
  /** What sizes the closed control: every label, never a reason (see the sizer below). */
  const labels = $derived([...(placeholder ? [placeholder] : []), ...all.map((option) => option.label)]);
</script>

<!-- An unavailable select refuses the press as well: the platform opens its picker on the press,
     and a picker opened over a reason is a list with nothing to choose. -->
<Field {label} {hint} {error} {disabledReason} guardOptions={{ isPressRefused: true, isEveryKeyRefused: true }} {isRequired}>
  {#snippet children({ id, describedBy, invalid })}
    <div class="shell" data-size={size} data-invalid={invalid ? '' : undefined}>
      <select
        {id}
        class="select"
        bind:value
        {...rest}
        aria-disabled={unavailable ? 'true' : undefined}
        required={isRequired && !unavailable}
        aria-invalid={invalid ? 'true' : undefined}
        aria-describedby={describedBy}
      >
        {#if unavailable}
          {#if current.length === 0 && placeholder}
            <option value="" selected>{placeholder}</option>
          {/if}
          {#each current as option (option.value)}
            <option value={option.value}>{option.label}</option>
          {/each}
        {:else}
          {#if placeholder}
            <option value="" disabled selected={value === ''}>{placeholder}</option>
          {/if}
          {#each options as option (option.value)}
            <option value={option.value} disabled={option.disabledReason !== undefined} title={option.disabledReason}>
              {textOf(option)}
            </option>
          {/each}
          {#each groups as group (group.label)}
            {#if group.options.length > 0}
              <optgroup label={group.label}>
                {#each group.options as option (option.value)}
                  <option value={option.value} disabled={option.disabledReason !== undefined} title={option.disabledReason}>
                    {textOf(option)}
                  </option>
                {/each}
              </optgroup>
            {/if}
          {/each}
        {/if}
      </select>
      <!-- Decoration only: the native control already has its own affordance on some platforms,
           and this one is `aria-hidden` so it is never announced twice. -->
      <Icon name="chevron-down" size="sm" />
      <!-- The closed control's width comes from here, not from its options: a native select is as
           wide as its longest option, and an option's reason would widen it - a reason moving the
           page. The labels sit in the select's column, take no height and are never seen; the
           select is laid over the shell and contributes no width of its own. -->
      <span class="sizer" aria-hidden="true">
        {#each labels as each, position (position)}<span>{each}</span>{/each}
      </span>
    </div>
  {/snippet}
</Field>

<style>
  .shell {
    position: relative;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    column-gap: var(--sp-100);
    padding-inline-end: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-subtle);
  }

  .shell[data-size='md'] { min-height: var(--density-control-md-min); }
  .shell[data-size='sm'] { min-height: var(--density-control-sm-min); }

  .shell:has(.select:focus-visible) {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-color: var(--border-strong);
  }

  .shell[data-invalid] { border-color: var(--status-danger-border); border-width: var(--bw-thick); background: var(--status-danger-surface); }

  .shell:has(.select[aria-disabled='true']) {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    cursor: not-allowed;
  }

  .select {
    flex: 1;
    min-width: 0;
    padding-inline-start: var(--sp-150);
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    /* The chevron above replaces the platform's, so the two do not both appear. */
    appearance: none;
  }

  /* Per size, for the reason Input's is: a flat step made a small field taller than its minimum. */
  .shell[data-size='md'] .select { padding-block: var(--density-control-md-block); }
  .shell[data-size='sm'] .select { padding-block: var(--density-control-sm-block); font-size: var(--fs-075); }

  .select:focus { outline: none; }
  .select[aria-disabled='true'] { color: var(--text-subtle); cursor: not-allowed; }

  /* Laid over the shell rather than in it, so its own width - its longest option, reason
     included - sizes nothing. It covers the chevron too, which makes the chevron a place to press. */
  .select {
    position: absolute;
    inset: 0;
    width: 100%;
  }

  .sizer {
    grid-row: 1;
    grid-column: 1;
    display: flex;
    flex-direction: column;
    height: 0;
    padding-inline-start: var(--sp-150);
    overflow: hidden;
    visibility: hidden;
    white-space: nowrap;
  }

  .shell[data-size='sm'] .sizer { font-size: var(--fs-075); }
</style>
