<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entry to search, in the bar from `medium` up (ADR-0063 decision 4, ADR-0066 decision 4).
  //
  // **It used to search nothing.** It took words and navigated, and the note here said why: the
  // debounce, the language and the widening belonged in one place. Two of those three were the
  // language's, and the language is gone (ADR-0066 decision 1) — so what the economy still buys is
  // one debounce, and what it costs is that the commonest search in the product, "where is that
  // one thing", needs a screen change to answer. It answers here now.
  //
  // **The words still travel in memory, not in the address.** `/search` is a `POST` with no `GET`
  // precisely so that a term never reaches an access log, a proxy or browser history
  // (`security.md` §9, `api-guidelines.md` §2). So the words are handed to the store and the
  // screen takes them; the address carries only the narrowing, which is structural rather than
  // content — `?f=is:open who:me` is a link somebody may send, and it says nothing about anybody.
  //
  // The menu itself, and what its arrow keys walk, are `search/SearchMenu.svelte` and
  // `search/menurows.ts`.

  import { SearchField } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import SearchMenu from '../search/SearchMenu.svelte';
  import { rowsOf, step } from '../search/menurows.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { queryFields } from '../data/query.ts';
  import { search } from '../data/search.svelte.ts';
  import { QUICK, compile, parse } from '../data/searchquery.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Where the bar sends the reader. The frame owns the router; this does not. */
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let term = $state('');
  let open = $state(false);
  let active = $state(-1);
  let hits = $state<readonly WorkItem[]>([]);
  let searching = $state(false);

  /** Only the narrowings this installation can answer — the rule every filter surface here keeps. */
  const fields = $derived(new Set(queryFields(manifest.value).map((field) => field.field)));
  const quick = $derived(QUICK.filter((each) => fields.has(each.field)));

  const rows = $derived(rowsOf(term, hits.map((hit) => hit.id), quick));

  /**
   * The first few hits, a beat after the typing stops.
   *
   * A generation, for `run`'s reason: the answer to "mil" can arrive after the answer to "milk",
   * and a menu that showed it would look exactly like one that ignores what was typed last.
   */
  let generation = 0;

  $effect(() => {
    const words = term.trim();
    if (!open || words === '') {
      hits = [];
      searching = false;
      return;
    }
    searching = true;
    const mine = (generation += 1);
    const timer = setTimeout(() => {
      void search
        .peek({ q: words }, 5)
        .then((found) => {
          if (mine !== generation) return;
          hits = found;
          searching = false;
        })
        .catch(() => {
          // A menu is an offer, not an answer: a failed peek shows no hits and the way on still
          // works. The screen it leads to is where a failure is reported, with its reference.
          if (mine !== generation) return;
          hits = [];
          searching = false;
        });
    }, 200);
    return () => clearTimeout(timer);
  });

  // A row highlighted by the arrows cannot survive the list changing under it: the third hit for
  // "mil" is not the third hit for "milk".
  $effect(() => {
    void term;
    active = -1;
  });

  /** Hands the words over and goes to the screen, optionally already narrowed. */
  function go(narrowing: string, openEditor = false) {
    const words = term.trim();
    if (words !== '') search.handOver(words);
    const carried = new URLSearchParams({
      ...(narrowing === '' ? {} : { f: narrowing }),
      ...(openEditor ? { edit: '1' } : {}),
    }).toString();
    // Emptied as it is handed over: the screen has a field of its own holding these words, and two
    // fields showing one question are two places to edit it — and they disagree the moment
    // somebody edits the lower one.
    term = '';
    open = false;
    active = -1;
    onnavigate(carried === '' ? '/search' : `/search?${carried}`);
  }

  function pick(index: number) {
    const row = rows[index];
    if (!row) return;
    if (row.kind === 'hit') {
      term = '';
      open = false;
      onnavigate(`/items/${row.id}`);
      return;
    }
    if (row.kind === 'quick') {
      go(row.id);
      return;
    }
    go('', row.kind === 'refine');
  }

  function submit(event: SubmitEvent) {
    event.preventDefault();
    // A highlighted row is what Enter presses; nothing highlighted means the words themselves,
    // which is the state a field somebody is typing in is always in (`menurows.ts`).
    if (active >= 0) {
      pick(active);
      return;
    }
    // And with nothing typed it means the row the menu draws the key on: "Open search". Enter used
    // to only open the menu here, which is the one thing it cannot mean - the menu is already open,
    // that is where the reader read the word "Enter".
    go('');
  }

  /**
   * The keys, caught on the way **down** to the field rather than on the field.
   *
   * Escape has to mean two things in order — close the menu, then empty the field — and the first
   * attempt at that did both at once. The cause was not `SearchField`'s own Escape handler, which
   * is what it looked like: **the browser empties a `type="search"` input on Escape by itself**.
   * `stopPropagation` does nothing about that, because it is not a listener; only
   * `preventDefault` is. Two hours of the right answer to the wrong question, found in one press
   * by the keyboard walk and by nothing else — no reading test can see it, because nothing in the
   * document changes.
   *
   * The capture phase is still right, for the other half: it runs before the field's handler, so
   * an Escape spent on the menu never reaches it, while one with the menu closed travels on and
   * empties the field — which is behaviour the field already has and this has no business
   * changing.
   */
  function keydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      if (!open) open = true;
      event.preventDefault();
      active = step(active, rows.length, event.key === 'ArrowDown' ? 1 : -1);
      return;
    }
    if (event.key !== 'Escape' || !open) return;
    open = false;
    active = -1;
    event.preventDefault();
    event.stopPropagation();
  }

  /** Closing on focus leaving the whole thing, rather than on the field's own blur: a press on a
      row in the menu is a blur, and a menu that closed on it would close before the press landed. */
  function focusout(event: FocusEvent) {
    const next = event.relatedTarget;
    if (next instanceof Node && event.currentTarget instanceof Node && event.currentTarget.contains(next)) return;
    open = false;
    active = -1;
  }

  /** What the compiled quick filters would ask, shown to nobody — it is here so that a line this
      installation cannot answer is never offered. The menu uses the list; this checks it. */
  const context = $derived({
    fields,
    collection: () => undefined,
    label: () => undefined,
    hasCollection: false,
  });

  const offered = $derived(
    quick.filter((each) => compile(parse(each.line), context).problems.length === 0),
  );
