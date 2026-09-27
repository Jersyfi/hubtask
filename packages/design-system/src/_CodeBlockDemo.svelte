<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import CodeBlock, { type LineKind } from './CodeBlock.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'request', hasCopy = true }: {
    mode?: 'request' | 'numbered' | 'diff' | 'shell' | 'wrapped' | 'long';
    hasCopy?: boolean;
  } = $props();

  const request = `POST /v1/items HTTP/1.1
Host: hubtask.example
Content-Type: application/json
Idempotency-Key: 11111111-2222-4333-8444-555555555555

{
  "type": "TASK",
  "title": "Move the socket by the window",
  "collection_id": "01J9Z1Q4K7"
}`;

  const long = `curl -sS -X POST https://hubtask.example/v1/items -H 'Authorization: Bearer $HUBTASK_TOKEN' -H 'Content-Type: application/json' -d '{"type":"TASK","title":"Move the socket by the window","collection_id":"01J9Z1Q4K7"}'`;

  const diff = `{
  "type": "TASK",
  "title": "Move the socket by the window",
  "due_at": "2026-10-02T09:00:00Z",
  "due_date_only": false,
  "collection_id": "01J9Z1Q4K7"
}`;

  /** Which line is what. One-based, and the prose beside the block is what decides it. */
  const marks: Record<number, LineKind> = {
    2: 'faded',
    4: 'added',
    5: 'added',
    6: 'removed',
  };

  const shell = `$ hubctl auth login --workspace renovation
Signed in as anna@example.org
$ hubctl items create --type TASK --title 'Order the tiles'
01J9Z1Q4K7`;

  const copy = $derived(hasCopy ? { copyLabel: 'Copy', copiedLabel: 'Copied' } : {});
</script>

{#if mode === 'numbered'}
  <CodeBlock
    code={request}
    label="Create an entry"
    language="http"
    fileName="examples/create-item.http"
    hasLineNumbers
    lines={{ 4: 'marked' }}
    {...copy}
  />
{:else if mode === 'diff'}
  <CodeBlock
    code={diff}
    label="What changed in 0.9"
    language="json"
    hasLineNumbers
    lines={marks}
    {...copy}
  />
{:else if mode === 'shell'}
  <CodeBlock code={shell} label="Sign in and create an entry" language="bash" hasPrompts {...copy} />
{:else if mode === 'wrapped'}
  <Stack gap="200">
    <CodeBlock code={long} label="As one line" language="bash" hasPrompts {...copy} />
    <CodeBlock code={long} label="Wrapped" language="bash" hasPrompts isWrapped {...copy} />
  </Stack>
{:else if mode === 'long'}
  <CodeBlock code={long} label="One long line" language="bash" {...copy} />
{:else}
  <CodeBlock code={request} label="Create an entry" language="http" {...copy} />
{/if}
