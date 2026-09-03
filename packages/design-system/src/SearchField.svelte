<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The input only. What it searches is F2-13's, and this component deliberately knows none of it:
  // a search field that also decided when to send a request would be a second place the debounce
  // and the language live.
  //
  // `type="search"` rather than `type="text"`, which buys the platform's own clear affordance and
  // the right software keyboard on a phone. The clear control is ours as well, because the
  // platform's is absent in several engines and inconsistent in the rest - and a control that
  // exists on some of them is one nobody can rely on.
  //
  // **The term is content.** It is never put in a URL, a log or a title attribute here; `/search`
  // is a POST for exactly that reason (security.md §9, ADR-0018), and a component that helpfully
  // reflected the term into the address bar would undo it.

  import type { HTMLInputAttributes } from 'svelte/elements';

  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import type { ControlSize } from './control.ts';

  interface Props extends Omit<HTMLInputAttributes, 'size' | 'type' | 'value'> {
    /** What is being searched. Resolved text (ADR-0011). */
    label: string;
    /** Whether the label is drawn or only announced. A toolbar usually wants it hidden. */
    isLabelHidden?: boolean;
    /** The name of the control that empties it. Required: it is a control, so it has a name. */
    clearLabel: string;
    size?: ControlSize;
    value?: string;
    onclear?: () => void;
  }

  let {
    label,
    isLabelHidden = false,
    clearLabel,
    size = 'md',
    value = $bindable(''),
    onclear,
    ...rest
  }: Props = $props();

  const id = `search-${Math.random().toString(36).slice(2, 9)}`;

  let input = $state<HTMLInputElement | null>(null);

  function clear() {
    value = '';
    onclear?.();
    // Focus stays in the field: emptying a search is the start of typing a different one, and a
    // reader sent back to the top of the page has to find their way here again.
    input?.focus();
  }
</script>

<div class="field">
  <label class="label" class:is-hidden={isLabelHidden} for={id}>{label}</label>
  <div class="shell" data-size={size}>
    <span class="mark" aria-hidden="true"><Icon name="search" size="sm" /></span>
    <input
      {id}
      class="input"
      type="search"
      bind:this={input}
      bind:value
      onkeydown={(event) => {
        // Escape empties the field rather than closing something. It is the one key a search field
        // is expected to answer, and it does not reach the layer register because a field is not
        // a layer - `layers.ts` only knows about things that were opened.
        if (event.key !== 'Escape' || value === '') return;
        event.stopPropagation();
        clear();
      }}
      {...rest}
    />
    {#if value !== ''}
      <IconButton icon="x" label={clearLabel} size="sm" onclick={clear} />
    {/if}
  </div>
</div>

<style>
  .field { display: flex; flex-direction: column; gap: var(--sp-050); min-width: 0; }

  .label { color: var(--text-secondary); font-size: var(--fs-075); }

  /* Announced but not drawn. Not `display: none`, which would take it out of the accessibility
     tree along with the label the field depends on. */
  .label.is-hidden {
    position: absolute;
    inline-size: var(--sp-025);
    block-size: var(--sp-025);
    margin: calc(var(--sp-025) * -1);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

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

  .shell:has(.input:focus-visible) {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-color: var(--border-strong);
  }

  .mark { display: inline-flex; flex: none; }

  .input {
    flex: 1;
    min-width: 0;
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
  }

  .shell[data-size='md'] .input { padding-block: var(--density-control-md-block); }
  .shell[data-size='sm'] .input {
    padding-block: var(--density-control-sm-block);
    font-size: var(--fs-075);
  }

  .input:focus { outline: none; }
  .input::placeholder { color: var(--text-subtle); }

  /* The platform's own clear button, removed: ours is beside it and two would be a puzzle. */
  .input::-webkit-search-cancel-button { appearance: none; }
</style>
