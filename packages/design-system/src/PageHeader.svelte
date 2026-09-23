<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The head of a page (ADR-0061 decision 2): where it is, what it is called, the one thing one
  // does here most, and everything else behind a menu.
  //
  // The shape is the rule. A caller hands in **one** primary action, at most **two** secondary
  // ones and the rest as the menu's items, and the props' types are what say so - there is no
  // slot a third button could be rendered into. That is deliberate: the collection screen had
  // twelve buttons of one tone and one size, with the action performed daily a form at the end of
  // the list, and every one of them was a reasonable decision on its own.
  //
  // Below `medium` the head folds: the primary action becomes a round button pinned at the end of
  // the screen above the bottom bar, where a thumb is, and the secondary actions join the menu.
  // The title stays an `h1` here on every width - a screen holds exactly one, and it belongs to
  // the content - but the frame that shows the title in its bar asks for it to be read rather
  // than drawn, so the reader is told the title once.
  //
  // The breadcrumb is `Breadcrumb`, which already collapses to hub › … › parent › current from
  // `medium` down; below `medium` this shows the parent alone as the way up.

  import type { Snippet } from 'svelte';

  import Breadcrumb from './Breadcrumb.svelte';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import Menu from './Menu.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';
  import type { IconName } from './icons/index.ts';
  import type { MenuItem } from './overlay.ts';
  import type { Crumb } from './structure.ts';

  /** An action in the head: the verb, its mark, what it does, and why it cannot (Disableable). */
  export interface PageAction {
    readonly label: string;
    readonly icon?: IconName;
    readonly onclick: () => void;
    readonly disabledReason?: string;
    /** For the tour and for tests: the attribute the caller finds the control by. */
    readonly tour?: string;
    /**
     * `data-opener`: the name a form or a dialog returns focus to when the control that opened it
     * has been re-rendered in the meantime (`focusFirst`'s `returnTo`), and what the tour's last
     * step points at.
     */
    readonly opener?: string;
    /**
     * A second way to do the same thing, beside the button: "create an entry" and "from a
     * template…". Drawn as a split button - the verb, and a chevron that opens the list - from
     * `medium` up; folded, the list's items join the page menu, because a round button pinned at
     * the bottom of a phone has no room for a second half.
     */
    readonly menu?: { readonly label: string; readonly items: readonly MenuItem[]; readonly onselect: (id: string) => void };
  }

  interface Props {
    title: string;
    subtitle?: string;
    /** The trail above the title, with its own two names (see `Breadcrumb`). */
    breadcrumb?: { trail: readonly Crumb[]; label: string; expandLabel: string; onnavigate?: (id: string) => void };
    /** The one thing one does here most. */
    primary?: PageAction;
    /** At most two; the third belongs in the menu, and the type says so. */
    secondary?: readonly [] | readonly [PageAction] | readonly [PageAction, PageAction];
    /** Everything else, grouped with `hasSeparatorBefore`, the destructive item last. */
    menu?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void; opener?: string };
    /** The bar that shows the title asks for it to be read, not drawn, below `medium`. */
    isTitleInBar?: boolean;
    /**
     * Whether the frame draws this page's menu in the app bar instead of here (ADR-0061 §1's
     * table, issue 904).
     *
     * The head still decides *what* is in it - the folding is this component's, and two foldings
     * would eventually disagree about which action is an action and which is an item - so it
     * hands the folded list over through `onmenu` and draws no control of its own.
     */
    isMenuInBar?: boolean;
    /**
     * What the head would have drawn, handed to whoever draws it instead.
     *
     * Called with the folded list while `isMenuInBar`, and with nothing when the menu comes back
     * here or the head leaves - so the bar never keeps a menu belonging to a page that is gone.
     */
    onmenu?: (menu: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void; opener?: string } | undefined) => void;
    /** Lines under the title: an archived notice, a refusal. The caller's `role="alert"` travels with them. */
    notices?: Snippet;
    /** The second row: a `ViewSwitcher`, `Tabs`, or nothing. */
    views?: Snippet;
  }

  const {
    title,
    subtitle,
    breadcrumb,
    primary,
    secondary = [],
    menu,
    isTitleInBar = false,
    isMenuInBar = false,
    onmenu,
    notices,
    views,
  }: Props = $props();

  // The way up on a phone: the crumb before the current one, if there is one.
  const parent = $derived(breadcrumb && breadcrumb.trail.length > 1 ? breadcrumb.trail[breadcrumb.trail.length - 2] : undefined);
  // With the title drawn elsewhere and no subtitle, the actions have no title to stand beside
  // and take the trail's line; the title row then holds only the read-not-drawn heading.
  const actionsOnTrail = $derived(isTitleInBar && !subtitle && breadcrumb !== undefined);

  // Below `medium` the secondary actions join the menu, so one control holds everything that is
  // not the primary. The ids are prefixed so a caller's own ids cannot collide with them.
  const foldedItems = $derived<readonly MenuItem[]>([
    ...(primary?.menu?.items ?? []).map((item) => ({ ...item, id: `primary-${item.id}` })),
    ...secondary.map((action, index) => ({
      id: `secondary-${index}`,
      label: action.label,
      icon: action.icon,
      disabledReason: action.disabledReason,
    })),
    ...(menu?.items ?? []).map((item, index) =>
      index === 0 && (secondary.length > 0 || (primary?.menu?.items.length ?? 0) > 0) ? { ...item, hasSeparatorBefore: true } : item,
    ),
  ].map((item, index) =>
    // The secondary actions stand apart from the primary's own list, when there is one.
    index === (primary?.menu?.items.length ?? 0) && index > 0 && secondary.length > 0 ? { ...item, hasSeparatorBefore: true } : item,
  ));

  function onFoldedSelect(id: string) {
    const fromPrimary = /^primary-(.+)$/.exec(id);
    if (fromPrimary) return primary?.menu?.onselect(fromPrimary[1] ?? '');
    const match = /^secondary-(\d+)$/.exec(id);
    if (match) secondary[Number(match[1])]?.onclick();
    else menu?.onselect(id);
  }

  // Whether the head folds is decided by the width the head *has*, not by the viewport: a head
  // inside a detail pane, or in a workbench pane set to a phone's width, folds like one on a
  // phone. A media query cannot see that, and a container query would make the head a containing
  // block for the pinned primary action; so the head measures itself against the token, read
  // from the stylesheet rather than written here.
  let head = $state<HTMLElement | null>(null);
  let isFolded = $state(false);

  // The menu, handed to the frame while the frame is the one drawing it. An effect rather than a
  // call at render time: what is in the list depends on the props, and the cleanup is what takes
  // the menu out of the bar when this page leaves.
  $effect(() => {
    if (!isMenuInBar || foldedItems.length === 0) {
      onmenu?.(undefined);
      return;
    }
    const label = menu?.label ?? secondary[0]?.label ?? '';
    onmenu?.({ label, items: foldedItems, onselect: onFoldedSelect, opener: menu?.opener });
    return () => onmenu?.(undefined);
  });

  $effect(() => {
    const element = head;
    if (!element || typeof ResizeObserver === 'undefined') return;
    const medium = Number.parseFloat(getComputedStyle(element).getPropertyValue('--bp-medium'));
    const observer = new ResizeObserver(([entry]) => {
      if (entry && Number.isFinite(medium)) isFolded = entry.contentRect.width < medium;
    });
    observer.observe(element);
    return () => observer.disconnect();
  });
