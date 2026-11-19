<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The query line (work-surface concept §6.11, Q-1…Q-4): chips and text chips in one line, no
  // switch between "text" and "controls". A typed `type: incident` becomes a chip when it is
  // finished; a press on a chip gives its spelling back to edit; what has no chip form stays a text
  // chip. Suggestions at every keystroke - fields, then values from the data with counts -, the
  // count of what matches in its slot while typing, and an error at its chip with the fix.
  //
  // It knows no grammar: the language (`parse`, `format`) and the field catalogue are handed in,
  // so the one parser on the server and this line can never disagree about a dialect.
  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import Sheet from './Sheet.svelte';
  import { isCompactWidth, watchCompactWidth } from './media.ts';
  import { openOverlay } from './overlay.ts';
  import { suggest, type FieldCatalogue, type QueryLanguage, type QuerySuggestion, type QueryToken } from './query.ts';

  interface Props {
    /** The field's name: *Filter*, *Search*. */
    label: string;
    language: QueryLanguage;
    catalogue: FieldCatalogue;
    tokens?: readonly QueryToken[];
    /** What is being typed and not yet a token. */
    text?: string;
    placeholder?: string;
    /** The suggestion list's name and its two headings. */
    suggestionsLabel: string;
    fieldsHeading: (typed: string) => string;
    valuesHeading: (field: string) => string;
    /** A chip's two acts, named with the chip's spelling. */
    editLabel: (spelling: string) => string;
    removeLabel: (spelling: string) => string;
    /** The count of what matches, while typing (Q-4): *4 entries*. */
    count?: Snippet;
    /** The list's foot: the keys, *Help on queries*. */
    keys?: Snippet;
    onchange?: (tokens: readonly QueryToken[]) => void;
    /** Enter on a line with nothing left to complete: run the query. */
    onsubmit?: (tokens: readonly QueryToken[]) => void;
    /**
     * *+ Condition* and *+ Group (or)* under the line (§6.11): the first opens the fields, the
     * second starts a group whose conditions are joined by OR. Without them, only typing adds.
     */
    addConditionLabel?: string;
    addGroupLabel?: string;
    /** A quiet line beside them: *A press on a chip edits it · typing goes on*. */
    hint?: string;
    /**
     * The phone's form (scene qline): the first chips in one strip with "+n", and the filters in a
     * sheet with a row per condition. `more` names the hidden ones: *+3*.
     */
    phone?: { open: string; title: string; close: string; reset: string; show: string; more: (hidden: number) => string };
    /** The phone's sheet is open; bound where a button elsewhere opens it. */
    isSheetOpen?: boolean;
    /** False where that button stands elsewhere: the strip carries only the chips and the count. */
    hasPhoneButton?: boolean;
    /** Under the sheet's conditions: a way on to what the filter does not hold. */
    sheetEnd?: Snippet;
  }

  let {
    label,
    language,
    catalogue,
    tokens = $bindable([]),
    text = $bindable(''),
    placeholder,
    suggestionsLabel,
    fieldsHeading,
    valuesHeading,
    editLabel,
    removeLabel,
    count,
    keys,
    onchange,
    onsubmit,
    addConditionLabel,
    addGroupLabel,
    hint,
    phone,
    isSheetOpen = $bindable(false),
    hasPhoneButton = true,
    sheetEnd,
  }: Props = $props();

  /** The chips a phone's strip shows before "+n". */
  const STRIP = 2;
  let isPhone = $state(isCompactWidth());
  $effect(() => watchCompactWidth((value) => (isPhone = value)));
  /** *+ Condition* pressed: every field is offered before a letter is typed. */
  let isBrowsing = $state(false);
  let line = $state<HTMLElement | null>(null);
  let list = $state<HTMLElement | null>(null);

  const uid = $props.id();
  const listId = `query-${uid}-list`;
  let input = $state<HTMLInputElement | null>(null);
  let active = $state(0);
  let isListOpen = $state(false);

  const suggestions = $derived<QuerySuggestion[]>(isListOpen ? suggest(catalogue, text, { isBrowsing }) : []);
  const fieldRows = $derived(suggestions.filter((item) => item.group === 'field'));
  const valueRows = $derived(suggestions.filter((item) => item.group === 'value'));
  const ordered = $derived([...fieldRows, ...valueRows]);
  const valueField = $derived(/^-?([\w.]+):/.exec(text.trimStart())?.[1]);
  const valueFieldLabel = $derived(catalogue.fields.find((field) => field.key === valueField)?.label ?? valueField ?? '');
  const problems = $derived(tokens.filter((token) => token.problem));
  const isShown = $derived(isListOpen && ordered.length > 0);

  // The list is drawn in the top layer, anchored under the line: a card, a table region or a sheet
  // around the line never clips it, and it floats over what follows (ADR-0039).
  $effect(() => {
    if (!isShown || !line || !list) return;
    return openOverlay({ layer: 'popover', trigger: line, surface: list, placement: { side: 'block-end', align: 'start' }, onDismiss: () => (isListOpen = false) });
  });

  function addCondition() {
    if (text.trim() !== '') settle(`${text} `);
    isBrowsing = true;
    isListOpen = true;
    active = 0;
    input?.focus();
  }

  function addGroup() {
    if (text.trim() !== '') settle(`${text} `);
    text = '(';
    isBrowsing = true;
    isListOpen = true;
    active = 0;
    input?.focus();
  }

  function setTokens(next: readonly QueryToken[]) {
    tokens = next;
    onchange?.(next);
  }

  /** Turns what is finished into tokens; the unfinished rest stays typed. */
  function settle(line: string) {
    const parsed = language.parse(line);
    if (parsed.tokens.length > 0) setTokens([...tokens, ...parsed.tokens]);
    text = parsed.rest;
  }

  function take(item: QuerySuggestion) {
    const line = item.insert;
    active = 0;
    if (item.group === 'value') settle(line);
    else text = line;
    input?.focus();
  }

  function edit(token: QueryToken) {
    setTokens(tokens.filter((candidate) => candidate.id !== token.id));
    text = `${language.format(token)} ${text}`.trimEnd();
    isListOpen = true;
    input?.focus();
  }

  function remove(token: QueryToken) {
    setTokens(tokens.filter((candidate) => candidate.id !== token.id));
    input?.focus();
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      if (!isListOpen) isListOpen = true;
      if (ordered.length > 0) active = (active + (event.key === 'ArrowDown' ? 1 : ordered.length - 1)) % ordered.length;
      event.preventDefault();
    } else if (event.key === 'Enter') {
      event.preventDefault();
      const item = ordered[active];
      if (isListOpen && item) take(item);
      else {
        settle(`${text} `);
        if (text.trim() === '') onsubmit?.(tokens);
      }
    } else if (event.key === 'Escape') {
      if (isListOpen) {
        isListOpen = false;
        event.stopPropagation();
        event.preventDefault();
      }
    } else if (event.key === 'Backspace' && text === '' && tokens.length > 0) {
      event.preventDefault();
      edit(tokens[tokens.length - 1]!);
    }
  }

  function onInput() {
    isBrowsing = false;
    isListOpen = true;
    active = 0;
    if (/\s$/.test(text) && !/[:,]\s*$/.test(text)) settle(text);
  }

  const spelling = (token: QueryToken) => language.format(token);
  const optionId = (item: QuerySuggestion) => `${listId}-${item.id}`;
