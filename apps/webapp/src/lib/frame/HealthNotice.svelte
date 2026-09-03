<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // design-system.md §4's `HealthBanner`, as F1-06 decided it: a `Banner` with content rather than
  // a component of its own. The design system's own `HealthBanner` is a wave-3 entry and stays
  // unbuilt - a component that arrived here out of turn would be exactly what that inventory
  // exists to prevent.
  //
  // It shows nothing at all in the ordinary case, which is most of the time: no bearer, no report,
  // a refusal, or an installation that is simply healthy.

  import { Banner, Stack } from '@hubtask/design-system/components';

  import { health } from '../data/health.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { session } from '../session.svelte.ts';

  // Read again when the session changes: without a bearer there is nothing to read, and the
  // subscription taken before somebody signed in is one the sign-out already dropped.
  $effect(() => {
    void session.status;
    return health.start();
  });
</script>

{#if health.isTroubled}
  <!-- `down` is red and `degraded` is amber: rule 3 says the words carry it too, which is what the
       lines below are - each one names a feature and the reason it is not working. -->
  <Banner tone={health.isDown ? 'danger' : 'warning'} title={t('app.health.title')}>
    <Stack gap="050">
      {#each health.degradations as degradation (degradation.feature)}
        <span>{t('app.health.feature', { feature: degradation.feature, reason: t(degradation.reasonCode) })}</span>
      {/each}
    </Stack>
  </Banner>
{/if}
