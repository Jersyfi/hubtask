<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { Callout, ParameterTable } from '@hubtask/design-system/components';

  import { eventSchemas, type EventSchema } from '$lib/api/document.ts';
  import { firstSentence } from '$lib/api/markdown.ts';
  import type { Row } from '$lib/api/reference.ts';

  const headings = { name: 'Name', type: 'Type', required: 'Required', description: 'Description' };
  const words = { required: 'required', optional: 'optional', deprecated: 'deprecated' };

  const typeOf = (type: string | readonly string[] | undefined, fallback: string): string =>
    Array.isArray(type) ? type.join(' or ') : typeof type === 'string' ? type : fallback;

  /** The schema's top-level fields as rows; the `data` object one level in. */
  function rowsOf(schema: EventSchema): Row[] {
    const required = new Set(schema.required ?? []);
    return Object.entries(schema.properties ?? {}).map(([name, property]) => {
      const type = typeOf(property.type, property.const !== undefined ? 'string' : 'object');
      const facts: string[] = [];
      if (property.const !== undefined) facts.push(`const: ${String(property.const)}`);
      if (property.format) facts.push(`format: ${property.format}`);
      const nested = property.properties as Readonly<Record<string, { description?: string; type?: string | readonly string[]; format?: string }>> | undefined;
      const children: Row[] = Object.entries(nested ?? {}).map(([child, inner]) => ({
        name: child,
        type: typeOf(inner.type, 'object'),
        isRequired: false,
        description: firstSentence(inner.description ?? ''),
        ...(inner.format ? { facts: [`format: ${inner.format}`] } : {}),
      }));
      return {
        name,
        type: property.format ? `${type} (${property.format})` : type,
        isRequired: required.has(name),
        description: firstSentence(property.description ?? ''),
        ...(facts.length > 0 ? { facts } : {}),
        ...(children.length > 0 ? { children } : {}),
      };
    });
  }
</script>

<svelte:head>
  <title>Events — API reference</title>
  <meta name="description" content="The CloudEvents every Hubtask installation publishes: one JSON schema per event type, delivered to webhook subscriptions and answered by trigger polling." />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker"><a href="/developers/api/">API reference</a> · events</p>
    <h1>The events</h1>
    <p class="lede">
      {eventSchemas.length} event types, each a CloudEvents 1.0 document with the schema below. A webhook
      subscription is sent the same document a poll of <code>/integrations/triggers/&lbrace;eventType&rbrace;</code>
      answers — one schema, two transports.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <Callout title="Stable by version">
      <p>
        The type name carries its version (<code>.v1</code>). A field may be added to a version; a field is never removed
        from one, and a change that would remove one is a new version beside the old.
      </p>
    </Callout>
    <div class="site-api-operations">
      {#each eventSchemas as [type, schema] (type)}
        <section class="site-api-schema" id={type}>
          <h2><a href="#{type}"><code>{type}</code></a></h2>
          {#if schema.description}<p>{firstSentence(schema.description)}</p>{/if}
          <ParameterTable label={type} isLabelHidden {headings} {words} rows={rowsOf(schema)} />
        </section>
      {/each}
    </div>
  </div>
</section>
