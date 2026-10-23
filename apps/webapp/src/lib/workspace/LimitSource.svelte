<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who set a limit, as the chip every value set above carries (UC-WRK-32 check 5): on *Limits*
  // and beside the feature the limit bounds (ux-inventory.md §E8), so the two places say it the
  // same way. Nothing where the server named no source; *Limits* then says it in words.

  import { manifest } from '../data/capabilities.svelte.ts';
  import type { Standing } from '../data/quotas.svelte.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import ValueSource from './ValueSource.svelte';

  const { standing }: { standing: Standing } = $props();

  // A single installation names no operator (deployments.md D1): its exception is the workspace's.
  const source = $derived(standing.source === 'EXCEPTION' && manifest.isSingleTenant ? 'WORKSPACE' : standing.source);

  /** The ceiling as a person reads it: `0` is no ceiling, attachment storage a size. */
  const value = $derived(
    standing.limit === 0
      ? t('app.quotas.unlimited')
      : standing.quota === 'media_bytes'
        ? formatBytes(standing.limit, messages.locale)
        : String(standing.limit),
  );
</script>

<ValueSource {source} lock={standing.lock} {value} />
