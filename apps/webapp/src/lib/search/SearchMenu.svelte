<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What the bar's field opens (ADR-0066 decision 4).
  //
  // Today the bar takes words and navigates; it searches nothing. That was a deliberate economy
  // ("the debounce, the language and the widening stay in one place"), and two thirds of its
  // reason go with the language: what is left is a debounce, which is four lines.
  //
  // So the bar answers. Three things are in here and each earns its place:
  //
  // * **The first few hits**, because most searches end at the first one, and a trip to another
  //   screen to read it is a trip nobody needed. It is a `peek`, not a search: one page, and the
  //   search screen's own state is never touched.
  // * **The narrowings that are pressed every day**, because they are how somebody reaches a list
  //   they could not have typed the name of. Pressing one is indistinguishable from having typed
  //   its line — there is no second vocabulary here, only a shortcut into the one there is.
  // * **The way on**, named twice: all the results, and the same question with the filter open.
  //   That is the hinge the whole concept turns on — the bar is where a search *starts* and the
  //   screen is where it is *built* — and a reader must cross from one to the other without
  //   retyping anything.
  //
  // **Not a `Popover`.** That one moves focus into its surface as it opens, which is right for a
  // filter panel and wrong for a list under a field somebody is still typing in. Focus stays in
  // the field; the arrow keys move a highlight, and a screen reader is told which row through
  // `aria-activedescendant` on the field. The rows are `menurows.ts`, so the markup below and the
  // field's key handling cannot count differently.

  import { Badge, Icon, Spinner } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import type { Quick } from '../data/searchquery.ts';
  import { rowsOf } from './menurows.ts';
  import { humanise } from '../i18n/messages.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** What is typed in the field above. Empty is the menu *before* a search, not a closed menu. */
    term: string;
    hits: readonly WorkItem[];
    searching: boolean;
    /** The quick narrowings this installation can answer. */
    quick: readonly Quick[];
    /** Which row the arrows are on, `-1` for none. Counted over `rowsOf`. */
    active: number;
    onhover: (index: number) => void;
    onpick: (index: number) => void;
  }

  const { term, hits, searching, quick, active, onhover, onpick }: Props = $props();

  const rows = $derived(rowsOf(term, hits.map((hit) => hit.id), quick));
  const allAt = $derived(rows.findIndex((row) => row.kind === 'all'));
  const refineAt = $derived(rows.findIndex((row) => row.kind === 'refine'));
  const quickFrom = $derived(hits.length);
</script>

<div class="menu" role="listbox" aria-label={t('app.searchmenu.label')} id="search-menu">
  {#if hits.length > 0}
    <p class="head">{t('app.searchmenu.hits')}</p>
    {#each hits as hit, index (hit.id)}
      <button
        type="button"
        role="option"
        id={`search-row-${index}`}
        aria-selected={active === index}
        class="row"
        data-active={active === index ? '' : undefined}
        onmouseenter={() => onhover(index)}
        onclick={() => onpick(index)}
      >
        <Icon name={hit.completion?.is_completed ? 'square-check' : 'square'} />
        <span class="title">{hit.title}</span>
        <!-- Wrapped, because a `Badge` is an inline-flex box and a flex item shrinks below its own
             content by default: beside a long title it was squeezed to a few characters' width and
             broke its word across two lines. The title is the only thing here that may give way. -->
        <span class="kind"><Badge>{humanise((hit.type ?? '').toLowerCase())}</Badge></span>
      </button>
    {/each}
  {:else if term.trim() !== '' && searching}
    <p class="head"><Spinner size="sm" /> {t('app.searchmenu.searching')}</p>
  {:else if term.trim() !== ''}
    <p class="head">{t('app.searchmenu.none')}</p>
  {/if}

  {#if quick.length > 0}
    <!-- With words these narrow *them*; without, each is a list of its own. The sentence says
         which, because a control that means two things without saying so is one people learn by
         being surprised. -->
    <p class="head">
      {t(term.trim() === '' ? 'app.searchmenu.quick' : 'app.searchmenu.quick_with_words')}
    </p>
    <div class="quick">
      {#each quick as each, offset (each.line)}
        <button
          type="button"
          role="option"
          id={`search-row-${quickFrom + offset}`}
          aria-selected={active === quickFrom + offset}
          class="pill"
          data-active={active === quickFrom + offset ? '' : undefined}
          onmouseenter={() => onhover(quickFrom + offset)}
          onclick={() => onpick(quickFrom + offset)}
        >
          {t(each.code)}
        </button>
      {/each}
    </div>
  {/if}

  <div class="feet">
    <button
      type="button"
      role="option"
      id={`search-row-${allAt}`}
      aria-selected={active === allAt}
      class="row foot"
      data-active={active === allAt ? '' : undefined}
      onmouseenter={() => onhover(allAt)}
      onclick={() => onpick(allAt)}
    >
      <Icon name="search" />
      <span class="title">{t(term.trim() === '' ? 'app.searchmenu.open' : 'app.searchmenu.all')}</span>
      <kbd>{t('app.searchmenu.enter')}</kbd>
    </button>
    {#if refineAt >= 0}
      <button
        type="button"
        role="option"
        id={`search-row-${refineAt}`}
        aria-selected={active === refineAt}
        class="row foot"
        data-active={active === refineAt ? '' : undefined}
        onmouseenter={() => onhover(refineAt)}
        onclick={() => onpick(refineAt)}
      >
        <Icon name="funnel" />
        <span class="title">{t('app.searchmenu.refine')}</span>
      </button>
    {/if}
  </div>
</div>

<style>
  .menu {
    position: absolute;
    inset-inline: 0;
    inset-block-start: calc(100% + var(--sp-050));
    z-index: var(--z-popover);
    max-block-size: 60vh;
    overflow-y: auto;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-overlay);
  }

  .head {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    margin: var(--sp-050) 0 var(--sp-025);
    padding-inline: var(--sp-100);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    inline-size: 100%;
    min-block-size: var(--density-control-md-min);
    padding-inline: var(--sp-100);
    border: 0;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .row[data-active] { background: var(--bg-surface-hover); }

  .title {
    flex: 1 1 auto;
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .kind { flex: none; white-space: nowrap; }

  .quick { display: flex; flex-wrap: wrap; gap: var(--sp-050); padding-inline: var(--sp-100); }

  .pill {
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .pill[data-active], .pill:hover { border-color: var(--accent-primary); color: var(--text-primary); }

  .feet {
    margin-block-start: var(--sp-050);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    padding-block-start: var(--sp-050);
  }

  .foot { color: var(--text-secondary); font-size: var(--fs-075); }

  kbd {
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-sm);
    padding-inline: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }
</style>
