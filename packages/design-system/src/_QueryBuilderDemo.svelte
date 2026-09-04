<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { untrack } from 'svelte';

  import QueryBuilder, {
    type QueryCondition,
    type QueryFieldOption,
  } from './QueryBuilder.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'fields' }: { mode?: 'fields' | 'absence' | 'nothing' } = $props();

  // What a manifest reports, in the shape the caller hands over. The component knows none of these
  // names — it is handed them, which is the whole point of the story below.
  const fields: QueryFieldOption[] = [
    {
      id: 'title',
      label: 'Title',
      operators: [
        { id: 'CONTAINS', label: 'contains', takesValue: true },
        { id: 'STARTS_WITH', label: 'starts with', takesValue: true },
      ],
    },
    {
      id: 'is_completed',
      label: 'Done',
      operators: [{ id: 'EQ', label: 'is', takesValue: true }],
      values: [
        { value: 'true', label: 'Yes' },
        { value: 'false', label: 'No' },
      ],
    },
    {
      id: 'label_ids',
      label: 'Labels',
      operators: [
        { id: 'CONTAINS_ANY', label: 'is any of', takesValue: true, hint: 'Separate them with commas' },
      ],
    },
    {
      id: 'due_at',
      label: 'Due',
      operators: [
        { id: 'LTE', label: 'is on or before', takesValue: true, hint: '@today, or a date' },
        { id: 'IS_NULL', label: 'is not set', takesValue: false },
      ],
    },
  ];

  // The rows the story opens with. `untrack` says the once is deliberate: the workbench sets
  // `mode` when it mounts the story and never again, and the reader edits the rows from there.
  let conditions = $state<QueryCondition[]>(
    untrack(() =>
      mode === 'absence'
        ? [{ field: 'due_at', op: 'IS_NULL', value: '' }]
        : [{ field: 'title', op: 'CONTAINS', value: 'milk' }],
    ),
  );
</script>

<Stack gap="200">
  <QueryBuilder
    label="Which entries to show"
    fields={mode === 'nothing' ? [] : fields}
    bind:conditions
    fieldLabel="Field"
    operatorLabel="Comparison"
    valueLabel="Value"
    addLabel="Add a condition"
    removeLabel="Remove this condition"
    emptyLabel="This installation reports nothing that can be filtered on."
  />
  <p>{conditions.length} condition(s).</p>
</Stack>