</script>

<div class="query">
  {#if isPhone && phone}
    {#if tokens.length > 0}
      <div class="strip">
        {#each tokens.slice(0, STRIP) as token (token.id)}
          <span class="chip" data-kind={token.kind} data-problem={token.problem ? '' : undefined}><span class="chip-body">{@render tokenWords(token)}</span></span>
        {/each}
        {#if tokens.length > STRIP}<span class="chip more"><span class="value">{phone.more(tokens.length - STRIP)}</span></span>{/if}
      </div>
    {/if}
    {#if hasPhoneButton || count}
      <div class="phone-bar">
        {#if hasPhoneButton}<Button tone="secondary" size="sm" icon="funnel" onclick={() => (isSheetOpen = true)}>{phone.open}</Button>{/if}
        {#if count}<span class="count" role="status">{@render count()}</span>{/if}
      </div>
    {/if}
    <Sheet title={phone.title} closeLabel={phone.close} bind:isOpen={isSheetOpen}>
      <ul class="rows">
        {#each tokens as token (token.id)}
          <li>
            <button type="button" class="row" aria-label={editLabel(spelling(token))} onclick={() => edit(token)}>
              <span class="row-field">{token.kind === 'chip' ? token.fieldLabel : spelling(token)}</span>
              {#if token.kind === 'chip'}<span class="row-values">{token.values.map((value) => value.label).join(', ')}</span>{/if}
              <Icon name="chevron-right" size="sm" />
            </button>
          </li>
        {/each}
      </ul>
      {@render editor(false)}
      {#if sheetEnd}{@render sheetEnd()}{/if}
      {#snippet actions()}
        <Button tone="subtle" onclick={() => setTokens([])}>{phone.reset}</Button>
        <Button tone="primary" onclick={() => { isSheetOpen = false; onsubmit?.(tokens); }}>{phone.show}</Button>
      {/snippet}
    </Sheet>
  {:else}
    {@render editor(true)}
    {#if count}<div class="count" role="status">{@render count()}</div>{/if}
  {/if}
</div>

{#snippet tokenWords(token: QueryToken)}
  {#if token.kind === 'chip'}
    <span class="field">{token.isNegated ? '−' : ''}{token.fieldLabel}</span>
    {#if token.operatorLabel}<span class="operator">{token.operatorLabel}</span>{/if}
    <span class="value">{token.values.map((value) => value.label).join(', ')}</span>
  {:else}
    <span class="value" data-text="">{token.text}</span>
  {/if}
{/snippet}

{#snippet editor(withChips: boolean)}
  <div class="line" bind:this={line} data-open={isShown ? '' : undefined}>
    <Icon name="funnel" size="sm" />
    {#each withChips ? tokens : [] as token (token.id)}
      {@const words = spelling(token)}
      <span class="chip" data-kind={token.kind} data-problem={token.problem ? '' : undefined}>
        <button
          type="button"
          class="chip-body"
          aria-label={editLabel(words)}
          aria-describedby={token.problem ? `query-${uid}-${token.id}-problem` : undefined}
          onclick={() => edit(token)}
        >
          {@render tokenWords(token)}
          {#if token.problem}<Icon name="circle-alert" size="sm" />{/if}
        </button>
        <span class="remove"><IconButton icon="x" label={removeLabel(words)} tone="subtle" size="sm" onclick={() => remove(token)} /></span>
      </span>
    {/each}
    <input
      bind:this={input}
      bind:value={text}
      class="input"
      type="text"
      role="combobox"
      aria-label={label}
      aria-autocomplete="list"
      aria-expanded={isShown}
      aria-controls={listId}
      aria-activedescendant={isListOpen && ordered[active] ? optionId(ordered[active]!) : undefined}
      autocomplete="off"
      spellcheck="false"
      {placeholder}
      oninput={onInput}
      onkeydown={onKeydown}
      onfocus={() => (isListOpen = text !== '')}
      onblur={() => setTimeout(() => (isListOpen = false), 0)}
    />
  </div>

  {#if problems.length > 0}
    <ul class="problems">
      {#each problems as token (token.id)}
        <li id={`query-${uid}-${token.id}-problem`}>
          <Icon name="circle-alert" size="sm" /><span><b>{spelling(token)}</b> {token.problem?.message}</span>
          {#if token.problem?.fix && token.problem.fixLabel}
            {@const fix = token.problem.fix}
            <button type="button" class="fix" onclick={() => { setTokens(tokens.filter((candidate) => candidate.id !== token.id)); settle(`${fix} `); }}>{token.problem.fixLabel}</button>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {#if addConditionLabel || addGroupLabel || hint}
    <div class="adds">
      {#if addConditionLabel}<Button tone="subtle" size="sm" icon="plus" onclick={addCondition}>{addConditionLabel}</Button>{/if}
      {#if addGroupLabel}<Button tone="subtle" size="sm" icon="plus" onclick={addGroup}>{addGroupLabel}</Button>{/if}
      {#if hint}<span class="hint-line">{hint}</span>{/if}
    </div>
  {/if}

  <div class="list" id={listId} role="listbox" aria-label={suggestionsLabel} hidden={!isShown} bind:this={list}>
    {#if fieldRows.length > 0}
      <div role="group" aria-label={fieldsHeading(text.trim())}>
        <div class="heading" aria-hidden="true">{fieldsHeading(text.trim())}</div>
        {#each fieldRows as item, index (item.id)}
          {@render option(item, index)}
        {/each}
      </div>
    {/if}
    {#if valueRows.length > 0}
      <div role="group" aria-label={valuesHeading(valueFieldLabel)}>
        <div class="heading" aria-hidden="true">{valuesHeading(valueFieldLabel)}</div>
        {#each valueRows as item, index (item.id)}
          {@render option(item, fieldRows.length + index)}
        {/each}
      </div>
    {/if}
    {#if keys}<div class="keys">{@render keys()}</div>{/if}
  </div>
{/snippet}

{#snippet option(item: QuerySuggestion, index: number)}
  <!-- Options are chosen with the pointer here and with the arrows in the field: the field keeps the focus. -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    class="option"
    role="option"
    id={optionId(item)}
    aria-selected={index === active}
    tabindex="-1"
    onpointerdown={(event) => event.preventDefault()}
    onclick={() => take(item)}
  >
    <Icon name={item.group === 'field' ? 'funnel' : 'arrow-right'} size="sm" />
    <span class="option-text"><span class="option-label">{item.label}</span>{#if item.detail}<span class="detail"> · {item.detail}</span>{/if}</span>
    {#if item.hint}<span class="hint">{item.hint}</span>{/if}
  </div>
{/snippet}

<style>
  .query { position: relative; display: flex; flex-direction: column; gap: var(--sp-100); min-inline-size: 0; }

  .line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-050) calc(var(--sp-050) + var(--sp-025));
    min-block-size: var(--layout-nav-touchrow);
    padding: calc(var(--sp-050) + var(--sp-025)) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-size: var(--fs-100);
    cursor: text;
    transition: box-shadow var(--motion-state-duration) var(--motion-state-easing);
  }

  .line:focus-within { border-color: var(--border-strong); box-shadow: 0 0 0 var(--sp-025) var(--accent-primary-subtle); }

  .chip {
    display: inline-flex;
    align-items: stretch;
    max-inline-size: 100%;
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-sm);
    background: var(--bg-surface-sunken);
    overflow: hidden;
  }

  .chip[data-problem] { border-color: var(--status-danger-accent); background: var(--status-danger-surface); }

  .chip-body {
    display: inline-flex;
    align-items: center;
    min-inline-size: 0;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-090);
    line-height: var(--lh-snug);
    cursor: pointer;
  }

  .chip-body .field,
  .chip-body .value { padding-block: var(--sp-025); }

  .chip-body:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(-1 * var(--bw-ring));
  }

  .field { align-self: stretch; display: inline-flex; align-items: center; padding: var(--sp-050) calc(var(--sp-050) + var(--sp-025)); background: var(--bg-surface-hover); color: var(--text-secondary); }
  .operator { padding-inline: var(--sp-025); color: var(--text-subtle); }
  .value { padding: var(--sp-050) calc(var(--sp-050) + var(--sp-025)); font-weight: var(--fw-medium); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .value[data-text] { font-family: var(--font-mono); font-size: var(--fs-075); font-weight: var(--fw-regular); }
  .chip[data-problem] .chip-body :global(.icon) { color: var(--status-danger-text); margin-inline-end: var(--sp-050); }

  .remove { display: inline-flex; align-items: center; }

  .input {
    flex: 1;
    min-inline-size: var(--sp-1000);
    padding: var(--sp-050) 0;
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
  }

  .input:focus-visible { outline: none; }
  .input::placeholder { color: var(--text-subtle); }

  .problems { display: flex; flex-direction: column; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; font-size: var(--fs-075); color: var(--text-secondary); }
  .problems li { display: flex; align-items: center; gap: var(--sp-050); }
  .problems :global(.icon) { color: var(--text-danger); }

  .fix {
    padding: 0 var(--sp-050);
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-brand);
    font: inherit;
    font-weight: var(--fw-semibold);
    cursor: pointer;
  }

  .fix:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .list {
    margin-block: var(--sp-050);
    z-index: var(--z-popover);
    display: flex;
    flex-direction: column;
    gap: var(--bw-hairline);
    inline-size: min(var(--layout-dialog-m), calc(100vw - var(--sp-400)));
    max-block-size: 60vh;
    overflow: auto;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
    font-size: var(--fs-090);
  }

  .list[hidden] { display: none; }

  .heading {
    padding: var(--sp-100) var(--sp-100) var(--sp-025);
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .option {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    min-block-size: var(--sp-400);
    padding: calc(var(--sp-050) + var(--sp-025)) var(--sp-100);
    border-radius: var(--r-sm);
    color: var(--text-primary);
    cursor: pointer;
  }

  .option[aria-selected='true'] { background: var(--accent-primary-subtle); }
  .option :global(.icon) { color: var(--text-secondary); flex: none; }
  .option-text { flex: 1; min-inline-size: 0; }
  .option-label { font-weight: var(--fw-medium); }
  .detail { color: var(--text-subtle); font-size: var(--fs-075); }
  .hint { color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--fs-075); white-space: nowrap; }

  .keys {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-200);
    margin-block-start: var(--sp-050);
    padding: var(--sp-100) var(--sp-100) var(--sp-025);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .count { color: var(--text-secondary); font-size: var(--fs-090); }

  .adds { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .hint-line { margin-inline-start: auto; color: var(--text-subtle); font-size: var(--fs-075); }

  /* The phone: one strip that never wraps, the bar under it, the sheet's rows. */
  .strip { display: flex; flex-wrap: nowrap; gap: var(--sp-050); overflow: hidden; }
  .strip .chip { flex: 0 1 auto; min-inline-size: 0; }
  .strip .chip.more { flex: none; }
  .strip .chip-body { cursor: default; }
  .more .value { color: var(--text-secondary); }
  .phone-bar { display: flex; align-items: center; gap: var(--sp-100); }
  .phone-bar > :global(.button) { flex: none; white-space: nowrap; }
  .rows { margin: 0 0 var(--sp-200); padding: 0; list-style: none; }
  .rows li + li { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    inline-size: 100%;
    min-block-size: var(--layout-target);
    padding: var(--sp-100) 0;
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }
  .row:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .row-field { flex: 1; }
  .row-values { color: var(--text-secondary); font-size: var(--fs-075); }

  /* A phone: the list under the line at the line's width. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .list { inline-size: 100%; }
    .option { min-block-size: var(--layout-nav-touchrow); }
    /* The words' field is a thumb's target (§7): it takes the line's height, not its padding. */
    .line { padding-block: 0; }
    .input { align-self: stretch; min-block-size: var(--layout-target); }
  }

  @media (prefers-reduced-motion: reduce) {
    .line { transition: none; }
  }

  :global([data-motion='reduced']) .line { transition: none; }
</style>
