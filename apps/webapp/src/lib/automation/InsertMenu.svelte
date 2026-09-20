<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The `+` in a gap of the chain (F8-04, decision 1): every kind the manifest serves, the common
  // ones grouped and everything else after them, with a search, in a popover anchored to the gap.
  //
  // The three flow kinds are named here rather than read from the manifest, which is not a
  // compiled-in list of use cases: the contract says they are the engine's own and in no catalogue.

  import { Icon, Popover } from '@hubtask/design-system/components';

  import { FLOW_KINDS } from './model.ts';
  import { grouped, kindWord } from './words.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The action kinds this installation serves, from `/meta/capabilities`. */
    kinds: readonly string[];
    /** Where the chosen kind goes: the list and the index the gap sits at. */
    list: string;
    index: number;
    onpick: (list: string, index: number, kind: string) => void;
    /** A label above the slot, such as "a day later" under a WAIT. */
    caption?: string;
  }

  const { kinds, list, index, onpick, caption }: Props = $props();

  let isOpen = $state(false);
  let query = $state('');

  const words = { t, has: (code: string) => messages.has(code) };
  const groups = $derived([
    ...grouped(kinds),
    { code: 'app.flow.group_flow', kinds: [...FLOW_KINDS] },
  ]);
  const shown = $derived(
    groups
      .map((group) => ({
        code: group.code,
        kinds: group.kinds.filter((kind) => {
          const needle = query.trim().toLowerCase();
          return needle === '' || kindWord(words, kind).toLowerCase().includes(needle) || kind.toLowerCase().includes(needle);
        }),
      }))
      .filter((group) => group.kinds.length > 0),
  );

  function pick(kind: string): void {
    isOpen = false;
    query = '';
    onpick(list, index, kind);
  }
</script>

<div class="gap" data-list={list} data-index={index}>
  <span class="line"></span>
  {#if caption}<span class="caption">{caption}</span>{/if}
  <Popover label={t('app.flow.insert_here')} bind:isOpen>
    {#snippet trigger(attributes)}
      <button {...attributes} class="slot" type="button" aria-label={t('app.flow.insert_here')}>
        <Icon name="plus" size="sm" />
      </button>
    {/snippet}
    <div class="menu">
      <input class="search" type="search" placeholder={t('app.flow.insert_search')} aria-label={t('app.flow.insert_search')} bind:value={query} />
      {#each shown as group (group.code)}
        <span class="group">{t(group.code)}</span>
        {#each group.kinds as kind (kind)}
          <button class="item" type="button" onclick={() => pick(kind)}>{kindWord(words, kind)}</button>
        {/each}
      {/each}
    </div>
  </Popover>
  <span class="line"></span>
</div>

<style>
  .gap { display: flex; flex-direction: column; align-items: center; width: 100%; }

  .line { width: var(--bw-ring); height: var(--sp-150); background: var(--border-default); border-radius: var(--r-full); }

  .caption {
    font-size: var(--fs-050);
    font-weight: var(--fw-medium);
   
    text-transform: uppercase;
    color: var(--text-subtle);
    padding: 0 var(--sp-050);
  }

  /* The circle: a control, so `border.default` (design-system.md §1). Padding zero, or the mark
     sits off centre by the browser's own button inset. */
  .slot {
    width: var(--sp-300);
    height: var(--sp-300);
    padding: 0;
    display: grid;
    place-items: center;
    border-radius: var(--r-full);
    border: var(--bw-hairline) dashed var(--border-default);
    background: var(--bg-surface);
    color: var(--text-subtle);
    transition: transform var(--motion-state-duration) var(--motion-emphasis-easing), background var(--motion-state-duration) var(--motion-state-easing);
  }

  .slot:hover, .slot[aria-expanded='true'] { background: var(--accent-primary); border-color: var(--accent-primary); color: var(--text-inverse); transform: scale(1.15); }

  .slot:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .menu { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 28ch; max-block-size: 50vh; overflow: auto; padding: var(--sp-050); }

  .search {
    min-height: var(--density-control-sm-min);
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
    margin-block-end: var(--sp-050);
  }

  .group { padding: var(--sp-100) var(--sp-100) var(--sp-025); font-size: var(--fs-050); font-weight: var(--fw-medium); text-transform: uppercase; color: var(--text-subtle); }

  .item { text-align: start; padding: var(--sp-050) var(--sp-100); border: 0; border-radius: var(--r-sm); background: transparent; color: var(--text-primary); font-size: var(--fs-075); }

  .item:hover { background: var(--bg-surface-hover); }

  .item:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--sp-025)); }

  @media (prefers-reduced-motion: reduce) { .slot { transition: none; } }
  :global([data-motion='reduced']) .slot { transition: none; }
</style>
