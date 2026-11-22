<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Several of many, in one of two presentations of the same option list (ADR-0110).
  //
  // `inline` is visible ticks, the design system's `Checkbox` in a row that wraps - every answer
  // read at a glance. `field` is one closed button that says what is chosen and opens a checklist:
  // a multi-selectable listbox (WAI-ARIA APG Listbox) inside the `Popover`, which a phone's width
  // turns into the bottom sheet. Not a combobox: the APG's combobox is single-select, and the
  // filter here only narrows the list. `auto` picks by the number of options (multiselect.ts).
  //
  // **The chips are a summary, not buttons.** Removing happens in the list: one stop in the tab
  // order, no small targets, and a locked option needs no special chip.
  //
  // **An option with a reason cannot change.** Ticked it is locked on, unticked it is refused; it
  // stays focusable and its reason is a line under its label, so the list never moves to say it.
  // The whole control locked still opens, read-only, so that a locked value can be read.
  //
  // One press emits one change; *All*, *None* and Ctrl+A emit one change with the whole list. A
  // caller reading one toggle per call can rely on the inline ticks never emitting two at once.

  import { tick } from 'svelte';

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
  import Reason from './_Reason.svelte';
  import SearchField from './SearchField.svelte';
  import Ticks from './_Ticks.svelte';
  import type { ControlSize, Disableable } from './control.ts';
  import { rovingIndex, typeAheadIndex } from './focus.ts';
  import {
    closedOf,
    cutChips,
    filterSections,
    hasFilter,
    isFixed,
    presentationOf,
    sectionsOf,
    toggled,
    withAll,
    withNone,
    type ChoiceGroup,
    type ChoiceOption,
    type Presentation,
    type Summary,
  } from './multiselect.ts';

  interface Props extends Disableable {
    /** The question the options answer. The field's label, or the ticks' group name. */
    label: string;
    /** Read but not drawn, where the surroundings already name it. */
    isLabelHidden?: boolean;
    /** A name drawn elsewhere - a setting row's - instead of `label`'s own. */
    'aria-labelledby'?: string;
    options?: readonly ChoiceOption[];
    /** Options under headings, drawn after `options`. A group of none is not drawn. */
    groups?: readonly ChoiceGroup[];
    selected?: readonly string[];
    onChange?: (selected: readonly string[]) => void;
    /** `auto` draws up to five options as ticks and more as a field. */
    presentation?: Presentation;
    hint?: string;
    /** What is wrong now - nothing chosen where at least one is needed. Names the fix. */
    error?: string;
    isRequired?: boolean;
    /**
     * What a reader hears for the drawn asterisk - "(required)" - after the label. Neither a button
     * nor a group takes `aria-required`, so the name says it; the open list is `aria-required` too.
     */
    requiredLabel?: string;
    /** What the closed field says with nothing chosen. */
    emptyLabel: string;
    /** A sentence instead of the chips - "All 15" - or `undefined` to show the chips. */
    summary?: Summary;
    filterLabel: string;
    /** The filter's button that empties it - "Clear the filter"; never the filter's own name. */
    clearLabel: string;
    /** What a filter that matched nothing says (voice-and-tone.md §4.2). */
    noMatchLabel: string;
    allLabel: string;
    noneLabel: string;
    /** The sheet's button that closes the list on a phone's width. */
    doneLabel: string;
    /** The count beside *All* and *None* - "4 of 15". Without it, no count. */
    countLabel?: (chosen: number, total: number) => string;
    /** The locale the filter folds case under, and the closed field's list is said in. */
    locale?: string;
    /** `sm` draws the ticks as the structure panel's, `md` as `CheckboxGroup`'s. */
    size?: ControlSize;
  }

  const {
    label,
    isLabelHidden = false,
    'aria-labelledby': labelledBy,
    options = [],
    groups = [],
    selected = [],
    onChange,
    presentation = 'auto',
    hint,
    error,
    isRequired = false,
    requiredLabel,
    disabledReason,
    emptyLabel,
    summary,
    filterLabel,
    clearLabel,
    noMatchLabel,
    allLabel,
    noneLabel,
    doneLabel,
    countLabel,
    locale,
    size = 'md',
  }: Props = $props();

  // Svelte's own id, not a random one: unique per instance and the same on every render.
  const instanceId = $props.id();
  const uid = `multiselect-${instanceId}`;
  const nameId = $derived(labelledBy ?? `${uid}-label`);
  const hintId = $derived(hint ? `${uid}-hint` : undefined);
  const errorId = $derived(error ? `${uid}-error` : undefined);
  const reasonId = `${uid}-reason`;
  const noticeId = `${uid}-notice`;
  const valueId = `${uid}-value`;
  const listId = `${uid}-list`;

  const isReadOnly = $derived(disabledReason !== undefined);
  const all = $derived([...options, ...groups.flatMap((group) => group.options)]);
  const shape = $derived(presentationOf(presentation, all.length));
  const sections = $derived(sectionsOf(options, groups));
  const isFilterable = $derived(hasFilter(all.length));
  const describedBy = $derived(
    [errorId, isReadOnly && shape === 'field' ? reasonId : undefined, hintId].filter(Boolean).join(' ') || undefined,
  );

  function emit(next: readonly string[]): void {
    const isSame = next.length === selected.length && next.every((value, index) => value === selected[index]);
    if (!isSame) onChange?.(next);
  }

  // --- the closed field ---------------------------------------------------------------------

  const closed = $derived(closedOf(all, selected, { emptyLabel, summary }));
  /** How many chips fit in the field's two lines, measured on a hidden copy of them all. */
  let fitting = $state(Number.POSITIVE_INFINITY);
  let width = $state(0);
  let valueBox = $state<HTMLElement | null>(null);
  let measure = $state<HTMLElement | null>(null);
  const cut = $derived(closed.kind === 'chips' ? cutChips(closed.chips, fitting) : { shown: [], more: 0 });
  const moreText = $derived(new Intl.NumberFormat(locale, { signDisplay: 'always' }).format(cut.more));
  /** What a reader hears after the label: every chosen option, not only the chips that fit. */
  const spoken = $derived(
    closed.kind === 'chips'
      ? new Intl.ListFormat(locale, { type: 'conjunction' }).format(closed.chips.map((chip) => chip.label))
      : closed.text,
  );

  $effect(() => {
    const node = valueBox;
    if (!node || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(([entry]) => (width = entry?.contentRect.width ?? 0));
    observer.observe(node);
    return () => observer.disconnect();
  });

  $effect(() => {
    void width;
    const node = measure;
    if (!node || closed.kind !== 'chips') return;
    const tops = [...node.children].map((child) => (child as HTMLElement).offsetTop);
    const rows = [...new Set(tops)].sort((a, b) => a - b);
    const last = rows[1] ?? rows[0] ?? 0;
    fitting = tops.filter((top) => top <= last).length;
  });

  // --- the open list ------------------------------------------------------------------------

  let isOpen = $state(false);
  let term = $state('');
  let active = $state(0);
  let button = $state<HTMLButtonElement | null>(null);
  let list = $state<HTMLElement | null>(null);

  const shown = $derived(filterSections(sections, term, locale));
  const flat = $derived(shown.flatMap((section) => section.options));
  /** Where each shown section starts in the flat order the arrows walk. */
  const starts = $derived(shown.map((_, at) => shown.slice(0, at).reduce((sum, section) => sum + section.options.length, 0)));
  const chosenCount = $derived(all.filter((each) => selected.includes(each.value)).length);

  let headBox = $state<HTMLElement | null>(null);
  let footBox = $state<HTMLElement | null>(null);

  function focusActive(): void {
    const node = list?.querySelector<HTMLElement>(`[data-index="${active}"]`);
    if (!node) return;
    node.focus({ preventScroll: true });
    keepClear(node);
  }

  /** The surface that scrolls the list: the popover's, or the sheet's body. */
  function scrollerOf(node: HTMLElement): HTMLElement | null {
    for (let at = node.parentElement; at; at = at.parentElement) {
      if (at.scrollHeight > at.clientHeight && /auto|scroll/.test(getComputedStyle(at).overflowY)) return at;
    }
    return null;
  }

  // Focus is never hidden by a stuck region (design-system.md §10, 2.4.11): the browser scrolls a
  // focused option into the surface, not clear of the head and the foot stuck inside it, so the
  // option and its ring are scrolled out from under them here.
  function keepClear(node: HTMLElement): void {
    node.scrollIntoView({ block: 'nearest' });
    const scroller = scrollerOf(node);
    if (!scroller) return;
    const style = getComputedStyle(node);
    const ring = (Number.parseFloat(style.outlineOffset) || 0) + (Number.parseFloat(style.outlineWidth) || 0);
    const box = node.getBoundingClientRect();
    const top = headBox?.getBoundingClientRect().bottom;
    const bottom = footBox?.getBoundingClientRect().top;
    if (top !== undefined && box.top - ring < top) scroller.scrollTop -= top - (box.top - ring);
    else if (bottom !== undefined && box.bottom + ring > bottom) scroller.scrollTop += box.bottom + ring - bottom;
  }

  async function open(): Promise<void> {
    term = '';
    const first = flat.findIndex((each) => selected.includes(each.value));
    active = first >= 0 ? first : 0;
    isOpen = true;
    // After the surface is drawn and has moved focus to its first control: the list's own first
    // focus is the first chosen option.
    await tick();
    focusActive();
  }

  async function done(): Promise<void> {
    isOpen = false;
    await tick();
    button?.focus();
  }

  function press(option: ChoiceOption | undefined): void {
    if (!option || isReadOnly || isFixed(option)) return;
    emit(toggled(selected, option.value, all));
  }

  function chooseAll(): void {
    if (!isReadOnly) emit(withAll(selected, flat));
  }

  function chooseNone(): void {
    if (!isReadOnly) emit(withNone(selected, flat));
  }

  function onFieldKeydown(event: KeyboardEvent): void {
    if (event.key !== 'ArrowDown' || isOpen) return;
    event.preventDefault();
    void open();
  }

  function onListKeydown(event: KeyboardEvent): void {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a') {
      event.preventDefault();
      chooseAll();
      return;
    }
    if (event.key === ' ' || event.key === 'Enter') {
      event.preventDefault();
      press(flat[active]);
      return;
    }
    const next = rovingIndex(event.key, active, flat.length);
    if (next !== null) {
      event.preventDefault();
      active = next;
      focusActive();
      return;
    }
    if (event.ctrlKey || event.metaKey || event.altKey) return;
    const to = typeAheadIndex(event.key, active, flat.map((each) => each.label));
    if (to === null) return;
    event.preventDefault();
    active = to;
    focusActive();
  }

  function onPanelFocusout(event: FocusEvent): void {
    const to = event.relatedTarget;
    // Nowhere (a press beside it) is the popover's to dismiss; the field toggles it itself; the
    // sheet's own *Done* and close are still inside the surface.
    const surface = (event.currentTarget as HTMLElement).closest('dialog, [role="dialog"]');
    if (!(to instanceof Node) || to === button || surface?.contains(to)) return;
    isOpen = false;
  }

  function onFilterKeydown(event: KeyboardEvent): void {
    if (event.key !== 'ArrowDown' || flat.length === 0) return;
    event.preventDefault();
    focusActive();
  }
