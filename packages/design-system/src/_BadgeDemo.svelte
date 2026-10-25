<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import Inline from './Inline.svelte';
  import Stack from './Stack.svelte';

  const { isLong = false, hasBold = false, hasSmall = false }: { isLong?: boolean; hasBold?: boolean; hasSmall?: boolean } = $props();

  const words = $derived(
    isLong
      ? { run: 'Wird ausgeführt', ok: 'Erfolgreich abgeschlossen', warn: 'Überfällig', fail: 'Fehlgeschlagen', count: '12 Vorgänge' }
      : { run: 'Dry run', ok: 'Succeeded', warn: 'Overdue', fail: 'Failed', count: '12 items' },
  );
</script>

<Stack gap="200">
  <Inline gap="100" align="center">
    <Badge>{words.count}</Badge>
    <Badge tone="info">{words.run}</Badge>
    <Badge tone="success">{words.ok}</Badge>
    <Badge tone="warning">{words.warn}</Badge>
    <Badge tone="danger">{words.fail}</Badge>
  </Inline>
  {#if hasBold}
    <Inline gap="100" align="center">
      <Badge emphasis="bold">{words.count}</Badge>
      <Badge tone="info" emphasis="bold">{words.run}</Badge>
      <Badge tone="success" emphasis="bold">{words.ok}</Badge>
      <Badge tone="warning" emphasis="bold">{words.warn}</Badge>
      <Badge tone="danger" emphasis="bold">{words.fail}</Badge>
    </Inline>
  {/if}
  {#if hasSmall}
    <Inline gap="100" align="center">
      <Badge size="sm" icon="calendar">{isLong ? 'morgen 10:00' : 'tomorrow 10:00'}</Badge>
      <Badge size="sm" tone="warning" icon="clock">{isLong ? 'heute 15:00' : 'today 3:00 PM'}</Badge>
      <Badge size="sm" tone="danger">{words.warn} · {isLong ? 'Mo' : 'Mon'}</Badge>
    </Inline>
  {/if}
</Stack>
