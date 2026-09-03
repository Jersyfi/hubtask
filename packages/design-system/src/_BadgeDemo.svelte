<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import Inline from './Inline.svelte';
  import Stack from './Stack.svelte';

  const { isLong = false }: { isLong?: boolean } = $props();

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
</Stack>
