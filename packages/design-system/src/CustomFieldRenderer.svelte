<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A field the installation added, drawn as whatever its kind says it is.
  //
  // **Eight kinds, and a ninth this component has never heard of.** The schema has carried
  // `TEXT`, `NUMBER`, `DATE`, `SELECT`, `MULTI_SELECT`, `BOOL`, `USER` and `URL` since the first
  // migration, and a client one version behind a server will meet a kind that is not among them.
  // Tolerance towards unknown fields is a binding client requirement, so an unknown kind is
  // rendered **read-only with the reason** rather than dropped: a value the reader cannot see is
  // indistinguishable from a value that is not there, and the second one is a data-loss report.
  //
  // **`USER` is a slot.** Who may be named in this container is the domain's answer (F3-07), so
  // the picker is handed in rather than imported — which is also what keeps this component from
  // acquiring a dependency on the people half of the model to draw the seven kinds that have
  // nothing to do with people.
  //
  // The key is an identifier and never a label: `custom_fields.<key>` appears in filters and can
  // never be renamed. What a person reads is `label`, resolved by the caller (ADR-0011).

  import type { Snippet } from 'svelte';

  import Checkbox from './Checkbox.svelte';
  import Input from './Input.svelte';
  import Select from './Select.svelte';
  import type { Disableable } from './control.ts';

  /** The eight the contract declares. A string beside them is a kind from a newer server. */
  export type CustomFieldKind =
    | 'TEXT'
    | 'NUMBER'
    | 'DATE'
    | 'SELECT'
    | 'MULTI_SELECT'
    | 'BOOL'
    | 'USER'
    | 'URL';

  /** One definition, narrowed to what is drawn. The shape `CustomFieldDefinition` has. */
  export interface FieldDefinition {
    /** The identifier the value is stored under. Never shown: it is a key, not a label. */
    readonly key: string;
    /** What a person reads. Resolved by the caller, because this package writes no text. */
    readonly label: string;
    /** One of the eight, or a kind from a server this client has not caught up with. */
    readonly kind: CustomFieldKind | (string & {});
    /** The permitted values of a `SELECT` or a `MULTI_SELECT`, and empty for every other kind. */
    readonly options?: readonly string[];
    /** Whether it has to hold a value. Shown, because a reader owes nothing they were not told. */
    readonly isRequired?: boolean;
    /** Standing help under the label. */
    readonly hint?: string;
  }

  /** What a field can hold. `null` is "not set", which is a different thing from an empty string. */
  export type FieldValue = string | number | boolean | readonly string[] | null;

  interface Props extends Disableable {
    definition: FieldDefinition;
    value?: FieldValue;
    /**
     * The picker for a `USER` field, handed in rather than imported. Absent renders the kind as
     * unknown — which is honest: without a picker there is nothing to draw.
     */
    user?: Snippet<[{ value: FieldValue }]>;
    /**
     * Why a kind this client does not know cannot be edited. Required, for the reason
     * `disabledReason` is: a control that cannot be used owes the reader the reason.
     */
    unknownKindLabel: string;
    /** What "no value" reads as, where a value is only shown rather than edited. */
    emptyValueLabel: string;
    onChange?: (value: FieldValue) => void;
  }

  const {
    definition,
    value = null,
    user,
    unknownKindLabel,
    emptyValueLabel,
    disabledReason,
    onChange,
  }: Props = $props();

  const options = $derived((definition.options ?? []).map((option) => ({ value: option, label: option })));
  const chosen = $derived(Array.isArray(value) ? (value as readonly string[]) : []);
  const text = $derived(value === null || value === undefined ? '' : String(value));

  const KNOWN = new Set(['TEXT', 'NUMBER', 'DATE', 'SELECT', 'MULTI_SELECT', 'BOOL', 'USER', 'URL']);
  /** A `USER` field with no picker is as undrawable as a kind nobody has heard of, so it is one. */
  const isUnknown = $derived(!KNOWN.has(definition.kind) || (definition.kind === 'USER' && !user));

  const reasonId = $derived(isUnknown ? `unknown-${Math.random().toString(36).slice(2, 9)}` : undefined);

  function toggle(option: string) {
    onChange?.(chosen.includes(option) ? chosen.filter((held) => held !== option) : [...chosen, option]);
  }
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
  <fieldset class="group">
    <legend>
      {definition.label}
      {#if definition.isRequired}<span class="required" aria-hidden="true">*</span>{/if}
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
{:else if definition.kind === 'USER'}
  <div class="slot">
    <p class="label">
      {definition.label}
      {#if definition.isRequired}<span class="required" aria-hidden="true">*</span>{/if}
    </p>
    {@render user?.({ value })}
  </div>
{:else}
  <Input
    label={definition.label}
    hint={definition.hint}
    isRequired={definition.isRequired}
    type={definition.kind === 'NUMBER' ? 'number' : definition.kind === 'DATE' ? 'date' : definition.kind === 'URL' ? 'url' : 'text'}
    value={text}
    {disabledReason}
    oninput={(event) => {
      const raw = (event.currentTarget as HTMLInputElement).value;
      onChange?.(definition.kind === 'NUMBER' ? (raw === '' ? null : Number(raw)) : raw);
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
