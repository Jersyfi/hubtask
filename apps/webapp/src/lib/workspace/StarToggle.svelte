<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // ☆ right after a head's title (UC-WRK-37, the mockup's heads): the same star as the sidebar's,
  // gold and filled once starred.
  import type { Star } from '@hubtask/sync-engine';
  import { IconButton } from '@hubtask/design-system/components';

  import { isStarred } from '../data/stars.ts';
  import { stars } from '../data/stars.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    kind: Star['kind'];
    id: string;
    /** The starred thing's name, for the button's label. */
    name: string;
  }
  const { kind, id, name }: Props = $props();

  const starred = $derived(isStarred(stars.all, kind, id));
</script>

<span class="star" data-starred={starred ? '' : undefined}>
  <IconButton
    icon="star"
    size="sm"
    label={t(starred ? 'app.nav.unstar' : 'app.nav.star', { name })}
    aria-pressed={starred}
    onclick={() => void stars.toggle(kind, id)}
  />
</span>

<style>
  .star { display: inline-flex; }
  .star[data-starred] :global(button) { color: var(--accent-star); }
  .star[data-starred] :global(svg) { fill: currentColor; }
</style>
