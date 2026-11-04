<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one settings shell (user-experience.md §5.4): the screen's sections at the start, the
  // sections beside them, a save bar while something is unsaved, and the leave guard that asks
  // before a move drops it. The caller draws its own head above it and its sections inside it,
  // each a `section` with the id the list gives it.
  //
  // **The sections are links into one page, not tabs.** Every section stays in the document, so
  // the browser's find, a reader's scroll and a test's locator reach all of them; the list marks
  // the one in view. One section needs no list, so none is drawn.
  //
  // **Saving belongs to the screen.** Only a screen whose writes wait for *Save* passes `save`; a
  // screen of instant acts (a switch, a card's own button) has no bar, and its `changes` count
  // what is typed in a form not yet sent, so the guard still asks before that is dropped.

  import type { Snippet } from 'svelte';

  import { SaveBar } from '@hubtask/design-system/components';

  import { leaveGuard } from '../frame/leave.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Section {
    id: string;
    /** What the section is called in the list: its heading's word. Resolved text. */
    label: string;
  }

  interface Save {
    label: string;
    savingLabel: string;
    isSaving: boolean;
    onSave: () => void;
    onDiscard: () => void;
    /** What the change reaches (configuration/ui.md A3), where the screen knows it. */
    reachLabel?: string;
    planLabel?: string;
    onPlan?: () => void;
  }

  interface Props {
    /** The list's landmark name: "Sections of E-mail". Resolved text. */
    label: string;
    sections: readonly Section[];
    /** How many changes are not saved yet; zero, or absent, means none. */
    changes?: number;
    /** The save bar, for a screen whose writes wait for *Save*. */
    save?: Save;
    children: Snippet;
  }

  const { label, sections, changes = 0, save, children }: Props = $props();

  $effect(() => leaveGuard.hold(() => changes));

  let content = $state<HTMLElement | undefined>(undefined);
  let current = $state<string | undefined>(undefined);
  const shown = $derived(current ?? sections[0]?.id);

  // The section in view: the first whose top has passed under the start of the page, else the
  // first. An observer, so a long screen is not measured on every scroll event.
  $effect(() => {
    const root = content;
    if (!root || sections.length < 2 || typeof IntersectionObserver === 'undefined') return;
    const seen = new Map<string, boolean>();
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) seen.set(entry.target.id, entry.isIntersecting);
      const first = sections.find((section) => seen.get(section.id));
      if (first) current = first.id;
    });
    for (const section of sections) {
      const element = root.querySelector(`#${CSS.escape(section.id)}`);
      if (element) observer.observe(element);
    }
    return () => observer.disconnect();
  });

  function go(event: MouseEvent, id: string): void {
    const target = content?.querySelector<HTMLElement>(`#${CSS.escape(id)}`);
    if (!target) return;
    // The router takes every same-origin link; a jump within the page is the page's own.
    event.preventDefault();
    current = id;
    const isReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    target.scrollIntoView({ block: 'start', behavior: isReduced ? 'auto' : 'smooth' });
    // Focus follows, so the keyboard continues in the section rather than in the list.
    if (!target.hasAttribute('tabindex')) target.setAttribute('tabindex', '-1');
    target.focus({ preventScroll: true });
  }
</script>

<div class="shell" data-settings-page class:compact={viewport.isCompact} class:single={sections.length < 2}>
  {#if sections.length >= 2}
    <nav class="sections" aria-label={label} data-settings-sections>
      <ul>
        {#each sections as section (section.id)}
          <li>
            <a href="#{section.id}" aria-current={shown === section.id ? 'location' : undefined} onclick={(event) => go(event, section.id)}>{section.label}</a>
          </li>
        {/each}
      </ul>
    </nav>
  {/if}
  <div class="content" bind:this={content}>
    {@render children()}
    {#if save}
      <SaveBar
        label={t('app.save_bar.label')}
        changesLabel={changes > 0 ? t('app.save_bar.changes', { count: String(changes) }) : undefined}
        discardLabel={t('app.save_bar.discard')}
        saveLabel={save.label}
        isSaving={save.isSaving}
        savingLabel={save.savingLabel}
        onDiscard={save.onDiscard}
        onSave={save.onSave}
        reachLabel={save.reachLabel}
        planLabel={save.planLabel}
        onPlan={save.onPlan}
      />
    {/if}
  </div>
</div>

<style>
  .shell {
    display: grid;
    grid-template-columns: minmax(0, calc(var(--sp-1000) * 2.5)) minmax(0, 1fr);
    gap: var(--sp-300);
    align-items: start;
  }

  .shell.single { grid-template-columns: minmax(0, 1fr); }

  /* On a phone the list is a row above the sections that scrolls on its own, so the page never
     scrolls sideways (user-experience.md §5.12). */
  .shell.compact { grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); }

  .content { display: flex; flex-direction: column; gap: var(--sp-400); min-inline-size: 0; }

  .sections { position: sticky; inset-block-start: var(--sp-200); }

  .compact .sections { position: static; overflow-x: auto; }

  ul { display: flex; flex-direction: column; gap: var(--sp-025); margin: 0; padding: 0; list-style: none; }

  .compact ul { flex-direction: row; gap: var(--sp-050); }

  a {
    display: block;
    padding: var(--sp-050) var(--sp-150);
    border-radius: var(--r-md);
    color: var(--text-secondary);
    text-decoration: none;
  }

  .compact a { white-space: nowrap; }

  a:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  a[aria-current] { background: var(--bg-surface-pressed); color: var(--text-primary); font-weight: var(--fw-medium); }

  .content :global(section[id]) { scroll-margin-block-start: var(--sp-200); }

  .content :global(section[id]:focus) { outline: none; }
</style>
