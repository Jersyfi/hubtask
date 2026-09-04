<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The query language made visible: a list of conditions, each a field, a comparison and a value.
  //
  // **It knows no grammar.** Which fields exist, which comparisons each permits, and whether a
  // comparison takes a value at all are answered by the data it is handed — `/meta/capabilities`
  // on the other side of it, through the caller — because the set grows with the installation and
  // a component that spelled the operators out would be the hard-coded list the manifest exists to
  // replace. `takesValue` on the operator is the whole of what would otherwise be knowledge of
  // ADR-0026's grammar living in here.
  //
  // Rows are data rather than children, for `Menu`'s reason: "which condition is the third one" is
  // a question about a list, and a component that read its rows back out of the DOM could not be
  // checked without a browser.
  //
  // A condition is not applied by this component. It reports the list and the caller decides what
  // to do with it — which is what lets an unfinished row sit on the screen without narrowing
  // anything, and what keeps the decision about which conditions are sendable in one place.

  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Input from './Input.svelte';
  import Select from './Select.svelte';

  /** One comparison a field permits. `takesValue` is false for the ones that ask about absence. */
  export interface QueryOperator {
    readonly id: string;
    /** Resolved text (ADR-0011). */
    readonly label: string;
    readonly takesValue: boolean;
    /** Whether the value is a list, so the field can say how it should be typed. */
    readonly hint?: string;
  }

  /** One field the installation reports, with what may be asked of it. */
  export interface QueryFieldOption {
    readonly id: string;
    readonly label: string;
    readonly operators: readonly QueryOperator[];
    /** The permitted values, where the field has a fixed set. A chooser rather than a text box. */
    readonly values?: readonly { readonly value: string; readonly label: string }[];
  }

  /** One row. Text, because an input is text; the caller converts by the field's kind. */
  export interface QueryCondition {
    field: string;
    op: string;
    value: string;
  }

  interface Props {
    /** What the editor filters. A group of controls with no name is a row of verbs. */
    label: string;
    fields: readonly QueryFieldOption[];
    conditions: QueryCondition[];
    fieldLabel: string;
    operatorLabel: string;
    valueLabel: string;
    addLabel: string;
    removeLabel: string;
    /** What to say when the installation reports nothing that can be filtered on. */
    emptyLabel: string;
    onchange?: (conditions: readonly QueryCondition[]) => void;
  }

  let {
    label,
    fields,
    conditions = $bindable([]),
    fieldLabel,
    operatorLabel,
    valueLabel,
    addLabel,
    removeLabel,
    emptyLabel,
    onchange,
  }: Props = $props();

  const fieldNamed = (id: string) => fields.find((each) => each.id === id);

  function apply(next: QueryCondition[]) {
    conditions = next;
    onchange?.(next);
  }

  function add() {
    const first = fields[0];
    if (!first) return;
    apply([...conditions, { field: first.id, op: first.operators[0]?.id ?? '', value: '' }]);
  }

  function remove(index: number) {
    apply(conditions.filter((_, at) => at !== index));
  }

  /**
   * Changing the field resets the comparison, and that is not tidiness.
   *
   * The operators belong to the field: `CONTAINS` is a comparison a text field permits and a
   * boolean does not, so a row that kept its old operator would be a row the installation refuses
   * — built by the editor that is supposed to prevent exactly that.
   */
  function chooseField(index: number, id: string) {
    const field = fieldNamed(id);
    apply(
      conditions.map((condition, at) =>
        at === index ? { field: id, op: field?.operators[0]?.id ?? '', value: '' } : condition,
      ),
    );
  }

  function change(index: number, patch: Partial<QueryCondition>) {
    apply(conditions.map((condition, at) => (at === index ? { ...condition, ...patch } : condition)));
  }
</script>

<fieldset class="builder">
  <legend class="legend">{label}</legend>

  {#if fields.length === 0}
    <p class="empty">{emptyLabel}</p>
  {:else}
    {#each conditions as condition, index (index)}
      {@const field = fieldNamed(condition.field)}
      {@const operator = field?.operators.find((each) => each.id === condition.op)}
      <div class="condition">
        <Select
          label={fieldLabel}
          size="sm"
          value={condition.field}
          options={fields.map((each) => ({ value: each.id, label: each.label }))}
          onchange={(event) => chooseField(index, (event.currentTarget as HTMLSelectElement).value)}
        />
        <Select
          label={operatorLabel}
          size="sm"
          value={condition.op}
          options={(field?.operators ?? []).map((each) => ({ value: each.id, label: each.label }))}
          onchange={(event) => change(index, { op: (event.currentTarget as HTMLSelectElement).value })}
        />
        <!-- The third control is the operator's answer, not this component's: one that asks about
             absence has nothing to compare against, and drawing an empty box beside it would be
             asking the reader for something that will not be sent. -->
        {#if operator?.takesValue !== false}
          {#if field?.values}
            <Select
              label={valueLabel}
              size="sm"
              value={condition.value}
              options={field.values.map((each) => ({ value: each.value, label: each.label }))}
              onchange={(event) => change(index, { value: (event.currentTarget as HTMLSelectElement).value })}
            />
          {:else}
            <Input
              label={valueLabel}
              size="sm"
              hint={operator?.hint}
              value={condition.value}
              oninput={(event) => change(index, { value: (event.currentTarget as HTMLInputElement).value })}
            />
          {/if}
        {/if}
        <IconButton icon="x" label={removeLabel} size="sm" onclick={() => remove(index)} />
      </div>
    {/each}

    <div>
      <Button size="sm" tone="secondary" icon="plus" onclick={add}>{addLabel}</Button>
    </div>
  {/if}
</fieldset>

<style>
  .builder {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
    margin: 0;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
  }

  .legend { padding-inline: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); }

  /* Rule 4: the row wraps rather than squeezing three controls into a phone's width. German and
     Finnish do the same thing to it that a narrow screen does. */
  .condition {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--sp-100);
  }

  .empty { margin: 0; max-width: 64ch; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
