<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ParameterTable, { type Parameter } from './ParameterTable.svelte';

  const { isLong = false }: { isLong?: boolean } = $props();

  const rows: readonly Parameter[] = [
    { name: 'collection_id', type: 'string (uuid)', isRequired: true, description: 'The collection the entry is created in.' },
    { name: 'type', type: 'string', isRequired: true, description: 'What level the entry is.', facts: ['TASK', 'WORK_PACKAGE', 'ACTIVITY'] },
    { name: 'title', type: 'string', isRequired: true, description: 'Up to 500 code points.', facts: ['maxLength: 500'] },
    {
      name: 'due',
      type: 'object',
      isRequired: false,
      description: 'When it is due; absent means undated.',
      children: [
        { name: 'at', type: 'string (date-time)', isRequired: true, description: 'The instant, RFC 3339.' },
        { name: 'time_zone', type: 'string', isRequired: false, description: 'The zone the date was set in.', facts: ['default: the account\'s'] },
      ],
    },
    { name: 'assignee', type: 'string (uuid)', isRequired: false, description: 'Replaced by assignee_id in 0.3.0.', isDeprecated: true },
  ];

  const headings = $derived(
    isLong
      ? { name: 'Bezeichnung', type: 'Datentyp', required: 'Erforderlich', description: 'Beschreibung' }
      : { name: 'Name', type: 'Type', required: 'Required', description: 'Description' },
  );
  const words = $derived(
    isLong
      ? { required: 'erforderlich', optional: 'optional', deprecated: 'veraltet' }
      : { required: 'required', optional: 'optional', deprecated: 'deprecated' },
  );
</script>

<ParameterTable label={isLong ? 'Anfragekörper' : 'Request body'} {headings} {words} {rows} />
