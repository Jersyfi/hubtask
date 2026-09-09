<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection, in both of its layouts, drawn with the product's own components - and it works.
  //
  // What works, and what makes it work: this site loads no JavaScript (`csr = false`, and
  // build/check-static.js fails the build on a script tag), so every interaction here is native
  // HTML plus a `:has()` selector. Ticking an entry off is a real `<input type="checkbox">` inside
  // the real `Checkbox`; switching between list and board is a radio group; collapsing a work
  // package is a checkbox whose state a sibling selector reads.
  //
  // That constraint also says which components can appear. `Checkbox`, `ListRow`, `LabelChip`,
  // `BucketColumn` and `WorkItemCard` render their whole selves from markup and CSS. `ViewSwitcher`
  // does not - its choice runs through an `onclick`, so on this page its buttons would be furniture.
  // The switcher below is therefore the site's own, built from the same tokens and the same shape:
  // a recessed group, the chosen view carrying a surface *and* a border so the choice reads in
  // greyscale (design-system.md §6 rule 3), and a focus ring on each control (rule 5).
  //
  // Dragging a card between columns is the one thing asked for that is not here. Reordering by
  // pointer is a JavaScript feature, and no `:has()` selector substitutes for it - see
  // docs/marketing/website-1.0.md § 6.8 for the decision that would have to be taken first.

  import { BucketColumn, Checkbox, Icon, LabelChip, ListRow, WorkItemCard } from '@hubtask/design-system/components';

  // `name` keeps the input ids and radio groups apart when more than one specimen is on a page.
  let { name = 'demo' }: { name?: string } = $props();

  type Row = {
    id: string;
    title: string;
    type: 'task' | 'work-package' | 'activity';
    depth: 0 | 1 | 2;
    done?: boolean;
    label?: { name: string; token: string };
  };

  const rows: Row[] = [
    { id: 'domain', title: 'Renew the domain', type: 'task', depth: 0, label: { name: 'admin', token: 'violet' } },
    { id: 'registrar', title: 'Check what the registrar charges now', type: 'work-package', depth: 1 },
    { id: 'zone', title: 'Export the DNS zone', type: 'activity', depth: 2, done: true },
    { id: 'nameservers', title: 'Move the nameservers', type: 'activity', depth: 2 },
    { id: 'invoice', title: 'Update the standing invoice', type: 'work-package', depth: 1 },
    { id: 'milk', title: 'Buy milk', type: 'task', depth: 0, done: true, label: { name: 'errand', token: 'teal' } },
  ];

  type Card = { id: string; title: string; cover: string | null; done?: boolean; label?: { name: string; token: string } };
  type Column = { name: string; wipLimit?: number; isDone?: boolean; cards: Card[] };

  const columns: Column[] = [
    {
      name: 'To do',
      cards: [
        { id: 'domain', title: 'Renew the domain', cover: 'violet', label: { name: 'admin', token: 'violet' } },
        { id: 'note', title: 'Draft the release note', cover: null, label: { name: 'release', token: 'blue' } },
      ],
    },
    {
      name: 'In progress',
      wipLimit: 3,
      cards: [{ id: 'ns', title: 'Move the nameservers', cover: 'amber', label: { name: 'infra', token: 'amber' } }],
    },
    {
      name: 'Done',
      isDone: true,
      cards: [{ id: 'zone', title: 'Export the DNS zone', cover: null, done: true }],
    },
  ];
</script>

<figure class="specimen">
  <div class="specimen-frame">
    <div class="specimen-bar">
      <span class="specimen-where">
        <Icon name="hub" size="sm" /> Personal
        <span class="specimen-sep">/</span>
        <Icon name="collection" size="sm" /> Errands
      </span>

      <!-- The layout choice. A radio group rather than a tab strip, for the reason
           design-system.md §4 gives: a tab switches between subjects and owns the panel it
           reveals; this switches between renderings of one subject and owns nothing. -->
      <div class="specimen-views" role="radiogroup" aria-label="Layout">
        <input type="radio" name="{name}-view" id="{name}-view-list" data-view="list" checked />
        <label for="{name}-view-list"><Icon name="menu" size="sm" /> List</label>
        <input type="radio" name="{name}-view" id="{name}-view-board" data-view="board" />
        <label for="{name}-view-board"><Icon name="bucket" size="sm" /> Board</label>
      </div>
    </div>

    <div class="specimen-list">
      {#each rows as row (row.id)}
        <div class="specimen-row" data-depth={row.depth} data-kind={row.type}>
          <ListRow>
            {#snippet leading()}
              <!-- The twist sits on the work package that has children, and collapsing it hides
                   the activities that follow. A sibling selector does the hiding, so only one
                   entry in this fixture may carry children - noted where it could bite. -->
              <span class="specimen-twist">
                {#if row.id === 'registrar'}
                  <input type="checkbox" id="{name}-twist" data-twist aria-label="Hide the steps under this work package" />
                  <label for="{name}-twist"><Icon name="chevron-down" size="sm" /></label>
                {/if}
              </span>
              <span class="specimen-mark"><Icon name={row.type} size="sm" /></span>
              <Checkbox label="Complete {row.title}" isLabelHidden checked={row.done} />
            {/snippet}
            {#snippet trailing()}
              {#if row.label}
                <LabelChip name={row.label.name} colorToken={row.label.token} />
              {/if}
            {/snippet}
            <span class="specimen-title">{row.title}</span>
          </ListRow>
        </div>
      {/each}
    </div>

    <div class="specimen-board">
      {#each columns as column (column.name)}
        <BucketColumn
          name={column.name}
          count={null}
          wipLimit={column.wipLimit ?? null}
          isDoneBucket={column.isDone ?? false}
          doneBucketLabel="Dropping here completes an entry"
          overLimitLabel="Over the limit this column announces"
        >
          {#each column.cards as card (card.id)}
            <WorkItemCard
              title={card.title}
              isCompleted={card.done ?? false}
              coverKind={card.cover ? 'COLOR' : null}
              coverColorToken={card.cover}
            >
              {#snippet footer()}
                {#if card.label}
                  <LabelChip name={card.label.name} colorToken={card.label.token} />
                {/if}
              {/snippet}
            </WorkItemCard>
          {/each}
        </BucketColumn>
      {/each}
    </div>
  </div>

  <figcaption>
    The real components, and they work: tick an entry off, collapse the steps under the work
    package, or switch the same collection between its list and its board. Nothing here is a
    screenshot &mdash; a screenshot goes stale the day a token moves.
  </figcaption>
</figure>
