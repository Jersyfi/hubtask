<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The index (ADR-0061 decision 5, F9-03): components in their waves, one row each.
  //
  // It groups by the segment before the slash in a story's title, which is how
  // `design-system.md` §4's waves reach the sidebar without anybody maintaining a second list -
  // and it lists the component, not its stories: those are the tabs above the stage. A wave is a
  // `<details>` and only the wave of the current component is open, so seventy-five rows are
  // nine lines until one is wanted. Which waves are open is the browser's to remember: a tool
  // may keep where one was, and nothing here is anybody's data.
  //
  // The filter above it is an `<input type="search">` with no form around it, no submit and
  // nothing kept - it narrows the list already on the page and does nothing else, which is what
  // `packages/design-system/CLAUDE.md` means by a control that is not a form.
  import SearchField from '../../src/SearchField.svelte';
  import { componentOf, filtered } from '../lib/index.ts';
  import type { StoryGroup } from '../lib/story.ts';

  interface Props {
    groups: readonly StoryGroup[];
    /** The selected story; the row of its component is current. */
    selected: string | null;
    /** Called with a story id: the first story of the chosen component. */
    onselect: (id: string) => void;
  }

  const { groups, selected, onselect }: Props = $props();

  let query = $state('');
  const shown = $derived(filtered(groups, query));
  const current = $derived(componentOf(groups, selected));

  const REMEMBERED = 'hubtask.workbench.open-waves';

  /** Which waves the reader opened, beyond the current one. Kept per browser, never elsewhere. */
  let opened = $state<string[]>(remember());

  function remember(): string[] {
    try {
      const raw = window.localStorage.getItem(REMEMBERED);
      return raw ? (JSON.parse(raw) as string[]) : [];
    } catch {
      return [];
    }
  }

  function toggle(wave: string, isOpen: boolean) {
    opened = isOpen ? [...new Set([...opened, wave])] : opened.filter((w) => w !== wave);
    try {
      window.localStorage.setItem(REMEMBERED, JSON.stringify(opened));
    } catch {
      // A private window or blocked storage: the list still works, it just forgets.
    }
  }

  // Open when the reader opened it, when it holds the current component, or when a query is
  // narrowing the list - a filter that hid its matches behind a closed wave would find nothing.
  const isOpen = (wave: string) =>
    opened.includes(wave) || wave === current?.group || query.trim() !== '';
</script>

<nav class="sidebar" aria-label="Components">
  <div class="filter">
    <SearchField
      label="Find a component or a story"
      isLabelHidden
      clearLabel="Clear the filter"
      size="sm"
      placeholder="Find…"
      bind:value={query}
      onclear={() => (query = '')}
    />
  </div>

  {#each shown as { wave, components } (wave)}
    <details class="wave" open={isOpen(wave)} ontoggle={(event) => toggle(wave, (event.currentTarget as HTMLDetailsElement).open)}>
      <summary>
        <span class="wave-name">{wave}</span>
        <span class="wave-count" aria-label={`${components.length} components`}>{components.length}</span>
      </summary>
      <ul>
        {#each components as component (component.meta.title)}
          <li>
            <button
              type="button"
              class="component"
              aria-current={component === current ? 'true' : undefined}
              onclick={() => onselect(component.stories[0]?.id ?? '')}
            >
              <span class="component-name">{component.title}</span>
              <!-- Rule 3: the status is the word, and the tint only repeats it. The count says
                   how many stories the tabs will offer. -->
              <span class="status" data-status={component.meta.status}>
                {component.meta.status} · {component.stories.length}
              </span>
            </button>
          </li>
        {/each}
      </ul>
    </details>
  {:else}
    <p class="empty">
      {#if query.trim()}
        Nothing matches “{query.trim()}”.
      {:else}
        No stories yet. <code>src/</code> stays empty until wave 1 builds it deliberately — see
        <code>src/README.md</code>.
      {/if}
    </p>
  {/each}
</nav>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    padding: var(--sp-200);
    min-width: 0;
  }

  .filter { margin-block-end: var(--sp-100); }

  .wave { border-block-end: 1px solid var(--border-subtle); }

  summary {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-050);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-subtle);
    cursor: pointer;
    list-style: none;
  }

  summary::-webkit-details-marker { display: none; }

  /* The twist is drawn, so its direction can follow the writing direction (rule: no physical
     side). A closed wave points along the line, an open one down. */
  summary::before {
    content: '';
    inline-size: 0.5em;
    block-size: 0.5em;
    border-inline-end: 2px solid currentColor;
    border-block-end: 2px solid currentColor;
    transform: rotate(-45deg);
    transition: transform var(--motion-state-duration) var(--motion-state-easing);
  }

  :global([dir='rtl']) summary::before { transform: rotate(135deg); }

  .wave[open] > summary::before { transform: rotate(45deg); }

  .wave-name { flex: 1; min-width: 0; }

  .wave-count { color: var(--text-subtle); letter-spacing: 0; }

  ul {
    margin: 0;
    padding: 0 0 var(--sp-100);
    list-style: none;
  }

  .component {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    width: 100%;
    padding: var(--sp-050) var(--sp-100);
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-secondary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .component:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  .component[aria-current='true'] {
    background: var(--accent-primary-subtle);
    color: var(--text-brand);
    font-weight: var(--fw-semibold);
  }

  .component-name { flex: 1; min-width: 0; overflow-wrap: break-word; }

  .status {
    flex: none;
    white-space: nowrap;
    padding: 0 var(--sp-050);
    border-radius: var(--r-sm);
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    letter-spacing: 0.06em;
    text-transform: uppercase;
    background: var(--label-slate-bg);
    color: var(--label-slate-fg);
  }

  .status[data-status='draft'] {
    background: var(--label-amber-bg);
    color: var(--label-amber-fg);
  }

  .status[data-status='stable'] {
    background: var(--label-green-bg);
    color: var(--label-green-fg);
  }

  .empty {
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  code { font-family: var(--font-mono); }

  @media (prefers-reduced-motion: reduce) {
    summary::before { transition: none; }
  }
</style>
