<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One line of text, with the label, hint and error a field owes it.

  import type { HTMLInputAttributes } from 'svelte/elements';

  import Field from './_Field.svelte';
  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';
  import type { ControlSize, Disableable } from './control.ts';

  interface Props extends Omit<HTMLInputAttributes, 'disabled' | 'size'>, Disableable {
    label: string;
    hint?: string;
    error?: string;
    isRequired?: boolean;
    size?: ControlSize;
    /** An icon inside the control, at the start. A search field's magnifier, a date's calendar. */
    icon?: IconName;
    value?: string;
  }

  let {
    label,
    hint,
    error,
    disabledReason,
    isRequired = false,
    size = 'md',
    icon,
    value = $bindable(''),
    type = 'text',
    ...rest
  }: Props = $props();
</script>

<Field {label} {hint} {error} {disabledReason} {isRequired}>
  {#snippet children({ id, describedBy, invalid })}
    <div class="shell" data-size={size} data-invalid={invalid ? '' : undefined}>
      {#if icon}
        <Icon name={icon} size="sm" />
      {/if}
      <input
        {id}
        {type}
        class="input"
        bind:value
        disabled={disabledReason !== undefined}
        required={isRequired}
        aria-invalid={invalid ? 'true' : undefined}
        aria-describedby={describedBy}
        {...rest}
      />
    </div>
  {/snippet}
</Field>

<style>
  .shell {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-subtle);
  }

  .shell[data-size='md'] { padding-inline: var(--sp-150); min-height: var(--density-control-md-min); }
  .shell[data-size='sm'] { padding-inline: var(--sp-100); min-height: var(--density-control-sm-min); }

  /* The ring goes on the shell, not on the control, so an icon inside it is inside the ring too -
     and through `:has(:focus-visible)` rather than `:focus-within`, because the latter also rings
     a field somebody clicked into, which rule 5 is not about. */
  .shell:has(.input:focus-visible) {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-color: var(--border-strong);
  }

  .shell[data-invalid] { border-color: var(--text-danger); border-width: var(--bw-thick); }

  .shell:has(.input:disabled) {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    cursor: not-allowed;
  }

  .input {
    flex: 1;
    min-width: 0;
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
  }

  /* Per size, because the shell's minimum is only true if the field inside it fits within one.
     A flat step here made a small field taller than the minimum it declared. */
  .shell[data-size='md'] .input { padding-block: var(--density-control-md-block); }
  .shell[data-size='sm'] .input { padding-block: var(--density-control-sm-block); font-size: var(--fs-075); }

  .input:focus { outline: none; }
  .input::placeholder { color: var(--text-subtle); }
  .input:disabled { color: var(--text-subtle); cursor: not-allowed; }
</style>
