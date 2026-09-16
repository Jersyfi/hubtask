<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ApiEndpointCard from './ApiEndpointCard.svelte';
  import Callout from './Callout.svelte';
  import CodeBlock from './CodeBlock.svelte';
  import Stack from './Stack.svelte';

  const { isLong = false }: { isLong?: boolean } = $props();
  const item = '/items/{itemId}';
  const summary = $derived(isLong ? 'Legt einen Eintrag in einer Sammlung an – eine Aufgabe, ein Arbeitspaket oder eine Aktivität, je nach Fähigkeitsprofil der Sammlung' : 'Create an entry');
</script>

<Stack gap="200">
  <ApiEndpointCard method="POST" path="/items" {summary} id="createWorkItem">
    <Callout title="Idempotent">
      <p>Send an <code>Idempotency-Key</code>; a repeat with the same key is answered from the stored response.</p>
    </Callout>
    <CodeBlock label="Request" language="bash" code={`curl -X POST "$HUBTASK/api/v1/items" -H "Authorization: Bearer $TOKEN" -d '{…}'`} />
  </ApiEndpointCard>
  <ApiEndpointCard method="GET" path={item} summary="Read one entry" id="getWorkItem" />
  <ApiEndpointCard method="PATCH" path={item} summary="Change an entry" id="updateWorkItem" />
  <ApiEndpointCard method="DELETE" path={item} summary="Move an entry to the trash" id="trashWorkItem" />
  <ApiEndpointCard method="POST" path={`${item}:auto-assign`} summary="Assign by the collection's strategy" id="autoAssign" deprecatedLabel="deprecated" />
</Stack>
