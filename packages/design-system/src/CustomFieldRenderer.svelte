<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A field the installation added, drawn as whatever its kind says it is.
  //
  // **Thirteen kinds, and one this component has never heard of.** The schema has carried
  // `TEXT`, `NUMBER`, `DATE`, `SELECT`, `MULTI_SELECT`, `BOOL`, `USER` and `URL` since the first
  // migration, and `MONEY`, `DURATION`, `REFERENCE`, `USER_SET` and `RICH_TEXT` since the typed
  // fields; a client one version behind a server will meet a kind that is not among them.
  // Tolerance towards unknown fields is a binding client requirement, so an unknown kind is
  // rendered **read-only with the reason** rather than dropped: a value the reader cannot see is
  // indistinguishable from a value that is not there, and the second one is a data-loss report.
  //
  // **`USER`, `USER_SET` and `REFERENCE` are slots.** Who may be named, and which entries may be
  // pointed at, are the domain's answers, so the pickers are handed in rather than imported — which
  // is also what keeps this component from depending on the people or the entries half of the model
  // to draw the kinds that have nothing to do with either.
  //
  // The key is an identifier and never a label: `custom_fields.<key>` appears in filters and can
  // never be renamed. What a person reads is `label`, resolved by the caller (ADR-0011).

  import type { Snippet } from 'svelte';

  import Checkbox from './Checkbox.svelte';
  import Input from './Input.svelte';
  import Select from './Select.svelte';
  import Textarea from './Textarea.svelte';
  import type { Disableable } from './control.ts';
  import ReasonMark from './_ReasonMark.svelte';
  import { provideReasonGroup } from './reason.ts';

  /** The thirteen the contract declares. A string beside them is a kind from a newer server. */
  export type CustomFieldKind =
    | 'TEXT'
    | 'NUMBER'
    | 'DATE'
    | 'SELECT'
    | 'MULTI_SELECT'
    | 'BOOL'
    | 'USER'
    | 'URL'
    | 'MONEY'
    | 'DURATION'
    | 'REFERENCE'
    | 'USER_SET'
    | 'RICH_TEXT';

  /** One definition, narrowed to what is drawn. The shape `CustomFieldDefinition` has. */
  export interface FieldDefinition {
    /** The identifier the value is stored under. Never shown: it is a key, not a label. */
    readonly key: string;
    /** What a person reads. Resolved by the caller, because this package writes no text. */
    readonly label: string;
    /** One of the thirteen, or a kind from a server this client has not caught up with. */
    readonly kind: CustomFieldKind | (string & {});
    /** The permitted values of a `SELECT` or a `MULTI_SELECT`, and empty for every other kind. */
    readonly options?: readonly string[];
    /** Whether it has to hold a value. Shown, because a reader owes nothing they were not told. */
    readonly isRequired?: boolean;
    /** Standing help under the label. */
    readonly hint?: string;
  }

  /** How a number is written and read back: a `NUMBER`, a `MONEY` amount, a `DURATION`. */
  export interface NumberReading {
    readonly format: (value: number) => string;
    /** `null` for an empty field, `undefined` for text that is not a value of the kind. */
    readonly parse: (text: string) => number | null | undefined;
    /** What the field says beside itself while the text is not a value. Resolved text. */
    readonly invalidLabel: string;
  }

  /** What a field can hold. `null` is "not set", which is a different thing from an empty string. */
  export type FieldValue = string | number | boolean | readonly string[] | null;

  interface Props extends Disableable {
    definition: FieldDefinition;
    value?: FieldValue;
    /**
     * The picker for a `USER` or a `USER_SET` field, handed in rather than imported; `multiple`
     * says which. Absent renders the kind as unknown — which is honest: without a picker there is
     * nothing to draw.
     */
    user?: Snippet<[{ value: FieldValue; multiple: boolean }]>;
    /** The picker for a `REFERENCE` field: which entry it points at. Absent, as for `user`. */
    reference?: Snippet<[{ value: FieldValue }]>;
    /**
     * Why a kind this client does not know cannot be edited. Required, for the reason
     * `disabledReason` is: a control that cannot be used owes the reader the reason.
     */
    unknownKindLabel: string;
    /** What "no value" reads as, where a value is only shown rather than edited. */
    emptyValueLabel: string;
    /**
     * How a `NUMBER` is written and read in the reader's locale, handed in by the application
     * (i18n-l10n.md §6 line 10): `1,5` under `de` is `1.5` to the contract. Absent, the field is
     * a native number input and the browser's own parsing applies. `parse` answers `null` for an
     * empty field and `undefined` for text that is not a number, which is shown as such rather
     * than sent.
     */
    numbers?: NumberReading;
    /**
     * How a `MONEY` amount is written and read: the same contract as `numbers`, and `numbers`
     * itself when absent. The currency is the definition's and the caller says it in `hint`.
     */
    amounts?: NumberReading;
    /**
     * How a `DURATION` is written and read, in whole minutes on the wire. Absent, the field is a
     * number input of minutes.
     */
    durations?: NumberReading;
    onChange?: (value: FieldValue) => void;
  }

  const {
    definition,
    value = null,
    user,
    unknownKindLabel,
    emptyValueLabel,
    disabledReason,
    numbers,
    amounts,
    durations,
    reference,
    onChange,
  }: Props = $props();


  const options = $derived((definition.options ?? []).map((option) => ({ value: option, label: option })));
  const chosen = $derived(Array.isArray(value) ? (value as readonly string[]) : []);
  const text = $derived(value === null || value === undefined ? '' : String(value));

  // A number is shown as the locale writes it and parsed back before it leaves; the text the
  // reader is typing is held here so that an unparsable draft is not overwritten by the value
  // it failed to become.
  // An amount and a duration are numbers written the reader's way too, each with its own reading.
  const reading = $derived(
    definition.kind === 'MONEY' ? (amounts ?? numbers) : definition.kind === 'DURATION' ? durations : numbers,
  );
  let numberDraft = $state<string | undefined>(undefined);
  const numberText = $derived(
    numberDraft ?? (typeof value === 'number' && reading ? reading.format(value) : text),
  );
  const numberInvalid = $derived(
    reading !== undefined && numberDraft !== undefined && reading.parse(numberDraft) === undefined,
  );
  const isNumeric = $derived(['NUMBER', 'MONEY', 'DURATION'].includes(definition.kind));

  const KNOWN = new Set([
    'TEXT', 'NUMBER', 'DATE', 'SELECT', 'MULTI_SELECT', 'BOOL', 'USER', 'URL',
    'MONEY', 'DURATION', 'REFERENCE', 'USER_SET', 'RICH_TEXT',
  ]);
  /** A people or entry field with no picker is as undrawable as a kind nobody has heard of. */
  const isUnknown = $derived(
    !KNOWN.has(definition.kind) ||
      ((definition.kind === 'USER' || definition.kind === 'USER_SET') && !user) ||
      (definition.kind === 'REFERENCE' && !reference),
  );

  // Svelte's own id, not a random one: unique per instance and the same on every render, so a
  // prerendered page is identical between builds and a hydrated one matches its markup.
  const instanceId = $props.id();
  const reasonId = $derived(isUnknown ? `unknown-${instanceId}` : undefined);

  function toggle(option: string) {
    onChange?.(chosen.includes(option) ? chosen.filter((held) => held !== option) : [...chosen, option]);
  }
  // One reason for every part (design-system.md §4): the parts that are off for it point at the
  // mark's and draw nothing of their own, and the mark's guard stands over all of them.
  const groupId = `group-reason-${instanceId}`;
  // Only a multiple choice has several parts; every other kind is one field, which says its reason
  // itself.
  provideReasonGroup(() => ({ id: groupId, reason: definition.kind === 'MULTI_SELECT' ? disabledReason : undefined }));
  let container = $state<HTMLElement | null>(null);
