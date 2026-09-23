<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entry to search, in the bar from `medium` up (ADR-0063 decision 4).
  //
  // **It is not a search.** It takes words and leads to the screen that searches; nothing here
  // sends a request, and the debounce, the language and the widening stay in one place. That is
  // also what keeps the rule ADR-0061 was protecting: one visible entry to search on every width -
  // this field from `medium` up, the bottom bar's destination below it, never both.
  //
  // **The words travel in memory, not in the address.** The decision says the bar navigates to
  // `/search?q=…`; this client does not, because `/search` is a `POST` with no `GET` precisely so
  // that a term never reaches an access log, a proxy or browser history (`security.md` §9,
  // `api-guidelines.md` §2). So the words are handed to the store and the screen takes them, and
  // the address carries only the narrowing, which is structural rather than content.

  import { SearchField } from '@hubtask/design-system/components';

  import { search } from '../data/search.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Where the bar sends the reader. The frame owns the router; this does not. */
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let term = $state('');

  function submit(event: SubmitEvent) {
    event.preventDefault();
    const words = term.trim();
    if (words === '') return;
    search.handOver(words);
    // Emptied as it is handed over: the screen it leads to has a field of its own holding these
    // words, and two fields showing one question are two places to edit it - and they disagree the
    // moment somebody edits the lower one.
    term = '';
    onnavigate('/search');
  }
</script>

<!-- A form, so that Enter submits the way every search field on the web does, and the landmark a
     screen reader looks for when it is asked to find the search. -->
<form class="entry" role="search" aria-label={t('app.search.title')} onsubmit={submit}>
  <SearchField
    label={t('app.search.label')}
    isLabelHidden
    clearLabel={t('app.search.clear')}
    placeholder={t('app.nav.search')}
    size="sm"
    bind:value={term}
  />
</form>

<style>
  /* The field takes the slot whole; the slot is what is bounded, in `AppBar`. */
  .entry { display: block; min-inline-size: 0; }
</style>
