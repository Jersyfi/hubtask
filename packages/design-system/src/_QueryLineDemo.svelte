<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { untrack } from 'svelte';

  import Kbd from './Kbd.svelte';
  import QueryLine from './QueryLine.svelte';
  import { QUERY_CATALOGUE, STUB_LANGUAGE, initialQuery } from './_workFixtures.ts';
  import { lineText, type QueryToken } from './query.ts';

  interface Props {
    mode?: 'filled' | 'typing' | 'empty' | 'error';
  }

  const { mode = 'filled' }: Props = $props();

  const start = (): QueryToken[] => {
    if (mode === 'empty') return [];
    if (mode === 'error') return STUB_LANGUAGE.parse('type: incident prio: P1 ').tokens.map((token) => token);
    return initialQuery();
  };
  let tokens = $state<readonly QueryToken[]>(untrack(start));
  let text = $state(untrack(() => (mode === 'typing' ? 'prio' : '')));
  const matches = $derived(Math.max(0, 7 - tokens.length));
</script>

<div class="column">
  <QueryLine
    label="Filter"
    language={STUB_LANGUAGE}
    catalogue={QUERY_CATALOGUE}
    bind:tokens
    bind:text
    placeholder="Type a field, a value or words"
    suggestionsLabel="Suggestions"
    fieldsHeading={(typed) => `Fields · “${typed}”`}
    valuesHeading={(field) => `Values for ${field}`}
    editLabel={(spelling) => `Edit ${spelling}`}
    removeLabel={(spelling) => `Remove ${spelling}`}
    addConditionLabel="Condition"
    addGroupLabel="Group (or)"
    hint="A press on a chip edits it · typing goes on · nothing is lost"
    phone={{ open: 'Change filter', title: 'Filter', close: 'Close', reset: 'Reset', show: `Show ${matches} entries`, more: (hidden) => `+${hidden}` }}
  >
    {#snippet count()}<b>{matches} entries</b> · the count is there while typing{/snippet}
    {#snippet keys()}<span><Kbd>↑↓</Kbd> choose</span><span><Kbd>↵</Kbd> take</span><span><Kbd>,</Kbd> or</span><span><Kbd>-</Kbd> not</span>{/snippet}
  </QueryLine>
  <div class="below" aria-hidden="true"><span></span><span></span><span></span></div>
  <p class="text"><span>The same query as text</span><code>{lineText(STUB_LANGUAGE, tokens, text)}</code></p>
</div>

<style>
  .column { display: flex; flex-direction: column; gap: var(--sp-300); min-block-size: var(--sp-1600); }
  /* Rows under the line, so the story shows the suggestions floating over what follows. */
  .below { display: flex; flex-direction: column; border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); overflow: hidden; }
  .below span { block-size: var(--density-row-height); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .text { display: flex; flex-direction: column; gap: var(--sp-050); margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  code { color: var(--text-primary); font-family: var(--font-mono); font-size: var(--fs-075); }
</style>
