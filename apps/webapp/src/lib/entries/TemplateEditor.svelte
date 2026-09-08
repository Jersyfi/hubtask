<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The shape a template stamps out.
  //
  // **The tree travels whole.** A `PATCH` carries `nodes` entire — "a tree is a shape rather than a
  // list of settings, and half of one is a different shape" — so every edit here rebuilds the tree
  // and the save sends what is on screen.
  //
  // **The editor cannot build a tree the definition would refuse.** Which types may sit under which,
  // and how deep, come from `item_types[]`: an activity is offered no children, and neither is a
  // node at `max_depth`. The server's refusal is the fallback rather than the plan — and when one
  // does arrive, it names a node (`/nodes/0/children/1/title`) and the sentence lands on that node.
  //
  // **A node's due date is a duration**, never a date: "a template outlives the week it was written
  // in". It is entered as an amount and a unit and shown as the duration it is.
  //
  // **The cap is the manifest's.** `max_template_nodes` is shown against the count, and the add
  // controls stop at it.

  import {
    AssigneeControl,
    Button,
    Checkbox,
    Input,
    Select,
    Stack,
    Textarea,
  } from '@hubtask/design-system/components';
  import type { Template, TemplateNode } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { rootTypes } from '../data/capability.svelte.ts';
  import { durationOf, partsOf, type Unit } from '../data/duration.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import {
    addUnder,
    childTypesAt,
    countNodes,
    fieldPathOf,
    fitsInCap,
    nodeCapOf,
    removeAt,
    replaceAt,
    walk,
  } from '../data/templates.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { renderProblem } from '../problem.ts';

  interface Props {
    /** The template being changed, or nothing while one is being defined. */
    template?: Template;
    nodes: readonly TemplateNode[];
    path: Path;
    /** A refusal from the last save, so a node-level field error lands on its node. */
    failure?: ReturnType<typeof renderProblem>;
    onnodes: (nodes: readonly TemplateNode[]) => void;
  }

  const { template, nodes, path, failure, onnodes }: Props = $props();

  const cap = $derived(nodeCapOf(manifest.value?.limits as Record<string, unknown> | undefined));
  const count = $derived(countNodes(nodes));
  const mayAdd = $derived(fitsInCap(nodes, cap));
  const walked = $derived(walk(nodes));

  const candidateIds = $derived(people.candidates(path));
  const candidates = $derived(
    candidateIds.map((id) => ({ id, name: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  /** The types a template's first node may be: the ones a collection takes directly. */
  const roots = $derived(rootTypes());

  function change(nodePath: readonly number[], patch: Partial<TemplateNode>) {
    const current = walked.find((each) => each.path.join() === nodePath.join())?.node;
    if (!current) return;
    onnodes(replaceAt(nodes, nodePath, { ...current, ...patch }));
  }

  function addChild(nodePath: readonly number[], type: string) {
    onnodes(addUnder(nodes, nodePath, { type, title: '' } as TemplateNode));
  }

  /** The amount-and-unit view of a node's offset, or nothing where it has none this can show. */
  function offsetParts(node: TemplateNode) {
    return node.due_offset ? partsOf(node.due_offset) : undefined;
  }

  function setOffset(nodePath: readonly number[], node: TemplateNode, patch: {
    amount?: number;
    unit?: Unit;
    isBefore?: boolean;
    clear?: boolean;
  }) {
    if (patch.clear) {
      // The flag qualifies a relative date, so it goes with it: the server refuses one without the
      // other by name (`templates.due_date_only_without_offset`).
      change(nodePath, { due_offset: null, due_date_only: false });
      return;
    }
    const held = offsetParts(node) ?? { amount: 1, unit: 'D' as Unit, isBefore: false };
    change(nodePath, {
      due_offset: durationOf(
        patch.amount ?? held.amount,
        patch.unit ?? held.unit,
        patch.isBefore ?? held.isBefore,
      ),
    });
  }
</script>

<Stack gap="150">
  <p class="count">
    {cap === undefined
      ? t('app.templates.node_count_unbounded', { count: String(count) })
      : t('app.templates.node_count', { count: String(count), maximum: String(cap) })}
  </p>

  <ul class="tree">
    {#each walked as walkedNode (walkedNode.path.join('-'))}
      {@const node = walkedNode.node}
      {@const nodePath = walkedNode.path}
      {@const offered = childTypesAt(manifest.value, node.type as string, walkedNode.depth)}
      {@const parts = offsetParts(node)}
      <li style:--depth={walkedNode.depth}>
        <div class="node">
          <Input
            label={t('app.templates.node_title')}
            value={node.title}
            error={failure?.fields.get(fieldPathOf(nodePath, 'title'))}
            onchange={(event: Event) =>
              change(nodePath, { title: (event.currentTarget as HTMLInputElement).value })}
          />
          <Textarea
            label={t('app.templates.node_notes')}
            value={node.notes ?? ''}
            rows={2}
            onchange={(event: Event) =>
              change(nodePath, { notes: (event.currentTarget as HTMLTextAreaElement).value || null })}
          />

          <div class="offset">
            {#if node.due_offset}
              <Input
                label={t('app.templates.node_offset_amount')}
                type="number"
                value={String(parts?.amount ?? 1)}
                error={failure?.fields.get(fieldPathOf(nodePath, 'due_offset'))}
                onchange={(event: Event) =>
                  setOffset(nodePath, node, {
                    amount: Number((event.currentTarget as HTMLInputElement).value),
                  })}
              />
              <Select
                label={t('app.templates.node_offset_unit')}
                size="sm"
                value={parts?.unit ?? 'D'}
                options={[
                  { value: 'W', label: t('app.reminders.unit_W') },
                  { value: 'D', label: t('app.reminders.unit_D') },
                  { value: 'H', label: t('app.reminders.unit_H') },
                  { value: 'M', label: t('app.reminders.unit_M') },
                ]}
                onchange={(event: Event) =>
                  setOffset(nodePath, node, {
                    unit: (event.currentTarget as HTMLSelectElement).value as Unit,
                  })}
              />
              <Checkbox
                label={t('app.templates.node_offset_before')}
                checked={parts?.isBefore ?? false}
                onchange={(event: Event) =>
                  setOffset(nodePath, node, {
                    isBefore: (event.currentTarget as HTMLInputElement).checked,
                  })}
              />
              <Checkbox
                label={t('app.templates.node_all_day')}
                checked={node.due_date_only ?? false}
                onchange={(event: Event) =>
                  change(nodePath, {
                    due_date_only: (event.currentTarget as HTMLInputElement).checked,
                  })}
              />
              <Button size="sm" tone="secondary" onclick={() => setOffset(nodePath, node, { clear: true })}>
                {t('app.templates.node_offset_none')}
              </Button>
            {:else}
              <Button size="sm" tone="secondary" onclick={() => setOffset(nodePath, node, {})}>
                {t('app.templates.node_offset')}
              </Button>
            {/if}
          </div>

          <AssigneeControl
            label={t('app.templates.node_assignee')}
            {candidates}
            selected={node.assignee_id ? [node.assignee_id] : []}
            selection="single"
            filterLabel={t('app.bulk.assignee_filter')}
            emptyLabel={t('app.bulk.assignee_empty')}
            noMatchLabel={t('app.bulk.assignee_no_match')}
            chosenLabel={t('app.bulk.assignee_chosen')}
            unassignedLabel={t('app.bulk.assignee_none')}
            onSelect={(ids) => change(nodePath, { assignee_id: ids[0] ?? null })}
          />

          <div class="node-actions">
            {#each offered as type (type)}
              <Button
                size="sm"
                tone="secondary"
                disabledReason={mayAdd ? undefined : t('app.templates.at_cap')}
                onclick={() => addChild(nodePath, type)}
              >
                {t('app.templates.add_child')} · {type}
              </Button>
            {/each}
            {#if offered.length === 0}
              <!-- Nothing may sit here, and saying so is better than an absent control: the
                   manifest decided it, not this screen. -->
              <span class="quiet">{t('app.templates.no_child_types')}</span>
            {/if}
            <Button size="sm" tone="secondary" onclick={() => onnodes(removeAt(nodes, nodePath))}>
              {t('app.templates.remove_node')}
            </Button>
          </div>
        </div>
      </li>
    {/each}
  </ul>

  <div class="node-actions">
    {#each roots as type (type)}
      <Button
        size="sm"
        tone="secondary"
        disabledReason={mayAdd ? undefined : t('app.templates.at_cap')}
        onclick={() => addChild([], type)}
      >
        {t('app.templates.add_root')} · {type}
      </Button>
    {/each}
  </div>

  {#if template}
    <p class="quiet">{template.name}</p>
  {/if}
</Stack>

<style>
  .tree { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-150); }

  .tree li { padding-left: calc(var(--depth) * var(--sp-300)); }

  .node {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
  }

  .offset { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .node-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }

  .count { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .quiet { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
