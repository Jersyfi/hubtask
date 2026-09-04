<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What happened to an entry, newest first.
  //
  // **`verb` is a code, and this component never sees one.** `domain-model.md` §3.5 stores
  // `item.completed` and the client renders `activity.item_completed`; a feed that wrote
  // "Completed" would be the message catalogue growing a second copy inside a component, which
  // ADR-0011 forbids and F1-07 built the renderer to prevent. So every string here arrives
  // resolved, and the component's whole job is the shape of a history.
  //
  // **An ordered list, and a real `<time>`.** The order is the content — newest first is what the
  // reader is being told — and a `<ul>` would say it was a set. `datetime` carries the machine
  // instant beside the reader's own formatting, which is what lets a screen reader and a browser
  // agree about a moment that is written differently in every locale.
  //
  // **A step with no change set is a shorter sentence, not an empty panel.** An activity's history
  // is compact by the capability matrix — the verb, the actor and the time are the whole of the
  // step — and drawing an empty detail area under each one would be inventing a gap the model does
  // not have.

  /** One step, entirely in resolved text (ADR-0011). */
  export interface ActivityStep {
    readonly id: string;
    /** The sentence: the verb's message, with the actor already in it. */
    readonly sentence: string;
    /** The moment, written for the reader's locale. */
    readonly when: string;
    /** The same moment as the machine has it, for `datetime`. */
    readonly at: string;
    /**
     * What moved. Empty for a compact history and for a step that moved no field.
     *
     * `detail` is what the history *kept* about the field and nothing more: a rename carries both
     * titles, a note carries that it changed and none of its text (ADR-0017). The component cannot
     * add to it — it has only what it was handed.
     */
    readonly changes: readonly { readonly field: string; readonly detail: string }[];
  }

  interface Props {
    /** What this history is of. A list of sentences with no name is a list nobody can place. */
    label: string;
    steps: readonly ActivityStep[];
    /** What to say when there is no history yet. */
    emptyLabel: string;
  }

  const { label, steps, emptyLabel }: Props = $props();
</script>

{#if steps.length === 0}
  <p class="empty">{emptyLabel}</p>
{:else}
  <ol class="feed" aria-label={label}>
    {#each steps as step (step.id)}
      <li class="step">
        <p class="sentence">{step.sentence}</p>
        <time class="when" datetime={step.at}>{step.when}</time>
        {#if step.changes.length > 0}
          <dl class="changes">
            {#each step.changes as change (change.field)}
              <dt>{change.field}</dt>
              <dd>{change.detail}</dd>
            {/each}
          </dl>
        {/if}
      </li>
    {/each}
  </ol>
{/if}

<style>
  .feed {
    display: flex;
    flex-direction: column;
    gap: var(--sp-200);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  /* The rail is what makes a list of moments read as a sequence. It is a border rather than a
     mark, so it costs nothing to a reader who does not see it — the order is in the markup. */
  .step {
    display: flex;
    flex-direction: column;
    gap: var(--sp-025);
    padding-inline-start: var(--sp-200);
    border-inline-start: var(--bw-thick) solid var(--border-subtle);
  }

  .sentence { margin: 0; font-size: var(--fs-100); overflow-wrap: anywhere; }

  .when { color: var(--text-subtle); font-size: var(--fs-075); }

  /* Field, then what the history kept about it. A description list, because that is what this is. */
  .changes {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: var(--sp-025) var(--sp-100);
    margin: var(--sp-050) 0 0;
    font-size: var(--fs-075);
  }

  .changes dt { color: var(--text-subtle); font-family: var(--font-mono); }

  .changes dd { margin: 0; color: var(--text-secondary); overflow-wrap: anywhere; }

  .empty { margin: 0; max-width: 64ch; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
