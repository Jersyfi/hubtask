<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one mark beside a value's name that says where it comes from and whether it is locked
  // (UC-WRK-32 check 5, P-06, P-07): `SourceChip` with the words of `lib/data/source.ts`. Every
  // administration screen that shows a value set above draws this, so the six phrasings it replaced
  // stay one. Nothing where the server named no source.

  import { SourceChip } from '@hubtask/design-system/components';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { chipCodes } from '../data/source.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The level the value came from, as the server names it. */
    source?: string | null;
    /** The level that locked it, where one did. */
    lock?: string | null;
    /** The value in force, as a person reads it. */
    value: string;
    /** Whether the value is read at a hub rather than the workspace. */
    atHub?: boolean;
  }

  const { source, lock, value, atHub = false }: Props = $props();

  const chip = $derived(chipCodes({ source, lock }, { operates: manifest.isInstanceReachable, atHub }));
</script>

{#if chip}
  <span data-source={chip.isLocked ? `${lock}-locked` : source}>
    <SourceChip
      label={t(chip.label)}
      isLocked={chip.isLocked}
      detailsLabel={t('app.source.details')}
      {value}
      source={t(chip.source)}
      whomToAsk={chip.whomToAsk ? t(chip.whomToAsk) : undefined}
    />
  </span>
{/if}
