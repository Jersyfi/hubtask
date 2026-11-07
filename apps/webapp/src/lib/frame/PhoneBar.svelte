<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The phone's top bar (the navigation concept §9): on a tab's root the avatar, the title and the
  // marks; deeper *‹ the level above*, the title, ☆ and ⋯. The same drawing as `BrowseStack`'s bar,
  // drawn by the frame because every screen of the phone is a step of a stack - the Browse steps
  // and the places they open - and because the frame's one live region says each change.
  import type { Snippet } from 'svelte';

  import { Icon, IconButton, Menu } from '@hubtask/design-system/components';

  import type { Star } from '@hubtask/sync-engine';

  import { isStarred } from '../data/stars.ts';
  import { stars } from '../data/stars.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { PageMenu } from './page.svelte.ts';

  interface Props {
    label: string;
    title?: string;
    /** *‹ the level above*: its name and what pressing it does. Absent on a root. */
    back?: { readonly label: string; readonly path: string };
    onback?: (path: string) => void;
    /** The place's ☆, where it has one. */
    star?: { readonly kind: Star['kind']; readonly id: string; readonly name: string };
    menu?: PageMenu;
    /** On a root: the avatar before the title. */
    account?: Snippet;
    /** On a root: the marks after it. */
    marks?: Snippet;
  }

  const { label, title, back, onback, star, menu, account, marks }: Props = $props();
  const starred = $derived(star ? isStarred(stars.all, star.kind, star.id) : false);
</script>

<header class="bar" aria-label={label} data-phone-bar>
  <div class="start">
    {#if back}
      <a class="back" href={back.path} data-back onclick={(event) => { if (!onback) return; event.preventDefault(); onback(back.path); }}>
        <Icon name="chevron-left" size="sm" />
        <span class="back-word">{back.label}</span>
      </a>
    {:else if account}
      {@render account()}
    {/if}
  </div>
  <!-- Named only once the screen has named itself, so that "the bar's title" is never empty. -->
  {#if title}<span class="title" data-bar="title">{title}</span>{:else}<span class="title"></span>{/if}
  <div class="end">
    {#if back}
      {#if star}
        <span class="star" data-starred={starred ? '' : undefined}>
          <IconButton icon="star" label={t(starred ? 'app.nav.unstar' : 'app.nav.star', { name: star.name })} aria-pressed={starred} onclick={() => void stars.toggle(star.kind, star.id)} />
        </span>
      {/if}
      {#if menu}
        <span class="page-menu" data-bar="menu">
        <Menu label={menu.label} items={menu.items} placement={{ side: 'block-end', align: 'end' }} onselect={menu.onselect}>
          {#snippet trigger(props)}
            <IconButton icon="ellipsis" label={menu.label} data-opener={menu.opener} {...props} />
          {/snippet}
        </Menu>
        </span>
      {/if}
    {:else if marks}
      {@render marks()}
    {/if}
  </div>
</header>

<style>
  /* Sticky at the top like the wider bar, under the system's own bar where a phone has one. */
  .bar {
    position: sticky;
    inset-block-start: 0;
    z-index: var(--z-sticky);
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, auto) minmax(0, 1fr);
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--layout-appbar-height);
    padding: env(safe-area-inset-top, 0) calc(var(--sp-100) + env(safe-area-inset-right, 0)) 0 calc(var(--sp-100) + env(safe-area-inset-left, 0));
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    background: var(--bg-surface);
  }

  .start { display: flex; align-items: center; min-inline-size: 0; }
  .end { display: flex; align-items: center; justify-content: flex-end; min-inline-size: 0; }

  /* Every control of the bar a thumb's size (§9: 44 px), the marks' included. */
  .bar :global(button) { min-inline-size: var(--layout-nav-touchrow); min-block-size: var(--layout-nav-touchrow); }

  .title {
    overflow: hidden;
    font-family: var(--font-ui);
    font-size: var(--fs-100);
    font-weight: var(--fw-semibold);
    text-align: center;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* The way up: as tall as a thumb needs, its words giving way first. */
  .back {
    display: flex;
    align-items: center;
    min-inline-size: 0;
    min-block-size: var(--layout-nav-touchrow);
    padding-inline-end: var(--sp-050);
    color: var(--text-brand);
    font-weight: var(--fw-medium);
    text-decoration: none;
  }

  .back :global(.icon) { flex: none; inline-size: var(--fs-300); block-size: var(--fs-300); }

  .back-word { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .back:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-sm); }

  .star, .page-menu { display: inline-flex; }
  .star[data-starred] :global(button) { color: var(--accent-star); }
  .star[data-starred] :global(svg) { fill: currentColor; }
</style>
