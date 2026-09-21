<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The building blocks: one list, three ways in (F8-16, decision 17). The panel's *Blocks* tab
  // holds it whole; the `+` popover of a gap draws the same list, filtered to what that gap may
  // take; and every item is dragged or clicked alike.
  //
  // Two sub-tabs and a search over both. *Blocks* is what a rule is usually built from: the six
  // kinds this workspace's rules use most, counted on the client from the rules it holds; the
  // trigger kinds; then the curated categories in one fixed order; and the engine's own flow
  // kinds. *All* is every kind the installation serves, grouped the same way with everything
  // else at the end, each with the sentence the manifest carries for it - nothing is hidden, and
  // nothing is compiled in that the manifest does not serve.

  import { Icon } from '@hubtask/design-system/components';

  import { FLOW_KINDS } from './model.ts';
  import { COMMON_GROUPS, TRIGGER_ICONS, kindFamily, kindIcon, kindWord, triggerWord } from './words.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** What the manifest serves: the trigger kinds, the action kinds, and each kind's sentence. */
    triggers: readonly string[];
    kinds: readonly string[];
    summaries: Readonly<Record<string, string>>;
    /** How often each kind is used in this workspace's rules; *Frequent* is the top of it. */
    usage: ReadonlyMap<string, number>;
    /** In the `+` popover: only what that gap may take, and no trigger. Absent offers everything. */
    allowed?: (kind: string) => boolean;
    onpick: (kind: string) => void;
    onpicktrigger?: (kind: string) => void;
    /** A piece lifted from the list; the caller carries the drag. */
    onlift?: (event: DragEvent, piece: { src: 'trigger' | 'action'; kind: string }) => void;
    ondrop?: () => void;
    /** Focus the search when the list opens - what a popover wants. */
    autofocus?: boolean;
  }

  const { triggers, kinds, summaries, usage, allowed, onpick, onpicktrigger, onlift, ondrop, autofocus = false }: Props = $props();

  const words = { t, has: (code: string) => messages.has(code) };

  let side = $state<'blocks' | 'all'>('blocks');
  let query = $state('');

  const served = $derived(new Set(kinds));
  const takes = (kind: string): boolean => (allowed ? allowed(kind) : true);
  const needle = $derived(query.trim().toLowerCase());
  const hit = (kind: string): boolean =>
    needle === '' || kindWord(words, kind).toLowerCase().includes(needle) || kind.toLowerCase().includes(needle) || (summaries[kind] ?? '').toLowerCase().includes(needle);

  interface Group {
    code: string;
    hint?: string;
    kinds: string[];
  }

  /** The curated list: Frequent, the categories in their order, Flow. */
  const curated = $derived.by((): Group[] => {
    const frequent = [...usage.entries()]
      .filter(([kind]) => served.has(kind) || (FLOW_KINDS as readonly string[]).includes(kind))
      .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
      .slice(0, 6)
      .map(([kind]) => kind);
    const groups: Group[] = [];
    if (frequent.length > 0) groups.push({ code: 'app.flow.blocks_frequent', hint: t('app.flow.blocks_frequent_hint'), kinds: frequent });
    for (const group of COMMON_GROUPS) {
      const present = group.kinds.filter((kind) => served.has(kind));
      if (present.length > 0) groups.push({ code: group.code, kinds: present });
    }
    groups.push({ code: 'app.flow.group_flow', kinds: [...FLOW_KINDS] });
    return groups;
  });

  /** Everything: the categories, then every served kind in no category, sorted. */
  const everything = $derived.by((): Group[] => {
    const placed = new Set(COMMON_GROUPS.flatMap((group) => group.kinds));
    const groups: Group[] = COMMON_GROUPS.map((group) => ({ code: group.code, kinds: group.kinds.filter((kind) => served.has(kind)) })).filter((group) => group.kinds.length > 0);
    groups.push({ code: 'app.flow.group_flow', kinds: [...FLOW_KINDS] });
    const rest = kinds.filter((kind) => !placed.has(kind)).sort();
    if (rest.length > 0) groups.push({ code: 'app.flow.group_other', kinds: rest });
    return groups;
  });

  const filtered = (groups: Group[]): Group[] =>
    groups.map((group) => ({ ...group, kinds: group.kinds.filter((kind) => takes(kind) && hit(kind)) })).filter((group) => group.kinds.length > 0);
  /** What the search finds: the sub-tab's own list, and the whole catalogue when the blocks have nothing for the words typed. */
  const fromAll = $derived(side === 'blocks' && needle !== '' && filtered(curated).length === 0);
  const shown = $derived(fromAll ? filtered(everything) : filtered(side === 'blocks' ? curated : everything));

  const shownTriggers = $derived(side === 'blocks' && !allowed ? triggers.filter((kind) => needle === '' || triggerWord(words, kind).toLowerCase().includes(needle)) : []);

  const count = $derived(kinds.length + FLOW_KINDS.length);
</script>

