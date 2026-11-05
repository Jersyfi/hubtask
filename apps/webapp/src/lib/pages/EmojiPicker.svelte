<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The page's emoji picker (fourth answers, 2): emojibase's set in the reader's language, one
  // group at a time or searched by its words, loaded the first time it opens.

  import { onMount } from 'svelte';
  import { t, messages } from '../i18n/i18n.svelte.ts';
  import { EMOJI_GROUPS, loadEmojis, searchEmojis, type PickerEmoji } from './emojis.ts';

  interface Props {
    onpick: (emoji: string) => void;
  }

  let { onpick }: Props = $props();

  let all = $state<PickerEmoji[] | undefined>(undefined);
  let failed = $state(false);
  let query = $state('');
  let group = $state<number>(EMOJI_GROUPS[0].order);

  onMount(() => {
    loadEmojis(messages.locale).then((set) => (all = set), () => (failed = true));
  });

  const shown = $derived(
    !all ? [] : query.trim() !== '' ? searchEmojis(all, query, messages.locale) : all.filter((each) => each.group === group),
  );
</script>

<div class="picker" data-emoji-picker>
  <input type="search" bind:value={query} placeholder={t('app.pages.emoji_search')} aria-label={t('app.pages.emoji_search')} />
  {#if failed}
    <p class="quiet" role="alert">{t('app.pages.emoji_failed')}</p>
  {:else if !all}
    <p class="quiet" aria-live="polite">{t('app.pages.emoji_loading')}</p>
  {:else}
    {#if query.trim() === ''}
      <div class="groups" role="group" aria-label={t('app.pages.tool_emoji')}>
        {#each EMOJI_GROUPS as each (each.order)}
          <button type="button" class="group" aria-pressed={group === each.order} onclick={() => (group = each.order)}>{t(`app.pages.emoji_group_${each.key}`)}</button>
        {/each}
      </div>
    {/if}
    <ul class="grid" aria-label={t('app.pages.tool_emoji')}>
      {#each shown as each (each.unicode)}
        <li><button type="button" class="emoji" title={each.label} aria-label={each.label} onclick={() => onpick(each.unicode)}>{each.unicode}</button></li>
      {:else}
        <li class="quiet">{t('app.pages.emoji_none')}</li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .picker input {
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
  }
  .groups { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .group {
    padding: var(--sp-025) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-sm);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }
  .group[aria-pressed='true'] { color: var(--text-primary); background: var(--bg-surface-hover); border-color: var(--border-strong); }
  .grid {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-025);
    max-block-size: var(--sp-1000);
    overflow-y: auto;
  }
  .emoji {
    min-inline-size: var(--sp-400);
    min-block-size: var(--sp-400);
    border: none;
    border-radius: var(--r-sm);
    background: none;
    font-size: var(--fs-200);
    cursor: pointer;
  }
  .emoji:hover, .emoji:focus-visible { background: var(--bg-surface-hover); }
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
