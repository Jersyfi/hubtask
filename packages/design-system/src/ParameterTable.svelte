<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The fields of a request, a response or a schema, one per row.
  //
  // A `Table` underneath, because that is what it is: a name, a type, whether it is required and
  // what it means are four columns a screen reader should be able to read down. The component
  // adds what a reference needs on top of the table - the name in the mono face, the type as a
  // small mark, "required" as a word rather than an asterisk (rule 3: a symbol alone is a
  // convention some readers were never taught), and a nested table for an object's own fields,
  // indented by depth rather than repeated as a second heading.
  //
  // Every word is resolved text handed in (ADR-0011): the column headings and the two words for
  // required and optional come from the caller's catalogue, never from here.

  import Table, { type Column } from './Table.svelte';

  export interface Parameter {
    readonly name: string;
    /** `string`, `integer`, `array of Label`, … - as the caller spells it. */
    readonly type: string;
    readonly isRequired: boolean;
    /** Resolved text, or empty. */
    readonly description: string;
    /** An enum's values, a default, a format - small facts drawn beneath the type. */
    readonly facts?: readonly string[];
    /** Marked, never hidden: a reader of an old integration needs to find it. */
    readonly isDeprecated?: boolean;
    /** The fields of an object, drawn beneath it one level in. */
    readonly children?: readonly Parameter[];
  }

  interface Props {
    /** What the table is: "Query parameters", "Response body". Becomes its caption. */
    label: string;
    isLabelHidden?: boolean;
    /** The four column headings, resolved. */
    headings: { readonly name: string; readonly type: string; readonly required: string; readonly description: string };
    /** The two words the required column shows. */
    words: { readonly required: string; readonly optional: string; readonly deprecated: string };
    rows: readonly Parameter[];
  }

  const { label, isLabelHidden = false, headings, words, rows }: Props = $props();

  const columns = $derived<readonly Column[]>([
    { id: 'name', label: headings.name },
    { id: 'type', label: headings.type },
    { id: 'required', label: headings.required },
    { id: 'description', label: headings.description },
  ]);

  /** Depth-first, with the depth beside each row, so the table stays one `<tbody>`. */
  function flatten(list: readonly Parameter[], depth = 0): { row: Parameter; depth: number }[] {
    return list.flatMap((row) => [{ row, depth }, ...flatten(row.children ?? [], depth + 1)]);
  }
  const flat = $derived(flatten(rows));
</script>

<Table {label} {isLabelHidden} {columns}>
  {#each flat as { row, depth } (`${depth}:${row.name}`)}
    <tr data-depth={depth} class:is-deprecated={row.isDeprecated}>
      <td><code class="name">{row.name}</code>{#if row.isDeprecated}<span class="deprecated">{words.deprecated}</span>{/if}</td>
      <td>
        <code class="type">{row.type}</code>
        {#if row.facts && row.facts.length > 0}
          <ul class="facts">
            {#each row.facts as fact (fact)}<li><code>{fact}</code></li>{/each}
          </ul>
        {/if}
      </td>
      <td><span class="required" data-required={row.isRequired}>{row.isRequired ? words.required : words.optional}</span></td>
      <td class="description">{row.description}</td>
    </tr>
  {/each}
</Table>

<style>
  .name, .type, .facts code {
    font-family: var(--font-mono);
    font-size: var(--fs-075);
  }
  .name { font-weight: var(--fw-medium); color: var(--text-primary); }
  .type { color: var(--text-secondary); }

  /* One level in per depth, on the name cell's start side. Four steps are as deep as any schema
     in the contract goes; a fifth reads like the fourth rather than falling off the table. */
  tr[data-depth='1'] td:first-child { padding-inline-start: var(--sp-400); }
  tr[data-depth='2'] td:first-child { padding-inline-start: var(--sp-600); }
  tr[data-depth='3'] td:first-child { padding-inline-start: var(--sp-800); }
  tr[data-depth='4'] td:first-child { padding-inline-start: var(--sp-1000); }

  .facts {
    margin: var(--sp-050) 0 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--sp-025);
    color: var(--text-subtle);
  }

  .required { font-size: var(--fs-075); color: var(--text-secondary); }
  .required[data-required='true'] { color: var(--text-warning); font-weight: var(--fw-medium); }

  .deprecated {
    margin-inline-start: var(--sp-100);
    font-size: var(--fs-050);
    text-transform: uppercase;
    color: var(--text-danger);
  }
  .is-deprecated .name { text-decoration: line-through; }

  .description {
    max-width: 60ch;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }
</style>
