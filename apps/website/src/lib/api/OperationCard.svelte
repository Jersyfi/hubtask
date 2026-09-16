<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One operation of the reference: the card, the description, the parameters, the body, the
  // responses, the codes, and the curl line. Every word the components need is handed in here,
  // once, because the site is English and the components know no language.
  import { ApiEndpointCard, Callout, CodeBlock, ParameterTable } from '@hubtask/design-system/components';

  import Markdown from './Markdown.svelte';
  import { catalogueUrl } from './document.ts';
  import type { Operation, Row } from './reference.ts';

  const { operation }: { operation: Operation } = $props();

  const headings = { name: 'Name', type: 'Type', required: 'Required', description: 'Description' };
  const words = { required: 'required', optional: 'optional', deprecated: 'deprecated' };

  const tables = $derived<readonly { label: string; rows: readonly Row[] }[]>(
    [
      { label: 'Path parameters', rows: operation.pathParameters },
      { label: 'Query parameters', rows: operation.queryParameters },
      { label: 'Headers', rows: operation.headerParameters },
    ].filter((table) => table.rows.length > 0),
  );
</script>

<ApiEndpointCard
  method={operation.method}
  path={operation.path}
  summary={operation.summary}
  id={operation.id}
  deprecatedLabel={operation.isDeprecated ? 'deprecated' : undefined}
>
  {#if operation.description}
    <Markdown source={operation.description} />
  {/if}

  {#if !operation.needsBearer}
    <Callout tone="warning" title="No bearer">
      <p>This route is public by design: it carries its own credential in the address, or none at all.</p>
    </Callout>
  {/if}

  {#each tables as table (table.label)}
    <ParameterTable label={table.label} {headings} {words} rows={table.rows} />
  {/each}

  {#if operation.requestBody}
    <div class="site-api-body">
      <p class="site-api-subhead">
        Request body <code>{operation.requestBody.contentType}</code>
        {#if operation.requestBody.schemaName}
          — <a href="/developers/api/schemas/#{operation.requestBody.schemaName}">{operation.requestBody.schemaName}</a>
        {/if}
      </p>
      {#if operation.requestBody.rows.length > 0}
        <ParameterTable label="Request body" isLabelHidden {headings} {words} rows={operation.requestBody.rows} />
      {/if}
    </div>
  {/if}

  <div class="site-api-body">
    <p class="site-api-subhead">Responses</p>
    <ul class="site-api-responses">
      {#each operation.responses as response (response.status)}
        <li>
          <code class="site-api-status">{response.status}</code>
          <span>{response.description}</span>
          {#if response.body?.schemaName}
            <a href="/developers/api/schemas/#{response.body.schemaName}">{response.body.schemaName}</a>
          {/if}
        </li>
      {/each}
    </ul>
  </div>

  {#if operation.codes.length > 0}
    <div class="site-api-body">
      <p class="site-api-subhead">Codes this operation names</p>
      <ul class="site-api-codes">
        {#each operation.codes as code (code)}
          <li><a href={catalogueUrl}><code>{code}</code></a></li>
        {/each}
      </ul>
    </div>
  {/if}

  <CodeBlock code={operation.curl} label="Example" language="bash" />
</ApiEndpointCard>
