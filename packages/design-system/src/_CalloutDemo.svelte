<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Callout from './Callout.svelte';
  import Stack from './Stack.svelte';

  const { isLong = false }: { isLong?: boolean } = $props();
</script>

<Stack gap="200">
  <Callout title={isLong ? 'Idempotenzschlüssel sind Pflicht' : 'Idempotency keys are required'}>
    <p>
      {isLong
        ? 'Jede verändernde Anfrage trägt einen Idempotency-Key. Dieselbe Anfrage mit demselben Schlüssel wird genau einmal ausgeführt und beantwortet danach aus dem Gespeicherten.'
        : 'Every mutation carries an Idempotency-Key. The same request with the same key takes effect exactly once and is answered from the stored response afterwards.'}
    </p>
  </Callout>
  <Callout tone="warning" title="Concurrency">
    <p>A <code>PATCH</code> without <code>If-Match</code> is refused with <code>428</code>: a client that lost the race must not overwrite a change it never saw.</p>
  </Callout>
  <Callout tone="danger" title="Destructive">
    <p>This operation ends the workspace. It needs a step-up token and the workspace name typed exactly.</p>
  </Callout>
  <Callout tone="success">
    <p>A read. Repeat it as often as you like; nothing is written.</p>
  </Callout>
</Stack>
