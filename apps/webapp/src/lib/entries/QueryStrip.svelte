<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What is seen, over a list on a phone (§6.11, §7, scenes qline 5 and board 4): the first chips in
  // one strip with "+n" and the count, and the filters in a sheet - a row per condition, *Reset*,
  // *Show n entries*, and the way on to the panel *View*. The sheet opens from *Filter* on the
  // view chip's line. The same line the panel edits; a change applies at once, so the count the
  // sheet's act names is the server's for what is typed.
  import { untrack, type Snippet } from 'svelte';
  import { QueryLine, lineText, type QueryToken } from '@hubtask/design-system/components';
  import { createCatalogue, relabelled, type CountedValue, type QueryKey } from '@hubtask/query';

  import { t } from '../i18n/i18n.svelte.ts';
  import { language, wording } from './viewLanguage.ts';

  interface Props {
    line: string;
    keys: readonly QueryKey[];
    /** How many entries the line keeps, counted by the server for the reader. */
    total?: number;
    values?: (key: string, prefix: string) => readonly CountedValue[];
    ontyping?: (key: string | undefined) => void;
    onchange: (line: string) => void;
    isSheetOpen?: boolean;
    /** Under the sheet's conditions: the way on to the panel. */
    sheetEnd?: Snippet;
  }

  let { line, keys, total, values, ontyping, onchange, isSheetOpen = $bindable(false), sheetEnd }: Props = $props();

  const catalogue = $derived(createCatalogue(keys, wording, values));
  let tokens = $state<readonly QueryToken[]>([]);
  let text = $state('');
  let spelled: string | undefined;
  $effect(() => {
    const wanted = line;
    // The cut reads the reader's words, so a catalogue that arrives later runs this again.
    const fresh = language.parse(`${wanted} `).tokens;
    if (wanted === spelled) {
      const renamed = relabelled(untrack(() => tokens), fresh);
      if (renamed) tokens = renamed;
      return;
    }
    tokens = fresh;
    text = '';
    spelled = wanted;
  });
  function changed(next: readonly QueryToken[]) {
    spelled = lineText(language, next, '');
    onchange(spelled);
  }
  $effect(() => ontyping?.(/^-?([\w.]+):/u.exec(text.trimStart())?.[1]));

  const phone = $derived({
    open: t('app.viewcfg.phone_open'),
    title: t('app.viewcfg.phone_title'),
    close: t('app.dismiss'),
    reset: t('app.viewcfg.phone_reset'),
    show: typeof total === 'number' ? t('app.viewcfg.phone_show', { count: String(total) }) : t('app.viewcfg.phone_show_some'),
    more: (hidden: number) => t('app.viewcfg.phone_more', { count: String(hidden) }),
  });
</script>

<div class="query-strip" data-query-strip>
  <QueryLine
    label={t('app.viewcfg.query')}
    {language}
    {catalogue}
    bind:tokens
    bind:text
    placeholder={t('app.viewcfg.query_placeholder')}
    suggestionsLabel={t('app.viewcfg.suggestions')}
    fieldsHeading={(typed) => t('app.viewcfg.fields_heading', { typed })}
    valuesHeading={(field) => t('app.viewcfg.values_heading', { field })}
    editLabel={(spelling) => t('app.viewcfg.edit_chip', { spelling })}
    removeLabel={(spelling) => t('app.viewcfg.remove_chip', { spelling })}
    onchange={changed}
    addConditionLabel={t('app.viewcfg.add_condition')}
    addGroupLabel={t('app.viewcfg.add_group')}
    hint={t('app.viewcfg.chip_hint')}
    {phone}
    bind:isSheetOpen
    hasPhoneButton={false}
    {sheetEnd}
    count={typeof total === 'number' && tokens.length > 0 ? count : undefined}
  />
</div>

{#snippet count()}<span data-view-count>{t('app.viewcfg.count', { count: String(total) })}</span>{/snippet}

<style>
  .query-strip { min-width: 0; flex: 1 1 100%; }
</style>