</script>

<!-- A form, so that Enter submits the way every search field on the web does, and the landmark a
     screen reader looks for when it is asked to find the search. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<form
  class="entry"
  role="search"
  aria-label={t('app.search.title')}
  onsubmit={submit}
  onfocusout={focusout}
  onkeydowncapture={keydown}
>
  <!-- Its own name, not the search screen's. Both fields carry `app.search.label` until one of
       them is renamed, which puts two controls with one accessible name on the same screen - a
       screen reader asked to find "the search" then finds two and can say nothing about which. -->
  <SearchField
    label={t('app.searchmenu.field')}
    isLabelHidden
    clearLabel={t('app.search.clear')}
    placeholder={t('app.nav.search')}
    size="sm"
    bind:value={term}
    onfocus={() => (open = true)}
    oninput={() => (open = true)}
    role="combobox"
    aria-expanded={open}
    aria-controls={open ? 'search-menu' : undefined}
    aria-activedescendant={open && active >= 0 ? `search-row-${active}` : undefined}
    autocomplete="off"
  />
  <!-- Typing reopens the menu. Escape closes it without leaving the field, and a reader who then
       types has asked the question again - a field that stayed silent after that would be one
       where Escape switches the menu off for good. -->
  {#if open}
    <SearchMenu
      {term}
      {hits}
      {searching}
      quick={offered}
      {active}
      onhover={(index) => (active = index)}
      onpick={pick}
    />
  {/if}
</form>

<style>
  /* The field takes the slot whole; the slot is what is bounded, in `AppBar`. The menu is
     positioned against this, which is why it is the containing block. */
  .entry { position: relative; display: block; min-inline-size: 0; }
</style>
