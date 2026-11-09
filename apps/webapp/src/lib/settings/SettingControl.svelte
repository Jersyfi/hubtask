<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One setting drawn from its catalogue declaration (configuration/ui.md A1, A2): the editor its
  // kind takes, its name, a count's bounds, its source chip with the why, and - where a level above
  // locked it or the reader's role does not write it here - the value unavailable with the reason.
  //
  // **The screen keeps its read and its write.** It hands in the value in force with the source and
  // lock its own read answered and the draft, and saves through its own use case, so the permission
  // and the version check stay the area's (P-18). What is drawn comes from the declaration, so the
  // same setting looks the same on every screen that draws it.

  import { Checkbox, Input, Radio, Select, SettingRow, Switch } from '@hubtask/design-system/components';
  import type { SettingDefinition } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { languageName } from '../i18n/locale.ts';
  import ValueSource from '../workspace/ValueSource.svelte';
  import { boundsWords, choiceWords, countFrom, editorOf, lockedWords, nameWords, shownValue, toggled, type Cell, type Words } from './control.ts';

  type Level = SettingDefinition['levels'][number];

  interface Props {
    definition: SettingDefinition;
    /** The value in force here, with its source and lock as the screen's read answered them. */
    cell: Cell | undefined;
    /** What the draft holds; undefined is the value in force. */
    value: unknown;
    /** The level the screen decides at. */
    level?: Level;
    /** Whether the reader holds a permission at the place the screen is drawn for. */
    holds?: (permission: string) => boolean;
    /** The languages and zones offered, for a locale's and a zone's list. */
    locales?: readonly string[];
    zones?: readonly string[];
    error?: string;
    onchange: (next: unknown) => void;
  }

  const {
    definition, cell, value, level = 'WORKSPACE', holds = (permission: string) => holdsHere(permission) === true,
    locales = [], zones = [], error, onchange,
  }: Props = $props();

  const say = (words: Words | undefined) => (words ? t(words.code, words.params ? { ...words.params } : undefined) : undefined);

  const editor = $derived(editorOf(definition));
  const label = $derived(say(nameWords(definition.key)) ?? definition.key);
  // A setting's own explanation, where its screen words one beside the name's code.
  const explanation = $derived.by(() => {
    const code = `${nameWords(definition.key).code}_hint`;
    return messages.has(code) ? t(code) : undefined;
  });
  const reason = $derived(say(lockedWords(definition, cell, level, holds)));
  const current = $derived(value === undefined ? cell?.value : value);
  const options = $derived(
    (definition.options ?? []).map((option) => {
      const words = choiceWords(definition.key, option);
      const hintCode = `${words.code}_hint`;
      return { value: option, label: say(words) ?? option, hint: !words.params && messages.has(hintCode) ? t(hintCode) : undefined };
    }),
  );
  const chosen = $derived(Array.isArray(current) ? (current as unknown[]).map(String) : []);
  const inForce = $derived(shownValue(definition, cell?.value, (code, params) => t(code, params)));
  // A few choices are all shown at once; a longer list folds into one field.
  const isFew = $derived(options.length > 0 && options.length <= 3);
  // A count says its bounds under the name, beside the explanation.
  const help = $derived(editor === 'count' ? [explanation, say(boundsWords(definition))].filter(Boolean).join(' ') || undefined : explanation);
  // As wide as what it holds (PS-13).
  const width = $derived<'xs' | 'sm' | 'md' | 'lg'>(
    editor === 'count' ? 'xs' : editor === 'flag' || editor === 'date' ? 'sm' : editor === 'choice' && !isFew ? 'md'
    : editor === 'locale' || editor === 'zone' ? 'md' : 'lg',
  );
</script>

<!-- A `SettingRow` as on every settings screen: the name, its source and its help at the start,
     the control at the end, labelled by the name and drawing none of its own. -->