</script>

{#snippet actionBar()}
    <div class="actions">
      {#if primary}
        <span class="primary" data-split={primary.menu && primary.menu.items.length > 0 ? '' : undefined}>
          <Button tone="primary" icon={primary.icon} onclick={primary.onclick} disabledReason={primary.disabledReason} data-tour={primary.tour} data-opener={primary.opener}>
            {primary.label}
          </Button>
          {#if primary.menu && primary.menu.items.length > 0}
            {@const submenu = primary.menu}
            <span class="primary-more">
              <Menu label={submenu.label} items={submenu.items} placement={{ side: 'block-end', align: 'end' }} onselect={submenu.onselect}>
                {#snippet trigger(props)}
                  <IconButton icon="chevron-down" label={submenu.label} tone="primary" {...props} />
                {/snippet}
              </Menu>
            </span>
          {/if}
        </span>
      {/if}
      {#each secondary as action, index (index)}
        <span class="secondary">
          <Button tone="secondary" icon={action.icon} onclick={action.onclick} disabledReason={action.disabledReason} data-tour={action.tour} data-opener={action.opener}>
            {action.label}
          </Button>
        </span>
      {/each}
      {#if menu && menu.items.length > 0 && !isMenuInBar}
        <span class="menu menu-full">
          <Menu label={menu.label} items={menu.items} placement={{ side: 'block-end', align: 'end' }} onselect={menu.onselect}>
            {#snippet trigger(props)}
              <IconButton icon="ellipsis" label={menu.label} tone="secondary" data-opener={isFolded ? undefined : menu.opener} {...props} />
            {/snippet}
          </Menu>
        </span>
      {/if}
      {#if foldedItems.length > 0 && !isMenuInBar}
        <span class="menu menu-folded">
          <Menu label={menu?.label ?? secondary[0]?.label ?? ''} items={foldedItems} placement={{ side: 'block-end', align: 'end' }} onselect={onFoldedSelect}>
            {#snippet trigger(props)}
              <IconButton icon="ellipsis" label={menu?.label ?? secondary[0]?.label ?? ''} tone="secondary" data-opener={isFolded ? menu?.opener : undefined} {...props} />
            {/snippet}
          </Menu>
        </span>
      {/if}
    </div>
{/snippet}

<header class="head" data-folded={isFolded ? '' : undefined} data-title-in-bar={isTitleInBar ? '' : undefined} bind:this={head}>
  {#if breadcrumb}
    <div class="top">
      <div class="trail">
        <Breadcrumb label={breadcrumb.label} trail={breadcrumb.trail} expandLabel={breadcrumb.expandLabel} onnavigate={breadcrumb.onnavigate} />
      </div>
      {#if parent}
        <!-- The way up, on a phone: one link, the parent, with the chevron pointing along the
             reading direction's start. It mirrors in RTL because the icon set mirrors it. -->
        <p class="parent">
          <a href={parent.href} onclick={(event) => { if (breadcrumb.onnavigate) { event.preventDefault(); breadcrumb.onnavigate(parent.id); } }}>
            <Icon name="chevron-left" size="sm" />
            <span>{parent.label}</span>
          </a>
        </p>
      {/if}
      <!-- A title that is drawn elsewhere - in the bar, or by the caller as a field - leaves the
           title row nothing to show beside the actions; the actions join the trail's line
           instead of standing on an empty one. -->
      {#if actionsOnTrail}{@render actionBar()}{/if}
    </div>
  {/if}

  <div class="row" data-empty={actionsOnTrail ? '' : undefined}>
    <div class="titles">
      {#if isTitleInBar}
        <VisuallyHidden as="h1">{title}</VisuallyHidden>
      {:else}
        <h1 class="title">{title}</h1>
      {/if}
      {#if subtitle}<p class="subtitle">{subtitle}</p>{/if}
    </div>
    {#if !actionsOnTrail}{@render actionBar()}{/if}
  </div>

  {#if notices}
    <div class="notices">{@render notices()}</div>
  {/if}

  {#if views}
    <div class="views">{@render views()}</div>
  {/if}
</header>

<style>
  .head {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
    min-width: 0;
  }

  .row {
    display: flex;
    align-items: flex-start;
    gap: var(--sp-200);
    flex-wrap: wrap;
  }

  /* The trail's line, which carries the actions when the title row would stand empty. */
  .top {
    display: flex;
    align-items: center;
    gap: var(--sp-200);
    min-width: 0;
  }

  .top .trail { flex: 1; min-width: 0; }

  /* A title row that holds only the read heading takes no room of its own. */
  .row[data-empty] { display: contents; }

  .titles {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    flex: 1;
    min-width: 0;
  }

  .title {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-500);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
    text-wrap: balance;
  }

  .subtitle {
    margin: 0;
    max-width: 72ch;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    flex: none;
    margin-inline-start: auto;
  }

  /* The split button: the verb and its list read as one control, joined at the seam. The two
     halves keep their own focus rings; only the corners between them are squared. */
  .primary[data-split] { display: inline-flex; align-items: stretch; }

  .primary[data-split] > :global(.button) {
    border-start-end-radius: 0;
    border-end-end-radius: 0;
  }

  .primary-more { display: inline-flex; margin-inline-start: var(--bw-hairline); }

  .primary-more :global(.icon-button) {
    border-start-start-radius: 0;
    border-end-start-radius: 0;
  }

  .parent { display: none; margin: 0; }

  .parent a {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    text-decoration: none;
  }

  .parent a:hover { color: var(--text-primary); }

  .parent a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  .notices { display: flex; flex-direction: column; gap: var(--sp-100); }

  .views { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); }

  /* Two menus are declared and one is shown: the wide one holds the caller's items, the narrow
     one those and the secondary actions folded in. Neither is duplicated in the accessibility
     tree, because `display: none` removes the hidden one from it. */
  .menu-folded { display: none; }

  /* Narrower than `medium`, the head folds (ADR-0061): the parent alone as the way up, one
     control for everything that is not the primary, and the primary where the thumb is. */
  .head[data-folded] .trail { display: none; }
  .head[data-folded] .parent { display: block; flex: 1; min-width: 0; }
  .head[data-folded] .secondary,
  .head[data-folded] .primary-more,
  .head[data-folded] .menu-full { display: none; }

  /* Folded, the button stands alone again and gets its corners back. */
  .head[data-folded] .primary[data-split] > :global(.button) {
    border-start-end-radius: var(--r-md);
    border-end-end-radius: var(--r-md);
  }
  .head[data-folded] .menu-folded { display: inline-flex; }

  /* The one primary action, pinned at the end of the screen above the bottom bar and the safe
     area. `position: fixed` puts it in the viewport, which is what a control at the bottom of a
     scrolling page needs; `raised` because it sits over content and under every overlay. */
  .head[data-folded] .primary {
    position: fixed;
    inset-inline-end: var(--sp-200);
    inset-block-end: calc(var(--layout-bottombar-height) + var(--sp-200) + env(safe-area-inset-bottom, 0));
    z-index: var(--z-raised);
    box-shadow: var(--shadow-overlay);
    border-radius: var(--r-xl);
  }
</style>