<div class="blocks" data-blocks>
  <div class="sub" role="tablist">
    <button type="button" role="tab" aria-selected={side === 'blocks'} onclick={() => (side = 'blocks')}>{t('app.flow.blocks_common')}</button>
    <button type="button" role="tab" aria-selected={side === 'all'} onclick={() => (side = 'all')}>{t('app.flow.blocks_all', { count })}</button>
  </div>
  <!-- svelte-ignore a11y_autofocus -->
  <input class="search" type="search" placeholder={t('app.flow.blocks_search')} aria-label={t('app.flow.blocks_search')} bind:value={query} {autofocus} />
  <div class="list">
    {#if shownTriggers.length > 0}
      <span class="group">{t('app.flow.palette_starts')} <em>{t('app.flow.palette_starts_where')}</em></span>
      {#each shownTriggers as kind (kind)}
        <button
          class="item trigger"
          type="button"
          draggable={onlift !== undefined}
          data-block={`T:${kind}`}
          ondragstart={(event) => onlift?.(event, { src: 'trigger', kind })}
          ondragend={() => ondrop?.()}
          onclick={() => onpicktrigger?.(kind)}
        >
          <span class="mark"><Icon name={TRIGGER_ICONS[kind] ?? 'zap'} size="sm" /></span>
          <span class="w">{triggerWord(words, kind)}</span>
        </button>
      {/each}
    {/if}
    {#each shown as group (group.code)}
      <span class="group">{t(group.code)}{#if group.hint}<em>{group.hint}</em>{/if}</span>
      {#each group.kinds as kind (kind)}
        <button
          class="item {kindFamily(kind)}"
          type="button"
          draggable={onlift !== undefined}
          data-block={kind}
          title={summaries[kind] ?? kindWord(words, kind)}
          ondragstart={(event) => onlift?.(event, { src: 'action', kind })}
          ondragend={() => ondrop?.()}
          onclick={() => onpick(kind)}
        >
          <span class="mark"><Icon name={kindIcon(kind)} size="sm" /></span>
          <span class="w">
            <span>{kindWord(words, kind)}</span>
            {#if (side === 'all' || fromAll) && summaries[kind]}<span class="d">{summaries[kind]}</span>{/if}
          </span>
          {#if usage.get(kind)}<span class="use" aria-hidden="true" title={t('app.flow.blocks_uses', { n: usage.get(kind) ?? 0 })}>{usage.get(kind)}</span>{/if}
        </button>
      {/each}
    {/each}
    {#if fromAll && shown.length > 0}<p class="quiet tiny">{t('app.flow.blocks_from_all')}</p>{/if}
    {#if shown.length === 0 && shownTriggers.length === 0}
      <p class="quiet">{t('app.flow.blocks_none_all')}</p>
    {/if}
    {#if allowed && needle === ''}<p class="quiet tiny">{t('app.flow.blocks_only_here')}</p>{/if}
  </div>
</div>

<style>
  .blocks { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }

  .sub { display: flex; border-block-end: var(--bw-hairline) solid var(--border-subtle); }

  .sub button { padding: var(--sp-050) var(--sp-100); border: 0; border-block-end: var(--bw-ring) solid transparent; background: transparent; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); cursor: pointer; }

  .sub button[aria-selected='true'] { color: var(--text-primary); border-block-end-color: var(--accent-primary); }

  .sub button:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--sp-025)); }

  .search { min-height: var(--density-control-sm-min); padding: var(--sp-050) var(--sp-100); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-sm); background: var(--bg-surface); color: var(--text-primary); }

  .list { display: flex; flex-direction: column; gap: var(--sp-025); }

  .group { display: flex; justify-content: space-between; gap: var(--sp-100); padding: var(--sp-100) var(--sp-050) var(--sp-025); font-size: var(--fs-050); font-weight: var(--fw-medium); text-transform: uppercase; color: var(--text-subtle); }

  .group em { font-style: normal; font-weight: var(--fw-regular); text-transform: none; }

  .item { display: flex; align-items: center; gap: var(--sp-100); width: 100%; padding: var(--sp-050); border: var(--bw-hairline) solid transparent; border-radius: var(--r-sm); background: transparent; color: var(--text-primary); font-size: var(--fs-075); text-align: start; cursor: grab; }

  .item:hover { background: var(--bg-surface-hover); border-color: var(--border-subtle); }

  .item:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--sp-025)); }

  .w { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; gap: var(--sp-025); overflow-wrap: anywhere; }

  .d { font-size: var(--fs-050); color: var(--text-secondary); }

  .use { flex: 0 0 auto; padding: 0 var(--sp-050); border-radius: var(--r-full); background: var(--bg-surface-sunken); color: var(--text-subtle); font-size: var(--fs-050); }

  /* The colour says what the block is (decision 18): the same families as on the card. */
  .mark { flex: 0 0 auto; display: inline-grid; place-items: center; width: var(--sp-250); height: var(--sp-250); border-radius: var(--r-xs); background: var(--label-blue-bg); color: var(--label-blue-fg); }

  .trigger .mark { background: var(--accent-primary-subtle); color: var(--accent-primary); }

  .people .mark { background: var(--label-teal-bg); color: var(--label-teal-fg); }

  .outbound .mark, .other .mark { background: var(--label-slate-bg); color: var(--label-slate-fg); }

  .ai .mark { background: var(--label-green-bg); color: var(--label-green-fg); }

  .flow .mark { background: var(--label-violet-bg); color: var(--label-violet-fg); }

  .quiet { margin: 0; padding: var(--sp-100) var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); }

  .tiny { font-size: var(--fs-050); color: var(--text-subtle); }
</style>