<div class="setting-control" data-setting={definition.key}>
  <SettingRow {label} help={help} fieldWidth={width}>
    {#snippet source()}<ValueSource source={cell?.source} lock={cell?.locked_by} value={inForce} atHub={level === 'HUB'} />{/snippet}
    {#snippet control(nameId)}
      {#if editor === 'count'}
        <Input
          {label}
          isLabelHidden
          aria-labelledby={nameId}
          type="number"
          inputmode="numeric"
          min={definition.minimum}
          max={definition.maximum}
          value={current === undefined || current === null ? '' : String(current)}
          disabledReason={reason}
          oninput={(event) => onchange(countFrom((event.currentTarget as HTMLInputElement).value))}
          {error}
        />
      {:else if editor === 'flag'}
        <Switch
          {label}
          isLabelHidden
          aria-labelledby={nameId}
          checked={current === true}
          disabledReason={reason}
          onchange={(event) => onchange((event.currentTarget as HTMLInputElement).checked)}
        />
      {:else if editor === 'choice' && isFew}
        <!-- A radio group names itself; it repeats the row's name, as every settings row does. -->
        <Radio
          {label}
          {options}
          bind:value={() => (current === undefined || current === null ? '' : String(current)), (next) => onchange(next)}
          disabledReason={reason}
          {error}
        />
      {:else if editor === 'choice'}
        <Select
          {label}
          isLabelHidden
          aria-labelledby={nameId}
          value={current === undefined || current === null ? '' : String(current)}
          options={options.map(({ value: option, label: words }) => ({ value: option, label: words }))}
          disabledReason={reason}
          onchange={(event) => onchange((event.currentTarget as HTMLSelectElement).value)}
          {error}
        />
      {:else if editor === 'choices'}
        <div class="choices" role="group" aria-labelledby={nameId}>
          {#each options as option (option.value)}
            <Checkbox
              label={option.label}
              hint={option.hint}
              checked={chosen.includes(option.value)}
              disabledReason={reason}
              onchange={(event) => onchange(toggled(definition.options ?? [], chosen, option.value, (event.currentTarget as HTMLInputElement).checked))}
            />
          {/each}
          {#if error}<p class="refusal" role="alert">{error}</p>{/if}
        </div>
      {:else if editor === 'locale' && locales.length > 0}
        <Select
          {label}
          isLabelHidden
          aria-labelledby={nameId}
          value={String(current ?? '')}
          options={locales.map((each) => ({ value: each, label: languageName(each, messages.locale) }))}
          disabledReason={reason}
          onchange={(event) => onchange((event.currentTarget as HTMLSelectElement).value)}
          {error}
        />
      {:else if editor === 'zone' && zones.length > 0}
        <Select
          {label}
          isLabelHidden
          aria-labelledby={nameId}
          value={String(current ?? '')}
          options={zones.map((each) => ({ value: each, label: each }))}
          disabledReason={reason}
          onchange={(event) => onchange((event.currentTarget as HTMLSelectElement).value)}
          {error}
        />
      {:else if editor !== 'none'}
        <!-- A day, a link, a text, and a language or a zone where none are offered to choose from. -->
        <Input
          {label}
          isLabelHidden
          aria-labelledby={nameId}
          type={editor === 'date' ? 'date' : editor === 'link' ? 'url' : 'text'}
          value={current === undefined || current === null ? '' : String(current)}
          disabledReason={reason}
          oninput={(event) => {
            const typed = (event.currentTarget as HTMLInputElement).value.trim();
            onchange(typed === '' ? null : typed);
          }}
          autocomplete="off"
          spellcheck={false}
          {error}
        />
      {/if}
    {/snippet}
  </SettingRow>
</div>

<style>
  /* Wrapped for `data-setting`, so the rows' hairline is drawn here (as `SettingRow` draws it). */
  :global(.setting-control + .setting-control > .setting-row > .setting-line:first-child) { border-block-start: var(--bw-hairline) solid var(--border-subtle); }

  .choices { display: grid; gap: var(--sp-050); }

  .refusal { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