</script>

{#if isUnknown}
  <!-- Read-only, and with the reason beside it. The value is still shown: a client one window
       behind the server meets this, and that is the normal state of the track rather than an
       error, so what the field holds must not disappear on the way through. -->
  <div class="unknown">
    <p class="label">
      {definition.label}
      {#if definition.isRequired}<span class="required" aria-hidden="true">*</span>{/if}
    </p>
    <p class="value" aria-describedby={reasonId}>{text === '' ? emptyValueLabel : text}</p>
    <p class="reason" id={reasonId}>{unknownKindLabel}</p>
  </div>
{:else if definition.kind === 'BOOL'}
  <Checkbox
    label={definition.label}
    hint={definition.hint}
    checked={value === true}
    {disabledReason}
    onchange={(event) => onChange?.((event.currentTarget as HTMLInputElement).checked)}
  />
{:else if definition.kind === 'SELECT'}
  <Select
    label={definition.label}
    hint={definition.hint}
    isRequired={definition.isRequired}
    {options}
    value={text}
    {disabledReason}
    onchange={(event) => onChange?.((event.currentTarget as HTMLSelectElement).value)}
  />
{:else if definition.kind === 'MULTI_SELECT'}
  <!-- A group of checkboxes rather than a native multi-select list: that one is operable in
       theory and unusable in practice, because holding a modifier while clicking is a gesture
       most people have never been taught and no touch device offers. -->
  <fieldset class="group" aria-describedby={disabledReason === undefined ? undefined : groupId} bind:this={container}>
    <legend>
      {definition.label}
      {#if definition.isRequired}<span class="required" aria-hidden="true">*</span>{/if}
      <ReasonMark id={groupId} text={disabledReason} {container} />
    </legend>
    {#if definition.hint}<p class="hint">{definition.hint}</p>{/if}
    {#each definition.options ?? [] as option (option)}
      <Checkbox
        label={option}
        checked={chosen.includes(option)}
        {disabledReason}
        onchange={() => toggle(option)}
      />
    {/each}
  </fieldset>
{:else if definition.kind === 'USER' || definition.kind === 'USER_SET'}
  <div class="slot">
    <p class="label">
      {definition.label}
      {#if definition.isRequired}<span class="required" aria-hidden="true">*</span>{/if}
    </p>
    {#if definition.hint}<p class="hint">{definition.hint}</p>{/if}
    {@render user?.({ value, multiple: definition.kind === 'USER_SET' })}
  </div>
{:else if definition.kind === 'REFERENCE'}
  <div class="slot">
    <p class="label">
      {definition.label}
      {#if definition.isRequired}<span class="required" aria-hidden="true">*</span>{/if}
    </p>
    {#if definition.hint}<p class="hint">{definition.hint}</p>{/if}
    {@render reference?.({ value })}
  </div>
{:else if definition.kind === 'RICH_TEXT'}
  <!-- Markdown, written as text: the formatting is read where the value is shown, and the field
       keeps every line break the writer typed. -->
  <Textarea
    label={definition.label}
    hint={definition.hint}
    isRequired={definition.isRequired}
    rows={4}
    value={text}
    {disabledReason}
    oninput={(event) => onChange?.((event.currentTarget as HTMLTextAreaElement).value)}
  />
{:else if isNumeric && reading}
  <!-- A text field with a decimal keypad rather than a native number input, so that the mark the
       reader writes with is the one the field accepts on every engine - and the value that leaves
       is the contract's. -->
  <Input
    label={definition.label}
    hint={definition.hint}
    isRequired={definition.isRequired}
    type="text"
    inputmode="decimal"
    value={numberText}
    error={numberInvalid ? reading.invalidLabel : undefined}
    {disabledReason}
    oninput={(event) => {
      const raw = (event.currentTarget as HTMLInputElement).value;
      numberDraft = raw;
      const parsed = reading.parse(raw);
      if (parsed !== undefined) onChange?.(parsed);
    }}
    onblur={() => {
      // A draft that became a number is shown as the locale writes it; one that did not stays.
      if (!numberInvalid) numberDraft = undefined;
    }}
  />
{:else}
  <Input
    label={definition.label}
    hint={definition.hint}
    isRequired={definition.isRequired}
    type={isNumeric ? 'number' : definition.kind === 'DATE' ? 'date' : definition.kind === 'URL' ? 'url' : 'text'}
    value={text}
    {disabledReason}
    oninput={(event) => {
      const raw = (event.currentTarget as HTMLInputElement).value;
      onChange?.(isNumeric ? (raw === '' ? null : Number(raw)) : raw);
    }}
  />
{/if}

<style>
  .unknown,
  .slot { display: flex; flex-direction: column; gap: var(--sp-050); min-width: 0; }

  .label,
  .group legend {
    margin: 0;
    padding: 0;
    color: var(--text-primary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .required { color: var(--text-danger); }

  .value { margin: 0; color: var(--text-primary); overflow-wrap: anywhere; }

  .hint,
  .reason { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  .group {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    margin: 0;
    border: 0;
    padding: 0;
    min-width: 0;
  }
</style>
