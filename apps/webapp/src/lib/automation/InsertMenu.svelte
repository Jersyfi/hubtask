<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The `+` in a gap of the chain (F8-04, decision 1): every kind the manifest serves, the common
  // ones grouped and everything else after them, with a search, in a popover anchored to the gap.
  //
  // The three flow kinds are named here rather than read from the manifest, which is not a
  // compiled-in list of use cases: the contract says they are the engine's own and in no catalogue.

  import { Icon, Popover } from '@hubtask/design-system/components';

  import { FLOW_KINDS, canPlace, type Step } from './model.ts';
  import { gapTakes, type Drag } from './selection.ts';
  import { grouped, kindIcon, kindWord } from './words.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The action kinds this installation serves, from `/meta/capabilities`. */
    kinds: readonly string[];
    /** Where the chosen kind goes: the list and the index the gap sits at. */
    list: string;
    index: number;
    /** The whole chain, which decides what this gap may take (decision 14: a stop goes last). */
    actions: readonly Step[];
    onpick: (list: string, index: number, kind: string) => void;
    /** A label above the slot, such as "a day later" under a WAIT. */
    caption?: string;
    /** What is being dragged, while something is (F8-05): decides whether this gap is a target. */
    drag?: Drag;
    /** A piece let go on this gap. */
    ondrop?: (list: string, index: number, drag: Drag) => void;
  }

  const { kinds, list, index, actions, onpick, caption, drag, ondrop }: Props = $props();

  const target = $derived(gapTakes(drag, list, index, actions));
  let over = $state(false);

  function dragover(event: DragEvent): void {
    if (!target) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = drag?.src === 'step' ? 'move' : 'copy';
    over = true;
  }

  function drop(event: DragEvent): void {
    over = false;
    if (!target || !drag) return;
    event.preventDefault();
    event.stopPropagation();
    ondrop?.(list, index, drag);
  }

  let isOpen = $state(false);
  let query = $state('');

  const words = { t, has: (code: string) => messages.has(code) };
  // The groups alone while nothing is typed; everything the installation serves once something
  // is (decision 12) - the search is how a kind outside the groups is reached, here and nowhere
  // else, so the palette can stay a palette.
  const groups = $derived([
    ...grouped(kinds, query.trim() === '' ? 'folded' : 'listed'),
    { code: 'app.flow.group_flow', kinds: FLOW_KINDS.filter((kind) => canPlace(actions, list, index, kind)) },
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

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="gap"
  class:target
  class:inert={drag !== undefined && !target}
  class:over
  data-list={list}
  data-index={index}
  ondragover={dragover}
  ondragleave={() => (over = false)}
  ondrop={drop}
>
  <span class="line"></span>
  {#if caption}<span class="caption">{caption}</span>{/if}
  <Popover label={t('app.flow.insert_here')} bind:isOpen>
    {#snippet trigger(attributes)}
      <button {...attributes} class="slot" type="button" aria-label={t('app.flow.insert_here')} data-slot>
        {#if target}<span class="word">{t('app.flow.drop_here')}</span>{:else}<Icon name="plus" size="sm" />{/if}
      </button>
    {/snippet}
    <div class="menu">
      <input class="search" type="search" placeholder={t('app.flow.insert_search')} aria-label={t('app.flow.insert_search')} bind:value={query} />
      {#if query.trim() === ''}<span class="more">{t('app.flow.insert_search_hint')}</span>{/if}
      {#each shown as group (group.code)}
        <span class="group">{t(group.code)}</span>
        {#each group.kinds as kind (kind)}
          <button class="item" type="button" onclick={() => pick(kind)}><Icon name={kindIcon(kind)} size="sm" />{kindWord(words, kind)}</button>
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

  /* While a piece is lifted (decision 7): a gap that may take it widens into a labelled target,
     one that may not fades and refuses; the one under the pointer fills. */
  /* Sized by its words, not by its wrapper: the popover's anchor shrinks to fit, so a percentage
     of it was a circle with two lines of text in it. */
  .target .slot { width: auto; min-width: 12ch; padding-inline: var(--sp-150); border-color: var(--accent-primary); color: var(--accent-primary); background: var(--accent-primary-subtle); transform: none; white-space: nowrap; }

  .over .slot { background: var(--accent-primary); color: var(--text-inverse); }

  .inert { opacity: 0.35; }

  .word { font-size: var(--fs-050); font-weight: var(--fw-medium); }

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

  .more { padding: 0 var(--sp-100) var(--sp-050); font-size: var(--fs-050); color: var(--text-subtle); }

  .group { padding: var(--sp-100) var(--sp-100) var(--sp-025); font-size: var(--fs-050); font-weight: var(--fw-medium); text-transform: uppercase; color: var(--text-subtle); }

  .item { display: flex; align-items: center; gap: var(--sp-100); text-align: start; padding: var(--sp-050) var(--sp-100); border: 0; border-radius: var(--r-sm); background: transparent; color: var(--text-primary); font-size: var(--fs-075); }

  .item :global(svg) { color: var(--text-subtle); flex: none; }

  .item:hover { background: var(--bg-surface-hover); }

  .item:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--sp-025)); }

  @media (prefers-reduced-motion: reduce) { .slot { transition: none; } }
  :global([data-motion='reduced']) .slot { transition: none; }
</style>
