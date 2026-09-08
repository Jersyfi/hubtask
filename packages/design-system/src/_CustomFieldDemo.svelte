<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The eight kinds side by side, and the ninth nobody has heard of.
  //
  // Every label here is resolved English: the key is `warranty_until`, and what a person reads is
  // "Warranty until". The component never sees a key where a label belongs.

  import CustomFieldRenderer, {
    type FieldDefinition,
    type FieldValue,
  } from './CustomFieldRenderer.svelte';
  import AssigneeControl from './AssigneeControl.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'all' }: { mode?: 'all' | 'unknown' | 'user' } = $props();

  const definitions: { definition: FieldDefinition; value: FieldValue }[] = [
    { definition: { key: 'supplier', label: 'Supplier', kind: 'TEXT', isRequired: true }, value: 'Nordhaus GmbH' },
    { definition: { key: 'quantity', label: 'Quantity', kind: 'NUMBER', hint: 'Whole units only.' }, value: 12 },
    { definition: { key: 'warranty_until', label: 'Warranty until', kind: 'DATE' }, value: '2028-03-01' },
    {
      definition: { key: 'room', label: 'Room', kind: 'SELECT', options: ['Kitchen', 'Hall', 'Studio'], isRequired: true },
      value: 'Kitchen',
    },
    {
      definition: { key: 'trades', label: 'Trades involved', kind: 'MULTI_SELECT', options: ['Electrics', 'Plumbing', 'Joinery'] },
      value: ['Electrics', 'Joinery'],
    },
    { definition: { key: 'is_insured', label: 'Insured', kind: 'BOOL' }, value: true },
    { definition: { key: 'datasheet', label: 'Datasheet', kind: 'URL' }, value: 'https://example.org/datasheet.pdf' },
  ];

  const owner: FieldDefinition = { key: 'owner', label: 'Responsible', kind: 'USER', isRequired: true };

  const future: FieldDefinition = { key: 'signature', label: 'Signature', kind: 'SIGNATURE' };

  const people = [
    { id: 'p1', name: 'Amelie Fischer' },
    { id: 'p2', name: 'Jonas Brandt' },
  ];

  let held = $state<Record<string, FieldValue>>({});
  let responsible = $state<readonly string[]>(['p1']);
</script>

<Stack gap="150">
  {#if mode === 'all'}
    {#each definitions as field (field.definition.key)}
      <CustomFieldRenderer
        definition={field.definition}
        value={held[field.definition.key] ?? field.value}
        unknownKindLabel="This installation is newer than this client, so the field is shown as it was stored."
        emptyValueLabel="Not set"
        onChange={(next) => {
          held = { ...held, [field.definition.key]: next };
        }}
      />
    {/each}
  {/if}

  {#if mode === 'all' || mode === 'user'}
    <CustomFieldRenderer
      definition={owner}
      value={responsible[0] ?? null}
      unknownKindLabel="This installation is newer than this client, so the field is shown as it was stored."
      emptyValueLabel="Not set"
    >
      {#snippet user()}
        <AssigneeControl
          label="Responsible"
          candidates={people}
          selected={responsible}
          filterLabel="Filter people"
          emptyLabel="Nobody here can be named yet."
          noMatchLabel="No one matches that. Try part of a name."
          chosenLabel="Responsible"
          unassignedLabel="Nobody yet"
          onSelect={(ids) => {
            responsible = ids;
          }}
        />
      {/snippet}
    </CustomFieldRenderer>
  {/if}

  {#if mode === 'all' || mode === 'unknown'}
    <CustomFieldRenderer
      definition={future}
      value="signed 4 Sep 2026"
      unknownKindLabel="This installation is newer than this client, so the field is shown as it was stored."
      emptyValueLabel="Not set"
    />
  {/if}
</Stack>
