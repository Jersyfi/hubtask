<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import CodeBlock from './CodeBlock.svelte';
  import Stack from './Stack.svelte';

  const { hasCopy = true, isLong = false }: { hasCopy?: boolean; isLong?: boolean } = $props();

  const request = $derived(
    isLong
      ? `curl -X POST "https://hubtask.example/api/v1/items" \\\n  -H "Authorization: Bearer hbt_pat_…" \\\n  -H "Idempotency-Key: $(uuidgen)" \\\n  -H "Content-Type: application/json" \\\n  -d '{"collection_id":"018f6e2a-…","type":"TASK","title":"Eine sehr lange Überschrift, die in keiner Zeile Platz findet und deshalb scrollt"}'`
      : `curl -X POST "https://hubtask.example/api/v1/items" \\\n  -H "Authorization: Bearer hbt_pat_…" \\\n  -d '{"collection_id":"…","type":"TASK","title":"Write the reference"}'`,
  );
  const response = `{\n  "id": "018f6e2a-2b7c-7c1e-9b1a-3d5e6f7a8b9c",\n  "type": "TASK",\n  "title": "Write the reference",\n  "version": 1\n}`;
</script>

<Stack gap="200">
  <CodeBlock code={request} label="Request" language="bash" copyLabel={hasCopy ? 'Copy' : undefined} copiedLabel="Copied" />
  <CodeBlock code={response} label="Response" language="json" />
</Stack>