</script>

{#snippet name()}
  {#if labelledBy === undefined}
    <span class="label" class:is-hidden={isLabelHidden} id={nameId}>
      {label}{#if isRequired}<span class="required" aria-hidden="true">*</span>{#if requiredLabel}<span class="said">{` ${requiredLabel}`}</span>{/if}{/if}
    </span>
  {/if}
{/snippet}

{#snippet messages()}
  {#if error}
    <!-- Rule 3: the sentence is the signal; the border is only its echo. -->
    <p id={errorId} class="message error"><span class="mark"><Icon name="circle-x" size="sm" /></span><span>{error}</span></p>
  {/if}
  {#if hint}<p id={hintId} class="message">{hint}</p>{/if}
{/snippet}

{#snippet row(option: ChoiceOption, index: number)}
  {@const optionId = `${uid}-option-${all.indexOf(option)}`}
  {@const isOn = selected.includes(option.value)}
  {@const isStuck = isReadOnly || isFixed(option)}
  {@const described = [
    option.disabledReason !== undefined ? `${optionId}-reason` : undefined,
    isReadOnly ? noticeId : undefined,
    option.hint ? `${optionId}-hint` : undefined,
  ].filter(Boolean).join(' ')}
  <div
    class="option"
    role="option"
    id={optionId}
    data-index={index}
    tabindex={index === active ? 0 : -1}
    aria-selected={isOn}
    aria-disabled={isStuck ? 'true' : undefined}
    aria-labelledby={`${optionId}-label`}
    aria-describedby={described || undefined}
    onclick={() => {
      active = index;
      press(option);
    }}
    onkeydown={onListKeydown}
  >
    <span class="box" aria-hidden="true">{#if isOn}<Icon name="check" size="sm" />{/if}</span>
    <span class="text">
      <span class="name">
        <span id={`${optionId}-label`}>{option.label}</span>
        {#if isOn && isStuck}<span class="lock" aria-hidden="true"><Icon name="lock" size="sm" /></span>{/if}
      </span>
      {#if option.disabledReason !== undefined}<span class="line reason" id={`${optionId}-reason`}>{option.disabledReason}</span>{/if}
      {#if option.hint}<span class="line" id={`${optionId}-hint`}>{option.hint}</span>{/if}
    </span>
  </div>
{/snippet}

{#snippet sheetDone()}
  <Button tone="primary" onclick={() => void done()}>{doneLabel}</Button>
{/snippet}

{#if shape === 'inline'}
  <div
    class="inline"
    role="group"
    aria-labelledby={nameId}
    aria-describedby={describedBy}
    data-size={size}
    data-invalid={error ? '' : undefined}
  >
    {@render name()}
    {#each sections as section, at (section.label ?? `#${at}`)}
      {#if section.label !== undefined}
        <div class="section" role="group" aria-labelledby={`${uid}-group-${at}`}>
          <span class="heading" id={`${uid}-group-${at}`}>{section.label}</span>
          {@render ticks(section.options)}
        </div>
      {:else}
        {@render ticks(section.options)}
      {/if}
    {/each}
    {@render messages()}
  </div>
{:else}
  <!-- A button's role takes no `aria-invalid`: the field's root is marked `data-invalid`, as an
       upload's button is, and the button is described by the error (design-system.md §4, case 3). -->
  <div class="multiselect" data-size={size} data-invalid={error ? '' : undefined}>
    {@render name()}
    <button
      type="button"
      class="field"
      bind:this={button}
      data-size={size}
      data-readonly={isReadOnly ? '' : undefined}
      aria-haspopup="listbox"
      aria-expanded={isOpen}
      aria-controls={isOpen && flat.length > 0 ? listId : undefined}
      aria-labelledby={`${nameId} ${valueId}`}
      aria-describedby={describedBy}
      onclick={() => (isOpen ? void done() : void open())}
      onkeydown={onFieldKeydown}
    >
      <span class="value" bind:this={valueBox} aria-hidden="true">
        {#if closed.kind === 'chips'}
          <span class="chips">
            {#each cut.shown as chip (chip.value)}
              <span class="chip" dir="auto" data-locked={chip.isLocked ? '' : undefined}>
                {#if chip.isLocked}<Icon name="lock" size="sm" />{/if}{chip.label}
              </span>
            {/each}
            {#if cut.more > 0}<span class="chip more">{moreText}</span>{/if}
          </span>
          <!-- Every chip, out of sight: where the two lines end is read here, not guessed. -->
          <span class="chips measure" bind:this={measure}>
            {#each closed.chips as chip (chip.value)}
              <span class="chip" dir="auto">{#if chip.isLocked}<Icon name="lock" size="sm" />{/if}{chip.label}</span>
            {/each}
          </span>
        {:else}
          <span class="text" data-kind={closed.kind}>{closed.text}</span>
        {/if}
      </span>
      <span class="said" id={valueId}>{spoken}</span>
      {#if isReadOnly}<span class="mark" aria-hidden="true"><Icon name="lock" size="sm" /></span>{/if}
      <span class="mark" aria-hidden="true"><Icon name="chevron-down" size="sm" /></span>
    </button>
    {#if isReadOnly && disabledReason !== undefined}
      <!-- Not a guard: the locked field still opens, so that what it holds can be read. -->
      <Reason id={reasonId} text={disabledReason} control={button} isGuarding={false} />
    {/if}
    {@render messages()}

    <Popover {label} bind:isOpen at={button ?? undefined} sheetActions={sheetDone}>
      <!-- Not a trap: Tab walks the list's own controls, and focus leaving them closes it, as
           Tab closes a combobox's popup (APG), and goes on. The sheet is modal and keeps it. -->
      <div class="panel" onfocusout={onPanelFocusout}>
        {#if isReadOnly || isFilterable}
          <!-- The read-only reason stands in the stuck head with the filter: under the head's cover
               it would be cut through as the list scrolls. -->
          <div class="top" bind:this={headBox}>
            {#if isReadOnly}
              <p class="notice" id={noticeId}><Icon name="lock" size="sm" /><span>{disabledReason}</span></p>
            {/if}
            {#if isFilterable}
              <SearchField
                label={filterLabel}
                isLabelHidden
                {clearLabel}
                size="sm"
                bind:value={term}
                oninput={() => (active = 0)}
                placeholder={filterLabel}
                onkeydown={onFilterKeydown}
              />
            {/if}
          </div>
        {/if}
        <div class="body">
          {#if isFilterable}
            <!-- Stands while the list is open, so that the sentence is heard when a filter empties it. -->
            <div role="status">{#if flat.length === 0}<p class="empty">{noMatchLabel}</p>{/if}</div>
          {/if}
          {#if flat.length > 0}
            <div
              class="list"
              role="listbox"
              id={listId}
              aria-multiselectable="true"
            aria-required={isRequired ? 'true' : undefined}
              aria-labelledby={nameId}
              tabindex="-1"
              bind:this={list}
            >
              {#each shown as section, at (section.label ?? `#${at}`)}
                {#if section.label !== undefined}
                  <div class="section" role="group" aria-labelledby={`${uid}-heading-${at}`}>
                    <span class="heading" id={`${uid}-heading-${at}`} role="presentation">{section.label}</span>
                    {#each section.options as option, position (option.value)}{@render row(option, (starts[at] ?? 0) + position)}{/each}
                  </div>
                {:else}
                  {#each section.options as option, position (option.value)}{@render row(option, (starts[at] ?? 0) + position)}{/each}
                {/if}
              {/each}
            </div>
          {/if}
        </div>
        {#if isFilterable && !isReadOnly}
          <div class="foot" bind:this={footBox}>
            {#if flat.length > 0}
              <Button tone="subtle" size="sm" onclick={chooseAll}>{allLabel}</Button>
              <Button tone="subtle" size="sm" onclick={chooseNone}>{noneLabel}</Button>
            {/if}
            {#if countLabel}<span class="count" dir="auto">{countLabel(chosenCount, all.length)}</span>{/if}
          </div>
        {/if}
      </div>
    </Popover>
  </div>
{/if}

{#snippet ticks(list: readonly ChoiceOption[])}
  <Ticks options={list} {selected} onChange={emit} {disabledReason} {errorId} {size} />
{/snippet}

<style>
  .multiselect,
  .inline {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    min-inline-size: 0;
  }

  .inline { gap: var(--sp-100); }

  .label {
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    text-align: start;
  }

  /* Announced but not drawn - never `display: none`, which would take the name away. */
  .label.is-hidden,
  .said {
    position: absolute;
    inline-size: var(--sp-025);
    block-size: var(--sp-025);
    margin: calc(var(--sp-025) * -1);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .required { margin-inline-start: var(--sp-025); color: var(--text-danger); }

  .inline .section { display: flex; flex-direction: column; gap: var(--sp-050); }

  /* Select's box: its border, radius, height and chevron. */
  .field {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    inline-size: 100%;
    min-block-size: var(--density-control-md-min);
    padding-block: var(--density-control-md-block);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-100);
    text-align: start;
    cursor: pointer;
  }

  .field[data-size='sm'] {
    min-block-size: var(--density-control-sm-min);
    padding-block: var(--density-control-sm-block);
    font-size: var(--fs-075);
  }

  .field:hover { border-color: var(--border-strong); }

  /* Focused, the field's own border takes the focus colour and doubles inward, as Select's. */
  .field:focus-visible {
    outline: none;
    border-color: var(--focus-ring);
    box-shadow: inset 0 0 0 var(--bw-hairline) var(--focus-ring);
  }

  .multiselect[data-invalid] .field { border-color: var(--status-danger-border); border-width: var(--bw-thick); background: var(--status-danger-surface); }
  .field[data-readonly] { border-color: var(--border-subtle); background: var(--bg-surface-sunken); }

  .value { position: relative; flex: 1 1 auto; min-inline-size: 0; overflow: hidden; }
  .value .text { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .value .text[data-kind='empty'] { color: var(--text-subtle); }

  .mark { display: inline-flex; flex: none; color: var(--text-subtle); }

  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); }

  .measure {
    position: absolute;
    inset-block-start: 0;
    inset-inline: 0;
    visibility: hidden;
    pointer-events: none;
  }

  /* The neutral pill TemplateCard draws a product with. */
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-025);
    max-inline-size: 100%;
    padding: 0 var(--sp-050);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .field[data-readonly] .chip { background: var(--bg-surface); }

  .panel { display: flex; flex-direction: column; gap: var(--sp-100); min-inline-size: 0; }

  .notice {
    display: flex;
    align-items: flex-start;
    gap: var(--sp-050);
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  /* The status and the list, with no gap for a status that says nothing. */
  .body { display: flex; flex-direction: column; min-inline-size: 0; }

  .list { display: flex; flex-direction: column; gap: var(--sp-025); outline: none; }

  .list .section { display: flex; flex-direction: column; gap: var(--sp-025); }
  .list .section + .section,
  .list .option + .section { margin-block-start: var(--sp-100); }

  .heading {
    padding-inline: var(--sp-100);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
  }

  .option {
    display: flex;
    align-items: flex-start;
    gap: var(--sp-100);
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-100);
    border-radius: var(--r-md);
    color: var(--text-primary);
    text-align: start;
    cursor: pointer;
  }

  .option:hover { background: var(--bg-surface-hover); }

  .option:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .option[aria-disabled='true'] { cursor: not-allowed; }
  .option[aria-disabled='true'][aria-selected='false'] .name { color: var(--text-subtle); }

  /* A thumb's target (design-system.md §9) where the list is the bottom sheet. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .option { min-block-size: var(--layout-target); align-items: center; }
  }

  /* Checkbox's box, drawn: the option is the control, the box its picture. */
  .box {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    inline-size: var(--sp-250);
    block-size: var(--sp-250);
    margin-block-start: calc((1lh - var(--sp-250)) / 2);
    border: var(--bw-thick) solid var(--border-strong);
    border-radius: var(--r-xs);
    background: var(--bg-surface);
    color: var(--text-inverse);
  }

  .option[aria-selected='true'] .box { border-color: var(--accent-primary); background: var(--accent-primary); }
  .option[aria-disabled='true'] .box { border-color: var(--border-subtle); background: var(--bg-surface-sunken); color: var(--text-subtle); }
  .option[aria-disabled='true'][aria-selected='true'] .box { border-color: var(--border-default); background: var(--border-default); }

  .text { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
  .name { display: inline-flex; align-items: center; gap: var(--sp-050); overflow-wrap: anywhere; }
  .lock { display: inline-flex; color: var(--text-subtle); }
  .line { color: var(--text-subtle); font-size: var(--fs-075); }
  .line.reason { color: var(--text-secondary); }

  .empty { margin: 0; padding: var(--sp-150); color: var(--text-subtle); font-size: var(--fs-075); }

  /* The head - the read-only reason and the filter - and the foot stay in view while a long list
     scrolls under them in the surface. */
  .top,
  .foot {
    position: sticky;
    z-index: var(--z-sticky);
    background: var(--bg-raised);
  }

  /* The surface's own padding lies between a stuck part and its edge: covered, so no row shows
     through the gap as it scrolls past. */
  .top::before,
  .foot::after {
    content: '';
    position: absolute;
    inset-inline: 0;
    block-size: var(--sp-200);
    background: var(--bg-raised);
  }

  .top::before { inset-block-end: 100%; }
  .foot::after { inset-block-start: 100%; }

  .top {
    inset-block-start: 0;
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    padding-block-end: var(--sp-050);
  }

  .foot {
    inset-block-end: 0;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
    padding-block: var(--sp-100) var(--sp-050);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  .count { margin-inline-start: auto; color: var(--text-subtle); font-size: var(--fs-075); }

  .message {
    max-width: 60ch;
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
    text-align: start;
    overflow-wrap: anywhere;
  }

  .message.error {
    display: flex;
    align-items: flex-start;
    gap: var(--sp-050);
    color: var(--text-danger);
    font-weight: var(--fw-medium);
  }

  .message.error .mark { color: inherit; margin-block-start: var(--sp-025); }
</style>
