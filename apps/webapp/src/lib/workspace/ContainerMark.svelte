<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's or a collection's own mark: its symbol, on its colour's tile where it has one
  // (UC-WRK-33). The same drawing the navigation gives a row, for the page head and for an entry's
  // place in a list that mixes collections.
  //
  // Decorative, always: it stands beside the place's name, which is what says which place it is.
  // Colour never says it alone (design-system.md §6 rule 3), so nothing here carries a name.

  import { Icon } from '@hubtask/design-system/components';

  import { chosenColour, markOf, type Look } from './look.ts';

  interface Props {
    look: Look;
    /** `md` for a page head, `sm` beside a line of text. */
    size?: 'sm' | 'md';
  }

  const { look, size = 'sm' }: Props = $props();

  const colour = $derived(chosenColour(look));
</script>

<span class="mark" data-size={size} data-token={colour} aria-hidden="true">
  <Icon name={markOf(look)} {size} />
</span>

<style>
  /* Without a colour the symbol takes the text's quieter ink; with one, the label's measured pair -
     its ink on its tile - so the symbol keeps the contrast the label test holds in both modes. */
  .mark {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    border-radius: var(--r-sm);
    color: var(--text-subtle);
  }

  .mark[data-size='sm'] { inline-size: var(--sp-250); block-size: var(--sp-250); }
  .mark[data-size='md'] { inline-size: var(--sp-400); block-size: var(--sp-400); border-radius: var(--r-md); }

  .mark[data-token] { background: var(--mark-bg); color: var(--mark-fg); }

  .mark[data-token='slate'] { --mark-bg: var(--label-slate-bg); --mark-fg: var(--label-slate-fg); }
  .mark[data-token='blue'] { --mark-bg: var(--label-blue-bg); --mark-fg: var(--label-blue-fg); }
  .mark[data-token='teal'] { --mark-bg: var(--label-teal-bg); --mark-fg: var(--label-teal-fg); }
  .mark[data-token='green'] { --mark-bg: var(--label-green-bg); --mark-fg: var(--label-green-fg); }
  .mark[data-token='lime'] { --mark-bg: var(--label-lime-bg); --mark-fg: var(--label-lime-fg); }
  .mark[data-token='amber'] { --mark-bg: var(--label-amber-bg); --mark-fg: var(--label-amber-fg); }
  .mark[data-token='orange'] { --mark-bg: var(--label-orange-bg); --mark-fg: var(--label-orange-fg); }
  .mark[data-token='red'] { --mark-bg: var(--label-red-bg); --mark-fg: var(--label-red-fg); }
  .mark[data-token='magenta'] { --mark-bg: var(--label-magenta-bg); --mark-fg: var(--label-magenta-fg); }
  .mark[data-token='violet'] { --mark-bg: var(--label-violet-bg); --mark-fg: var(--label-violet-fg); }
</style>
