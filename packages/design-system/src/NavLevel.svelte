<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  import type { IconName } from './icons/index.ts';
  import type { RowAction } from './NavItem.svelte';

  /** The hub level's head (the navigation concept's §2.4, item 1). */
  export interface HubHead {
    /** Resolved text: the hub's name. */
    readonly name: string;
    readonly icon: IconName;
    readonly color?: string;
    /** *‹ All hubs*, back to the workspace level. */
    readonly backLabel: string;
    readonly backHref?: string;
    readonly onback?: () => void;
    /** ☆: its words say what pressing does, "Star Product development". */
    readonly starLabel: string;
    readonly isStarred?: boolean;
    readonly onstar?: () => void;
    /** ⋯: the hub's own menu, its settings among it (`placeMenu.ts`). */
    readonly menu?: RowAction;
  }
</script>

<script lang="ts">
  // The scrolling middle of the sidebar, one level at a time (design-system.md §4, wave 5; the
  // navigation concept's §2.1, §2.4, §6.1): the workspace level, or a hub level with its head -
  // *‹ All hubs*, the hub's tile and name with the quick switch, ☆ and ⋯ - and its tools above
  // the parts. A new level slides in for `motion.reveal`, none under reduced motion, and focus
  // lands on *‹ All hubs*. Its scrollbar is the shared thin one: shown while hovered, its track
  // then drawn, its room reserved, so nothing shifts when it appears (`scrollbar.css`).
  import { tick, type Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import Menu from './Menu.svelte';
  import ProductTile from './ProductTile.svelte';

  interface Props {
    /** Which level is shown - `workspace`, or the hub's id. A change slides the new one in. */
    levelKey: string;
    /** What the region is called for a screen reader: the level's name. */
    label: string;
    hub?: HubHead;
    /** A level that is not a hub - the administration's, Your settings' - has the way back alone. */
    back?: { readonly label: string; readonly href?: string; readonly onback?: () => void };
    /** The quick switch beside the hub's name, usually a `NavFlyout`; given the head to drop from. */
    switcher?: Snippet<[HTMLElement | null]>;
    /** *Overview* and the hub tools, above a hairline. */
    tools?: Snippet;
    children: Snippet;
  }

  const { levelKey, label, hub, back: way, switcher, tools, children }: Props = $props();

  const out = $derived(hub ? { label: hub.backLabel, href: hub.backHref, onback: hub.onback } : way);

  let back = $state<HTMLElement | null>(null);
  let head = $state<HTMLElement | null>(null);
  let isMenuOpen = $state(false);
  let shown: string | undefined;

  // Focus follows a change of level, never the first drawing: a page that opens on a hub does not
  // take the focus from where the reader is.
  $effect(() => {
    const key = levelKey;
    if (shown !== undefined && shown !== key) void tick().then(() => back?.focus());
    shown = key;
  });
</script>

<div class="level" role="region" aria-label={label} data-nav-scroll data-scrollbar="thin">
  {#key levelKey}
    <div class="slide">
      {#if out}
        <div class="back-row">
          <!-- An anchor or a button: both are interactive, the linter only cannot tell from the tag. -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <svelte:element
            this={out.href ? 'a' : 'button'}
            bind:this={back}
            class="back"
            type={out.href ? undefined : 'button'}
            href={out.href}
            onclick={(event: MouseEvent) => {
              if (!out.onback || event.metaKey || event.ctrlKey || event.shiftKey) return;
              event.preventDefault();
              out.onback();
            }}
          >
            <Icon name="chevron-left" size="sm" />{out.label}
          </svelte:element>
        </div>
      {/if}
      {#if hub}
        <div class="hub" bind:this={head}>
          <ProductTile icon={hub.icon} color={hub.color} />
          <h2 class="hub-name" title={hub.name}>{hub.name}</h2>
          {#if switcher}{@render switcher(head)}{/if}
          <button
            type="button"
            class="hub-act"
            data-on={hub.isStarred ? '' : undefined}
            aria-pressed={hub.isStarred ?? false}
            aria-label={hub.starLabel}
            title={hub.starLabel}
            onclick={() => hub.onstar?.()}
          >
            <Icon name="star" size="sm" />
          </button>
          {#if hub.menu}
            {@const menu = hub.menu}
            {#if menu.items}
              <Menu label={menu.label} items={menu.items} placement={{ side: 'block-end', align: 'end' }} bind:isOpen={isMenuOpen} onselect={(id) => menu.onselect(id)}>
                {#snippet trigger(props)}
                  <button type="button" class="hub-act" aria-label={menu.label} title={menu.label} {...props}><Icon name="ellipsis" size="sm" /></button>
                {/snippet}
              </Menu>
            {:else}
              <button type="button" class="hub-act" aria-label={menu.label} title={menu.label} onclick={() => menu.onselect()}><Icon name="ellipsis" size="sm" /></button>
            {/if}
          {/if}
        </div>
      {/if}
      {#if tools}
        <ul class="tools">{@render tools()}</ul>
      {/if}
      {@render children()}
    </div>
  {/key}
</div>

<style>
  /* Its scrollbar is the shared thin one (`scrollbar.css`). */
  .level {
    flex: 1;
    min-block-size: 0;
    overflow: auto;
    padding: calc(var(--sp-050) + var(--sp-025)) var(--sp-100) calc(var(--sp-100) + var(--sp-025));
  }

  .slide {
    --slide-from: var(--sp-200);
    animation: arrive var(--motion-reveal-duration) var(--motion-reveal-easing);
  }

  :global([dir='rtl']) .slide { --slide-from: calc(-1 * var(--sp-200)); }

  @keyframes arrive {
    from { opacity: 0.3; transform: translateX(var(--slide-from)); }
    to { opacity: 1; transform: none; }
  }

  .back-row {
    display: flex;
    align-items: center;
    padding: var(--sp-100) calc(var(--sp-100) + var(--sp-025));
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .back {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-025);
    margin: 0;
    padding: calc(var(--sp-025) + var(--bw-hairline)) var(--sp-050);
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    text-decoration: none;
    cursor: pointer;
  }

  .back:hover { background: var(--bg-surface-hover); }

  .back :global(.icon),
  .hub-act :global(.icon) {
    inline-size: var(--layout-nav-actionglyph);
    block-size: var(--layout-nav-actionglyph);
  }

  /* The head's marks sit close, as a row's actions do, so the name keeps the room between them. */
  .hub {
    display: flex;
    align-items: center;
    gap: var(--sp-025);
    padding: var(--sp-100) calc(var(--sp-100) + var(--sp-025)) var(--sp-050);
  }

  /* Two lines at most, then an ellipsis: a long name wraps between its words, and nothing runs
     under the switcher, ☆ and ⋯ at the narrowest sidebar. A word longer than the room ends in the
     ellipsis rather than breaking or being hyphenated: a name states no language, and the
     document's - the reader's - would hyphenate a German word by English rules (i18n-l10n.md §6).
     The full words are the title. */
  .hub-name {
    flex: 1;
    min-inline-size: 0;
    margin: 0;
    margin-inline: calc(var(--sp-100) - var(--sp-025)) var(--sp-050);
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    text-overflow: ellipsis;
    font-family: var(--font-ui);
    font-size: var(--fs-100);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-snug);
    color: var(--text-primary);
  }

  .hub-act {
    display: grid;
    place-items: center;
    flex: none;
    inline-size: var(--layout-nav-action);
    block-size: var(--layout-nav-action);
    margin: 0;
    padding: 0;
    border: 0;
    border-radius: var(--r-xs);
    background: none;
    color: var(--text-subtle);
    cursor: pointer;
  }

  .hub-act:hover { background: var(--bg-surface-hover); }
  .hub-act[data-on] { color: var(--accent-star); }
  .hub-act[data-on] :global(.icon) { fill: currentColor; }

  .back:focus-visible,
  .hub-act:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .tools {
    margin: 0 0 var(--sp-025);
    padding: var(--sp-025) 0 calc(var(--sp-050) + var(--sp-025));
    list-style: none;
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  @media (prefers-reduced-motion: reduce) {
    .slide { animation: none; }
  }

  :global([data-motion='reduced']) .slide { animation: none; }
</style>
