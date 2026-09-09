<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The five levels, drawn with the product's own components rather than described.
  //
  // This is a picture of the interface, and it is built out of the real `ListRow`, `Checkbox`,
  // `Icon` and `LabelChip` - so it cannot drift from what the application looks like, and a token
  // change moves it with everything else. What it is not is a running application: the body is
  // `inert` and out of the accessibility tree, and the figure's caption carries the meaning in
  // words. A brochure that offered a checkbox which does nothing would be teaching the reader that
  // the product's controls do nothing.
  //
  // `TaskRow` would have been the obvious component and is deliberately not used: it writes
  // `style:--depth`, and this site's build fails on an inline style attribute. The indent below
  // travels as a `data-` attribute instead, which is the technique wave 0 settled on.

  import { Checkbox, Icon, LabelChip, ListRow } from '@hubtask/design-system/components';

  type Row = {
    title: string;
    type: 'task' | 'work-package' | 'activity';
    depth: 0 | 1 | 2;
    done?: boolean;
    label?: { name: string; token: string };
  };

  const rows: Row[] = [
    { title: 'Renew the domain', type: 'task', depth: 0, label: { name: 'admin', token: 'violet' } },
    { title: 'Check what the registrar charges now', type: 'work-package', depth: 1 },
    { title: 'Export the DNS zone', type: 'activity', depth: 2, done: true },
    { title: 'Move the nameservers', type: 'activity', depth: 2 },
    { title: 'Update the standing invoice', type: 'work-package', depth: 1 },
    { title: 'Buy milk', type: 'task', depth: 0, done: true, label: { name: 'errand', token: 'teal' } },
  ];
</script>

<figure class="specimen">
  <div class="specimen-frame" inert aria-hidden="true">
    <div class="specimen-bar">
      <span class="specimen-where">
        <Icon name="hub" size="sm" /> Personal
        <span class="specimen-sep">/</span>
        <Icon name="collection" size="sm" /> Errands
      </span>
      <span class="specimen-tools">
        <Icon name="search" size="sm" />
        <Icon name="funnel" size="sm" />
        <Icon name="saved-view" size="sm" />
      </span>
    </div>

    <div class="specimen-body">
      {#each rows as row (row.title)}
        <div class="specimen-row" data-depth={row.depth}>
          <ListRow>
            {#snippet leading()}
              <span class="specimen-mark"><Icon name={row.type} size="sm" /></span>
              <Checkbox label={row.title} isLabelHidden checked={row.done} />
            {/snippet}
            {#snippet trailing()}
              {#if row.label}
                <LabelChip name={row.label.name} colorToken={row.label.token} />
              {/if}
            {/snippet}
            <span class="specimen-title" data-done={row.done ? '' : undefined}>{row.title}</span>
          </ListRow>
        </div>
      {/each}
    </div>
  </div>

  <figcaption>
    A collection in the list layout: a task, the work packages under it, and the activities under
    those — each level carrying only what its capability profile allows. Drawn here with the same
    components the application is built from.
  </figcaption>
</figure>
